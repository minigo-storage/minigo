#!/bin/sh
set -eu
root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"
revision=$(git rev-parse HEAD)
for arch in ${MINIGO_ARCHES:-amd64 arm64}; do
    case "$arch" in amd64|arm64) ;; *) echo "Unsupported architecture: $arch" >&2; exit 1 ;; esac
    (cd "dist/minigo/linux-$arch" && shasum -a 256 -c minio.sha256sum mc.sha256sum)
    # Security candidates must refresh the package layer even with a warm cache.
    set --
    if [ "${MINIGO_SECURITY_RELEASE:-false}" = true ]; then set -- --pull --no-cache; fi
    docker buildx build "$@" --platform "linux/$arch" --load \
        --build-arg "MINIGO_REVISION=$revision" \
        --build-arg "MINIGO_VERSION=${MINIGO_VERSION:-dev}" \
        -t "minigo:local-$arch" -f Dockerfile .
done
