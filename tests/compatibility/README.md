# Same-volume replacement acceptance

`scripts/test-compatibility.sh` runs a real Docker Compose replacement test against
the locally cached official MinIO image. It creates a uniquely named test volume,
loads it with the official image, changes **only the Compose image field**, and
verifies the exact same Docker volume with Minigo. It then writes new data, kills
Minigo with SIGKILL during an in-flight multipart request, restarts, and finally changes the image back to MinIO and
checks that both the original and new data remain readable.

The test never attaches an existing user volume. The driver refuses to seed a
server that already contains buckets. Credentials are random for each run and
kept in a mode-0600 file under the ignored `dist/compatibility/` directory. Ports
bind only to loopback because this is an automated test, not a review server.
Images have `pull_policy: never`: the baseline does not need a working registry.

## Run

Docker with Compose, Go, the MinIO `mc` command, Python 3, OpenSSL, and curl are
required. Build/load Minigo's local image first, then run from the repository root:

```sh
scripts/test-compatibility.sh
```

The defaults are `minio/minio:RELEASE.2025-09-07T16-13-09Z`,
`minigo:local-arm64`, and `linux/arm64`. Override `BASELINE_IMAGE`, `TARGET_IMAGE`,
and `COMPAT_PLATFORM` for another cached image pair. Running on arm64 tests the
native arm64 implementation; a separate amd64 run is needed to claim execution
coverage on amd64.

To seed the official image while a Minigo image is building:

```sh
scripts/test-compatibility.sh prepare
TARGET_IMAGE=minigo:local-arm64 scripts/test-compatibility.sh resume dist/compatibility/<printed-run-directory>
```

`prepare-resume <run-directory>` repeats baseline verification without reseeding
when diagnosing a test assertion against an already completed seed.

`COMPAT_DRIVER=/absolute/path/to/already-built-driver` optionally supplies a
prebuilt native test driver (useful when the root module is being updated).

## Fixture and assertions

- **10,000 small objects under one prefix**, including empty objects. V1, V2 and
  version listings use small page sizes to force pagination.
- A streamed **5 GiB + 1 MiB** object: 5,369,757,696 bytes, larger than the S3
  single-request upload ceiling. Multipart upload uses 64 MiB parts and two
  workers, with bounded memory and no local giant fixture file.
- Unicode and URL-escaped keys, spaces, percent signs, `+`, `?`, `#`, `&`, `=`,
  overlapping file/prefix names, deep paths, and a 1,024-byte key made of path
  components below the filesystem component limit.
- Full **SHA256 of every object and every nondeleted version** at each verification
  phase; size, ETag, content type, custom metadata and tags; exact inventories
  including version IDs, modification timestamps, latest flags and delete markers.
- Range reads across the 64 MiB part boundary, 4 GiB boundary and final bytes.
- Version history and a current delete marker; legal hold persists and rejects
  deletion even when governance bypass is requested.
- An incomplete multipart upload created by MinIO is resumed and completed by
  Minigo, preserving the already uploaded part.
- Stored IAM user/policy survives the swap: allowed reads work and forbidden
  writes return `AccessDenied`; admin API calls succeed.
- Real `mc admin info`, user and policy info, version info, object stat and bucket
  listing execute against the baseline, Minigo, the recovered server and rollback.
- New objects and a new version of an existing object are written through Minigo.
  Another multipart upload stores its first part, begins streaming a slow second
  part, and signals the runner to inject SIGKILL. All acknowledged data is checked
  after restart and after rollback to MinIO; the uncommitted object must never
  become visible. Existing versions, delete markers, and listing entries must
  also survive the new writes without unrelated changes.
- Docker mount inspection proves the same named volume is used at each phase;
  normalized Compose configurations prove only the image field changed.

## Resources, evidence, and cleanup

Allow at least **8 GiB of free Docker disk**, plus the two images. The default test
stores just over 5 GiB and reads that data five times across verification phases.
The server and driver's overhead generally needs at least 2 GiB of available RAM;
actual duration depends on Docker's disk and network performance. On failure,
the volume and server are retained for diagnosis. On success, the test removes
only its own Compose stack and volume, leaving JSON evidence and logs in
`dist/compatibility/<unique-run>/`. Set `KEEP_VOLUME=1` to retain the stopped stack
after success. A generated `cleanup.sh` removes only that run's resources.

Each phase has a JSON result. `report.json` is emitted **only after all phases
pass**, and records image IDs, volume name, object sizes/counts, verification
duration and limitations. Do not publish `private/`, which contains test secrets.

For iteration only, `SMALL_OBJECTS=2000 LARGE_BYTES=67108865` reduces load. Such a
run does **not** meet the >5 GiB acceptance threshold. The report records actual
values so a reduced run cannot be mistaken for the default suite.

## Limits

This is a single-node, single-volume acceptance suite. It does not prove every
S3/admin extension, distributed erasure-set failures, disk-loss recovery, KMS,
replication, concurrent cluster upgrades, or all historical storage layouts.
SIGKILL is injected during one multipart request after prior acknowledged writes;
the test verifies recovery of those writes and absence of an unfinished object,
not every timing window of concurrent writes or host power failure.
Those require additional suites before claiming production replacement safety.

The cached official image has an existing listing quirk: if `prefix/file` and
`prefix/file/child` both exist, ordinary V1/V2 listings omit the child. Direct reads
and version listings still access it. The suite records that exact baseline
listing behavior and requires Minigo to preserve it, while hashing the child
independently. This is a compatibility finding, not evidence that the object was
lost. Changing this behavior should be a separate, explicitly reviewed fix.
