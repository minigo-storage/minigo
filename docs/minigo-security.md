# Minigo security and compatibility notes

This fork retains upstream storage code and disk formats. It does not promise
zero vulnerabilities or guaranteed absence of data loss. Keep raw scan evidence,
regression results and compatibility results together when qualifying an image.

## Intentional compatibility changes

| Area | New behavior | Operational effect |
| --- | --- | --- |
| Unsigned-trailer uploads | Credential-bearing uploads verify header or query signatures before reading data, including Snowball and multipart. Mixed authentication is rejected. | Correct signed/presigned uploads and policy-permitted anonymous uploads remain supported; invalid signatures no longer write objects. |
| Encryption replication metadata | Internal encryption fields require an actual replication request and `s3:ReplicateObject` authorization. Untrusted PAX/form metadata cannot inject those fields. | Normal object writers cannot forge sealed keys. Genuine replication retains its metadata. |
| S3 Select | CSV and JSON Lines physical lines are limited to 1 MiB of payload plus CRLF, including compressed inputs and the accelerated JSON parser. | Oversized lines now fail with `OverMaxRecordSize`. Large objects containing ordinary short records and normal object GET/PUT are unaffected. |
| OIDC | Identity tokens require asymmetric provider signing keys; OAuth client secrets are never registered as token-signing keys. Algorithm restrictions survive JWKS refresh. | Providers relying on HS256/384/512 identity tokens must switch to asymmetric signing. |
| LDAP STS | Bind and missing-policy failures return the same generic `403 AccessDenied`. Attempts are limited per actual TCP peer: burst 30, then 1/second. | Excess attempts return HTTP 429 with `Retry-After: 1`. Users behind one proxy/NAT share its budget. Limits are per process and reset on restart; forwarded headers do not bypass them. |
| Internal storage reads | Every msgpack batch path is checked before reading any file. | Absolute/traversing/invalid paths fail. Empty-bucket multi-bucket reads and valid relative metadata paths remain supported. |
| Console TLS | TLS certificate and hostname verification is enabled. Embedded loopback HTTPS verifies the configured certificate identity and follows certificate reloads. | DNS-only certificates work on loopback. Explicit upstream/IdP endpoints require trusted certificates with matching SANs; expired, mismatched and CN-only certificates are rejected. |
| Console browser origins | WebSockets and new administrative writes enforce expected origins. New writes require JSON and a body no larger than 1 MiB. | Reverse proxies must preserve Host or configure the public `MINIO_BROWSER_REDIRECT_URL`. Cross-site requests are rejected. |
| Container userland | Digest-pinned Alpine replaces UBI; static Go binaries, shell entrypoint, GNU coreutils, curl, paths and environment contract remain. | Custom glibc/UBI-specific executables or package scripts need separate adaptation. |

## Published upstream issues addressed

These advisories were checked directly because scanning dependencies alone does
not cover vulnerabilities in the current main module or unpublished fork fixes.
The reference checkout and archived baseline image intentionally retain original
behavior and are only test oracles.

- [Unsigned-trailer authentication](https://github.com/minio/minio/security/advisories/GHSA-hv4r-mvr4-25vw) and [Snowball authentication](https://github.com/minio/minio/security/advisories/GHSA-9c4q-hq6p-c237): `cmd/streaming-v4-unsigned.go` and handler dispatch; signed-request regressions in `cmd/minigo_unsigned_auth_test.go`.
- [Encryption metadata injection, CVE-2026-34204](https://github.com/minio/minio/security/advisories/GHSA-3rh2-v3gr-35p9): `cmd/handler-utils.go`; metadata and real IAM-restricted PUT regressions in `cmd/minigo_metadata_security_test.go`.
- [CSV memory exhaustion, CVE-2026-39414](https://github.com/minio/minio/security/advisories/GHSA-h749-fxx7-pwpg): bounded split readers in `internal/s3select/csv`, with plain/gzip, header-mode and boundary tests. Equivalent unbounded reads found in JSON Lines and its SIMD dependency are also guarded and tested locally.
- [OIDC algorithm confusion, CVE-2026-33322](https://github.com/minio/minio/security/advisories/GHSA-5cx5-wh4m-82fh): `internal/config/identity/openid/jwt.go`; forged HMAC rejection and valid RSA/JWKS-refresh tests.
- [LDAP enumeration/brute force, CVE-2026-33419](https://github.com/minio/minio/security/advisories/GHSA-jv87-32hw-hh99): `cmd/minigo_ldap.go` and the LDAP STS handler; generic-response, actual-peer, burst/refill, concurrency and bounded-state tests.
- [Internal read path traversal](https://github.com/minio/minio/security/advisories/GHSA-xh8f-g2qw-gcm7): `cmd/xl-storage.go`; tests decode real msgpack requests and assert invalid batches return no file data, while valid multi-bucket paths still work.
- [Session-policy bypass](https://github.com/minio/minio/security/advisories/GHSA-jjjj-jwhf-8rgr): the copied upstream revision already contains upstream fix `c1a49490c`; inherited IAM/STS tests remain included.

## Dependency and image checks

`make security` runs pinned `govulncheck` for Linux amd64 and arm64 and audits the
complete frontend lockfile. The inherited Go dependency graph was updated,
including networking/cryptography, gRPC, telemetry, MQTT, AMQP, Thrift and other
flagged libraries. Both the server and bundled `mc` use this patched graph and
Go 1.27.2. No new production Go library was added for the console API extensions.

`make image-security` retains full image reports and fails on unassessed findings.
Full image reports include OS packages and both Go binaries, at all severities,
including unfixed findings. Two scanner entries need explicit interpretation:

- `GO-2026-5932` describes the unmaintained `golang.org/x/crypto/openpgp` package.
  The parent module is used for other cryptography, but OpenPGP is not imported
  by either shipped program. `govulncheck` reports it only at module level.
- A version-only scanner still identifies this fork as `github.com/minio/minio`
  and reports CVE-2026-39414 without recognizing the local bounded-reader patch.
  Keep the raw finding and the regression evidence; do not treat a renamed
  module or hidden finding as remediation.

Scan results are time-specific, not a security warranty. Changes to these
assessments require renewed source/reachability review. Public exposure, external
identity providers, KMS and distributed operation require deployment-specific
qualification. Do not use the archived vulnerable baseline image as a service.

## New console boundaries

The console keeps its existing encrypted server-managed session. New routes sign
upstream S3/admin calls with that session's STS credentials; there is no root
credential fallback. Secrets are not stored in browser localStorage. The service
account creation response shows a generated secret once. General status/config
responses redact secrets, and configuration editing is restricted to approved
nonsecret subsystems. Bucket deletion only removes an empty bucket.

TLS/origin tests, full console API tests, native backend tests, browser journeys
and the same-volume suite complement scanners. Remaining qualification gaps
include distributed disk/host failures, cluster upgrades, KMS and replication
integration, exhaustive IAM policy combinations, and production load/soak tests.
