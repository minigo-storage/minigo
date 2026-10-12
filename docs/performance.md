# Performance optimization — October 2026

This pass retains the storage format, API, write quorum, erasure parity,
checksums and encryption. No production dependency was added. The pre-change
arm64 image is preserved as `minigo:perf-baseline-arm64`, image ID
`sha256:354d792f7044c081f031bb0d2b4cd2f4efab010acb2134deb48e7b485465214b`.

## Changes retained

### Reuse the reserved read buffers

The parallel erasure reader prepared slices of a pooled buffer, then discarded
them and allocated separate shard buffers. It now uses the reserved slices,
with each slice's capacity bounded so it cannot overwrite another shard.
Legacy oversized shards retain their original allocation path. Reconstruction
still uses the existing Reed-Solomon implementation.

In-memory 4+4 erasure decoding on Darwin arm64, Apple M4 Pro, Go 1.27.2,
three one-second samples per case:

| Workload | Before allocated bytes/op | After allocated bytes/op | Before median time | After median time |
| --- | ---: | ---: | ---: | ---: |
| Full read | 1,051,856 | 3,088 | 45.5 µs | 27.5 µs |
| Cross-block range | 1,051,856 | 3,088 | 51.7 µs | 27.3 µs |
| Two unavailable data shards | 1,595,585 | 546,817 | See raw report | See raw report |
| Prefer parity shards | 2,135,856 | 1,087,088 | See raw report | See raw report |

These measure allocation and CPU time in a memory-backed decoder, **not process
RSS or end-to-end S3 throughput**. Fixtures verify every output byte through
normal reads, ranges, degradation, parity preference, oversized legacy blocks,
healing, mid-read corruption, and buffer ownership on success/error paths.

[Raw results and details](../dist/performance/read-buffer-report.md).

### Bound the cache and release idle buffers

The byte pool's retained capacity now respects the smaller of available memory
and the explicit `MINIO_MEMLIMIT`/`--memlimit` budget, with at most half of that
budget reserved for returned buffers. Previous single-node/distributed entry
ceilings remain. Active requests and other caches still consume memory outside
this cache: this is not a hard process RSS limit.

A server-scoped worker checks once per minute. After a complete interval with
no new buffer acquisition, it releases excess returned buffers and keeps at
most 16 warm buffers (32 MiB). It never touches checked-out buffers or forces a
garbage collection. Steady traffic can retain the larger cache; subsequent
bursts may need new allocations. The Go runtime decides when released memory
is reclaimed and returned to the OS.

A deterministic 64-buffer burst retained **128 MiB before, 32 MiB after** the
idle policy. The serial Get/Put microbenchmark changed from 19.44 to 20.55 ns/op
(about 1.1 ns overhead), still zero allocations. Parallel timings were noisy
and are not treated as a throughput gain. Tests cover concurrent borrowers,
idle transitions, memory budgets, worker timing and server-context cancellation.

[Measurement details](../dist/performance/bpool-comparison.json).

### Compute identical SHA-256 streams once

Uploads supplying both a whole-payload SHA-256 and an S3 SHA-256 checksum now
share computation when both cover the same bytes from the beginning. Both
expected values are still checked separately, in the same order. Other checksum
algorithms, late-added checksums and the externally accessible server-side
hasher remain separate.

In isolated 32 KiB streaming reads, five 300 ms samples, one CPU:

| Payload | Hashing mode | Median processing-time reduction |
| --- | --- | ---: |
| 64 KiB | MD5 ETag plus both SHA-256 checks | 19.5% |
| 8 MiB | MD5 ETag plus both SHA-256 checks | 17.8% |
| 64 KiB | Both SHA-256 checks, MD5 already disabled | 49.7% |
| 8 MiB | Both SHA-256 checks, MD5 already disabled | 47.5% |

Each applicable reader saves 160 allocated bytes and two allocations. These
numbers do not apply to unsigned uploads or requests with only one SHA-256.
Single-digest control timings varied, so small timing changes are not attributed
to this patch. MD5/ETag behavior is unchanged.

Regression testing also exposed an inherited reader-merge validation defect.
Its correction is documented in [security compatibility notes](minigo-security.md).
Valid, mismatching, missing and malformed checksum/trailer cases are tested.

[Raw results and details](../dist/performance/checksum-report.md).

## Experiment deferred

Smaller listing queues reduced allocation by 20.8% and time by 15.1% for a
100-object pipeline. For 10,000 objects, allocation fell 6.9% but processing
time increased 5.8%. The production listing changes were **reverted**. Original
queue sizes and persisted metacache behavior remain; the reusable benchmark and
ordering/error/cancellation tests are retained. The rejected patch and results
remain in `dist/performance/listing-*`.

## Reproduction

Run benchmarks sequentially without other builds or workloads. New regression
and benchmark sources are committed; raw reports and binaries are local ignored
artifacts in `dist/performance/`.

```sh
go test ./cmd -run '^$' -bench '^BenchmarkMinigoErasureDecodeBuffers$' -benchmem -count=3
go test ./internal/hash -run '^$' -bench 'Benchmark.*SHA' -benchmem -count=5 -cpu=1
go test ./internal/bpool -run '^$' -bench '^BenchmarkBytePool' -benchmem -count=5 -cpu=4
```

The burst-idle benchmark's `retained-B` metric measures held backing capacity;
its empty-loop `ns/op` is not a useful timing result. Tests that require embedded
console files assume `make build` has generated them.

[`scripts/benchmark-s3.sh`](../scripts/benchmark-s3.sh) provides an independent
ABBA comparison using new disposable containers/volumes and identical verified
fixtures. Its [scope and workload](../tests/performance/README.md) include 1 KiB
GETs, complete paginated listings, and 16 MiB GET/dual-SHA PUT at concurrency
one and eight. SDK/network/verification costs are included; these are local
warm-cache observations, not native amd64 or distributed performance claims.
