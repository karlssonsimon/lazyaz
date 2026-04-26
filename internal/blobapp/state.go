package blobapp

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/azure"
	"github.com/karlssonsimon/lazyaz/internal/azure/blob"
	"github.com/karlssonsimon/lazyaz/internal/cache"
	"github.com/karlssonsimon/lazyaz/internal/keymap"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	"charm.land/bubbles/v2/cursor"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
)

const defaultBlobPrefixSearchLimit = 5000
const defaultHierarchyBlobLoadLimit = 5000

const (
	accountsPane = iota
	containersPane
	blobsPane
	previewPane
)

// InputMode represents the user's current interaction mode.
type InputMode int

const (
	ModeNormal       InputMode = iota // Browsing lists
	ModeOverlay                       // Sub/Theme/Help overlay open
	ModeActionMenu                    // Action menu open
	ModeSortOverlay                   // Sort picker open
	ModePreview                       // Blob preview focused
	ModePrefixSearch                  // Server prefix search input open
	ModeListFilter                    // User is typing a list filter
	ModeVisualLine                    // Visual line selection active
)

func (m Model) inputMode() InputMode {
	switch {
	case m.SubOverlay.Active, m.ThemeOverlay.Active, m.HelpOverlay.Active:
		return ModeOverlay
	case m.actionMenu.Active:
		return ModeActionMenu
	case m.sortOverlay.active:
		return ModeSortOverlay
	case m.preview.open && m.focus == previewPane:
		return ModePreview
	case m.filter.inputOpen && m.focus == blobsPane:
		return ModePrefixSearch
	case m.focusedListSettingFilter():
		return ModeListFilter
	case m.visualLineMode && m.focus == blobsPane:
		return ModeVisualLine
	default:
		return ModeNormal
	}
}

type blobSortField int

const (
	blobSortNone blobSortField = iota // Azure default order
	blobSortName
	blobSortSize
	blobSortDate
)

type blobFilter struct {
	inputOpen     bool             // prefix search overlay is showing
	prefixQuery   string           // API prefix query (set via action menu)
	fetching      bool             // API fetch in progress
	prefixFetched bool             // API fetch completed for current prefix
	apiResults    []blob.BlobEntry // results from API prefix search
	apiCount      int              // total API result count
}

type Model struct {
	appshell.Model

	service *blob.Service

	accountsList    list.Model
	containersList  list.Model
	blobsList       list.Model
	parentBlobsList list.Model // parent folder view (display-only, never focused)

	focus int

	accounts       []blob.Account
	containers     []blob.ContainerInfo
	blobs          []blob.BlobEntry
	markedBlobs    map[string]blob.BlobEntry
	visualLineMode bool
	visualAnchor   string

	// Per-scope list state history. The blobs list has its scope change
	// not just on account/container switches but also every time the
	// user enters or leaves a prefix ("folder"), so blobsHistory keys
	// are the full prefix path. Search mode and load-all are separate
	// scopes too — each carries its own cursor/filter memory.
	accountsHistory   map[string]ui.ListState // keyed by subscription ID
	containersHistory map[string]ui.ListState // keyed by sub+account
	blobsHistory      map[string]ui.ListState // keyed by sub+account+container+prefix+loadAll

	hasAccount       bool
	currentAccount   blob.Account
	hasContainer     bool
	containerName    string
	prefix           string
	blobLoadAll      bool
	blobSortField    blobSortField
	blobSortDesc     bool
	filter           blobFilter
	sortOverlay      sortOverlayState
	actionMenu       actionMenuState
	loadingSpinnerID int
	preview          previewState
	pendingPreviewG  bool
	textSelection    ui.TextSelection

	// downloadDir is the resolved root directory under which marked
	// blobs are saved. Set once at construction time from
	// ui.Config.ResolvedDownloadDir(). May be empty if neither the
	// configured nor the default OS Downloads folder could be
	// resolved — in that case the download action surfaces an error
	// rather than silently picking a fallback path.
	downloadDir string

	cache blobCache

	// usage records every drill-in (account / container) so the
	// dashboard can surface frequently-used resources. nil when the
	// parent runs in-memory.
	usage *cache.DB

	// pendingNav is set by the parent app (via SetPendingNav) when
	// the dashboard wants this tab to navigate to a specific resource.
	// advancePendingNav drives the selection forward as fetches land.
	pendingNav PendingNav

	// applyingNav suppresses RecordJumpMsg emission while ApplyNav
	// (jump-list restoration) is driving navigation. See sbapp's
	// equivalent field for the full rationale.
	applyingNav bool

	// Per-pane inspect strip toggle. When inspectPanes[pane] is true, the
	// pane renders an inline detail strip (via ui.RenderInspectStrip) under
	// its list. The strip updates live as the cursor moves so the user can
	// keep browsing while details remain visible. Toggled with K.
	inspectPanes map[int]bool

	clickTracker ui.ClickTracker
	paneWidths   [4]int // acc, con, blob, preview — set by resize
	paneHeight   int

	// Upload state. The browser, conflict prompt, and progress panel are
	// all driven from these fields. nil/false when no upload is in flight.
	uploadBrowser        ui.FileBrowserState
	uploadBrowserActive  bool
	uploadProgress       *uploadProgress
	uploadActivityUnreg  func() // nil when no upload is tracked
	uploadConflict       *pendingConflict
	uploadConflictPolicy conflictAnswer
	uploadCancelFn       context.CancelFunc

	// CRUD modal state. Only one of these is ever active at a time.
	// On Confirm/Submit, the pending closure runs and emits a tea.Cmd
	// that results in a crudDoneMsg.
	confirmModal    ui.ConfirmModalState
	confirmAction   func() tea.Cmd
	textInput       ui.TextInputState
	textInputAction func(value string) tea.Cmd
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

type blobContentClipboardMsg struct {
	blobName string
	content  string
	err      error
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
	if svc == nil {
		svc = blob.NewService(nil)
	}
	delegate := list.NewDefaultDelegate()

	accounts := list.New([]list.Item{}, delegate, 24, 10)
	accounts.SetShowTitle(false) // title is rendered by ui.RenderListPane
	accounts.SetShowHelp(false)
	accounts.SetShowPagination(false)
	accounts.SetShowStatusBar(true)
	accounts.SetStatusBarItemName("account", "accounts")
	accounts.SetFilteringEnabled(true)
	accounts.DisableQuitKeybindings()

	containers := list.New([]list.Item{}, delegate, 24, 10)
	containers.SetShowTitle(false)
	containers.SetShowHelp(false)
	containers.SetShowPagination(false)
	containers.SetShowStatusBar(true)
	containers.SetStatusBarItemName("container", "containers")
	containers.SetFilteringEnabled(true)
	containers.DisableQuitKeybindings()

	blobs := list.New([]list.Item{}, delegate, 40, 10)
	blobs.SetShowTitle(false)
	blobs.SetShowHelp(false)
	blobs.SetShowPagination(false)
	blobs.SetShowStatusBar(true)
	blobs.SetStatusBarItemName("entry", "entries")
	blobs.SetFilteringEnabled(true)
	blobs.Filter = blobListFilter
	blobs.DisableQuitKeybindings()

	parentBlobs := list.New([]list.Item{}, delegate, 20, 10)
	parentBlobs.SetShowTitle(false)
	parentBlobs.SetShowHelp(false)
	parentBlobs.SetShowPagination(false)
	parentBlobs.SetShowStatusBar(false)
	parentBlobs.SetFilteringEnabled(false)
	parentBlobs.DisableQuitKeybindings()

	m := Model{
		Model:             appshell.New(cfg, km),
		service:           svc,
		accountsList:      accounts,
		containersList:    containers,
		blobsList:         blobs,
		parentBlobsList:   parentBlobs,
		markedBlobs:       make(map[string]blob.BlobEntry),
		preview:           newPreviewState(),
		cache:             newCache(db),
		downloadDir:       cfg.ResolvedDownloadDir(),
		focus:             accountsPane,
		blobSortField:     blobSortDate,
		blobSortDesc:      true,
		accountsHistory:   make(map[string]ui.ListState),
		containersHistory: make(map[string]ui.ListState),
		blobsHistory:      make(map[string]ui.ListState),
		inspectPanes:      make(map[int]bool),
	}
	m.applyScheme(cfg.ActiveScheme())
	// Hydrate subscriptions from cache without hitting Azure. The fetch
	// only runs when the subscription overlay is explicitly opened.
	m.HydrateSubscriptionsFromCache(m.cache.subscriptions)
	// Open the subscription picker on first run (no subscription yet).
	if !m.HasSubscription {
		m.SubOverlay.Open()
		m.startLoading(-1, "Loading Azure subscriptions...")
	}
	return m
}

// NewModelWithCache creates a Model using pre-built shared cache stores.
// Used by the tabapp to share cache data across tabs.
func NewModelWithCache(svc *blob.Service, cfg ui.Config, stores BlobStores, km keymap.Keymap) Model {
	m := NewModelWithKeyMap(svc, cfg, km, nil)
	m.cache = NewCacheWithStores(stores)
	m.usage = stores.Usage
	// Re-hydrate subscriptions from the shared (SQLite-backed) store now
	// that it's wired up. The constructor's hydration above ran against a
	// temporary empty in-memory cache.
	m.HydrateSubscriptionsFromCache(m.cache.subscriptions)
	return m
}

func (m *Model) SetCredential(cred azcore.TokenCredential) {
	if m.service != nil {
		m.service.SetCredential(cred)
	}
}

func (m Model) WithCredential(cred azcore.TokenCredential) tea.Model {
	m.SetCredential(cred)
	return m
}

func (m Model) WithNotification(level appshell.NotificationLevel, message string) tea.Model {
	m.Notify(level, message)
	return m
}

func (m *Model) applyScheme(scheme ui.Scheme) {
	m.SetScheme(scheme)
	m.Styles.ApplyToLists([]*list.Model{
		&m.accountsList, &m.containersList, &m.blobsList, &m.parentBlobsList,
	}, &m.Spinner)
	// Blobs list uses a custom delegate for mark/visual borders.
	// Preserve existing mark/visual state across scheme changes.
	d := newBlobDelegate(m.Styles.Delegate, m.Styles)
	d.marked = m.markedBlobs
	d.visual = m.visualSelectionNames()
	m.blobsList.SetDelegate(d)
}

// ApplyScheme applies the given scheme to all lists and spinner.
func (m *Model) ApplyScheme(scheme ui.Scheme) {
	m.applyScheme(scheme)
}

func (m Model) WithScheme(scheme ui.Scheme) tea.Model {
	m.ApplyScheme(scheme)
	return m
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
				keymap.HelpEntry(km.ActionMenu, "action menu"),
				keymap.HelpEntry(km.SortBlobs, "sort blobs"),
				keymap.HelpEntry(km.ToggleLoadAll, "toggle load-all blobs"),
				keymap.HelpEntry(km.ToggleMark, "toggle mark on current blob"),
				keymap.HelpEntry(km.ToggleVisualLine, "start/end visual-line selection"),
				keymap.HelpEntry(km.ExitVisualLine, "exit visual mode"),
				keymap.HelpEntry(km.DownloadSelection, "download marked/visual selection"),
				keymap.HelpEntry(km.YankBlobContent, "yank blob content to clipboard"),
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

// SetSubscription overrides the embedded appshell.Model method to also
// hydrate accounts from cache. Tabapp calls this after constructing the
// model and before Init() issues the first fetch.
func (m *Model) SetSubscription(sub azure.Subscription) {
	m.Model.SetSubscription(sub)
	// Scope the credential to the subscription's tenant so ARM and data
	// plane calls authenticate against the correct directory.
	if m.service != nil && sub.TenantID != "" {
		if cred, err := azure.NewCredentialForTenant(sub.TenantID); err == nil {
			m.service.SetCredential(cred)
		}
	}
	if cached, ok := m.cache.accounts.Get(sub.ID); ok {
		m.accounts = cached
		m.accountsList.Title = fmt.Sprintf("Storage Accounts (%d)", len(cached))
		ui.SetItemsPreserveKey(&m.accountsList, accountsToItems(cached), accountItemKey)
	}
}

func (m Model) WithSubscription(sub azure.Subscription) tea.Model {
	m.SetSubscription(sub)
	return m
}

func (m Model) WithSubscriptions(subs []azure.Subscription) tea.Model {
	m.Subscriptions = subs
	return m
}

func (m Model) WithoutSubscription(subs []azure.Subscription) tea.Model {
	m.HasSubscription = false
	m.CurrentSub = azure.Subscription{}
	m.Subscriptions = subs
	m.SubOverlay.Open()
	return m
}

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.Spinner.Tick, cursor.Blink}
	// Only fetch subscriptions from Azure if the picker is open.
	if m.SubOverlay.Active {
		cmds = append(cmds, fetchSubscriptionsCmd(m.service, m.cache.subscriptions, m.Subscriptions))
	}
	if m.HasSubscription {
		cmds = append(cmds, fetchAccountsCmd(m.service, m.cache.accounts, m.CurrentSub.ID, m.accounts))
	}
	return tea.Batch(cmds...)
}
