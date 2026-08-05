package sbapp

import (
	"fmt"

	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/ui"
	"github.com/karlssonsimon/lazyaz/internal/vim"

	tea "charm.land/bubbletea/v2"
)

// The message body's format view. Unlike the blob preview there are no
// windows to swap: msgBody() returns the formatted override while
// active, and every view path — buffer, search, yank, highlight — reads
// through it. No size limit either; the body is already in memory.

// msgFormatStash is the raw view's position, saved across the toggle so
// = again restores where the reader was.
type msgFormatStash struct {
	cur     vim.Cursor
	yoffset int
	xoffset int
}

// msgBody is what the message view shows and operates on: the formatted
// document while = is active, the raw body otherwise.
func (m Model) msgBody() string {
	if m.msgFormatted {
		return m.msgFormattedBody
	}
	return m.selectedMessage.FullBody
}

// toggleMsgFormat is =: format the body in memory, or restore the raw
// view. Cursor, search and selection reset on entry — there is no
// honest mapping of a raw position into pretty-printed text.
func (m Model) toggleMsgFormat() (Model, tea.Cmd) {
	if m.msgFormatted {
		st := m.msgFormatStash
		m.clearMsgFormat()
		m.msgVim.span.Stop()
		m.messageSearch.bar.Clear()
		m.messageSearch.cursor = 0
		m.messageViewport.SetContent(m.Styles.Syntax.HighlightJSON(m.selectedMessage.FullBody))
		if st != nil {
			m.msgVim.cur = st.cur
			m.messageViewport.SetYOffset(st.yoffset)
			m.messageViewport.SetXOffset(st.xoffset)
		}
		m.Notify(appshell.LevelInfo, "Raw view restored")
		return m, nil
	}

	formatted, kind, ok := ui.FormatDocument([]byte(m.selectedMessage.FullBody))
	if !ok {
		m.Notify(appshell.LevelInfo, "Content is neither valid JSON nor XML")
		return m, nil
	}

	m.msgFormatStash = &msgFormatStash{
		cur:     m.msgVim.cur,
		yoffset: m.messageViewport.YOffset(),
		xoffset: m.messageViewport.XOffset(),
	}
	m.msgFormatted = true
	m.msgFormattedBody = string(formatted)
	m.msgFormattedKind = kind
	m.msgVim.cur = vim.Cursor{}
	m.msgVim.span.Stop()
	m.messageSearch.bar.Clear()
	m.messageSearch.cursor = 0
	m.messageViewport.SetContent(m.Styles.Syntax.HighlightJSON(m.msgFormattedBody))
	m.messageViewport.SetYOffset(0)
	m.messageViewport.SetXOffset(0)

	m.Notify(appshell.LevelSuccess, fmt.Sprintf("Formatted as %s (in-memory) — = to restore", kind))
	return m, nil
}

// clearMsgFormat drops the format state — the toggle back, and any
// selection change: a formatted view of one message must not survive
// onto another.
func (m *Model) clearMsgFormat() {
	m.msgFormatted = false
	m.msgFormattedBody = ""
	m.msgFormattedKind = ""
	m.msgFormatStash = nil
}
