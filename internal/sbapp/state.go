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
	subscriptionsPane  // only visible when a topic is selected
	queueTypePane      // Active / DLQ picker
	messagesPane       // messages from selected queue type
	messagePreviewPane // optional scrolling JSON preview
)

// InputMode represents the user's current interaction mode.
type InputMode int

const (
	ModeNormal         InputMode = iota // Browsing lists
	ModeOverlay                         // Sub/Theme/Help overlay open
	ModeSortOverlay                     // Entity sort picker open
	ModeTargetPicker                    // Target entity picker open
	ModeActionMenu                      // Action menu open
	ModeMessagePreview                  // Viewing message detail
	ModeListFilter                      // User is typing a list filter
	ModeVisualLine                      // Visual line selection active
)

func (m Model) inputMode() InputMode {
	switch {
	case m.SubOverlay.Active, m.ThemeOverlay.Active, m.HelpOverlay.Active:
		return ModeOverlay
	case m.entitySortOverlay.active:
		return ModeSortOverlay
	case m.targetPicker.active:
		return ModeTargetPicker
	case m.actionMenu.active:
		return ModeActionMenu
	case m.viewingMessage && m.focus == messagePreviewPane:
		return ModeMessagePreview
	case m.focusedListSettingFilter():
		return ModeListFilter
	case m.visualLineMode && m.focus == messagesPane:
		return ModeVisualLine
	default:
		return ModeNormal
	}
}

type Model struct {
	appshell.Model

	service *servicebus.Service

	namespacesList    list.Model
	entitiesList      list.Model
	subscriptionsList list.Model // topic subscriptions
	queueTypeList     list.Model // Active / DLQ picker (2 items)
	messageList       list.Model // messages from selected queue type

	focus int

	namespaces    []servicebus.Namespace
	entities      []servicebus.Entity
	subscriptions []servicebus.TopicSubscription // subs for selected topic

	peekedMessages []servicebus.PeekedMessage

	// Per-scope list state history.
	namespacesHistory    map[string]ui.ListState
	entitiesHistory      map[string]ui.ListState
	subscriptionsHistory map[string]ui.ListState

	hasNamespace bool
	currentNS    servicebus.Namespace

	// hasPeekTarget is true when the queue type picker is bound to
	// a queue or topic-subscription.
	hasPeekTarget bool
	currentEntity  servicebus.Entity
	currentSubName string

	// deadLetter is true when the user selected "DLQ" in the queue
	// type picker.
	deadLetter bool

	visualLineMode bool
	visualAnchor   string // message ID of the anchor

	// lockedMessages holds the result of a receive-with-lock operation.
	// Non-nil means the user has received DLQ messages with locks held.
	// The receiver must be closed (abandonAll + close) when navigating
	// away or when the user explicitly abandons.
	lockedMessages *servicebus.ReceivedMessages

	entitySortField entitySortField
	entitySortDesc  bool
	entityDLQFilter bool // show only entities with dead letters

	entitySortOverlay entitySortOverlayState

	messageViewport viewport.Model
	viewingMessage  bool
	selectedMessage servicebus.PeekedMessage
	textSelection   ui.TextSelection

	markedMessages    map[string]map[string]struct{}
	duplicateMessages map[string]map[string]struct{}

	cache sbCache

	actionMenu   actionMenuState
	targetPicker targetPickerState
	inspectPanes map[int]bool

	loadingSpinnerID int

	clickTracker ui.ClickTracker
	paneWidths   [6]int // ns, ent, subs, qtype, msg, preview
	paneHeight   int

	// pendingNav is set by the parent app (via SetPendingNav) when the
	// dashboard wants this tab to navigate to a specific entity. The
	// state machine in advancePendingNav drives the selection forward
	// each time a fetch completes.
	pendingNav PendingNav

	// usage records every drill-in (namespace / queue / topic / sub)
	// to a shared SQLite table the dashboard reads to surface
	// frequently-used resources. nil when the parent runs in-memory.
	usage *cache.DB
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
	namespace      servicebus.Namespace
	source         string
	messages       []servicebus.PeekedMessage
	deadLetter     bool
	repeek         bool
	preserveCursor bool
	err            error
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

type dlqReceivedMsg struct {
	result *servicebus.ReceivedMessages
	err    error
}

type dlqCompleteMsg struct {
	completed []string
	err       error
}

type dlqRequeueMsg struct {
	requeued []string
	err      error
}

type dlqAbandonMsg struct {
	err error
}

type dlqRequeueAllMsg struct {
	requeued int
	err      error
}

type entitiesRefreshedMsg struct {
	entities []servicebus.Entity
	err      error
}

type moveAllDoneMsg struct {
	moved int
	err   error
}

type moveMarkedDoneMsg struct {
	moved []string
	err   error
}

type targetEntitiesLoadedMsg struct {
	namespace servicebus.Namespace
	entities  []servicebus.Entity
	err       error
}

func newList(delegate list.DefaultDelegate, name, plural string) list.Model {
	l := list.New([]list.Item{}, delegate, 40, 10)
	l.SetShowTitle(false)
	l.SetShowHelp(false)
	l.SetShowPagination(false)
	l.SetShowStatusBar(true)
	l.SetStatusBarItemName(name, plural)
	l.SetFilteringEnabled(true)
	l.DisableQuitKeybindings()
	return l
}

func NewModel(svc *servicebus.Service, cfg ui.Config, db *cache.DB) Model {
	return NewModelWithKeyMap(svc, cfg, keymap.Default(), db)
}

func NewModelWithKeyMap(svc *servicebus.Service, cfg ui.Config, km keymap.Keymap, db *cache.DB) Model {
	delegate := list.NewDefaultDelegate()

	namespaces := newList(delegate, "namespace", "namespaces")
	entities := newList(delegate, "entity", "entities")
	entities.Filter = entityListFilter
	subs := newList(delegate, "subscription", "subscriptions")
	queueType := newList(delegate, "queue", "queues")
	queueType.SetFilteringEnabled(false)
	queueType.SetShowStatusBar(false)
	messages := newList(delegate, "message", "messages")

	m := Model{
		Model:                appshell.New(cfg, km),
		service:              svc,
		namespacesList:       namespaces,
		entitiesList:         entities,
		subscriptionsList:    subs,
		queueTypeList:        queueType,
		messageList:          messages,
		focus:                namespacesPane,
		markedMessages:       make(map[string]map[string]struct{}),
		duplicateMessages:    make(map[string]map[string]struct{}),
		cache:                newCache(db),
		namespacesHistory:    make(map[string]ui.ListState),
		entitiesHistory:      make(map[string]ui.ListState),
		subscriptionsHistory: make(map[string]ui.ListState),
		inspectPanes:         make(map[int]bool),
	}
	m.applyScheme(cfg.ActiveScheme())
	m.HydrateSubscriptionsFromCache(m.cache.subscriptions)
	if !m.HasSubscription {
		m.SubOverlay.Open()
		m.startLoading(-1, "Loading Azure subscriptions...")
	}
	return m
}

// NewModelWithCache creates a Model using pre-built shared cache stores.
func NewModelWithCache(svc *servicebus.Service, cfg ui.Config, stores SBStores, km keymap.Keymap) Model {
	m := NewModelWithKeyMap(svc, cfg, km, nil)
	m.cache = NewCacheWithStores(stores)
	m.usage = stores.Usage
	m.HydrateSubscriptionsFromCache(m.cache.subscriptions)
	return m
}

func (m *Model) applyScheme(scheme ui.Scheme) {
	m.SetScheme(scheme)
	m.Styles.ApplyToLists([]*list.Model{
		&m.namespacesList, &m.entitiesList, &m.subscriptionsList,
		&m.queueTypeList, &m.messageList,
	}, &m.Spinner)
	d := newMessageDelegate(m.Styles.Delegate, m.Styles)
	d.marked = m.currentMarks()
	d.visual = m.visualSelectionSet()
	m.messageList.SetDelegate(d)
}

// ApplyScheme applies the given scheme to all lists and spinner.
func (m *Model) ApplyScheme(scheme ui.Scheme) {
	m.applyScheme(scheme)
}

// isTopicSelected reports whether the currently selected entity is a topic.
func (m Model) isTopicSelected() bool {
	return m.currentEntity.Kind == servicebus.EntityTopic && m.currentEntity.Name != ""
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
				keymap.HelpEntry(km.ActionMenu, "actions (peek, peek more, clear)"),
				keymap.HelpEntry(km.ToggleDLQFilter, "entity actions (sort, filter)"),
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

// SetSubscription overrides the embedded appshell.Model method.
func (m *Model) SetSubscription(sub azure.Subscription) {
	m.Model.SetSubscription(sub)
	if sub.TenantID != "" {
		if cred, err := azure.NewCredentialForTenant(sub.TenantID); err == nil {
			m.service.SetCredential(cred)
		}
	}
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
