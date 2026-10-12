# Local S3 performance comparison

Run `scripts/benchmark-s3.sh` after building two distinct local images. Defaults
are `minigo:perf-baseline-arm64` and `minigo:local-arm64`, linux/arm64, and an
ABBA sequence (baseline, target, target, baseline). Override `BASELINE_IMAGE`,
`TARGET_IMAGE`, `PERF_PLATFORM`, or `PERF_ORDER` if needed. Images are pinned to
their local IDs at startup and never pulled. Do not run alongside builds,
other benchmarks, or compatibility workloads.

Every run creates a new random container and volume, with loopback-only ports
and private random credentials. Cleanup removes only those generated resources,
including on failure. Reports and logs remain under `dist/performance/s3-*`.

The deterministic fixture contains 2,048 one-KiB objects and a 16-MiB object.
The driver verifies every fixture byte before timing, then measures concurrency
1 and 8 for small-object GET, complete paginated LIST, 16-MiB GET, and signed
16-MiB PUT carrying both whole-payload and S3 SHA256 digests. A transport guard
checks the actual outgoing headers so an SDK fallback cannot silently replace
that upload mode. Every upload is independently read back and hashed outside
the PUT timing. Each run writes 32 large objects, plus the source object.

JSON reports contain operation throughput, MiB/s, p50 and p95 latency. Each
LIST operation walks and checks all pages, so its rate is complete listings per
second. These warm-cache, single-node, Docker-loopback measurements include
client/network work and GET verification. They are not isolated server CPU
measurements or predictions for a distributed deployment. The Docker stats
snapshot is post-workload context, not a peak-memory or sustained-CPU metric.
