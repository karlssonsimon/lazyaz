// Package vim holds the vim-flavored interaction machinery shared by
// every surface in the app: multi-key chord resolution, visual-line
// selection and mark sets.
//
// The division of labor is fixed: vim resolves keys and state into
// concrete answers ("the chord completed", "rows 4–9 are selected",
// "this key is marked") and the consumer applies them. vim never
// scrolls a list, renders a bar or touches a tea.Model — which is what
// lets a new motion land here once and reach every surface.
package vim

import "github.com/karlssonsimon/lazyaz/internal/keymap"

// ChordResult is what one keystroke did to a chord in progress.
type ChordResult int

const (
	// ChordNone: the key is not part of this chord. Any pending state
	// was cleared; the caller routes the key normally.
	ChordNone ChordResult = iota
	// ChordArmed: the chord's first key landed; the caller may show a
	// hint while the second key is awaited.
	ChordArmed
	// ChordFired: the chord completed.
	ChordFired
	// ChordSwallowed: an armed chord got a continuation it doesn't
	// know. The key is consumed so a mistyped chord cannot trigger some
	// unrelated binding.
	ChordSwallowed
)

// ScrollOp is which z-family motion a fired scroll chord resolved to.
type ScrollOp int

const (
	ScrollOpCenter ScrollOp = iota // zz
	ScrollOpTop                    // zt
	ScrollOpBottom                 // zb
)

// Hints shown while a chord is armed.
const (
	HintGG     = "Press g again for top"
	HintScroll = "z: t top · z center · b bottom"
)

// Resolver is the pending-key state machine for multi-key chords. One
// instance per model replaces the scattered per-context boolean flags.
// The two chords keep independent pending state, exactly like the
// separate flags they replace, and each handler consults only the chord
// that exists in its context — the resolver never routes a chord into a
// surface that didn't ask for it.
type Resolver struct {
	gg    bool
	z     bool
	count int
}

// countCap bounds an accumulated count. Far beyond any real list, it
// exists so held-down digits cannot overflow the arithmetic downstream.
const countCap = 999999

// Digit feeds a key to the count prefix. Digits 1-9 start a count and
// 0-9 continue one; a lone 0 is not consumed: it is unbound today and
// stays free for the future line-start motion. Reports whether the key
// was consumed.
func (r *Resolver) Digit(key string) bool {
	if len(key) != 1 || key[0] < '0' || key[0] > '9' {
		return false
	}
	if key == "0" && r.count == 0 {
		return false
	}
	r.count = r.count*10 + int(key[0]-'0')
	if r.count > countCap {
		r.count = countCap
	}
	return true
}

// TakeCount returns the pending count (1 when none) and clears it.
// The motion that fires is the one that consumes the count.
func (r *Resolver) TakeCount() int {
	n := r.count
	r.count = 0
	if n < 1 {
		return 1
	}
	return n
}

// PendingCount is the accumulated count, 0 when none. For display.
func (r *Resolver) PendingCount() int {
	return r.count
}

// ClearCount drops a pending count without using it. Any key that is
// neither a digit nor a counted motion does this, so a stray digit
// cannot ambush the next motion.
func (r *Resolver) ClearCount() {
	r.count = 0
}

// Clear drops any pending chord. Call when leaving the context the
// chord was armed in — closing the preview, say — so a stale first key
// cannot complete a chord somewhere else.
func (r *Resolver) Clear() {
	r.gg = false
	r.z = false
	r.count = 0
}

// GG feeds a key to the gg chord. homeImmediate controls the Home key:
// the preview and message viewers jump on a single Home, while the file
// browser treats every JumpTopPrefix key as arming.
//
// gg never swallows: a non-chord key clears the pending state and
// returns ChordNone so the caller processes it normally. Hoisting this
// call to the top of a handler is safe as long as the chord's keys are
// not bound to anything else in that context.
func (r *Resolver) GG(b keymap.Binding, key string, homeImmediate bool) ChordResult {
	armed := r.gg
	r.gg = false
	if !b.Matches(key) {
		return ChordNone
	}
	if armed || (homeImmediate && key == "home") {
		return ChordFired
	}
	r.gg = true
	return ChordArmed
}

// Scroll feeds a key to the z chord. The ScrollOp is only meaningful
// when the result is ChordFired.
func (r *Resolver) Scroll(km keymap.Keymap, key string) (ChordResult, ScrollOp) {
	if r.z {
		r.z = false
		switch {
		case km.ScrollCenter.Matches(key):
			return ChordFired, ScrollOpCenter
		case km.ScrollTop.Matches(key):
			return ChordFired, ScrollOpTop
		case km.ScrollBottom.Matches(key):
			return ChordFired, ScrollOpBottom
		}
		return ChordSwallowed, 0
	}
	if km.ScrollPrefix.Matches(key) {
		r.z = true
		return ChordArmed, 0
	}
	return ChordNone, 0
}
