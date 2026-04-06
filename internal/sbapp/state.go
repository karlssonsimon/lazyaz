package sbapp

import (
	"time"

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
	service *servicebus.Service

	spinner spinner.Model

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
	keymap keymap.Keymap

	schemes      []ui.Scheme
	themeOverlay ui.ThemeOverlayState
	helpOverlay  ui.HelpOverlayState
	subOverlay   ui.SubscriptionOverlayState

	inspectFields []ui.InspectField
	inspectTitle  string

	cache sbCache

	// EmbeddedMode suppresses theme/help overlay handling and quit
	// interception so the parent tabapp can own those concerns.
	EmbeddedMode bool

	loading          bool
	loadingPane      int
	loadingStartedAt time.Time
	status           string
	lastErr          string

	width      int
	height     int
	paneWidths    [4]int // ns, ent, det, preview — set by resize
	paneHeight    int
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

func NewModel(svc *servicebus.Service, cfg ui.Config, db *cache.DB) Model {
	return NewModelWithKeyMap(svc, cfg, keymap.Default(), db)
}

func NewModelWithKeyMap(svc *servicebus.Service, cfg ui.Config, km keymap.Keymap, db *cache.DB) Model {
	delegate := list.NewDefaultDelegate()

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
		namespacesList:    namespaces,
		entitiesList:      entities,
		detailList:        detail,
		focus:             namespacesPane,
		loadingPane:       -1,
		markedMessages:    make(map[string]struct{}),
		duplicateMessages: make(map[string]struct{}),
		cache:             newCache(db),
		schemes:           cfg.Schemes,
		themeOverlay: ui.ThemeOverlayState{
			ActiveThemeIdx: ui.ActiveSchemeIndex(cfg),
		},
		keymap: km,
	}
	m.applyScheme(cfg.ActiveScheme())
	// Hydrate subscriptions from cache without hitting Azure.
	if cached, ok := m.cache.subscriptions.Get(""); ok {
		m.subscriptions = cached
	}
	if !m.hasSubscription {
		m.subOverlay.Open()
		m.setLoading(-1)
		m.status = "Loading Azure subscriptions..."
	}
	return m
}

// NewModelWithCache creates a Model using pre-built shared cache stores.
func NewModelWithCache(svc *servicebus.Service, cfg ui.Config, stores SBStores, km keymap.Keymap) Model {
	m := NewModelWithKeyMap(svc, cfg, km, nil)
	m.cache = NewCacheWithStores(stores)
	// Re-hydrate subscriptions from the shared store.
	if cached, ok := m.cache.subscriptions.Get(""); ok {
		m.subscriptions = cached
	}
	return m
}

func (m *Model) setLoading(pane int) {
	if !m.loading {
		m.loadingStartedAt = time.Now()
	}
	m.loading = true
	m.loadingPane = pane
}

func (m *Model) clearLoading() {
	m.loading = false
	m.loadingPane = -1
}

// loadingHoldExpiredMsg is sent after the min-visible spinner hold elapses.
type loadingHoldExpiredMsg struct {
	status string
}

// finishLoading completes a load, holding the spinner visible for at least
// ui.SpinnerMinVisible. If the hold has not yet elapsed, returns a delayed
// command; otherwise clears loading immediately and sets the status.
func (m *Model) finishLoading(status string) tea.Cmd {
	remaining := ui.SpinnerMinVisible - time.Since(m.loadingStartedAt)
	if remaining > 0 {
		return tea.Tick(remaining, func(t time.Time) tea.Msg {
			return loadingHoldExpiredMsg{status: status}
		})
	}
	m.clearLoading()
	m.status = status
	return nil
}

func (m *Model) applyScheme(scheme ui.Scheme) {
	m.styles = ui.NewStyles(scheme)
	m.styles.ApplyToLists([]*list.Model{
		&m.namespacesList, &m.entitiesList, &m.detailList,
	}, &m.spinner)
}

// ApplyScheme applies the given scheme to all lists and spinner.
func (m *Model) ApplyScheme(scheme ui.Scheme) {
	m.applyScheme(scheme)
}

// HelpSections returns the help sections for the service bus explorer.
func (m Model) HelpSections() []ui.HelpSection {
	km := m.keymap
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
	cmds := []tea.Cmd{spinner.Tick}
	if m.subOverlay.Active {
		cmds = append(cmds, fetchSubscriptionsCmd(m.service, m.cache.subscriptions, true))
	}
	if m.hasSubscription {
		cmds = append(cmds, fetchNamespacesCmd(m.service, m.cache.namespaces, m.currentSub.ID))
	}
	return tea.Batch(cmds...)
}
