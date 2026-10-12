// Copyright (c) 2026 Minigo contributors.
// SPDX-License-Identifier: AGPL-3.0-or-later

package cmd

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/minio/minio/internal/bpool"
)

func TestBytePoolMaintenance(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		pool := bpool.NewBytePoolCap(32, 1024, 2048)
		buffers := make([][]byte, 32)
		for i := range buffers {
			buffers[i] = pool.Get()
		}
		for _, b := range buffers {
			pool.Put(b)
		}
		previous := globalBytePoolCap.Swap(pool)
		defer globalBytePoolCap.Store(previous)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		done := make(chan struct{})
		go func() { maintainBytePool(ctx); close(done) }()
		synctest.Wait()
		time.Sleep(time.Minute)
		synctest.Wait()
		if pool.CurrentSize() != 32*1024 {
			t.Fatal("worker trimmed an interval with request activity")
		}
		time.Sleep(time.Minute)
		synctest.Wait()
		if pool.CurrentSize() != 16*1024 {
			t.Fatal("worker did not retain exactly the warm cache after idle")
		}
		cancel()
		synctest.Wait()
		select {
		case <-done:
		default:
			t.Fatal("worker did not stop with server context")
		}
	})
}

func TestBytePoolCapacityRespectsMemoryBudget(t *testing.T) {
	for _, tc := range []struct {
		name               string
		budget             uint64
		distributed, small bool
		want               uint64
	}{
		{"unset", 0, false, false, 0},
		{"tiny", 1 << 20, false, false, 0},
		{"512MiB", 512 << 20, false, false, 128},
		{"1GiB", 1 << 30, false, false, 256},
		{"1GiB-distributed", 1 << 30, true, false, 256},
		{"large-single", 32 << 30, false, false, 1024},
		{"large-distributed", 32 << 30, true, false, 2048},
		{"large-test", 32 << 30, false, true, 256},
		{"small-test", 128 << 20, false, true, 32},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := bytePoolCapacity(tc.budget, tc.distributed, tc.small)
			if got != tc.want || got*2*blockSizeV2 > tc.budget/2 {
				t.Fatalf("capacity %d for budget %d, want %d and <= half budget", got, tc.budget, tc.want)
			}
		})
	}
}
