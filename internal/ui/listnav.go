package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/list"
)

func ApplyFilterState(l *list.Model) {
	if strings.TrimSpace(l.FilterValue()) == "" {
		l.SetFilterState(list.Unfiltered)
		return
	}
	l.SetFilterState(list.FilterApplied)
}

func HalfPageStep(l list.Model) int {
	if l.Paginator.PerPage > 1 {
		if half := l.Paginator.PerPage / 2; half > 0 {
			return half
		}
	}

	if visible := len(l.VisibleItems()); visible > 1 {
		if half := visible / 2; half > 0 {
			return half
		}
	}

	return 1
}

func ClampListSelection(l *list.Model) {
	items := l.Items()
	if len(items) == 0 {
		l.Select(0)
		return
	}

	idx := l.Index()
	if idx < 0 {
		l.Select(0)
		return
	}
	if idx >= len(items) {
		l.Select(len(items) - 1)
	}
}
