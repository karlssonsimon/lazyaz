package vim

import "github.com/karlssonsimon/lazyaz/internal/keymap"

// MotionKeys is the binding set a buffer surface hands to BufferMotion.
// The preview builds it from its Preview* bindings; the message body can
// build it from CursorDown/Up. The engine never reads the keymap
// directly, so surfaces with different binding names share one dispatch.
type MotionKeys struct {
	Left, Right, Down, Up          keymap.Binding
	WordForward, WordBack, WordEnd keymap.Binding
	LineStart, LineEnd             keymap.Binding
	FindChar, FindCharBack         keymap.Binding
	TillChar, TillCharBack         keymap.Binding
	RepeatFind, RepeatFindBack     keymap.Binding
}

// BufferMotionResult is what a key did to the buffer cursor.
type BufferMotionResult int

const (
	// BufNone: not a motion key; the caller keeps routing it.
	BufNone BufferMotionResult = iota
	// BufMoved: the cursor moved; apply it.
	BufMoved
	// BufPending: f/F/t/T armed; the next key is the target rune.
	BufPending
	// BufFailed: consumed, cursor unchanged — a find that missed, a
	// cancelled chord, or a repeat with no find to repeat.
	BufFailed
)

// BufferMotion resolves one key against a buffer and cursor. Pending
// find state and repeat memory live in the resolver; counts are taken
// from it by whichever motion fires.
//
// Call this before digit handling: while a find is pending the next key
// is a rune argument, so f3 finds the character 3 rather than starting
// a count. A key consumed here also disarms the gg chord — f g must
// not leave g armed.
func (r *Resolver) BufferMotion(mk MotionKeys, key string, b TextBuffer, cur Cursor) (Cursor, BufferMotionResult) {
	if r.findPending {
		r.findPending = false
		r.gg = false
		target, ok := keyRune(key)
		if !ok {
			// Esc or a non-rune key cancels the chord.
			r.ClearCount()
			return cur, BufFailed
		}
		till, back := r.findTill, r.findBack
		r.lastFindRune, r.lastFindTill, r.lastFindBack, r.lastFindOK = target, till, back, true
		nc, found := FindOnLine(b, cur, target, till, back, r.TakeCount())
		if !found {
			return cur, BufFailed
		}
		return nc, BufMoved
	}

	consume := func(c Cursor) (Cursor, BufferMotionResult) {
		r.gg = false
		return c, BufMoved
	}

	switch {
	case mk.Left.Matches(key):
		return consume(MoveLeft(b, cur, r.TakeCount()))
	case mk.Right.Matches(key):
		return consume(MoveRight(b, cur, r.TakeCount()))
	case mk.Down.Matches(key):
		return consume(MoveDown(b, cur, r.TakeCount()))
	case mk.Up.Matches(key):
		return consume(MoveUp(b, cur, r.TakeCount()))
	case mk.WordForward.Matches(key):
		return consume(WordForward(b, cur, r.TakeCount()))
	case mk.WordBack.Matches(key):
		return consume(WordBack(b, cur, r.TakeCount()))
	case mk.WordEnd.Matches(key):
		return consume(WordEnd(b, cur, r.TakeCount()))
	// 0 is a motion only when no count is pending — with one, the
	// caller's digit handling consumes it first (vim's rule). Reaching
	// here with a pending count means the caller mis-ordered; refuse.
	case mk.LineStart.Matches(key) && r.PendingCount() == 0:
		return consume(LineStart(cur))
	case mk.LineEnd.Matches(key):
		return consume(LineEnd(b, cur, r.TakeCount()))
	case mk.FindChar.Matches(key):
		return r.armFind(cur, false, false)
	case mk.FindCharBack.Matches(key):
		return r.armFind(cur, false, true)
	case mk.TillChar.Matches(key):
		return r.armFind(cur, true, false)
	case mk.TillCharBack.Matches(key):
		return r.armFind(cur, true, true)
	case mk.RepeatFind.Matches(key):
		return r.repeatFind(b, cur, false)
	case mk.RepeatFindBack.Matches(key):
		return r.repeatFind(b, cur, true)
	}
	return cur, BufNone
}

// FindPending reports whether the next key will be taken as an f/t
// target rune. Callers route to BufferMotion unconditionally while true.
func (r *Resolver) FindPending() bool {
	return r.findPending
}

func (r *Resolver) armFind(cur Cursor, till, back bool) (Cursor, BufferMotionResult) {
	r.gg = false
	r.findPending = true
	r.findTill = till
	r.findBack = back
	return cur, BufPending
}

// repeatFind is ; and ,. Reversing keeps the till-ness. Repeating a
// till gets vim's adjacency skip: the search starts one cell further so
// a cursor already sitting before the target jumps to the next one.
func (r *Resolver) repeatFind(b TextBuffer, cur Cursor, reverse bool) (Cursor, BufferMotionResult) {
	r.gg = false
	if !r.lastFindOK {
		r.ClearCount()
		return cur, BufFailed
	}
	back := r.lastFindBack
	if reverse {
		back = !back
	}
	start := cur
	if r.lastFindTill {
		if back {
			start = MoveLeft(b, start, 1)
		} else {
			start = MoveRight(b, start, 1)
		}
	}
	nc, found := FindOnLine(b, start, r.lastFindRune, r.lastFindTill, back, r.TakeCount())
	if !found {
		return cur, BufFailed
	}
	return nc, BufMoved
}

// keyRune maps a key string to the rune it types. Printable keys are
// their single rune; space arrives as the word "space".
func keyRune(key string) (rune, bool) {
	if key == "space" {
		return ' ', true
	}
	rs := []rune(key)
	if len(rs) == 1 {
		return rs[0], true
	}
	return 0, false
}
