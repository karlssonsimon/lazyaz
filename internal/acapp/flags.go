package acapp

import (
	"fmt"
	"strings"

	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/azure/appconfig"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	tea "charm.land/bubbletea/v2"
)

// Feature flags: flipping is a conditional save of the value with
// "enabled" changed, through the same confirmation as an edit; creating
// is an add with the flag's key prefix, content type and the portal's
// document shape.

// openToggleFlagConfirm asks before flipping the action setting's flag.
func (m Model) openToggleFlagConfirm() (Model, tea.Cmd) {
	setting, ok := m.actionSetting()
	if !ok || !m.hasStore {
		return m, nil
	}
	ff, ok := appconfig.ParseFeatureFlag(setting)
	if !ok {
		m.Notify(appshell.LevelWarn, fmt.Sprintf("%s is not a feature flag with a readable value", setting.DisplayKey()))
		return m, nil
	}
	if setting.ReadOnly {
		m.Notify(appshell.LevelWarn, fmt.Sprintf("%s is locked (read-only); unlock it in the portal before changing it", ff.ID))
		return m, nil
	}
	newValue, err := appconfig.ToggleFeatureFlagValue(setting.Value, !ff.Enabled)
	if err != nil {
		m.Notify(appshell.LevelError, err.Error())
		return m, nil
	}
	crumbs := []string{m.currentStore.Name, ff.ID}
	if setting.Label != "" {
		crumbs = append(crumbs, setting.Label)
	}
	msg := fmt.Sprintf("Applications reading this store see the change immediately.\n\n%s → %s", onOff(ff.Enabled), onOff(!ff.Enabled))
	if ff.HasFilters {
		msg += "\n\nThis flag has filters; \"on\" means the filters decide, \"off\" is off for everyone."
	}
	// Turning a flag on is the change users notice, so it gets the
	// destructive styling; turning off is the safe direction.
	turningOn := !ff.Enabled
	m.confirmModal.OpenWithBreadcrumb("Turn flag "+onOff(turningOn), crumbs, msg, "confirm", "cancel", turningOn)
	store := m.currentStore
	m.confirmAction = func() tea.Cmd {
		return setValueCmd(m.service, store, setting, newValue)
	}
	return m, nil
}

func onOff(enabled bool) string {
	if enabled {
		return "on"
	}
	return "off"
}

// Create flag form field positions.
const (
	flagNameField = iota
	flagLabelField
	flagDescriptionField
	flagEnabledField
)

// openCreateFlagForm mounts the create form for a feature flag.
func (m Model) openCreateFlagForm() (Model, tea.Cmd) {
	if !m.hasStore {
		return m, nil
	}
	m.createFlagForm.OpenWithBreadcrumb("Create feature flag", []string{m.currentStore.Name}, []ui.FormField{
		{
			Label:       "Name",
			Placeholder: "new-checkout",
			Section:     "required",
			Validate:    appconfig.ValidateFeatureFlagName,
			Help:        `becomes the flag id; no "*", ",", "\", ":" or "/"`,
		},
		{
			Label:       "Label",
			Placeholder: "(no label)",
			Section:     "optional",
			Validate:    appconfig.ValidateLabel,
		},
		{
			Label:       "Description",
			Placeholder: "what the flag switches",
		},
		{
			Label:    "Enabled",
			Value:    "off",
			Cursor:   3,
			Help:     "on or off; new flags usually start off",
			Validate: validateOnOff,
		},
	})
	return m, nil
}

func validateOnOff(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "on", "off", "true", "false", "yes", "no", "":
		return ""
	}
	return "on or off"
}

func parseOnOff(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "on", "true", "yes":
		return true
	}
	return false
}

// handleCreateFlagFormKey routes keys to the flag form and dispatches
// the add on submit.
func (m Model) handleCreateFlagFormKey(key string) (Model, tea.Cmd) {
	res := m.createFlagForm.HandleKey(key)
	if res.Action != ui.FormActionSubmit || len(res.Values) < 4 {
		return m, nil
	}
	name := strings.TrimSpace(res.Values[flagNameField])
	label := strings.TrimSpace(res.Values[flagLabelField])
	key_ := appconfig.FeatureFlagKey(name)
	for _, s := range m.settings {
		if s.Key == key_ && s.Label == label {
			m.Notify(appshell.LevelWarn, fmt.Sprintf("flag %s already exists under that label", name))
			return m, nil
		}
	}
	value := appconfig.NewFeatureFlagValue(name, strings.TrimSpace(res.Values[flagDescriptionField]), parseOnOff(res.Values[flagEnabledField]))
	m.StartLoading(settingsPane, fmt.Sprintf("Creating flag %s in %s...", name, m.currentStore.Name))
	return m, tea.Batch(m.Spinner.Tick, addSettingCmd(m.service, m.currentStore, key_, label, value, appconfig.FeatureFlagContentType))
}
