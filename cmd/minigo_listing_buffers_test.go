// Copyright (c) 2026 Minigo contributors.
// SPDX-License-Identifier: AGPL-3.0-or-later

package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"
)

// Capture both the block headers and their serialized contents, validating
// round-trip ordering across metacache block boundaries.
func minigoListingBlocks(t *testing.T, queueSize, count int) []metacacheBlock {
	t.Helper()
	in := make(chan metaCacheEntry, queueSize)
	var blocks []metacacheBlock
	w := newMetacacheBlockWriter(in, func(block *metacacheBlock) error {
		copy := *block
		copy.data = bytes.Clone(block.data)
		blocks = append(blocks, copy)
		return nil
	})
	for i := range count {
		in <- metaCacheEntry{name: fmt.Sprintf("objects/%08d", i), metadata: []byte("unchanged object metadata")}
	}
	close(in)
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, block := range blocks {
		r := newMetacacheReader(bytes.NewReader(block.data))
		part, err := r.readNames(-1)
		r.Close()
		if err != io.EOF {
			t.Fatal(err)
		}
		names = append(names, part...)
	}
	if len(names) != count {
		t.Fatalf("saved %d objects, want %d", len(names), count)
	}
	for i, name := range names {
		if want := fmt.Sprintf("objects/%08d", i); name != want {
			t.Fatalf("object %d: got %q, want %q", i, name, want)
		}
	}
	return blocks
}

func TestMinigoListingBlocks(t *testing.T) {
	for _, count := range []int{0, 1, 4999, 5000, 10001} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			minigoListingBlocks(t, metacacheBlockSize, count)
		})
	}
}

func TestMinigoListingBlockErrorDrainsQueue(t *testing.T) {
	in := make(chan metaCacheEntry, metacacheBlockSize)
	want := errors.New("metacache destination unavailable")
	w := newMetacacheBlockWriter(in, func(*metacacheBlock) error { return want })
	done := make(chan error, 1)
	go func() {
		for i := range 10001 {
			in <- metaCacheEntry{name: fmt.Sprintf("objects/%08d", i)}
		}
		close(in)
		done <- w.Close()
	}()
	select {
	case err := <-done:
		if err != want {
			t.Fatalf("got %v, want original storage error", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("storage error did not drain the listing queue")
	}
}

func TestMinigoListingCanceledResultsDrainQueue(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	o := listPathOptions{Limit: 1, Recursive: true, IncludeDirectories: true}
	in := make(chan metaCacheEntry, metacacheBlockSize)
	results := o.gatherResults(ctx, in)
	cancel()
	if _, err := results(); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want canceled", err)
	}
	done := make(chan struct{})
	go func() {
		for i := range 10001 {
			in <- metaCacheEntry{name: fmt.Sprintf("objects/%08d/", i)}
		}
		close(in)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("canceled consumer blocked the background listing")
	}
}

// This isolates the fresh-list producer/forwarder/cache-writer pipeline from disk
// latency. It includes both production queue allocations and actual metacache
// compression, but is not an end-to-end S3 latency or process RSS benchmark.
func BenchmarkMinigoListingPipeline(b *testing.B) {
	samples := loadMetacacheSampleEntries(b)
	var metadata []byte
	for _, entry := range samples.entries() {
		if entry.isObject() {
			metadata = entry.metadata
			break
		}
	}
	if len(metadata) == 0 {
		b.Fatal("missing object metadata fixture")
	}
	for _, test := range []struct{ count, limit int }{{100, 100}, {10000, 1000}} {
		b.Run(fmt.Sprintf("objects=%d/page=%d", test.count, test.limit), func(b *testing.B) {
			entries := make([]metaCacheEntry, test.count)
			for i := range entries {
				entries[i] = metaCacheEntry{name: fmt.Sprintf("objects/%08d", i), metadata: metadata}
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				in := make(chan metaCacheEntry, metacacheBlockSize)
				save := make(chan metaCacheEntry, metacacheBlockSize)
				w := newMetacacheBlockWriter(save, func(*metacacheBlock) error { return nil })
				go func() {
					for _, entry := range entries {
						in <- entry
					}
					close(in)
				}()
				for entry := range in {
					save <- entry
				}
				close(save)
				if err := w.Close(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
