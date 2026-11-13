package pim

import (
	"testing"
	"time"
)

func TestParseDuration(t *testing.T) {
	ok := map[string]time.Duration{
		"30m": 30 * time.Minute, "2h": 2 * time.Hour, "1h30m": 90 * time.Minute,
		" 2H ": 2 * time.Hour, "5m": 5 * time.Minute,
	}
	for in, want := range ok {
		got, err := ParseDuration(in)
		if err != nil || got != want {
			t.Errorf("ParseDuration(%q) = %v, %v; want %v, nil", in, got, err, want)
		}
	}
	bad := []string{"", "abc", "2", "4m", "0h", "-1h", "90s", "1h30s", "1.5h", "2d"}
	for _, in := range bad {
		if got, err := ParseDuration(in); err == nil {
			t.Errorf("ParseDuration(%q) = %v, nil; want error", in, got)
		}
	}
}

func TestFormatDuration(t *testing.T) {
	cases := map[time.Duration]string{
		2 * time.Hour: "2h", 90 * time.Minute: "1h30m", 30 * time.Minute: "30m", 8 * time.Hour: "8h",
	}
	for in, want := range cases {
		if got := FormatDuration(in); got != want {
			t.Errorf("FormatDuration(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestISO8601RoundTrip(t *testing.T) {
	format := map[time.Duration]string{
		2 * time.Hour: "PT2H", 90 * time.Minute: "PT1H30M", 30 * time.Minute: "PT30M",
	}
	for in, want := range format {
		if got := FormatISO8601(in); got != want {
			t.Errorf("FormatISO8601(%v) = %q, want %q", in, got, want)
		}
	}
	parse := map[string]time.Duration{
		"PT8H": 8 * time.Hour, "PT30M": 30 * time.Minute, "PT1H30M": 90 * time.Minute,
		"P1D": 24 * time.Hour, "P1DT2H": 26 * time.Hour,
	}
	for in, want := range parse {
		got, err := ParseISO8601(in)
		if err != nil || got != want {
			t.Errorf("ParseISO8601(%q) = %v, %v; want %v, nil", in, got, err, want)
		}
	}
	for _, in := range []string{"", "8H", "PT"} {
		if got, err := ParseISO8601(in); err == nil {
			t.Errorf("ParseISO8601(%q) = %v, nil; want error", in, got)
		}
	}
}
