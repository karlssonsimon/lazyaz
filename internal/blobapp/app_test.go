package blobapp

import (
	"azure-storage/internal/ui"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

var testConfig = ui.Config{
	ThemeName: "fallback",
	Schemes:   []ui.Scheme{ui.FallbackScheme()},
}

func TestParentPrefix(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		output string
	}{
		{name: "root", input: "", output: ""},
		{name: "single folder", input: "foo/", output: ""},
		{name: "nested", input: "foo/bar/", output: "foo/"},
		{name: "nested without trailing slash", input: "foo/bar", output: "foo/"},
		{name: "deep", input: "foo/bar/baz/", output: "foo/bar/"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := parentPrefix(tc.input); got != tc.output {
				t.Fatalf("expected %q, got %q", tc.output, got)
			}
		})
	}
}

func TestTrimPrefixForDisplay(t *testing.T) {
	tests := []struct {
		name   string
		value  string
		prefix string
		want   string
	}{
		{name: "no prefix", value: "folder/file.txt", prefix: "", want: "folder/file.txt"},
		{name: "with prefix", value: "folder/file.txt", prefix: "folder/", want: "file.txt"},
		{name: "same as prefix", value: "folder/", prefix: "folder/", want: "folder/"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := trimPrefixForDisplay(tc.value, tc.prefix); got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestBlobSearchPrefix(t *testing.T) {
	tests := []struct {
		name          string
		currentPrefix string
		query         string
		want          string
	}{
		{name: "plain query at root", currentPrefix: "", query: "foo", want: "foo"},
		{name: "query scoped to current prefix", currentPrefix: "logs/", query: "2026", want: "logs/2026"},
		{name: "query already includes prefix", currentPrefix: "logs/", query: "logs/2026", want: "logs/2026"},
		{name: "leading slash means absolute", currentPrefix: "logs/", query: "/archive/", want: "archive/"},
		{name: "windows slash normalized", currentPrefix: "logs/", query: "2026\\02", want: "logs/2026/02"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := blobSearchPrefix(tc.currentPrefix, tc.query); got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestComputePreviewWindow(t *testing.T) {
	tests := []struct {
		name         string
		totalSize    int64
		cursor       int64
		visibleLines int
	}{
		{name: "small blob", totalSize: 1024, cursor: 0, visibleLines: 20},
		{name: "middle of large blob", totalSize: 10 * 1024 * 1024, cursor: 5 * 1024 * 1024, visibleLines: 30},
		{name: "near end", totalSize: 10 * 1024 * 1024, cursor: 10*1024*1024 - 10, visibleLines: 25},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			start, count := computePreviewWindow(tc.totalSize, tc.cursor, tc.visibleLines)
			if start < 0 {
				t.Fatalf("expected non-negative start, got %d", start)
			}
			if count < 0 {
				t.Fatalf("expected non-negative count, got %d", count)
			}
			if start+count > tc.totalSize {
				t.Fatalf("window exceeds blob bounds: start=%d count=%d total=%d", start, count, tc.totalSize)
			}
		})
	}
}

func TestTypingQWhileFilteringDoesNotQuit(t *testing.T) {
	m := NewModel(nil, testConfig)
	m.focus = blobsPane
	m.blobsList.SetFilterState(list.Filtering)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	model, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected updated model type %T, got %T", Model{}, updated)
	}

	if isQuitCmd(cmd) {
		t.Fatal("expected typing q in active filter not to quit")
	}

	if model.blobsList.FilterValue() != "q" {
		t.Fatalf("expected filter value %q, got %q", "q", model.blobsList.FilterValue())
	}
}

func TestHelpToggleOpensAndCloses(t *testing.T) {
	m := NewModel(nil, testConfig)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	model := updated.(Model)
	if !model.helpOverlay.Active {
		t.Fatal("expected ? to open help overlay")
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	model = updated.(Model)
	if model.helpOverlay.Active {
		t.Fatal("expected ? to close help overlay")
	}
}

func TestViewShowsCompactHelpHint(t *testing.T) {
	m := NewModel(nil, testConfig)
	m.width = 120
	m.height = 40
	m.resize()

	view := m.View()
	if !strings.Contains(view, "?: help") {
		t.Fatal("expected compact help hint in footer")
	}
	if strings.Contains(view, "keys:") {
		t.Fatal("expected long key list to be removed from footer")
	}
}

func isQuitCmd(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}
