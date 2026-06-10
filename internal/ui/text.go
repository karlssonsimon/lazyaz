package ui

import "time"

func TrimToWidth(s string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max <= 3 {
		return string(r[:max])
	}
	return string(r[:max-3]) + "..."
}

func EmptyToDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func FormatTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Local().Format("2006-01-02 15:04")
}

// FormatThousands formats n with space thousand separators
// (e.g. 6051 → "6 051"), matching typical terminal-app conventions.
// Negative numbers preserve their sign.
func FormatThousands(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [32]byte
	pos := len(buf)
	digits := 0
	for n > 0 {
		if digits == 3 {
			pos--
			buf[pos] = ' '
			digits = 0
		}
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
		digits++
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
