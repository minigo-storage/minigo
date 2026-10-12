#!/bin/sh
set -eu
root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"
revision=$(git rev-parse HEAD)
for arch in ${MINIGO_ARCHES:-amd64 arm64}; do
    case "$arch" in amd64|arm64) ;; *) echo "Unsupported architecture: $arch" >&2; exit 1 ;; esac
    (cd "dist/minigo/linux-$arch" && shasum -a 256 -c minio.sha256sum mc.sha256sum)
    docker buildx build --platform "linux/$arch" --load \
        --build-arg "MINIGO_REVISION=$revision" -t "minigo:local-$arch" -f Dockerfile .
done
