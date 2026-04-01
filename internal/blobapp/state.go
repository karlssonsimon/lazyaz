package blobapp

import (
	"azure-storage/internal/azure"
	"azure-storage/internal/azure/blob"
	"azure-storage/internal/ui"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
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

type Model struct {
	service *blob.Service

	spinner spinner.Model

	subscriptionsList list.Model
	accountsList      list.Model
	containersList    list.Model
	blobsList         list.Model

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
	prefix          string
	blobLoadAll     bool
	blobSearchQuery string
	preview         previewState
	pendingPreviewG bool
	keymap          KeyMap
	styles          ui.Styles

	schemes      []ui.Scheme
	themeOverlay ui.ThemeOverlayState
	helpOverlay  ui.HelpOverlayState

	cache blobCache

	// EmbeddedMode suppresses theme/help overlay handling and quit
	// interception so the parent tabapp can own those concerns.
	EmbeddedMode bool

	loading bool
	status  string
	lastErr string

	width  int
	height int
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

func NewModel(svc *blob.Service, cfg ui.Config) Model {
	return NewModelWithKeyMap(svc, cfg, DefaultKeyMap())
}

func NewModelWithKeyMap(svc *blob.Service, cfg ui.Config, keymap KeyMap) Model {
	delegate := list.NewDefaultDelegate()

	subscriptions := list.New([]list.Item{}, delegate, 28, 10)
	subscriptions.Title = "Subscriptions"
	subscriptions.SetShowHelp(false)
	subscriptions.SetShowPagination(false)
	subscriptions.SetShowStatusBar(true)
	subscriptions.SetStatusBarItemName("subscription", "subscriptions")
	subscriptions.SetFilteringEnabled(true)
	subscriptions.DisableQuitKeybindings()

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
	blobs.SetFilteringEnabled(true)
	blobs.DisableQuitKeybindings()

	spin := spinner.New()
	spin.Spinner = spinner.Dot

	m := Model{
		service:           svc,
		spinner:           spin,
		subscriptionsList: subscriptions,
		accountsList:      accounts,
		containersList:    containers,
		blobsList:         blobs,
		markedBlobs:       make(map[string]blob.BlobEntry),
		preview:           newPreviewState(),
		cache:             newCache(),
		keymap:            keymap,
		schemes:           cfg.Schemes,
		themeOverlay: ui.ThemeOverlayState{
			ActiveThemeIdx: ui.ActiveSchemeIndex(cfg),
		},
		focus:   subscriptionsPane,
		status:  "Loading Azure subscriptions...",
		loading: true,
	}
	m.applyScheme(cfg.ActiveScheme())
	return m
}

// NewModelWithCache creates a Model using pre-built shared cache stores.
// Used by the tabapp to share cache data across tabs.
func NewModelWithCache(svc *blob.Service, cfg ui.Config, stores BlobStores) Model {
	m := NewModel(svc, cfg)
	m.cache = NewCacheWithStores(stores)
	return m
}

func (m *Model) applyScheme(scheme ui.Scheme) {
	m.styles = ui.NewStyles(scheme)
	m.styles.ApplyToLists([]*list.Model{
		&m.subscriptionsList, &m.accountsList, &m.containersList, &m.blobsList,
	}, &m.spinner)
}

// ApplyScheme applies the given scheme to all lists and spinner.
func (m *Model) ApplyScheme(scheme ui.Scheme) {
	m.applyScheme(scheme)
}

// HelpSections returns the help sections for the blob explorer.
func (m Model) HelpSections() []ui.HelpSection {
	return m.keymap.HelpSections()
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(spinner.Tick, fetchSubscriptionsCmd(m.service, m.cache.subscriptions))
}
