// Copyright (c) 2026 Minigo contributors.
// SPDX-License-Identifier: AGPL-3.0-or-later

package bpool

import (
	"sync"
	"testing"
)

func TestBytePoolIdleRelease(t *testing.T) {
	p := NewBytePoolCap(32, 1024, 2048)
	checkedOut := make([][]byte, 32)
	for i := range checkedOut {
		checkedOut[i] = p.Get()
		checkedOut[i][0] = byte(i)
	}
	// Keep one outstanding buffer: maintenance must never reclaim or alter it.
	for _, b := range checkedOut[1:] {
		p.Put(b)
	}
	if freed := p.TrimIdle(2); freed != 0 {
		t.Fatalf("active interval released %d bytes", freed)
	}
	if freed := p.TrimIdle(2); freed != 29*2048 {
		t.Fatalf("idle interval released %d bytes; want %d", freed, 29*2048)
	}
	if checkedOut[0][0] != 0 || len(p.c) != 2 {
		t.Fatal("maintenance altered checked-out data or retained count")
	}
	for i := 1; i < 32; i++ {
		if checkedOut[i][0] != byte(i) {
			t.Fatal("maintenance modified buffer contents")
		}
	}
	p.Put(checkedOut[0])
	b := p.Get()
	p.Put(b)
	if freed := p.TrimIdle(0); freed != 0 {
		t.Fatal("new activity must prevent idle release")
	}
	if freed := p.TrimIdle(0); freed != 3*2048 {
		t.Fatalf("second idle release: %d", freed)
	}
	if freed := p.TrimIdle(-1); freed != 0 {
		t.Fatalf("empty pool release: %d", freed)
	}
	var nilPool *BytePoolCap
	if nilPool.TrimIdle(0) != 0 {
		t.Fatal("nil pool must be harmless")
	}
}

func TestBytePoolConcurrentMaintenance(t *testing.T) {
	p := NewBytePoolCap(16, 1024, 2048)
	var wg sync.WaitGroup
	for id := range 8 {
		wg.Go(func() {
			for range 1000 {
				b := p.Get()
				for i := range b {
					b[i] = byte(id)
				}
				for _, value := range b {
					if value != byte(id) {
						t.Error("buffer concurrently reused while checked out")
						return
					}
				}
				p.Put(b)
			}
		})
	}
	wg.Go(func() {
		for range 1000 {
			p.TrimIdle(2)
		}
	})
	wg.Wait()
}

func BenchmarkBytePoolReuse(b *testing.B) {
	for _, parallel := range []bool{false, true} {
		name := "serial"
		if parallel {
			name = "parallel"
		}
		b.Run(name, func(b *testing.B) {
			p := NewBytePoolCap(64, 1<<20, 2<<20)
			p.Populate()
			b.ReportAllocs()
			b.ResetTimer()
			if parallel {
				b.RunParallel(func(pb *testing.PB) {
					for pb.Next() {
						data := p.Get()
						data[0] = 1
						p.Put(data)
					}
				})
			} else {
				for range b.N {
					data := p.Get()
					data[0] = 1
					p.Put(data)
				}
			}
		})
	}
}

// Measure logical retained backing capacity, not process RSS or OS reclamation.
func BenchmarkBytePoolBurstIdle(b *testing.B) {
	p := NewBytePoolCap(64, 1<<20, 2<<20)
	bufs := make([][]byte, 64)
	for i := range bufs {
		bufs[i] = p.Get()
	}
	for _, data := range bufs {
		p.Put(data)
	}
	bufs = nil
	p.TrimIdle(16) // Observe the active interval.
	p.TrimIdle(16) // One complete interval without any new request.
	b.ReportMetric(float64(len(p.c)*p.wcap), "retained-B")
	for range b.N {
	}
}
