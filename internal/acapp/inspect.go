package acapp

import (
	"sort"
	"strings"

	"github.com/karlssonsimon/lazyaz/internal/azure/appconfig"
	"github.com/karlssonsimon/lazyaz/internal/ui"
)

func (m Model) inspectFor(pane int) (string, []ui.InspectField) {
	switch pane {
	case storesPane:
		item, ok := m.storesList.SelectedItem().(storeItem)
		if !ok {
			return "Store", nil
		}
		s := item.store
		return "Store", []ui.InspectField{
			{Label: "Name", Value: s.Name},
			{Label: "Subscription", Value: s.SubscriptionID},
			{Label: "Resource Group", Value: s.ResourceGroup},
			{Label: "Endpoint", Value: s.Endpoint},
			{Label: "Location", Value: ui.EmptyToDash(s.Location)},
		}
	case settingsPane, valuePane:
		var s appconfig.Setting
		if pane == valuePane && m.viewingValue {
			s = m.currentSetting
		} else if item, ok := m.settingsList.SelectedItem().(settingItem); ok {
			s = item.setting
		} else {
			return "Setting", nil
		}
		return "Setting", settingInspect(s)
	}
	return "", nil
}

func settingInspect(s appconfig.Setting) []ui.InspectField {
	kind := "key-value"
	switch {
	case s.IsKeyVaultReference():
		kind = "Key Vault reference"
	case s.IsFeatureFlag():
		kind = "feature flag"
	}
	fields := []ui.InspectField{
		{Label: "Key", Value: s.Key},
		{Label: "Label", Value: ui.EmptyToDash(s.Label)},
		{Label: "Kind", Value: kind},
		{Label: "Content Type", Value: ui.EmptyToDash(s.ContentType)},
		{Label: "Last Modified", Value: ui.FormatTime(s.LastModified)},
		{Label: "Read Only", Value: yesNo(s.ReadOnly)},
		{Label: "Tags", Value: tagsLine(s.Tags)},
	}
	if ref, ok := s.KeyVaultReference(); ok {
		fields = append(fields,
			ui.InspectField{Label: "Vault", Value: ref.VaultName},
			ui.InspectField{Label: "Secret", Value: ref.SecretName},
		)
	}
	return fields
}

func tagsLine(tags map[string]string) string {
	if len(tags) == 0 {
		return "-"
	}
	keys := make([]string, 0, len(tags))
	for k := range tags {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k + "=" + tags[k]
	}
	return strings.Join(parts, " · ")
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func (m Model) inspectFooterHeight(pane int) int {
	if !m.inspectPanes[pane] {
		return 0
	}
	_, fields := m.inspectFor(pane)
	return ui.InspectStripHeight(fields)
}

func (m Model) inspectFooter(pane, contentWidth int) string {
	if !m.inspectPanes[pane] {
		return ""
	}
	title, fields := m.inspectFor(pane)
	return ui.RenderInspectStrip(title, fields, m.Styles, contentWidth)
}

// toggleInspect flips the strip for the focused list pane. From the
// value pane it toggles the settings strip, which stays visible beside
// the value.
func (m *Model) toggleInspect() {
	pane := m.focus
	if pane == valuePane {
		pane = settingsPane
	}
	m.inspectPanes[pane] = !m.inspectPanes[pane]
	m.resize()
}
