package ui

import (
	"github.com/karlssonsimon/lazyaz/internal/fuzzy"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
)

// ListFilter is a bubbles list FilterFunc backed by the shared fzf
// matcher, so pane lists understand the same extended-search syntax as
// the overlays ('exact, ^prefix, suffix$, !negation, AND, |).
// MatchedIndexes are rune positions, ready for lipgloss.StyleRunes.
func ListFilter(term string, targets []string) []list.Rank {
	ranked := fuzzy.Ranks(term, targets)
	out := make([]list.Rank, len(ranked))
	for i, r := range ranked {
		out[i] = list.Rank{Index: r.Index, MatchedIndexes: r.Pos}
	}
	return out
}

// SyncFilter re-runs the list filter synchronously for the current
// input, keeping the Filtering (typing) state. Bubbles' own pipeline
// filters on a background goroutine per keystroke and applies results
// last-writer-wins, so a stale keystroke's result can arrive after a
// newer one and stick. Apps route input through UpdateListSyncFilter
// and drop list.FilterMatchesMsg messages entirely.
func SyncFilter(l *list.Model) {
	if l.FilterState() != list.Filtering {
		return
	}
	l.SetFilterText(l.FilterInput.Value())
	l.SetFilterState(list.Filtering)
}

// UpdateListSyncFilter routes msg to the list and refilters
// synchronously when the message changed the filter input — whatever
// kind of message it was (keystroke, paste, ...). See SyncFilter.
func UpdateListSyncFilter(l *list.Model, msg tea.Msg) tea.Cmd {
	before := l.FilterInput.Value()
	updated, cmd := l.Update(msg)
	*l = updated
	if updated.FilterInput.Value() != before {
		SyncFilter(l)
		// On an input change, the cmd bubbles returned carries its own
		// async filter pass (redundant — the line above just filtered)
		// and a blink for its filter input (never rendered; every list
		// runs with SetShowFilter(false)). Drop it: at large item
		// counts the background pass is a full re-filter for nothing.
		return nil
	}
	return cmd
}
