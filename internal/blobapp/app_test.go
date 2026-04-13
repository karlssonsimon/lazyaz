package blobapp

import (
	"strings"
	"testing"

	"github.com/karlssonsimon/lazyaz/internal/ui"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
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

func TestTypingQWhileSearchActiveDoesNotQuit(t *testing.T) {
	m := NewModel(nil, testConfig, nil)
	m.focus = blobsPane
	m.hasContainer = true
	m.filter.inputOpen = true

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if _, ok := updated.(Model); !ok {
		t.Fatalf("expected updated model type %T, got %T", Model{}, updated)
	}

	if isQuitCmd(cmd) {
		t.Fatal("expected typing q during active search not to quit")
	}
}

func TestHelpToggleOpensAndCloses(t *testing.T) {
	m := NewModel(nil, testConfig, nil)
	m.SubOverlay.Close() // close auto-opened picker so keys reach help handler

	updated, _ := m.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	model := updated.(Model)
	if !model.HelpOverlay.Active {
		t.Fatal("expected ? to open help overlay")
	}

	updated, _ = model.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	model = updated.(Model)
	if model.HelpOverlay.Active {
		t.Fatal("expected ? to close help overlay")
	}
}

func TestViewRenders(t *testing.T) {
	m := NewModel(nil, testConfig, nil)
	m.Width = 120
	m.Height = 40
	m.resize()

	view := m.View()
	if view.Content == "" {
		t.Fatal("expected non-empty view")
	}
}

func isQuitCmd(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

// TestPreviewPaneOverflowDoesNotEatStatusBar verifies that even with a
// pathologically wide and long preview (a long blob name that wraps the
// title, very wide content lines that would force lipgloss to wrap, and
// far more content rows than fit in the viewport), the preview pane
// stays inside its frame and the status bar stays visible. Regression
// test for the v1→v2 lipgloss MaxHeight-clips-the-border bug.
func TestPreviewPaneOverflowDoesNotEatStatusBar(t *testing.T) {
	m := NewModel(nil, testConfig, nil)
	m.SubOverlay.Close()
	m.Width = 200
	m.Height = 60
	m.hasAccount = true
	m.currentAccount.Name = "test-account"
	m.hasContainer = true
	m.containerName = "test-container"
	m.focus = blobsPane
	m.preview.open = true
	m.preview.blobName = "verylongblobnamethatwillforcetitletowrapacrossseverallinesinthepreviewtitle.xml"
	m.preview.blobSize = 1024
	m.preview.contentType = "text/plain"
	wide := strings.Repeat("X", 500)
	m.preview.viewport.SetContent(strings.Repeat(wide+"\n", 200))
	m.preview.rendered = strings.Repeat(wide+"\n", 200)
	m.resize()

	view := m.View()
	if got := strings.Count(view.Content, "\n") + 1; got != m.Height {
		t.Errorf("rendered view: %d lines, want %d", got, m.Height)
	}
	stripped := ansi.Strip(view.Content)
	if !strings.Contains(stripped, "Account:") {
		t.Errorf("status bar 'Account:' missing from view (preview likely ate it)")
	}
	if !strings.Contains(stripped, "test-account") {
		t.Errorf("status bar account value missing from view")
	}
	// Sanity: the bottom-border corner character should appear in every
	// pane's bottom row. If the preview pane's bottom border is missing
	// (because we let lipgloss MaxHeight clip from below), this fails.
	lines := strings.Split(stripped, "\n")
	// Find the last line containing border corner characters.
	var lastBorderLine string
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.Contains(lines[i], "╯") {
			lastBorderLine = lines[i]
			break
		}
	}
	if lastBorderLine == "" {
		t.Fatal("no pane bottom-border row found")
	}
	// Miller layout: containers (parent) + blobs (focused) + preview = 3 panes.
	if got := strings.Count(lastBorderLine, "╯"); got < 2 {
		t.Errorf("expected at least 2 bottom-border corners on the pane row, got %d in %q",
			got, lastBorderLine)
	}
}
