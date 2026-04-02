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
	if m.hasSubscription {
		sbItems = append(sbItems, ui.StatusBarItem{Label: "Subscription:", Value: subscriptionDisplayName(m.currentSub)})
	}
	if m.hasNamespace {
		sbItems = append(sbItems, ui.StatusBarItem{Label: "Namespace:", Value: m.currentNS.Name})
	}
	if m.hasEntity {
		sbItems = append(sbItems, ui.StatusBarItem{Label: "Entity:", Value: entityDisplayName(m.currentEntity)})
	}

	m.subscriptionsList.Title = m.subscriptionsPaneTitle()
	m.namespacesList.Title = m.namespacesPaneTitle()
	m.entitiesList.Title = m.entitiesPaneTitle()
	m.detailList.Title = m.detailPaneTitle()

	if m.deadLetter && m.detailMode == detailMessages {
		m.detailList.Styles.Title = m.styles.DangerBold.Padding(0, 1)
	} else {
		m.detailList.Styles.Title = m.styles.List.Title
	}

	pw := m.paneWidths

	subscriptionsView := m.subscriptionsList.View()
	namespacesView := m.namespacesList.View()
	entitiesView := m.entitiesList.View()
	detailView := m.detailList.View()

	subscriptionsPaneStyle := styles.Pane.Copy().Width(pw[0])
	namespacesPaneStyle := styles.Pane.Copy().Width(pw[1])
	entitiesPaneStyle := styles.Pane.Copy().Width(pw[2])
	detailPaneStyle := styles.Pane.Copy().Width(pw[3])

	if m.focus == subscriptionsPane {
		subscriptionsPaneStyle = styles.FocusedPane.Copy().Width(pw[0])
	}
	if m.focus == namespacesPane {
		namespacesPaneStyle = styles.FocusedPane.Copy().Width(pw[1])
	}
	if m.focus == entitiesPane {
		entitiesPaneStyle = styles.FocusedPane.Copy().Width(pw[2])
	}

	if m.deadLetter && m.detailMode == detailMessages {
		detailPaneStyle = styles.Pane.Copy().Width(pw[3]).BorderForeground(m.styles.Danger.GetForeground())
	} else if m.focus == detailPane && !m.viewingMessage {
		detailPaneStyle = styles.FocusedPane.Copy().Width(pw[3])
	}

	panesList := []string{
		subscriptionsPaneStyle.Render(subscriptionsView),
		namespacesPaneStyle.Render(namespacesView),
		entitiesPaneStyle.Render(entitiesView),
		detailPaneStyle.Render(detailView),
	}

	if m.viewingMessage {
		previewTitleStyle := m.styles.Accent.Copy().Padding(0, 1)
		msgID := ui.EmptyToDash(m.selectedMessage.MessageID)
		previewTitle := previewTitleStyle.Render(fmt.Sprintf("Message: %s", msgID))
		previewContent := lipgloss.JoinVertical(lipgloss.Left, previewTitle, m.messageViewport.View())

		previewPaneStyle := styles.FocusedPane.Copy().Width(pw[4])
		panesList = append(panesList, previewPaneStyle.Render(previewContent))
	}

	panes := lipgloss.JoinHorizontal(lipgloss.Top, panesList...)

	sbStatus := m.status
	sbErr := m.lastErr != ""
	if sbErr {
		sbStatus = m.lastErr
	} else if m.loading {
		sbStatus = m.spinner.View() + " " + m.status
	}
	statusBar := ui.RenderStatusBar(m.styles, sbItems, sbStatus, sbErr, m.width)

	parts := []string{panes, statusBar}

	view := ui.RenderCanvas(lipgloss.JoinVertical(lipgloss.Left, parts...), m.width, m.height, m.styles.Bg)

	if !m.EmbeddedMode && m.themeOverlay.Active {
		view = ui.RenderThemeOverlay(m.themeOverlay, m.schemes, m.styles, m.width, m.height, view)
	}
	if !m.EmbeddedMode && m.helpOverlay.Active {
		view = ui.RenderHelpOverlay(m.helpOverlay, m.styles, m.width, m.height, view)
	}

	return view
}

