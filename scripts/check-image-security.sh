#!/bin/sh
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"
mkdir -p dist/security
for arch in ${MINIGO_ARCHES:-amd64 arm64}; do
    dependencies=$(GOOS=linux GOARCH="$arch" go list -mod=readonly -deps . github.com/minio/mc)
    case "$dependencies" in *golang.org/x/crypto/openpgp*) echo 'OpenPGP is now imported: the existing assessment is invalid.' >&2; exit 1 ;; esac
    image_id=$(docker image inspect "minigo:local-$arch" --format '{{.Id}}')
    docker run --rm -v /var/run/docker.sock:/var/run/docker.sock \
        -v minigo-trivy-cache:/root/.cache aquasec/trivy:0.75.0 image \
        --scanners vuln --format json "$image_id" > "dist/security/image-$arch.json"
done
python3 scripts/assess-image-security.py ${MINIGO_ARCHES:-amd64 arm64}
