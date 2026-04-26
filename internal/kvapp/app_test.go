package kvapp

import (
	"testing"

	"github.com/karlssonsimon/lazyaz/internal/azure"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	tea "charm.land/bubbletea/v2"
)

var testConfig = ui.Config{
	ThemeName: "fallback",
	Schemes:   []ui.Scheme{ui.FallbackScheme()},
}

func TestSetSubscriptionAllowsNilServiceWithTenant(t *testing.T) {
	m := NewModel(nil, testConfig, nil)
	if m.service == nil {
		t.Fatalf("NewModel(nil) left service nil")
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("SetSubscription panicked with nil service: %v", r)
		}
	}()
	m.SetSubscription(azure.Subscription{ID: "sub", TenantID: "tenant"})
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

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if _, ok := updated.(Model); !ok {
		t.Fatalf("expected updated model type %T, got %T", Model{}, updated)
	}

	if isQuitCmd(cmd) {
		t.Fatal("expected typing q in active filter not to quit")
	}
}

func TestHelpToggleOpensAndCloses(t *testing.T) {
	m := NewModel(nil, testConfig, nil)
	m.SubOverlay.Close()

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

func TestViewShowsStatusBar(t *testing.T) {
	m := NewModel(nil, testConfig, nil)
	m.Width = 120
	m.Height = 40
	m.resize()

	view := m.View()
	if view.Content == "" {
		t.Fatal("expected view to render content")
	}
}

func isQuitCmd(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

// TestCurrentNavCapturesRootPane ensures the vaults-list view is a
// recordable jump target (ctrl+o must be able to walk back here after
// the user drills into a vault).
func TestCurrentNavCapturesRootPane(t *testing.T) {
	m := NewModel(nil, testConfig, nil)
	m.SetSubscription(azure.Subscription{ID: "sub-1", Name: "Test"})
	m.focus = vaultsPane

	snap := m.CurrentNav()
	if snap == nil {
		t.Fatal("expected non-nil snapshot on vaults pane with subscription set")
	}
	kv, ok := snap.(kvNavSnapshot)
	if !ok {
		t.Fatalf("expected kvNavSnapshot, got %T", snap)
	}
	if kv.vaultName != "" {
		t.Errorf("expected empty vaultName, got %q", kv.vaultName)
	}
	if kv.focusedPane != vaultsPane {
		t.Errorf("expected focusedPane=vaultsPane, got %d", kv.focusedPane)
	}
}

// TestValidateSecretName covers Azure's naming rules so client-side
// feedback matches what SetSecret would return.
func TestValidateSecretName(t *testing.T) {
	tests := []struct {
		in      string
		wantErr bool
	}{
		{"", true},
		{"ok-name", false},
		{"OKName123", false},
		{"has_underscore", true},
		{"has space", true},
		{"has.dot", true},
	}
	for _, tc := range tests {
		got := validateSecretName(tc.in)
		if tc.wantErr && got == "" {
			t.Errorf("validateSecretName(%q) = empty, want error", tc.in)
		}
		if !tc.wantErr && got != "" {
			t.Errorf("validateSecretName(%q) = %q, want empty", tc.in, got)
		}
	}
}

// TestOpenCreateSecretFormGatesOnVault ensures opening the form
// without a selected vault is a no-op — executeAction relies on this.
func TestOpenCreateSecretFormGatesOnVault(t *testing.T) {
	m := NewModel(nil, testConfig, nil)
	m.hasVault = false
	m.openCreateSecretForm()
	if m.createSecret.Active {
		t.Fatal("expected form to stay inactive with no vault selected")
	}

	m.hasVault = true
	m.currentVault.Name = "vault-a"
	m.openCreateSecretForm()
	if !m.createSecret.Active {
		t.Fatal("expected form to open with vault selected")
	}
	if len(m.createSecret.Fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(m.createSecret.Fields))
	}
}

// TestApplyNavEmptyVaultRestoresFocus ensures the root-pane snapshot
// restores focus without dispatching a PendingNav drill-in.
func TestApplyNavEmptyVaultRestoresFocus(t *testing.T) {
	m := NewModel(nil, testConfig, nil)
	m.SetSubscription(azure.Subscription{ID: "sub-1", Name: "Test"})
	m.focus = secretsPane
	m.hasVault = true

	cmd := m.ApplyNav(kvNavSnapshot{focusedPane: vaultsPane})

	if cmd != nil {
		t.Errorf("expected no cmd for root-pane restore, got %v", cmd)
	}
	if m.focus != vaultsPane {
		t.Errorf("expected focus=vaultsPane after restore, got %d", m.focus)
	}
	if m.pendingNav.hasTarget() {
		t.Error("expected no pending nav target after root-pane restore")
	}
}
