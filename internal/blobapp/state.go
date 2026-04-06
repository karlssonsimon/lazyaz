package blobapp

import (
	"azure-storage/internal/appshell"
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
	stage        int              // searchStagePrefix or searchStageFuzzy
	prefixQuery  string           // user-typed prefix for API search
	prefixLocked bool             // prefix submitted via Enter
	fuzzyQuery   string           // user-typed fuzzy filter text
	fetching     bool             // API fetch in progress
	results      []blob.BlobEntry // API results (separate from m.blobs)
	filtered     []int            // fuzzy.Filter indices into results
	totalResults int              // count before fzf filtering
}

type Model struct {
	appshell.Model

	service *blob.Service

	accountsList   list.Model
	containersList list.Model
	blobsList      list.Model

	focus int

	accounts       []blob.Account
	containers     []blob.ContainerInfo
	blobs          []blob.BlobEntry
	markedBlobs    map[string]blob.BlobEntry
	visualLineMode bool
	visualAnchor   string

	hasAccount     bool
	currentAccount blob.Account
	hasContainer   bool
	containerName  string
	prefix         string
	blobLoadAll    bool
	search         blobSearch
	preview        previewState
	pendingPreviewG bool

	cache blobCache

	paneWidths [4]int // acc, con, blob, preview — set by resize
	paneHeight int
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

	m := Model{
		Model:          appshell.New(cfg, km),
		service:        svc,
		accountsList:   accounts,
		containersList: containers,
		blobsList:      blobs,
		markedBlobs:    make(map[string]blob.BlobEntry),
		preview:        newPreviewState(),
		cache:          newCache(db),
		focus:          accountsPane,
	}
	m.applyScheme(cfg.ActiveScheme())
	// Hydrate subscriptions from cache without hitting Azure. The fetch
	// only runs when the subscription overlay is explicitly opened.
	m.HydrateSubscriptionsFromCache(m.cache.subscriptions)
	// Open the subscription picker on first run (no subscription yet).
	if !m.HasSubscription {
		m.SubOverlay.Open()
		m.SetLoading(-1)
		m.Status = "Loading Azure subscriptions..."
	}
	return m
}

// NewModelWithCache creates a Model using pre-built shared cache stores.
// Used by the tabapp to share cache data across tabs.
func NewModelWithCache(svc *blob.Service, cfg ui.Config, stores BlobStores, km keymap.Keymap) Model {
	m := NewModelWithKeyMap(svc, cfg, km, nil)
	m.cache = NewCacheWithStores(stores)
	// Re-hydrate subscriptions from the shared (SQLite-backed) store now
	// that it's wired up. The constructor's hydration above ran against a
	// temporary empty in-memory cache.
	m.HydrateSubscriptionsFromCache(m.cache.subscriptions)
	return m
}

func (m *Model) applyScheme(scheme ui.Scheme) {
	m.SetScheme(scheme)
	m.Styles.ApplyToLists([]*list.Model{
		&m.accountsList, &m.containersList, &m.blobsList,
	}, &m.Spinner)
	// Blobs list uses a custom delegate: two rows + mark/visual borders.
	m.blobsList.SetDelegate(newBlobDelegate(m.Styles.DelegateTwoRow, m.Styles))
}

// ApplyScheme applies the given scheme to all lists and spinner.
func (m *Model) ApplyScheme(scheme ui.Scheme) {
	m.applyScheme(scheme)
}

// HelpSections returns the help sections for the blob explorer.
func (m Model) HelpSections() []ui.HelpSection {
	km := m.Keymap
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

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{spinner.Tick}
	// Only fetch subscriptions from Azure if the picker is open.
	if m.SubOverlay.Active {
		cmds = append(cmds, fetchSubscriptionsCmd(m.service, m.cache.subscriptions, true))
	}
	if m.HasSubscription {
		cmds = append(cmds, fetchAccountsCmd(m.service, m.cache.accounts, m.CurrentSub.ID, false))
	}
	return tea.Batch(cmds...)
}
