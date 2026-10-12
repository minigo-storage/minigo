# Minigo

Minigo is an open-source Go fork of MinIO with an expanded React/shadcn management
console. It retains the MinIO storage engine, disk formats, S3/admin protocols,
`mc` integration, server flags and `MINIO_*` configuration. Security fixes and
their compatibility effects are listed in [security notes](docs/minigo-security.md).

The source repository is [minigo-storage/minigo](https://github.com/minigo-storage/minigo).
An optional, separate `reference/minio/` checkout retains unchanged upstream
history, tests and fixtures; see [reference setup](reference/UPSTREAM.md).
Images are published to [Docker Hub](https://hub.docker.com/r/minigostorage/minigo)
for Linux amd64 and arm64. Docker selects the matching platform automatically:

```sh
docker pull minigostorage/minigo:latest
```

Use the recorded image digest to pin a deployment. See
[image publication](docs/publishing.md) for tags, checks and the release workflow.

## Build and run

Requires Go (the scripts select Go 1.27.2), Node.js 24, npm, Git, Make and Docker
with Buildx/Compose. First builds need network access for pinned Go/npm modules,
the digest-pinned Alpine base, and current OS security packages. No MinIO image
registry or discontinued binary download is needed to build Minigo.

```sh
make images       # console, static Go binaries, both local container images
make smoke        # execute both architectures with real mc/S3 operations
make preview      # persistent local review deployment; prints verified LAN URLs
```

| Platform | Server binary | Local image |
| --- | --- | --- |
| Linux amd64 (v1 CPU baseline) | `dist/minigo/linux-amd64/minio` | `minigo:local-amd64` |
| Linux arm64 (v8.0) | `dist/minigo/linux-arm64/minio` | `minigo:local-arm64` |

Each directory also contains a source-built, security-updated `mc` and SHA-256
checksums. `CGO_ENABLED=0` keeps the binaries statically linked. Override
`MINIGO_ARCHES=arm64` for a single-platform build. Docker Desktop emulation is
needed to execute the other architecture locally. These are separate locally
loaded tags, not a published multiarchitecture manifest.

The images retain `/usr/bin/minio`, `/usr/bin/mc`, the upstream shell entrypoint,
`/data`, credential-file environment defaults, GNU coreutils and `curl`.
`/usr/bin/minigo` is also available as a symlink. The runtime uses Alpine instead
of the original UBI userland to reduce dependencies and known OS advisories;
custom UBI/glibc-specific side tools are outside the image compatibility contract.

For a Compose deployment, replace only its image with the matching Minigo tag
and retain the command, environment and volume configuration. The automated
acceptance test below exercises precisely that operation on disposable data.
Do not run two servers concurrently against the same storage volume.

## Console

The console is embedded in the server binary and needs no Node.js service at
runtime. It includes desktop/mobile layouts, dark mode and local static assets.

- Buckets and object browsing, filtering, folders, file/folder uploads, downloads
  and ZIP downloads, metadata/tags, text/image previews and sharing.
- Object versions, restore/rewind, deletion confirmations and bucket versioning.
- Users, groups, policies and associations, service accounts/access keys.
- Lifecycle and replication rule editing, deployment status, and allowlisted
  nonsecret server configuration.

All operations use the signed credentials of the existing authenticated session;
MinIO's server-side IAM remains authoritative. Replication destination credentials
and secret configuration remain managed with `mc`. Statistics come from the
server's background scanner and may lag recent writes. Raw lifecycle/replication
JSON editing is provided for the full underlying rule schema.

`make preview` creates its own persistent `minigo-preview_preview-data` volume,
binds the console on port 19001 and S3 on 19000, and prints a LAN address suitable
for a phone on the same network. Random credentials are stored in the ignored
`dist/preview/credentials.env` file (mode 0600). Set `MINIGO_LAN_IP` explicitly if
the default-route interface is not the desired network. Stop it with:

```sh
docker compose --env-file dist/preview/runtime.env -f compose.preview.yaml stop
```

## Existing-volume acceptance

```sh
make compatibility
```

The suite uses the cached official arm64 image
`minio/minio:RELEASE.2025-09-07T16-13-09Z`. It creates a new volume, writes 10,000
small objects and a streamed 5 GiB + 1 MiB multipart object, then changes only the
Compose image. It checks every object/version hash, metadata, tags, unusual keys,
range reads, IAM, legal holds and real `mc` commands. It completes an upload
started by MinIO, writes new data, injects SIGKILL during another upload, verifies
recovery, and switches back to the original image to verify rollback.

[Test details, evidence and limits](tests/compatibility/README.md) describe exact
assertions and cleanup. No existing user volume is attached. The cached official
image is archived under `dist/reference/archive/`, with metadata and checksums.
No official amd64 image was cached: official-image swap evidence is arm64; both
Minigo architectures have separate executable smoke tests.

[Completed local validation](docs/validation.md) records the final image IDs,
passing same-volume result, browser checks, security assessments and scope.

[Performance work](docs/performance.md) records measured allocation and hashing
improvements, idle buffer-cache behavior, and the listing experiment deferred
because it slowed larger scans.

This is a compatibility baseline with a substantive single-volume acceptance
suite, not proof of every deployment or failure mode. Distributed upgrades,
disk-loss/healing, KMS, remote replication and production workload qualification
need their own environment-specific tests before production replacement.

## Development and verification

```sh
MINIO_API_REQUESTS_MAX=10000 CGO_ENABLED=0 go test -p 4 -tags kqueue,dev -timeout 25m ./cmd ./internal/...
go test github.com/minio/console/api
npm --prefix console/web-app test
make security
make image-security
```

Browser integration tests are in `console/web-app/tests-minigo/integration.mjs`;
run them only against a disposable deployment using its root credentials in
environment variables. They create and remove their own test fixtures.

The Go import paths remain `github.com/minio/minio` and `github.com/minio/console`
to keep upstream internals/tests intact. A local `replace` uses `./console`.
This does not configure or contact a Git remote for this repository. Existing
backend libraries are retained and patched; the console has eight direct runtime
dependencies and adds no production Go dependencies for its new admin routes.

Source provenance: upstream commit
`7aac2a2c5b7c882e68c1ce017d8256be2feea27f`; details in
[reference/UPSTREAM.md](reference/UPSTREAM.md). The upstream build/smoke targets
remain available as `make reference-images` and `make reference-smoke`.

Minigo and the console retain AGPL-3.0-or-later licensing and upstream notices.
shadcn component attribution is included in the frontend. This is a derivative
baseline, not a clean-room rewrite or an official MinIO release.
