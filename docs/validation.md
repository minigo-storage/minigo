# Local validation — 13 October 2026

This records the initial compatibility baseline. The local image tags now point
to the optimized builds documented in [performance results](performance.md),
which include a fresh same-volume verification and current image IDs.

The local images below were built from source commit
`8089a9bf3d7e2da7a2e65b62bf107914a1182dc8`, using Go 1.27.2 and the embedded
production console. Subsequent documentation-only commits do not change them.
The equivalent source commit in the published history is
`edc71ec02132fdd15e85f1f1c86a1b579174d7f1`; the identity rewrite changed commit
metadata only. At the time of this validation, neither source nor images had
been published and both Git repositories had no remotes.

## Delivered artifacts

| Platform | Local tag | Docker image ID |
| --- | --- | --- |
| Linux amd64 | `minigo:local-amd64` | `sha256:581cdf27c42e993975c87b60147da2e4af0e7a3050706d1dac6fd184fa9e83c6` |
| Linux arm64 | `minigo:local-arm64` | `sha256:354d792f7044c081f031bb0d2b4cd2f4efab010acb2134deb48e7b485465214b` |

Static server and client binaries, with individual SHA-256 files, are under
`dist/minigo/linux-amd64/` and `dist/minigo/linux-arm64/`. The unchanged reference
checkout is `reference/minio/`; its provenance is in [UPSTREAM.md](../reference/UPSTREAM.md).
The cached official image is archived under `dist/reference/archive/`.

## Existing-volume image swap: passed

The cached official arm64 MinIO release `RELEASE.2025-09-07T16-13-09Z` created a
fresh volume containing 10,000 small objects, a 5 GiB + 1 MiB multipart object
(5,369,757,696 bytes), and additional edge-case fixtures. The test changed only
the Compose image and confirmed the exact volume identity remained unchanged.

All eight phases passed: original creation and verification, Minigo image-swap
verification, new writes, verification of new writes, interrupted multipart
upload, crash recovery, and rollback verification using the original image.
All 10,013 original versions passed SHA-256 checks after the swap. All 10,047
stored versions after new writes passed again after recovery and rollback.

The suite also checked metadata/tags, Unicode and escaped keys, long/deep keys,
prefix/file collisions, version and delete-marker listings, range reads across
4 GiB, legal holds, IAM allowed/denied requests, and real `mc` commands. An upload
initiated by MinIO was completed by Minigo. The unfinished object from the killed
multipart request remained absent. An existing MinIO listing quirk is preserved
and explicitly recorded rather than mistaken for missing stored data.

- [Exact-image report](../dist/compatibility/minigo-compat-20261013081151-fb3fa4/report.json)
- [Suite, assertions, and cleanup instructions](../tests/compatibility/README.md)
- Retained fixture volume: `minigo-compat-20261013081151-fb3fa4_fixture`.

## Other completed checks

| Check | Result | Local evidence |
| --- | --- | --- |
| Both image architectures: startup/readiness and real `mc` create/read/stat/list/delete | Passed; amd64 executed through Docker Desktop emulation | [Smoke log](../dist/verification/smoke-final.log) |
| Backend `./cmd ./internal/...` tests | Passed | [Backend log](../dist/verification/backend-final.log) |
| Console API suite after final URL-path changes | Passed | [Console log](../dist/verification/console-final.log) |
| Accelerated JSON reader on amd64 | Passed | [SIMD log](../dist/verification/simdj-amd64.log) |
| Production frontend build and seven client tests | Passed | [Build log](../dist/verification/build-final.log), `npm --prefix console/web-app test` |
| Real embedded console, desktop and 390px mobile | Passed login, bucket creation, Unicode upload/download byte equality, metadata/text preview, versioning, admin screens, navigation and overflow checks | [Browser log](../dist/verification/ui/integration.log) |
| Source reachability scan, both architectures; frontend audit | Zero reachable or imported-package Go advisories; zero frontend advisories | [Security log](../dist/verification/security-final.log) |
| Full image scans, both architectures | Zero OS advisories; three raw binary findings per image, each assessed; zero unresolved findings | [Assessments](../dist/security/image-assessment.json) |

Image findings remain visible: the OpenPGP package is not imported by either
program, and the version-only MinIO CSV finding is fixed locally with bounded
reads and regression tests. [Security notes](minigo-security.md) document these
assessments, source fixes, and intentional compatibility changes.

Desktop/mobile and light/dark screenshots are retained in `dist/verification/ui/`.
The console remains available at <http://10.1.1.222:19001/> and S3 at
<http://10.1.1.222:19000/> on this LAN. Both addresses returned HTTP 200 through
the LAN interface. Preview credentials remain only in ignored
`dist/preview/credentials.env`; the preview uses its own volume and sample data.
Temporary frontend servers and browser-test buckets were removed.

## Qualification limits

Official-image volume-swap evidence is **single-node, single-volume, arm64**.
No official amd64 image was cached, so amd64 has separate executable smoke
coverage. These tests do not establish production safety for distributed
upgrades, disk/host failure, healing, KMS, remote replication, external SSO,
every IAM/S3 combination, or sustained load. The interrupted-write test covers
one multipart timing window, not every crash or acknowledged-write window.

There is no promise of zero vulnerabilities or data loss. Keep these results
with the exact image IDs, and qualify the remaining deployment-specific cases
before replacing production storage.
