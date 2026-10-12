#!/bin/sh
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"
export GOTOOLCHAIN=${GOTOOLCHAIN:-go1.27.2}
mkdir -p dist/tools dist/security
GOBIN="$root/dist/tools" go install golang.org/x/vuln/cmd/govulncheck@v1.9.0
for arch in amd64 arm64; do
    GOOS=linux GOARCH="$arch" dist/tools/govulncheck \
        ./cmd ./internal/... github.com/minio/console/api github.com/minio/mc \
        > "dist/security/govulncheck-$arch.txt"
    cat "dist/security/govulncheck-$arch.txt"
done
npm --prefix console/web-app audit --audit-level=low
