package sbapp

import (
	"fmt"
	"strings"

	"github.com/karlssonsimon/lazyaz/internal/ui"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const (
	namespacesTitle    = "NAMESPACES"
	entitiesTitle      = "ENTITIES"
	subscriptionsTitle = "SUBSCRIPTIONS"
	queueTypeTitle     = "QUEUE"
	messagesTitle      = "MESSAGES"
	messageDetailTitle = "DETAILS"
)

func (m Model) View() tea.View {
	if m.Width == 0 || m.Height == 0 {
		v := tea.NewView("loading...")
		v.AltScreen = true
		return v
	}

	ui.ClampListSelection(&m.namespacesList)
	ui.ClampListSelection(&m.entitiesList)
	ui.ClampListSelection(&m.subscriptionsList)
	ui.ClampListSelection(&m.queueTypeList)
	ui.ClampListSelection(&m.messageList)

	pw := m.paneWidths
	h := m.paneHeight
	rightmostPane := m.focus
	if childIdx := m.childPane(); childIdx >= 0 && pw[childIdx] > 0 {
		rightmostPane = childIdx
	}
	frame := func(pane int) ui.MillerColumnFrame {
		return ui.MillerColumnFrame{Width: pw[pane], Height: h, Focused: m.focus == pane, RightRule: pane != rightmostPane}
	}
	footer := func(pane int, l *list.Model) string {
		f := frame(pane)
		contentWidth := ui.MillerContentWidth(f)
		base := m.columnFooter(pane)
		if inspect := m.inspectFooter(pane, contentWidth); inspect != "" {
			base = lipgloss.JoinVertical(lipgloss.Left, base, inspect)
		}
		if pane != m.focus || l == nil {
			return base
		}
		switch l.FilterState() {
		case list.Filtering:
			return ui.RenderFilterLine(l.FilterInput.Value(), m.Cursor.View(),
				m.Styles, contentWidth, true)
		case list.FilterApplied:
			return lipgloss.JoinVertical(lipgloss.Left,
				ui.RenderFilterLine(l.FilterValue(), "", m.Styles, contentWidth, false),
				base)
		}
		return base
	}

	// Build all pane renderings.
	paneMap := make(map[int]string)

	paneMap[namespacesPane] = ui.RenderMillerListColumn(ui.MillerListColumn{
		List:      &m.namespacesList,
		Title:     namespacesTitle,
		TitleMeta: m.columnTitleMeta(namespacesPane),
		Footer:    footer(namespacesPane, &m.namespacesList),
		Frame:     frame(namespacesPane),
	}, m.Styles)

	paneMap[entitiesPane] = ui.RenderMillerListColumn(ui.MillerListColumn{
		List:      &m.entitiesList,
		Title:     entitiesTitle,
		TitleMeta: m.columnTitleMeta(entitiesPane),
		Footer:    footer(entitiesPane, &m.entitiesList),
		Frame:     frame(entitiesPane),
	}, m.Styles)

	paneMap[subscriptionsPane] = ui.RenderMillerListColumn(ui.MillerListColumn{
		List:      &m.subscriptionsList,
		Title:     subscriptionsTitle,
		TitleMeta: m.columnTitleMeta(subscriptionsPane),
		Footer:    footer(subscriptionsPane, &m.subscriptionsList),
		Frame:     frame(subscriptionsPane),
	}, m.Styles)

	paneMap[queueTypePane] = ui.RenderMillerListColumn(ui.MillerListColumn{
		List:      &m.queueTypeList,
		Title:     queueTypeTitle,
		TitleMeta: m.columnTitleMeta(queueTypePane),
		Footer:    footer(queueTypePane, nil),
		Frame:     frame(queueTypePane),
	}, m.Styles)

	paneMap[messagesPane] = ui.RenderMillerListColumn(ui.MillerListColumn{
		List:      &m.messageList,
		Title:     messagesTitle,
		TitleMeta: m.columnTitleMeta(messagesPane),
		Footer:    footer(messagesPane, &m.messageList),
		Frame:     frame(messagesPane),
	}, m.Styles)

	// Message preview pane.
	if pw[messagePreviewPane] > 0 && m.viewingMessage {
		contentWidth := ui.MillerContentWidth(frame(messagePreviewPane))
		msgID := ui.EmptyToDash(m.selectedMessage.MessageID)
		titleText := fmt.Sprintf("Message: %s", msgID)
		previewTitle := m.Styles.Accent.Copy().
			Width(contentWidth).
			MaxWidth(contentWidth).
			Render(titleText)
		vpView := m.messageViewport.View()
		if m.textSelection.Active {
			vpView = m.textSelection.HighlightContent(m.messageViewport, m.Styles.SelectionHighlight)
		}
		gutter := ui.RenderLineGutter(m.messageViewport, m.Styles, previewGutterMinDigits)
		body := lipgloss.JoinHorizontal(lipgloss.Top, gutter, vpView)
		previewContent := lipgloss.JoinVertical(lipgloss.Left, previewTitle, body)
		paneMap[messagePreviewPane] = ui.RenderMillerColumn(ui.MillerColumn{
			Title: messageDetailTitle,
			Body:  previewContent,
			Frame: frame(messagePreviewPane),
		}, m.Styles)
	}

	// Assemble: parent, focused, child. All three slots are reserved
	// by the layout — empty slots fall back to spacers (with a right
	// rule on the parent slot so the focused column always borders
	// against a vertical line).
	plainSpacer := func(width int) string {
		if width <= 0 {
			return ""
		}
		return lipgloss.NewStyle().Width(width).Height(h).Render("")
	}
	paneParts := make([]string, 0, 3)

	parentIdx := m.parentPane()
	if parentIdx >= 0 && pw[parentIdx] > 0 {
		paneParts = append(paneParts, paneMap[parentIdx])
	}
	// Topmost focus skips the parent slot — focus expanded via
	// MillerLayout(hasParent=false).

	if rendered, ok := paneMap[m.focus]; ok {
		paneParts = append(paneParts, rendered)
	}

	if childIdx := m.childPane(); childIdx >= 0 {
		if rendered, ok := paneMap[childIdx]; ok && pw[childIdx] > 0 {
			paneParts = append(paneParts, rendered)
		} else if pw[childIdx] > 0 {
			paneParts = append(paneParts, plainSpacer(pw[childIdx]))
		}
	}

	cols := ui.MillerLayout(m.Styles.Chrome.Pane, m.Width, m.focus > namespacesPane, true)
	if margin := ui.MillerSideMargin(cols, m.Width); margin > 0 {
		paneParts = append([]string{plainSpacer(margin)}, paneParts...)
	}
	panes := lipgloss.JoinHorizontal(lipgloss.Top, paneParts...)

	header := ui.RenderAppHeader(ui.HeaderConfig{
		Brand: "lazyaz",
		Path:  m.headerPath(),
		Meta:  ui.HeaderMeta(m.CurrentSub, m.HasSubscription, m.Styles),
	}, m.Styles, m.Width)
	statusBar := ui.RenderStatusLine(ui.StatusLineConfig{
		Mode:    m.inputMode().String(),
		Actions: m.statusActions(),
	}, m.Styles, m.Width)
	ticks := m.columnTickPositions()
	topRule := ui.RenderHorizontalRule(m.Width, m.Styles, ticks)
	bottomRule := ui.RenderHorizontalRuleBottom(m.Width, m.Styles, ticks)
	view := ui.RenderCanvas(lipgloss.JoinVertical(lipgloss.Left, header, topRule, panes, bottomRule, statusBar), m.Width, m.Height, m.Styles.Bg)
	if m.entitySortOverlay.active {
		view = m.renderEntitySortOverlay(view)
	} else if m.targetPicker.active {
		view = m.renderTargetPicker(view)
	} else if m.actionMenu.Active {
		view = m.renderActionMenu(view)
	}
	out := tea.NewView(m.RenderOverlays(view))
	out.AltScreen = true
	out.MouseMode = tea.MouseModeCellMotion
	return out
}

func (m Model) headerPath() []string {
	// Tab bar already says "Service Bus" — breadcrumb starts at the
	// resource: sub → namespace → entity → subscription → queue type.
	var path []string
	if m.HasSubscription {
		path = append(path, ui.SubscriptionDisplayName(m.CurrentSub))
	}
	if m.hasNamespace {
		path = append(path, m.currentNS.Name)
	}
	if m.currentEntity.Name != "" {
		path = append(path, m.currentEntity.Name)
	}
	if m.currentSubName != "" {
		path = append(path, m.currentSubName)
	}
	if m.hasPeekTarget {
		if m.deadLetter {
			path = append(path, "DLQ")
		} else {
			path = append(path, "Active")
		}
	}
	return path
}

// columnTickPositions reports x-coords where vertical column rules
// live so the app-level horizontal rules can place ┬/┴ tees there.
func (m Model) columnTickPositions() []int {
	pw := m.paneWidths
	parentIdx := m.parentPane()
	childIdx := m.childPane()
	parentVisible := parentIdx >= 0 && pw[parentIdx] > 0
	hasChild := childIdx >= 0 && pw[childIdx] > 0

	cols := ui.MillerLayout(m.Styles.Chrome.Pane, m.Width, m.focus > namespacesPane, true)
	pos := ui.MillerSideMargin(cols, m.Width)
	var ticks []int
	if parentVisible {
		pos += pw[parentIdx]
		ticks = append(ticks, pos-1)
	}
	pos += pw[m.focus]
	if hasChild {
		ticks = append(ticks, pos-1)
	}
	return ticks
}

func (m Model) statusActions() []ui.StatusAction {
	km := m.Keymap
	actions := []ui.StatusAction{
		{Key: km.CursorDown.Short() + "/" + km.CursorUp.Short(), Label: "move"},
		{Key: km.OpenFocusedAlt.Short(), Label: "open"},
		{Key: km.NavigateLeft.Short(), Label: "back"},
		{Key: km.FilterInput.Short(), Label: "filter"},
		{Key: km.RefreshScope.Short(), Label: "refresh"},
		{Key: km.ToggleHelp.Short(), Label: "help"},
	}
	if m.focus == entitiesPane || m.focus == queueTypePane || m.focus == messagesPane {
		actions = append(actions, ui.StatusAction{Key: km.ActionMenu.Short(), Label: "actions"})
	}
	if m.focus == messagesPane {
		actions = append(actions, ui.StatusAction{Key: km.ToggleMark.Short(), Label: "mark"})
	}
	return actions
}

func (m Model) columnFooter(pane int) string {
	l := m.listForPane(pane)
	if l == nil {
		return ""
	}
	idx := l.Index() + 1
	count := len(l.VisibleItems())
	if count == 0 {
		idx = 0
	}
	return fmt.Sprintf("%d of %d · ↕ j/k", idx, count)
}

// columnTitleMeta is the right-aligned summary on the column title row.
// The cursor index sits in the footer; the title meta carries totals
// and (where relevant) a marked count so it stays stable while scrolling.
func (m Model) columnTitleMeta(pane int) string {
	switch pane {
	case namespacesPane:
		if total := len(m.namespaces); total > 0 {
			return fmt.Sprintf("%d total", total)
		}
	case entitiesPane:
		shown := len(m.entitiesList.VisibleItems())
		total := len(m.entities)
		if total == 0 {
			return ""
		}
		if shown != total {
			return fmt.Sprintf("%d / %d", shown, total)
		}
		return fmt.Sprintf("%d total", total)
	case subscriptionsPane:
		if total := len(m.subscriptions); total > 0 {
			return fmt.Sprintf("%d total", total)
		}
	case messagesPane:
		shown := len(m.messageList.VisibleItems())
		total := len(m.peekedMessages)
		if total == 0 {
			return ""
		}
		parts := []string{fmt.Sprintf("%d shown", shown)}
		if shown != total {
			parts[0] = fmt.Sprintf("%d / %d", shown, total)
		}
		marked := 0
		for _, ids := range m.markedMessages {
			marked += len(ids)
		}
		if marked > 0 {
			parts = append(parts, fmt.Sprintf("%d marked", marked))
		}
		return strings.Join(parts, " · ")
	}
	return ""
}
