package ui

import (
	"fmt"
	"strings"

	"github.com/karlssonsimon/lazyaz/internal/keymap"

	"charm.land/bubbles/v2/cursor"
)

// CopyTarget is one copyable value offered by the copy palette. Value
// holds the real, untruncated text that lands on the clipboard — the
// palette truncates only for display, so values that render truncated
// elsewhere in the UI (breadcrumbs, list rows) still copy in full.
type CopyTarget struct {
	Label string
	Value string
}

// CopyOverlay is the copy palette popup: a searchable list of the
// copyable values in the current context. Apps open it with their
// context-specific targets and copy whatever the user picks.
type CopyOverlay struct {
	SearchableOverlay[CopyTarget]
}

// Open populates the palette. Filtering matches on label and value so
// typing part of a path finds the right entry.
func (s *CopyOverlay) Open(targets []CopyTarget) {
	s.SearchableOverlay.Open(targets, func(t CopyTarget) string {
		return t.Label + " " + t.Value
	})
}

// HandleKey processes a key press in the copy palette. Returns the
// picked target and true when the user applied a selection.
func (s *CopyOverlay) HandleKey(key string, km keymap.Keymap) (CopyTarget, bool) {
	switch {
	case km.ThemeUp.Matches(key):
		s.Move(-1)
	case km.ThemeDown.Matches(key):
		s.Move(1)
	case km.ThemeApply.Matches(key):
		if t, ok := s.Selected(); ok {
			s.Close()
			return t, true
		}
	case km.ThemeCancel.Matches(key):
		s.Cancel()
	case key == "ctrl+v":
		if text := ReadClipboard(); text != "" {
			s.TypeText(text)
		}
	default:
		s.HandleQueryKey(key)
	}
	return CopyTarget{}, false
}

// RenderCopyOverlay paints the copy palette on top of base.
func RenderCopyOverlay(s CopyOverlay, km keymap.Keymap, cur cursor.Model, styles Styles, width, height int, base string) string {
	visible := s.Visible()
	items := make([]OverlayItem, len(visible))
	for i, t := range visible {
		items[i] = OverlayItem{Label: copyTargetRow(t)}
	}
	cfg := OverlayListConfig{
		Title:       "Copy",
		Query:       s.Query,
		QueryCursor: s.QueryCaret,
		Cursor:      cur,
		CloseHint:   km.Cancel.Short(),
		Bindings: &OverlayBindings{
			MoveUp:   km.ThemeUp,
			MoveDown: km.ThemeDown,
			Apply:    km.ThemeApply,
			Cancel:   km.ThemeCancel,
			Erase:    km.BackspaceUp,
		},
		MaxVisible:     12,
		Center:         true,
		NoActiveMarker: true,
	}
	return RenderOverlayList(cfg, items, s.CursorIdx, styles, width, height, base)
}

// copyTargetRow lays out one palette row: fixed-width label, then a
// display-truncated single-line preview of the value.
func copyTargetRow(t CopyTarget) string {
	preview := strings.ReplaceAll(t.Value, "\n", " ")
	return fmt.Sprintf("%-18s %s", t.Label, truncateMiddle(preview, 38))
}
