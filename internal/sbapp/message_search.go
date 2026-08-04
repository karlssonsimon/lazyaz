package sbapp

import (
	"strings"

	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	tea "charm.land/bubbletea/v2"
)

// messageSearchState is the message body's search. The body is one
// string already in memory, so there is no streaming, no budget and no
// resume offset — just a bar and the line the last match landed on.
type messageSearchState struct {
	bar ui.SearchBar
	// cursor is the byte offset the last match started at, so n advances
	// instead of finding the same hit again.
	cursor int
}

// handleMessageSearchKey routes a key to the message body's search.
// Returns false when the key is not a search key so the caller keeps
// looking.
func (m *Model) handleMessageSearchKey(key string) (tea.Cmd, bool) {
	if m.messageSearch.bar.InputOpen {
		consumed, submitted := m.messageSearch.bar.HandleKey(key, m.Keymap)
		if submitted {
			if err := m.messageSearch.bar.Accept(); err != nil {
				m.Notify(appshell.LevelError, err.Error())
				return nil, true
			}
			m.messageSearch.cursor = 0
			m.runMessageSearch(m.messageSearch.bar.Direction, true)
		}
		return nil, consumed || submitted
	}

	switch {
	case m.Keymap.SearchForward.Matches(key):
		m.messageSearch.bar.Open(ui.SearchForward)
		return nil, true
	case m.Keymap.SearchBackward.Matches(key):
		m.messageSearch.bar.Open(ui.SearchBackward)
		return nil, true
	case m.Keymap.SearchNext.Matches(key):
		m.runMessageSearch(m.messageSearch.bar.Direction, false)
		return nil, true
	case m.Keymap.SearchPrev.Matches(key):
		m.runMessageSearch(m.messageSearch.bar.Direction.Opposite(), false)
		return nil, true
	}
	return nil, false
}

// runMessageSearch moves to the next match, scrolling it into view.
// Unlike the blob preview this wraps immediately: the body is already in
// memory, so wrapping costs nothing and there is no reason to make the
// user press n twice.
func (m *Model) runMessageSearch(dir ui.SearchDirection, fromStart bool) {
	pattern := m.messageSearch.bar.Pattern
	if !pattern.Valid() {
		m.Notify(appshell.LevelInfo, "No previous search")
		return
	}

	body := []byte(m.selectedMessage.FullBody)
	if len(body) == 0 {
		m.Notify(appshell.LevelInfo, "Nothing to search")
		return
	}

	from := m.messageSearch.cursor
	if fromStart {
		from = 0
		if dir == ui.SearchBackward {
			from = len(body)
		}
	} else if dir == ui.SearchBackward {
		// Backward looks strictly before the current match.
	} else {
		from++
	}

	var match *ui.SearchMatch
	if dir == ui.SearchBackward {
		match = ui.SearchBufferBackward(pattern, body, from)
		if match == nil {
			match = ui.SearchBufferBackward(pattern, body, len(body))
			if match != nil {
				m.Notify(appshell.LevelInfo, "search hit TOP, continuing at BOTTOM")
			}
		}
	} else {
		match = ui.SearchBufferForward(pattern, body, from)
		if match == nil {
			match = ui.SearchBufferForward(pattern, body, 0)
			if match != nil {
				m.Notify(appshell.LevelInfo, "search hit BOTTOM, continuing at TOP")
			}
		}
	}

	if match == nil {
		m.Notify(appshell.LevelWarn, "Pattern not found: "+pattern.Query)
		return
	}

	m.messageSearch.cursor = match.Start
	if m.msgVim.active {
		m.msgSyncFromByte(int64(match.Start))
		m.msgFollowCursor()
		return
	}
	m.revealMessageOffset(match.Start)
}

// revealMessageOffset scrolls the viewport so the line holding the given
// byte offset is visible, centring it when the body is long enough.
func (m *Model) revealMessageOffset(offset int) {
	line := strings.Count(m.selectedMessage.FullBody[:offset], "\n")

	height := m.messageViewport.Height()
	target := line
	if height > 2 {
		target = line - height/2
	}
	if target < 0 {
		target = 0
	}
	m.messageViewport.SetYOffset(target)
}

// messageMatchRanges maps matches in the body onto viewport rows and
// display columns. The body is highlighted as one string, so the row is
// the line index less the scroll offset.
func (m Model) messageMatchRanges() map[int][]ui.ColumnRange {
	pattern := m.messageSearch.bar.Pattern
	if !pattern.Valid() || m.selectedMessage.FullBody == "" {
		return nil
	}

	body := m.selectedMessage.FullBody
	matches := ui.SearchBufferAll(pattern, []byte(body))
	if len(matches) == 0 {
		return nil
	}

	lineStarts := messageLineStarts(body)
	top := m.messageViewport.YOffset()
	height := m.messageViewport.Height()

	byRow := make(map[int][]ui.ColumnRange)
	for _, match := range matches {
		line := lineOfOffset(lineStarts, match.Start)
		row := line - top
		if row < 0 || row >= height {
			continue
		}
		lineStart := lineStarts[line]
		xoff := m.messageViewport.XOffset()
		width := m.messageViewport.Width()
		startCol := ui.PlainWidth([]byte(body[lineStart:match.Start])) - xoff
		endCol := startCol + ui.PlainWidth([]byte(body[match.Start:match.End]))
		if endCol <= 0 || startCol >= width {
			continue
		}
		if startCol < 0 {
			startCol = 0
		}
		if endCol > width {
			endCol = width
		}

		style := m.Styles.SearchMatch
		if match.Start == m.messageSearch.cursor {
			style = m.Styles.SearchMatchCurrent
		}
		byRow[row] = append(byRow[row], ui.ColumnRange{Start: startCol, End: endCol, Style: style})
	}
	return byRow
}

func messageLineStarts(s string) []int {
	starts := []int{0}
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			starts = append(starts, i+1)
		}
	}
	return starts
}

func lineOfOffset(starts []int, offset int) int {
	lo, hi := 0, len(starts)-1
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if starts[mid] <= offset {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return lo
}

// BufferSearchFocused reports that the message body is focused and owns
// the vim search keys.
func (m Model) BufferSearchFocused() bool {
	return m.viewingMessage
}
