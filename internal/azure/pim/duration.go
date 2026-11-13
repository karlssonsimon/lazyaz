package pim

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// MinDuration is the shortest activation the UI accepts.
const MinDuration = 5 * time.Minute

var (
	userDurationRe = regexp.MustCompile(`^(\d+h)?(\d+m)?$`)
	isoDurationRe  = regexp.MustCompile(`^P(?:(\d+)D)?(?:T(?:(\d+)H)?(?:(\d+)M)?)?$`)
)

// ParseDuration parses user input such as "30m", "2h" or "1h30m". Only hours
// and minutes are accepted, and the result must be at least MinDuration.
func ParseDuration(s string) (time.Duration, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" || !userDurationRe.MatchString(s) {
		return 0, fmt.Errorf("invalid duration %q: use hours and minutes, e.g. 30m, 2h, 1h30m", s)
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q: %w", s, err)
	}
	if d < MinDuration {
		return 0, fmt.Errorf("duration %q is below the minimum of %s", s, FormatDuration(MinDuration))
	}
	return d, nil
}

// FormatDuration renders d as "2h", "1h30m" or "30m".
func FormatDuration(d time.Duration) string {
	h := int(d / time.Hour)
	m := int(d % time.Hour / time.Minute)
	switch {
	case h > 0 && m > 0:
		return fmt.Sprintf("%dh%dm", h, m)
	case h > 0:
		return fmt.Sprintf("%dh", h)
	default:
		return fmt.Sprintf("%dm", m)
	}
}

// ParseISO8601 parses the P[nD][T[nH][nM]] subset of ISO 8601 durations.
func ParseISO8601(s string) (time.Duration, error) {
	m := isoDurationRe.FindStringSubmatch(s)
	if m == nil || s == "P" || strings.HasSuffix(s, "T") {
		return 0, fmt.Errorf("invalid ISO 8601 duration %q", s)
	}
	var d time.Duration
	for i, unit := range []time.Duration{24 * time.Hour, time.Hour, time.Minute} {
		if m[i+1] == "" {
			continue
		}
		n, err := strconv.Atoi(m[i+1])
		if err != nil {
			return 0, fmt.Errorf("invalid ISO 8601 duration %q: %w", s, err)
		}
		d += time.Duration(n) * unit
	}
	return d, nil
}

// FormatISO8601 renders d as "PT2H", "PT1H30M" or "PT30M".
func FormatISO8601(d time.Duration) string {
	h := int(d / time.Hour)
	m := int(d % time.Hour / time.Minute)
	out := "PT"
	if h > 0 {
		out += strconv.Itoa(h) + "H"
	}
	if m > 0 || h == 0 {
		out += strconv.Itoa(m) + "M"
	}
	return out
}
