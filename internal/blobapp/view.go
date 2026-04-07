package blobapp

import (
	"fmt"
	"strings"
	"time"

	"github.com/karlssonsimon/lazyaz/internal/ui"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) View() string {
	if m.Width == 0 || m.Height == 0 {
		return "loading..."
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
	if m.search.active {
		blobsHintSet = []ui.PaneHint{
			{Key: km.OpenFocused.Short(), Desc: "submit"},
			{Key: km.Cancel.Short(), Desc: "cancel"},
			{Key: km.BackspaceUp.Short(), Desc: "back"},
		}
	} else {
		blobsHintSet = []ui.PaneHint{
			{Key: km.FilterInput.Short(), Desc: "search"},
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
	if m.search.active {
		blobsPaneParams.Prefix = m.renderSearchInput(ui.PaneContentWidth(m.Styles.Chrome.Pane, pw[2]))
	} else if m.committedFilter.active {
		blobsPaneParams.Prefix = m.renderCommittedFilterBanner()
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

	sbStatus := m.Status
	sbErr := m.LastErr != ""
	if sbErr {
		sbStatus = m.LastErr
	} else if m.Loading {
		sbStatus = ui.SpinnerFrameAt(time.Since(m.LoadingStartedAt)) + " " + m.Status
	}
	statusBar := ui.RenderStatusBar(m.Styles, sbItems, sbStatus, sbErr, m.Width)

	view := ui.RenderCanvas(lipgloss.JoinVertical(lipgloss.Left, subBar, panes, statusBar), m.Width, m.Height, m.Styles.Bg)
	return m.RenderOverlays(view)
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
	if m.hasContainer {
		if m.blobLoadAll {
			title = fmt.Sprintf("%s | ALL", title)
		} else if m.search.active {
			title = fmt.Sprintf("%s | SEARCH", title)
		} else if m.committedFilter.active {
			title = fmt.Sprintf("%s | FILTER", title)
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
