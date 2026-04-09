package blobapp

import (
	"fmt"
	"strings"

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

	// Build status bar items.
	var sbItems []ui.StatusBarItem
	if m.hasAccount {
		sbItems = append(sbItems, ui.StatusBarItem{Label: "Account:", Value: m.currentAccount.Name})
	}
	if m.hasContainer {
		label := m.containerName
		if m.prefix != "" {
			label += "/" + strings.TrimSuffix(m.prefix, "/")
		}
		sbItems = append(sbItems, ui.StatusBarItem{Label: "Container:", Value: label})
	}

	if m.preview.open {
		m.preview.viewport.SetContent(m.preview.rendered)
	}

	ui.ClampListSelection(&m.accountsList)
	ui.ClampListSelection(&m.containersList)
	ui.ClampListSelection(&m.blobsList)

	pw := m.paneWidths
	h := m.paneHeight
	km := m.Keymap
	paneStyle := m.Styles.Chrome.Pane

	accounts := ui.RenderListPane(ui.ListPane{
		List:     &m.accountsList,
		Title:    m.accountsPaneTitle(),
		Loading:  m.Loading && m.LoadingPane == accountsPane,
		LoadedAt: m.LoadingStartedAt,
		Hints: []ui.PaneHint{
			{Key: km.OpenFocusedAlt.Short(), Desc: "open"},
			{Key: km.FilterInput.Short(), Desc: "filter"},
			{Key: km.NextFocus.Short(), Desc: "next"},
			{Key: km.SubscriptionPicker.Short(), Desc: "sub"},
			{Key: km.Inspect.Short(), Desc: "inspect"},
		},
		Footer: m.inspectFooter(accountsPane, ui.PaneContentWidth(paneStyle, pw[0])),
		Frame:  ui.PaneFrame{Width: pw[0], Height: h, Focused: m.focus == accountsPane},
	}, m.Styles)

	containers := ui.RenderListPane(ui.ListPane{
		List:     &m.containersList,
		Title:    m.containersPaneTitle(),
		Loading:  m.Loading && m.LoadingPane == containersPane,
		LoadedAt: m.LoadingStartedAt,
		Hints: []ui.PaneHint{
			{Key: km.OpenFocusedAlt.Short(), Desc: "open"},
			{Key: km.NavigateLeft.Short(), Desc: "back"},
			{Key: km.FilterInput.Short(), Desc: "filter"},
		},
		Footer: m.inspectFooter(containersPane, ui.PaneContentWidth(paneStyle, pw[1])),
		Frame:  ui.PaneFrame{Width: pw[1], Height: h, Focused: m.focus == containersPane},
	}, m.Styles)

	var blobsHintSet []ui.PaneHint
	if m.filter.inputOpen {
		blobsHintSet = []ui.PaneHint{
			{Key: km.NextFocus.Short(), Desc: "switch input"},
			{Key: km.OpenFocused.Short(), Desc: "submit"},
			{Key: km.Cancel.Short(), Desc: "close"},
		}
	} else {
		blobsHintSet = []ui.PaneHint{
			{Key: km.FilterInput.Short(), Desc: "search"},
			{Key: km.SortBlobs.Short(), Desc: "sort"},
			{Key: km.ToggleMark.Short(), Desc: "mark"},
			{Key: km.DownloadSelection.Short(), Desc: "download"},
			{Key: km.OpenFocusedAlt.Short(), Desc: "preview"},
		}
	}

	blobsPaneParams := ui.ListPane{
		List:     &m.blobsList,
		Title:    m.blobsPaneTitle(),
		Loading:  m.Loading && m.LoadingPane == blobsPane,
		LoadedAt: m.LoadingStartedAt,
		Hints:    blobsHintSet,
		Footer:   m.inspectFooter(blobsPane, ui.PaneContentWidth(paneStyle, pw[2])),
		Frame:    ui.PaneFrame{Width: pw[2], Height: h, Focused: m.focus == blobsPane},
	}
	if m.filter.inputOpen || m.hasActiveFilter() {
		contentWidth := ui.PaneContentWidth(m.Styles.Chrome.Pane, pw[2])
		var filterPrefix string
		if m.filter.inputOpen {
			filterPrefix = m.renderFilterInput(contentWidth)
		} else {
			filterPrefix = m.renderFilterBanner()
		}
		// Append the entry count so hiding the status bar loses no info.
		n := len(m.blobsList.Items())
		countText := m.Styles.List.StatusBar.Padding(0, 0, 0, 2).Render(fmt.Sprintf("%d entries", n))
		blobsPaneParams.Prefix = filterPrefix + "\n" + countText
		m.blobsList.SetShowStatusBar(false)
	} else {
		m.blobsList.SetShowStatusBar(true)
	}
	blobsPane := ui.RenderListPane(blobsPaneParams, m.Styles)

	paneParts := []string{accounts, containers, blobsPane}

	if m.preview.open {
		previewHints := ui.RenderPaneHints([]ui.PaneHint{
			{Key: km.PreviewBack.Short(), Desc: "back"},
			{Key: km.PreviewDown.Short() + "/" + km.PreviewUp.Short(), Desc: "scroll"},
			{Key: km.PreviewBottom.Short(), Desc: "bottom"},
		}, m.Styles, ui.PaneContentWidth(m.Styles.Chrome.Pane, pw[3]))
		previewTitle := m.Styles.Accent.Render(m.preview.title(m.Styles))
		previewContent := lipgloss.JoinVertical(lipgloss.Left,
			previewTitle,
			m.preview.viewport.View(),
			previewHints,
		)
		focused := m.focus == previewPane
		preview := ui.RenderPane(previewContent, ui.PaneFrame{Width: pw[3], Height: h, Focused: focused}, m.Styles)
		paneParts = append(paneParts, preview)
	}

	panes := lipgloss.JoinHorizontal(lipgloss.Top, paneParts...)

	subBar := ui.RenderSubscriptionBar(m.CurrentSub, m.HasSubscription, m.Styles, m.Width)

	statusBar := ui.RenderStatusBar(m.Styles, sbItems, "", false, m.Width)

	view := ui.RenderCanvas(lipgloss.JoinVertical(lipgloss.Left, subBar, panes, statusBar), m.Width, m.Height, m.Styles.Bg)
	if m.sortOverlay.active {
		view = m.renderSortOverlay(view)
	}
	out := tea.NewView(m.RenderOverlays(view))
	out.AltScreen = true
	out.MouseMode = tea.MouseModeCellMotion
	return out
}

func (m Model) accountsPaneTitle() string {
	title := "Storage Accounts"
	if m.HasSubscription {
		title = fmt.Sprintf("Storage Accounts · %s", ui.SubscriptionDisplayName(m.CurrentSub))
	}
	if len(m.accounts) > 0 {
		title = fmt.Sprintf("%s (%d)", title, len(m.accounts))
	}
	return title
}

func (m Model) containersPaneTitle() string {
	title := "Containers"
	if m.hasAccount {
		title = fmt.Sprintf("Containers · %s", m.currentAccount.Name)
	}
	if m.containers != nil {
		title = fmt.Sprintf("%s (%d)", title, len(m.containers))
	}
	return title
}

func (m Model) blobsPaneTitle() string {
	title := "Blobs"
	if m.hasAccount && m.hasContainer {
		path := "/"
		if m.prefix != "" {
			path = "/" + strings.TrimPrefix(m.prefix, "/")
		}
		title = fmt.Sprintf("Blobs · %s/%s · %s", m.currentAccount.Name, m.containerName, path)
	} else if m.hasAccount {
		title = fmt.Sprintf("Blobs · %s", m.currentAccount.Name)
	}
	if m.hasContainer && m.blobs != nil {
		title = fmt.Sprintf("%s (%d)", title, len(m.blobs))
	}
	if m.hasContainer && m.blobLoadAll {
		title = fmt.Sprintf("%s | ALL", title)
	}
	if ind := blobSortIndicator(m.blobSortField, m.blobSortDesc); ind != "" {
		title = fmt.Sprintf("%s | %s", title, ind)
	}
	if len(m.markedBlobs) > 0 {
		title = fmt.Sprintf("%s | marked:%d", title, len(m.markedBlobs))
	}
	if m.visualLineMode {
		title = fmt.Sprintf("%s | VISUAL:%d", title, len(m.visualSelectionBlobNames()))
	}
	return title
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
		MaxVisible: len(sortOptions),
		Center:     true,
	}
	return ui.RenderOverlayList(cfg, items, m.sortOverlay.cursorIdx, m.Styles.Overlay, m.Width, m.Height, base)
}

func humanSize(bytes int64) string {
	const (
		kb = 1024
		mb = kb * 1024
		gb = mb * 1024
	)

	switch {
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
