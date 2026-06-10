package ui

import (
	"time"

	"github.com/karlssonsimon/lazyaz/internal/azure"
	"github.com/karlssonsimon/lazyaz/internal/fuzzy"
	"github.com/karlssonsimon/lazyaz/internal/keymap"

	"charm.land/bubbles/v2/cursor"
)

// SubscriptionOverlayState manages the subscription picker overlay.
type SubscriptionOverlayState struct {
	Active     bool
	CursorIdx  int
	Query      string
	QueryCaret int // rune index of the edit caret within Query
	filtered   []int
}

// Open activates the overlay. If the overlay is already active (e.g. the
// user is mid-filter and a late subscriptions-loaded message triggers
// another Open call), this is a no-op — the user's query and cursor are
// preserved.
func (s *SubscriptionOverlayState) Open() {
	if s.Active {
		return
	}
	s.Active = true
	s.Query = ""
	s.QueryCaret = 0
	s.CursorIdx = 0
	s.filtered = nil
}

func (s *SubscriptionOverlayState) Close() {
	s.Active = false
}

// Refilter re-applies the current Query against the given subscription
// list. Call this after the underlying subscriptions change (e.g. when a
// new page streams in) so the overlay's view stays in sync with the data.
// Safe to call while the overlay is inactive.
func (s *SubscriptionOverlayState) Refilter(subs []azure.Subscription) {
	s.filtered = fuzzy.Filter(s.Query, subs, func(sub azure.Subscription) string {
		return sub.Name + " " + sub.ID
	})
	if s.CursorIdx >= len(s.filtered) {
		s.CursorIdx = max(0, len(s.filtered)-1)
	}
}

// HandleKey processes a key event. Returns the selected subscription and true
// if the user confirmed a selection.
func (s *SubscriptionOverlayState) HandleKey(key string, bindings ThemeKeyBindings, subs []azure.Subscription) (azure.Subscription, bool) {
	if len(subs) == 0 {
		s.Active = false
		return azure.Subscription{}, false
	}

	if s.filtered == nil {
		s.Refilter(subs)
	}

	switch {
	case bindings.Up.Matches(key):
		if s.CursorIdx > 0 {
			s.CursorIdx--
		}
		return azure.Subscription{}, false
	case bindings.Down.Matches(key):
		if s.CursorIdx < len(s.filtered)-1 {
			s.CursorIdx++
		}
		return azure.Subscription{}, false
	case bindings.Apply.Matches(key):
		if len(s.filtered) > 0 && s.CursorIdx < len(s.filtered) {
			sub := subs[s.filtered[s.CursorIdx]]
			s.Active = false
			return sub, true
		}
		return azure.Subscription{}, false
	case bindings.Cancel.Matches(key):
		if s.Query != "" {
			s.Query = ""
			s.QueryCaret = 0
			s.Refilter(subs)
		} else {
			s.Active = false
		}
		return azure.Subscription{}, false
	case key == "ctrl+v":
		if text := ReadClipboard(); text != "" {
			ti := TextInput{Value: s.Query, Cursor: s.QueryCaret}
			ti.Insert(text)
			s.Query = ti.Value
			s.QueryCaret = ti.Cursor
			s.Refilter(subs)
		}
		return azure.Subscription{}, false
	}
	ti := TextInput{Value: s.Query, Cursor: s.QueryCaret}
	if ti.HandleKey(key) {
		changed := ti.Value != s.Query
		s.Query = ti.Value
		s.QueryCaret = ti.Cursor
		if changed {
			s.Refilter(subs)
		}
	}
	return azure.Subscription{}, false
}

// RenderSubscriptionOverlay renders the subscription picker overlay.
// If loading is true, a spinner frame is appended to the title.
func RenderSubscriptionOverlay(state SubscriptionOverlayState, closeHint string, cur cursor.Model, subs []azure.Subscription, currentSub azure.Subscription, loading bool, loadingStartedAt time.Time, styles Styles, km *keymap.Keymap, width, height int, base string) string {
	filtered := state.filtered
	if filtered == nil {
		filtered = make([]int, len(subs))
		for i := range subs {
			filtered[i] = i
		}
	}

	items := make([]OverlayItem, len(filtered))
	for ci, si := range filtered {
		sub := subs[si]
		items[ci] = OverlayItem{
			Label:    SubscriptionDisplayName(sub),
			Hint:     sub.ID,
			IsActive: sub.ID == currentSub.ID && currentSub.ID != "",
		}
	}

	title := "Subscriptions"
	if loading {
		title += " " + SpinnerFrameAt(time.Since(loadingStartedAt))
	}

	cfg := OverlayListConfig{
		Title:       title,
		Query:       state.Query,
		QueryCursor: state.QueryCaret,
		Cursor:      cur,
		CloseHint:   closeHint,
		MaxVisible:  18,
		InnerWidth:  100,
		Center:      true,
		Bindings: &OverlayBindings{
			MoveUp:   km.ThemeUp,
			MoveDown: km.ThemeDown,
			Apply:    km.ThemeApply,
			Cancel:   km.Cancel,
			Erase:    km.BackspaceUp,
		},
	}
	return RenderOverlayList(cfg, items, state.CursorIdx, styles, width, height, base)
}
