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
	if m.hasVault {
		sbItems = append(sbItems, ui.StatusBarItem{Label: "Vault:", Value: m.currentVault.Name})
	}
	if m.hasSecret {
		sbItems = append(sbItems, ui.StatusBarItem{Label: "Secret:", Value: m.currentSecret.Name})
	}

	m.vaultsList.Title = m.vaultsPaneTitle()
	m.secretsList.Title = m.secretsPaneTitle()
	m.versionsList.Title = m.versionsPaneTitle()

	pw := m.paneWidths

	vaultsView := m.vaultsList.View()
	secretsView := m.secretsList.View()
	versionsView := m.versionsList.View()

	vaultsPaneStyle := styles.Pane.Copy().Width(pw[0])
	secretsPaneStyle := styles.Pane.Copy().Width(pw[1])
	versionsPaneStyle := styles.Pane.Copy().Width(pw[2])

	if m.focus == vaultsPane {
		vaultsPaneStyle = styles.FocusedPane.Copy().Width(pw[0])
	}
	if m.focus == secretsPane {
		secretsPaneStyle = styles.FocusedPane.Copy().Width(pw[1])
	}
	if m.focus == versionsPane {
		versionsPaneStyle = styles.FocusedPane.Copy().Width(pw[2])
	}

	panes := lipgloss.JoinHorizontal(lipgloss.Top,
		vaultsPaneStyle.Render(vaultsView),
		secretsPaneStyle.Render(secretsView),
		versionsPaneStyle.Render(versionsView),
	)

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

