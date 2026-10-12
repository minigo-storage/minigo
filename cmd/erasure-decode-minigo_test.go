// SPDX-License-Identifier: AGPL-3.0-or-later

package cmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/minio/minio/internal/bpool"
)

// Keep shards in memory so the benchmark isolates decoding allocations rather
// than disk, checksum or network costs. The regular erasure tests cover those.
func minigoDecodeFixture(tb testing.TB, blockSize int64) (Erasure, []byte, [][]byte) {
	tb.Helper()
	e, err := NewErasure(context.Background(), 4, 4, blockSize)
	if err != nil {
		tb.Fatal(err)
	}
	data := make([]byte, 2*blockSize+123)
	state := uint32(1)
	for i := range data {
		state ^= state << 13
		state ^= state >> 17
		state ^= state << 5
		data[i] = byte(state)
	}
	shards := make([][]byte, 8)
	for offset := int64(0); offset < int64(len(data)); offset += blockSize {
		block := bytes.Clone(data[offset:min(offset+blockSize, int64(len(data)))])
		encoded, err := e.EncodeData(context.Background(), block)
		if err != nil {
			tb.Fatal(err)
		}
		for i := range shards {
			shards[i] = append(shards[i], encoded[i]...)
		}
	}
	return e, data, shards
}

func minigoDecodeReaders(shards [][]byte, degraded bool) []io.ReaderAt {
	readers := make([]io.ReaderAt, len(shards))
	for i := range shards {
		if degraded && (i == 0 || i == 2) {
			continue
		}
		readers[i] = bytes.NewReader(shards[i])
	}
	return readers
}

func minigoDecodePool(tb testing.TB) *bpool.BytePoolCap {
	tb.Helper()
	pool := bpool.NewBytePoolCap(1, 1<<20, 2<<20)
	pool.Populate()
	previous := globalBytePoolCap.Swap(pool)
	tb.Cleanup(func() { globalBytePoolCap.Store(previous) })
	return pool
}

func TestMinigoErasureDecodeBuffers(t *testing.T) {
	for _, blockSize := range []int64{1 << 20, 4 << 20} {
		name := "pooled"
		if blockSize == 4<<20 {
			name = "legacy-oversized"
		}
		t.Run(name, func(t *testing.T) {
			pool := minigoDecodePool(t)
			e, data, shards := minigoDecodeFixture(t, blockSize)
			for _, scenario := range []struct {
				name     string
				offset   int64
				length   int64
				degraded bool
				prefer   []bool
			}{
				{name: "full", length: int64(len(data))},
				{name: "range", offset: blockSize - 17, length: blockSize + 73},
				{name: "degraded", length: int64(len(data)), degraded: true},
				{name: "preferred-parity", length: int64(len(data)), prefer: []bool{false, false, false, false, true, true, true, true}},
			} {
				t.Run(scenario.name, func(t *testing.T) {
					// Repeat to reuse both the pool and reconstruction buffers across blocks.
					for i := 0; i < 3; i++ {
						var output bytes.Buffer
						n, err := e.Decode(t.Context(), &output, minigoDecodeReaders(shards, scenario.degraded), scenario.offset, scenario.length, int64(len(data)), scenario.prefer)
						if err != nil || n != scenario.length {
							t.Fatalf("Decode = %d, %v; want %d, nil", n, err, scenario.length)
						}
						if !bytes.Equal(output.Bytes(), data[scenario.offset:scenario.offset+scenario.length]) {
							t.Fatal("decoded bytes differ from original data")
						}
						if pool.CurrentSize() != pool.Width() {
							t.Fatal("Decode did not return its buffer to the pool")
						}
					}
					// Heal shares the parallel reader and reconstructs parity too.
					writers := make([]io.Writer, len(shards))
					outputs := make([]bytes.Buffer, len(shards))
					for i := range writers {
						writers[i] = &outputs[i]
					}
					if err := e.Heal(t.Context(), writers, minigoDecodeReaders(shards, scenario.degraded), int64(len(data)), scenario.prefer); err != nil {
						t.Fatal(err)
					}
					for i := range outputs {
						if !bytes.Equal(outputs[i].Bytes(), shards[i]) {
							t.Fatalf("healed shard %d differs from original shard", i)
						}
					}
				})
			}
		})
	}
}

type minigoFailingDecodeReader struct {
	io.ReaderAt
	failAt int64
}

func (r minigoFailingDecodeReader) ReadAt(p []byte, off int64) (int, error) {
	if off >= r.failAt {
		return 0, errFileCorrupt
	}
	return r.ReaderAt.ReadAt(p, off)
}

func TestMinigoErasureDecodeMidReadFailure(t *testing.T) {
	pool := minigoDecodePool(t)
	e, data, shards := minigoDecodeFixture(t, 1<<20)
	readers := minigoDecodeReaders(shards, false)
	readers[0] = minigoFailingDecodeReader{ReaderAt: readers[0], failAt: e.ShardSize()}
	var output bytes.Buffer
	n, err := e.Decode(t.Context(), &output, readers, 0, int64(len(data)), int64(len(data)), nil)
	if !errors.Is(err, errFileCorrupt) || n != int64(len(data)) || !bytes.Equal(output.Bytes(), data) {
		t.Fatalf("Decode after a disk fails mid-read = %d, %v; reconstructed bytes must match", n, err)
	}
	if readers[0] != nil || pool.CurrentSize() != pool.Width() {
		t.Fatal("failed reader was not removed or pooled buffer was not returned")
	}
}

func TestMinigoParallelReaderUsesIsolatedPoolShards(t *testing.T) {
	pool := minigoDecodePool(t)
	e, data, shards := minigoDecodeFixture(t, 1<<20)
	reader := newParallelReader(minigoDecodeReaders(shards, false), e, 0, int64(len(data)))
	defer reader.Done()
	if pool.CurrentSize() != 0 {
		t.Fatal("parallel reader did not reserve its pooled buffer")
	}
	for i, buf := range reader.buf {
		if len(buf) != int(e.ShardSize()) || cap(buf) != int(e.ShardSize()) {
			t.Fatalf("shard %d length/capacity = %d/%d; want %d/%d", i, len(buf), cap(buf), e.ShardSize(), e.ShardSize())
		}
		if &buf[0] != &reader.stashBuffer[:cap(reader.stashBuffer)][i*int(e.ShardSize())] {
			t.Fatalf("shard %d does not use reserved pool storage", i)
		}
	}
	reader.Done()
	if pool.CurrentSize() != pool.Width() {
		t.Fatal("Done did not release its buffer")
	}
	// Borrow the returned buffer before calling Done again: a repeated release
	// must not expose the same backing array to two live borrowers.
	borrowed := pool.Get()
	defer pool.Put(borrowed)
	reader.Done()
	if pool.CurrentSize() != 0 {
		t.Fatal("Done released an already returned buffer twice")
	}
}

type minigoDecodeWriter struct {
	pool *bpool.BytePoolCap
	err  error
}

func (w minigoDecodeWriter) Write(p []byte) (int, error) {
	if w.pool.CurrentSize() != 0 {
		return 0, errors.New("buffer released before output write completed")
	}
	if w.err != nil {
		return 0, w.err
	}
	return len(p), nil
}

func TestMinigoErasureDecodeBufferLifetime(t *testing.T) {
	pool := minigoDecodePool(t)
	e, data, shards := minigoDecodeFixture(t, 1<<20)
	wantErr := errors.New("output failed")
	for _, writeErr := range []error{nil, wantErr} {
		_, err := e.Decode(t.Context(), minigoDecodeWriter{pool: pool, err: writeErr}, minigoDecodeReaders(shards, false), 0, int64(len(data)), int64(len(data)), nil)
		if !errors.Is(err, writeErr) {
			t.Fatalf("Decode error = %v; want %v", err, writeErr)
		}
		if pool.CurrentSize() != pool.Width() {
			t.Fatal("buffer leaked after output completed or failed")
		}
	}
	_, err := e.Decode(t.Context(), io.Discard, make([]io.ReaderAt, 8), 0, int64(len(data)), int64(len(data)), nil)
	if !errors.Is(err, errErasureReadQuorum) || pool.CurrentSize() != pool.Width() {
		t.Fatalf("quorum failure did not release buffer: %v", err)
	}
}

func BenchmarkMinigoErasureDecodeBuffers(b *testing.B) {
	for _, scenario := range []struct {
		name      string
		blockSize int64
		ranged    bool
		degraded  bool
		prefer    []bool
	}{
		{name: "full", blockSize: 1 << 20},
		{name: "range", blockSize: 1 << 20, ranged: true},
		{name: "degraded", blockSize: 1 << 20, degraded: true},
		{name: "preferred-parity", blockSize: 1 << 20, prefer: []bool{false, false, false, false, true, true, true, true}},
		{name: "legacy-oversized", blockSize: 4 << 20},
	} {
		b.Run(scenario.name, func(b *testing.B) {
			minigoDecodePool(b)
			e, data, shards := minigoDecodeFixture(b, scenario.blockSize)
			offset, length := int64(0), int64(len(data))
			if scenario.ranged {
				offset, length = scenario.blockSize-17, scenario.blockSize+73
			}
			b.ReportAllocs()
			b.SetBytes(length)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				n, err := e.Decode(b.Context(), io.Discard, minigoDecodeReaders(shards, scenario.degraded), offset, length, int64(len(data)), scenario.prefer)
				if err != nil || n != length {
					b.Fatalf("Decode = %d, %v", n, err)
				}
			}
		})
	}
}
