#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"
revision=$(git -C reference/minio rev-parse HEAD)

for arch in amd64 arm64; do
    test -f "dist/reference/linux-$arch/minio" || {
        echo 'Run make reference-binaries first.' >&2
        exit 1
    }
    docker buildx build --platform "linux/$arch" --load \
        --build-arg "REFERENCE_REVISION=$revision" \
        --tag "minigo-reference:local-$arch" \
        --file Dockerfile.reference .
done
