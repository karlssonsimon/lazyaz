package sbapp

import (
	"azure-storage/internal/azure"
	"azure-storage/internal/azure/servicebus"
	"azure-storage/internal/ui"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

const peekMaxMessages = 50

const (
	subscriptionsPane = iota
	namespacesPane
	entitiesPane
	detailPane
)

type detailView int

const (
	detailMessages           detailView = iota // peeked messages (queue or topic sub)
	detailTopicSubscriptions                   // list of topic subscriptions
)

type Model struct {
	service *servicebus.Service

	spinner spinner.Model

	subscriptionsList list.Model
	namespacesList    list.Model
	entitiesList      list.Model
	detailList        list.Model

	focus int

	subscriptions  []azure.Subscription
	namespaces     []servicebus.Namespace
	entities       []servicebus.Entity
	topicSubs      []servicebus.TopicSubscription
	peekedMessages []servicebus.PeekedMessage

	hasSubscription bool
	currentSub      azure.Subscription
	hasNamespace    bool
	currentNS       servicebus.Namespace
	hasEntity       bool
	currentEntity   servicebus.Entity

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

	styles ui.Styles
	keymap KeyMap

	schemes      []ui.Scheme
	themeOverlay ui.ThemeOverlayState
	helpOverlay  ui.HelpOverlayState

	cache sbCache

	// EmbeddedMode suppresses theme/help overlay handling and quit
	// interception so the parent tabapp can own those concerns.
	EmbeddedMode bool

	loading bool
	status  string
	lastErr string

	width      int
	height     int
	paneWidths [4]int // sub, ns, ent, det — set by resize
	paneHeight int
}

type subscriptionsLoadedMsg struct {
	subscriptions []azure.Subscription
	done          bool
	err           error
	next          tea.Cmd
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

func NewModel(svc *servicebus.Service, cfg ui.Config) Model {
	return NewModelWithKeyMap(svc, cfg, DefaultKeyMap())
}

func NewModelWithKeyMap(svc *servicebus.Service, cfg ui.Config, keymap KeyMap) Model {
	delegate := list.NewDefaultDelegate()

	subscriptions := list.New([]list.Item{}, delegate, 28, 10)
	subscriptions.Title = "Subscriptions"
	subscriptions.SetShowHelp(false)
	subscriptions.SetShowPagination(false)
	subscriptions.SetShowStatusBar(true)
	subscriptions.SetStatusBarItemName("subscription", "subscriptions")
	subscriptions.SetFilteringEnabled(true)
	subscriptions.DisableQuitKeybindings()

	namespaces := list.New([]list.Item{}, delegate, 24, 10)
	namespaces.Title = "Namespaces"
	namespaces.SetShowHelp(false)
	namespaces.SetShowPagination(false)
	namespaces.SetShowStatusBar(true)
	namespaces.SetStatusBarItemName("namespace", "namespaces")
	namespaces.SetFilteringEnabled(true)
	namespaces.DisableQuitKeybindings()

	entities := list.New([]list.Item{}, delegate, 24, 10)
	entities.Title = "Entities"
	entities.SetShowHelp(false)
	entities.SetShowPagination(false)
	entities.SetShowStatusBar(true)
	entities.SetStatusBarItemName("entity", "entities")
	entities.SetFilteringEnabled(true)
	entities.DisableQuitKeybindings()

	detail := list.New([]list.Item{}, delegate, 40, 10)
	detail.Title = "Detail"
	detail.SetShowHelp(false)
	detail.SetShowPagination(false)
	detail.SetShowStatusBar(true)
	detail.SetStatusBarItemName("item", "items")
	detail.SetFilteringEnabled(true)
	detail.DisableQuitKeybindings()

	spin := spinner.New()
	spin.Spinner = spinner.Dot

	m := Model{
		service:           svc,
		spinner:           spin,
		subscriptionsList: subscriptions,
		namespacesList:    namespaces,
		entitiesList:      entities,
		detailList:        detail,
		focus:             subscriptionsPane,
		markedMessages:    make(map[string]struct{}),
		duplicateMessages: make(map[string]struct{}),
		cache:   newCache(),
		schemes: cfg.Schemes,
		themeOverlay: ui.ThemeOverlayState{
			ActiveThemeIdx: ui.ActiveSchemeIndex(cfg),
		},
		keymap:  keymap,
		status:  "Loading Azure subscriptions...",
		loading: true,
	}
	m.applyScheme(cfg.ActiveScheme())
	return m
}

// NewModelWithCache creates a Model using pre-built shared cache stores.
func NewModelWithCache(svc *servicebus.Service, cfg ui.Config, stores SBStores) Model {
	m := NewModel(svc, cfg)
	m.cache = NewCacheWithStores(stores)
	return m
}

func (m *Model) applyScheme(scheme ui.Scheme) {
	m.styles = ui.NewStyles(scheme)
	m.styles.ApplyToLists([]*list.Model{
		&m.subscriptionsList, &m.namespacesList, &m.entitiesList, &m.detailList,
	}, &m.spinner)
}

// ApplyScheme applies the given scheme to all lists and spinner.
func (m *Model) ApplyScheme(scheme ui.Scheme) {
	m.applyScheme(scheme)
}

// HelpSections returns the help sections for the service bus explorer.
func (m Model) HelpSections() []ui.HelpSection {
	return m.keymap.HelpSections()
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(spinner.Tick, fetchSubscriptionsCmd(m.service, m.cache.subscriptions))
}
