package kvapp

import (
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
	if m.hasVault {
		sbItems = append(sbItems, ui.StatusBarItem{Label: "Vault:", Value: m.currentVault.Name})
	}
	if m.hasSecret {
		sbItems = append(sbItems, ui.StatusBarItem{Label: "Secret:", Value: m.currentSecret.Name})
	}

	pw := m.paneWidths
	h := m.paneHeight
	km := m.Keymap
	paneStyle := m.Styles.Chrome.Pane

	vaults := ui.RenderListPane(ui.ListPane{
		List:     &m.vaultsList,
		Title:    m.vaultsPaneTitle(),
		Loading:  m.Loading && m.LoadingPane == vaultsPane,
		LoadedAt: m.LoadingStartedAt,
		Hints: []ui.PaneHint{
			{Key: km.OpenFocusedAlt.Short(), Desc: "open"},
			{Key: km.FilterInput.Short(), Desc: "filter"},
			{Key: km.NextFocus.Short(), Desc: "next"},
			{Key: km.SubscriptionPicker.Short(), Desc: "sub"},
			{Key: km.Inspect.Short(), Desc: "inspect"},
		},
		Footer: m.inspectFooter(vaultsPane, ui.PaneContentWidth(paneStyle, pw[0])),
		Frame:  ui.PaneFrame{Width: pw[0], Height: h, Focused: m.focus == vaultsPane},
	}, m.Styles)

	secrets := ui.RenderListPane(ui.ListPane{
		List:     &m.secretsList,
		Title:    m.secretsPaneTitle(),
		Loading:  m.Loading && m.LoadingPane == secretsPane,
		LoadedAt: m.LoadingStartedAt,
		Hints: []ui.PaneHint{
			{Key: km.OpenFocusedAlt.Short(), Desc: "versions"},
			{Key: km.YankSecret.Short(), Desc: "yank"},
			{Key: km.NavigateLeft.Short(), Desc: "back"},
		},
		Footer: m.inspectFooter(secretsPane, ui.PaneContentWidth(paneStyle, pw[1])),
		Frame:  ui.PaneFrame{Width: pw[1], Height: h, Focused: m.focus == secretsPane},
	}, m.Styles)

	versions := ui.RenderListPane(ui.ListPane{
		List:     &m.versionsList,
		Title:    m.versionsPaneTitle(),
		Loading:  m.Loading && m.LoadingPane == versionsPane,
		LoadedAt: m.LoadingStartedAt,
		Hints: []ui.PaneHint{
			{Key: km.YankSecret.Short(), Desc: "yank version"},
			{Key: km.NavigateLeft.Short(), Desc: "back"},
		},
		Footer: m.inspectFooter(versionsPane, ui.PaneContentWidth(paneStyle, pw[2])),
		Frame:  ui.PaneFrame{Width: pw[2], Height: h, Focused: m.focus == versionsPane},
	}, m.Styles)

	panes := lipgloss.JoinHorizontal(lipgloss.Top, vaults, secrets, versions)

	subBar := ui.RenderSubscriptionBar(m.CurrentSub, m.HasSubscription, m.Styles, m.Width)

	sbStatus := m.Status
	sbErr := m.LastErr != ""
	if sbErr {
		sbStatus = m.LastErr
	}
	statusBar := ui.RenderStatusBar(m.Styles, sbItems, sbStatus, sbErr, m.Width)

	view := ui.RenderCanvas(lipgloss.JoinVertical(lipgloss.Left, subBar, panes, statusBar), m.Width, m.Height, m.Styles.Bg)
	out := tea.NewView(m.RenderOverlays(view))
	out.AltScreen = true
	out.MouseMode = tea.MouseModeCellMotion
	return out
}
