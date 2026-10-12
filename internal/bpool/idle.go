// Copyright (c) 2026 Minigo contributors.
// SPDX-License-Identifier: AGPL-3.0-or-later

package bpool

// TrimIdle releases cached buffers beyond keep after a complete inactive
// maintenance interval. Checked-out buffers always remain owned by callers.
// It returns the backing capacity released, not bytes returned to the OS.
func (bp *BytePoolCap) TrimIdle(keep int) int {
	if bp == nil || bp.used.Swap(false) {
		return 0
	}
	keep = max(keep, 0)
	freed := 0
	// Bound the work to the idle snapshot; never chase concurrent returns.
	for n := len(bp.c) - keep; n > 0; n-- {
		if bp.used.Load() {
			break // A new request restarted activity.
		}
		select {
		case <-bp.c:
			freed += bp.wcap
		default:
			return freed
		}
	}
	return freed
}
