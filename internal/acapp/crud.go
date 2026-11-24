package acapp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/azure/appconfig"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	tea "charm.land/bubbletea/v2"
)

// Creating and deleting key-values. Create is a form (key, label,
// value, content type) with validation and no confirmation, like Key
// Vault's create secret: a new key cannot change what running
// applications read. Delete confirms, names the key and store, and is
// conditional on the ETag so a concurrently changed setting is not
// removed blind.

// Create form field positions.
const (
	createKeyField = iota
	createLabelField
	createValueField
	createContentTypeField
)

// composeHint is the Value field's help while nothing is composed.
const composeHint = "single line here, or ctrl+e to compose in $EDITOR — use that for JSON"

// openCreateForm mounts the create form for the current store.
func (m Model) openCreateForm() (Model, tea.Cmd) {
	if !m.hasStore {
		return m, nil
	}
	m.createValue = ""
	m.createForm.OpenWithBreadcrumb("Create key-value", []string{m.currentStore.Name}, []ui.FormField{
		{
			Label:       "Key",
			Placeholder: "App/Feature/Setting",
			Section:     "required",
			Validate:    appconfig.ValidateKey,
			Help:        "any text except * , and \\",
		},
		{
			Label:       "Label",
			Placeholder: "(no label)",
			Section:     "optional",
			Validate:    appconfig.ValidateLabel,
			Help:        "leave empty for the unlabelled value",
		},
		{
			Label:       "Value",
			Placeholder: "",
			Hint:        "ctrl+e editor",
			Help:        composeHint,
		},
		{
			Label:       "Content type",
			Placeholder: "application/json",
			Help:        "optional; application/json makes the value a JSON document",
			Validate:    validateContentType,
		},
	})
	return m, nil
}

// validateContentType accepts a media type, with optional parameters
// after a semicolon ("text/plain; charset=utf-8").
func validateContentType(ct string) string {
	ct = strings.TrimSpace(ct)
	if ct == "" {
		return ""
	}
	mediaType := strings.TrimSpace(strings.SplitN(ct, ";", 2)[0])
	parts := strings.Split(mediaType, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" || strings.ContainsAny(mediaType, " \t") {
		return "must look like type/subtype, e.g. application/json"
	}
	return ""
}

// isJSONContentType reports a content type that marks the value as JSON.
func isJSONContentType(ct string) bool {
	return strings.Contains(strings.ToLower(ct), "json")
}

// handleCreateFormKey routes keys to the create form. ctrl+e composes
// the value in $EDITOR; Enter with a JSON content type and no value
// does the same rather than creating an empty document. Submit
// validates a JSON value and dispatches the add.
func (m Model) handleCreateFormKey(key string) (Model, tea.Cmd) {
	if key == "ctrl+e" {
		return m.composeCreateValue()
	}
	if key == "enter" && m.createValue == "" &&
		strings.TrimSpace(m.createForm.Fields[createValueField].Value) == "" &&
		isJSONContentType(m.createForm.Fields[createContentTypeField].Value) {
		return m.composeCreateValue()
	}
	res := m.createForm.HandleKey(key)
	if res.Action != ui.FormActionSubmit || len(res.Values) < 4 {
		return m, nil
	}
	k, label, value, ct := strings.TrimSpace(res.Values[0]), strings.TrimSpace(res.Values[1]), res.Values[2], strings.TrimSpace(res.Values[3])
	// A value typed in the field wins; otherwise the composed one.
	if strings.TrimSpace(value) == "" && m.createValue != "" {
		value = m.createValue
	}
	m.createValue = ""
	if isJSONContentType(ct) && strings.TrimSpace(value) != "" {
		var v any
		if err := json.Unmarshal([]byte(value), &v); err != nil {
			m.Notify(appshell.LevelError, "Value is not valid JSON for a JSON content type; nothing created: "+err.Error())
			return m, nil
		}
	}
	for _, s := range m.settings {
		if s.Key == k && s.Label == label {
			m.Notify(appshell.LevelWarn, fmt.Sprintf("%s already exists under that label; edit it instead", k))
			return m, nil
		}
	}
	m.StartLoading(settingsPane, fmt.Sprintf("Creating %s in %s...", k, m.currentStore.Name))
	return m, tea.Batch(m.Spinner.Tick, addSettingCmd(m.service, m.currentStore, k, label, value, ct))
}

// composeCreateValue opens $EDITOR on the create form's value: what is
// typed in the field, else what was composed before, else a skeleton
// when the content type is JSON. The form stays open underneath and
// receives the text when the editor exits.
func (m Model) composeCreateValue() (Model, tea.Cmd) {
	if !m.createForm.Active {
		return m, nil
	}
	ct := m.createForm.Fields[createContentTypeField].Value
	typed := m.createForm.Fields[createValueField].Value
	text := typed
	if text == "" {
		text = m.createValue
	}
	wantJSON := isJSONContentType(ct) || json.Valid([]byte(strings.TrimSpace(text))) && strings.TrimSpace(text) != ""
	if text == "" && wantJSON {
		text = "{\n  \n}\n"
	} else if wantJSON && json.Valid([]byte(text)) {
		if pretty, isJSON := editorText(appconfig.Setting{Value: text}); isJSON {
			text = pretty
		}
	}
	hint := strings.TrimSpace(m.createForm.Fields[createKeyField].Value)
	if hint == "" {
		hint = "new"
	}
	return m.launchEditor(externalEdit{original: text, wantJSON: wantJSON, forCreate: true}, hint)
}

// handleComposedCreateValue takes the editor's text back into the create
// form. The field itself stays empty and its help describes what was
// composed; typing into the field replaces the composed value.
func (m Model) handleComposedCreateValue(ed *externalEdit, edited string, exitErr, readErr error) (Model, tea.Cmd) {
	if !m.createForm.Active {
		return m, nil
	}
	if exitErr != nil {
		m.Notify(appshell.LevelError, "Editor exited with an error; value not taken: "+exitErr.Error())
		return m, nil
	}
	if readErr != nil {
		m.Notify(appshell.LevelError, "Could not read the composed value: "+readErr.Error())
		return m, nil
	}
	edited = strings.TrimSuffix(edited, "\n")
	if strings.TrimSpace(edited) == "" || edited == strings.TrimSuffix(ed.original, "\n") && ed.original == "{\n  \n}\n" {
		m.Notify(appshell.LevelInfo, "No value composed")
		return m, nil
	}
	m.createValue = edited
	field := &m.createForm.Fields[createValueField]
	field.Value = ""
	field.Cursor = 0
	lines := strings.Count(edited, "\n") + 1
	field.Help = fmt.Sprintf("composed in editor: %d lines, %d chars · ctrl+e to edit it again · typing here replaces it", lines, len([]rune(edited)))
	// Recognise JSON by what came back, not only by the content type:
	// a value composed as JSON fills an empty content type, and a
	// JSON content type with text that does not parse is flagged.
	jsonErr := validateJSON(edited)
	switch {
	case jsonErr == nil:
		if ct := &m.createForm.Fields[createContentTypeField]; strings.TrimSpace(ct.Value) == "" {
			ct.Value = "application/json"
			ct.Cursor = len([]rune(ct.Value))
		}
	case ed.wantJSON:
		field.Help += " · NOT VALID JSON: " + ui.TrimToWidth(jsonErr.Error(), 40)
		m.Notify(appshell.LevelWarn, "Composed value is not valid JSON; fix it with ctrl+e before creating: "+jsonErr.Error())
	}
	m.createForm.Focus = createContentTypeField
	return m, nil
}

type settingCreatedMsg struct {
	store   appconfig.Store
	key     string
	setting appconfig.Setting
	err     error
}

func addSettingCmd(svc *appconfig.Service, store appconfig.Store, key, label, value, contentType string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		created, err := svc.AddSetting(ctx, store, key, label, value, contentType)
		return settingCreatedMsg{store: store, key: key, setting: created, err: err}
	}
}

func (m Model) handleSettingCreated(msg settingCreatedMsg) (Model, tea.Cmd) {
	m.ClearLoading()
	if msg.err != nil {
		if appconfig.IsConflict(msg.err) {
			m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelError, fmt.Sprintf("%s already exists in %s; nothing created", msg.key, msg.store.Name))
		} else {
			m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelError, fmt.Sprintf("Failed to create %s: %s", msg.key, msg.err.Error()))
		}
		return m, nil
	}
	m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelSuccess, fmt.Sprintf("Created %s in %s", msg.key, msg.store.Name))
	if !m.hasStore || m.currentStore.Name != msg.store.Name {
		return m, nil
	}
	// Show the row at once and park the cursor on it; the refresh then
	// brings the cache into line.
	updated := make([]appconfig.Setting, 0, len(m.settings)+1)
	updated = append(updated, m.settings...)
	updated = append(updated, msg.setting)
	m.settings = updated
	ui.SetItemsPreserveKey(&m.settingsList, settingsToItems(m.settings), settingItemKey)
	ui.SelectByKey(&m.settingsList, appconfig.SettingKey(msg.setting), settingItemKey)
	m.StartLoading(settingsPane, fmt.Sprintf("Refreshing settings in %s", m.currentStore.Name))
	return m, tea.Batch(m.Spinner.Tick, fetchSettingsCmd(m.service, m.cache.settings, m.currentStore, m.settings))
}

// openDeleteConfirm asks before deleting the action setting.
func (m Model) openDeleteConfirm() (Model, tea.Cmd) {
	setting, ok := m.actionSetting()
	if !ok || !m.hasStore {
		return m, nil
	}
	if setting.ReadOnly {
		m.Notify(appshell.LevelWarn, fmt.Sprintf("%s is locked (read-only); unlock it in the portal before deleting", setting.DisplayKey()))
		return m, nil
	}
	crumbs := []string{m.currentStore.Name, setting.DisplayKey()}
	if setting.Label != "" {
		crumbs = append(crumbs, setting.Label)
	}
	what := "key-value"
	if setting.IsFeatureFlag() {
		what = "feature flag"
	} else if setting.IsKeyVaultReference() {
		what = "Key Vault reference (the secret itself stays in the vault)"
	}
	m.confirmModal.OpenWithBreadcrumb(
		"Delete "+what,
		crumbs,
		fmt.Sprintf("Applications reading %s stop seeing this %s immediately. There is no undo; the value is\n\n%s",
			m.currentStore.Name, what, ui.TrimToWidth(ui.EmptyToDash(setting.Value), 70)),
		"delete", "cancel", true)
	store := m.currentStore
	m.confirmAction = func() tea.Cmd {
		return deleteSettingCmd(m.service, store, setting)
	}
	return m, nil
}

type settingDeletedMsg struct {
	store   appconfig.Store
	setting appconfig.Setting
	err     error
}

func deleteSettingCmd(svc *appconfig.Service, store appconfig.Store, setting appconfig.Setting) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		err := svc.DeleteSetting(ctx, store, setting)
		return settingDeletedMsg{store: store, setting: setting, err: err}
	}
}

func (m Model) handleSettingDeleted(msg settingDeletedMsg) (Model, tea.Cmd) {
	m.ClearLoading()
	key := msg.setting.DisplayKey()
	if msg.err != nil {
		switch {
		case appconfig.IsConflict(msg.err):
			m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelError, fmt.Sprintf("%s changed since you opened it; refresh and look again before deleting", key))
		case appconfig.IsLocked(msg.err):
			m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelError, fmt.Sprintf("%s is locked (read-only); unlock it before deleting", key))
		default:
			m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelError, fmt.Sprintf("Failed to delete %s: %s", key, msg.err.Error()))
		}
		return m, nil
	}
	m.ResolveSpinner(m.LoadingSpinnerID, appshell.LevelSuccess, fmt.Sprintf("Deleted %s from %s", key, msg.store.Name))
	if !m.hasStore || m.currentStore.Name != msg.store.Name {
		return m, nil
	}
	// Drop the row now; close the value pane if it showed the deleted
	// setting; then refresh so the cache agrees.
	gone := appconfig.SettingKey(msg.setting)
	remaining := make([]appconfig.Setting, 0, len(m.settings))
	for _, s := range m.settings {
		if appconfig.SettingKey(s) != gone {
			remaining = append(remaining, s)
		}
	}
	m.settings = remaining
	ui.SetItemsPreserveKey(&m.settingsList, settingsToItems(m.settings), settingItemKey)
	if m.viewingValue && appconfig.SettingKey(m.currentSetting) == gone {
		m.closeValue()
		if m.focus == valuePane {
			m.transitionTo(settingsPane)
		}
	}
	m.StartLoading(settingsPane, fmt.Sprintf("Refreshing settings in %s", m.currentStore.Name))
	return m, tea.Batch(m.Spinner.Tick, fetchSettingsCmd(m.service, m.cache.settings, m.currentStore, m.settings))
}
