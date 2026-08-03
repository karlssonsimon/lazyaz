package ui

import "github.com/karlssonsimon/lazyaz/internal/keymap"

// ListMotion reports what HandleListMotion did with a key, so the caller
// knows whether to keep routing it and whether to show the chord hint.
type ListMotion int

const (
	// MotionNone means the key was not a scroll motion; keep routing it.
	MotionNone ListMotion = iota
	// MotionHandled means the motion was applied and the key is consumed.
	MotionHandled
	// MotionChordOpen means `z` was typed and the second key is pending.
	MotionChordOpen
)

// ScrollChordHint is what to show while a `z` chord is waiting for its
// second key. Unlike the gg chord there are three continuations, so
// spelling them out beats a bare "press z again".
const ScrollChordHint = "z: t top · z center · b bottom"

// HandleListMotion routes a key through the vim-style scroll motions for
// one list. chordPending carries the half-typed `z` across keystrokes and
// is owned by the caller, so each pane keeps its own chord state.
//
// Plain cursor movement is not handled here — the bubbles list still owns
// CursorUp/CursorDown via the user's keymap, and List re-derives the
// window offset afterwards.
func HandleListMotion(l *List, km keymap.Keymap, key string, chordPending *bool) ListMotion {
	if l == nil {
		return MotionNone
	}

	if *chordPending {
		*chordPending = false
		switch {
		case km.ScrollCenter.Matches(key):
			l.CenterOnCursor()
		case km.ScrollTop.Matches(key):
			l.CursorToTop()
		case km.ScrollBottom.Matches(key):
			l.CursorToBottom()
		}
		// An unrecognized continuation is swallowed rather than passed
		// on, so a mistyped chord cannot trigger some unrelated action.
		return MotionHandled
	}

	switch {
	case km.ScrollPrefix.Matches(key):
		*chordPending = true
		return MotionChordOpen
	case km.ScrollLineDown.Matches(key):
		l.ScrollBy(1)
	case km.ScrollLineUp.Matches(key):
		l.ScrollBy(-1)
	case km.FullPageDown.Matches(key):
		l.MoveCursor(l.Window().Height)
	case km.FullPageUp.Matches(key):
		l.MoveCursor(-l.Window().Height)
	default:
		return MotionNone
	}
	return MotionHandled
}
