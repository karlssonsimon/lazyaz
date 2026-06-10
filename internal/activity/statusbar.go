package activity

import (
	"fmt"
)

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
