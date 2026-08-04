package vim

import "github.com/karlssonsimon/lazyaz/internal/keymap"

// MotionKeys is the binding set a buffer surface hands to BufferMotion.
// The preview builds it from its Preview* bindings; the message body can
// build it from CursorDown/Up. The engine never reads the keymap
// directly, so surfaces with different binding names share one dispatch.
type MotionKeys struct {
	Left, Right, Down, Up                   keymap.Binding
	WordForward, WordBack, WordEnd          keymap.Binding
	BigWordForward, BigWordBack, BigWordEnd keymap.Binding
	LineStart, LineEnd                      keymap.Binding
	FirstNonBlank, Underscore               keymap.Binding
	FindChar, FindCharBack                  keymap.Binding
	TillChar, TillCharBack                  keymap.Binding
	RepeatFind, RepeatFindBack              keymap.Binding
	ObjectInner, ObjectAround               keymap.Binding
	// YankOp is the operator key — the consumer arms the operator, and
	// this binding doubling while pending is yy.
	YankOp keymap.Binding
}

// BufferActionKind is what a key did to the buffer.
type BufferActionKind int

const (
	// BufNone: not a grammar key; the caller keeps routing it.
	BufNone BufferActionKind = iota
	// BufMoved: the cursor moved; apply Cursor.
	BufMoved
	// BufPending: a chord, operator or object is awaiting its next key.
	BufPending
	// BufFailed: consumed, nothing happened — a missed find, a cancelled
	// grammar, an empty span.
	BufFailed
	// BufYank: the operator completed; yank Region.
	BufYank
	// BufSelect: a text object resolved in visual mode; select Region.
	BufSelect
)

// BufferAction is BufferMotion's resolved instruction.
type BufferAction struct {
	Kind   BufferActionKind
	Cursor Cursor
	Region Region
}

func actMoved(c Cursor) BufferAction   { return BufferAction{Kind: BufMoved, Cursor: c} }
func actPending(c Cursor) BufferAction { return BufferAction{Kind: BufPending, Cursor: c} }
func actFailed(c Cursor) BufferAction  { return BufferAction{Kind: BufFailed, Cursor: c} }
func actYank(r Region) BufferAction    { return BufferAction{Kind: BufYank, Region: r} }
func actSelect(r Region) BufferAction  { return BufferAction{Kind: BufSelect, Region: r} }

// BufferMotion resolves one key of the buffer grammar. All pending
// state — find targets, the operator, object selection — lives in the
// resolver, and whenever any of it is armed this is the single router:
// call it before digit handling and before other chords (f3 finds the
// character 3, yg must not arm the gg chord).
//
// visual reports whether a selection is active, which is what lets
// i/a text objects resolve without an operator (vi", va{).
func (r *Resolver) BufferMotion(mk MotionKeys, key string, b TextBuffer, cur Cursor, visual bool) BufferAction {
	if r.findPending {
		return r.finishFind(b, cur, key)
	}
	if r.objPending != 0 {
		return r.finishObject(b, cur, key)
	}
	if r.opPending {
		return r.operatorKey(mk, key, b, cur)
	}

	if visual && mk.ObjectInner.Matches(key) {
		r.gg = false
		r.objPending = 1
		return actPending(cur)
	}
	if visual && mk.ObjectAround.Matches(key) {
		r.gg = false
		r.objPending = 2
		return actPending(cur)
	}

	switch {
	case mk.FindChar.Matches(key):
		return r.armFind(cur, false, false)
	case mk.FindCharBack.Matches(key):
		return r.armFind(cur, false, true)
	case mk.TillChar.Matches(key):
		return r.armFind(cur, true, false)
	case mk.TillCharBack.Matches(key):
		return r.armFind(cur, true, true)
	case mk.RepeatFind.Matches(key):
		nc, _, found := r.resolveRepeat(b, cur, false)
		if !found {
			return actFailed(cur)
		}
		r.gg = false
		return actMoved(nc)
	case mk.RepeatFindBack.Matches(key):
		nc, _, found := r.resolveRepeat(b, cur, true)
		if !found {
			return actFailed(cur)
		}
		r.gg = false
		return actMoved(nc)
	}

	if to, _, ok := r.motionTarget(mk, key, b, cur); ok {
		r.gg = false
		return actMoved(to)
	}
	return BufferAction{Kind: BufNone}
}

// ArmOperator arms y. The consumer decides when the key means the
// operator (no selection active) versus a visual yank.
func (r *Resolver) ArmOperator() {
	r.opPending = true
	r.gg = false
}

// OperatorPending reports an armed operator.
func (r *Resolver) OperatorPending() bool {
	return r.opPending
}

// BufferPending reports any armed buffer-grammar state — while true,
// every key belongs to BufferMotion.
func (r *Resolver) BufferPending() bool {
	return r.findPending || r.opPending || r.objPending != 0
}

// FindPending reports whether the next key is an f/t target rune.
func (r *Resolver) FindPending() bool {
	return r.findPending
}

// ObjectPending reports an armed i/a awaiting its object key.
func (r *Resolver) ObjectPending() bool {
	return r.objPending != 0
}

// ConsumeOperator reports and clears an armed operator. Consumers use
// it for operator motions the engine cannot express — yG and ygg reach
// beyond the loaded window, so the preview resolves them in byte space.
func (r *Resolver) ConsumeOperator() bool {
	if !r.opPending {
		return false
	}
	r.opPending = false
	return true
}

// motionTarget resolves a plain motion key to its landing cursor and
// kind, consuming the pending count. This is the one table both the
// move path and the operator path share — a new motion added here
// reaches y automatically.
func (r *Resolver) motionTarget(mk MotionKeys, key string, b TextBuffer, cur Cursor) (Cursor, MotionKind, bool) {
	switch {
	case mk.Left.Matches(key):
		return MoveLeft(b, cur, r.TakeCount()), KindExclusive, true
	case mk.Right.Matches(key):
		return MoveRight(b, cur, r.TakeCount()), KindExclusive, true
	case mk.Down.Matches(key):
		return MoveDown(b, cur, r.TakeCount()), KindLinewise, true
	case mk.Up.Matches(key):
		return MoveUp(b, cur, r.TakeCount()), KindLinewise, true
	case mk.WordForward.Matches(key):
		return WordForward(b, cur, r.TakeCount()), KindExclusive, true
	case mk.WordBack.Matches(key):
		return WordBack(b, cur, r.TakeCount()), KindExclusive, true
	case mk.WordEnd.Matches(key):
		return WordEnd(b, cur, r.TakeCount()), KindInclusive, true
	case mk.BigWordForward.Matches(key):
		return WORDForward(b, cur, r.TakeCount()), KindExclusive, true
	case mk.BigWordBack.Matches(key):
		return WORDBack(b, cur, r.TakeCount()), KindExclusive, true
	case mk.BigWordEnd.Matches(key):
		return WORDEnd(b, cur, r.TakeCount()), KindInclusive, true
	// 0 is a motion only when no count is pending — with one, the
	// caller's digit handling consumes it first (vim's rule).
	case mk.LineStart.Matches(key) && r.PendingCount() == 0:
		return LineStart(cur), KindExclusive, true
	case mk.LineEnd.Matches(key):
		return LineEnd(b, cur, r.TakeCount()), KindInclusive, true
	case mk.FirstNonBlank.Matches(key):
		// Vim ignores the count for ^; drop a pending one.
		r.ClearCount()
		return FirstNonBlank(b, cur), KindExclusive, true
	case mk.Underscore.Matches(key):
		return LineFirstNonBlank(b, cur, r.TakeCount()), KindLinewise, true
	}
	return cur, 0, false
}

// operatorKey routes a key while y is armed.
func (r *Resolver) operatorKey(mk MotionKeys, key string, b TextBuffer, cur Cursor) BufferAction {
	r.gg = false
	if r.Digit(key) {
		return actPending(cur)
	}
	switch {
	case mk.YankOp.Matches(key):
		// yy: linewise, count lines.
		n := r.TakeCount()
		r.opPending = false
		to := Cursor{Line: cur.Line + n - 1}
		return actYank(RegionFor(b, cur, to, KindLinewise))
	case mk.ObjectInner.Matches(key):
		r.objPending = 1
		return actPending(cur)
	case mk.ObjectAround.Matches(key):
		r.objPending = 2
		return actPending(cur)
	case mk.FindChar.Matches(key):
		return r.armFind(cur, false, false)
	case mk.FindCharBack.Matches(key):
		return r.armFind(cur, false, true)
	case mk.TillChar.Matches(key):
		return r.armFind(cur, true, false)
	case mk.TillCharBack.Matches(key):
		return r.armFind(cur, true, true)
	case mk.RepeatFind.Matches(key), mk.RepeatFindBack.Matches(key):
		nc, back, found := r.resolveRepeat(b, cur, mk.RepeatFindBack.Matches(key))
		r.opPending = false
		if !found {
			return actFailed(cur)
		}
		kind := KindInclusive
		if back {
			kind = KindExclusive
		}
		reg := RegionFor(b, cur, nc, kind)
		if reg.Empty() {
			return actFailed(cur)
		}
		return actYank(reg)
	}

	if to, kind, ok := r.motionTarget(mk, key, b, cur); ok {
		r.opPending = false
		reg := RegionFor(b, cur, to, kind)
		if reg.Empty() {
			return actFailed(cur)
		}
		return actYank(reg)
	}

	// Anything else — esc included — cancels the operator, consumed.
	r.opPending = false
	r.ClearCount()
	return actFailed(cur)
}

// finishFind consumes the f/t target rune. With the operator armed the
// find completes as a yank: f and t inclusive, F and T exclusive.
func (r *Resolver) finishFind(b TextBuffer, cur Cursor, key string) BufferAction {
	r.findPending = false
	r.gg = false
	target, ok := keyRune(key)
	if !ok {
		// Esc or a non-rune cancels the chord and any operator with it.
		r.opPending = false
		r.ClearCount()
		return actFailed(cur)
	}
	till, back := r.findTill, r.findBack
	r.lastFindRune, r.lastFindTill, r.lastFindBack, r.lastFindOK = target, till, back, true
	nc, found := FindOnLine(b, cur, target, till, back, r.TakeCount())
	if !found {
		r.opPending = false
		return actFailed(cur)
	}
	if r.opPending {
		r.opPending = false
		kind := KindInclusive
		if back {
			kind = KindExclusive
		}
		reg := RegionFor(b, cur, nc, kind)
		if reg.Empty() {
			return actFailed(cur)
		}
		return actYank(reg)
	}
	return actMoved(nc)
}

// finishObject consumes the object key after i or a. Counts do not
// apply to these objects and are dropped.
func (r *Resolver) finishObject(b TextBuffer, cur Cursor, key string) BufferAction {
	around := r.objPending == 2
	r.objPending = 0
	wasOp := r.opPending
	r.opPending = false
	r.ClearCount()

	target, ok := keyRune(key)
	if !ok {
		return actFailed(cur)
	}
	reg, ok := resolveObject(b, cur, target, around)
	if !ok {
		return actFailed(cur)
	}
	if wasOp {
		return actYank(reg)
	}
	return actSelect(reg)
}

// resolveObject maps an object key to its region, including vim's b/B
// bracket aliases.
func resolveObject(b TextBuffer, cur Cursor, target rune, around bool) (Region, bool) {
	switch target {
	case '"', '\'', '`':
		return QuoteObject(b, cur, target, around)
	case '(', ')', 'b':
		return BracketObject(b, cur, '(', ')', around)
	case '[', ']':
		return BracketObject(b, cur, '[', ']', around)
	case '{', '}', 'B':
		return BracketObject(b, cur, '{', '}', around)
	case '<', '>':
		return BracketObject(b, cur, '<', '>', around)
	case 't':
		return TagObject(b, cur, around)
	case 'w':
		return WordObject(b, cur, false, around)
	case 'W':
		return WordObject(b, cur, true, around)
	}
	return Region{}, false
}

func (r *Resolver) armFind(cur Cursor, till, back bool) BufferAction {
	r.gg = false
	r.findPending = true
	r.findTill = till
	r.findBack = back
	return actPending(cur)
}

// resolveRepeat is ; and , — the remembered find, reversible, with
// vim's adjacency skip for till repeats.
func (r *Resolver) resolveRepeat(b TextBuffer, cur Cursor, reverse bool) (Cursor, bool, bool) {
	if !r.lastFindOK {
		r.ClearCount()
		return cur, false, false
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
		return cur, back, false
	}
	return nc, back, true
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
