package kvapp

import (
	"strings"
	"testing"

	"azure-storage/internal/ui"

	tea "github.com/charmbracelet/bubbletea"
)

var testConfig = ui.Config{
	ThemeName: "fallback",
	Schemes:   []ui.Scheme{ui.FallbackScheme()},
}

func TestPaneName(t *testing.T) {
	tests := []struct {
		pane int
		want string
	}{
		{vaultsPane, "vaults"},
		{secretsPane, "secrets"},
		{versionsPane, "versions"},
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

func TestTypingQWhileFilteringDoesNotQuit(t *testing.T) {
	m := NewModel(nil, testConfig, nil)
	m.focus = vaultsPane
	m.vaultsList.SetFilterState(1) // list.Filtering

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if _, ok := updated.(Model); !ok {
		t.Fatalf("expected updated model type %T, got %T", Model{}, updated)
	}

	if isQuitCmd(cmd) {
		t.Fatal("expected typing q in active filter not to quit")
	}
}

func TestHelpToggleOpensAndCloses(t *testing.T) {
	m := NewModel(nil, testConfig, nil)

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

func TestViewShowsStatusBar(t *testing.T) {
	m := NewModel(nil, testConfig, nil)
	m.width = 120
	m.height = 40
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
