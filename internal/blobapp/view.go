package blobapp

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

	if m.preview.open {
		m.preview.viewport.SetContent(m.preview.rendered)
	}

	ui.ClampListSelection(&m.accountsList)
	ui.ClampListSelection(&m.containersList)
	ui.ClampListSelection(&m.blobsList)

	pw := m.paneWidths
	h := m.paneHeight
	rightmostPane := m.focus
	childIdx := m.focus + 1
	if m.focus == blobsPane {
		childIdx = previewPane
	}
	if childIdx <= previewPane && pw[childIdx] > 0 {
		rightmostPane = childIdx
	}
	frame := func(pane int) ui.MillerColumnFrame {
		return ui.MillerColumnFrame{Width: pw[pane], Height: h, Focused: m.focus == pane, RightRule: pane != rightmostPane}
	}
	footer := func(pane int, l *list.Model) string {
		f := frame(pane)
		contentWidth := ui.MillerContentWidth(f)
		base := m.columnFooter(pane)
		if inspect := m.inspectFooter(pane, contentWidth); inspect != "" {
			base = lipgloss.JoinVertical(lipgloss.Left, base, inspect)
		}
		// Filter input lives in the focused column's footer so the
		// scope stays attached to its column without covering the
		// table header at the top.
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

	accounts := ui.RenderMillerListColumn(ui.MillerListColumn{
		List:      &m.accountsList,
		Title:     "ACCOUNTS",
		TitleMeta: m.columnTitleMeta(accountsPane),
		Footer:    footer(accountsPane, &m.accountsList),
		Frame:     frame(accountsPane),
	}, m.Styles)

	containers := ui.RenderMillerListColumn(ui.MillerListColumn{
		List:      &m.containersList,
		Title:     "CONTAINERS",
		TitleMeta: m.columnTitleMeta(containersPane),
		Footer:    footer(containersPane, &m.containersList),
		Frame:     frame(containersPane),
	}, m.Styles)

	blobsFrame := frame(blobsPane)
	blobsContentWidth := ui.MillerContentWidth(blobsFrame)
	blobsTableHeader := ""
	if m.hasContainer {
		blobsTableHeader = m.Styles.Chrome.RowMeta.Render(blobsColumnHeader(blobsContentWidth))
	}
	blobsPaneParams := ui.MillerListColumn{
		List:      &m.blobsList,
		Title:     "BLOBS",
		TitleMeta: m.columnTitleMeta(blobsPane),
		SubHeader: blobsTableHeader,
		Footer:    footer(blobsPane, &m.blobsList),
		Frame:     blobsFrame,
	}
	blobsPaneRendered := ui.RenderMillerListColumn(blobsPaneParams, m.Styles)

	// Build pane map for lookup by index.
	paneMap := map[int]string{
		accountsPane:   accounts,
		containersPane: containers,
		blobsPane:      blobsPaneRendered,
	}

	// Render preview pane if it has a width assigned.
	if pw[previewPane] > 0 && m.preview.open {
		vpView := m.preview.viewport.View()
		if m.textSelection.Active {
			vpView = m.textSelection.HighlightContent(m.preview.viewport, m.Styles.SelectionHighlight)
		}
		gutter := ui.RenderLineGutter(m.preview.viewport, m.Styles, previewGutterMinDigits)
		body := lipgloss.JoinHorizontal(lipgloss.Top, gutter, vpView)
		paneMap[previewPane] = ui.RenderMillerColumn(ui.MillerColumn{
			Title: "DETAILS",
			Body:  body,
			Frame: frame(previewPane),
		}, m.Styles)
	}

	// When blobs are focused and inside a folder, render the parent folder's
	// blobs in the left column instead of containers.
	if m.focus == blobsPane && m.prefix != "" && pw[containersPane] > 0 {
		paneMap[containersPane] = ui.RenderMillerListColumn(ui.MillerListColumn{
			List:   &m.parentBlobsList,
			Title:  "BLOBS",
			Footer: fmt.Sprintf("%d items · / filter", len(m.parentBlobsList.VisibleItems())),
			Frame:  frame(containersPane),
		}, m.Styles)
	}

	// Assemble panes in visual order: parent (left), focused (center),
	// child (right). The MillerLayout reserves all three slots so the
	// focused column's width stays at ~60% even when parent or child
	// has nothing to render — those slots become column spacers (with
	// a right rule so the focused column always borders against a
	// vertical line instead of floating against empty bg).
	plainSpacer := func(width int) string {
		if width <= 0 {
			return ""
		}
		return lipgloss.NewStyle().Width(width).Height(h).Render("")
	}
	paneParts := make([]string, 0, 3)
	if m.focus > accountsPane && pw[m.focus-1] > 0 {
		paneParts = append(paneParts, paneMap[m.focus-1])
	}
	// Topmost focus skips the parent slot entirely — focus expanded to
	// absorb it via MillerLayout(hasParent=false).
	paneParts = append(paneParts, paneMap[m.focus])

	if childIdx <= previewPane {
		if rendered, ok := paneMap[childIdx]; ok && pw[childIdx] > 0 {
			paneParts = append(paneParts, rendered)
		} else if pw[childIdx] > 0 {
			// Child column slot is empty: no right rule (it's the
			// rightmost block), just blank space to fill the slot.
			paneParts = append(paneParts, plainSpacer(pw[childIdx]))
		}
	}

	// Center the column block on wide terminals. Same hasParent flag
	// as resize() so margin matches the actual rendered widths.
	cols := ui.MillerLayout(m.Styles.Chrome.Pane, m.Width, m.focus > accountsPane, true)
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
	if m.sortOverlay.active {
		view = m.renderSortOverlay(view)
	}
	if m.actionMenu.Active {
		view = m.renderActionMenu(view)
	}
	if m.filter.inputOpen {
		view = m.renderPrefixSearchOverlay(view)
	}

	if m.uploadBrowserActive {
		view = ui.RenderFileBrowser(m.uploadBrowser, m.Styles, m.Width, m.Height, view)
	}

	// The conflict prompt is rendered by the parent app AFTER its ops
	// center overlay, so the prompt stays at the top of the Z-stack.
	// See (*app.Model).View and blobapp.Model.RenderUploadConflictPrompt.

	if m.confirmModal.Active {
		view = ui.RenderConfirmModal(m.confirmModal, m.Styles, m.Width, m.Height, view)
	}
	if m.textInput.Active {
		view = ui.RenderTextInputOverlay(m.textInput, m.Cursor.View(), m.Styles, m.Width, m.Height, view)
	}

	out := tea.NewView(m.RenderOverlays(view))
	out.AltScreen = true
	out.MouseMode = tea.MouseModeCellMotion
	return out
}

func (m Model) headerPath() []string {
	// Tab bar already labels the explorer (Blob/Service Bus/etc.), so
	// the header breadcrumb starts at the resource path: subscription
	// → account → container → prefix. When nothing is drilled, the
	// crumb collapses to just the subscription name.
	var path []string
	if m.HasSubscription {
		path = append(path, ui.SubscriptionDisplayName(m.CurrentSub))
	}
	if m.hasAccount {
		path = append(path, m.currentAccount.Name)
	}
	if m.hasContainer {
		path = append(path, m.containerName)
	}
	if m.prefix != "" {
		path = append(path, strings.TrimSuffix(m.prefix, "/"))
	}
	return path
}

// columnTickPositions reports x-coordinates where a vertical column
// rule lives, so app-level horizontal rules can place ┬/┴ tees at the
// matching cells. Each column block ends with a `│` at its last cell
// (when RightRule=true); the spacer rendered when accountsPane is
// focused has no rule. Position math walks the same join order as the
// pane assembly above.
func (m Model) columnTickPositions() []int {
	pw := m.paneWidths
	childIdx := m.focus + 1
	if m.focus == blobsPane {
		childIdx = previewPane
	}
	hasChild := childIdx <= previewPane && pw[childIdx] > 0
	parentVisible := m.focus > accountsPane && pw[m.focus-1] > 0

	cols := ui.MillerLayout(m.Styles.Chrome.Pane, m.Width, m.focus > accountsPane, true)
	pos := ui.MillerSideMargin(cols, m.Width)
	var ticks []int
	if parentVisible {
		pos += pw[m.focus-1]
		ticks = append(ticks, pos-1) // parent's right rule
	}
	pos += pw[m.focus]
	if hasChild {
		ticks = append(ticks, pos-1) // focused column's right rule
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
	if m.focus == blobsPane {
		actions = append(actions,
			ui.StatusAction{Key: km.ActionMenu.Short(), Label: "actions"},
			ui.StatusAction{Key: km.ToggleMark.Short(), Label: "mark"},
		)
	}
	return actions
}

func (m Model) columnFooter(pane int) string {
	var l *list.Model
	switch pane {
	case accountsPane:
		l = &m.accountsList
	case containersPane:
		l = &m.containersList
	case blobsPane:
		l = &m.blobsList
	default:
		return ""
	}
	idx := l.Index() + 1
	count := len(l.VisibleItems())
	if count == 0 {
		idx = 0
	}
	return fmt.Sprintf("%d of %d · ↕ j/k", idx, count)
}

// columnTitleMeta is the right-aligned summary that sits on the column
// title row. Mirrors the visual mockup: containers show
// "shown / total", blobs show "shown · total · selected", accounts show
// just total. The cursor index lives in the footer so the meta stays
// stable while you scroll.
func (m Model) columnTitleMeta(pane int) string {
	switch pane {
	case accountsPane:
		if total := len(m.accounts); total > 0 {
			return fmt.Sprintf("%s total", formatThousands(total))
		}
	case containersPane:
		shown := len(m.containersList.VisibleItems())
		total := len(m.containers)
		if total == 0 {
			return ""
		}
		if shown != total {
			return fmt.Sprintf("%s / %s", formatThousands(shown), formatThousands(total))
		}
		return fmt.Sprintf("%s total", formatThousands(total))
	case blobsPane:
		shown := len(m.blobsList.VisibleItems())
		total := len(m.blobs)
		if total == 0 {
			return ""
		}
		parts := []string{fmt.Sprintf("%s shown", formatThousands(shown))}
		if shown != total {
			parts = append(parts, fmt.Sprintf("%s total", formatThousands(total)))
		}
		if marked := len(m.markedBlobs); marked > 0 {
			parts = append(parts, fmt.Sprintf("%d selected", marked))
		}
		return strings.Join(parts, " · ")
	}
	return ""
}

// formatThousands inserts a thin space every three digits. Mockup uses
// space rather than comma; matches typical terminal-app conventions.
func formatThousands(n int) string {
	s := fmt.Sprintf("%d", n)
	if n < 1000 {
		return s
	}
	var b strings.Builder
	rem := len(s) % 3
	if rem == 0 {
		rem = 3
	}
	b.WriteString(s[:rem])
	for i := rem; i < len(s); i += 3 {
		b.WriteByte(' ')
		b.WriteString(s[i : i+3])
	}
	return b.String()
}


func (m Model) renderSortOverlay(base string) string {
	indices := m.sortOverlay.filtered
	if indices == nil {
		indices = make([]int, len(sortOptions))
		for i := range sortOptions {
			indices[i] = i
		}
	}
	items := make([]ui.OverlayItem, len(indices))
	for ci, si := range indices {
		opt := sortOptions[si]
		items[ci] = ui.OverlayItem{
			Label:    opt.label,
			IsActive: opt.field == m.blobSortField && opt.desc == m.blobSortDesc,
		}
	}
	cfg := ui.OverlayListConfig{
		Title:      "Sort Blobs",
		Query:      m.sortOverlay.query,
		CursorView: m.Cursor.View(),
		CloseHint:  m.Keymap.Cancel.Short(),
		Bindings: &ui.OverlayBindings{

			MoveUp:   m.Keymap.ThemeUp,

			MoveDown: m.Keymap.ThemeDown,

			Apply:    m.Keymap.ThemeApply,

			Cancel:   m.Keymap.ThemeCancel,

			Erase:    m.Keymap.BackspaceUp,

		},
		MaxVisible: len(sortOptions),
		Center:     true,
	}
	return ui.RenderOverlayList(cfg, items, m.sortOverlay.cursorIdx, m.Styles, m.Width, m.Height, base)
}

func humanSize(bytes int64) string {
	const (
		kb = 1000
		mb = kb * 1000
		gb = mb * 1000
		tb = gb * 1000
		pb = tb * 1000
	)

	switch {
	case bytes >= pb:
		return fmt.Sprintf("%.2f PB", float64(bytes)/float64(pb))
	case bytes >= tb:
		return fmt.Sprintf("%.2f TB", float64(bytes)/float64(tb))
	case bytes >= gb:
		return fmt.Sprintf("%.2f GB", float64(bytes)/float64(gb))
	case bytes >= mb:
		return fmt.Sprintf("%.2f MB", float64(bytes)/float64(mb))
	case bytes >= kb:
		return fmt.Sprintf("%.2f KB", float64(bytes)/float64(kb))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
