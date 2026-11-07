#!/usr/bin/env python3
"""Bind release artifacts and local security evidence to one successful CI run."""
import argparse
import datetime as dt
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess

REPOSITORY = "minigo-storage/minigo"
ARCHES = ("amd64", "arm64")
SCANNER = "aquasec/trivy:0.76.2@sha256:8f6bb947dbc528902e11f2f5be52f7b519f6a39d5d148f41166f2040499768fb"


def digest(path):
    with Path(path).open("rb") as stream:
        return hashlib.file_digest(stream, "sha256").hexdigest()


def read(path):
    return json.loads(Path(path).read_text())


def require(condition, message):
    if not condition:
        raise ValueError(message)


def check_run(run, source_sha):
    require(run["conclusion"] == "success" and run["status"] == "completed", "candidate run must have completed successfully")
    require(run["head_branch"] == "main" and run["head_sha"] == source_sha, "candidate must match this main source commit")
    require(run["repository"]["full_name"] == REPOSITORY, "wrong candidate repository")
    require(run["path"] == ".github/workflows/publish-images.yml", "wrong candidate workflow")
    require(run["event"] == "workflow_dispatch", "candidate must be manually dispatched")


def inspect_artifacts(directory, run, source_sha):
    check_run(run, source_sha)
    images = {}
    common = None
    for arch in ARCHES:
        folder = Path(directory) / f"minigo-image-{arch}"
        candidate = read(folder / "candidate.json")
        require(candidate["architecture"] == arch, "candidate architecture mismatch")
        require(candidate["source_sha"] == source_sha, "candidate revision mismatch")
        require(candidate["run_id"] == str(run["id"]), "candidate run mismatch")
        require(candidate["run_attempt"] == str(run["run_attempt"]), "candidate attempt mismatch; download the current attempt")
        require(re.fullmatch(r"[0-9]+\.[0-9]+\.[0-9]+", candidate["version"]), "invalid release version")
        require(type(candidate["security_release"]) is bool, "missing security-release classification")
        require(candidate["increment"] in ("patch", "minor", "major"), "invalid release increment")
        metadata = {k: candidate[k] for k in ("source_sha", "run_id", "run_attempt", "version", "increment", "security_release")}
        require(common is None or common == metadata, "architectures belong to different candidates")
        common = metadata
        image = read(folder / "image.json")[0]
        require(image["Os"] == "linux" and image["Architecture"] == arch, "image platform mismatch")
        labels = image["Config"]["Labels"]
        require(labels["org.opencontainers.image.revision"] == source_sha, "image revision mismatch")
        require(labels["org.opencontainers.image.version"] == common["version"], "image version mismatch")
        checksum = digest(folder / "image.tar.gz")
        require(checksum == (folder / "image.tar.gz.sha256").read_text().split()[0], "image archive checksum mismatch")
        images[arch] = {"image_id": image["Id"], "archive_sha256": checksum}
    return {**common, "images": images}


def check_evidence(directory, candidate, expected_digest=None, now=None):
    directory = Path(directory)
    receipt_path = directory / "security-assessment.json"
    if expected_digest:
        require(re.fullmatch(r"[0-9a-f]{64}", expected_digest), "invalid assessment checksum")
        require(digest(receipt_path) == expected_digest, "security assessment checksum mismatch")
    receipt = read(receipt_path)
    require(receipt["candidate"] == candidate, "security evidence belongs to a different candidate")
    require(receipt["scanner"] == SCANNER, "unexpected scanner digest")
    now = now or dt.datetime.now(dt.timezone.utc)
    created = dt.datetime.fromisoformat(receipt["created_at"])
    require(dt.timedelta(0) <= now - created <= dt.timedelta(hours=24), "security evidence is stale or future-dated; scan again")
    files = receipt["files"]
    required = {"image-assessment.json", "scanner.json", "compatibility.json", "hardened-smoke.txt", "browser-smoke.txt"}
    required.update(f"image-{arch}.{suffix}" for arch in ARCHES for suffix in ("json", "cdx.json"))
    require(set(files) == required, "incomplete security evidence inventory")
    for name, checksum in files.items():
        require(digest(directory / name) == checksum, f"evidence checksum mismatch: {name}")
    scanner = read(directory / "scanner.json")
    require(scanner["Version"] == "0.76.2", "unexpected scanner version")
    db_updated = dt.datetime.fromisoformat(scanner["VulnerabilityDB"]["UpdatedAt"].replace("Z", "+00:00"))
    require(dt.timedelta(0) <= now - db_updated <= dt.timedelta(hours=48), "vulnerability database is stale or future-dated")
    assessments = read(directory / "image-assessment.json")
    require({a["architecture"] for a in assessments} == set(ARCHES) and len(assessments) == 2, "missing architecture assessment")
    for arch in ARCHES:
        raw = read(directory / f"image-{arch}.json")
        require(raw["Metadata"]["ImageID"] == candidate["images"][arch]["image_id"], "scan image does not match tested image")
        scanned = dt.datetime.fromisoformat(raw["CreatedAt"].replace("Z", "+00:00"))
        require(dt.timedelta(0) <= now - scanned <= dt.timedelta(hours=24), "raw scan is stale or future-dated")
        require(raw.get("Results"), "scan has no results")
        targets = {r["Target"] for r in raw["Results"]}
        require({"usr/bin/minio", "usr/bin/mc"}.issubset(targets), "scan did not inspect both shipped binaries")
        assessment = next(a for a in assessments if a["architecture"] == arch)
        require(assessment["image_id"] == raw["Metadata"]["ImageID"], "assessment image mismatch")
        require(all(f["assessment"] != "unresolved" for f in assessment["findings"]), "unresolved image findings")
        sbom = read(directory / f"image-{arch}.cdx.json")
        require(sbom.get("bomFormat") == "CycloneDX" and sbom.get("components"), "missing image SBOM")
    compatibility = read(directory / "compatibility.json")
    require(compatibility.get("passed") is True, "compatibility suite did not pass")
    require(compatibility.get("targetImage") == candidate["images"]["arm64"]["image_id"], "compatibility test used another image")
    require(compatibility.get("smallObjects", 0) >= 10000 and compatibility.get("largeObjectBytes", 0) > 5 * 1024**3, "compatibility fixture was reduced")
    smoke = (directory / "hardened-smoke.txt").read_text()
    require(all(f"PASS linux/{arch}:" in smoke for arch in ARCHES), "hardened smoke tests did not pass on both architectures")
    require("PASS real UI login," in (directory / "browser-smoke.txt").read_text(), "browser acceptance did not pass")
    return receipt


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("command", choices=("inspect", "record", "verify"))
    parser.add_argument("directory", type=Path)
    parser.add_argument("run_id", type=int)
    parser.add_argument("--evidence", type=Path)
    parser.add_argument("--assessment-sha256")
    args = parser.parse_args()
    source_sha = subprocess.check_output(["git", "rev-parse", "HEAD"], text=True).strip()
    run = json.loads(subprocess.check_output(["gh", "api", f"repos/{REPOSITORY}/actions/runs/{args.run_id}"], text=True))
    candidate = inspect_artifacts(args.directory, run, source_sha)
    if args.command == "record":
        require(candidate["security_release"], "candidate is not a security release")
        require(args.evidence is not None, "evidence directory required")
        names = ["image-assessment.json", "scanner.json", "compatibility.json", "hardened-smoke.txt", "browser-smoke.txt"]
        names += [f"image-{arch}.{suffix}" for arch in ARCHES for suffix in ("json", "cdx.json")]
        receipt = {"candidate": candidate, "scanner": SCANNER,
                   "created_at": dt.datetime.now(dt.timezone.utc).isoformat(),
                   "files": {name: digest(args.evidence / name) for name in names}}
        (args.evidence / "security-assessment.json").write_text(json.dumps(receipt, indent=2) + "\n")
        check_evidence(args.evidence, candidate)
        print(digest(args.evidence / "security-assessment.json"))
    elif args.command == "verify" and candidate["security_release"]:
        require(args.evidence is not None and args.assessment_sha256, "security release requires local evidence and its checksum")
        check_evidence(args.evidence, candidate, args.assessment_sha256)
        # Re-evaluate every raw finding using the policy from the tested source.
        subprocess.run(["python3", "scripts/assess-image-security.py", "--check-only", "--directory", str(args.evidence), *ARCHES], check=True)
    if args.command != "record":
        print(json.dumps(candidate, indent=2))
    if output := os.environ.get("GITHUB_ENV"):
        with open(output, "a") as stream:
            stream.write(f"MINIGO_VERSION={candidate['version']}\n")
            stream.write(f"SECURITY_RELEASE={str(candidate['security_release']).lower()}\n")


if __name__ == "__main__":
    try:
        main()
    except (KeyError, ValueError, OSError, subprocess.CalledProcessError) as error:
        raise SystemExit(str(error))
