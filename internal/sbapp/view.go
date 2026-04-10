package sbapp

import (
	"fmt"

	"github.com/karlssonsimon/lazyaz/internal/ui"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func (m Model) View() tea.View {
	if m.Width == 0 || m.Height == 0 {
		v := tea.NewView("loading...")
		v.AltScreen = true
		return v
	}

	var sbItems []ui.StatusBarItem
	if m.hasNamespace {
		sbItems = append(sbItems, ui.StatusBarItem{Label: "Namespace:", Value: m.currentNS.Name})
	}
	if m.hasPeekTarget {
		label := m.currentEntity.Name
		if m.currentSubName != "" {
			label += "/" + m.currentSubName
		}
		sbItems = append(sbItems, ui.StatusBarItem{Label: "Peeking:", Value: label})
	}

	ui.ClampListSelection(&m.namespacesList)
	ui.ClampListSelection(&m.entitiesList)
	ui.ClampListSelection(&m.subscriptionsList)
	ui.ClampListSelection(&m.queueTypeList)
	ui.ClampListSelection(&m.messageList)

	pw := m.paneWidths
	h := m.paneHeight
	km := m.Keymap
	paneStyle := m.Styles.Chrome.Pane

	// Build all pane renderings.
	paneMap := make(map[int]string)

	paneMap[namespacesPane] = ui.RenderListPane(ui.ListPane{
		List:  &m.namespacesList,
		Title: m.namespacesPaneTitle(),
		Hints: []ui.PaneHint{
			{Key: km.OpenFocusedAlt.Short(), Desc: "open"},
			{Key: km.FilterInput.Short(), Desc: "filter"},
			{Key: km.NextFocus.Short(), Desc: "next"},
			{Key: km.SubscriptionPicker.Short(), Desc: "sub"},
			{Key: km.Inspect.Short(), Desc: "inspect"},
		},
		Footer: m.inspectFooter(namespacesPane, ui.PaneContentWidth(paneStyle, pw[namespacesPane])),
		Frame:  ui.PaneFrame{Width: pw[namespacesPane], Height: h, Focused: m.focus == namespacesPane},
	}, m.Styles)

	paneMap[entitiesPane] = ui.RenderListPane(ui.ListPane{
		List:  &m.entitiesList,
		Title: m.entitiesPaneTitle(),
		Hints: []ui.PaneHint{
			{Key: km.OpenFocusedAlt.Short(), Desc: "open"},
			{Key: km.NavigateLeft.Short(), Desc: "back"},
			{Key: km.ToggleDLQFilter.Short(), Desc: "DLQ-first"},
		},
		Footer: m.inspectFooter(entitiesPane, ui.PaneContentWidth(paneStyle, pw[entitiesPane])),
		Frame:  ui.PaneFrame{Width: pw[entitiesPane], Height: h, Focused: m.focus == entitiesPane},
	}, m.Styles)

	paneMap[subscriptionsPane] = ui.RenderListPane(ui.ListPane{
		List:  &m.subscriptionsList,
		Title: m.subscriptionsPaneTitle(),
		Hints: []ui.PaneHint{
			{Key: km.OpenFocusedAlt.Short(), Desc: "open"},
			{Key: km.NavigateLeft.Short(), Desc: "back"},
		},
		Footer: m.inspectFooter(subscriptionsPane, ui.PaneContentWidth(paneStyle, pw[subscriptionsPane])),
		Frame:  ui.PaneFrame{Width: pw[subscriptionsPane], Height: h, Focused: m.focus == subscriptionsPane},
	}, m.Styles)

	// Queue type pane — DLQ item gets danger styling.
	queueTypeLp := ui.ListPane{
		List:  &m.queueTypeList,
		Title: m.queueTypePaneTitle(),
		Hints: []ui.PaneHint{
			{Key: km.OpenFocusedAlt.Short(), Desc: "open"},
			{Key: km.NavigateLeft.Short(), Desc: "back"},
		},
		Frame: ui.PaneFrame{Width: pw[queueTypePane], Height: h, Focused: m.focus == queueTypePane},
	}
	paneMap[queueTypePane] = ui.RenderListPane(queueTypeLp, m.Styles)

	// Messages pane — danger border when viewing DLQ.
	messagesLp := ui.ListPane{
		List:  &m.messageList,
		Title: m.messagesPaneTitle(),
		Hints: []ui.PaneHint{
			{Key: km.ActionMenu.Short(), Desc: "actions"},
			{Key: km.NavigateLeft.Short(), Desc: "back"},
		},
		Footer: m.inspectFooter(messagesPane, ui.PaneContentWidth(paneStyle, pw[messagesPane])),
		Frame:  ui.PaneFrame{Width: pw[messagesPane], Height: h, Focused: m.focus == messagesPane},
	}
	if m.deadLetter {
		dangerFrame := paneStyle.Copy().BorderForeground(m.Styles.Danger.GetForeground())
		messagesLp.FrameStyle = &dangerFrame
	}
	paneMap[messagesPane] = ui.RenderListPane(messagesLp, m.Styles)

	// Message preview pane.
	if pw[messagePreviewPane] > 0 && m.viewingMessage {
		contentWidth := ui.PaneContentWidth(paneStyle, pw[messagePreviewPane])
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
		previewContent := lipgloss.JoinVertical(lipgloss.Left, previewTitle, vpView)
		paneMap[messagePreviewPane] = ui.RenderPane(previewContent, ui.PaneFrame{Width: pw[messagePreviewPane], Height: h, Focused: m.focus == messagePreviewPane}, m.Styles)
	}

	// Assemble: parent, focused, child.
	parentWidth := m.Width * 20 / 100
	paneParts := make([]string, 0, 3)

	parentIdx := m.parentPane()
	if parentIdx >= 0 && pw[parentIdx] > 0 {
		paneParts = append(paneParts, paneMap[parentIdx])
	} else if m.focus == namespacesPane {
		spacer := lipgloss.NewStyle().Width(parentWidth).Height(h).Render("")
		paneParts = append(paneParts, spacer)
	}

	if rendered, ok := paneMap[m.focus]; ok {
		paneParts = append(paneParts, rendered)
	}

	childIdx := m.childPane()
	if childIdx >= 0 && pw[childIdx] > 0 {
		if rendered, ok := paneMap[childIdx]; ok {
			paneParts = append(paneParts, rendered)
		}
	}

	panes := lipgloss.JoinHorizontal(lipgloss.Top, paneParts...)

	subBar := ui.RenderSubscriptionBar(m.CurrentSub, m.HasSubscription, m.Styles, m.Width)
	statusBar := ui.RenderStatusBar(m.Styles, sbItems, "", false, m.Width)

	view := ui.RenderCanvas(lipgloss.JoinVertical(lipgloss.Left, subBar, panes, statusBar), m.Width, m.Height, m.Styles.Bg)
	if m.actionMenu.active {
		view = m.renderActionMenu(view)
	}
	out := tea.NewView(m.RenderOverlays(view))
	out.AltScreen = true
	out.MouseMode = tea.MouseModeCellMotion
	return out
}
