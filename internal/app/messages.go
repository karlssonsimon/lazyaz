package app

import (
	"github.com/karlssonsimon/lazyaz/internal/blobapp"
	"github.com/karlssonsimon/lazyaz/internal/dashapp"
	"github.com/karlssonsimon/lazyaz/internal/jumplist"

	tea "charm.land/bubbletea/v2"
)

// tabMsg wraps a message with the tab ID it belongs to, so the parent
// can route it to the correct child even when multiple tabs of the
// same type produce identical message types.
type tabMsg struct {
	tabID int
	inner tea.Msg
}

// closeTabMsg is sent when an embedded child returns tea.Quit.
type closeTabMsg struct {
	tabID int
}

// tabPickerMsg carries the user's choice from the tab-type picker.
type tabPickerMsg struct {
	kind TabKind
}

// Command palette action messages.
type nextTabMsg struct{}
type prevTabMsg struct{}
type jumpTabMsg struct{ index int }
type openThemePickerMsg struct{}
type toggleHelpMsg struct{}
type toggleNotificationsMsg struct{}
type toggleActivityMsg struct{}

// activityAutoOpenMsg is dispatched when an upload starts to pop the
// activity overlay into detail view for that activity. Ignored if the overlay
// is already open.
type activityAutoOpenMsg struct {
	ActivityID string
}

// activityEventMsg is emitted by the registry-observer goroutine each
// time the registry fires an Event. Receiving it is the signal to
// re-render. The msg carries a next cmd that re-enters the observer
// loop, mirroring the broker recv pattern.
type activityEventMsg struct {
	next tea.Cmd
}

// toastTickMsg drives the periodic re-render that lets toasts expire
// off-screen. It self-extinguishes once no toasts are active. See
// the toastTickActive flag on Model.
type toastTickMsg struct{}

// wrapCmd wraps a tea.Cmd so its resulting message is tagged with tabID.
// It recursively handles tea.BatchMsg to wrap each sub-command.
func wrapCmd(id int, cmd tea.Cmd) tea.Cmd {
	if cmd == nil {
		return nil
	}
	return func() tea.Msg {
		msg := cmd()
		return wrapMsg(id, msg)
	}
}

// wrapMsg tags a message with the given tab ID.
// tea.BatchMsg is handled by wrapping each sub-command.
func wrapMsg(id int, msg tea.Msg) tea.Msg {
	if msg == nil {
		return nil
	}
	switch msg := msg.(type) {
	case tea.QuitMsg:
		return closeTabMsg{tabID: id}
	case tea.BatchMsg:
		wrapped := make(tea.BatchMsg, len(msg))
		for i, cmd := range msg {
			wrapped[i] = wrapCmd(id, cmd)
		}
		return wrapped
	// Cross-tab messages: bypass the tabMsg wrap so the parent
	// handles them directly instead of routing them back to the
	// emitting tab.
	case dashapp.OpenSBNamespaceMsg, dashapp.OpenSBEntityMsg,
		dashapp.OpenBlobAccountMsg, dashapp.OpenBlobContainerMsg,
		jumplist.RecordJumpMsg,
		blobapp.ActivityAutoOpenRequestMsg:
		return msg
	default:
		return tabMsg{tabID: id, inner: msg}
	}
}
