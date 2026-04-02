package ui

import (
	"azure-storage/internal/azure"
	"azure-storage/internal/fuzzy"

	"github.com/charmbracelet/lipgloss"
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

func (s *SubscriptionOverlayState) Open() {
	s.Active = true
	s.Query = ""
	s.CursorIdx = 0
	s.filtered = nil
}

func (s *SubscriptionOverlayState) Close() {
	s.Active = false
}

func (s *SubscriptionOverlayState) refilter(subs []azure.Subscription) {
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
		s.refilter(subs)
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
	case key == "backspace":
		if len(s.Query) > 0 {
			s.Query = s.Query[:len(s.Query)-1]
			s.refilter(subs)
		}
	default:
		if len(key) == 1 && key[0] >= 32 && key[0] < 127 {
			s.Query += key
			s.refilter(subs)
		}
	}
	return azure.Subscription{}, false
}

// RenderSubscriptionOverlay renders the subscription picker overlay.
func RenderSubscriptionOverlay(state SubscriptionOverlayState, subs []azure.Subscription, currentSub azure.Subscription, styles Styles, width, height int, base string) string {
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

	cfg := OverlayListConfig{
		Title: "Subscriptions",
		Query: state.Query,
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
