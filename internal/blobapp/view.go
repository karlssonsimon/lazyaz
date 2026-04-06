package blobapp

import (
	"fmt"
	"strings"
	"time"

	"azure-storage/internal/ui"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "loading..."
	}

	styles := m.styles.Chrome

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

	pw := m.paneWidths
	pane := m.styles.Chrome.Pane
	m.accountsList.Title = m.withPaneSpinner(m.accountsPaneTitle(), accountsPane, ui.PaneContentWidth(pane, pw[0]))
	m.containersList.Title = m.withPaneSpinner(m.containersPaneTitle(), containersPane, ui.PaneContentWidth(pane, pw[1]))
	m.blobsList.Title = m.withPaneSpinner(m.blobsPaneTitle(), blobsPane, ui.PaneContentWidth(pane, pw[2]))
	if m.preview.open {
		m.preview.viewport.SetContent(m.preview.rendered)
	}

	ui.ClampListSelection(&m.accountsList)
	ui.ClampListSelection(&m.containersList)
	ui.ClampListSelection(&m.blobsList)

	km := m.keymap

	accountsHints := ui.RenderPaneHints([]ui.PaneHint{
		{km.OpenFocusedAlt.Short(), "open"},
		{km.FilterInput.Short(), "filter"},
		{km.NextFocus.Short(), "next"},
		{km.SubscriptionPicker.Short(), "sub"},
		{km.Inspect.Short(), "inspect"},
	}, m.styles, ui.PaneContentWidth(pane, pw[0]))

	containersHints := ui.RenderPaneHints([]ui.PaneHint{
		{km.OpenFocusedAlt.Short(), "open"},
		{km.NavigateLeft.Short(), "back"},
		{km.FilterInput.Short(), "filter"},
	}, m.styles, ui.PaneContentWidth(pane, pw[1]))

	var blobsHints string
	if m.search.active {
		blobsHints = ui.RenderPaneHints([]ui.PaneHint{
			{"enter", "submit"},
			{"esc", "cancel"},
			{"backspace", "back"},
		}, m.styles, ui.PaneContentWidth(pane, pw[2]))
	} else {
		blobsHints = ui.RenderPaneHints([]ui.PaneHint{
			{km.FilterInput.Short(), "search"},
			{km.ToggleMark.Short(), "mark"},
			{km.DownloadSelection.Short(), "download"},
			{km.OpenFocusedAlt.Short(), "preview"},
		}, m.styles, ui.PaneContentWidth(pane, pw[2]))
	}

	accountsView := lipgloss.JoinVertical(lipgloss.Left, m.accountsList.View(), accountsHints)
	containersView := lipgloss.JoinVertical(lipgloss.Left, m.containersList.View(), containersHints)

	var blobsViewParts []string
	if m.search.active {
		blobsViewParts = append(blobsViewParts, m.renderSearchInput(ui.PaneContentWidth(pane, pw[2])))
	}
	blobsViewParts = append(blobsViewParts, m.blobsList.View(), blobsHints)
	blobsView := lipgloss.JoinVertical(lipgloss.Left, blobsViewParts...)

	previewView := ""
	if m.preview.open {
		previewHints := ui.RenderPaneHints([]ui.PaneHint{
			{km.PreviewBack.Short(), "back"},
			{km.PreviewDown.Short() + "/" + km.PreviewUp.Short(), "scroll"},
			{km.PreviewBottom.Short(), "bottom"},
		}, m.styles, ui.PaneContentWidth(pane, pw[3]))
		previewView = m.preview.viewport.View()
		previewView = lipgloss.JoinVertical(lipgloss.Left, previewView, previewHints)
	}

	h := m.paneHeight
	accountsPaneStyle := styles.Pane.Copy().Width(pw[0]).Height(h)
	containersPaneStyle := styles.Pane.Copy().Width(pw[1]).Height(h)
	blobsPaneStyle := styles.Pane.Copy().Width(pw[2]).Height(h)
	previewPaneStyle := styles.Pane.Copy().Width(pw[3]).Height(h)

	if m.focus == accountsPane {
		accountsPaneStyle = styles.FocusedPane.Copy().Width(pw[0]).Height(h)
	}
	if m.focus == containersPane {
		containersPaneStyle = styles.FocusedPane.Copy().Width(pw[1]).Height(h)
	}
	if m.focus == blobsPane {
		blobsPaneStyle = styles.FocusedPane.Copy().Width(pw[2]).Height(h)
	}
	if m.preview.open && m.focus == previewPane {
		previewPaneStyle = styles.FocusedPane.Copy().Width(pw[3]).Height(h)
	}

	paneParts := []string{
		accountsPaneStyle.Render(accountsView),
		containersPaneStyle.Render(containersView),
		blobsPaneStyle.Render(blobsView),
	}
	if m.preview.open {
		previewTitle := m.preview.title(m.styles)
		previewPaneContent := lipgloss.JoinVertical(lipgloss.Left,
			m.styles.Accent.Render(previewTitle),
			previewView,
		)
		paneParts = append(paneParts, previewPaneStyle.Render(previewPaneContent))
	}

	panes := lipgloss.JoinHorizontal(lipgloss.Top, paneParts...)

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

// withPaneSpinner is a thin wrapper around ui.RenderPaneSpinner that
// checks whether the given pane is the current loading target.
func (m Model) withPaneSpinner(title string, pane int, width int) string {
	loading := m.loading && m.loadingPane == pane
	return ui.RenderPaneSpinner(title, loading, m.loadingStartedAt, m.styles, width)
}

func (m Model) accountsPaneTitle() string {
	title := "Storage Accounts"
	if m.hasSubscription {
		title = fmt.Sprintf("Storage Accounts · %s", ui.SubscriptionDisplayName(m.currentSub))
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
	if m.hasContainer {
		if m.blobLoadAll {
			title = fmt.Sprintf("%s | ALL", title)
		} else if m.search.active {
			title = fmt.Sprintf("%s | SEARCH", title)
		}
	}
	if len(m.markedBlobs) > 0 {
		title = fmt.Sprintf("%s | marked:%d", title, len(m.markedBlobs))
	}
	if m.visualLineMode {
		title = fmt.Sprintf("%s | VISUAL:%d", title, len(m.visualSelectionBlobNames()))
	}
	return title
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
