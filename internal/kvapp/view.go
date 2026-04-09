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
			{Key: km.ActionMenu.Short(), Desc: "actions"},
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

	// Build pane map for lookup by index.
	paneMap := map[int]string{
		vaultsPane:   vaults,
		secretsPane:  secrets,
		versionsPane: versions,
	}

	// Assemble panes in visual order: parent (left), focused (center), child (right).
	// When there's no parent pane, add an empty spacer to keep the focused pane centered.
	parentWidth := m.Width * 20 / 100
	paneParts := make([]string, 0, 3)
	if m.focus > vaultsPane && pw[m.focus-1] > 0 {
		paneParts = append(paneParts, paneMap[m.focus-1])
	} else if m.focus == vaultsPane {
		spacer := lipgloss.NewStyle().Width(parentWidth).Height(h).Render("")
		paneParts = append(paneParts, spacer)
	}
	paneParts = append(paneParts, paneMap[m.focus])

	// Child column (right side).
	childIdx := m.focus + 1
	if childIdx <= versionsPane && pw[childIdx] > 0 {
		paneParts = append(paneParts, paneMap[childIdx])
	}

	panes := lipgloss.JoinHorizontal(lipgloss.Top, paneParts...)

	subBar := ui.RenderSubscriptionBar(m.CurrentSub, m.HasSubscription, m.Styles, m.Width)

	statusBar := ui.RenderStatusBar(m.Styles, sbItems, "", false, m.Width)

	view := ui.RenderCanvas(lipgloss.JoinVertical(lipgloss.Left, subBar, panes, statusBar), m.Width, m.Height, m.Styles.Bg)
	if m.actionMenu.active {
		view = m.renderActionMenu(view)
	}
	out := tea.NewView(m.RenderOverlays(view))
	out.AltScreen = true
	out.MouseMode = tea.MouseModeCellMotion
	return out
}
