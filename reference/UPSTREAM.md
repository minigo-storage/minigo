# MinIO reference

- Source: https://github.com/minio/minio
- Default branch: `master`
- Commit: `7aac2a2c5b7c882e68c1ce017d8256be2feea27f`
- Commit date: 2026-02-12T12:18:48-08:00
- Retrieved: 2026-10-13 (Australia/Sydney)
- Full Git history: 12,565 commits at this revision.
- Test source: 247 tracked `*_test.go` files, plus integration scripts and fixtures.
- License and notices: preserved unchanged in `minio/LICENSE`, `minio/NOTICE`, and source files.

`minio/` is an unchanged, full local Git clone with its `origin` remote removed.
The source repository is archived. This revision was verified against upstream
HEAD when retrieved. The nested checkout is ignored by the parent Minigo
repository, so it is neither a submodule nor a duplicate source import.

If this workspace must be recreated later, clone the URL above to
`reference/minio`, check out the recorded commit, and remove `origin`.
The upstream URL is provenance only; the reference checkout has no remote.
The Minigo fork is published at https://github.com/minigo-storage/minigo.

## Runtime reference

The cached official image `minio/minio:RELEASE.2025-09-07T16-13-09Z` is Linux
arm64, image ID `sha256:8f08aee614800a237906bd48114d733e5ac5bfac4ccdf731f141b0e880d7a253`.
It is saved with its inspected metadata under `dist/reference/archive/`.
Official Docker Hub and Quay pulls were unavailable when this workspace was
prepared; `dl.min.io` client downloads returned HTTP 410. Rebuilt images use
the original entrypoint and environment defaults with a public UBI 9 minimal
runtime, pinned by digest in `Dockerfile.reference`.

The bundled client is built from `github.com/minio/mc` at
`v0.0.0-20250813083541-7394ce0dd2a8` (release
`RELEASE.2025-08-13T08-35-41Z`, commit
`7394ce0dd2a80935aded936b09fa12cbb3cb8096`). Its module is downloaded through Go's
module proxy/checksum database. Client license, notice and credits are copied
into each image under `/licenses/mc/`.
