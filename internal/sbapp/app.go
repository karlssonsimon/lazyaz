package sbapp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode"

	"azure-storage/internal/azure"
	"azure-storage/internal/servicebus"

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

const (
	colorBorder        = "#4B5563"
	colorBorderFocused = "#22C55E"
	colorText          = "#E5E7EB"
	colorMuted         = "#94A3B8"
	colorAccent        = "#60A5FA"
	colorAccentStrong  = "#38BDF8"
	colorDanger        = "#F87171"
	colorFilterMatch   = "#F59E0B"
	colorSelectedBg    = "#334155"
	colorSelectedText  = "#F8FAFC"
)

type detailView int

const (
	detailMessages         detailView = iota // peeked messages (queue or topic sub)
	detailTopicSubscriptions                 // list of topic subscriptions
)

type Model struct {
	service *servicebus.Service

	spinner spinner.Model

	subscriptionsList list.Model
	namespacesList    list.Model
	entitiesList      list.Model
	detailList        list.Model

	focus int

	subscriptions    []azure.Subscription
	namespaces       []servicebus.Namespace
	entities         []servicebus.Entity
	topicSubs        []servicebus.TopicSubscription
	peekedMessages   []servicebus.PeekedMessage

	hasSubscription bool
	currentSub      azure.Subscription
	hasNamespace    bool
	currentNS       servicebus.Namespace
	hasEntity       bool
	currentEntity   servicebus.Entity

	detailMode        detailView
	viewingTopicSub   bool
	currentTopicSub   servicebus.TopicSubscription
	deadLetter        bool

	messageViewport viewport.Model
	viewingMessage  bool
	selectedMessage servicebus.PeekedMessage

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

func NewModel(svc *servicebus.Service) Model {
	delegate := list.NewDefaultDelegate()
	delegate.SetSpacing(0)
	delegate.Styles.NormalTitle = delegate.Styles.NormalTitle.Foreground(lipgloss.Color(colorText))
	delegate.Styles.NormalDesc = delegate.Styles.NormalDesc.Foreground(lipgloss.Color(colorMuted))
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.
		Foreground(lipgloss.Color(colorSelectedText)).
		Background(lipgloss.Color(colorSelectedBg)).
		Bold(true)
	delegate.Styles.SelectedDesc = delegate.Styles.SelectedDesc.
		Foreground(lipgloss.Color(colorSelectedText)).
		Background(lipgloss.Color(colorSelectedBg))
	delegate.Styles.FilterMatch = delegate.Styles.FilterMatch.Foreground(lipgloss.Color(colorFilterMatch)).Underline(true)

	subscriptions := list.New([]list.Item{}, delegate, 28, 10)
	subscriptions.Title = "Subscriptions"
	subscriptions.SetShowHelp(false)
	subscriptions.SetShowPagination(false)
	subscriptions.SetShowStatusBar(true)
	subscriptions.SetStatusBarItemName("subscription", "subscriptions")
	subscriptions.SetFilteringEnabled(true)
	subscriptions.DisableQuitKeybindings()
	styleList(&subscriptions)

	namespaces := list.New([]list.Item{}, delegate, 24, 10)
	namespaces.Title = "Namespaces"
	namespaces.SetShowHelp(false)
	namespaces.SetShowPagination(false)
	namespaces.SetShowStatusBar(true)
	namespaces.SetStatusBarItemName("namespace", "namespaces")
	namespaces.SetFilteringEnabled(true)
	namespaces.DisableQuitKeybindings()
	styleList(&namespaces)

	entities := list.New([]list.Item{}, delegate, 24, 10)
	entities.Title = "Entities"
	entities.SetShowHelp(false)
	entities.SetShowPagination(false)
	entities.SetShowStatusBar(true)
	entities.SetStatusBarItemName("entity", "entities")
	entities.SetFilteringEnabled(true)
	entities.DisableQuitKeybindings()
	styleList(&entities)

	detail := list.New([]list.Item{}, delegate, 40, 10)
	detail.Title = "Detail"
	detail.SetShowHelp(false)
	detail.SetShowPagination(false)
	detail.SetShowStatusBar(true)
	detail.SetStatusBarItemName("item", "items")
	detail.SetFilteringEnabled(true)
	detail.DisableQuitKeybindings()
	styleList(&detail)

	spin := spinner.New()
	spin.Spinner = spinner.Dot
	spin.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(colorAccentStrong))

	return Model{
		service:           svc,
		spinner:           spin,
		subscriptionsList: subscriptions,
		namespacesList:    namespaces,
		entitiesList:      entities,
		detailList:        detail,
		focus:             subscriptionsPane,
		status:            "Loading Azure subscriptions...",
		loading:           true,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(spinner.Tick, loadSubscriptionsCmd(m.service))
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resize()
		return m, nil

	case spinner.TickMsg:
		if !m.loading {
			return m, nil
		}
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case subscriptionsLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.lastErr = msg.err.Error()
			m.status = "Failed to load subscriptions"
			return m, nil
		}

		m.lastErr = ""
		m.subscriptions = msg.subscriptions
		m.subscriptionsList.ResetFilter()
		m.subscriptionsList.SetItems(subscriptionsToItems(msg.subscriptions))
		m.subscriptionsList.Title = fmt.Sprintf("Subscriptions (%d)", len(msg.subscriptions))

		if len(msg.subscriptions) == 0 {
			m.hasSubscription = false
			m.hasNamespace = false
			m.hasEntity = false
			m.status = "No subscriptions found. Verify az login context and tenant access."
			m.clearDetailState()
			m.namespaces = nil
			m.entities = nil
			m.namespacesList.ResetFilter()
			m.entitiesList.ResetFilter()
			m.detailList.ResetFilter()
			m.namespacesList.SetItems(nil)
			m.entitiesList.SetItems(nil)
			m.detailList.SetItems(nil)
			m.namespacesList.Title = "Namespaces"
			m.entitiesList.Title = "Entities"
			m.detailList.Title = "Detail"
			return m, nil
		}

		m.subscriptionsList.Select(0)
		m.hasSubscription = false
		m.currentSub = azure.Subscription{}
		m.hasNamespace = false
		m.hasEntity = false
		m.status = fmt.Sprintf("Loaded %d subscriptions. Select one and press Enter.", len(msg.subscriptions))
		return m, nil

	case namespacesLoadedMsg:
		if !m.hasSubscription || m.currentSub.ID != msg.subscriptionID {
			return m, nil
		}

		m.loading = false
		if msg.err != nil {
			m.lastErr = msg.err.Error()
			m.status = fmt.Sprintf("Failed to load namespaces in %s", subscriptionDisplayName(m.currentSub))
			return m, nil
		}

		m.lastErr = ""
		m.namespaces = msg.namespaces
		m.namespacesList.ResetFilter()
		m.namespacesList.SetItems(namespacesToItems(msg.namespaces))
		m.namespacesList.Title = fmt.Sprintf("Namespaces (%d)", len(msg.namespaces))

		if len(msg.namespaces) == 0 {
			m.hasNamespace = false
			m.hasEntity = false
			m.status = fmt.Sprintf("No Service Bus namespaces found in %s", subscriptionDisplayName(m.currentSub))
			m.clearDetailState()
			m.entities = nil
			m.entitiesList.ResetFilter()
			m.detailList.ResetFilter()
			m.entitiesList.SetItems(nil)
			m.detailList.SetItems(nil)
			m.entitiesList.Title = "Entities"
			m.detailList.Title = "Detail"
			return m, nil
		}

		m.namespacesList.Select(0)
		m.hasNamespace = false
		m.currentNS = servicebus.Namespace{}
		m.clearDetailState()
		m.entities = nil
		m.entitiesList.ResetFilter()
		m.detailList.ResetFilter()
		m.entitiesList.SetItems(nil)
		m.detailList.SetItems(nil)
		m.entitiesList.Title = "Entities"
		m.detailList.Title = "Detail"
		m.status = fmt.Sprintf("Loaded %d namespaces from %s. Open a namespace to view entities.", len(msg.namespaces), subscriptionDisplayName(m.currentSub))
		return m, nil

	case entitiesLoadedMsg:
		if !m.hasNamespace || m.currentNS.Name != msg.namespace.Name {
			return m, nil
		}

		m.loading = false
		if msg.err != nil {
			m.lastErr = msg.err.Error()
			m.status = fmt.Sprintf("Failed to load entities in %s", msg.namespace.Name)
			m.clearDetailState()
			m.entities = nil
			m.entitiesList.ResetFilter()
			m.detailList.ResetFilter()
			m.entitiesList.SetItems(nil)
			m.detailList.SetItems(nil)
			m.hasEntity = false
			return m, nil
		}

		m.lastErr = ""
		m.entities = msg.entities
		m.entitiesList.ResetFilter()
		m.entitiesList.SetItems(entitiesToItems(msg.entities))
		m.entitiesList.Title = fmt.Sprintf("Entities (%d)", len(msg.entities))
		m.entitiesList.Select(0)

		if len(msg.entities) == 0 {
			m.hasEntity = false
			m.clearDetailState()
			m.detailList.ResetFilter()
			m.detailList.SetItems(nil)
			m.detailList.Title = "Detail"
			m.status = fmt.Sprintf("No queues or topics found in %s", msg.namespace.Name)
			return m, nil
		}

		m.hasEntity = false
		m.clearDetailState()
		m.detailList.ResetFilter()
		m.detailList.SetItems(nil)
		m.detailList.Title = "Detail"
		m.status = fmt.Sprintf("Loaded %d entities from %s. Open an entity to peek messages.", len(msg.entities), msg.namespace.Name)
		return m, nil

	case topicSubscriptionsLoadedMsg:
		if !m.hasEntity || m.currentEntity.Kind != servicebus.EntityTopic {
			return m, nil
		}
		if m.currentNS.Name != msg.namespace.Name || m.currentEntity.Name != msg.topicName {
			return m, nil
		}

		m.loading = false
		if msg.err != nil {
			m.lastErr = msg.err.Error()
			m.status = fmt.Sprintf("Failed to load subscriptions for topic %s", msg.topicName)
			return m, nil
		}

		m.lastErr = ""
		m.topicSubs = msg.subs
		m.detailMode = detailTopicSubscriptions
		m.viewingTopicSub = false
		m.detailList.ResetFilter()
		m.detailList.SetItems(topicSubsToItems(msg.subs))
		m.detailList.Title = fmt.Sprintf("Topic Subscriptions (%d)", len(msg.subs))
		if len(msg.subs) > 0 {
			m.detailList.Select(0)
		}
		m.status = fmt.Sprintf("Loaded %d subscriptions for topic %s", len(msg.subs), msg.topicName)
		return m, nil

	case messagesLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.lastErr = msg.err.Error()
			m.status = fmt.Sprintf("Failed to peek messages from %s", msg.source)
			return m, nil
		}

		m.lastErr = ""
		m.peekedMessages = msg.messages
		m.detailMode = detailMessages
		m.viewingMessage = false
		m.selectedMessage = servicebus.PeekedMessage{}
		m.detailList.ResetFilter()
		m.detailList.SetItems(messagesToItems(msg.messages))
		m.detailList.Title = fmt.Sprintf("Messages (%d)", len(msg.messages))
		if len(msg.messages) > 0 {
			m.detailList.Select(0)
		}
		m.resize()
		m.status = fmt.Sprintf("Peeked %d messages from %s", len(msg.messages), msg.source)
		return m, nil

	case tea.KeyMsg:
		if m.viewingMessage {
			switch msg.String() {
			case "ctrl+c", "q":
				return m, tea.Quit
			case "h", "left", "backspace", "esc":
				m.viewingMessage = false
				m.selectedMessage = servicebus.PeekedMessage{}
				m.resize()
				m.status = "Back to messages"
				return m, nil
			default:
				m.messageViewport, cmd = m.messageViewport.Update(msg)
				return m, cmd
			}
		}

		focusedFilterActive := m.focusedListSettingFilter()

		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "ctrl+d":
			m.scrollFocusedHalfPage(1)
			return m, nil
		case "ctrl+u":
			m.scrollFocusedHalfPage(-1)
			return m, nil
		case "tab":
			if !focusedFilterActive {
				m.nextFocus()
				return m, nil
			}
		case "shift+tab":
			if !focusedFilterActive {
				m.previousFocus()
				return m, nil
			}
		case "d":
			if !focusedFilterActive {
				m.loading = true
				m.lastErr = ""
				m.status = "Refreshing subscriptions..."
				return m, tea.Batch(spinner.Tick, loadSubscriptionsCmd(m.service))
			}
		case "r":
			if !focusedFilterActive {
				return m.refresh()
			}
		case "enter":
			if focusedFilterActive {
				m.commitFocusedFilter()
				m.status = fmt.Sprintf("Filter applied for %s", paneName(m.focus))
				return m, nil
			}
			return m.handleEnter()
		case "l", "right":
			if !focusedFilterActive {
				return m.handleEnter()
			}
		case "h", "left":
			if !focusedFilterActive {
				return m.navigateLeft()
			}
		case "[":
			if !focusedFilterActive && m.focus == detailPane && m.detailMode == detailMessages {
				if m.deadLetter {
					m.deadLetter = false
					return m.rePeekMessages()
				}
			}
		case "]":
			if !focusedFilterActive && m.focus == detailPane && m.detailMode == detailMessages {
				if !m.deadLetter {
					m.deadLetter = true
					return m.rePeekMessages()
				}
			}
		case "backspace":
			if !focusedFilterActive {
				return m.handleBackspace()
			}
		}
	}

	switch m.focus {
	case subscriptionsPane:
		m.subscriptionsList, cmd = m.subscriptionsList.Update(msg)
	case namespacesPane:
		m.namespacesList, cmd = m.namespacesList.Update(msg)
	case entitiesPane:
		m.entitiesList, cmd = m.entitiesList.Update(msg)
	case detailPane:
		m.detailList, cmd = m.detailList.Update(msg)
	}

	return m, cmd
}

func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "loading..."
	}

	headerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorAccent)).
		Bold(true).
		Padding(0, 1)

	metaStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorMuted)).
		Padding(0, 1)

	paneStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorText)).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(colorBorder)).
		Padding(0, 1)

	focusedPaneStyle := paneStyle.Copy().
		BorderForeground(lipgloss.Color(colorBorderFocused))

	statusStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorText)).
		Padding(0, 1)

	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorMuted)).
		Padding(0, 1)

	errorStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorDanger)).
		Padding(0, 1)

	filterHintStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorAccent)).
		Padding(0, 1)

	subscriptionName := "-"
	namespaceName := "-"
	entityName := "-"
	if m.hasSubscription {
		subscriptionName = subscriptionDisplayName(m.currentSub)
	}
	if m.hasNamespace {
		namespaceName = m.currentNS.Name
	}
	if m.hasEntity {
		entityName = entityDisplayName(m.currentEntity)
	}

	header := headerStyle.Width(m.width).Render(trimToWidth("Azure Service Bus Explorer", m.width-2))
	headerMeta := metaStyle.Width(m.width).Render(trimToWidth(fmt.Sprintf("Subscription: %s | Namespace: %s | Entity: %s", subscriptionName, namespaceName, entityName), m.width-2))

	m.subscriptionsList.Title = m.subscriptionsPaneTitle()
	m.namespacesList.Title = m.namespacesPaneTitle()
	m.entitiesList.Title = m.entitiesPaneTitle()
	m.detailList.Title = m.detailPaneTitle()

	if m.deadLetter && m.detailMode == detailMessages {
		m.detailList.Styles.Title = m.detailList.Styles.Title.
			Foreground(lipgloss.Color(colorDanger))
	} else {
		m.detailList.Styles.Title = m.detailList.Styles.Title.
			Foreground(lipgloss.Color(colorAccent))
	}

	subscriptionsView := m.subscriptionsList.View()
	namespacesView := m.namespacesList.View()
	entitiesView := m.entitiesList.View()
	detailView := m.detailList.View()

	subscriptionsPaneStyle := paneStyle.Copy().MarginRight(1)
	namespacesPaneStyle := paneStyle.Copy().MarginRight(1)
	entitiesPaneStyle := paneStyle.Copy().MarginRight(1)
	detailPaneStyle := paneStyle.Copy()

	if m.focus == subscriptionsPane {
		subscriptionsPaneStyle = focusedPaneStyle.Copy().MarginRight(1)
	}
	if m.focus == namespacesPane {
		namespacesPaneStyle = focusedPaneStyle.Copy().MarginRight(1)
	}
	if m.focus == entitiesPane {
		entitiesPaneStyle = focusedPaneStyle.Copy().MarginRight(1)
	}

	if m.deadLetter && m.detailMode == detailMessages {
		detailPaneStyle = paneStyle.Copy().BorderForeground(lipgloss.Color(colorDanger))
	} else if m.focus == detailPane && !m.viewingMessage {
		detailPaneStyle = focusedPaneStyle.Copy()
	}
	if m.viewingMessage {
		detailPaneStyle = detailPaneStyle.Copy().MarginRight(1)
	}

	panesList := []string{
		subscriptionsPaneStyle.Render(subscriptionsView),
		namespacesPaneStyle.Render(namespacesView),
		entitiesPaneStyle.Render(entitiesView),
		detailPaneStyle.Render(detailView),
	}

	if m.viewingMessage {
		previewTitleStyle := lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(colorAccent)).
			Padding(0, 1)
		msgID := emptyToDash(m.selectedMessage.MessageID)
		previewTitle := previewTitleStyle.Render(fmt.Sprintf("Message: %s", msgID))
		previewContent := lipgloss.JoinVertical(lipgloss.Left, previewTitle, m.messageViewport.View())

		previewPaneStyle := focusedPaneStyle.Copy()
		panesList = append(panesList, previewPaneStyle.Render(previewContent))
	}

	panes := lipgloss.JoinHorizontal(lipgloss.Top, panesList...)

	filterHint := "Press / to filter the focused pane (fzf-style live filter)."
	if m.focusedListSettingFilter() {
		filterHint = fmt.Sprintf("Filtering %s: type to narrow, up/down to move, Enter applies filter.", paneName(m.focus))
	}
	filterLine := filterHintStyle.Width(m.width).Render(trimToWidth(filterHint, m.width-2))

	errorLine := ""
	if m.lastErr != "" {
		errorLine = errorStyle.Width(m.width).Render(trimToWidth("Error: "+m.lastErr, m.width-2))
	}

	statusText := m.status
	if m.loading {
		statusText = fmt.Sprintf("%s %s", m.spinner.View(), m.status)
	}
	statusLine := statusStyle.Width(m.width).Render(trimToWidth(statusText, m.width-2))

	help := "keys: tab/shift+tab focus | / filter | enter/l open | h/left back | backspace up | [/] active/dlq | ctrl+d/u half-page | r refresh | d reload | q quit"
	helpLine := helpStyle.Width(m.width).Render(trimToWidth(help, m.width-2))

	parts := []string{header, headerMeta, panes, filterLine}
	if errorLine != "" {
		parts = append(parts, errorLine)
	}
	parts = append(parts, statusLine, helpLine)

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m *Model) resize() {
	if m.width <= 0 || m.height <= 0 {
		return
	}

	sub := m.width / 5
	ns := m.width / 5
	ent := m.width / 5
	if sub < 24 {
		sub = 24
	}
	if ns < 24 {
		ns = 24
	}
	if ent < 24 {
		ent = 24
	}
	det := m.width - sub - ns - ent - 12
	if det < 40 {
		det = 40
	}

	height := m.height - 10
	if height < 8 {
		height = 8
	}

	if m.viewingMessage {
		detHalf := det / 3
		if detHalf < 30 {
			detHalf = 30
		}
		previewW := det - detHalf - 3
		if previewW < 30 {
			previewW = 30
		}
		m.detailList.SetSize(detHalf, height)
		m.messageViewport.Width = previewW
		m.messageViewport.Height = height - 2
	} else {
		m.detailList.SetSize(det, height)
		m.messageViewport.Width = 0
		m.messageViewport.Height = 0
	}

	m.subscriptionsList.SetSize(sub, height)
	m.namespacesList.SetSize(ns, height)
	m.entitiesList.SetSize(ent, height)
}

func (m *Model) nextFocus() {
	m.blurAllFilters()
	m.focus = (m.focus + 1) % 4
}

func (m *Model) previousFocus() {
	m.blurAllFilters()
	m.focus--
	if m.focus < 0 {
		m.focus = 3
	}
}

func (m *Model) blurAllFilters() {
	m.subscriptionsList.FilterInput.Blur()
	m.namespacesList.FilterInput.Blur()
	m.entitiesList.FilterInput.Blur()
	m.detailList.FilterInput.Blur()
}

func (m *Model) commitFocusedFilter() {
	m.blurAllFilters()

	switch m.focus {
	case subscriptionsPane:
		applyFilterState(&m.subscriptionsList)
	case namespacesPane:
		applyFilterState(&m.namespacesList)
	case entitiesPane:
		applyFilterState(&m.entitiesList)
	case detailPane:
		applyFilterState(&m.detailList)
	}
}

func applyFilterState(l *list.Model) {
	if strings.TrimSpace(l.FilterValue()) == "" {
		l.SetFilterState(list.Unfiltered)
		return
	}
	l.SetFilterState(list.FilterApplied)
}

func (m *Model) clearDetailState() {
	m.topicSubs = nil
	m.peekedMessages = nil
	m.viewingTopicSub = false
	m.currentTopicSub = servicebus.TopicSubscription{}
	m.detailMode = detailMessages
	m.deadLetter = false
}

func (m *Model) scrollFocusedHalfPage(direction int) {
	if direction == 0 {
		return
	}

	var target *list.Model
	switch m.focus {
	case subscriptionsPane:
		target = &m.subscriptionsList
	case namespacesPane:
		target = &m.namespacesList
	case entitiesPane:
		target = &m.entitiesList
	case detailPane:
		target = &m.detailList
	default:
		return
	}

	steps := halfPageStep(*target)
	for i := 0; i < steps; i++ {
		if direction > 0 {
			target.CursorDown()
		} else {
			target.CursorUp()
		}
	}
}

func halfPageStep(l list.Model) int {
	if l.Paginator.PerPage > 1 {
		if half := l.Paginator.PerPage / 2; half > 0 {
			return half
		}
	}

	if visible := len(l.VisibleItems()); visible > 1 {
		if half := visible / 2; half > 0 {
			return half
		}
	}

	return 1
}

func (m Model) focusedListSettingFilter() bool {
	switch m.focus {
	case subscriptionsPane:
		return m.subscriptionsList.SettingFilter()
	case namespacesPane:
		return m.namespacesList.SettingFilter()
	case entitiesPane:
		return m.entitiesList.SettingFilter()
	case detailPane:
		return m.detailList.SettingFilter()
	default:
		return false
	}
}

func (m Model) refresh() (Model, tea.Cmd) {
	if m.focus == subscriptionsPane || !m.hasSubscription {
		m.loading = true
		m.lastErr = ""
		m.status = "Refreshing subscriptions..."
		return m, tea.Batch(spinner.Tick, loadSubscriptionsCmd(m.service))
	}

	if !m.hasNamespace || m.focus == namespacesPane {
		m.loading = true
		m.lastErr = ""
		m.status = fmt.Sprintf("Loading namespaces in %s", subscriptionDisplayName(m.currentSub))
		return m, tea.Batch(spinner.Tick, loadNamespacesCmd(m.service, m.currentSub.ID))
	}

	if m.focus == entitiesPane || !m.hasEntity {
		m.loading = true
		m.lastErr = ""
		m.status = fmt.Sprintf("Loading entities in %s", m.currentNS.Name)
		return m, tea.Batch(spinner.Tick, loadEntitiesCmd(m.service, m.currentNS))
	}

	return m.refreshDetail()
}

func (m Model) refreshDetail() (Model, tea.Cmd) {
	if m.currentEntity.Kind == servicebus.EntityQueue {
		m.loading = true
		m.lastErr = ""
		m.status = fmt.Sprintf("Peeking messages from queue %s", m.currentEntity.Name)
		return m, tea.Batch(spinner.Tick, peekQueueMessagesCmd(m.service, m.currentNS, m.currentEntity.Name, m.deadLetter))
	}

	if m.viewingTopicSub {
		m.loading = true
		m.lastErr = ""
		m.status = fmt.Sprintf("Peeking messages from %s/%s", m.currentEntity.Name, m.currentTopicSub.Name)
		return m, tea.Batch(spinner.Tick, peekSubscriptionMessagesCmd(m.service, m.currentNS, m.currentEntity.Name, m.currentTopicSub.Name, m.deadLetter))
	}

	m.loading = true
	m.lastErr = ""
	m.status = fmt.Sprintf("Loading subscriptions for topic %s", m.currentEntity.Name)
	return m, tea.Batch(spinner.Tick, loadTopicSubscriptionsCmd(m.service, m.currentNS, m.currentEntity.Name))
}

func (m Model) rePeekMessages() (Model, tea.Cmd) {
	m.loading = true
	m.lastErr = ""
	dlqLabel := "active"
	if m.deadLetter {
		dlqLabel = "DLQ"
	}

	if m.currentEntity.Kind == servicebus.EntityQueue {
		m.status = fmt.Sprintf("Peeking %s messages from queue %s", dlqLabel, m.currentEntity.Name)
		return m, tea.Batch(spinner.Tick, peekQueueMessagesCmd(m.service, m.currentNS, m.currentEntity.Name, m.deadLetter))
	}

	if m.viewingTopicSub {
		m.status = fmt.Sprintf("Peeking %s messages from %s/%s", dlqLabel, m.currentEntity.Name, m.currentTopicSub.Name)
		return m, tea.Batch(spinner.Tick, peekSubscriptionMessagesCmd(m.service, m.currentNS, m.currentEntity.Name, m.currentTopicSub.Name, m.deadLetter))
	}

	return m, nil
}

func (m Model) navigateLeft() (Model, tea.Cmd) {
	switch m.focus {
	case detailPane:
		if m.viewingTopicSub {
			m.viewingTopicSub = false
			m.currentTopicSub = servicebus.TopicSubscription{}
			m.peekedMessages = nil
			m.detailMode = detailTopicSubscriptions
			m.detailList.ResetFilter()
			m.detailList.SetItems(topicSubsToItems(m.topicSubs))
			m.detailList.Title = fmt.Sprintf("Topic Subscriptions (%d)", len(m.topicSubs))
			m.status = "Back to topic subscriptions"
			return m, nil
		}
		m.focus = entitiesPane
		m.status = "Focus: entities"
		return m, nil
	case entitiesPane:
		m.focus = namespacesPane
		m.status = "Focus: namespaces"
		return m, nil
	case namespacesPane:
		m.focus = subscriptionsPane
		m.status = "Focus: subscriptions"
		return m, nil
	default:
		return m, nil
	}
}

func (m Model) handleBackspace() (Model, tea.Cmd) {
	if m.focus == detailPane {
		if m.viewingTopicSub {
			m.viewingTopicSub = false
			m.currentTopicSub = servicebus.TopicSubscription{}
			m.peekedMessages = nil
			m.detailMode = detailTopicSubscriptions
			m.detailList.ResetFilter()
			m.detailList.SetItems(topicSubsToItems(m.topicSubs))
			m.detailList.Title = fmt.Sprintf("Topic Subscriptions (%d)", len(m.topicSubs))
			m.status = "Back to topic subscriptions"
			return m, nil
		}
		m.focus = entitiesPane
		m.status = "Focus: entities"
	}
	return m, nil
}

func (m Model) handleEnter() (Model, tea.Cmd) {
	if m.focus == subscriptionsPane {
		item, ok := m.subscriptionsList.SelectedItem().(subscriptionItem)
		if !ok {
			return m, nil
		}

		m.currentSub = item.subscription
		m.hasSubscription = true
		m.hasNamespace = false
		m.hasEntity = false
		m.currentNS = servicebus.Namespace{}
		m.currentEntity = servicebus.Entity{}
		m.clearDetailState()
		m.focus = namespacesPane

		m.namespaces = nil
		m.entities = nil
		m.namespacesList.ResetFilter()
		m.entitiesList.ResetFilter()
		m.detailList.ResetFilter()
		m.namespacesList.SetItems(nil)
		m.entitiesList.SetItems(nil)
		m.detailList.SetItems(nil)
		m.namespacesList.Title = "Namespaces"
		m.entitiesList.Title = "Entities"
		m.detailList.Title = "Detail"

		m.loading = true
		m.status = fmt.Sprintf("Loading namespaces in %s", subscriptionDisplayName(item.subscription))
		return m, tea.Batch(spinner.Tick, loadNamespacesCmd(m.service, item.subscription.ID))
	}

	if m.focus == namespacesPane {
		item, ok := m.namespacesList.SelectedItem().(namespaceItem)
		if !ok {
			return m, nil
		}

		m.currentNS = item.namespace
		m.hasNamespace = true
		m.hasEntity = false
		m.currentEntity = servicebus.Entity{}
		m.clearDetailState()
		m.focus = entitiesPane

		m.entities = nil
		m.entitiesList.ResetFilter()
		m.detailList.ResetFilter()
		m.entitiesList.SetItems(nil)
		m.detailList.SetItems(nil)
		m.entitiesList.Title = "Entities"
		m.detailList.Title = "Detail"

		m.loading = true
		m.status = fmt.Sprintf("Loading entities in %s", item.namespace.Name)
		return m, tea.Batch(spinner.Tick, loadEntitiesCmd(m.service, item.namespace))
	}

	if m.focus == entitiesPane {
		item, ok := m.entitiesList.SelectedItem().(entityItem)
		if !ok {
			return m, nil
		}

		m.currentEntity = item.entity
		m.hasEntity = true
		m.clearDetailState()
		m.focus = detailPane

		m.detailList.ResetFilter()
		m.detailList.SetItems(nil)
		m.detailList.Title = "Detail"

		if item.entity.Kind == servicebus.EntityQueue {
			m.loading = true
			m.status = fmt.Sprintf("Peeking messages from queue %s", item.entity.Name)
			return m, tea.Batch(spinner.Tick, peekQueueMessagesCmd(m.service, m.currentNS, item.entity.Name, m.deadLetter))
		}

		m.loading = true
		m.status = fmt.Sprintf("Loading subscriptions for topic %s", item.entity.Name)
		return m, tea.Batch(spinner.Tick, loadTopicSubscriptionsCmd(m.service, m.currentNS, item.entity.Name))
	}

	if m.focus == detailPane {
		if m.detailMode == detailTopicSubscriptions && !m.viewingTopicSub {
			item, ok := m.detailList.SelectedItem().(topicSubItem)
			if !ok {
				return m, nil
			}

			m.currentTopicSub = item.sub
			m.viewingTopicSub = true
			m.peekedMessages = nil
			m.detailList.ResetFilter()
			m.detailList.SetItems(nil)

			m.loading = true
			m.status = fmt.Sprintf("Peeking messages from %s/%s", m.currentEntity.Name, item.sub.Name)
			return m, tea.Batch(spinner.Tick, peekSubscriptionMessagesCmd(m.service, m.currentNS, m.currentEntity.Name, item.sub.Name, m.deadLetter))
		}

		if m.detailMode == detailMessages {
			item, ok := m.detailList.SelectedItem().(messageItem)
			if !ok {
				return m, nil
			}
			m.selectedMessage = item.message
			m.viewingMessage = true
			m.resize()
			m.messageViewport.SetContent(highlightJSON(item.message.FullBody))
			m.messageViewport.GotoTop()
			m.status = fmt.Sprintf("Viewing message %s (Esc/h to go back)", emptyToDash(item.message.MessageID))
			return m, nil
		}
	}

	return m, nil
}

// --- Item types ---

type subscriptionItem struct {
	subscription azure.Subscription
}

func (i subscriptionItem) Title() string {
	if strings.TrimSpace(i.subscription.Name) != "" {
		return i.subscription.Name
	}
	return i.subscription.ID
}

func (i subscriptionItem) Description() string {
	id := i.subscription.ID
	if len(id) > 12 {
		id = id[:12]
	}
	state := strings.TrimSpace(i.subscription.State)
	if state == "" {
		return fmt.Sprintf("id %s", id)
	}
	return fmt.Sprintf("%s | id %s", state, id)
}

func (i subscriptionItem) FilterValue() string {
	return i.subscription.Name + " " + i.subscription.ID + " " + i.subscription.State
}

type namespaceItem struct {
	namespace servicebus.Namespace
}

func (i namespaceItem) Title() string {
	return i.namespace.Name
}

func (i namespaceItem) Description() string {
	shortSub := i.namespace.SubscriptionID
	if len(shortSub) > 8 {
		shortSub = shortSub[:8]
	}
	if i.namespace.ResourceGroup == "" {
		return fmt.Sprintf("sub %s", shortSub)
	}
	return fmt.Sprintf("sub %s | rg %s", shortSub, i.namespace.ResourceGroup)
}

func (i namespaceItem) FilterValue() string {
	return i.namespace.Name + " " + i.namespace.SubscriptionID + " " + i.namespace.ResourceGroup
}

type entityItem struct {
	entity servicebus.Entity
}

func (i entityItem) Title() string {
	tag := "[Q]"
	if i.entity.Kind == servicebus.EntityTopic {
		tag = "[T]"
	}
	return fmt.Sprintf("%s %s", tag, i.entity.Name)
}

func (i entityItem) Description() string {
	kind := "queue"
	if i.entity.Kind == servicebus.EntityTopic {
		kind = "topic"
	}
	return fmt.Sprintf("%s · active: %d · dlq: %d", kind, i.entity.ActiveMsgCount, i.entity.DeadLetterCount)
}

func (i entityItem) FilterValue() string {
	return i.entity.Name
}

type topicSubItem struct {
	sub servicebus.TopicSubscription
}

func (i topicSubItem) Title() string { return i.sub.Name }
func (i topicSubItem) Description() string {
	return fmt.Sprintf("active: %d · dlq: %d", i.sub.ActiveMsgCount, i.sub.DeadLetterCount)
}
func (i topicSubItem) FilterValue() string { return i.sub.Name }

type messageItem struct {
	message servicebus.PeekedMessage
}

func (i messageItem) Title() string {
	id := i.message.MessageID
	if id == "" {
		id = "(no id)"
	}
	return id
}

func (i messageItem) Description() string {
	enqueued := formatTime(i.message.EnqueuedAt)
	preview := i.message.BodyPreview
	if len(preview) > 80 {
		preview = preview[:80] + "..."
	}
	if preview == "" {
		return enqueued
	}
	return fmt.Sprintf("%s | %s", enqueued, preview)
}

func (i messageItem) FilterValue() string {
	return i.message.MessageID + " " + i.message.BodyPreview
}

// --- Item conversion ---

func subscriptionsToItems(subs []azure.Subscription) []list.Item {
	items := make([]list.Item, 0, len(subs))
	for _, s := range subs {
		items = append(items, subscriptionItem{subscription: s})
	}
	return items
}

func namespacesToItems(namespaces []servicebus.Namespace) []list.Item {
	items := make([]list.Item, 0, len(namespaces))
	for _, ns := range namespaces {
		items = append(items, namespaceItem{namespace: ns})
	}
	return items
}

func entitiesToItems(entities []servicebus.Entity) []list.Item {
	items := make([]list.Item, 0, len(entities))
	for _, e := range entities {
		items = append(items, entityItem{entity: e})
	}
	return items
}

func topicSubsToItems(subs []servicebus.TopicSubscription) []list.Item {
	items := make([]list.Item, 0, len(subs))
	for _, s := range subs {
		items = append(items, topicSubItem{sub: s})
	}
	return items
}

func messagesToItems(messages []servicebus.PeekedMessage) []list.Item {
	items := make([]list.Item, 0, len(messages))
	for _, msg := range messages {
		items = append(items, messageItem{message: msg})
	}
	return items
}

// --- Async commands ---

func loadSubscriptionsCmd(svc *servicebus.Service) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		subs, err := svc.ListSubscriptions(ctx)
		return subscriptionsLoadedMsg{subscriptions: subs, err: err}
	}
}

func loadNamespacesCmd(svc *servicebus.Service, subscriptionID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		namespaces, err := svc.ListNamespaces(ctx, subscriptionID)
		return namespacesLoadedMsg{subscriptionID: subscriptionID, namespaces: namespaces, err: err}
	}
}

func loadEntitiesCmd(svc *servicebus.Service, ns servicebus.Namespace) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		entities, err := svc.ListEntities(ctx, ns)
		return entitiesLoadedMsg{namespace: ns, entities: entities, err: err}
	}
}

func loadTopicSubscriptionsCmd(svc *servicebus.Service, ns servicebus.Namespace, topicName string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		subs, err := svc.ListTopicSubscriptions(ctx, ns, topicName)
		return topicSubscriptionsLoadedMsg{namespace: ns, topicName: topicName, subs: subs, err: err}
	}
}

func peekQueueMessagesCmd(svc *servicebus.Service, ns servicebus.Namespace, queueName string, deadLetter bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		messages, err := svc.PeekQueueMessages(ctx, ns, queueName, peekMaxMessages, deadLetter)
		return messagesLoadedMsg{namespace: ns, source: queueName, messages: messages, err: err}
	}
}

func peekSubscriptionMessagesCmd(svc *servicebus.Service, ns servicebus.Namespace, topicName, subName string, deadLetter bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		messages, err := svc.PeekSubscriptionMessages(ctx, ns, topicName, subName, peekMaxMessages, deadLetter)
		return messagesLoadedMsg{namespace: ns, source: topicName + "/" + subName, messages: messages, err: err}
	}
}

// --- Helpers ---

func styleList(l *list.Model) {
	l.Styles.TitleBar = l.Styles.TitleBar.
		Foreground(lipgloss.Color(colorMuted)).
		Padding(0, 1)
	l.Styles.Title = l.Styles.Title.
		Bold(true).
		Foreground(lipgloss.Color(colorAccent))
	l.Styles.Spinner = l.Styles.Spinner.Foreground(lipgloss.Color(colorAccentStrong))
	l.Styles.FilterPrompt = l.Styles.FilterPrompt.Foreground(lipgloss.Color(colorAccent))
	l.Styles.FilterCursor = l.Styles.FilterCursor.Foreground(lipgloss.Color(colorAccentStrong))
	l.Styles.DefaultFilterCharacterMatch = l.Styles.DefaultFilterCharacterMatch.Foreground(lipgloss.Color(colorFilterMatch)).Underline(true)
	l.Styles.StatusBar = l.Styles.StatusBar.
		Foreground(lipgloss.Color(colorMuted))
	l.Styles.StatusBarActiveFilter = l.Styles.StatusBarActiveFilter.Foreground(lipgloss.Color(colorAccent)).Bold(true)
	l.Styles.StatusBarFilterCount = l.Styles.StatusBarFilterCount.Foreground(lipgloss.Color(colorAccentStrong)).Bold(true)
	l.Styles.NoItems = l.Styles.NoItems.Foreground(lipgloss.Color(colorMuted))
	l.Styles.PaginationStyle = l.Styles.PaginationStyle.Foreground(lipgloss.Color(colorMuted))
	l.Styles.HelpStyle = l.Styles.HelpStyle.Foreground(lipgloss.Color(colorMuted))
}

func paneName(pane int) string {
	switch pane {
	case subscriptionsPane:
		return "subscriptions"
	case namespacesPane:
		return "namespaces"
	case entitiesPane:
		return "entities"
	case detailPane:
		return "detail"
	default:
		return "items"
	}
}

func subscriptionDisplayName(sub azure.Subscription) string {
	if strings.TrimSpace(sub.Name) != "" {
		return sub.Name
	}
	if strings.TrimSpace(sub.ID) == "" {
		return "-"
	}
	return sub.ID
}

func entityDisplayName(e servicebus.Entity) string {
	tag := "[Q]"
	if e.Kind == servicebus.EntityTopic {
		tag = "[T]"
	}
	return fmt.Sprintf("%s %s", tag, e.Name)
}

func (m Model) subscriptionsPaneTitle() string {
	title := "Subscriptions"
	if len(m.subscriptions) > 0 {
		title = fmt.Sprintf("Subscriptions (%d)", len(m.subscriptions))
	}
	return title
}

func (m Model) namespacesPaneTitle() string {
	title := "Namespaces"
	if m.hasSubscription {
		title = fmt.Sprintf("Namespaces · %s", subscriptionDisplayName(m.currentSub))
	}
	if len(m.namespaces) > 0 {
		title = fmt.Sprintf("%s (%d)", title, len(m.namespaces))
	}
	return title
}

func (m Model) entitiesPaneTitle() string {
	title := "Entities"
	if m.hasNamespace {
		title = fmt.Sprintf("Entities · %s", m.currentNS.Name)
	}
	if m.entities != nil {
		title = fmt.Sprintf("%s (%d)", title, len(m.entities))
	}
	return title
}

func (m Model) detailPaneTitle() string {
	if !m.hasEntity {
		return "Detail"
	}

	queueLabel := "ACTIVE"
	if m.deadLetter {
		queueLabel = "DLQ"
	}

	if m.currentEntity.Kind == servicebus.EntityQueue {
		title := fmt.Sprintf("[%s] %s", queueLabel, m.currentEntity.Name)
		if m.peekedMessages != nil {
			title = fmt.Sprintf("%s (%d)", title, len(m.peekedMessages))
		}
		return title
	}

	if m.viewingTopicSub {
		title := fmt.Sprintf("[%s] %s/%s", queueLabel, m.currentEntity.Name, m.currentTopicSub.Name)
		if m.peekedMessages != nil {
			title = fmt.Sprintf("%s (%d)", title, len(m.peekedMessages))
		}
		return title
	}

	title := fmt.Sprintf("Topic Subs · %s", m.currentEntity.Name)
	if m.topicSubs != nil {
		title = fmt.Sprintf("%s (%d)", title, len(m.topicSubs))
	}
	return title
}

func trimToWidth(s string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max <= 3 {
		return string(r[:max])
	}
	return string(r[:max-3]) + "..."
}

func emptyToDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Local().Format("2006-01-02 15:04")
}

func truncateForStatus(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

var (
	jsonKeyStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color(colorAccent))
	jsonStringStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colorAccentStrong))
	jsonNumberStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colorFilterMatch))
	jsonNullStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color(colorDanger))
	jsonPunctStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color(colorMuted))
)

func highlightJSON(body string) string {
	var buf bytes.Buffer
	if err := json.Indent(&buf, []byte(body), "", "  "); err != nil {
		return body
	}

	formatted := buf.String()
	var out strings.Builder
	lines := strings.Split(formatted, "\n")

	for i, line := range lines {
		if i > 0 {
			out.WriteByte('\n')
		}
		out.WriteString(colorizeLine(line))
	}

	return out.String()
}

func colorizeLine(line string) string {
	trimmed := strings.TrimLeftFunc(line, unicode.IsSpace)
	indent := line[:len(line)-len(trimmed)]

	if trimmed == "" {
		return line
	}

	var out strings.Builder
	out.WriteString(indent)

	if trimmed[0] == '"' {
		colonIdx := strings.Index(trimmed, "\":")
		if colonIdx > 0 {
			key := trimmed[:colonIdx+1]
			rest := trimmed[colonIdx+1:]
			out.WriteString(jsonKeyStyle.Render(key))
			out.WriteString(jsonPunctStyle.Render(":"))
			val := strings.TrimSpace(rest[1:])
			if val != "" {
				out.WriteString(" ")
				out.WriteString(colorizeValue(val))
			}
			return out.String()
		}
		out.WriteString(colorizeValue(trimmed))
		return out.String()
	}

	out.WriteString(colorizeValue(trimmed))
	return out.String()
}

func colorizeValue(val string) string {
	if val == "" {
		return val
	}

	trailing := ""
	clean := val
	for strings.HasSuffix(clean, ",") {
		trailing = "," + trailing
		clean = clean[:len(clean)-1]
	}

	var styled string
	switch {
	case clean == "{" || clean == "}" || clean == "[" || clean == "]" ||
		clean == "{}" || clean == "[]":
		styled = jsonPunctStyle.Render(clean)
	case clean == "null":
		styled = jsonNullStyle.Render(clean)
	case clean == "true" || clean == "false":
		styled = jsonNumberStyle.Render(clean)
	case len(clean) > 0 && clean[0] == '"':
		styled = jsonStringStyle.Render(clean)
	default:
		styled = jsonNumberStyle.Render(clean)
	}

	if trailing != "" {
		return styled + jsonPunctStyle.Render(trailing)
	}
	return styled
}
