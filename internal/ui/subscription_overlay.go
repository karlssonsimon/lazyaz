package ui

import (
	"time"

	"github.com/karlssonsimon/lazyaz/internal/azure"
	"github.com/karlssonsimon/lazyaz/internal/fuzzy"

	"charm.land/lipgloss/v2"
)

// SubscriptionBarHeight is the vertical space reserved for the subscription context bar.
const SubscriptionBarHeight = 2

// SubscriptionOverlayState manages the subscription picker overlay.
type SubscriptionOverlayState struct {
	Active    bool
	CursorIdx int
	Query     string
	filtered  []int
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
	case bindings.Down.Matches(key):
		if s.CursorIdx < len(s.filtered)-1 {
			s.CursorIdx++
		}
	case bindings.Apply.Matches(key):
		if len(s.filtered) > 0 && s.CursorIdx < len(s.filtered) {
			sub := subs[s.filtered[s.CursorIdx]]
			s.Active = false
			return sub, true
		}
	case bindings.Cancel.Matches(key):
		s.Active = false
	case bindings.Erase != nil && bindings.Erase.Matches(key):
		if len(s.Query) > 0 {
			s.Query = s.Query[:len(s.Query)-1]
			s.Refilter(subs)
		}
	default:
		if len(key) == 1 && key[0] >= 32 && key[0] < 127 {
			s.Query += key
			s.Refilter(subs)
		}
	}
	return azure.Subscription{}, false
}

// RenderSubscriptionOverlay renders the subscription picker overlay.
// If loading is true, a spinner frame is appended to the title.
func RenderSubscriptionOverlay(state SubscriptionOverlayState, closeHint string, subs []azure.Subscription, currentSub azure.Subscription, loading bool, loadingStartedAt time.Time, styles Styles, width, height int, base string) string {
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
			Desc:     sub.ID,
			IsActive: sub.ID == currentSub.ID && currentSub.ID != "",
		}
	}

	title := "Subscriptions"
	if loading {
		title += " " + SpinnerFrameAt(time.Since(loadingStartedAt))
	}

	cfg := OverlayListConfig{
		Title:      title,
		Query:      state.Query,
		CloseHint:  closeHint,
		MaxVisible: 12,
		Center:     true,
	}
	return RenderOverlayList(cfg, items, state.CursorIdx, styles.Overlay, width, height, base)
}

// RenderSubscriptionBar renders a 2-line bar showing the current subscription context.
func RenderSubscriptionBar(sub azure.Subscription, hasSub bool, styles Styles, width int) string {
	bg := styles.StatusBar.Box.GetBackground()
	barStyle := lipgloss.NewStyle().Background(bg).Width(width).Padding(0, 1)

	if hasSub {
		name := styles.Accent.Bold(true).Render(SubscriptionDisplayName(sub))
		id := styles.Muted.Render(sub.ID)
		return barStyle.Render(name + "\n" + id)
	}

	line1 := styles.Muted.Render("No subscription selected")
	line2 := styles.Muted.Render("Press S to choose")
	return barStyle.Render(line1 + "\n" + line2)
}
