package sbapp

import (
	"fmt"
	"time"

	"azure-storage/internal/ui"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) View() string {
	if m.Width == 0 || m.Height == 0 {
		return "loading..."
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
		List:     &m.namespacesList,
		Title:    m.namespacesPaneTitle(),
		Loading:  m.Loading && m.LoadingPane == namespacesPane,
		LoadedAt: m.LoadingStartedAt,
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
		List:     &m.entitiesList,
		Title:    m.entitiesPaneTitle(),
		Loading:  m.Loading && m.LoadingPane == entitiesPane,
		LoadedAt: m.LoadingStartedAt,
		Hints: []ui.PaneHint{
			{Key: km.OpenFocusedAlt.Short(), Desc: "open"},
			{Key: km.NavigateLeft.Short(), Desc: "back"},
			{Key: "[/]", Desc: "type"},
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
		List:     &m.detailList,
		Title:    m.detailPaneTitle(),
		Loading:  m.Loading && m.LoadingPane == detailPane,
		LoadedAt: m.LoadingStartedAt,
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

	panesList := []string{namespaces, entities, detail}

	if m.viewingMessage {
		previewTitleStyle := m.Styles.Accent.Copy().Padding(0, 1)
		msgID := ui.EmptyToDash(m.selectedMessage.MessageID)
		previewTitle := previewTitleStyle.Render(fmt.Sprintf("Message: %s", msgID))
		previewContent := lipgloss.JoinVertical(lipgloss.Left, previewTitle, m.messageViewport.View())
		preview := ui.RenderPane(previewContent, ui.PaneFrame{Width: pw[3], Height: h, Focused: m.focus == messagePreviewPane}, m.Styles)
		panesList = append(panesList, preview)
	}

	panes := lipgloss.JoinHorizontal(lipgloss.Top, panesList...)

	subBar := ui.RenderSubscriptionBar(m.CurrentSub, m.HasSubscription, m.Styles, m.Width)

	sbStatus := m.Status
	sbErr := m.LastErr != ""
	if sbErr {
		sbStatus = m.LastErr
	} else if m.Loading {
		sbStatus = ui.SpinnerFrameAt(time.Since(m.LoadingStartedAt)) + " " + m.Status
	}
	statusBar := ui.RenderStatusBar(m.Styles, sbItems, sbStatus, sbErr, m.Width)

	view := ui.RenderCanvas(lipgloss.JoinVertical(lipgloss.Left, subBar, panes, statusBar), m.Width, m.Height, m.Styles.Bg)
	return m.RenderOverlays(view)
}
