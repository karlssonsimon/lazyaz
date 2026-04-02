package kvapp

import (
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
	if m.hasVault {
		sbItems = append(sbItems, ui.StatusBarItem{Label: "Vault:", Value: m.currentVault.Name})
	}
	if m.hasSecret {
		sbItems = append(sbItems, ui.StatusBarItem{Label: "Secret:", Value: m.currentSecret.Name})
	}

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
		view = ui.RenderHelpOverlay("Azure Key Vault Explorer Help", m.keymap.HelpSections(), m.styles, m.width, m.height, view)
	}

	return view
}

