#!/bin/sh
set -eu
root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"
export GOTOOLCHAIN="${GOTOOLCHAIN:-go1.27.2}"
export CGO_ENABLED=0
unset GOOS GOARCH

npm --prefix console/web-app ci
npm --prefix console/web-app run build

# Keep release timestamps parseable by mc and the existing admin protocol.
ldflags=$(MINIO_RELEASE=DEVELOPMENT go run buildscripts/gen-ldflags.go)
ldflags="$ldflags -X github.com/minio/minio/cmd.MinioBannerName=Minigo"
mc_ldflags="-s -w -X github.com/minio/mc/cmd.Version=2025-08-13T08:35:41Z"
mc_ldflags="$mc_ldflags -X github.com/minio/mc/cmd.ReleaseTag=DEVELOPMENT.2025-08-13T08-35-41Z"
mc_ldflags="$mc_ldflags -X github.com/minio/mc/cmd.CommitID=7394ce0dd2a80935aded936b09fa12cbb3cb8096"
mc_ldflags="$mc_ldflags -X github.com/minio/mc/cmd.ShortCommitID=7394ce0dd2a8"
mc_ldflags="$mc_ldflags -X github.com/minio/mc/cmd.CopyrightYear=2025"

for arch in ${MINIGO_ARCHES:-amd64 arm64}; do
    case "$arch" in amd64|arm64) ;; *) echo "Unsupported architecture: $arch" >&2; exit 1 ;; esac
    output="$root/dist/minigo/linux-$arch"
    mkdir -p "$output"
    printf 'Building Minigo: linux/%s\n' "$arch"
    GOOS=linux GOARCH="$arch" GOAMD64=v1 GOARM64=v8.0 \
        go build -mod=readonly -tags kqueue -trimpath -ldflags "$ldflags" -o "$output/minio" .
    # Keep the pinned mc source while using this module's security-patched
    # dependency graph and current Go toolchain for the shipped client too.
    GOOS=linux GOARCH="$arch" GOAMD64=v1 GOARM64=v8.0 \
        go build -mod=readonly -trimpath -ldflags "$mc_ldflags" -o "$output/mc" github.com/minio/mc
    (cd "$output" && shasum -a 256 minio > minio.sha256sum && shasum -a 256 mc > mc.sha256sum)
done

mc_source=$(go list -m -f '{{.Dir}}' github.com/minio/mc)
mkdir -p dist/minigo/licenses/mc
install -m 644 "$mc_source/LICENSE" "$mc_source/NOTICE" "$mc_source/CREDITS" dist/minigo/licenses/mc/
