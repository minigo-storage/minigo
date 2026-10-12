# Minigo

A new object store written in Go, with the goal of being a 1:1 replacement for
MinIO on Linux amd64 and arm64. This repository currently contains the project
scaffold and an unchanged local MinIO reference, including its full history,
tests and fixtures. The independent Minigo server is not implemented yet.

## Layout

- `go.mod`, `doc.go`: independent Minigo module, using a local module name until a public repository is chosen.
- `reference/minio/`: upstream source and tests in a separate Git checkout, with no remote.
- `reference/UPSTREAM.md`: exact upstream revision and provenance.
- `scripts/`, `Dockerfile.reference`: local reference builds and smoke tests.
- `dist/reference/`: generated Linux binaries and SHA-256 checksums (ignored by Git).

Both repositories are local only. The root repository uses `main`; the reference
retains upstream `master`. No public repository, registry push or CI publishing
has been configured.

## Local reference builds

Requires Go, Git, `make`, `shasum`, and Docker with Buildx. The binary build uses
the upstream-pinned Go 1.24.8 toolchain, which Go downloads automatically if
needed. Initial builds also download pinned module dependencies and the runtime
base image. `GOTOOLCHAIN` can be overridden explicitly.

```sh
make reference-binaries
make reference-images
make reference-smoke
```

`reference-images` also builds the binaries. Outputs are:

| Platform | Binary | Local image |
| --- | --- | --- |
| Linux amd64 | `dist/reference/linux-amd64/minio` | `minigo-reference:local-amd64` |
| Linux arm64 | `dist/reference/linux-arm64/minio` | `minigo-reference:local-arm64` |

Both binaries are statically linked with CGO disabled. amd64 targets the v1 CPU
baseline; arm64 targets v8.0. Images are loaded into Docker locally, without
pushing. Cross-architecture smoke tests require Docker emulation (included in
Docker Desktop). They publish no ports and remove their temporary containers
and volumes when finished.

The reference images compile the latest checked-out source and reconstruct the
upstream runtime contract on a digest-pinned UBI 9 minimal base, including
`/usr/bin/minio`, source-built `mc`, `curl`, the original shell entrypoint,
`/data`, port 9000 and credential-file environment defaults. No build depends on
an upstream MinIO container or binary download. Locally built binaries have
SHA-256 checksums, not upstream signatures. The OS package set differs from the
old UBI micro image; complete image equivalence has not been established.
These are upstream reference images, not an independent Minigo implementation.

The cached official arm64 image
`minio/minio:RELEASE.2025-09-07T16-13-09Z` is also archived locally under
`dist/reference/archive/` with its image metadata, for future compatibility and
existing-data tests. Restore it with `docker load -i <archive.tar>` if needed.
No cached official amd64 MinIO image was available.

## Compatibility target

The future Minigo server and images must preserve the MinIO command/flag and
`MINIO_*` environment contracts, S3 wire behavior and errors, authentication and
policies, admin interfaces, console behavior, health/metrics endpoints, data
volume semantics and supported on-disk formats. Existing deployments must be
able to switch images on both architectures without changing their configuration.

Use the upstream tests and fixtures to establish that contract. Its Go unit
tests depend on upstream internals, so a new implementation also needs shared
black-box compatibility tests run against both servers. Full compatibility,
including existing-data migration and distributed operation, remains to be
implemented and verified; the included smoke test covers only the reference
image startup and basic S3 operations.

Upstream licensing, attribution and notices remain intact under
`reference/minio/` and in the reference images.
