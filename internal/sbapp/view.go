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

	styles := ui.NewChromeStyles(m.palette)

	subscriptionName := "-"
	namespaceName := "-"
	entityName := "-"
	if m.hasSubscription {
		subscriptionName = subscriptionDisplayName(m.currentSub)
	}
	if m.hasNamespace {
		namespaceName = m.currentNS.Name
	}
	if m.hasEntity {
		entityName = entityDisplayName(m.currentEntity)
	}

	header := styles.Header.Width(m.width).Render(ui.TrimToWidth("Azure Service Bus Explorer", m.width-2))
	headerMeta := styles.Meta.Width(m.width).Render(ui.TrimToWidth(fmt.Sprintf("Subscription: %s | Namespace: %s | Entity: %s", subscriptionName, namespaceName, entityName), m.width-2))

	m.subscriptionsList.Title = m.subscriptionsPaneTitle()
	m.namespacesList.Title = m.namespacesPaneTitle()
	m.entitiesList.Title = m.entitiesPaneTitle()
	m.detailList.Title = m.detailPaneTitle()

	if m.deadLetter && m.detailMode == detailMessages {
		m.detailList.Styles.Title = m.detailList.Styles.Title.
			Foreground(lipgloss.Color(m.palette.Danger))
	} else {
		m.detailList.Styles.Title = m.detailList.Styles.Title.
			Foreground(lipgloss.Color(m.palette.Accent))
	}

	subscriptionsView := m.subscriptionsList.View()
	namespacesView := m.namespacesList.View()
	entitiesView := m.entitiesList.View()
	detailView := m.detailList.View()

	subscriptionsPaneStyle := styles.Pane.Copy().MarginRight(1)
	namespacesPaneStyle := styles.Pane.Copy().MarginRight(1)
	entitiesPaneStyle := styles.Pane.Copy().MarginRight(1)
	detailPaneStyle := styles.Pane.Copy()

	if m.focus == subscriptionsPane {
		subscriptionsPaneStyle = styles.FocusedPane.Copy().MarginRight(1)
	}
	if m.focus == namespacesPane {
		namespacesPaneStyle = styles.FocusedPane.Copy().MarginRight(1)
	}
	if m.focus == entitiesPane {
		entitiesPaneStyle = styles.FocusedPane.Copy().MarginRight(1)
	}

	if m.deadLetter && m.detailMode == detailMessages {
		detailPaneStyle = styles.Pane.Copy().BorderForeground(lipgloss.Color(m.palette.Danger))
	} else if m.focus == detailPane && !m.viewingMessage {
		detailPaneStyle = styles.FocusedPane.Copy()
	}
	if m.viewingMessage {
		detailPaneStyle = detailPaneStyle.Copy().MarginRight(1)
	}

	panesList := []string{
		subscriptionsPaneStyle.Render(subscriptionsView),
		namespacesPaneStyle.Render(namespacesView),
		entitiesPaneStyle.Render(entitiesView),
		detailPaneStyle.Render(detailView),
	}

	if m.viewingMessage {
		previewTitleStyle := lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(m.palette.Accent)).
			Padding(0, 1)
		msgID := ui.EmptyToDash(m.selectedMessage.MessageID)
		previewTitle := previewTitleStyle.Render(fmt.Sprintf("Message: %s", msgID))
		previewContent := lipgloss.JoinVertical(lipgloss.Left, previewTitle, m.messageViewport.View())

		previewPaneStyle := styles.FocusedPane.Copy()
		panesList = append(panesList, previewPaneStyle.Render(previewContent))
	}

	panes := lipgloss.JoinHorizontal(lipgloss.Top, panesList...)

	filterHint := "Press / to filter the focused pane (fzf-style live filter)."
	if m.focusedListSettingFilter() {
		filterHint = fmt.Sprintf("Filtering %s: type to narrow, up/down to move, Enter applies filter.", paneName(m.focus))
	}
	filterLine := styles.FilterHint.Width(m.width).Render(ui.TrimToWidth(filterHint, m.width-2))

	errorLine := ""
	if m.lastErr != "" {
		errorLine = styles.Error.Width(m.width).Render(ui.TrimToWidth("Error: "+m.lastErr, m.width-2))
	}

	statusText := m.status
	if m.loading {
		statusText = fmt.Sprintf("%s %s", m.spinner.View(), m.status)
	}
	statusLine := styles.Status.Width(m.width).Render(ui.TrimToWidth(statusText, m.width-2))

	helpLine := styles.Help.Width(m.width).Render(ui.TrimToWidth(m.keymap.FooterHelpText(), m.width-2))

	parts := []string{header, headerMeta, panes, filterLine}
	if errorLine != "" {
		parts = append(parts, errorLine)
	}
	parts = append(parts, statusLine, helpLine)

	view := lipgloss.JoinVertical(lipgloss.Left, parts...)

	if m.themeOverlay.Active {
		view = ui.RenderThemeOverlay(m.themeOverlay, m.themes, m.palette, m.width, m.height, view)
	}
	if m.helpOverlay.Active {
		view = ui.RenderHelpOverlay("Azure Service Bus Explorer Help", m.keymap.HelpSections(), m.palette, m.width, m.height, view)
	}

	return view
}
