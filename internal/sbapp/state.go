package sbapp

import (
	"fmt"

	"azure-storage/internal/appshell"
	"azure-storage/internal/azure"
	"azure-storage/internal/azure/servicebus"
	"azure-storage/internal/cache"
	"azure-storage/internal/keymap"
	"azure-storage/internal/ui"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

const peekMaxMessages = 50

const (
	namespacesPane = iota
	entitiesPane
	detailPane
)

type detailView int

const (
	detailMessages           detailView = iota // peeked messages (queue or topic sub)
	detailTopicSubscriptions                   // list of topic subscriptions
)

type Model struct {
	appshell.Model

	service *servicebus.Service

	namespacesList list.Model
	entitiesList   list.Model
	detailList     list.Model

	focus int

	namespaces     []servicebus.Namespace
	entities       []servicebus.Entity
	topicSubs      []servicebus.TopicSubscription
	peekedMessages []servicebus.PeekedMessage

	// Streaming-refresh sessions. See cache.FetchSession. Peeked messages
	// are deliberately excluded — they're ephemeral and use plain replace.
	namespacesSession *cache.FetchSession[servicebus.Namespace]
	entitiesSession   *cache.FetchSession[servicebus.Entity]
	topicSubsSession  *cache.FetchSession[servicebus.TopicSubscription]

	// Per-scope list state history. Snapshots of cursor + filter for
	// each list are stashed here when the user navigates to a different
	// scope, so returning later restores the view. Keyed by the same
	// scope identifier the cache uses.
	namespacesHistory map[string]ui.ListState // keyed by subscription ID
	entitiesHistory   map[string]ui.ListState // keyed by sub+namespace
	topicSubsHistory  map[string]ui.ListState // keyed by sub+namespace+entity

	// fetchGen is the monotonic generation token copied into each fetch
	// so pages from superseded or cancelled fetches can be dropped.
	fetchGen int

	hasNamespace  bool
	currentNS     servicebus.Namespace
	hasEntity     bool
	currentEntity servicebus.Entity

	detailMode      detailView
	viewingTopicSub bool
	currentTopicSub servicebus.TopicSubscription
	deadLetter      bool
	dlqFilter       bool

	messageViewport   viewport.Model
	viewingMessage    bool
	selectedMessage   servicebus.PeekedMessage
	markedMessages    map[string]struct{}
	duplicateMessages map[string]struct{}

	cache sbCache

	paneWidths [4]int // ns, ent, det, preview — set by resize
	paneHeight int
}

type namespacesLoadedMsg struct {
	gen            int
	subscriptionID string
	namespaces     []servicebus.Namespace
	done           bool
	err            error
	next           tea.Cmd
}

type entitiesLoadedMsg struct {
	gen       int
	namespace servicebus.Namespace
	entities  []servicebus.Entity
	done      bool
	err       error
	next      tea.Cmd
}

type topicSubscriptionsLoadedMsg struct {
	gen       int
	namespace servicebus.Namespace
	topicName string
	subs      []servicebus.TopicSubscription
	done      bool
	err       error
	next      tea.Cmd
}

type messagesLoadedMsg struct {
	namespace servicebus.Namespace
	source    string
	messages  []servicebus.PeekedMessage
	err       error
}

type requeueDoneMsg struct {
	requeued int
	total    int
	err      error
}

type deleteDuplicateDoneMsg struct {
	messageID string
	err       error
}

type entitiesRefreshedMsg struct {
	entities []servicebus.Entity
	err      error
}

func NewModel(svc *servicebus.Service, cfg ui.Config, db *cache.DB) Model {
	return NewModelWithKeyMap(svc, cfg, keymap.Default(), db)
}

func NewModelWithKeyMap(svc *servicebus.Service, cfg ui.Config, km keymap.Keymap, db *cache.DB) Model {
	delegate := list.NewDefaultDelegate()

	namespaces := list.New([]list.Item{}, delegate, 24, 10)
	namespaces.SetShowTitle(false) // title is rendered by ui.RenderListPane
	namespaces.SetShowHelp(false)
	namespaces.SetShowPagination(false)
	namespaces.SetShowStatusBar(true)
	namespaces.SetStatusBarItemName("namespace", "namespaces")
	namespaces.SetFilteringEnabled(true)
	namespaces.DisableQuitKeybindings()

	entities := list.New([]list.Item{}, delegate, 24, 10)
	entities.SetShowTitle(false)
	entities.SetShowHelp(false)
	entities.SetShowPagination(false)
	entities.SetShowStatusBar(true)
	entities.SetStatusBarItemName("entity", "entities")
	entities.SetFilteringEnabled(true)
	entities.DisableQuitKeybindings()

	detail := list.New([]list.Item{}, delegate, 40, 10)
	detail.SetShowTitle(false)
	detail.SetShowHelp(false)
	detail.SetShowPagination(false)
	detail.SetShowStatusBar(true)
	detail.SetStatusBarItemName("item", "items")
	detail.SetFilteringEnabled(true)
	detail.DisableQuitKeybindings()

	m := Model{
		Model:             appshell.New(cfg, km),
		service:           svc,
		namespacesList:    namespaces,
		entitiesList:      entities,
		detailList:        detail,
		focus:             namespacesPane,
		markedMessages:    make(map[string]struct{}),
		duplicateMessages: make(map[string]struct{}),
		cache:             newCache(db),
		namespacesHistory: make(map[string]ui.ListState),
		entitiesHistory:   make(map[string]ui.ListState),
		topicSubsHistory:  make(map[string]ui.ListState),
	}
	m.applyScheme(cfg.ActiveScheme())
	// Hydrate subscriptions from cache without hitting Azure.
	m.HydrateSubscriptionsFromCache(m.cache.subscriptions)
	if !m.HasSubscription {
		m.SubOverlay.Open()
		m.SetLoading(-1)
		m.Status = "Loading Azure subscriptions..."
	}
	return m
}

// NewModelWithCache creates a Model using pre-built shared cache stores.
func NewModelWithCache(svc *servicebus.Service, cfg ui.Config, stores SBStores, km keymap.Keymap) Model {
	m := NewModelWithKeyMap(svc, cfg, km, nil)
	m.cache = NewCacheWithStores(stores)
	// Re-hydrate subscriptions from the shared store.
	m.HydrateSubscriptionsFromCache(m.cache.subscriptions)
	return m
}

func (m *Model) applyScheme(scheme ui.Scheme) {
	m.SetScheme(scheme)
	m.Styles.ApplyToLists([]*list.Model{
		&m.namespacesList, &m.entitiesList, &m.detailList,
	}, &m.Spinner)
}

// ApplyScheme applies the given scheme to all lists and spinner.
func (m *Model) ApplyScheme(scheme ui.Scheme) {
	m.applyScheme(scheme)
}

// HelpSections returns the help sections for the service bus explorer.
func (m Model) HelpSections() []ui.HelpSection {
	km := m.Keymap
	return []ui.HelpSection{
		{
			Title: "Navigation",
			Items: []string{
				keymap.HelpEntry(km.NextFocus, "next focus"),
				keymap.HelpEntry(km.PreviousFocus, "previous focus"),
				keymap.HelpEntry(km.FilterInput, "filter focused pane"),
				keymap.HelpEntry(keymap.New(km.OpenFocused.Label()+"/"+km.OpenFocusedAlt.Label()), "open selected item"),
				keymap.HelpEntry(km.NavigateLeft, "go back"),
				keymap.HelpEntry(km.BackspaceUp, "backspace navigation"),
				keymap.HelpEntry(keymap.New(km.HalfPageDown.Label()+"/"+km.HalfPageUp.Label()), "half-page scroll"),
			},
		},
		{
			Title: "Messages",
			Items: []string{
				keymap.HelpEntry(km.ToggleMark, "mark message"),
				keymap.HelpEntry(keymap.New(km.ShowActiveQueue.Label()+"/"+km.ShowDeadLetterQueue.Label()), "switch active and DLQ"),
				keymap.HelpEntry(km.ToggleDLQFilter, "toggle entities with DLQ only"),
				keymap.HelpEntry(km.RequeueDLQ, "requeue marked/current DLQ messages"),
				keymap.HelpEntry(km.DeleteDuplicate, "delete duplicate DLQ message"),
				keymap.HelpEntry(km.MessageBack, "close message preview"),
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
// hydrate namespaces from cache and prime the initial namespaces fetch
// session. Tabapp calls this after constructing the model and before
// Init() issues the first fetch.
func (m *Model) SetSubscription(sub azure.Subscription) {
	m.Model.SetSubscription(sub)
	if cached, ok := m.cache.namespaces.Get(sub.ID); ok {
		m.namespaces = cached
		m.namespacesList.Title = fmt.Sprintf("Namespaces (%d)", len(cached))
		ui.SetItemsPreserveKey(&m.namespacesList, namespacesToItems(cached), namespaceItemKey)
	}
	m.fetchGen++
	m.namespacesSession = cache.NewFetchSession(m.namespaces, m.fetchGen, namespaceKey)
}

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{spinner.Tick}
	if m.SubOverlay.Active {
		cmds = append(cmds, fetchSubscriptionsCmd(m.service, m.cache.subscriptions, true))
	}
	if m.HasSubscription {
		cmds = append(cmds, fetchNamespacesCmd(m.service, m.cache.namespaces, m.CurrentSub.ID, m.fetchGen))
	}
	return tea.Batch(cmds...)
}
