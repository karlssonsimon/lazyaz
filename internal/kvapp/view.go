package kvapp

import (
	"time"

	"azure-storage/internal/ui"

	"github.com/charmbracelet/lipgloss"
)

// withPaneSpinner is a thin wrapper around ui.RenderPaneSpinner that
// checks whether the given pane is the current loading target.
func (m Model) withPaneSpinner(title string, pane int, width int) string {
	loading := m.loading && m.loadingPane == pane
	return ui.RenderPaneSpinner(title, loading, m.loadingStartedAt, m.styles, width)
}

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

	pw := m.paneWidths
	pane := m.styles.Chrome.Pane
	m.vaultsList.Title = m.withPaneSpinner(m.vaultsPaneTitle(), vaultsPane, ui.PaneContentWidth(pane, pw[0]))
	m.secretsList.Title = m.withPaneSpinner(m.secretsPaneTitle(), secretsPane, ui.PaneContentWidth(pane, pw[1]))
	m.versionsList.Title = m.withPaneSpinner(m.versionsPaneTitle(), versionsPane, ui.PaneContentWidth(pane, pw[2]))

	km := m.keymap

	vaultsHints := ui.RenderPaneHints([]ui.PaneHint{
		{km.OpenFocusedAlt.Short(), "open"},
		{km.FilterInput.Short(), "filter"},
		{km.NextFocus.Short(), "next"},
		{km.SubscriptionPicker.Short(), "sub"},
		{km.Inspect.Short(), "inspect"},
	}, m.styles, ui.PaneContentWidth(pane, pw[0]))

	secretsHints := ui.RenderPaneHints([]ui.PaneHint{
		{km.OpenFocusedAlt.Short(), "versions"},
		{km.YankSecret.Short(), "yank"},
		{km.NavigateLeft.Short(), "back"},
	}, m.styles, ui.PaneContentWidth(pane, pw[1]))

	versionsHints := ui.RenderPaneHints([]ui.PaneHint{
		{km.YankSecret.Short(), "yank version"},
		{km.NavigateLeft.Short(), "back"},
	}, m.styles, ui.PaneContentWidth(pane, pw[2]))

	vaultsView := lipgloss.JoinVertical(lipgloss.Left, m.vaultsList.View(), vaultsHints)
	secretsView := lipgloss.JoinVertical(lipgloss.Left, m.secretsList.View(), secretsHints)
	versionsView := lipgloss.JoinVertical(lipgloss.Left, m.versionsList.View(), versionsHints)

	h := m.paneHeight
	vaultsPaneStyle := styles.Pane.Copy().Width(pw[0]).Height(h)
	secretsPaneStyle := styles.Pane.Copy().Width(pw[1]).Height(h)
	versionsPaneStyle := styles.Pane.Copy().Width(pw[2]).Height(h)

	if m.focus == vaultsPane {
		vaultsPaneStyle = styles.FocusedPane.Copy().Width(pw[0]).Height(h)
	}
	if m.focus == secretsPane {
		secretsPaneStyle = styles.FocusedPane.Copy().Width(pw[1]).Height(h)
	}
	if m.focus == versionsPane {
		versionsPaneStyle = styles.FocusedPane.Copy().Width(pw[2]).Height(h)
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
		sbStatus = ui.SpinnerFrameAt(time.Since(m.loadingStartedAt)) + " " + m.status
	}
	statusBar := ui.RenderStatusBar(m.styles, sbItems, sbStatus, sbErr, m.width)

	parts := []string{subBar, panes, statusBar}

	view := ui.RenderCanvas(lipgloss.JoinVertical(lipgloss.Left, parts...), m.width, m.height, m.styles.Bg)

	if m.inspectFields != nil {
		view = ui.RenderInspectOverlay(m.inspectTitle, m.inspectFields, m.styles, m.width, m.height, view)
	}
	if m.subOverlay.Active {
		view = ui.RenderSubscriptionOverlay(m.subOverlay, m.subscriptions, m.currentSub, m.loading, m.loadingStartedAt, m.styles, m.width, m.height, view)
	}
	if !m.EmbeddedMode && m.themeOverlay.Active {
		view = ui.RenderThemeOverlay(m.themeOverlay, m.schemes, m.styles, m.width, m.height, view)
	}
	if !m.EmbeddedMode && m.helpOverlay.Active {
		view = ui.RenderHelpOverlay(m.helpOverlay, m.styles, m.width, m.height, view)
	}

	return view
}

