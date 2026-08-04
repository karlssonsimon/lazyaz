package vim

// SpanMode is how a buffer selection interprets its endpoints.
type SpanMode int

const (
	SpanChar SpanMode = iota // v: character-wise
	SpanLine                 // V: line-wise
)

// Span is a visual selection over a byte-addressed buffer: a fixed
// anchor offset with the cursor as the moving end. Deliberately not
// Visual — the key-identified anchor exists because list items reorder
// under a selection, and bytes in a blob do not.
type Span struct {
	Anchor int64
	Mode   SpanMode
	Active bool
}

// Start begins a selection anchored at the given offset.
func (s *Span) Start(anchor int64, mode SpanMode) {
	s.Anchor = anchor
	s.Mode = mode
	s.Active = true
}

// Stop ends the selection.
func (s *Span) Stop() {
	s.Active = false
}

// Range returns the selection's byte bounds given the cursor offset,
// ordered lo ≤ hi. Both endpoints are rune starts; the consumer extends
// hi to its rune end (charwise) or both ends to line bounds (linewise).
func (s Span) Range(head int64) (lo, hi int64) {
	if s.Anchor <= head {
		return s.Anchor, head
	}
	return head, s.Anchor
}
