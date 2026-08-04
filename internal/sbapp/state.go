package sbapp

import (
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/azure"
	"github.com/karlssonsimon/lazyaz/internal/azure/servicebus"
	"github.com/karlssonsimon/lazyaz/internal/cache"
	"github.com/karlssonsimon/lazyaz/internal/keymap"
	"github.com/karlssonsimon/lazyaz/internal/ui"
	"github.com/karlssonsimon/lazyaz/internal/vim"

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
	ModeConfirm                         // Destructive-action confirm modal open
	ModeOverlay                         // Sub/Theme/Help overlay open
	ModeSortOverlay                     // Entity sort picker open
	ModeTargetPicker                    // Target entity picker open
	ModeActionMenu                      // Action menu open
	ModeMessagePreview                  // Viewing message detail
	ModeListFilter                      // User is typing a list filter
	ModeVisualLine                      // Visual line selection active
	ModeCopyPalette                     // Copy palette overlay open
)

func (m Model) inputMode() InputMode {
	switch {
	case m.confirmModal.Active:
		return ModeConfirm
	case m.SubOverlay.Active, m.ThemeOverlay.Active, m.HelpOverlay.Active:
		return ModeOverlay
	case m.entitySortOverlay.Active:
		return ModeSortOverlay
	case m.targetPicker.active:
		return ModeTargetPicker
	case m.copyOverlay.Active:
		return ModeCopyPalette
	case m.actionMenu.Active:
		return ModeActionMenu
	case m.viewingMessage && m.focus == messagePreviewPane:
		return ModeMessagePreview
	case m.focusedListSettingFilter():
		return ModeListFilter
	case m.visual.Active() && m.focus == messagesPane:
		return ModeVisualLine
	default:
		return ModeNormal
	}
}

func (mode InputMode) String() string {
	switch mode {
	case ModeVisualLine:
		return "VISUAL"
	case ModeListFilter:
		return "FILTER"
	default:
		return "NORMAL"
	}
}

type Model struct {
	appshell.Model

	service *servicebus.Service

	namespacesList    ui.List
	entitiesList      ui.List
	subscriptionsList ui.List // topic subscriptions
	queueTypeList     ui.List // Active / DLQ picker (2 items)
	messageList       ui.List // messages from selected queue type

	focus int

	// vimr resolves multi-key vim chords (gg, z...) for this model.
	vimr vim.Resolver

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
	hasPeekTarget  bool
	currentEntity  servicebus.Entity
	currentSubName string

	// deadLetter is true when the user selected "DLQ" in the queue
	// type picker.
	deadLetter bool

	// visual is the vim selection over the messages list; the anchor is
	// a message operation key and its index cache follows the list's
	// visible version.
	visual vim.Visual

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

	// messageSearch is the / prompt over the message body.
	messageSearch   messageSearchState
	selectedMessage servicebus.PeekedMessage
	textSelection   ui.TextSelection

	// markedMessages holds one mark set per queue/DLQ scope. Only this
	// surface needs scoping, so the map lives here rather than in vim.
	markedMessages map[string]vim.MarkSet

	cache sbCache

	actionMenu   actionMenuState
	copyOverlay  ui.CopyOverlay
	targetPicker targetPickerState
	inspectPanes map[int]bool

	// confirmModal guards destructive DLQ operations (complete, requeue,
	// move) the same way blob/kv guard deletes. confirmAction receives the
	// model at confirm time so it can start spinners before dispatching.
	confirmModal  ui.ConfirmModalState
	confirmAction func(Model) (Model, tea.Cmd)

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

	// applyingNav is true while ApplyNav (jump-list restoration) is
	// driving navigation. Suppresses RecordJumpMsg emission from the
	// drill-in helpers — without this guard, restoring to position X
	// re-records X, truncating the forward history and trapping the
	// user in an oscillation between two adjacent jump entries.
	applyingNav bool

	// CredResolver, when set, supplies a credential for the active
	// subscription's tenant. The parent injects this; standalone
	// (non-embedded) usage leaves it nil and falls back to the service's
	// existing credential.
	CredResolver interface {
		CredentialFor(tenantID string) (azcore.TokenCredential, error)
	}
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
	entityName     string
	subName        string
	messages       []servicebus.PeekedMessage
	deadLetter     bool
	repeek         bool
	preserveCursor bool
	err            error
}

type messagesReceivedMsg struct {
	namespace  servicebus.Namespace
	entityName string
	subName    string
	deadLetter bool
	result     *servicebus.ReceivedMessages
	err        error
}

// The lock-mutation results carry the lock session they operated on so
// their handlers can tell whether that session is still the one on
// screen. Locks are released by navigating away, and a new session may
// be installed before a late result lands — matching by pointer
// identity keeps stale results from mutating the wrong view.
type dlqCompleteMsg struct {
	locked    *servicebus.ReceivedMessages
	completed []string
	err       error
}

type dlqRequeueMsg struct {
	locked   *servicebus.ReceivedMessages
	requeued []string
	err      error
}

type dlqAbandonMsg struct {
	locked *servicebus.ReceivedMessages
	err    error
}

type dlqRequeueAllMsg struct {
	requeued int
	err      error
}

type entitiesRefreshedMsg struct {
	namespace servicebus.Namespace
	entities  []servicebus.Entity
	err       error
}

type moveAllDoneMsg struct {
	moved      int
	deadLetter bool // echoes the cmd input so the handler labels the result correctly
	err        error
}

type moveMarkedDoneMsg struct {
	locked *servicebus.ReceivedMessages
	moved  []string
	err    error
}

type targetEntitiesLoadedMsg struct {
	namespace servicebus.Namespace
	entities  []servicebus.Entity
	err       error
}

func newList(delegate list.DefaultDelegate, name, plural string) ui.List {
	l := ui.NewList([]list.Item{}, delegate, 40, 10)
	l.SetShowTitle(false)
	l.SetShowFilter(false) // we render our own /<query>█ as a SubHeader
	l.SetShowHelp(false)
	l.SetShowPagination(false)
	l.SetShowStatusBar(false)
	l.SetStatusBarItemName(name, plural)
	l.SetFilteringEnabled(true)
	l.DisableQuitKeybindings()
	return l
}

func NewModel(svc *servicebus.Service, cfg ui.Config, db *cache.DB) Model {
	return NewModelWithKeyMap(svc, cfg, keymap.Default(), db)
}

func NewModelWithKeyMap(svc *servicebus.Service, cfg ui.Config, km keymap.Keymap, db *cache.DB) Model {
	if svc == nil {
		svc = servicebus.NewService(nil)
	}
	delegate := list.NewDefaultDelegate()

	namespaces := newList(delegate, "namespace", "namespaces")
	entities := newList(delegate, "entity", "entities")
	subs := newList(delegate, "subscription", "subscriptions")
	queueType := newList(delegate, "queue", "queues")
	queueType.SetFilteringEnabled(false)
	queueType.SetShowStatusBar(false)
	messages := newList(delegate, "message", "messages")

	// Override bubbles list cursor and filter bindings so they follow
	// the user's configured CursorUp/CursorDown/FilterInput keys.
	for _, l := range []*ui.List{&namespaces, &entities, &subs, &queueType, &messages} {
		l.KeyMap.CursorUp = km.CursorUp.AsBubbleKey()
		l.KeyMap.CursorDown = km.CursorDown.AsBubbleKey()
		l.KeyMap.Filter = km.FilterInput.AsBubbleKey()
		l.Filter = ui.ListFilter
		l.SetScrolloff(cfg.ScrolloffValue())
	}
	// Entity titles carry a kind glyph the filter must offset past —
	// re-set after the loop so the wrapper wins.
	entities.Filter = entityListFilter

	m := Model{
		Model:                appshell.New(cfg, km),
		service:              svc,
		namespacesList:       namespaces,
		entitiesList:         entities,
		subscriptionsList:    subs,
		queueTypeList:        queueType,
		messageList:          messages,
		focus:                namespacesPane,
		markedMessages:       make(map[string]vim.MarkSet),
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
		m.StartLoading(-1, "Loading Azure subscriptions...")
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
	m.Styles.ApplyToLists([]*ui.List{
		&m.namespacesList, &m.entitiesList, &m.subscriptionsList,
		&m.queueTypeList, &m.messageList,
	}, &m.Spinner)
	d := ui.NewMarkDelegate(m.Styles.Delegate, m.Styles, messageMarkKey)
	d.Marked = m.currentMarks().Items()
	d.Visual = m.visualSelectionSet()
	m.messageList.SetDelegate(d)
	m.entitiesList.SetDelegate(newEntityDelegate(m.Styles.Delegate, m.Styles))
	m.subscriptionsList.SetDelegate(newSubscriptionDelegate(m.Styles.Delegate, m.Styles))
	m.rehighlightSelectedMessage()
}

// ApplyScheme applies the given scheme to all lists and spinner.
func (m *Model) ApplyScheme(scheme ui.Scheme) {
	m.applyScheme(scheme)
}

func (m Model) WithScheme(scheme ui.Scheme) tea.Model {
	m.ApplyScheme(scheme)
	return m
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
				keymap.HelpEntry(km.NextFocus, "focus next column"),
				keymap.HelpEntry(km.PreviousFocus, "focus previous column"),
				keymap.HelpEntry(km.FilterInput, "filter focused column"),
				keymap.HelpEntry(km.OpenFocused, "open selected row"),
				keymap.HelpEntry(km.BackspaceUp, "go up/back"),
				keymap.HelpEntry(keymap.New(km.HalfPageDown.Label()+"/"+km.HalfPageUp.Label()), "half-page scroll"),
			},
		},
		{
			Title: "Messages",
			Items: []string{
				keymap.HelpEntry(km.ActionMenu, "actions (peek, peek more, clear)"),
				keymap.HelpEntry(km.ToggleDLQFilter, "entity actions (sort, filter)"),
				keymap.HelpEntry(km.RequeueDLQ, "requeue received DLQ message(s)"),
				keymap.HelpEntry(km.YankMessageBody, "yank message body to clipboard"),
				keymap.HelpEntry(km.CopyPalette, "copy palette (IDs, names, body)"),
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
	if m.service != nil && m.CredResolver != nil && sub.TenantID != "" {
		if cred, err := m.CredResolver.CredentialFor(sub.TenantID); err == nil {
			m.service.SetCredential(cred)
		} else {
			m.Notify(appshell.LevelError, fmt.Sprintf("Credential for tenant %s: %s", sub.TenantID, err.Error()))
		}
	}
	if cached, ok := m.cache.namespaces.Get(sub.ID); ok {
		m.namespaces = cached
		m.namespacesList.Title = fmt.Sprintf("Namespaces (%d)", len(cached))
		ui.SetItemsPreserveKey(&m.namespacesList, namespacesToItems(cached), namespaceItemKey)
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
	if m.SubOverlay.Active {
		cmds = append(cmds, appshell.FetchSubscriptionsCmd(m.service, m.cache.subscriptions, m.Tenant, m.Subscriptions))
	}
	if m.HasSubscription {
		cmds = append(cmds, fetchNamespacesCmd(m.service, m.cache.namespaces, m.CurrentSub.ID, m.namespaces))
	}
	return tea.Batch(cmds...)
}
