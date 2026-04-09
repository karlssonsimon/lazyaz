package appshell

import (
	"time"
)

// SetLoading marks the given pane as loading and records the start time.
// If a load is already in progress, the start time is preserved so the
// pane title spinner reflects the earliest load.
func (m *Model) SetLoading(pane int) {
	if !m.Loading {
		m.LoadingStartedAt = time.Now()
	}
	m.Loading = true
	m.LoadingPane = pane
}

// ClearLoading immediately clears loading state and resets the target pane.
func (m *Model) ClearLoading() {
	m.Loading = false
	m.LoadingPane = -1
}
