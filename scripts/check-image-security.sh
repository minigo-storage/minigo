#!/bin/sh
# Local-only image qualification. Never expose the Docker socket to the scanner.
set -eu
if [ "${GITHUB_ACTIONS:-false}" = true ]; then
    echo 'Image scans run locally only, never on GitHub Actions runners.' >&2
    exit 1
fi
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"
export GOTOOLCHAIN=${GOTOOLCHAIN:-go1.27.3}
export CGO_ENABLED=0
mkdir -p "${MINIGO_SCAN_DIR:-dist/security}"
report_dir=$(cd "${MINIGO_SCAN_DIR:-dist/security}" && pwd)
scanner=$(python3 -c 'import runpy; print(runpy.run_path("scripts/release-artifacts.py")["SCANNER"])')
for arch in ${MINIGO_ARCHES:-amd64 arm64}; do
    case "$arch" in amd64|arm64) ;; *) echo 'Unsupported architecture' >&2; exit 1 ;; esac
    dependencies=$(GOOS=linux GOARCH="$arch" go list -mod=readonly -tags=kqueue -deps . github.com/minio/mc)
    case "$dependencies" in *golang.org/x/crypto/openpgp*) echo 'OpenPGP is imported; the existing assessment is invalid.' >&2; exit 1 ;; esac
    if [ -n "${MINIGO_ARCHIVE_ROOT:-}" ]; then
        archive="$(cd "$MINIGO_ARCHIVE_ROOT/minigo-image-$arch" && pwd)/image.tar.gz"
    else
        mkdir -p "$report_dir/archives"
        archive="$report_dir/archives/image-$arch.tar"
        docker save "${MINIGO_IMAGE_PREFIX:-minigo}:local-$arch" --output "$archive"
    fi
    docker run --rm -v "$archive:/image.tar:ro" -v minigo-trivy-cache:/root/.cache \
        "$scanner" image --input /image.tar --scanners vuln --format json --quiet \
        > "$report_dir/image-$arch.json"
    docker run --rm -v "$report_dir:/reports:ro" "$scanner" convert --format cyclonedx \
        "/reports/image-$arch.json" > "$report_dir/image-$arch.cdx.json"
done
docker run --rm -v minigo-trivy-cache:/root/.cache "$scanner" version --format json > "$report_dir/scanner.json"
python3 scripts/assess-image-security.py --directory "$report_dir" ${MINIGO_ARCHES:-amd64 arm64}
