#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root/reference/minio"

# Match the toolchain pinned by the reference checkout. Override explicitly
# when evaluating another compiler version.
export GOTOOLCHAIN=${GOTOOLCHAIN:-go1.24.8}
export CGO_ENABLED=0
unset GOOS GOARCH
ldflags=$(go run buildscripts/gen-ldflags.go)

for arch in amd64 arm64; do
    output="$root/dist/reference/linux-$arch"
    mkdir -p "$output"
    printf 'Building MinIO reference: linux/%s\n' "$arch"
    GOOS=linux GOARCH="$arch" GOAMD64=v1 GOARM64=v8.0 \
        go build -mod=readonly -tags kqueue -trimpath -ldflags "$ldflags" \
        -o "$output/minio" .
    (cd "$output" && shasum -a 256 minio > minio.sha256sum)
done

# The original image also bundles mc. Build its pinned source instead of
# depending on retired MinIO image registries or binary download endpoints.
mc_version=v0.0.0-20250813083541-7394ce0dd2a8
mc_commit=7394ce0dd2a80935aded936b09fa12cbb3cb8096
go mod download "github.com/minio/mc@$mc_version"
mc_source=$(go list -m -f '{{.Dir}}' "github.com/minio/mc@$mc_version")
mc_ldflags="-s -w -X github.com/minio/mc/cmd.Version=2025-08-13T08:35:41Z"
mc_ldflags="$mc_ldflags -X github.com/minio/mc/cmd.ReleaseTag=DEVELOPMENT.2025-08-13T08-35-41Z"
mc_ldflags="$mc_ldflags -X github.com/minio/mc/cmd.CommitID=$mc_commit"
mc_ldflags="$mc_ldflags -X github.com/minio/mc/cmd.ShortCommitID=7394ce0dd2a8"
mc_ldflags="$mc_ldflags -X github.com/minio/mc/cmd.CopyrightYear=2025"
mkdir -p "$root/dist/reference/licenses/mc"
cp "$mc_source/LICENSE" "$mc_source/NOTICE" "$mc_source/CREDITS" \
    "$root/dist/reference/licenses/mc/"

for arch in amd64 arm64; do
    output="$root/dist/reference/linux-$arch"
    printf 'Building mc reference: linux/%s\n' "$arch"
    (cd "$mc_source" && GOOS=linux GOARCH="$arch" GOAMD64=v1 GOARM64=v8.0 \
        go build -mod=readonly -trimpath -ldflags "$mc_ldflags" -o "$output/mc" .)
    (cd "$output" && shasum -a 256 mc > mc.sha256sum)
done
