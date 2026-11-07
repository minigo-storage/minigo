#!/usr/bin/env python3
"""Regression checks for tampered, stale and mixed release artifacts."""
import datetime as dt
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

spec = importlib.util.spec_from_file_location("release", Path(__file__).with_name("release-artifacts.py"))
release = importlib.util.module_from_spec(spec)
spec.loader.exec_module(release)


class ReleaseEvidenceTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.now = dt.datetime.now(dt.timezone.utc)
        self.sha = "a" * 40
        self.run = {"id": 123, "run_attempt": 1, "status": "completed", "conclusion": "success",
                    "head_branch": "main", "head_sha": self.sha,
                    "repository": {"full_name": release.REPOSITORY},
                    "path": ".github/workflows/publish-images.yml", "event": "workflow_dispatch"}
        for arch in release.ARCHES:
            folder = self.root / f"minigo-image-{arch}"
            folder.mkdir()
            candidate = {"source_sha": self.sha, "run_id": "123", "run_attempt": "1",
                         "architecture": arch, "version": "3.0.1", "increment": "patch", "security_release": True}
            self.write(folder / "candidate.json", candidate)
            image = {"Id": f"sha256:{arch}", "Os": "linux", "Architecture": arch,
                     "Config": {"Labels": {"org.opencontainers.image.revision": self.sha,
                                           "org.opencontainers.image.version": "3.0.1"}}}
            self.write(folder / "image.json", [image])
            (folder / "image.tar.gz").write_bytes(arch.encode())
            (folder / "image.tar.gz.sha256").write_text(release.digest(folder / "image.tar.gz"))
        self.candidate = release.inspect_artifacts(self.root, self.run, self.sha)
        self.evidence = self.root / "evidence"
        self.evidence.mkdir()
        for arch in release.ARCHES:
            self.write(self.evidence / f"image-{arch}.json", {
                "Metadata": {"ImageID": f"sha256:{arch}"}, "CreatedAt": self.now.isoformat(),
                "Results": [{"Target": "usr/bin/minio"}, {"Target": "usr/bin/mc"}]})
            self.write(self.evidence / f"image-{arch}.cdx.json", {"bomFormat": "CycloneDX", "components": [{"name": "minio"}]})
        self.write(self.evidence / "image-assessment.json", [
            {"architecture": a, "image_id": f"sha256:{a}", "findings": []} for a in release.ARCHES])
        self.write(self.evidence / "scanner.json", {"Version": "0.76.2", "VulnerabilityDB": {"UpdatedAt": self.now.isoformat()}})
        self.write(self.evidence / "compatibility.json", {"passed": True, "targetImage": "sha256:arm64 arm64", "platform": "linux/arm64", "smallObjects": 10000, "largeObjectBytes": 5369757696})
        (self.evidence / "hardened-smoke.txt").write_text("PASS linux/amd64: ok\nPASS linux/arm64: ok\n")
        (self.evidence / "browser-smoke.txt").write_text("PASS real UI login, bucket operations and mobile checks\n")
        self.receipt = {"candidate": self.candidate, "scanner": release.SCANNER, "created_at": self.now.isoformat()}
        self.refresh_receipt()

    def write(self, path, value):
        path.write_text(json.dumps(value))

    def refresh_receipt(self):
        self.receipt["files"] = {p.name: release.digest(p) for p in self.evidence.iterdir() if p.name != "security-assessment.json"}
        self.write(self.evidence / "security-assessment.json", self.receipt)

    def verify(self):
        return release.check_evidence(self.evidence, self.candidate, release.digest(self.evidence / "security-assessment.json"), self.now)

    def test_valid_candidate(self):
        self.verify()

    def test_reject_failed_wrong_branch_workflow_or_attempt(self):
        for key, value in [("conclusion", "failure"), ("head_branch", "untrusted"), ("head_sha", "b" * 40),
                           ("path", ".github/workflows/other.yml"), ("run_attempt", 2)]:
            with self.subTest(key=key), self.assertRaises(ValueError):
                release.inspect_artifacts(self.root, {**self.run, key: value}, self.sha)

    def test_reject_changed_archive(self):
        (self.root / "minigo-image-arm64/image.tar.gz").write_bytes(b"another build")
        with self.assertRaisesRegex(ValueError, "checksum"):
            release.inspect_artifacts(self.root, self.run, self.sha)

    def test_reject_mixed_architecture_source(self):
        path = self.root / "minigo-image-arm64/candidate.json"
        data = release.read(path)
        data["source_sha"] = "b" * 40
        self.write(path, data)
        with self.assertRaisesRegex(ValueError, "revision"):
            release.inspect_artifacts(self.root, self.run, self.sha)

    def test_reject_stale_and_future_assessment(self):
        for age in (dt.timedelta(hours=25), -dt.timedelta(minutes=1)):
            self.receipt["created_at"] = (self.now - age).isoformat()
            self.refresh_receipt()
            with self.assertRaisesRegex(ValueError, "stale|future"):
                self.verify()

    def test_reject_evidence_tampering(self):
        (self.evidence / "image-arm64.json").write_text("{}")
        with self.assertRaisesRegex(ValueError, "checksum"):
            self.verify()

    def test_reject_other_image_even_with_new_checksums(self):
        path = self.evidence / "image-arm64.json"
        data = release.read(path)
        data["Metadata"]["ImageID"] = "sha256:other"
        self.write(path, data)
        self.refresh_receipt()
        with self.assertRaisesRegex(ValueError, "tested image"):
            self.verify()

    def test_reject_incomplete_or_unresolved_scans(self):
        path = self.evidence / "image-assessment.json"
        data = release.read(path)
        data[0]["findings"] = [{"assessment": "unresolved"}]
        self.write(path, data)
        self.refresh_receipt()
        with self.assertRaisesRegex(ValueError, "unresolved"):
            self.verify()

    def test_reject_reduced_compatibility_test(self):
        path = self.evidence / "compatibility.json"
        data = release.read(path)
        data["largeObjectBytes"] = 100
        self.write(path, data)
        self.refresh_receipt()
        with self.assertRaisesRegex(ValueError, "reduced"):
            self.verify()

    def test_reject_other_compatibility_image_architecture_or_platform(self):
        path = self.evidence / "compatibility.json"
        original = release.read(path)
        for key, value in (("targetImage", "sha256:other arm64"),
                           ("targetImage", "sha256:arm64 amd64"),
                           ("platform", "linux/amd64")):
            with self.subTest(key=key, value=value):
                self.write(path, {**original, key: value})
                self.refresh_receipt()
                with self.assertRaisesRegex(ValueError, "another image|another platform"):
                    self.verify()

    def test_scanner_refuses_github_actions_before_invoking_docker(self):
        result = subprocess.run(["sh", "scripts/check-image-security.sh"],
                                env={**os.environ, "GITHUB_ACTIONS": "true"}, capture_output=True, text=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("locally only", result.stderr)

    def test_workflows_never_launch_image_scans(self):
        for workflow in Path(".github/workflows").glob("*.yml"):
            text = workflow.read_text().lower()
            for forbidden in ("make image-security", "check-image-security.sh", "qualify-release.sh", "aquasec/trivy", "trivy-action"):
                self.assertNotIn(forbidden, text, str(workflow))


if __name__ == "__main__":
    unittest.main()
