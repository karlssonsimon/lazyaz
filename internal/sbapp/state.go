package sbapp

import (
	"azure-storage/internal/azure"
	"azure-storage/internal/servicebus"
	ui "azure-storage/internal/ui"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

	messageViewport   viewport.Model
	viewingMessage    bool
	selectedMessage   servicebus.PeekedMessage
	markedMessages    map[string]struct{}
	duplicateMessages map[string]struct{}

	ui           UIColors
	syntaxStyles ui.SyntaxStyles
	keymap       KeyMap

	themes         []Theme
	activeThemeIdx int
	selectingTheme bool
	themeIdx       int

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

type namespacesLoadedMsg struct {
	subscriptionID string
	namespaces     []servicebus.Namespace
	err            error
}

type entitiesLoadedMsg struct {
	namespace servicebus.Namespace
	entities  []servicebus.Entity
	err       error
}

type topicSubscriptionsLoadedMsg struct {
	namespace servicebus.Namespace
	topicName string
	subs      []servicebus.TopicSubscription
	err       error
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

func NewModel(svc *servicebus.Service, cfg Config) Model {
	return NewModelWithKeyMap(svc, cfg, DefaultKeyMap())
}

func NewModelWithKeyMap(svc *servicebus.Service, cfg Config, keymap KeyMap) Model {
	active := cfg.ActiveTheme()
	activeIdx := 0
	for i, t := range cfg.Themes {
		if t.Name == active.Name {
			activeIdx = i
			break
		}
	}

	delegate := ui.NewDefaultDelegate(uiPalette(active.UIColors))

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
		themes:            cfg.Themes,
		activeThemeIdx:    activeIdx,
		keymap:            keymap,
		status:            "Loading Azure subscriptions...",
		loading:           true,
	}
	m.applyTheme(active)
	return m
}

func uiPalette(colors UIColors) ui.Palette {
	return ui.Palette{
		Border:        colors.Border,
		BorderFocused: colors.BorderFocused,
		Text:          colors.Text,
		Muted:         colors.Muted,
		Accent:        colors.Accent,
		AccentStrong:  colors.AccentStrong,
		Danger:        colors.Danger,
		FilterMatch:   colors.FilterMatch,
		SelectedBg:    colors.SelectedBg,
		SelectedText:  colors.SelectedText,
	}
}

func (m *Model) applyTheme(theme Theme) {
	uiColors := theme.UIColors
	m.ui = uiColors
	m.syntaxStyles = syntaxStylesForTheme(theme)

	palette := uiPalette(uiColors)
	delegate := ui.NewDefaultDelegate(palette)

	m.subscriptionsList.SetDelegate(delegate)
	m.namespacesList.SetDelegate(delegate)
	m.entitiesList.SetDelegate(delegate)
	m.detailList.SetDelegate(delegate)

	ui.StyleList(&m.subscriptionsList, palette)
	ui.StyleList(&m.namespacesList, palette)
	ui.StyleList(&m.entitiesList, palette)
	ui.StyleList(&m.detailList, palette)

	m.spinner.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(uiColors.AccentStrong))
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(spinner.Tick, loadSubscriptionsCmd(m.service))
}
