package kvapp

import (
	"time"

	"azure-storage/internal/ui"

	"github.com/charmbracelet/lipgloss"
)

// withPaneSpinner is a thin wrapper around ui.RenderPaneSpinner that
// checks whether the given pane is the current loading target.
func (m Model) withPaneSpinner(title string, pane int, width int) string {
	loading := m.Loading && m.LoadingPane == pane
	return ui.RenderPaneSpinner(title, loading, m.LoadingStartedAt, m.Styles, width)
}

func (m Model) View() string {
	if m.Width == 0 || m.Height == 0 {
		return "loading..."
	}

	styles := m.Styles.Chrome

	var sbItems []ui.StatusBarItem
	if m.hasVault {
		sbItems = append(sbItems, ui.StatusBarItem{Label: "Vault:", Value: m.currentVault.Name})
	}
	if m.hasSecret {
		sbItems = append(sbItems, ui.StatusBarItem{Label: "Secret:", Value: m.currentSecret.Name})
	}

	pw := m.paneWidths
	pane := m.Styles.Chrome.Pane
	m.vaultsList.Title = m.withPaneSpinner(m.vaultsPaneTitle(), vaultsPane, ui.PaneContentWidth(pane, pw[0]))
	m.secretsList.Title = m.withPaneSpinner(m.secretsPaneTitle(), secretsPane, ui.PaneContentWidth(pane, pw[1]))
	m.versionsList.Title = m.withPaneSpinner(m.versionsPaneTitle(), versionsPane, ui.PaneContentWidth(pane, pw[2]))

	km := m.Keymap

	vaultsHints := ui.RenderPaneHints([]ui.PaneHint{
		{km.OpenFocusedAlt.Short(), "open"},
		{km.FilterInput.Short(), "filter"},
		{km.NextFocus.Short(), "next"},
		{km.SubscriptionPicker.Short(), "sub"},
		{km.Inspect.Short(), "inspect"},
	}, m.Styles, ui.PaneContentWidth(pane, pw[0]))

	secretsHints := ui.RenderPaneHints([]ui.PaneHint{
		{km.OpenFocusedAlt.Short(), "versions"},
		{km.YankSecret.Short(), "yank"},
		{km.NavigateLeft.Short(), "back"},
	}, m.Styles, ui.PaneContentWidth(pane, pw[1]))

	versionsHints := ui.RenderPaneHints([]ui.PaneHint{
		{km.YankSecret.Short(), "yank version"},
		{km.NavigateLeft.Short(), "back"},
	}, m.Styles, ui.PaneContentWidth(pane, pw[2]))

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

	subBar := ui.RenderSubscriptionBar(m.CurrentSub, m.HasSubscription, m.Styles, m.Width)

	sbStatus := m.Status
	sbErr := m.LastErr != ""
	if sbErr {
		sbStatus = m.LastErr
	} else if m.Loading {
		sbStatus = ui.SpinnerFrameAt(time.Since(m.LoadingStartedAt)) + " " + m.Status
	}
	statusBar := ui.RenderStatusBar(m.Styles, sbItems, sbStatus, sbErr, m.Width)

	parts := []string{subBar, panes, statusBar}

	view := ui.RenderCanvas(lipgloss.JoinVertical(lipgloss.Left, parts...), m.Width, m.Height, m.Styles.Bg)
	return m.RenderOverlays(view)
}

