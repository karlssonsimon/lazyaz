package appshell

import (
	"time"

	"azure-storage/internal/ui"

	tea "github.com/charmbracelet/bubbletea"
)

// LoadingHoldExpiredMsg is sent after the min-visible spinner hold elapses.
// Apps should clear their loading state and apply the status string when
// they see this message.
type LoadingHoldExpiredMsg struct {
	Status string
}

// SetLoading marks the given pane as loading and records the start time.
// If a load is already in progress, the start time is preserved so the
// min-visible spinner hold reflects the earliest load.
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

// FinishLoading completes a load, holding the spinner visible for at least
// ui.SpinnerMinVisible. If the hold has not yet elapsed, returns a delayed
// command that will eventually emit a LoadingHoldExpiredMsg with the supplied
// status; otherwise clears loading immediately and sets Status.
func (m *Model) FinishLoading(status string) tea.Cmd {
	remaining := ui.SpinnerMinVisible - time.Since(m.LoadingStartedAt)
	if remaining > 0 {
		return tea.Tick(remaining, func(t time.Time) tea.Msg {
			return LoadingHoldExpiredMsg{Status: status}
		})
	}
	m.ClearLoading()
	m.Status = status
	return nil
}
