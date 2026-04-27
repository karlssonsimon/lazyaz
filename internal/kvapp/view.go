package kvapp

import (
	"fmt"
	"strings"

	"github.com/karlssonsimon/lazyaz/internal/ui"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func (m Model) View() tea.View {
	if m.Width == 0 || m.Height == 0 {
		v := tea.NewView("loading...")
		v.AltScreen = true
		return v
	}

	pw := m.paneWidths
	h := m.paneHeight
	rightmost := m.focus
	if visible := m.visiblePanes(); len(visible) > 0 {
		rightmost = visible[len(visible)-1].Index
	}
	frame := func(pane int) ui.MillerColumnFrame {
		return ui.MillerColumnFrame{Width: pw[pane], Height: h, Focused: m.focus == pane, RightRule: pane != rightmost}
	}
	footer := func(pane int, l *list.Model) string {
		f := frame(pane)
		contentWidth := ui.MillerContentWidth(f)
		base := m.columnFooter(pane)
		if inspect := m.inspectFooter(pane, contentWidth); inspect != "" {
			base = lipgloss.JoinVertical(lipgloss.Left, base, inspect)
		}
		if pane != m.focus || l == nil {
			return base
		}
		switch l.FilterState() {
		case list.Filtering:
			return ui.RenderFilterLine(l.FilterInput.Value(), m.Cursor.View(),
				m.Styles, contentWidth, true)
		case list.FilterApplied:
			return lipgloss.JoinVertical(lipgloss.Left,
				ui.RenderFilterLine(l.FilterValue(), "", m.Styles, contentWidth, false),
				base)
		}
		return base
	}

	vaults := ui.RenderMillerListColumn(ui.MillerListColumn{
		List:      &m.vaultsList,
		Title:     "VAULTS",
		TitleMeta: m.columnTitleMeta(vaultsPane),
		Footer:    footer(vaultsPane, &m.vaultsList),
		Frame:     frame(vaultsPane),
	}, m.Styles)

	secrets := ui.RenderMillerListColumn(ui.MillerListColumn{
		List:      &m.secretsList,
		Title:     "SECRETS",
		TitleMeta: m.columnTitleMeta(secretsPane),
		Footer:    footer(secretsPane, &m.secretsList),
		Frame:     frame(secretsPane),
	}, m.Styles)

	versions := ui.RenderMillerListColumn(ui.MillerListColumn{
		List:      &m.versionsList,
		Title:     "VERSIONS",
		TitleMeta: m.columnTitleMeta(versionsPane),
		Footer:    footer(versionsPane, &m.versionsList),
		Frame:     frame(versionsPane),
	}, m.Styles)

	// Build pane map for lookup by index.
	paneMap := map[int]string{
		vaultsPane:   vaults,
		secretsPane:  secrets,
		versionsPane: versions,
	}

	// Three slots always reserved — empty slots become spacers (with
	// a right rule on the parent slot so the focused column always
	// borders against a vertical line).
	plainSpacer := func(width int) string {
		if width <= 0 {
			return ""
		}
		return lipgloss.NewStyle().Width(width).Height(h).Render("")
	}
	paneParts := make([]string, 0, 3)
	if m.focus > vaultsPane && pw[m.focus-1] > 0 {
		paneParts = append(paneParts, paneMap[m.focus-1])
	}
	// Topmost focus skips the parent slot — focus expanded via
	// MillerLayout(hasParent=false).
	paneParts = append(paneParts, paneMap[m.focus])

	childIdx := m.focus + 1
	if childIdx <= versionsPane && pw[childIdx] > 0 {
		if rendered, ok := paneMap[childIdx]; ok {
			paneParts = append(paneParts, rendered)
		} else {
			paneParts = append(paneParts, plainSpacer(pw[childIdx]))
		}
	}

	cols := ui.MillerLayout(m.Styles.Chrome.Pane, m.Width, m.focus > vaultsPane, true)
	if margin := ui.MillerSideMargin(cols, m.Width); margin > 0 {
		paneParts = append([]string{plainSpacer(margin)}, paneParts...)
	}
	panes := lipgloss.JoinHorizontal(lipgloss.Top, paneParts...)

	header := ui.RenderAppHeader(ui.HeaderConfig{
		Brand: "lazyaz",
		Path:  m.headerPath(),
		Meta:  ui.HeaderMeta(m.CurrentSub, m.HasSubscription, m.Styles),
	}, m.Styles, m.Width)
	statusBar := ui.RenderStatusLine(ui.StatusLineConfig{
		Mode:    m.inputMode().String(),
		Actions: m.statusActions(),
	}, m.Styles, m.Width)
	ticks := m.columnTickPositions()
	topRule := ui.RenderHorizontalRule(m.Width, m.Styles, ticks)
	bottomRule := ui.RenderHorizontalRuleBottom(m.Width, m.Styles, ticks)

	view := ui.RenderCanvas(lipgloss.JoinVertical(lipgloss.Left, header, topRule, panes, bottomRule, statusBar), m.Width, m.Height, m.Styles.Bg)
	if m.actionMenu.Active {
		view = m.renderActionMenu(view)
	}
	if m.createSecret.Active {
		view = ui.RenderFormOverlay(m.createSecret, m.Cursor.View(), m.Styles, m.Width, m.Height, view)
	}
	out := tea.NewView(m.RenderOverlays(view))
	out.AltScreen = true
	out.MouseMode = tea.MouseModeCellMotion
	return out
}

func (m Model) headerPath() []string {
	// Tab bar already says "Key Vault" — breadcrumb is the resource path.
	var path []string
	if m.HasSubscription {
		path = append(path, ui.SubscriptionDisplayName(m.CurrentSub))
	}
	if m.hasVault {
		path = append(path, m.currentVault.Name)
	}
	if m.hasSecret {
		path = append(path, m.currentSecret.Name)
	}
	return path
}

// columnTickPositions reports x-coords where vertical column rules
// live so the app-level horizontal rules can place ┬/┴ tees.
func (m Model) columnTickPositions() []int {
	pw := m.paneWidths
	parentVisible := m.focus > vaultsPane && pw[m.focus-1] > 0
	childIdx := m.focus + 1
	hasChild := childIdx <= versionsPane && pw[childIdx] > 0

	cols := ui.MillerLayout(m.Styles.Chrome.Pane, m.Width, m.focus > vaultsPane, true)
	pos := ui.MillerSideMargin(cols, m.Width)
	var ticks []int
	if parentVisible {
		pos += pw[m.focus-1]
		ticks = append(ticks, pos-1)
	}
	pos += pw[m.focus]
	if hasChild {
		ticks = append(ticks, pos-1)
	}
	return ticks
}

func (m Model) statusActions() []ui.StatusAction {
	km := m.Keymap
	actions := []ui.StatusAction{
		{Key: km.CursorDown.Short() + "/" + km.CursorUp.Short(), Label: "move"},
		{Key: km.OpenFocusedAlt.Short(), Label: "open"},
		{Key: km.NavigateLeft.Short(), Label: "back"},
		{Key: km.FilterInput.Short(), Label: "filter"},
		{Key: km.RefreshScope.Short(), Label: "refresh"},
		{Key: km.ToggleHelp.Short(), Label: "help"},
	}
	if m.focus == secretsPane || m.focus == versionsPane {
		actions = append(actions,
			ui.StatusAction{Key: km.ActionMenu.Short(), Label: "actions"},
			ui.StatusAction{Key: km.YankSecret.Short(), Label: "yank"},
		)
	}
	return actions
}

func (m Model) columnFooter(pane int) string {
	l := m.listForPane(pane)
	if l == nil {
		return ""
	}
	idx := l.Index() + 1
	count := len(l.VisibleItems())
	if count == 0 {
		idx = 0
	}
	return fmt.Sprintf("%d of %d · ↕ j/k", idx, count)
}

// columnTitleMeta is the right-aligned summary on the title row.
func (m Model) columnTitleMeta(pane int) string {
	switch pane {
	case vaultsPane:
		if total := len(m.vaults); total > 0 {
			return fmt.Sprintf("%d total", total)
		}
	case secretsPane:
		shown := len(m.secretsList.VisibleItems())
		total := len(m.secrets)
		if total == 0 {
			return ""
		}
		parts := []string{fmt.Sprintf("%d total", total)}
		if shown != total {
			parts[0] = fmt.Sprintf("%d / %d", shown, total)
		}
		if marked := len(m.markedSecrets); marked > 0 {
			parts = append(parts, fmt.Sprintf("%d marked", marked))
		}
		return strings.Join(parts, " · ")
	case versionsPane:
		if total := len(m.versions); total > 0 {
			return fmt.Sprintf("%d total", total)
		}
	}
	return ""
}
