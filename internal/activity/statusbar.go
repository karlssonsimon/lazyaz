package activity

import (
	"fmt"
	"time"
)

// shortFetchGrace is the minimum running duration before a fetch
// counts toward the status-bar indicator. Keeps sub-second fetches
// from flickering the UI.
const shortFetchGrace = 2 * time.Second

// StatusBarItem inspects the registry and returns the headline string
// for the status bar indicator. The second return is false when nothing
// noteworthy is running.
//
// Rules (in order):
//  1. Any KindUpload running → "↑ <rate> · <keyHint> for activity"
//     (if multiple uploads, use the slowest rate — the bottleneck)
//  2. Any activity running > shortFetchGrace → "N active · <keyHint> for activity"
//  3. Nothing noteworthy → ok=false
func StatusBarItem(r *Registry, keyHint string) (value string, ok bool) {
	if r == nil {
		return "", false
	}
	views := r.Snapshot()

	// Pass 1: find uploads in Running state.
	var slowestUpload *Snapshot
	for i := range views {
		v := &views[i]
		if v.Activity.Kind() != KindUpload {
			continue
		}
		if v.Snapshot.Status != StatusRunning {
			continue
		}
		if slowestUpload == nil || v.Snapshot.BytesPerSec < slowestUpload.BytesPerSec {
			s := v.Snapshot
			slowestUpload = &s
		}
	}
	if slowestUpload != nil {
		return fmt.Sprintf("↑ %s · %s for activity", FormatDecimalRate(slowestUpload.BytesPerSec), keyHint), true
	}

	// Pass 2: count running activities older than grace.
	now := r.clock.Now()
	count := 0
	for _, v := range views {
		if v.Snapshot.Status != StatusRunning {
			continue
		}
		if now.Sub(v.Snapshot.StartedAt) < shortFetchGrace {
			continue
		}
		count++
	}
	if count == 0 {
		return "", false
	}
	return fmt.Sprintf("%d active · %s for activity", count, keyHint), true
}

// FormatDecimalRate formats a bytes-per-second rate using decimal SI
// units. Exported so the ui package can call it without duplicating.
func FormatDecimalRate(bps float64) string {
	const (
		kb = 1000.0
		mb = kb * 1000
		gb = mb * 1000
	)
	switch {
	case bps >= gb:
		return fmt.Sprintf("%.2f GB/s", bps/gb)
	case bps >= mb:
		return fmt.Sprintf("%.2f MB/s", bps/mb)
	case bps >= kb:
		return fmt.Sprintf("%.1f KB/s", bps/kb)
	default:
		return fmt.Sprintf("%.0f B/s", bps)
	}
}

// FormatDecimalBytes formats a byte count using decimal SI units.
// Exported so the ui package can call it without duplicating.
func FormatDecimalBytes(bytes int64) string {
	const (
		kb = 1000
		mb = kb * 1000
		gb = mb * 1000
	)
	switch {
	case bytes >= gb:
		return fmt.Sprintf("%.2f GB", float64(bytes)/float64(gb))
	case bytes >= mb:
		return fmt.Sprintf("%.2f MB", float64(bytes)/float64(mb))
	case bytes >= kb:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(kb))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
