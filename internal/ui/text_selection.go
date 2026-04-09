package ui

import (
	"strings"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/x/ansi"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// TextSelection tracks mouse-driven text selection inside a viewport.
// Embed this in app models that have a preview pane with selectable text.
type TextSelection struct {
	Active   bool
	StartRow int // viewport-relative row where drag started
	StartCol int // viewport-relative column where drag started
	EndRow   int // viewport-relative row of current drag position
	EndCol   int // viewport-relative column of current drag position
}

// ViewportRegion describes where a viewport sits on screen so mouse
// coordinates can be translated to viewport-relative positions.
type ViewportRegion struct {
	X      int // screen column of viewport's top-left content cell
	Y      int // screen row of viewport's top-left content cell
	Width  int
	Height int
}

// Reset clears the selection state.
func (s *TextSelection) Reset() {
	*s = TextSelection{}
}

// HandleMouseClick starts a new selection if the click is inside the region.
// Returns true if the click was inside the viewport.
func (s *TextSelection) HandleMouseClick(msg tea.MouseClickMsg, region ViewportRegion) bool {
	if msg.Button != tea.MouseLeft {
		return false
	}
	row, col, inside := screenToViewport(msg.X, msg.Y, region)
	if !inside {
		return false
	}
	s.Active = true
	s.StartRow = row
	s.StartCol = col
	s.EndRow = row
	s.EndCol = col
	return true
}

// HandleMouseMotion updates the selection end position during a drag.
// Returns true if the motion was inside the viewport and the selection updated.
func (s *TextSelection) HandleMouseMotion(msg tea.MouseMotionMsg, region ViewportRegion) bool {
	if !s.Active {
		return false
	}
	row, col, _ := screenToViewport(msg.X, msg.Y, region)
	// Clamp to viewport bounds.
	if row < 0 {
		row = 0
	}
	if row >= region.Height {
		row = region.Height - 1
	}
	if col < 0 {
		col = 0
	}
	if col >= region.Width {
		col = region.Width - 1
	}
	s.EndRow = row
	s.EndCol = col
	return true
}

// HandleMouseRelease finalises the selection and copies the selected text
// to the clipboard. Returns the copied text and true if text was copied.
func (s *TextSelection) HandleMouseRelease(msg tea.MouseReleaseMsg, vp viewport.Model, region ViewportRegion) (string, bool) {
	if !s.Active {
		return "", false
	}
	defer s.Reset()

	text := s.extractText(vp)
	if text == "" {
		return "", false
	}
	return text, true
}

// HighlightContent returns the viewport content with the current
// selection range rendered in reverse video. Call this from View()
// when s.Active is true to show the selection to the user.
func (s *TextSelection) HighlightContent(vp viewport.Model, highlight lipgloss.Style) string {
	if !s.Active {
		return vp.View()
	}

	content := vp.View()
	lines := strings.Split(content, "\n")
	vpWidth := vp.Width()

	startRow, startCol, endRow, endCol := s.ordered()

	for i := startRow; i <= endRow && i < len(lines); i++ {
		if i < 0 {
			continue
		}
		line := lines[i]
		lineWidth := ansi.StringWidth(line)

		selStart := 0
		selEnd := lineWidth
		if i == startRow {
			selStart = startCol
		}
		if i == endRow {
			selEnd = endCol + 1
		}
		if selStart >= lineWidth {
			continue
		}
		if selEnd > lineWidth {
			selEnd = lineWidth
		}
		if selStart >= selEnd {
			continue
		}

		// Build the line from three visual slices: before | selected | after.
		// Use ansi.Cut for clean extraction that preserves surrounding styles.
		before := ""
		if selStart > 0 {
			before = ansi.Truncate(line, selStart, "")
		}
		selectedPlain := ansi.Strip(ansi.Cut(line, selStart, selEnd))
		after := ansi.Cut(line, selEnd, lineWidth)

		rebuilt := before + highlight.Render(selectedPlain) + after

		// Clamp to viewport width so we never break the pane border.
		if ansi.StringWidth(rebuilt) > vpWidth {
			rebuilt = ansi.Truncate(rebuilt, vpWidth, "")
		}
		lines[i] = rebuilt
	}

	return strings.Join(lines, "\n")
}

// extractText returns the plain text covered by the current selection.
func (s *TextSelection) extractText(vp viewport.Model) string {
	content := vp.GetContent()
	allLines := strings.Split(content, "\n")

	startRow, startCol, endRow, endCol := s.ordered()
	yOff := vp.YOffset()

	// Convert viewport-relative rows to content rows.
	startRow += yOff
	endRow += yOff

	var sb strings.Builder
	for i := startRow; i <= endRow && i < len(allLines); i++ {
		if i < 0 {
			continue
		}
		line := ansi.Strip(allLines[i])
		lineLen := len([]rune(line))

		selStart := 0
		selEnd := lineLen
		if i == startRow {
			selStart = startCol
		}
		if i == endRow {
			selEnd = endCol + 1
		}
		if selStart > lineLen {
			selStart = lineLen
		}
		if selEnd > lineLen {
			selEnd = lineLen
		}
		if selStart > selEnd {
			selStart = selEnd
		}

		runes := []rune(line)
		sb.WriteString(string(runes[selStart:selEnd]))
		if i < endRow {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

// ordered returns start/end ensuring start <= end (top-left to bottom-right).
func (s *TextSelection) ordered() (startRow, startCol, endRow, endCol int) {
	sr, sc, er, ec := s.StartRow, s.StartCol, s.EndRow, s.EndCol
	if sr > er || (sr == er && sc > ec) {
		sr, sc, er, ec = er, ec, sr, sc
	}
	return sr, sc, er, ec
}

// screenToViewport converts screen coordinates to viewport-relative row/col.
func screenToViewport(screenX, screenY int, r ViewportRegion) (row, col int, inside bool) {
	col = screenX - r.X
	row = screenY - r.Y
	inside = col >= 0 && col < r.Width && row >= 0 && row < r.Height
	return row, col, inside
}

// WriteClipboard writes text to the system clipboard.
func WriteClipboard(text string) error {
	return clipboard.WriteAll(text)
}
