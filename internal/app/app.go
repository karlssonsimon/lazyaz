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
const defaultBlobPrefixSearchLimit = 500
const defaultHierarchyBlobLoadLimit = 500

const (
	subscriptionsPane = iota
	accountsPane
	containersPane
	blobsPane
	previewPane
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

	subscriptionsList list.Model
	accountsList      list.Model
	containersList    list.Model
	blobsList         list.Model

	focus int

	subscriptions   []azure.Subscription
	accounts        []azure.Account
	containers      []azure.ContainerInfo
	blobs           []azure.BlobEntry
	markedBlobs     map[string]azure.BlobEntry
	visualLineMode  bool
	visualAnchor    string
	hasSubscription bool
	currentSub      azure.Subscription
	hasAccount      bool
	currentAccount  azure.Account
	hasContainer    bool
	containerName   string
	prefix          string
	blobLoadAll     bool
	blobSearchQuery string
	preview         previewState
	pendingPreviewG bool

	loading bool
	status  string
	lastErr string

	width  int
	height int
}

type subscriptionsLoadedMsg struct {
	subscriptions []azure.Subscription
	err           error
}

type accountsLoadedMsg struct {
	subscriptionID string
	accounts       []azure.Account
	err            error
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
	loadAll   bool
	query     string
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

type previewWindowLoadedMsg struct {
	requestID   int
	account     azure.Account
	container   string
	blobName    string
	blobSize    int64
	contentType string
	windowStart int64
	cursor      int64
	data        []byte
	err         error
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

	subscriptions := list.New([]list.Item{}, delegate, 28, 10)
	subscriptions.Title = "Subscriptions"
	subscriptions.SetShowHelp(false)
	subscriptions.SetShowPagination(false)
	subscriptions.SetShowStatusBar(true)
	subscriptions.SetStatusBarItemName("subscription", "subscriptions")
	subscriptions.SetFilteringEnabled(true)
	subscriptions.DisableQuitKeybindings()
	styleList(&subscriptions)

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
		service:           svc,
		spinner:           spin,
		subscriptionsList: subscriptions,
		accountsList:      accounts,
		containersList:    containers,
		blobsList:         blobs,
		markedBlobs:       make(map[string]azure.BlobEntry),
		preview:           newPreviewState(),
		focus:             subscriptionsPane,
		status:            "Loading Azure subscriptions...",
		loading:           true,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(spinner.Tick, loadSubscriptionsCmd(m.service))
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

	case subscriptionsLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.lastErr = msg.err.Error()
			m.status = "Failed to load subscriptions"
			return m, nil
		}

		m.lastErr = ""
		m.subscriptions = msg.subscriptions
		m.subscriptionsList.ResetFilter()
		m.subscriptionsList.SetItems(subscriptionsToItems(msg.subscriptions))
		m.subscriptionsList.Title = fmt.Sprintf("Subscriptions (%d)", len(msg.subscriptions))

		if len(msg.subscriptions) == 0 {
			m.hasSubscription = false
			m.hasAccount = false
			m.hasContainer = false
			m.status = "No subscriptions found. Verify az login context and tenant access."
			m.clearBlobSelectionState()
			m.resetBlobLoadState()
			m.resetPreviewState()
			m.accounts = nil
			m.containers = nil
			m.blobs = nil
			m.accountsList.ResetFilter()
			m.containersList.ResetFilter()
			m.blobsList.ResetFilter()
			m.accountsList.SetItems(nil)
			m.containersList.SetItems(nil)
			m.blobsList.SetItems(nil)
			m.accountsList.Title = "Storage Accounts"
			m.containersList.Title = "Containers"
			m.blobsList.Title = "Blobs"
			return m, nil
		}

		m.subscriptionsList.Select(0)
		m.hasSubscription = false
		m.currentSub = azure.Subscription{}
		m.hasAccount = false
		m.hasContainer = false
		m.resetBlobLoadState()
		m.resetPreviewState()
		m.status = fmt.Sprintf("Loaded %d subscriptions. Select one and press Enter.", len(msg.subscriptions))
		return m, nil

	case accountsLoadedMsg:
		if !m.hasSubscription || m.currentSub.ID != msg.subscriptionID {
			return m, nil
		}

		m.loading = false
		if msg.err != nil {
			m.lastErr = msg.err.Error()
			m.status = fmt.Sprintf("Failed to load storage accounts in %s", subscriptionDisplayName(m.currentSub))
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
			m.status = fmt.Sprintf("No storage accounts found in %s", subscriptionDisplayName(m.currentSub))
			m.clearBlobSelectionState()
			m.resetBlobLoadState()
			m.resetPreviewState()
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
		m.hasAccount = false
		m.currentAccount = azure.Account{}
		m.clearBlobSelectionState()
		m.resetBlobLoadState()
		m.resetPreviewState()
		m.containers = nil
		m.blobs = nil
		m.containersList.ResetFilter()
		m.blobsList.ResetFilter()
		m.containersList.SetItems(nil)
		m.blobsList.SetItems(nil)
		m.containersList.Title = "Containers"
		m.blobsList.Title = "Blobs"
		m.status = fmt.Sprintf("Loaded %d storage accounts from %s. Open an account to view containers.", len(msg.accounts), subscriptionDisplayName(m.currentSub))
		return m, nil

	case containersLoadedMsg:
		if !m.hasAccount || !sameAccount(m.currentAccount, msg.account) {
			return m, nil
		}

		m.loading = false
		if msg.err != nil {
			m.lastErr = msg.err.Error()
			m.status = fmt.Sprintf("Failed to load containers for %s", msg.account.Name)
			m.clearBlobSelectionState()
			m.resetBlobLoadState()
			m.resetPreviewState()
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
			m.resetBlobLoadState()
			m.resetPreviewState()
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
		m.resetBlobLoadState()
		m.resetPreviewState()
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
		if m.blobLoadAll != msg.loadAll {
			return m, nil
		}
		if m.blobSearchQuery != msg.query {
			return m, nil
		}

		m.loading = false
		if msg.err != nil {
			m.lastErr = msg.err.Error()
			m.status = fmt.Sprintf("Failed to load blobs in %s/%s", msg.account.Name, msg.container)
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
		if msg.loadAll {
			m.status = fmt.Sprintf("Loaded all %d blobs in %s/%s", len(msg.blobs), msg.account.Name, msg.container)
		} else if msg.query != "" {
			effectivePrefix := blobSearchPrefix(m.prefix, msg.query)
			m.status = fmt.Sprintf("Found %d blobs by prefix %q in %s/%s", len(msg.blobs), effectivePrefix, msg.account.Name, msg.container)
		} else {
			m.status = fmt.Sprintf("Loaded %d entries (max %d) in %s/%s under %q", len(msg.blobs), defaultHierarchyBlobLoadLimit, msg.account.Name, msg.container, msg.prefix)
		}
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

	case previewWindowLoadedMsg:
		return m.handlePreviewWindowLoaded(msg)

	case tea.KeyMsg:
		if m.preview.open && m.focus == previewPane {
			return m.handlePreviewKey(msg)
		}

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
		case "a", "A":
			if m.focus == blobsPane && !focusedFilterActive {
				return m.toggleBlobLoadAllMode()
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
				m.status = "Refreshing subscriptions..."
				return m, tea.Batch(spinner.Tick, loadSubscriptionsCmd(m.service))
			}
		case "r":
			if !focusedFilterActive {
				return m.refresh()
			}
		case "enter":
			if focusedFilterActive {
				cmd := m.commitFocusedFilter()
				return m, cmd
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
				if m.focus == blobsPane && m.hasContainer && !m.blobLoadAll && m.prefix != "" {
					m.prefix = parentPrefix(m.prefix)
					m.blobSearchQuery = ""
					m.loading = true
					m.status = fmt.Sprintf("Loading up to %d entries under %q", defaultHierarchyBlobLoadLimit, m.prefix)
					return m, tea.Batch(spinner.Tick, loadHierarchyBlobsCmd(m.service, m.currentAccount, m.containerName, m.prefix, defaultHierarchyBlobLoadLimit))
				}
			}
		}
	}

	switch m.focus {
	case subscriptionsPane:
		m.subscriptionsList, cmd = m.subscriptionsList.Update(msg)
	case accountsPane:
		m.accountsList, cmd = m.accountsList.Update(msg)
	case containersPane:
		m.containersList, cmd = m.containersList.Update(msg)
	case blobsPane:
		m.blobsList, cmd = m.blobsList.Update(msg)
	case previewPane:
		cmd = nil
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

	header := headerStyle.Width(m.width).Render(trimToWidth("Azure Blob Explorer", m.width-2))
	headerMeta := metaStyle.Width(m.width).Render(trimToWidth(fmt.Sprintf("Subscription: %s | Account: %s | Container: %s | Prefix: %q", subscriptionName, accountName, containerName, m.prefix), m.width-2))

	m.subscriptionsList.Title = m.subscriptionsPaneTitle()
	m.accountsList.Title = m.accountsPaneTitle()
	m.containersList.Title = m.containersPaneTitle()
	m.blobsList.Title = m.blobsPaneTitle()
	if m.preview.open {
		m.preview.viewport.SetContent(m.preview.rendered)
	}

	clampListSelection(&m.subscriptionsList)
	clampListSelection(&m.accountsList)
	clampListSelection(&m.containersList)
	clampListSelection(&m.blobsList)

	subscriptionsView := m.subscriptionsList.View()
	accountsView := m.accountsList.View()
	containersView := m.containersList.View()
	blobsView := m.blobsList.View()
	previewView := ""
	if m.preview.open {
		previewView = m.preview.viewport.View()
	}

	subscriptionsPaneStyle := paneStyle.Copy().MarginRight(1)
	accountsPaneStyle := paneStyle.Copy().MarginRight(1)
	containersPaneStyle := paneStyle.Copy().MarginRight(1)
	blobsPaneStyle := paneStyle.Copy()
	previewPaneStyle := paneStyle.Copy()

	if m.focus == subscriptionsPane {
		subscriptionsPaneStyle = focusedPaneStyle.Copy().MarginRight(1)
	}
	if m.focus == accountsPane {
		accountsPaneStyle = focusedPaneStyle.Copy().MarginRight(1)
	}
	if m.focus == containersPane {
		containersPaneStyle = focusedPaneStyle.Copy().MarginRight(1)
	}
	if m.focus == blobsPane {
		blobsPaneStyle = focusedPaneStyle.Copy()
	}
	if m.preview.open && m.focus == previewPane {
		previewPaneStyle = focusedPaneStyle.Copy()
	}

	paneParts := []string{
		subscriptionsPaneStyle.Render(subscriptionsView),
		accountsPaneStyle.Render(accountsView),
		containersPaneStyle.Render(containersView),
		blobsPaneStyle.Render(blobsView),
	}
	if m.preview.open {
		previewTitle := m.preview.title()
		previewPaneContent := lipgloss.JoinVertical(lipgloss.Left,
			lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorAccent)).Render(previewTitle),
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

	help := "keys: tab/shift+tab focus | / filter pane | enter/l/right open->focus right | h/left left/up | a toggle load-all blobs | space toggle mark | v/V visual-line range | D download selection | preview: j/k ctrl+d/u gg G h | backspace up folder | r refresh scope | d reload subscriptions | q quit"
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

	sub := m.width / 5
	acc := m.width / 5
	con := m.width / 5
	if sub < 24 {
		sub = 24
	}
	if acc < 24 {
		acc = 24
	}
	if con < 24 {
		con = 24
	}
	marginBudget := 12
	if m.preview.open {
		marginBudget = 14
	}
	blob := m.width - sub - acc - con - marginBudget
	preview := 0
	if m.preview.open {
		preview = blob / 2
		blob = blob - preview
	}
	if blob < 40 {
		blob = 40
	}
	if m.preview.open && preview < 40 {
		preview = 40
	}

	height := m.height - 10
	if height < 8 {
		height = 8
	}

	m.subscriptionsList.SetSize(sub, height)
	m.accountsList.SetSize(acc, height)
	m.containersList.SetSize(con, height)
	m.blobsList.SetSize(blob, height)
	if m.preview.open {
		m.preview.viewport.Width = preview
		m.preview.viewport.Height = height
	}
}

func (m *Model) nextFocus() {
	if m.focus == blobsPane && m.visualLineMode {
		m.visualLineMode = false
		m.visualAnchor = ""
		m.refreshBlobItems()
	}
	m.blurAllFilters()
	count := 4
	if m.preview.open {
		count = 5
	}
	m.focus = (m.focus + 1) % count
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
		m.focus = 3
		if m.preview.open {
			m.focus = 4
		}
	}
}

func (m *Model) blurAllFilters() {
	m.subscriptionsList.FilterInput.Blur()
	m.accountsList.FilterInput.Blur()
	m.containersList.FilterInput.Blur()
	m.blobsList.FilterInput.Blur()
}

func (m *Model) commitFocusedFilter() tea.Cmd {
	m.blurAllFilters()

	switch m.focus {
	case subscriptionsPane:
		applyFilterState(&m.subscriptionsList)
		m.status = fmt.Sprintf("Filter applied for %s", paneName(m.focus))
		return nil
	case accountsPane:
		applyFilterState(&m.accountsList)
		m.status = fmt.Sprintf("Filter applied for %s", paneName(m.focus))
		return nil
	case containersPane:
		applyFilterState(&m.containersList)
		m.status = fmt.Sprintf("Filter applied for %s", paneName(m.focus))
		return nil
	case blobsPane:
		if !m.hasContainer {
			m.status = "Open a container before searching blobs"
			return nil
		}

		if m.blobLoadAll {
			applyFilterState(&m.blobsList)
			m.status = "Filter applied for blobs"
			return nil
		}

		query := strings.TrimSpace(m.blobsList.FilterValue())
		if query == "" {
			m.blobsList.ResetFilter()
			m.blobSearchQuery = ""
			m.loading = true
			m.status = fmt.Sprintf("Loading up to %d entries under %q", defaultHierarchyBlobLoadLimit, m.prefix)
			return tea.Batch(spinner.Tick, loadHierarchyBlobsCmd(m.service, m.currentAccount, m.containerName, m.prefix, defaultHierarchyBlobLoadLimit))
		}

		m.blobSearchQuery = query
		m.loading = true
		m.status = fmt.Sprintf("Searching blobs by prefix %q...", blobSearchPrefix(m.prefix, query))
		return tea.Batch(spinner.Tick, searchBlobsByPrefixCmd(m.service, m.currentAccount, m.containerName, m.prefix, query, defaultBlobPrefixSearchLimit))
	}

	return nil
}

func applyFilterState(l *list.Model) {
	if strings.TrimSpace(l.FilterValue()) == "" {
		l.SetFilterState(list.Unfiltered)
		return
	}
	l.SetFilterState(list.FilterApplied)
}

func clampListSelection(l *list.Model) {
	items := l.Items()
	if len(items) == 0 {
		l.Select(0)
		return
	}

	idx := l.Index()
	if idx < 0 {
		l.Select(0)
		return
	}
	if idx >= len(items) {
		l.Select(len(items) - 1)
	}
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

func (m *Model) resetBlobLoadState() {
	m.blobLoadAll = false
	m.blobSearchQuery = ""
}

func (m *Model) refreshBlobItems() {
	m.blobsList.SetItems(blobsToItems(m.blobs, m.prefix, m.markedBlobs, m.visualSelectionNames()))
	clampListSelection(&m.blobsList)
}

func (m Model) toggleBlobLoadAllMode() (Model, tea.Cmd) {
	if !m.hasContainer {
		m.status = "Open a container before loading blobs"
		return m, nil
	}

	m.blobsList.ResetFilter()
	m.blobSearchQuery = ""
	m.loading = true
	m.lastErr = ""

	if m.blobLoadAll {
		m.blobLoadAll = false
		m.status = fmt.Sprintf("Loading up to %d entries under %q", defaultHierarchyBlobLoadLimit, m.prefix)
		return m, tea.Batch(spinner.Tick, loadHierarchyBlobsCmd(m.service, m.currentAccount, m.containerName, m.prefix, defaultHierarchyBlobLoadLimit))
	}

	m.blobLoadAll = true
	m.status = fmt.Sprintf("Loading all blobs in %s/%s", m.currentAccount.Name, m.containerName)
	return m, tea.Batch(spinner.Tick, loadAllBlobsCmd(m.service, m.currentAccount, m.containerName, m.prefix))
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
	case subscriptionsPane:
		target = &m.subscriptionsList
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
	case subscriptionsPane:
		return m.subscriptionsList.SettingFilter()
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
	if m.focus == subscriptionsPane || !m.hasSubscription {
		m.loading = true
		m.lastErr = ""
		m.status = "Refreshing subscriptions..."
		return m, tea.Batch(spinner.Tick, loadSubscriptionsCmd(m.service))
	}

	if !m.hasAccount || m.focus == accountsPane {
		m.loading = true
		m.lastErr = ""
		m.status = fmt.Sprintf("Loading storage accounts in %s", subscriptionDisplayName(m.currentSub))
		return m, tea.Batch(spinner.Tick, loadAccountsForSubscriptionCmd(m.service, m.currentSub.ID))
	}

	if m.focus == containersPane || !m.hasContainer {
		m.loading = true
		m.lastErr = ""
		m.status = fmt.Sprintf("Loading containers in %s", m.currentAccount.Name)
		return m, tea.Batch(spinner.Tick, loadContainersCmd(m.service, m.currentAccount))
	}
	if m.focus == previewPane && m.preview.open {
		return m.ensurePreviewWindowAtCursor()
	}

	m.loading = true
	m.lastErr = ""
	if m.blobLoadAll {
		m.status = fmt.Sprintf("Loading all blobs in %s/%s", m.currentAccount.Name, m.containerName)
		return m, tea.Batch(spinner.Tick, loadAllBlobsCmd(m.service, m.currentAccount, m.containerName, m.prefix))
	}
	if m.blobSearchQuery != "" {
		effectivePrefix := blobSearchPrefix(m.prefix, m.blobSearchQuery)
		m.status = fmt.Sprintf("Searching blobs by prefix %q...", effectivePrefix)
		return m, tea.Batch(spinner.Tick, searchBlobsByPrefixCmd(m.service, m.currentAccount, m.containerName, m.prefix, m.blobSearchQuery, defaultBlobPrefixSearchLimit))
	}
	m.loading = true
	m.status = fmt.Sprintf("Loading up to %d entries under %q", defaultHierarchyBlobLoadLimit, m.prefix)
	return m, tea.Batch(spinner.Tick, loadHierarchyBlobsCmd(m.service, m.currentAccount, m.containerName, m.prefix, defaultHierarchyBlobLoadLimit))
}

func (m Model) navigateLeft() (Model, tea.Cmd) {
	switch m.focus {
	case previewPane:
		m.focus = blobsPane
		m.status = "Focus: blobs"
		return m, nil
	case blobsPane:
		if m.hasContainer && !m.blobLoadAll && m.prefix != "" {
			m.prefix = parentPrefix(m.prefix)
			m.blobSearchQuery = ""
			m.loading = true
			m.status = fmt.Sprintf("Loading up to %d entries under %q", defaultHierarchyBlobLoadLimit, m.prefix)
			return m, tea.Batch(spinner.Tick, loadHierarchyBlobsCmd(m.service, m.currentAccount, m.containerName, m.prefix, defaultHierarchyBlobLoadLimit))
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
	case accountsPane:
		m.focus = subscriptionsPane
		m.status = "Focus: subscriptions"
		return m, nil
	default:
		return m, nil
	}
}

func (m Model) handleEnter() (Model, tea.Cmd) {
	if m.focus == subscriptionsPane {
		item, ok := m.subscriptionsList.SelectedItem().(subscriptionItem)
		if !ok {
			return m, nil
		}

		m.currentSub = item.subscription
		m.hasSubscription = true
		m.hasAccount = false
		m.hasContainer = false
		m.currentAccount = azure.Account{}
		m.containerName = ""
		m.prefix = ""
		m.clearBlobSelectionState()
		m.resetBlobLoadState()
		m.resetPreviewState()
		m.focus = accountsPane

		m.accounts = nil
		m.containers = nil
		m.blobs = nil
		m.accountsList.ResetFilter()
		m.containersList.ResetFilter()
		m.blobsList.ResetFilter()
		m.accountsList.SetItems(nil)
		m.containersList.SetItems(nil)
		m.blobsList.SetItems(nil)
		m.accountsList.Title = "Storage Accounts"
		m.containersList.Title = "Containers"
		m.blobsList.Title = "Blobs"

		m.loading = true
		m.status = fmt.Sprintf("Loading storage accounts in %s", subscriptionDisplayName(item.subscription))
		return m, tea.Batch(spinner.Tick, loadAccountsForSubscriptionCmd(m.service, item.subscription.ID))
	}

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
		m.resetBlobLoadState()
		m.resetPreviewState()
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
		m.resetBlobLoadState()
		m.resetPreviewState()
		m.focus = blobsPane

		m.blobs = nil
		m.blobsList.ResetFilter()
		m.blobsList.SetItems(nil)
		m.blobsList.Title = "Blobs"

		m.loading = true
		m.status = fmt.Sprintf("Loading up to %d entries in %s/%s", defaultHierarchyBlobLoadLimit, m.currentAccount.Name, m.containerName)
		return m, tea.Batch(spinner.Tick, loadHierarchyBlobsCmd(m.service, m.currentAccount, m.containerName, m.prefix, defaultHierarchyBlobLoadLimit))
	}

	if m.focus == blobsPane {
		item, ok := m.blobsList.SelectedItem().(blobItem)
		if !ok {
			return m, nil
		}

		if item.blob.IsPrefix {
			if m.blobLoadAll {
				m.status = "Directory navigation is unavailable when all blobs are loaded"
				return m, nil
			}
			m.prefix = item.blob.Name
			m.blobSearchQuery = ""
			m.blobsList.ResetFilter()
			m.loading = true
			m.status = fmt.Sprintf("Loading up to %d entries under %q", defaultHierarchyBlobLoadLimit, m.prefix)
			return m, tea.Batch(spinner.Tick, loadHierarchyBlobsCmd(m.service, m.currentAccount, m.containerName, m.prefix, defaultHierarchyBlobLoadLimit))
		}

		return m.openPreview(item.blob)
	}

	return m, nil
}

type accountItem struct {
	account azure.Account
}

type subscriptionItem struct {
	subscription azure.Subscription
}

func (i subscriptionItem) Title() string {
	if strings.TrimSpace(i.subscription.Name) != "" {
		return i.subscription.Name
	}
	return i.subscription.ID
}

func (i subscriptionItem) Description() string {
	id := i.subscription.ID
	if len(id) > 12 {
		id = id[:12]
	}
	state := strings.TrimSpace(i.subscription.State)
	if state == "" {
		return fmt.Sprintf("id %s", id)
	}
	return fmt.Sprintf("%s | id %s", state, id)
}

func (i subscriptionItem) FilterValue() string {
	return i.subscription.Name + " " + i.subscription.ID + " " + i.subscription.State
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

func subscriptionsToItems(subscriptions []azure.Subscription) []list.Item {
	items := make([]list.Item, 0, len(subscriptions))
	for _, subscription := range subscriptions {
		items = append(items, subscriptionItem{subscription: subscription})
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

func blobSearchPrefix(currentPrefix, query string) string {
	needle := strings.TrimSpace(strings.ReplaceAll(query, "\\", "/"))
	if needle == "" {
		return strings.TrimSpace(currentPrefix)
	}
	if strings.HasPrefix(needle, "/") {
		return strings.TrimPrefix(needle, "/")
	}
	base := strings.TrimSpace(currentPrefix)
	if base == "" || strings.HasPrefix(needle, base) {
		return needle
	}
	return base + needle
}

func loadSubscriptionsCmd(svc *azure.Service) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		subscriptions, err := svc.ListSubscriptions(ctx)
		return subscriptionsLoadedMsg{subscriptions: subscriptions, err: err}
	}
}

func loadAccountsForSubscriptionCmd(svc *azure.Service, subscriptionID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		accounts, err := svc.DiscoverAccountsForSubscription(ctx, subscriptionID)
		return accountsLoadedMsg{subscriptionID: subscriptionID, accounts: accounts, err: err}
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

func loadHierarchyBlobsCmd(svc *azure.Service, account azure.Account, containerName, prefix string, limit int) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		blobs, err := svc.ListBlobsLimited(ctx, account, containerName, prefix, limit)
		return blobsLoadedMsg{account: account, container: containerName, prefix: prefix, loadAll: false, query: "", blobs: blobs, err: err}
	}
}

func loadAllBlobsCmd(svc *azure.Service, account azure.Account, containerName, prefix string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		blobs, err := svc.ListAllBlobs(ctx, account, containerName)
		return blobsLoadedMsg{account: account, container: containerName, prefix: prefix, loadAll: true, query: "", blobs: blobs, err: err}
	}
}

func searchBlobsByPrefixCmd(svc *azure.Service, account azure.Account, containerName, currentPrefix, query string, limit int) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()

		effectivePrefix := blobSearchPrefix(currentPrefix, query)
		blobs, err := svc.SearchBlobsByPrefix(ctx, account, containerName, effectivePrefix, limit)
		return blobsLoadedMsg{account: account, container: containerName, prefix: currentPrefix, loadAll: false, query: strings.TrimSpace(query), blobs: blobs, err: err}
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
	case subscriptionsPane:
		return "subscriptions"
	case accountsPane:
		return "storage accounts"
	case containersPane:
		return "containers"
	case blobsPane:
		return "blobs"
	case previewPane:
		return "preview"
	default:
		return "items"
	}
}

func subscriptionDisplayName(sub azure.Subscription) string {
	if strings.TrimSpace(sub.Name) != "" {
		return sub.Name
	}
	if strings.TrimSpace(sub.ID) == "" {
		return "-"
	}
	return sub.ID
}

func isBlobNavigationKey(key string) bool {
	switch key {
	case "up", "down", "j", "k", "pgup", "pgdown", "home", "end", "g", "G":
		return true
	default:
		return false
	}
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
