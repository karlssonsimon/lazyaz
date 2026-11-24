package acapp

import (
	"errors"
	"strings"
	"testing"

	"github.com/karlssonsimon/lazyaz/internal/azure/appconfig"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/charmbracelet/x/ansi"
)

// The action menu on a setting offers Edit; choosing it opens the form
// prefilled with the current value.
func TestEditFormPrefills(t *testing.T) {
	m := storeModel(t)
	m, _ = m.handleEnter()
	m.settingsList.Select(1) // Db/ConnectionTimeout @ prod = 60

	var hasEdit bool
	for _, a := range m.buildActions() {
		if a.id == actionEditValue && strings.Contains(a.label, "Db/ConnectionTimeout (prod)") {
			hasEdit = true
		}
	}
	if !hasEdit {
		t.Fatal("action menu lacks the edit entry for the selected setting")
	}

	m, _ = m.openEditValueForm()
	if !m.editForm.Active || m.inputMode() != ModeForm {
		t.Fatalf("form not open: active=%v mode=%v", m.editForm.Active, m.inputMode())
	}
	if got := m.editForm.Fields[0].Value; got != "60" {
		t.Fatalf("form prefilled with %q, want 60", got)
	}
	if !m.IsTextInputActive() {
		t.Fatal("form must claim the keyboard")
	}
}

// Submitting an unchanged value closes the form without a confirmation;
// a changed value opens the confirmation showing old and new.
func TestEditSubmitOpensConfirm(t *testing.T) {
	m := storeModel(t)
	m, _ = m.handleEnter()
	m.settingsList.Select(1)
	m, _ = m.openEditValueForm()

	m, _ = m.handleFormKey("enter")
	if m.confirmModal.Active {
		t.Fatal("unchanged value should not ask for confirmation")
	}

	m, _ = m.openEditValueForm()
	m.editForm.Fields[0].Value = "90"
	m, _ = m.handleFormKey("enter")
	if !m.confirmModal.Active || m.inputMode() != ModeConfirm {
		t.Fatal("changed value should open the confirmation")
	}
	view := ansi.Strip(m.View().Content)
	for _, want := range []string{"old: 60", "new: 90", "cfg-htg-prod", "immediately"} {
		if !strings.Contains(view, want) {
			t.Errorf("confirmation missing %q", want)
		}
	}
	if m.confirmAction == nil {
		t.Fatal("confirm action not armed")
	}
}

// A locked setting is refused before any form opens.
func TestEditRefusesLockedSetting(t *testing.T) {
	m := storeModel(t)
	m, _ = m.handleEnter()
	// Copy before mutating: the cache's fetch worker reads the slice it
	// handed out.
	locked := make([]appconfig.Setting, len(m.settings))
	copy(locked, m.settings)
	locked[0].ReadOnly = true
	m.settings = locked
	m.settingsList.SetItems(settingsToItems(m.settings))
	m.settingsList.Select(0)
	m, _ = m.openEditValueForm()
	if m.editForm.Active {
		t.Fatal("form opened for a read-only setting")
	}
}

// A successful save swaps the row and the open value at once; a
// conflict is reported in plain words.
func TestSettingSavedUpdatesRowAndValue(t *testing.T) {
	m := storeModel(t)
	m, _ = m.handleEnter()
	m.settingsList.Select(1)
	m, _ = m.handleEnter() // value pane on Db/ConnectionTimeout@prod
	before := m.currentSetting
	saved := before
	saved.Value = "90"
	saved.ETag = `"new"`

	m, _ = m.handleSettingSaved(settingSavedMsg{store: testStore(), before: before, setting: saved})
	if m.currentSetting.Value != "90" || m.settings[1].Value != "90" {
		t.Fatalf("save not applied: current=%q row=%q", m.currentSetting.Value, m.settings[1].Value)
	}
	if !strings.Contains(ansi.Strip(m.View().Content), "90") {
		t.Fatal("value pane does not show the saved value")
	}

	m2, _ := m.handleSettingSaved(settingSavedMsg{store: testStore(), before: before, err: &azcore.ResponseError{StatusCode: 412}})
	if !strings.Contains(m2.Status, "changed since you opened it") {
		t.Fatalf("conflict status = %q", m2.Status)
	}
	m3, _ := m.handleSettingSaved(settingSavedMsg{store: testStore(), before: before, err: errors.New("boom")})
	if !strings.Contains(m3.Status, "Failed to save") {
		t.Fatalf("generic error status = %q", m3.Status)
	}
}

// The value pane's action set includes edit as well.
func TestActionSettingFromValuePane(t *testing.T) {
	m := storeModel(t)
	m, _ = m.handleEnter()
	m.settingsList.Select(2)
	m, _ = m.handleEnter()
	s, ok := m.actionSetting()
	if !ok || s.Key != "Features/Checkout" {
		t.Fatalf("actionSetting from value pane = %+v ok=%v", s, ok)
	}
	m.focus = storesPane
	if _, ok := m.actionSetting(); ok {
		t.Fatal("stores pane must not offer a setting to edit")
	}
}

var _ = ui.FormActionSubmit
