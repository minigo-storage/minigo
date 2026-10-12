#!/usr/bin/env python3
"""Keep raw image findings and record two explicit, source-reviewed assessments."""
import json
import pathlib
import sys

unresolved = []
reports = []
for arch in sys.argv[1:]:
    path = pathlib.Path(f"dist/security/image-{arch}.json")
    raw = json.loads(path.read_text())
    findings = []
    for result in raw.get("Results", []):
        for item in result.get("Vulnerabilities", []):
            issue = item["VulnerabilityID"]
            package = item["PkgName"]
            target = result["Target"]
            reason = None
            if (issue, package) == ("GO-2026-5932", "golang.org/x/crypto") and target in ("usr/bin/minio", "usr/bin/mc"):
                reason = "OpenPGP is not imported; architecture dependency check and govulncheck must remain clean."
            elif (issue, package, target) == ("CVE-2026-39414", "github.com/minio/minio", "usr/bin/minio"):
                reason = "Locally fixed by bounded CSV reads; reader_limits_test.go covers plain/gzip and record boundaries."
            finding = {"id": issue, "package": package, "target": target,
                       "severity": item["Severity"], "assessment": reason or "unresolved"}
            findings.append(finding)
            if reason is None:
                unresolved.append({"architecture": arch, **finding})
    report = {"architecture": arch, "image_id": raw.get("Metadata", {}).get("ImageID"),
              "scan_time": raw.get("CreatedAt"), "raw_report": str(path), "findings": findings}
    reports.append(report)
    print(f"{arch}: {len(findings)} raw findings, {sum(f['assessment'] == 'unresolved' for f in findings)} unresolved")
    for finding in findings:
        print(f"  {finding['id']}: {finding['assessment']}")
pathlib.Path("dist/security/image-assessment.json").write_text(json.dumps(reports, indent=2) + "\n")
if unresolved:
    print(json.dumps(unresolved, indent=2), file=sys.stderr)
    sys.exit(1)
