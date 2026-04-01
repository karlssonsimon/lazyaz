package kvapp

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

	subscriptionName := "-"
	vaultName := "-"
	secretName := "-"
	if m.hasSubscription {
		subscriptionName = subscriptionDisplayName(m.currentSub)
	}
	if m.hasVault {
		vaultName = m.currentVault.Name
	}
	if m.hasSecret {
		secretName = m.currentSecret.Name
	}

	header := styles.Header.Width(m.width).Render(ui.TrimToWidth("Azure Key Vault Explorer", m.width-2))
	headerMeta := styles.Meta.Width(m.width).Render(ui.TrimToWidth(fmt.Sprintf("Subscription: %s | Vault: %s | Secret: %s", subscriptionName, vaultName, secretName), m.width-2))

	m.subscriptionsList.Title = m.subscriptionsPaneTitle()
	m.vaultsList.Title = m.vaultsPaneTitle()
	m.secretsList.Title = m.secretsPaneTitle()
	m.versionsList.Title = m.versionsPaneTitle()

	pw := m.paneWidths

	subscriptionsView := m.subscriptionsList.View()
	vaultsView := m.vaultsList.View()
	secretsView := m.secretsList.View()
	versionsView := m.versionsList.View()

	subscriptionsPaneStyle := styles.Pane.Copy().Width(pw[0])
	vaultsPaneStyle := styles.Pane.Copy().Width(pw[1])
	secretsPaneStyle := styles.Pane.Copy().Width(pw[2])
	versionsPaneStyle := styles.Pane.Copy().Width(pw[3])

	if m.focus == subscriptionsPane {
		subscriptionsPaneStyle = styles.FocusedPane.Copy().Width(pw[0])
	}
	if m.focus == vaultsPane {
		vaultsPaneStyle = styles.FocusedPane.Copy().Width(pw[1])
	}
	if m.focus == secretsPane {
		secretsPaneStyle = styles.FocusedPane.Copy().Width(pw[2])
	}
	if m.focus == versionsPane {
		versionsPaneStyle = styles.FocusedPane.Copy().Width(pw[3])
	}

	panes := lipgloss.JoinHorizontal(lipgloss.Top,
		subscriptionsPaneStyle.Render(subscriptionsView),
		vaultsPaneStyle.Render(vaultsView),
		secretsPaneStyle.Render(secretsView),
		versionsPaneStyle.Render(versionsView),
	)

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

	view := ui.RenderCanvas(lipgloss.JoinVertical(lipgloss.Left, parts...), m.width, m.height, m.styles.Bg)

	if !m.EmbeddedMode && m.themeOverlay.Active {
		view = ui.RenderThemeOverlay(m.themeOverlay, m.schemes, m.styles, m.width, m.height, view)
	}
	if !m.EmbeddedMode && m.helpOverlay.Active {
		view = ui.RenderHelpOverlay("Azure Key Vault Explorer Help", m.keymap.HelpSections(), m.styles, m.width, m.height, view)
	}

	return view
}
