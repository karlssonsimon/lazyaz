package blobapp

import (
	"fmt"
	"strings"

	"azure-storage/internal/ui"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "loading..."
	}

	styles := ui.NewChromeStyles(m.palette)

	subscriptionName := "-"
	accountName := "-"
	containerName := "-"
	if m.hasSubscription {
		subscriptionName = subscriptionDisplayName(m.currentSub)
	}
	if m.hasAccount {
		accountName = m.currentAccount.Name
	}
	if m.hasContainer {
		containerName = m.containerName
	}

	header := styles.Header.Width(m.width).Render(ui.TrimToWidth("Azure Blob Explorer", m.width-2))
	headerMeta := styles.Meta.Width(m.width).Render(ui.TrimToWidth(fmt.Sprintf("Subscription: %s | Account: %s | Container: %s | Prefix: %q", subscriptionName, accountName, containerName, m.prefix), m.width-2))

	m.subscriptionsList.Title = m.subscriptionsPaneTitle()
	m.accountsList.Title = m.accountsPaneTitle()
	m.containersList.Title = m.containersPaneTitle()
	m.blobsList.Title = m.blobsPaneTitle()
	if m.preview.open {
		m.preview.viewport.SetContent(m.preview.rendered)
	}

	ui.ClampListSelection(&m.subscriptionsList)
	ui.ClampListSelection(&m.accountsList)
	ui.ClampListSelection(&m.containersList)
	ui.ClampListSelection(&m.blobsList)

	subscriptionsView := m.subscriptionsList.View()
	accountsView := m.accountsList.View()
	containersView := m.containersList.View()
	blobsView := m.blobsList.View()
	previewView := ""
	if m.preview.open {
		previewView = m.preview.viewport.View()
	}

	subscriptionsPaneStyle := styles.Pane.Copy().MarginRight(1)
	accountsPaneStyle := styles.Pane.Copy().MarginRight(1)
	containersPaneStyle := styles.Pane.Copy().MarginRight(1)
	blobsPaneStyle := styles.Pane.Copy()
	previewPaneStyle := styles.Pane.Copy()

	if m.focus == subscriptionsPane {
		subscriptionsPaneStyle = styles.FocusedPane.Copy().MarginRight(1)
	}
	if m.focus == accountsPane {
		accountsPaneStyle = styles.FocusedPane.Copy().MarginRight(1)
	}
	if m.focus == containersPane {
		containersPaneStyle = styles.FocusedPane.Copy().MarginRight(1)
	}
	if m.focus == blobsPane {
		blobsPaneStyle = styles.FocusedPane.Copy()
	}
	if m.preview.open && m.focus == previewPane {
		previewPaneStyle = styles.FocusedPane.Copy()
	}

	paneParts := []string{
		subscriptionsPaneStyle.Render(subscriptionsView),
		accountsPaneStyle.Render(accountsView),
		containersPaneStyle.Render(containersView),
		blobsPaneStyle.Render(blobsView),
	}
	if m.preview.open {
		previewTitle := m.preview.title(m.palette)
		previewPaneContent := lipgloss.JoinVertical(lipgloss.Left,
			lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(m.palette.Accent)).Render(previewTitle),
			previewView,
		)
		paneParts = append(paneParts, previewPaneStyle.Render(previewPaneContent))
	}

	panes := lipgloss.JoinHorizontal(lipgloss.Top, paneParts...)

	filterHint := "Press / to filter the focused pane (fzf-style live filter)."
	if m.focusedListSettingFilter() {
		if m.focus == blobsPane && !m.blobLoadAll {
			filterHint = "Blob search mode: type a prefix, Enter runs server-side prefix search."
		} else {
			filterHint = fmt.Sprintf("Filtering %s: type to narrow, up/down to move, Enter applies filter.", paneName(m.focus))
		}
	} else if m.focus == blobsPane && m.visualLineMode {
		filterHint = "Visual mode: move to select a line range, Space toggles persistent marks, D downloads selection, v/V exits."
	}
	filterLine := styles.FilterHint.Width(m.width).Render(ui.TrimToWidth(filterHint, m.width-2))

	errorLine := ""
	if m.lastErr != "" {
		errorLine = styles.Error.Width(m.width).Render(ui.TrimToWidth("Error: "+m.lastErr, m.width-2))
	}

	statusText := m.status
	if m.loading {
		statusText = fmt.Sprintf("%s %s", m.spinner.View(), m.status)
	}
	statusLine := styles.Status.Width(m.width).Render(ui.TrimToWidth(statusText, m.width-2))

	helpLine := styles.Help.Width(m.width).Render(ui.TrimToWidth(m.keymap.FooterHelpText(), m.width-2))

	parts := []string{header, headerMeta, panes, filterLine}
	if errorLine != "" {
		parts = append(parts, errorLine)
	}
	parts = append(parts, statusLine, helpLine)

	view := lipgloss.JoinVertical(lipgloss.Left, parts...)

	if !m.EmbeddedMode && m.themeOverlay.Active {
		view = ui.RenderThemeOverlay(m.themeOverlay, m.themes, m.palette, m.width, m.height, view)
	}
	if !m.EmbeddedMode && m.helpOverlay.Active {
		view = ui.RenderHelpOverlay("Azure Blob Explorer Help", m.keymap.HelpSections(), m.palette, m.width, m.height, view)
	}

	return view
}

func (m Model) subscriptionsPaneTitle() string {
	title := "Subscriptions"
	if len(m.subscriptions) > 0 {
		title = fmt.Sprintf("Subscriptions (%d)", len(m.subscriptions))
	}
	return title
}

func (m Model) accountsPaneTitle() string {
	title := "Storage Accounts"
	if m.hasSubscription {
		title = fmt.Sprintf("Storage Accounts · %s", subscriptionDisplayName(m.currentSub))
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
		} else if m.blobSearchQuery != "" {
			title = fmt.Sprintf("%s | PREFIX:%s", title, blobSearchPrefix(m.prefix, m.blobSearchQuery))
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
