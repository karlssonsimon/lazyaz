package sbapp

import (
	"fmt"

	"azure-storage/internal/ui"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "loading..."
	}

	styles := m.styles.Chrome

	var sbItems []ui.StatusBarItem
	if m.hasNamespace {
		sbItems = append(sbItems, ui.StatusBarItem{Label: "Namespace:", Value: m.currentNS.Name})
	}
	if m.hasEntity {
		sbItems = append(sbItems, ui.StatusBarItem{Label: "Entity:", Value: entityDisplayName(m.currentEntity)})
	}

	m.namespacesList.Title = m.namespacesPaneTitle()
	m.entitiesList.Title = m.entitiesPaneTitle()
	m.detailList.Title = m.detailPaneTitle()

	if m.deadLetter && m.detailMode == detailMessages {
		m.detailList.Styles.Title = m.styles.DangerBold.Padding(0, 1)
	} else {
		m.detailList.Styles.Title = m.styles.List.Title
	}

	ui.ClampListSelection(&m.namespacesList)
	ui.ClampListSelection(&m.entitiesList)
	ui.ClampListSelection(&m.detailList)

	pw := m.paneWidths

	pane := m.styles.Chrome.Pane
	km := m.keymap

	nsHints := ui.RenderPaneHints([]ui.PaneHint{
		{km.OpenFocusedAlt.Short(), "open"},
		{km.FilterInput.Short(), "filter"},
		{km.NextFocus.Short(), "next"},
		{km.SubscriptionPicker.Short(), "sub"},
	}, m.styles, ui.PaneContentWidth(pane, pw[0]))

	entHints := ui.RenderPaneHints([]ui.PaneHint{
		{km.OpenFocusedAlt.Short(), "open"},
		{km.NavigateLeft.Short(), "back"},
		{km.ToggleDLQFilter.Short(), "DLQ filter"},
	}, m.styles, ui.PaneContentWidth(pane, pw[1]))

	detHints := ui.RenderPaneHints([]ui.PaneHint{
		{km.ToggleMark.Short(), "mark"},
		{km.ShowActiveQueue.Short() + "/" + km.ShowDeadLetterQueue.Short(), "active/DLQ"},
		{km.RequeueDLQ.Short(), "requeue"},
	}, m.styles, ui.PaneContentWidth(pane, pw[2]))

	namespacesView := lipgloss.JoinVertical(lipgloss.Left, m.namespacesList.View(), nsHints)
	entitiesView := lipgloss.JoinVertical(lipgloss.Left, m.entitiesList.View(), entHints)
	detailView := lipgloss.JoinVertical(lipgloss.Left, m.detailList.View(), detHints)

	namespacesPaneStyle := styles.Pane.Copy().Width(pw[0])
	entitiesPaneStyle := styles.Pane.Copy().Width(pw[1])
	detailPaneStyle := styles.Pane.Copy().Width(pw[2])

	if m.focus == namespacesPane {
		namespacesPaneStyle = styles.FocusedPane.Copy().Width(pw[0])
	}
	if m.focus == entitiesPane {
		entitiesPaneStyle = styles.FocusedPane.Copy().Width(pw[1])
	}

	if m.deadLetter && m.detailMode == detailMessages {
		detailPaneStyle = styles.Pane.Copy().Width(pw[2]).BorderForeground(m.styles.Danger.GetForeground())
	} else if m.focus == detailPane && !m.viewingMessage {
		detailPaneStyle = styles.FocusedPane.Copy().Width(pw[2])
	}

	panesList := []string{
		namespacesPaneStyle.Render(namespacesView),
		entitiesPaneStyle.Render(entitiesView),
		detailPaneStyle.Render(detailView),
	}

	if m.viewingMessage {
		previewTitleStyle := m.styles.Accent.Copy().Padding(0, 1)
		msgID := ui.EmptyToDash(m.selectedMessage.MessageID)
		previewTitle := previewTitleStyle.Render(fmt.Sprintf("Message: %s", msgID))
		previewContent := lipgloss.JoinVertical(lipgloss.Left, previewTitle, m.messageViewport.View())

		previewPaneStyle := styles.FocusedPane.Copy().Width(pw[3])
		panesList = append(panesList, previewPaneStyle.Render(previewContent))
	}

	panes := lipgloss.JoinHorizontal(lipgloss.Top, panesList...)

	subBar := ui.RenderSubscriptionBar(m.currentSub, m.hasSubscription, m.styles, m.width)

	sbStatus := m.status
	sbErr := m.lastErr != ""
	if sbErr {
		sbStatus = m.lastErr
	} else if m.loading {
		sbStatus = m.spinner.View() + " " + m.status
	}
	statusBar := ui.RenderStatusBar(m.styles, sbItems, sbStatus, sbErr, m.width)

	parts := []string{subBar, panes, statusBar}

	view := ui.RenderCanvas(lipgloss.JoinVertical(lipgloss.Left, parts...), m.width, m.height, m.styles.Bg)

	if m.subOverlay.Active {
		view = ui.RenderSubscriptionOverlay(m.subOverlay, m.subscriptions, m.currentSub, m.styles, m.width, m.height, view)
	}
	if !m.EmbeddedMode && m.themeOverlay.Active {
		view = ui.RenderThemeOverlay(m.themeOverlay, m.schemes, m.styles, m.width, m.height, view)
	}
	if !m.EmbeddedMode && m.helpOverlay.Active {
		view = ui.RenderHelpOverlay(m.helpOverlay, m.styles, m.width, m.height, view)
	}

	return view
}

