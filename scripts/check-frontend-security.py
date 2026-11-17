#!/usr/bin/env python3
"""Audit the full lockfile and recognize only the locally guarded dev advisory."""
import json
import subprocess
from pathlib import Path

frontend = Path(__file__).resolve().parents[1] / "console/web-app"
subprocess.run(["npm", "audit", "--omit=dev", "--audit-level=low"], cwd=frontend, check=True)
subprocess.run(["node", "scripts/guard-picomatch.mjs", "--check"], cwd=frontend, check=True)
audit = subprocess.run(["npm", "audit", "--json"], cwd=frontend, capture_output=True, text=True)
report = json.loads(audit.stdout)
if report.get("auditReportVersion") != 2 or "error" in report or "vulnerabilities" not in report:
    raise SystemExit("npm audit did not return a vulnerability report")
findings = report["vulnerabilities"]
if not findings:
    raise SystemExit(audit.returncode)

advisory = "https://github.com/advisories/GHSA-fc5q-39xc-vjpf"
expected = {
    "picomatch": {advisory},
    "fdir": {"picomatch"},
    "tinyglobby": {"fdir", "picomatch"},
    "vite": {"fdir", "picomatch", "tinyglobby"},
}
print(audit.stdout)
if audit.returncode != 1 or findings.keys() != expected.keys():
    raise SystemExit("Unexpected npm audit findings")
for package, finding in findings.items():
    causes = {v["url"] if isinstance(v, dict) else v for v in finding["via"]}
    if causes != expected[package] or finding["nodes"] != ["node_modules/" + package]:
        raise SystemExit(f"Unassessed npm finding in {package}")
print("Verified local picomatch guard for the build-only advisory GHSA-fc5q-39xc-vjpf.")
