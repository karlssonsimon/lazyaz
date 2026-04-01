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
	palette         ui.Palette
	syntaxStyles    ui.SyntaxStyles

	appName      string
	themes       []ui.Theme
	themeOverlay ui.ThemeOverlayState
	helpOverlay  ui.HelpOverlayState

	cache blobCache

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
	accounts       []blob.Account
	err            error
}

type containersLoadedMsg struct {
	account    blob.Account
	containers []blob.ContainerInfo
	err        error
}

type blobsLoadedMsg struct {
	account   blob.Account
	container string
	prefix    string
	loadAll   bool
	query     string
	blobs     []blob.BlobEntry
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
		appName:           cfg.AppName,
		themes:            cfg.Themes,
		themeOverlay: ui.ThemeOverlayState{
			ActiveThemeIdx: ui.ActiveThemeIndex(cfg),
		},
		focus:   subscriptionsPane,
		status:  "Loading Azure subscriptions...",
		loading: true,
	}
	m.applyTheme(cfg.ActiveTheme())
	return m
}

func (m *Model) applyTheme(theme ui.Theme) {
	m.palette, m.syntaxStyles = ui.ApplyThemeToLists(theme, []*list.Model{
		&m.subscriptionsList, &m.accountsList, &m.containersList, &m.blobsList,
	}, &m.spinner)
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(spinner.Tick, loadSubscriptionsCmd(m.service))
}
