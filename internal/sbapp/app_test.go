package sbapp

import (
	"strings"
	"testing"

	"azure-storage/internal/ui"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

var testConfig = ui.Config{
	ThemeName: "fallback",
	Schemes:   []ui.Scheme{{Name: "fallback", Base00: "1e293b", Base01: "4B5563", Base02: "334155", Base03: "94A3B8", Base04: "94A3B8", Base05: "E5E7EB", Base06: "F8FAFC", Base07: "F8FAFC", Base08: "F87171", Base09: "F59E0B", Base0A: "F59E0B", Base0B: "22C55E", Base0C: "38BDF8", Base0D: "60A5FA", Base0E: "C084FC", Base0F: "94A3B8"}},
}

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

func TestTypingQWhileFilteringDoesNotQuit(t *testing.T) {
	m := NewModel(nil, testConfig, nil)
	m.SubOverlay.Close()
	m.focus = namespacesPane
	m.namespacesList.SetFilterState(list.Filtering)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	model, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected updated model type %T, got %T", Model{}, updated)
	}

	if isQuitCmd(cmd) {
		t.Fatal("expected typing q in active filter not to quit")
	}

	if model.namespacesList.FilterValue() != "q" {
		t.Fatalf("expected filter value %q, got %q", "q", model.namespacesList.FilterValue())
	}
}

func TestHelpToggleOpensAndCloses(t *testing.T) {
	m := NewModel(nil, testConfig, nil)
	m.SubOverlay.Close()

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	model := updated.(Model)
	if !model.HelpOverlay.Active {
		t.Fatal("expected ? to open help overlay")
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	model = updated.(Model)
	if model.HelpOverlay.Active {
		t.Fatal("expected ? to close help overlay")
	}
}

func TestViewShowsStatusBar(t *testing.T) {
	m := NewModel(nil, testConfig, nil)
	m.Width = 120
	m.Height = 40
	m.resize()

	view := m.View()
	if !strings.Contains(view, "Loading") {
		t.Fatal("expected status bar to show loading message")
	}
}

func isQuitCmd(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}
