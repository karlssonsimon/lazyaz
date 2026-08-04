package vim

// Visual is a linewise selection over a list's visible items: a fixed
// anchor plus the cursor as the moving end. The consumer owns the
// cursor (it is the list's index); Visual owns the anchor and the range
// arithmetic.
//
// The anchor is identity — a stable item key — not an index. Visible
// items shift under an active selection (streaming refreshes append,
// filter edits reshape the view), and a key self-heals where an index
// would silently drift onto different items.
//
// Resolving key→index is a full scan of the visible items, which is too
// expensive per keypress at 200k items, so the resolved index is cached
// and reused until the caller-supplied version says the visible set
// changed. ui.List bumps its version in every mutating path, so the
// cache needs no per-app invalidation calls.
type Visual struct {
	active bool
	anchor string

	cachedIdx  int
	cachedVer  int
	cacheValid bool
}

// Start enters visual mode anchored at the given key. An empty key is
// valid — visual mode on an empty list has no anchor until the range
// collapses to the cursor.
func (v *Visual) Start(anchorKey string) {
	v.active = true
	v.anchor = anchorKey
	v.cacheValid = false
}

// Stop leaves visual mode.
func (v *Visual) Stop() {
	v.active = false
	v.anchor = ""
	v.cacheValid = false
}

// Active reports whether visual mode is on.
func (v Visual) Active() bool {
	return v.active
}

// Anchor returns the anchor's key.
func (v Visual) Anchor() string {
	return v.anchor
}

// SetAnchorWithIdx swaps in a new anchor whose index is already known —
// the anchor-swap motion has both ends in hand, and the visible set
// does not change during a swap, so the cache stays warm instead of
// forcing a rescan.
func (v *Visual) SetAnchorWithIdx(key string, idx, version int) {
	v.anchor = key
	v.cachedIdx = idx
	v.cachedVer = version
	v.cacheValid = true
}

// AnchorIdx resolves the anchor to its index in the visible items, or
// -1 when there is no anchor or it is filtered out of view. find does
// the scan; it runs at most once per version, and a miss is cached like
// a hit — an anchor hidden by a filter must not rescan per keypress.
func (v *Visual) AnchorIdx(version int, find func(key string) int) int {
	if v.anchor == "" {
		return -1
	}
	if v.cacheValid && v.cachedVer == version {
		return v.cachedIdx
	}
	v.cachedIdx = find(v.anchor)
	v.cachedVer = version
	v.cacheValid = true
	return v.cachedIdx
}

// Range returns the selection bounds in visible-index space, ordered.
// An anchor that is absent (empty, or filtered out of view) collapses
// the range to the cursor row. ok is false when visual mode is off.
func (v *Visual) Range(cursor, version int, find func(key string) int) (lo, hi int, ok bool) {
	if !v.active {
		return 0, 0, false
	}
	anchor := v.AnchorIdx(version, find)
	if anchor < 0 {
		anchor = cursor
	}
	if anchor > cursor {
		return cursor, anchor, true
	}
	return anchor, cursor, true
}
