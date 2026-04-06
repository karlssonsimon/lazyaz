package appshell

// Notify publishes a notification to the global log AND mirrors the
// message into the embedded status bar fields so the existing UI keeps
// working unchanged. Errors also set LastErr so the status bar paints
// them red.
//
// This is the opt-in helper for code that wants its message to show as
// a top-right toast and appear in the history overlay. Existing direct
// `m.Status = "..."` assignments are not auto-promoted — they remain
// silent status updates and only get logged when migrated.
func (m *Model) Notify(level NotificationLevel, message string) {
	if m == nil {
		return
	}
	m.Notifier.Push(level, message)
	m.Status = message
	if level == LevelError {
		m.LastErr = message
	} else {
		m.LastErr = ""
	}
}
