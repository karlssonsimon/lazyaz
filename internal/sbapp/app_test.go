package sbapp

import (
	"testing"

	"azure-storage/internal/ui"
)

func TestTrimToWidth(t *testing.T) {
	tests := []struct {
		name  string
		input string
		max   int
		want  string
	}{
		{name: "short string", input: "hello", max: 10, want: "hello"},
		{name: "exact fit", input: "hello", max: 5, want: "hello"},
		{name: "truncated", input: "hello world", max: 8, want: "hello..."},
		{name: "zero max", input: "hello", max: 0, want: ""},
		{name: "max 3", input: "hello", max: 3, want: "hel"},
		{name: "max 2", input: "hello", max: 2, want: "he"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ui.TrimToWidth(tc.input, tc.max); got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestTruncateForStatus(t *testing.T) {
	tests := []struct {
		name  string
		input string
		max   int
		want  string
	}{
		{name: "short", input: "hello", max: 10, want: "hello"},
		{name: "exact", input: "hello", max: 5, want: "hello"},
		{name: "truncated", input: "hello world", max: 5, want: "hello..."},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := truncateForStatus(tc.input, tc.max); got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestPaneName(t *testing.T) {
	tests := []struct {
		pane int
		want string
	}{
		{subscriptionsPane, "subscriptions"},
		{namespacesPane, "namespaces"},
		{entitiesPane, "entities"},
		{detailPane, "detail"},
		{99, "items"},
	}

	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			if got := paneName(tc.pane); got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestEntityDisplayName(t *testing.T) {
	tests := []struct {
		name   string
		entity struct {
			name string
			kind int
		}
		want string
	}{
		{name: "queue", entity: struct {
			name string
			kind int
		}{"orders", 0}, want: "[Q] orders"},
		{name: "topic", entity: struct {
			name string
			kind int
		}{"events", 1}, want: "[T] events"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tag := "[Q]"
			if tc.entity.kind == 1 {
				tag = "[T]"
			}
			got := tag + " " + tc.entity.name
			if got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}
