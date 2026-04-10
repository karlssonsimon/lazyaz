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
		label := entityDisplayName(m.currentEntity)
		if m.currentSubName != "" {
			label += "/" + m.currentSubName
		}
		sbItems = append(sbItems, ui.StatusBarItem{Label: "Peeking:", Value: label})
	}

	ui.ClampListSelection(&m.namespacesList)
	ui.ClampListSelection(&m.entitiesList)
	ui.ClampListSelection(&m.detailList)

	pw := m.paneWidths
	h := m.paneHeight
	km := m.Keymap
	paneStyle := m.Styles.Chrome.Pane

	namespaces := ui.RenderListPane(ui.ListPane{
		List:  &m.namespacesList,
		Title: m.namespacesPaneTitle(),
		Hints: []ui.PaneHint{
			{Key: km.OpenFocusedAlt.Short(), Desc: "open"},
			{Key: km.FilterInput.Short(), Desc: "filter"},
			{Key: km.NextFocus.Short(), Desc: "next"},
			{Key: km.SubscriptionPicker.Short(), Desc: "sub"},
			{Key: km.Inspect.Short(), Desc: "inspect"},
		},
		Footer: m.inspectFooter(namespacesPane, ui.PaneContentWidth(paneStyle, pw[0])),
		Frame:  ui.PaneFrame{Width: pw[0], Height: h, Focused: m.focus == namespacesPane},
	}, m.Styles)

	entitiesContentWidth := ui.PaneContentWidth(paneStyle, pw[1])
	entities := ui.RenderListPane(ui.ListPane{
		List:  &m.entitiesList,
		Title: m.entitiesPaneTitle(),
		Hints: []ui.PaneHint{
			{Key: km.OpenFocusedAlt.Short(), Desc: "open"},
			{Key: km.NavigateLeft.Short(), Desc: "back"},
			{Key: km.ShowActiveQueue.Short() + "/" + km.ShowDeadLetterQueue.Short(), Desc: "type"},
			{Key: km.ToggleDLQFilter.Short(), Desc: "DLQ-first"},
		},
		Header: m.renderEntityTabs(entitiesContentWidth),
		Footer: m.inspectFooter(entitiesPane, entitiesContentWidth),
		Frame:  ui.PaneFrame{Width: pw[1], Height: h, Focused: m.focus == entitiesPane},
	}, m.Styles)

	// Detail pane: when showing DLQ messages, paint the title and border
	// with the danger color regardless of focus, so the user has a loud
	// visual reminder they're operating on dead-lettered data.
	detailPaneListPane := ui.ListPane{
		List:  &m.detailList,
		Title: m.detailPaneTitle(),
		Hints: []ui.PaneHint{
			{Key: km.ToggleMark.Short(), Desc: "mark"},
			{Key: km.ShowActiveQueue.Short() + "/" + km.ShowDeadLetterQueue.Short(), Desc: "active/DLQ"},
			{Key: km.RequeueDLQ.Short(), Desc: "requeue"},
		},
		Footer: m.inspectFooter(detailPane, ui.PaneContentWidth(paneStyle, pw[2])),
		Frame:  ui.PaneFrame{Width: pw[2], Height: h, Focused: m.focus == detailPane},
	}
	if m.hasPeekTarget {
		detailPaneListPane.Header = m.renderDLQTabs(ui.PaneContentWidth(paneStyle, pw[2]))
	}
	if m.deadLetter && m.hasPeekTarget {
		dangerTitle := m.Styles.DangerBold.Padding(0, 1)
		dangerFrame := m.Styles.Chrome.Pane.Copy().BorderForeground(m.Styles.Danger.GetForeground())
		detailPaneListPane.TitleStyle = &dangerTitle
		detailPaneListPane.FrameStyle = &dangerFrame
	}
	detail := ui.RenderListPane(detailPaneListPane, m.Styles)

	// Build pane map for lookup by index.
	paneMap := map[int]string{
		namespacesPane: namespaces,
		entitiesPane:   entities,
		detailPane:     detail,
	}

	// Render preview pane if it has a width assigned.
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

	// Assemble panes in visual order: parent (left), focused (center), child (right).
	// When there's no parent pane, add an empty spacer to keep the focused pane centered.
	parentWidth := m.Width * 20 / 100
	paneParts := make([]string, 0, 3)
	if m.focus > namespacesPane && pw[m.focus-1] > 0 {
		paneParts = append(paneParts, paneMap[m.focus-1])
	} else if m.focus == namespacesPane {
		spacer := lipgloss.NewStyle().Width(parentWidth).Height(h).Render("")
		paneParts = append(paneParts, spacer)
	}
	paneParts = append(paneParts, paneMap[m.focus])

	// Child column (right side).
	childIdx := m.focus + 1
	if m.focus == detailPane {
		childIdx = messagePreviewPane
	}
	if childIdx <= messagePreviewPane && pw[childIdx] > 0 {
		if rendered, ok := paneMap[childIdx]; ok {
			paneParts = append(paneParts, rendered)
		}
	}

	panes := lipgloss.JoinHorizontal(lipgloss.Top, paneParts...)

	subBar := ui.RenderSubscriptionBar(m.CurrentSub, m.HasSubscription, m.Styles, m.Width)

	statusBar := ui.RenderStatusBar(m.Styles, sbItems, "", false, m.Width)

	view := ui.RenderCanvas(lipgloss.JoinVertical(lipgloss.Left, subBar, panes, statusBar), m.Width, m.Height, m.Styles.Bg)
	out := tea.NewView(m.RenderOverlays(view))
	out.AltScreen = true
	out.MouseMode = tea.MouseModeCellMotion
	return out
}
