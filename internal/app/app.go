package app

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"azure-storage/internal/azure"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const defaultDownloadRoot = "downloads"

const (
	accountsPane = iota
	containersPane
	blobsPane
)

const (
	colorBorder        = "#4B5563"
	colorBorderFocused = "#22C55E"
	colorText          = "#E5E7EB"
	colorMuted         = "#94A3B8"
	colorAccent        = "#60A5FA"
	colorAccentStrong  = "#38BDF8"
	colorDanger        = "#F87171"
	colorFilterMatch   = "#F59E0B"
	colorSelectedBg    = "#334155"
	colorSelectedText  = "#F8FAFC"
)

type Model struct {
	service *azure.Service

	spinner spinner.Model

	accountsList   list.Model
	containersList list.Model
	blobsList      list.Model

	focus int

	accounts       []azure.Account
	containers     []azure.ContainerInfo
	blobs          []azure.BlobEntry
	markedBlobs    map[string]azure.BlobEntry
	visualLineMode bool
	visualAnchor   string
	hasAccount     bool
	currentAccount azure.Account
	hasContainer   bool
	containerName  string
	prefix         string

	loading bool
	status  string
	lastErr string

	width  int
	height int
}

type accountsLoadedMsg struct {
	accounts []azure.Account
	err      error
}

type containersLoadedMsg struct {
	account    azure.Account
	containers []azure.ContainerInfo
	err        error
}

type blobsLoadedMsg struct {
	account   azure.Account
	container string
	prefix    string
	blobs     []azure.BlobEntry
	err       error
}

type blobsDownloadedMsg struct {
	destinationRoot string
	total           int
	downloaded      int
	failed          int
	failures        []string
	err             error
}

func NewModel(svc *azure.Service) Model {
	delegate := list.NewDefaultDelegate()
	delegate.SetSpacing(0)
	delegate.Styles.NormalTitle = delegate.Styles.NormalTitle.Foreground(lipgloss.Color(colorText))
	delegate.Styles.NormalDesc = delegate.Styles.NormalDesc.Foreground(lipgloss.Color(colorMuted))
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.
		Foreground(lipgloss.Color(colorSelectedText)).
		Background(lipgloss.Color(colorSelectedBg)).
		Bold(true)
	delegate.Styles.SelectedDesc = delegate.Styles.SelectedDesc.
		Foreground(lipgloss.Color(colorSelectedText)).
		Background(lipgloss.Color(colorSelectedBg))
	delegate.Styles.FilterMatch = delegate.Styles.FilterMatch.Foreground(lipgloss.Color(colorFilterMatch)).Underline(true)

	accounts := list.New([]list.Item{}, delegate, 24, 10)
	accounts.Title = "Storage Accounts"
	accounts.SetShowHelp(false)
	accounts.SetShowPagination(false)
	accounts.SetShowStatusBar(true)
	accounts.SetStatusBarItemName("account", "accounts")
	accounts.SetFilteringEnabled(true)
	accounts.DisableQuitKeybindings()
	styleList(&accounts)

	containers := list.New([]list.Item{}, delegate, 24, 10)
	containers.Title = "Containers"
	containers.SetShowHelp(false)
	containers.SetShowPagination(false)
	containers.SetShowStatusBar(true)
	containers.SetStatusBarItemName("container", "containers")
	containers.SetFilteringEnabled(true)
	containers.DisableQuitKeybindings()
	styleList(&containers)

	blobs := list.New([]list.Item{}, delegate, 40, 10)
	blobs.Title = "Blobs"
	blobs.SetShowHelp(false)
	blobs.SetShowPagination(false)
	blobs.SetShowStatusBar(true)
	blobs.SetStatusBarItemName("entry", "entries")
	blobs.SetFilteringEnabled(true)
	blobs.DisableQuitKeybindings()
	styleList(&blobs)

	spin := spinner.New()
	spin.Spinner = spinner.Dot
	spin.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(colorAccentStrong))

	return Model{
		service:        svc,
		spinner:        spin,
		accountsList:   accounts,
		containersList: containers,
		blobsList:      blobs,
		markedBlobs:    make(map[string]azure.BlobEntry),
		focus:          accountsPane,
		status:         "Discovering storage accounts from Azure subscriptions...",
		loading:        true,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(spinner.Tick, discoverAccountsCmd(m.service))
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	markVisualAfterListUpdate := false

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resize()
		return m, nil

	case spinner.TickMsg:
		if !m.loading {
			return m, nil
		}
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case accountsLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.lastErr = msg.err.Error()
			m.status = "Failed to discover storage accounts"
			return m, nil
		}

		m.lastErr = ""
		m.accounts = msg.accounts
		m.accountsList.ResetFilter()
		m.accountsList.SetItems(accountsToItems(msg.accounts))
		m.accountsList.Title = fmt.Sprintf("Storage Accounts (%d)", len(msg.accounts))

		if len(msg.accounts) == 0 {
			m.hasAccount = false
			m.hasContainer = false
			m.status = "No storage accounts discovered. Verify az login context and RBAC visibility."
			m.clearBlobSelectionState()
			m.containers = nil
			m.blobs = nil
			m.containersList.ResetFilter()
			m.blobsList.ResetFilter()
			m.containersList.SetItems(nil)
			m.blobsList.SetItems(nil)
			m.containersList.Title = "Containers"
			m.blobsList.Title = "Blobs"
			return m, nil
		}

		m.accountsList.Select(0)
		selected, ok := m.accountsList.SelectedItem().(accountItem)
		if !ok {
			selected = accountItem{account: msg.accounts[0]}
		}

		m.hasAccount = true
		m.currentAccount = selected.account
		m.clearBlobSelectionState()
		m.status = fmt.Sprintf("Discovered %d storage accounts", len(msg.accounts))
		m.loading = true
		return m, tea.Batch(spinner.Tick, loadContainersCmd(m.service, m.currentAccount))

	case containersLoadedMsg:
		if !m.hasAccount || !sameAccount(m.currentAccount, msg.account) {
			return m, nil
		}

		m.loading = false
		if msg.err != nil {
			m.lastErr = msg.err.Error()
			m.status = fmt.Sprintf("Failed to load containers for %s", msg.account.Name)
			m.clearBlobSelectionState()
			m.containers = nil
			m.blobs = nil
			m.containersList.ResetFilter()
			m.blobsList.ResetFilter()
			m.containersList.SetItems(nil)
			m.blobsList.SetItems(nil)
			m.hasContainer = false
			m.containerName = ""
			m.prefix = ""
			return m, nil
		}

		m.lastErr = ""
		m.containers = msg.containers
		m.containersList.ResetFilter()
		m.containersList.SetItems(containersToItems(msg.containers))
		m.containersList.Title = fmt.Sprintf("Containers (%d)", len(msg.containers))
		m.containersList.Select(0)

		if len(msg.containers) == 0 {
			m.hasContainer = false
			m.containerName = ""
			m.prefix = ""
			m.clearBlobSelectionState()
			m.blobs = nil
			m.blobsList.ResetFilter()
			m.blobsList.SetItems(nil)
			m.blobsList.Title = "Blobs"
			m.status = fmt.Sprintf("No containers found in %s", msg.account.Name)
			return m, nil
		}

		m.hasContainer = false
		m.containerName = ""
		m.prefix = ""
		m.clearBlobSelectionState()
		m.blobs = nil
		m.blobsList.ResetFilter()
		m.blobsList.SetItems(nil)
		m.blobsList.Title = "Blobs"
		m.status = fmt.Sprintf("Loaded %d containers from %s. Open a container to browse blobs.", len(msg.containers), msg.account.Name)
		return m, nil

	case blobsLoadedMsg:
		if !m.hasAccount || !m.hasContainer {
			return m, nil
		}
		if !sameAccount(m.currentAccount, msg.account) || m.containerName != msg.container {
			return m, nil
		}
		if m.prefix != msg.prefix {
			return m, nil
		}

		m.loading = false
		if msg.err != nil {
			m.lastErr = msg.err.Error()
			m.status = fmt.Sprintf("Failed to browse blobs in %s/%s", msg.account.Name, msg.container)
			m.visualLineMode = false
			m.visualAnchor = ""
			m.blobs = nil
			m.blobsList.ResetFilter()
			m.blobsList.SetItems(nil)
			m.blobsList.Title = "Blobs"
			return m, nil
		}

		m.lastErr = ""
		m.visualLineMode = false
		m.visualAnchor = ""
		m.blobs = msg.blobs
		m.blobsList.ResetFilter()
		m.blobsList.Title = fmt.Sprintf("Blobs (%d)", len(msg.blobs))
		m.refreshBlobItems()
		m.status = fmt.Sprintf("%d entries in %s/%s under %q", len(msg.blobs), msg.account.Name, msg.container, msg.prefix)
		return m, nil

	case blobsDownloadedMsg:
		m.loading = false
		if msg.err != nil {
			m.lastErr = msg.err.Error()
			m.status = "Failed to download blobs"
			return m, nil
		}

		if msg.failed > 0 {
			m.lastErr = strings.Join(msg.failures, " | ")
			m.status = fmt.Sprintf("Downloaded %d/%d blobs to %s", msg.downloaded, msg.total, msg.destinationRoot)
			return m, nil
		}

		m.lastErr = ""
		m.status = fmt.Sprintf("Downloaded %d blob(s) to %s", msg.downloaded, msg.destinationRoot)
		return m, nil

	case tea.KeyMsg:
		focusedFilterActive := m.focusedListSettingFilter()
		if m.focus == blobsPane && m.visualLineMode && msg.String() == "/" {
			m.visualLineMode = false
			m.visualAnchor = ""
			m.refreshBlobItems()
			m.status = "Visual mode off"
		}
		if m.focus == blobsPane && m.visualLineMode && !focusedFilterActive && isBlobNavigationKey(msg.String()) {
			markVisualAfterListUpdate = true
		}

		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "ctrl+d":
			m.scrollFocusedHalfPage(1)
			return m, nil
		case "ctrl+u":
			m.scrollFocusedHalfPage(-1)
			return m, nil
		case "D":
			if m.focus == blobsPane && !focusedFilterActive {
				return m.startMarkedAction("download")
			}
		case "v", "V":
			if m.focus == blobsPane && !focusedFilterActive {
				m.toggleVisualLineMode()
				return m, nil
			}
		case " ":
			if m.focus == blobsPane && !focusedFilterActive {
				m.toggleCurrentBlobMark()
				return m, nil
			}
		case "esc":
			if m.focus == blobsPane && m.visualLineMode && !focusedFilterActive {
				m.visualLineMode = false
				m.visualAnchor = ""
				m.refreshBlobItems()
				m.status = "Visual mode off"
				return m, nil
			}
		case "tab":
			if !focusedFilterActive {
				m.nextFocus()
				return m, nil
			}
		case "shift+tab":
			if !focusedFilterActive {
				m.previousFocus()
				return m, nil
			}
		case "d":
			if !focusedFilterActive {
				m.loading = true
				m.lastErr = ""
				m.status = "Refreshing account discovery across subscriptions..."
				return m, tea.Batch(spinner.Tick, discoverAccountsCmd(m.service))
			}
		case "r":
			if !focusedFilterActive {
				return m.refresh()
			}
		case "enter":
			if focusedFilterActive {
				m.commitFocusedFilter()
				m.status = fmt.Sprintf("Filter applied for %s", paneName(m.focus))
				return m, nil
			}
			return m.handleEnter()
		case "l":
			if !focusedFilterActive {
				return m.handleEnter()
			}
		case "right":
			if !focusedFilterActive {
				return m.handleEnter()
			}
		case "h":
			if !focusedFilterActive {
				return m.navigateLeft()
			}
		case "left":
			if !focusedFilterActive {
				return m.navigateLeft()
			}
		case "backspace":
			if !focusedFilterActive {
				if m.focus == blobsPane && m.hasContainer && m.prefix != "" {
					m.prefix = parentPrefix(m.prefix)
					m.loading = true
					m.status = fmt.Sprintf("Browsing %q", m.prefix)
					return m, tea.Batch(spinner.Tick, loadBlobsCmd(m.service, m.currentAccount, m.containerName, m.prefix))
				}
			}
		}
	}

	switch m.focus {
	case accountsPane:
		m.accountsList, cmd = m.accountsList.Update(msg)
	case containersPane:
		m.containersList, cmd = m.containersList.Update(msg)
	case blobsPane:
		m.blobsList, cmd = m.blobsList.Update(msg)
	}

	if markVisualAfterListUpdate && m.focus == blobsPane && m.visualLineMode {
		m.refreshBlobItems()
		m.status = fmt.Sprintf("Visual mode on. %d in range.", len(m.visualSelectionBlobNames()))
	}

	return m, cmd
}

func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "loading..."
	}

	headerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorAccent)).
		Bold(true).
		Padding(0, 1)

	metaStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorMuted)).
		Padding(0, 1)

	paneStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorText)).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(colorBorder)).
		Padding(0, 1)

	focusedPaneStyle := paneStyle.Copy().
		BorderForeground(lipgloss.Color(colorBorderFocused))

	statusStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorText)).
		Padding(0, 1)

	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorMuted)).
		Padding(0, 1)

	errorStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorDanger)).
		Padding(0, 1)

	filterHintStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorAccent)).
		Padding(0, 1)

	accountName := "-"
	containerName := "-"
	if m.hasAccount {
		accountName = m.currentAccount.Name
	}
	if m.hasContainer {
		containerName = m.containerName
	}

	header := headerStyle.Width(m.width).Render(trimToWidth("Azure Blob Explorer", m.width-2))
	headerMeta := metaStyle.Width(m.width).Render(trimToWidth(fmt.Sprintf("Account: %s | Container: %s | Prefix: %q", accountName, containerName, m.prefix), m.width-2))

	m.accountsList.Title = m.accountsPaneTitle()
	m.containersList.Title = m.containersPaneTitle()
	m.blobsList.Title = m.blobsPaneTitle()

	accountsView := m.accountsList.View()
	containersView := m.containersList.View()
	blobsView := m.blobsList.View()

	accountsPaneStyle := paneStyle.Copy().MarginRight(1)
	containersPaneStyle := paneStyle.Copy().MarginRight(1)
	blobsPaneStyle := paneStyle.Copy()

	if m.focus == accountsPane {
		accountsPaneStyle = focusedPaneStyle.Copy().MarginRight(1)
	}
	if m.focus == containersPane {
		containersPaneStyle = focusedPaneStyle.Copy().MarginRight(1)
	}
	if m.focus == blobsPane {
		blobsPaneStyle = focusedPaneStyle.Copy()
	}

	panes := lipgloss.JoinHorizontal(
		lipgloss.Top,
		accountsPaneStyle.Render(accountsView),
		containersPaneStyle.Render(containersView),
		blobsPaneStyle.Render(blobsView),
	)

	filterHint := "Press / to filter the focused pane (fzf-style live filter)."
	if m.focusedListSettingFilter() {
		filterHint = fmt.Sprintf("Filtering %s: type to narrow, up/down to move, Enter applies filter.", paneName(m.focus))
	} else if m.focus == blobsPane && m.visualLineMode {
		filterHint = "Visual mode: move to select a line range, Space toggles persistent marks, D downloads selection, v/V exits."
	}
	filterLine := filterHintStyle.Width(m.width).Render(trimToWidth(filterHint, m.width-2))

	errorLine := ""
	if m.lastErr != "" {
		errorLine = errorStyle.Width(m.width).Render(trimToWidth("Error: "+m.lastErr, m.width-2))
	}

	statusText := m.status
	if m.loading {
		statusText = fmt.Sprintf("%s %s", m.spinner.View(), m.status)
	}
	statusLine := statusStyle.Width(m.width).Render(trimToWidth(statusText, m.width-2))

	help := "keys: tab/shift+tab focus | / filter pane | enter/l/right open->focus right | h/left left/up | space toggle mark | v/V visual-line range | D download selection | ctrl+d/ctrl+u half-page | backspace up folder | r refresh | d rediscover | q quit"
	helpLine := helpStyle.Width(m.width).Render(trimToWidth(help, m.width-2))

	parts := []string{header, headerMeta, panes, filterLine}
	if errorLine != "" {
		parts = append(parts, errorLine)
	}
	parts = append(parts, statusLine, helpLine)

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m *Model) resize() {
	if m.width <= 0 || m.height <= 0 {
		return
	}

	left := m.width / 4
	mid := m.width / 4
	right := m.width - left - mid - 8
	if left < 26 {
		left = 26
	}
	if mid < 26 {
		mid = 26
	}
	if right < 40 {
		right = 40
	}

	height := m.height - 10
	if height < 8 {
		height = 8
	}

	m.accountsList.SetSize(left, height)
	m.containersList.SetSize(mid, height)
	m.blobsList.SetSize(right, height)
}

func (m *Model) nextFocus() {
	if m.focus == blobsPane && m.visualLineMode {
		m.visualLineMode = false
		m.visualAnchor = ""
		m.refreshBlobItems()
	}
	m.blurAllFilters()
	m.focus = (m.focus + 1) % 3
}

func (m *Model) previousFocus() {
	if m.focus == blobsPane && m.visualLineMode {
		m.visualLineMode = false
		m.visualAnchor = ""
		m.refreshBlobItems()
	}
	m.blurAllFilters()
	m.focus--
	if m.focus < 0 {
		m.focus = 2
	}
}

func (m *Model) blurAllFilters() {
	m.accountsList.FilterInput.Blur()
	m.containersList.FilterInput.Blur()
	m.blobsList.FilterInput.Blur()
}

func (m *Model) commitFocusedFilter() {
	m.blurAllFilters()

	switch m.focus {
	case accountsPane:
		applyFilterState(&m.accountsList)
	case containersPane:
		applyFilterState(&m.containersList)
	case blobsPane:
		applyFilterState(&m.blobsList)
	}
}

func applyFilterState(l *list.Model) {
	if strings.TrimSpace(l.FilterValue()) == "" {
		l.SetFilterState(list.Unfiltered)
		return
	}
	l.SetFilterState(list.FilterApplied)
}

func (m *Model) clearBlobSelectionState() {
	m.visualLineMode = false
	m.visualAnchor = ""
	if m.markedBlobs == nil {
		m.markedBlobs = make(map[string]azure.BlobEntry)
		return
	}
	for name := range m.markedBlobs {
		delete(m.markedBlobs, name)
	}
}

func (m *Model) refreshBlobItems() {
	m.blobsList.SetItems(blobsToItems(m.blobs, m.prefix, m.markedBlobs, m.visualSelectionNames()))
}

func (m *Model) toggleVisualLineMode() {
	if !m.hasContainer {
		m.status = "Open a container before visual selection"
		return
	}

	m.visualLineMode = !m.visualLineMode
	if !m.visualLineMode {
		m.visualAnchor = ""
		m.refreshBlobItems()
		m.status = fmt.Sprintf("Visual mode off. %d marked with space.", len(m.markedBlobs))
		return
	}

	m.visualAnchor = m.currentBlobName()
	m.refreshBlobItems()
	if m.visualAnchor == "" {
		m.status = "Visual mode on. Move up/down to select a range."
		return
	}
	selectionCount := len(m.visualSelectionBlobNames())
	m.status = fmt.Sprintf("Visual mode on. %d in range.", selectionCount)
}

func (m *Model) toggleCurrentBlobMark() {
	if !m.hasContainer {
		m.status = "Open a container before marking blobs"
		return
	}

	item, ok := m.blobsList.SelectedItem().(blobItem)
	if !ok {
		m.status = "No blob selected"
		return
	}
	if item.blob.IsPrefix {
		m.status = "Folder selection is not supported yet"
		return
	}

	if _, exists := m.markedBlobs[item.blob.Name]; exists {
		delete(m.markedBlobs, item.blob.Name)
		m.refreshBlobItems()
		m.status = fmt.Sprintf("Unmarked %s (%d marked)", item.displayName, len(m.markedBlobs))
		return
	}

	m.markedBlobs[item.blob.Name] = item.blob
	m.refreshBlobItems()
	m.status = fmt.Sprintf("Marked %s (%d marked)", item.displayName, len(m.markedBlobs))
}

func (m Model) currentBlobName() string {
	item, ok := m.blobsList.SelectedItem().(blobItem)
	if !ok {
		return ""
	}
	return item.blob.Name
}

func (m Model) visualSelectionItems() []blobItem {
	if !m.visualLineMode {
		return nil
	}

	visibleItems := m.blobsList.VisibleItems()
	if len(visibleItems) == 0 {
		return nil
	}

	items := make([]blobItem, 0, len(visibleItems))
	for _, item := range visibleItems {
		blobEntry, ok := item.(blobItem)
		if !ok {
			continue
		}
		items = append(items, blobEntry)
	}
	if len(items) == 0 {
		return nil
	}

	current := m.currentBlobName()
	if current == "" {
		return nil
	}

	anchor := m.visualAnchor
	if anchor == "" {
		anchor = current
	}

	anchorIdx := -1
	currentIdx := -1
	for i, item := range items {
		if anchorIdx < 0 && item.blob.Name == anchor {
			anchorIdx = i
		}
		if currentIdx < 0 && item.blob.Name == current {
			currentIdx = i
		}
	}
	if currentIdx < 0 {
		return nil
	}
	if anchorIdx < 0 {
		anchorIdx = currentIdx
	}

	start, end := anchorIdx, currentIdx
	if start > end {
		start, end = end, start
	}

	return items[start : end+1]
}

func (m Model) visualSelectionNames() map[string]struct{} {
	selectedItems := m.visualSelectionItems()
	if len(selectedItems) == 0 {
		return nil
	}

	selectedNames := make(map[string]struct{}, len(selectedItems))
	for _, item := range selectedItems {
		selectedNames[item.blob.Name] = struct{}{}
	}
	return selectedNames
}

func (m Model) visualSelectionBlobNames() []string {
	selectedItems := m.visualSelectionItems()
	if len(selectedItems) == 0 {
		return nil
	}

	unique := make(map[string]struct{}, len(selectedItems))
	for _, item := range selectedItems {
		if item.blob.IsPrefix {
			continue
		}
		unique[item.blob.Name] = struct{}{}
	}

	names := make([]string, 0, len(unique))
	for name := range unique {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (m Model) startMarkedAction(action string) (Model, tea.Cmd) {
	switch action {
	case "download":
		return m.startDownloadMarkedBlobs()
	default:
		m.status = fmt.Sprintf("Unknown marked action: %s", action)
		return m, nil
	}
}

func (m Model) startDownloadMarkedBlobs() (Model, tea.Cmd) {
	if !m.hasAccount || !m.hasContainer {
		m.status = "Open a container before downloading"
		return m, nil
	}

	blobNameSet := make(map[string]struct{})
	for _, name := range m.sortedMarkedBlobNames() {
		blobNameSet[name] = struct{}{}
	}
	for _, name := range m.visualSelectionBlobNames() {
		blobNameSet[name] = struct{}{}
	}
	blobNames := sortedBlobNameSet(blobNameSet)
	if len(blobNames) == 0 {
		item, ok := m.blobsList.SelectedItem().(blobItem)
		if !ok || item.blob.IsPrefix {
			m.status = "Select blobs with space or visual mode before downloading"
			return m, nil
		}
		blobNames = []string{item.blob.Name}
	}

	destinationRoot := filepath.Join(defaultDownloadRoot, m.currentAccount.Name, m.containerName)
	m.loading = true
	m.lastErr = ""
	m.status = fmt.Sprintf("Downloading %d blob(s) to %s", len(blobNames), destinationRoot)
	return m, tea.Batch(spinner.Tick, downloadBlobsCmd(m.service, m.currentAccount, m.containerName, blobNames, destinationRoot))
}

func (m Model) sortedMarkedBlobNames() []string {
	if len(m.markedBlobs) == 0 {
		return nil
	}
	names := make([]string, 0, len(m.markedBlobs))
	for name := range m.markedBlobs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func sortedBlobNameSet(values map[string]struct{}) []string {
	if len(values) == 0 {
		return nil
	}

	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (m *Model) scrollFocusedHalfPage(direction int) {
	if direction == 0 {
		return
	}

	var target *list.Model
	switch m.focus {
	case accountsPane:
		target = &m.accountsList
	case containersPane:
		target = &m.containersList
	case blobsPane:
		target = &m.blobsList
	default:
		return
	}

	steps := halfPageStep(*target)
	for i := 0; i < steps; i++ {
		if direction > 0 {
			target.CursorDown()
		} else {
			target.CursorUp()
		}
	}

	if m.focus == blobsPane && m.visualLineMode {
		m.refreshBlobItems()
		m.status = fmt.Sprintf("Visual mode on. %d in range.", len(m.visualSelectionBlobNames()))
	}
}

func halfPageStep(l list.Model) int {
	if l.Paginator.PerPage > 1 {
		if half := l.Paginator.PerPage / 2; half > 0 {
			return half
		}
	}

	if visible := len(l.VisibleItems()); visible > 1 {
		if half := visible / 2; half > 0 {
			return half
		}
	}

	return 1
}

func (m Model) focusedListSettingFilter() bool {
	switch m.focus {
	case accountsPane:
		return m.accountsList.SettingFilter()
	case containersPane:
		return m.containersList.SettingFilter()
	case blobsPane:
		return m.blobsList.SettingFilter()
	default:
		return false
	}
}

func (m Model) refresh() (Model, tea.Cmd) {
	if !m.hasAccount || m.focus == accountsPane {
		m.loading = true
		m.lastErr = ""
		m.status = "Refreshing account discovery..."
		return m, tea.Batch(spinner.Tick, discoverAccountsCmd(m.service))
	}

	if m.focus == containersPane || !m.hasContainer {
		m.loading = true
		m.lastErr = ""
		m.status = fmt.Sprintf("Loading containers in %s", m.currentAccount.Name)
		return m, tea.Batch(spinner.Tick, loadContainersCmd(m.service, m.currentAccount))
	}

	m.loading = true
	m.lastErr = ""
	m.status = fmt.Sprintf("Browsing %s/%s", m.currentAccount.Name, m.containerName)
	return m, tea.Batch(spinner.Tick, loadBlobsCmd(m.service, m.currentAccount, m.containerName, m.prefix))
}

func (m Model) navigateLeft() (Model, tea.Cmd) {
	switch m.focus {
	case blobsPane:
		if m.hasContainer && m.prefix != "" {
			m.prefix = parentPrefix(m.prefix)
			m.loading = true
			m.status = fmt.Sprintf("Browsing %q", m.prefix)
			return m, tea.Batch(spinner.Tick, loadBlobsCmd(m.service, m.currentAccount, m.containerName, m.prefix))
		}
		if m.visualLineMode {
			m.visualLineMode = false
			m.visualAnchor = ""
			m.refreshBlobItems()
		}
		m.focus = containersPane
		m.status = "Focus: containers"
		return m, nil
	case containersPane:
		m.focus = accountsPane
		m.status = "Focus: storage accounts"
		return m, nil
	default:
		return m, nil
	}
}

func (m Model) handleEnter() (Model, tea.Cmd) {
	if m.focus == accountsPane {
		item, ok := m.accountsList.SelectedItem().(accountItem)
		if !ok {
			return m, nil
		}

		m.currentAccount = item.account
		m.hasAccount = true
		m.hasContainer = false
		m.containerName = ""
		m.prefix = ""
		m.clearBlobSelectionState()
		m.focus = containersPane

		m.containers = nil
		m.blobs = nil
		m.containersList.ResetFilter()
		m.blobsList.ResetFilter()
		m.containersList.SetItems(nil)
		m.blobsList.SetItems(nil)
		m.containersList.Title = "Containers"
		m.blobsList.Title = "Blobs"

		m.loading = true
		m.status = fmt.Sprintf("Loading containers in %s", item.account.Name)
		return m, tea.Batch(spinner.Tick, loadContainersCmd(m.service, item.account))
	}

	if m.focus == containersPane {
		item, ok := m.containersList.SelectedItem().(containerItem)
		if !ok {
			return m, nil
		}

		m.containerName = item.container.Name
		m.hasContainer = true
		m.prefix = ""
		m.clearBlobSelectionState()
		m.focus = blobsPane

		m.blobs = nil
		m.blobsList.ResetFilter()
		m.blobsList.SetItems(nil)
		m.blobsList.Title = "Blobs"

		m.loading = true
		m.status = fmt.Sprintf("Browsing %s/%s", m.currentAccount.Name, m.containerName)
		return m, tea.Batch(spinner.Tick, loadBlobsCmd(m.service, m.currentAccount, m.containerName, m.prefix))
	}

	if m.focus == blobsPane {
		item, ok := m.blobsList.SelectedItem().(blobItem)
		if !ok {
			return m, nil
		}

		if item.blob.IsPrefix {
			m.prefix = item.blob.Name
			m.loading = true
			m.blobsList.ResetFilter()
			m.status = fmt.Sprintf("Browsing %q", m.prefix)
			return m, tea.Batch(spinner.Tick, loadBlobsCmd(m.service, m.currentAccount, m.containerName, m.prefix))
		}

		details := fmt.Sprintf("Blob %s | size: %s | modified: %s | type: %s | tier: %s | metadata: %d",
			item.blob.Name,
			humanSize(item.blob.Size),
			formatTime(item.blob.LastModified),
			emptyToDash(item.blob.ContentType),
			emptyToDash(item.blob.AccessTier),
			item.blob.MetadataCount,
		)
		m.status = details
		return m, nil
	}

	return m, nil
}

type accountItem struct {
	account azure.Account
}

func (i accountItem) Title() string {
	return i.account.Name
}

func (i accountItem) Description() string {
	shortSub := i.account.SubscriptionID
	if len(shortSub) > 8 {
		shortSub = shortSub[:8]
	}
	if i.account.ResourceGroup == "" {
		return fmt.Sprintf("sub %s", shortSub)
	}
	return fmt.Sprintf("sub %s | rg %s", shortSub, i.account.ResourceGroup)
}

func (i accountItem) FilterValue() string {
	return i.account.Name + " " + i.account.SubscriptionID + " " + i.account.ResourceGroup
}

type containerItem struct {
	container azure.ContainerInfo
}

func (i containerItem) Title() string {
	return i.container.Name
}

func (i containerItem) Description() string {
	if i.container.LastModified.IsZero() {
		return "-"
	}
	return formatTime(i.container.LastModified)
}

func (i containerItem) FilterValue() string {
	return i.container.Name
}

type blobItem struct {
	blob        azure.BlobEntry
	displayName string
	marked      bool
	visual      bool
}

func (i blobItem) Title() string {
	prefix := "   "
	if i.visual {
		prefix = ">  "
	}
	if i.marked {
		if i.visual {
			prefix = ">* "
		} else {
			prefix = "*  "
		}
	}

	if i.blob.IsPrefix {
		if i.visual {
			return "> [DIR] " + i.displayName
		}
		return "  [DIR] " + i.displayName
	}

	return prefix + i.displayName
}

func (i blobItem) Description() string {
	if i.blob.IsPrefix {
		return ""
	}
	return fmt.Sprintf("%s | %s | %s", humanSize(i.blob.Size), formatTime(i.blob.LastModified), emptyToDash(i.blob.AccessTier))
}

func (i blobItem) FilterValue() string {
	return i.blob.Name
}

func accountsToItems(accounts []azure.Account) []list.Item {
	items := make([]list.Item, 0, len(accounts))
	for _, account := range accounts {
		items = append(items, accountItem{account: account})
	}
	return items
}

func containersToItems(containers []azure.ContainerInfo) []list.Item {
	items := make([]list.Item, 0, len(containers))
	for _, containerInfo := range containers {
		items = append(items, containerItem{container: containerInfo})
	}
	return items
}

func blobsToItems(entries []azure.BlobEntry, prefix string, marked map[string]azure.BlobEntry, visual map[string]struct{}) []list.Item {
	items := make([]list.Item, 0, len(entries))
	for _, entry := range entries {
		items = append(items, blobItem{
			blob:        entry,
			displayName: trimPrefixForDisplay(entry.Name, prefix),
			marked:      isBlobMarked(marked, entry.Name),
			visual:      isBlobVisualSelected(visual, entry.Name),
		})
	}
	return items
}

func isBlobMarked(marked map[string]azure.BlobEntry, blobName string) bool {
	if len(marked) == 0 {
		return false
	}
	_, ok := marked[blobName]
	return ok
}

func isBlobVisualSelected(visual map[string]struct{}, blobName string) bool {
	if len(visual) == 0 {
		return false
	}
	_, ok := visual[blobName]
	return ok
}

func trimPrefixForDisplay(name, prefix string) string {
	if prefix == "" {
		return name
	}
	trimmed := strings.TrimPrefix(name, prefix)
	if trimmed == "" {
		return name
	}
	return trimmed
}

func parentPrefix(prefix string) string {
	prefix = strings.TrimSuffix(prefix, "/")
	idx := strings.LastIndex(prefix, "/")
	if idx < 0 {
		return ""
	}
	return prefix[:idx+1]
}

func discoverAccountsCmd(svc *azure.Service) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		accounts, err := svc.DiscoverAccounts(ctx)
		return accountsLoadedMsg{accounts: accounts, err: err}
	}
}

func loadContainersCmd(svc *azure.Service, account azure.Account) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()

		containers, err := svc.ListContainers(ctx, account)
		return containersLoadedMsg{account: account, containers: containers, err: err}
	}
}

func loadBlobsCmd(svc *azure.Service, account azure.Account, containerName, prefix string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		blobs, err := svc.ListBlobs(ctx, account, containerName, prefix)
		return blobsLoadedMsg{account: account, container: containerName, prefix: prefix, blobs: blobs, err: err}
	}
}

func downloadBlobsCmd(svc *azure.Service, account azure.Account, containerName string, blobNames []string, destinationRoot string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()

		results, err := svc.DownloadBlobs(ctx, account, containerName, blobNames, destinationRoot)
		msg := blobsDownloadedMsg{
			destinationRoot: destinationRoot,
			total:           len(blobNames),
			err:             err,
		}
		if err != nil {
			return msg
		}

		for _, result := range results {
			if result.Err != nil {
				msg.failed++
				if len(msg.failures) < 3 {
					msg.failures = append(msg.failures, fmt.Sprintf("%s: %v", result.BlobName, result.Err))
				}
				continue
			}
			msg.downloaded++
		}

		if msg.failed > 3 {
			msg.failures = append(msg.failures, fmt.Sprintf("... and %d more", msg.failed-3))
		}

		return msg
	}
}

func styleList(l *list.Model) {
	l.Styles.TitleBar = l.Styles.TitleBar.
		Foreground(lipgloss.Color(colorMuted)).
		Padding(0, 1)
	l.Styles.Title = l.Styles.Title.
		Bold(true).
		Foreground(lipgloss.Color(colorAccent))
	l.Styles.Spinner = l.Styles.Spinner.Foreground(lipgloss.Color(colorAccentStrong))
	l.Styles.FilterPrompt = l.Styles.FilterPrompt.Foreground(lipgloss.Color(colorAccent))
	l.Styles.FilterCursor = l.Styles.FilterCursor.Foreground(lipgloss.Color(colorAccentStrong))
	l.Styles.DefaultFilterCharacterMatch = l.Styles.DefaultFilterCharacterMatch.Foreground(lipgloss.Color(colorFilterMatch)).Underline(true)
	l.Styles.StatusBar = l.Styles.StatusBar.
		Foreground(lipgloss.Color(colorMuted))
	l.Styles.StatusBarActiveFilter = l.Styles.StatusBarActiveFilter.Foreground(lipgloss.Color(colorAccent)).Bold(true)
	l.Styles.StatusBarFilterCount = l.Styles.StatusBarFilterCount.Foreground(lipgloss.Color(colorAccentStrong)).Bold(true)
	l.Styles.NoItems = l.Styles.NoItems.Foreground(lipgloss.Color(colorMuted))
	l.Styles.PaginationStyle = l.Styles.PaginationStyle.Foreground(lipgloss.Color(colorMuted))
	l.Styles.HelpStyle = l.Styles.HelpStyle.Foreground(lipgloss.Color(colorMuted))
}

func paneName(pane int) string {
	switch pane {
	case accountsPane:
		return "storage accounts"
	case containersPane:
		return "containers"
	case blobsPane:
		return "blobs"
	default:
		return "items"
	}
}

func isBlobNavigationKey(key string) bool {
	switch key {
	case "up", "down", "j", "k", "pgup", "pgdown", "home", "end", "g", "G":
		return true
	default:
		return false
	}
}

func (m Model) accountsPaneTitle() string {
	title := "Storage Accounts"
	if len(m.accounts) > 0 {
		title = fmt.Sprintf("Storage Accounts (%d)", len(m.accounts))
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
	if len(m.markedBlobs) > 0 {
		title = fmt.Sprintf("%s | marked:%d", title, len(m.markedBlobs))
	}
	if m.visualLineMode {
		title = fmt.Sprintf("%s | VISUAL:%d", title, len(m.visualSelectionBlobNames()))
	}
	return title
}

func sameAccount(a, b azure.Account) bool {
	return a.Name == b.Name && a.SubscriptionID == b.SubscriptionID
}

func trimToWidth(s string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max <= 3 {
		return string(r[:max])
	}
	return string(r[:max-3]) + "..."
}

func emptyToDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Local().Format("2006-01-02 15:04")
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
