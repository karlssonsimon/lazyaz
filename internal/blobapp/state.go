package blobapp

import (
	"azure-storage/internal/azure"
	"azure-storage/internal/azure/blob"
	"azure-storage/internal/cache"
	"azure-storage/internal/keymap"
	"azure-storage/internal/ui"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

const defaultDownloadRoot = "downloads"
const defaultBlobPrefixSearchLimit = 500
const defaultHierarchyBlobLoadLimit = 500

const (
	accountsPane = iota
	containersPane
	blobsPane
	previewPane
)

const (
	searchStagePrefix = 0
	searchStageFuzzy  = 1
)

type blobSearch struct {
	active       bool
	stage        int               // searchStagePrefix or searchStageFuzzy
	prefixQuery  string            // user-typed prefix for API search
	prefixLocked bool              // prefix submitted via Enter
	fuzzyQuery   string            // user-typed fuzzy filter text
	fetching     bool              // API fetch in progress
	results      []blob.BlobEntry  // API results (separate from m.blobs)
	filtered     []int             // fuzzy.Filter indices into results
	totalResults int               // count before fzf filtering
}

type Model struct {
	service *blob.Service

	spinner spinner.Model

	accountsList   list.Model
	containersList list.Model
	blobsList      list.Model

	focus int

	subscriptions   []azure.Subscription
	accounts        []blob.Account
	containers      []blob.ContainerInfo
	blobs           []blob.BlobEntry
	markedBlobs     map[string]blob.BlobEntry
	visualLineMode  bool
	visualAnchor    string
	hasSubscription bool
	currentSub      azure.Subscription
	hasAccount      bool
	currentAccount  blob.Account
	hasContainer    bool
	containerName   string
	prefix      string
	blobLoadAll bool
	search      blobSearch
	preview     previewState
	pendingPreviewG bool
	keymap          keymap.Keymap
	styles          ui.Styles

	schemes      []ui.Scheme
	themeOverlay ui.ThemeOverlayState
	helpOverlay  ui.HelpOverlayState
	subOverlay   ui.SubscriptionOverlayState

	inspectFields []ui.InspectField
	inspectTitle  string

	cache blobCache

	// EmbeddedMode suppresses theme/help overlay handling and quit
	// interception so the parent tabapp can own those concerns.
	EmbeddedMode bool

	loading bool
	status  string
	lastErr string

	width      int
	height     int
	paneWidths [4]int // acc, con, blob, preview — set by resize
	paneHeight int
}

type subscriptionsLoadedMsg struct {
	subscriptions []azure.Subscription
	done          bool
	err           error
	next          tea.Cmd
}

type accountsLoadedMsg struct {
	subscriptionID string
	accounts       []blob.Account
	done           bool
	err            error
	next           tea.Cmd
}

type containersLoadedMsg struct {
	account    blob.Account
	containers []blob.ContainerInfo
	done       bool
	err        error
	next       tea.Cmd
}

type blobsLoadedMsg struct {
	account   blob.Account
	container string
	prefix    string
	loadAll   bool
	query     string
	blobs     []blob.BlobEntry
	done      bool
	err       error
	next      tea.Cmd
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
	account     blob.Account
	container   string
	blobName    string
	blobSize    int64
	contentType string
	windowStart int64
	cursor      int64
	data        []byte
	err         error
}

func NewModel(svc *blob.Service, cfg ui.Config, db *cache.DB) Model {
	return NewModelWithKeyMap(svc, cfg, keymap.Default(), db)
}

func NewModelWithKeyMap(svc *blob.Service, cfg ui.Config, km keymap.Keymap, db *cache.DB) Model {
	delegate := list.NewDefaultDelegate()

	accounts := list.New([]list.Item{}, delegate, 24, 10)
	accounts.Title = "Storage Accounts"
	accounts.SetShowHelp(false)
	accounts.SetShowPagination(false)
	accounts.SetShowStatusBar(true)
	accounts.SetStatusBarItemName("account", "accounts")
	accounts.SetFilteringEnabled(true)
	accounts.DisableQuitKeybindings()

	containers := list.New([]list.Item{}, delegate, 24, 10)
	containers.Title = "Containers"
	containers.SetShowHelp(false)
	containers.SetShowPagination(false)
	containers.SetShowStatusBar(true)
	containers.SetStatusBarItemName("container", "containers")
	containers.SetFilteringEnabled(true)
	containers.DisableQuitKeybindings()

	blobs := list.New([]list.Item{}, delegate, 40, 10)
	blobs.Title = "Blobs"
	blobs.SetShowHelp(false)
	blobs.SetShowPagination(false)
	blobs.SetShowStatusBar(true)
	blobs.SetStatusBarItemName("entry", "entries")
	blobs.SetFilteringEnabled(false)
	blobs.DisableQuitKeybindings()

	spin := spinner.New()
	spin.Spinner = spinner.Dot

	m := Model{
		service:        svc,
		spinner:        spin,
		accountsList:   accounts,
		containersList: containers,
		blobsList:      blobs,
		markedBlobs:    make(map[string]blob.BlobEntry),
		preview:        newPreviewState(),
		cache:          newCache(db),
		keymap:         km,
		schemes:        cfg.Schemes,
		themeOverlay: ui.ThemeOverlayState{
			ActiveThemeIdx: ui.ActiveSchemeIndex(cfg),
		},
		focus:   accountsPane,
		status:  "Loading Azure subscriptions...",
		loading: true,
	}
	m.applyScheme(cfg.ActiveScheme())
	return m
}

// NewModelWithCache creates a Model using pre-built shared cache stores.
// Used by the tabapp to share cache data across tabs.
func NewModelWithCache(svc *blob.Service, cfg ui.Config, stores BlobStores, km keymap.Keymap) Model {
	m := NewModelWithKeyMap(svc, cfg, km, nil)
	m.cache = NewCacheWithStores(stores)
	return m
}

func (m *Model) applyScheme(scheme ui.Scheme) {
	m.styles = ui.NewStyles(scheme)
	m.styles.ApplyToLists([]*list.Model{
		&m.accountsList, &m.containersList, &m.blobsList,
	}, &m.spinner)
	// Blobs list uses two-row delegate for title + metadata on separate lines.
	m.blobsList.SetDelegate(m.styles.DelegateTwoRow)
}

// ApplyScheme applies the given scheme to all lists and spinner.
func (m *Model) ApplyScheme(scheme ui.Scheme) {
	m.applyScheme(scheme)
}

// HelpSections returns the help sections for the blob explorer.
func (m Model) HelpSections() []ui.HelpSection {
	km := m.keymap
	return []ui.HelpSection{
		{
			Title: "Navigation",
			Items: []string{
				keymap.HelpEntry(km.NextFocus, "next focus"),
				keymap.HelpEntry(km.PreviousFocus, "previous focus"),
				keymap.HelpEntry(km.FilterInput, "filter focused pane"),
				keymap.HelpEntry(keymap.New(km.OpenFocused.Label()+"/"+km.OpenFocusedAlt.Label()), "open and move right"),
				keymap.HelpEntry(km.NavigateLeft, "go left/back"),
				keymap.HelpEntry(km.BackspaceUp, "up one folder"),
				keymap.HelpEntry(keymap.New(km.HalfPageDown.Label()+"/"+km.HalfPageUp.Label()), "half-page scroll"),
			},
		},
		{
			Title: "Blob Actions",
			Items: []string{
				keymap.HelpEntry(km.ToggleLoadAll, "toggle load-all blobs"),
				keymap.HelpEntry(km.ToggleMark, "toggle mark on current blob"),
				keymap.HelpEntry(km.ToggleVisualLine, "start/end visual-line selection"),
				keymap.HelpEntry(km.ExitVisualLine, "exit visual mode"),
				keymap.HelpEntry(km.DownloadSelection, "download marked/visual selection"),
			},
		},
		{
			Title: "Preview",
			Items: []string{
				keymap.HelpEntry(km.PreviewNextFocus, "next preview focus"),
				keymap.HelpEntry(km.PreviewPreviousFocus, "previous preview focus"),
				keymap.HelpEntry(keymap.New(km.PreviewDown.Label()+"/"+km.PreviewUp.Label()), "scroll preview"),
				keymap.HelpEntry(keymap.New(km.HalfPageDown.Label()+"/"+km.HalfPageUp.Label()), "half-page preview scroll"),
				keymap.HelpEntry(km.PreviewTopPrefix, "go to top with gg"),
				keymap.HelpEntry(km.PreviewBottom, "go to bottom"),
				keymap.HelpEntry(km.PreviewBack, "close preview / go back"),
			},
		},
		{
			Title: "App",
			Items: []string{
				keymap.HelpEntry(km.Inspect, "inspect item"),
				keymap.HelpEntry(km.SubscriptionPicker, "change subscription"),
				keymap.HelpEntry(km.ToggleThemePicker, "open theme picker"),
				keymap.HelpEntry(km.RefreshScope, "refresh current scope"),
				keymap.HelpEntry(km.ReloadSubscriptions, "reload subscriptions"),
				keymap.HelpEntry(km.ToggleHelp, "toggle help"),
				keymap.HelpEntry(km.Quit, "quit"),
			},
		},
	}
}

// CurrentSubscription returns the active subscription and whether one is set.
func (m Model) CurrentSubscription() (azure.Subscription, bool) {
	return m.currentSub, m.hasSubscription
}

// SetSubscription sets the active subscription without triggering navigation.
func (m *Model) SetSubscription(sub azure.Subscription) {
	m.currentSub = sub
	m.hasSubscription = true
}

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{spinner.Tick, fetchSubscriptionsCmd(m.service, m.cache.subscriptions)}
	if m.hasSubscription {
		cmds = append(cmds, fetchAccountsCmd(m.service, m.cache.accounts, m.currentSub.ID))
	}
	return tea.Batch(cmds...)
}
