package acapp

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/karlssonsimon/lazyaz/internal/azure/appconfig"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
)

func menuHas(m Model, id actionID) (string, bool) {
	for _, a := range m.buildActions() {
		if a.id == id {
			return a.label, true
		}
	}
	return "", false
}

// Create is offered once a store is open, and its form validates key,
// label and content type before anything is sent.
func TestCreateFormValidation(t *testing.T) {
	m := storeModel(t)
	if _, ok := menuHas(m, actionCreate); ok {
		t.Fatal("create offered before a store is open")
	}
	m, _ = m.handleEnter()
	if label, ok := menuHas(m, actionCreate); !ok || !strings.Contains(label, "cfg-htg-prod") {
		t.Fatalf("create entry = %q ok=%v", label, ok)
	}

	m, _ = m.openCreateForm()
	if !m.createForm.Active || m.inputMode() != ModeForm {
		t.Fatal("create form not open")
	}
	m.createForm.Fields[0].Value = "bad*key"
	m, cmd := m.handleCreateFormKey("enter")
	if cmd != nil || !m.createForm.Active || m.createForm.Fields[0].Error == "" {
		t.Fatalf("invalid key accepted: cmd=%v active=%v err=%q", cmd != nil, m.createForm.Active, m.createForm.Fields[0].Error)
	}

	// A duplicate of an existing key+label is refused client-side.
	m.createForm.Fields[0].Value = "Db/ConnectionTimeout"
	m.createForm.Fields[1].Value = "prod"
	m, cmd = m.handleCreateFormKey("enter")
	if cmd != nil || !strings.Contains(m.Status, "already exists") {
		t.Fatalf("duplicate not refused: status=%q", m.Status)
	}

	// Invalid JSON under a JSON content type is refused.
	m, _ = m.openCreateForm()
	m.createForm.Fields[0].Value = "New/Key"
	m.createForm.Fields[2].Value = "{nope"
	m.createForm.Fields[3].Value = "application/json"
	m, cmd = m.handleCreateFormKey("enter")
	if cmd != nil || !strings.Contains(m.Status, "not valid JSON") {
		t.Fatalf("bad JSON not refused: status=%q", m.Status)
	}

	// A good submission dispatches the create.
	m, _ = m.openCreateForm()
	m.createForm.Fields[0].Value = "New/Key"
	m.createForm.Fields[2].Value = `{"a":1}`
	m.createForm.Fields[3].Value = "application/json"
	m, cmd = m.handleCreateFormKey("enter")
	if cmd == nil || !m.Loading {
		t.Fatal("valid create did not dispatch")
	}
}

// A created setting appears in the list with the cursor on it; a
// conflict from the service is explained.
func TestSettingCreatedAddsRow(t *testing.T) {
	m := storeModel(t)
	m, _ = m.handleEnter()
	created := appconfig.Setting{Key: "New/Key", Label: "prod", Value: "1", ETag: `"e"`}
	m, _ = m.handleSettingCreated(settingCreatedMsg{store: testStore(), key: "New/Key", setting: created})
	if len(m.settings) != 6 || settingItemKey(m.settingsList.SelectedItem()) != appconfig.SettingKey(created) {
		t.Fatalf("row not added or not selected: n=%d selected=%q", len(m.settings), settingItemKey(m.settingsList.SelectedItem()))
	}
	m2, _ := m.handleSettingCreated(settingCreatedMsg{store: testStore(), key: "Dup", err: &azcore.ResponseError{StatusCode: 412}})
	if !strings.Contains(m2.Status, "already exists") {
		t.Fatalf("conflict status = %q", m2.Status)
	}
}

// Delete confirms with a destructive prompt naming the setting; locked
// settings are refused up front.
func TestDeleteConfirm(t *testing.T) {
	m := storeModel(t)
	m, _ = m.handleEnter()
	m.settingsList.Select(3) // the Key Vault reference
	if label, ok := menuHas(m, actionDelete); !ok || !strings.Contains(label, "Db/Password (prod)") {
		t.Fatalf("delete entry = %q ok=%v", label, ok)
	}
	m, _ = m.openDeleteConfirm()
	if !m.confirmModal.Active || !m.confirmModal.Destructive {
		t.Fatalf("confirm not open or not destructive: %+v", m.confirmModal)
	}
	if !strings.Contains(m.confirmModal.Title, "Key Vault reference") || !strings.Contains(m.confirmModal.Message, "secret itself stays") {
		t.Fatalf("reference delete not explained: title=%q msg=%q", m.confirmModal.Title, m.confirmModal.Message)
	}
	if m.confirmAction == nil {
		t.Fatal("delete not armed")
	}

	locked := make([]appconfig.Setting, len(m.settings))
	copy(locked, m.settings)
	locked[0].ReadOnly = true
	m.settings = locked
	m.settingsList.SetItems(settingsToItems(m.settings))
	m.settingsList.Select(0)
	m.confirmModal.Close()
	m, _ = m.openDeleteConfirm()
	if m.confirmModal.Active {
		t.Fatal("locked setting reached the delete confirmation")
	}
}

// A successful delete drops the row and closes the value pane if it
// showed that setting; errors are explained.
func TestSettingDeletedRemovesRow(t *testing.T) {
	m := storeModel(t)
	m, _ = m.handleEnter()
	m.settingsList.Select(2)
	m, _ = m.handleEnter() // value pane on Features/Checkout
	target := m.currentSetting
	m, _ = m.handleSettingDeleted(settingDeletedMsg{store: testStore(), setting: target})
	if len(m.settings) != 4 || m.viewingValue || m.focus != settingsPane {
		t.Fatalf("after delete: n=%d viewing=%v focus=%d", len(m.settings), m.viewingValue, m.focus)
	}
	for _, s := range m.settings {
		if s.Key == target.Key {
			t.Fatal("deleted setting still listed")
		}
	}
	m2, _ := m.handleSettingDeleted(settingDeletedMsg{store: testStore(), setting: target, err: &azcore.ResponseError{StatusCode: 412}})
	if !strings.Contains(m2.Status, "changed since you opened it") {
		t.Fatalf("conflict status = %q", m2.Status)
	}
	m3, _ := m.handleSettingDeleted(settingDeletedMsg{store: testStore(), setting: target, err: errors.New("boom")})
	if !strings.Contains(m3.Status, "Failed to delete") {
		t.Fatalf("generic status = %q", m3.Status)
	}
}

func TestValidateContentType(t *testing.T) {
	for _, good := range []string{"", "application/json", "text/plain; charset=utf-8", "application/vnd.microsoft.appconfig.ff+json;charset=utf-8"} {
		if msg := validateContentType(good); msg != "" {
			t.Errorf("validateContentType(%q) = %q", good, msg)
		}
	}
	for _, bad := range []string{"json", "application/", "/json", "text plain/x"} {
		if validateContentType(bad) == "" {
			t.Errorf("validateContentType(%q) accepted", bad)
		}
	}
}

// composeDone drives ctrl+e (or the given key) on the create form with
// a fake editor, writes newContent into the temp file, and reports the
// editor done.
func composeDone(t *testing.T, m Model, key, newContent string) (Model, ExternalEditRequestMsg) {
	t.Helper()
	m.EmbeddedMode = true
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "true")
	m, cmd := m.handleCreateFormKey(key)
	if cmd == nil {
		t.Fatalf("%s did not launch the editor", key)
	}
	req, ok := cmd().(ExternalEditRequestMsg)
	if !ok {
		t.Fatalf("%s produced %T, want an editor request", key, cmd())
	}
	if newContent != "" {
		if err := os.WriteFile(req.Path, []byte(newContent), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	m, _ = m.handleExternalEditDone(ExternalEditDoneMsg{Path: req.Path})
	return m, req
}

// ctrl+e on a JSON create opens a .json skeleton; the composed text
// returns into the form, fills the content type, and is what gets
// created on Enter.
func TestCreateComposeInEditor(t *testing.T) {
	m := storeModel(t)
	m, _ = m.handleEnter()
	m, _ = m.openCreateForm()
	m.createForm.Fields[createKeyField].Value = "Features/New"
	m.createForm.Fields[createContentTypeField].Value = "application/json"

	m, req := composeDone(t, m, "ctrl+e", "{\n  \"enabled\": true\n}\n")
	if !strings.HasSuffix(req.Path, ".json") {
		t.Fatalf("temp file should be .json for a JSON content type: %s", req.Path)
	}
	if !m.createForm.Active {
		t.Fatal("create form should still be open after composing")
	}
	if m.createValue != "{\n  \"enabled\": true\n}" {
		t.Fatalf("composed value = %q", m.createValue)
	}
	if !strings.Contains(m.createForm.Fields[createValueField].Help, "3 lines") {
		t.Fatalf("value help = %q", m.createForm.Fields[createValueField].Help)
	}

	m, cmd := m.handleCreateFormKey("enter")
	if cmd == nil || !m.Loading || !strings.Contains(m.Status, "Features/New") {
		t.Fatalf("Enter after composing should create: cmd=%v loading=%v status=%q", cmd != nil, m.Loading, m.Status)
	}
}

// Enter with a JSON content type and an empty value composes instead of
// creating an empty document; composing valid JSON fills an empty
// content type.
func TestCreateEnterOnEmptyJSONComposes(t *testing.T) {
	m := storeModel(t)
	m, _ = m.handleEnter()
	m, _ = m.openCreateForm()
	m.createForm.Fields[createKeyField].Value = "Features/New"
	m.createForm.Fields[createContentTypeField].Value = "application/json"
	m, _ = composeDone(t, m, "enter", "")
	if !m.createForm.Active || m.createValue != "" || !strings.Contains(m.Status, "No value composed") {
		t.Fatalf("empty skeleton should leave nothing composed: active=%v value=%q status=%q", m.createForm.Active, m.createValue, m.Status)
	}

	m, _ = m.openCreateForm()
	m.createForm.Fields[createKeyField].Value = "Plain/Key"
	m, _ = composeDone(t, m, "ctrl+e", `{"a": 1}`+"\n")
	if m.createForm.Fields[createContentTypeField].Value != "application/json" {
		t.Fatalf("valid JSON should fill the content type, got %q", m.createForm.Fields[createContentTypeField].Value)
	}
}

// Broken JSON is kept for fixing but flagged in the help and a warning.
func TestCreateComposeInvalidJSONFlagged(t *testing.T) {
	m := storeModel(t)
	m, _ = m.handleEnter()
	m, _ = m.openCreateForm()
	m.createForm.Fields[createKeyField].Value = "Features/New"
	m.createForm.Fields[createContentTypeField].Value = "application/json"
	m, _ = composeDone(t, m, "ctrl+e", "{broken\n")
	if m.createValue != "{broken" || !strings.Contains(m.createForm.Fields[createValueField].Help, "NOT VALID JSON") {
		t.Fatalf("value=%q help=%q", m.createValue, m.createForm.Fields[createValueField].Help)
	}
	m, cmd := m.handleCreateFormKey("enter")
	if cmd != nil || !strings.Contains(m.Status, "not valid JSON") {
		t.Fatalf("broken JSON should not be created: cmd=%v status=%q", cmd != nil, m.Status)
	}
}
