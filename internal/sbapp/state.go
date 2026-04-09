package sbapp

import (
	"fmt"

	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/azure"
	"github.com/karlssonsimon/lazyaz/internal/azure/servicebus"
	"github.com/karlssonsimon/lazyaz/internal/cache"
	"github.com/karlssonsimon/lazyaz/internal/keymap"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	"charm.land/bubbles/v2/cursor"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
)

const peekMaxMessages = 50

const (
	namespacesPane = iota
	entitiesPane
	detailPane
	// messagePreviewPane is the optional 4th pane that hosts the
	// scrolling JSON body of the currently selected message. It is only
	// part of the focus cycle while m.viewingMessage is true.
	messagePreviewPane
)

// entityFilterMode controls which kinds of entities the entities pane
// shows. Toggled via the tab strip at the top of the pane (and the
// [/] keys when focused).
type entityFilterMode int

const (
	entityFilterAll    entityFilterMode = iota // queues + topics
	entityFilterQueues                         // queues only
	entityFilterTopics                         // topics only
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
	peekedMessages []servicebus.PeekedMessage

	// topicSubsByTopic holds the subscriptions for each expanded topic
	// in the current namespace, keyed by topic name. Populated lazily
	// when the user expands a topic in the entities pane. Cleared when
	// the namespace changes.
	topicSubsByTopic map[string][]servicebus.TopicSubscription

	// expandedTopics tracks which topics in the current namespace are
	// expanded in the entities pane tree view. Cleared on namespace
	// change.
	expandedTopics map[string]bool

	// topicSubsFetching is the topic name whose subscriptions are
	// currently being fetched, used to validate incoming pages.
	topicSubsFetching string

	// Per-scope list state history. Snapshots of cursor + filter for
	// each list are stashed here when the user navigates to a different
	// scope, so returning later restores the view. Keyed by the same
	// scope identifier the cache uses.
	namespacesHistory map[string]ui.ListState // keyed by subscription ID
	entitiesHistory   map[string]ui.ListState // keyed by sub+namespace

	hasNamespace bool
	currentNS    servicebus.Namespace

	// hasPeekTarget is true when the detail pane is bound to a queue or
	// topic-subscription and showing (or about to show) its messages.
	// When false, the detail pane is empty.
	hasPeekTarget bool
	// currentEntity is the queue or topic backing the current peek.
	// For a queue peek, currentSubName is "". For a topic-sub peek,
	// currentEntity is the topic and currentSubName is the sub name.
	currentEntity  servicebus.Entity
	currentSubName string

	deadLetter bool

	// dlqSort, when true, pulls entities with DLQ messages to the top
	// of the entities pane (sorted by DLQ count desc) instead of hiding
	// the rest. Toggled with ToggleDLQFilter — the keymap name is kept
	// for backwards compatibility but the behavior is sort-not-filter.
	dlqSort bool

	// entityFilter is the currently selected tab in the entities pane —
	// All, Queues only, or Topics only. Cycled via [ / ] when focus is
	// on the entities pane.
	entityFilter entityFilterMode

	messageViewport viewport.Model
	viewingMessage  bool
	selectedMessage servicebus.PeekedMessage

	// markedMessages and duplicateMessages are scoped by peek target
	// (entity + sub + active/DLQ) so switching tabs no longer destroys
	// the user's selections. Outer key is the scope string returned by
	// markScope(...); inner set is the marked / duplicate message IDs
	// for that scope.
	markedMessages    map[string]map[string]struct{}
	duplicateMessages map[string]map[string]struct{}

	cache sbCache

	// Per-pane inspect strip toggle. When inspectPanes[pane] is true, the
	// pane renders an inline detail strip (via ui.RenderInspectStrip) under
	// its list. The strip updates live as the cursor moves so the user can
	// keep browsing while details remain visible. Toggled with K.
	inspectPanes map[int]bool

	loadingSpinnerID int

	paneWidths [4]int // ns, ent, det, preview — set by resize
	paneHeight int
}

type namespacesLoadedMsg struct {
	subscriptionID string
	namespaces     []servicebus.Namespace
	done           bool
	err            error
	next           tea.Cmd
}

type entitiesLoadedMsg struct {
	namespace servicebus.Namespace
	entities  []servicebus.Entity
	done      bool
	err       error
	next      tea.Cmd
}

type topicSubscriptionsLoadedMsg struct {
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
	// repeek is true when this peek is replacing an existing message list
	// the user was already browsing (after requeue or delete-duplicate),
	// so the handler should preserve cursor position by message ID rather
	// than resetting to the top.
	repeek bool
	err    error
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
		markedMessages:    make(map[string]map[string]struct{}),
		duplicateMessages: make(map[string]map[string]struct{}),
		cache:             newCache(db),
		namespacesHistory: make(map[string]ui.ListState),
		entitiesHistory:   make(map[string]ui.ListState),
		topicSubsByTopic:  make(map[string][]servicebus.TopicSubscription),
		expandedTopics:    make(map[string]bool),
		inspectPanes:      make(map[int]bool),
	}
	m.applyScheme(cfg.ActiveScheme())
	// Hydrate subscriptions from cache without hitting Azure.
	m.HydrateSubscriptionsFromCache(m.cache.subscriptions)
	if !m.HasSubscription {
		m.SubOverlay.Open()
		m.SetLoading(-1)
		m.loadingSpinnerID = m.NotifySpinner("Loading Azure subscriptions...")
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
				keymap.HelpEntry(km.ToggleDLQFilter, "toggle DLQ-first sort"),
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
// hydrate namespaces from cache. Tabapp calls this after constructing
// the model and before Init() issues the first fetch.
func (m *Model) SetSubscription(sub azure.Subscription) {
	m.Model.SetSubscription(sub)
	if cached, ok := m.cache.namespaces.Get(sub.ID); ok {
		m.namespaces = cached
		m.namespacesList.Title = fmt.Sprintf("Namespaces (%d)", len(cached))
		ui.SetItemsPreserveKey(&m.namespacesList, namespacesToItems(cached), namespaceItemKey)
	}
}

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.Spinner.Tick, cursor.Blink}
	if m.SubOverlay.Active {
		cmds = append(cmds, fetchSubscriptionsCmd(m.service, m.cache.subscriptions, m.Subscriptions))
	}
	if m.HasSubscription {
		cmds = append(cmds, fetchNamespacesCmd(m.service, m.cache.namespaces, m.CurrentSub.ID, m.namespaces))
	}
	return tea.Batch(cmds...)
}
