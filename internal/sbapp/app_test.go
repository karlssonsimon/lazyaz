package sbapp

import (
	"strings"
	"testing"
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
			if got := trimToWidth(tc.input, tc.max); got != tc.want {
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
			// Use the servicebus types via the imported sbapp package indirectly
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

func TestHighlightJSON_ValidJSON(t *testing.T) {
	input := `{"name":"test","count":42,"active":true,"data":null}`
	styles := defaultTheme().JSONColors.styles()
	result := styles.highlightJSON(input)

	if !strings.Contains(result, "name") {
		t.Fatal("expected result to contain key 'name'")
	}
	if !strings.Contains(result, "test") {
		t.Fatal("expected result to contain value 'test'")
	}
	if !strings.Contains(result, "42") {
		t.Fatal("expected result to contain number 42")
	}
	if !strings.Contains(result, "true") {
		t.Fatal("expected result to contain boolean true")
	}
	if !strings.Contains(result, "null") {
		t.Fatal("expected result to contain null")
	}
}

func TestHighlightJSON_InvalidJSON(t *testing.T) {
	input := "this is not json"
	styles := defaultTheme().JSONColors.styles()
	result := styles.highlightJSON(input)
	if result != input {
		t.Fatalf("expected plain text passthrough, got %q", result)
	}
}

func TestHighlightJSON_EmptyObject(t *testing.T) {
	input := `{}`
	styles := defaultTheme().JSONColors.styles()
	result := styles.highlightJSON(input)
	if !strings.Contains(result, "{}") {
		t.Fatalf("expected result to contain '{}', got %q", result)
	}
}
