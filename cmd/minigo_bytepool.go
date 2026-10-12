// Copyright (c) 2026 Minigo contributors.
// SPDX-License-Identifier: AGPL-3.0-or-later

package cmd

import (
	"context"
	"time"
)

// Limit retained buffers to half the effective memory budget. Each buffer's
// backing capacity is twice blockSizeV2, regardless of its current length.
// Active requests may allocate beyond this cache; admission and Go's soft
// memory limit remain separate controls.
func bytePoolCapacity(memoryBudget uint64, distributed, small bool) uint64 {
	n := uint64(1024)
	if distributed {
		n = 2048
	}
	if small {
		n = 256
	}
	return min(n, memoryBudget/(4*blockSizeV2))
}

// One server-scoped worker trims only buffers returned to the pool after a
// complete inactive interval. Keep a small warm cache (at most 32 MiB); a new
// burst can still grow to the configured cap. Do not force a garbage collection
// or touch any in-flight request's buffer.
func maintainBytePool(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			globalBytePoolCap.Load().TrimIdle(16)
		}
	}
}
