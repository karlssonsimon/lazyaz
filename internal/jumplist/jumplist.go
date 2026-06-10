// Package jumplist defines the small contract that lets the parent
// app maintain a vim-style ctrl+o / ctrl+i jump list across every
// child tab.
//
// Each tab Model implements (informally — the parent uses a type
// switch rather than an interface to avoid pointer/value-receiver
// awkwardness):
//
//	CurrentNav() NavSnapshot
//	ApplyNav(NavSnapshot) tea.Cmd
//
// CurrentNav returns the user's present navigable position (or nil
// if the tab isn't on a position worth recording — e.g., before a
// subscription is selected). ApplyNav restores a previously captured
// snapshot. Both work with the existing PendingNav fast-forward path
// so cache-warmed restorations are instant.
//
// Recording happens via RecordJumpMsg, which children emit and the
// parent intercepts via the standard cross-tab wrap bypass.
package jumplist

import (
	tea "charm.land/bubbletea/v2"
)

// NavSnapshot is an opaque, app-specific snapshot of "where the user
// is" inside one tab. The parent stores them as interface values; only
// each app's own ApplyNav knows how to restore them.
type NavSnapshot interface {
	// Description is shown in the optional status-bar jump indicator
	// and in any debug overlay listing the jump list.
	Description() string
}

// RecordJumpMsg asks the parent to append a snapshot to the jump
// list (truncating any forward history). Emitted by children right
// after they finish a navigation step that should be reachable via
// ctrl+o.
type RecordJumpMsg struct {
	Snap NavSnapshot
}

// AppendRecord batches cmd with a Cmd that emits a RecordJumpMsg for
// snap. It returns cmd unchanged (no record) when:
//   - applying: a programmatic restoration is in progress, so jump-list
//     walks don't re-record the entries they're traversing;
//   - pendingTarget: a pending navigation is still in flight — the parent
//     records the final destination directly, so intermediate hops would
//     just pollute ctrl+o history with phantom stops;
//   - snap is nil: there is no navigable position to record.
func AppendRecord(applying bool, pendingTarget bool, snap NavSnapshot, cmd tea.Cmd) tea.Cmd {
	if applying || pendingTarget || snap == nil {
		return cmd
	}
	rec := func() tea.Msg { return RecordJumpMsg{Snap: snap} }
	if cmd == nil {
		return rec
	}
	return tea.Batch(cmd, rec)
}
