package dashapp

import tea "charm.land/bubbletea/v2"

// clearUsageMsg asks the dashboard to drop all usage rows for the
// current subscription matching one of the listed resource types.
// Empty types means "all types".
type clearUsageMsg struct {
	types []string
}

func clearUsageCmd(types ...string) tea.Cmd {
	return func() tea.Msg { return clearUsageMsg{types: types} }
}

// clearUsageAction is the per-widget "Clear usage stats" entry. Each
// usage widget passes its own resource_type list so clearing one
// widget's stats doesn't wipe the other.
func clearUsageAction(types ...string) Action {
	return Action{
		Label: "Clear usage stats",
		Key:   "x",
		Cmd:   clearUsageCmd(types...),
	}
}
