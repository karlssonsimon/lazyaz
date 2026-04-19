package dashapp

import (
	"time"

	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/azure/servicebus"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	"charm.land/bubbles/v2/cursor"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// dashboardRefreshInterval is how often the dashboard re-fires its
// fetches in the background. The refreshInFlight guard means a slow
// network can't pile up overlapping refreshes — if the previous one
// hasn't finished by the next tick, the tick is a no-op.
const dashboardRefreshInterval = 30 * time.Second

// refreshTickMsg is the periodic auto-refresh trigger. Scheduled by
// scheduleRefreshTick; the handler re-schedules itself so the chain
// runs as long as the dashboard tab is alive.
type refreshTickMsg struct{}

func scheduleRefreshTick() tea.Cmd {
	return tea.Tick(dashboardRefreshInterval, func(time.Time) tea.Msg {
		return refreshTickMsg{}
	})
}

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.Spinner.Tick, cursor.Blink, scheduleRefreshTick()}
	if m.SubOverlay.Active || len(m.Subscriptions) == 0 {
		cmds = append(cmds, fetchSubscriptionsCmd(m.service, m.stores.Subscriptions, m.Subscriptions))
	}
	if m.HasSubscription {
		cmds = append(cmds, m.kickoffFetches()...)
	}
	return tea.Batch(cmds...)
}

// kickoffFetches fans out fetches that populate the widgets: namespaces
// for the current subscription, then entities per namespace (scheduled
// once the namespace list arrives). topic subscriptions are fetched
// lazily once entities reveal which namespaces contain topics.
func (m *Model) kickoffFetches() []tea.Cmd {
	cmds := []tea.Cmd{
		fetchNamespacesCmd(m.service, m.stores.Namespaces, m.CurrentSub.ID, m.namespaces),
	}
	for _, ns := range m.namespaces {
		cmds = append(cmds, fetchEntitiesCmd(m.service, m.stores.Entities, ns))
		m.pendingFetches++
	}
	return cmds
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if paste, ok := msg.(tea.PasteMsg); ok {
		text := paste.String()
		if m.SubOverlay.Active {
			m.SubOverlay.Query += text
			m.SubOverlay.Refilter(m.Subscriptions)
		}
		return m, nil
	}

	if cursorModel, cursorCmd := m.Cursor.Update(msg); cursorCmd != nil {
		m.Cursor = cursorModel
		return m, cursorCmd
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height
		m.recomputeWidgetHeights()
		return m, nil

	case spinner.TickMsg:
		// Keep ticking while the initial-load Loading flag is set OR a
		// refresh is in flight — the widget titles render the spinner
		// glyph during either, so they need fresh frames.
		if !m.Loading && m.refreshInFlight == 0 {
			return m, nil
		}
		var cmd tea.Cmd
		m.Spinner, cmd = m.Spinner.Update(msg)
		return m, cmd

	case appshell.SubscriptionsLoadedMsg:
		return m.handleSubscriptionsLoaded(msg)

	case namespacesLoadedMsg:
		return m.handleNamespacesLoaded(msg)

	case entitiesLoadedMsg:
		return m.handleEntitiesLoaded(msg)

	case topicSubsLoadedMsg:
		return m.handleTopicSubsLoaded(msg)

	case refreshTickMsg:
		// refreshAll is debounced via refreshInFlight, so a slow
		// network can't cause overlapping refreshes. Always reschedule
		// so the tick chain continues for the tab's lifetime.
		updated, refreshCmd := m.refreshAll()
		return updated, tea.Batch(refreshCmd, scheduleRefreshTick())

	case openSortOverlayMsg:
		if m.focusedIdx < 0 || m.focusedIdx >= len(m.widgets) {
			return m, nil
		}
		fields := m.widgets[m.focusedIdx].SortFields()
		if len(fields) == 0 {
			return m, nil
		}
		m.sortOverlay.open(m.focusedIdx, fields, m.viewStates[m.focusedIdx])
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m Model) handleSubscriptionsLoaded(msg appshell.SubscriptionsLoadedMsg) (tea.Model, tea.Cmd) {
	m.Subscriptions = msg.Subscriptions
	if msg.Err != nil {
		m.ClearLoading()
		m.LastErr = msg.Err.Error()
		return m, nil
	}
	if !msg.Done {
		return m, msg.Next
	}
	m.ClearLoading()
	m.Status = ""
	// Try to apply a preferred subscription now that the list is known.
	if sub, ok := m.TryApplyPreferredSubscription(); ok {
		m.SetSubscription(sub)
		return m, tea.Batch(m.kickoffFetches()...)
	}
	return m, nil
}

func (m Model) handleNamespacesLoaded(msg namespacesLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.subscriptionID != m.CurrentSub.ID {
		// Stale result from a previous subscription — ignore.
		return m, msg.next
	}
	if msg.err != nil {
		m.LastErr = msg.err.Error()
		m.refreshDone()
		return m, nil
	}
	// Skip empty intermediate pages — the broker emits one to a second
	// subscriber that joins a stream before it has produced any data.
	// The final page is always authoritative, so a real empty result
	// still lands.
	if !msg.done && len(msg.namespaces) == 0 {
		return m, msg.next
	}
	m.namespaces = msg.namespaces
	if !msg.done {
		return m, msg.next
	}
	m.refreshDone()
	m.ClearLoading()
	m.clampCursorsToData()
	// On final page, kick off entity fetches for any namespace we
	// haven't already started or finished.
	var cmds []tea.Cmd
	for _, ns := range m.namespaces {
		if _, have := m.entitiesByNS[ns.Name]; have {
			continue
		}
		cmds = append(cmds, fetchEntitiesCmd(m.service, m.stores.Entities, ns))
		m.pendingFetches++
	}
	return m, tea.Batch(cmds...)
}

func (m Model) handleEntitiesLoaded(msg entitiesLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.LastErr = msg.err.Error()
		m.pendingFetches--
		m.refreshDone()
		return m, nil
	}
	if !msg.done && len(msg.entities) == 0 {
		return m, msg.next
	}
	m.entitiesByNS[msg.namespace.Name] = msg.entities
	if !msg.done {
		return m, msg.next
	}
	m.pendingFetches--
	m.refreshDone()
	m.clampCursorsToData()
	// For any topics in this namespace, kick off a topic-subs fetch so
	// the DLQ widget can see per-subscription DLQ counts.
	var cmds []tea.Cmd
	for _, e := range msg.entities {
		if e.Kind != servicebus.EntityTopic {
			continue
		}
		key := msg.namespace.Name + "/" + e.Name
		if _, have := m.topicSubsByKey[key]; have {
			continue
		}
		cmds = append(cmds, fetchTopicSubsCmd(m.service, m.stores.TopicSubs, msg.namespace, e.Name))
	}
	return m, tea.Batch(cmds...)
}

func (m Model) handleTopicSubsLoaded(msg topicSubsLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.LastErr = msg.err.Error()
		m.refreshDone()
		return m, nil
	}
	if !msg.done && len(msg.subs) == 0 {
		return m, msg.next
	}
	key := msg.namespace.Name + "/" + msg.topicName
	m.topicSubsByKey[key] = msg.subs
	if !msg.done {
		return m, msg.next
	}
	m.refreshDone()
	m.clampCursorsToData()
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Overlay handling (subscription picker).
	if res := m.HandleOverlayKeys(key); res.Handled {
		if res.SelectSub != nil {
			m.SetSubscription(*res.SelectSub)
			return m, tea.Batch(m.kickoffFetches()...)
		}
		return m, nil
	}

	km := m.Keymap

	// Filter input mode — typed characters extend the focused widget's
	// filter; nothing else fires. Has to come BEFORE the action menu
	// and other shortcuts so e.g. `/test` doesn't trigger `t`-bound
	// actions.
	if m.filterInputActive {
		return m.handleFilterKey(key)
	}

	// Sort overlay — owns input while open. Each entry is a fully
	// specified (field, direction) combo (or "Default" = clear sort).
	// applySortResult mutates view state directly; no toggle semantics.
	if m.sortOverlay.active {
		if res := m.sortOverlay.handleKey(key, km); res.applied {
			m.applySortResult(res)
		}
		return m, nil
	}

	// Action menu overlay — consumes input while open. Direct action
	// keybinds inside the menu fire and close; up/down navigate;
	// enter confirms; esc cancels.
	if m.actionMenu.active {
		if a, fired := m.actionMenu.handleKey(key, km); fired {
			return m, fireAction(a)
		}
		return m, nil
	}

	// `a` opens the action menu for the focused widget's cursor row.
	if km.ActionMenu.Matches(key) {
		var actions []Action
		if m.focusedIdx >= 0 && m.focusedIdx < len(m.widgets) {
			actions = m.widgets[m.focusedIdx].Actions(&m, m.focusedCursor())
		}
		m.actionMenu.open(actions)
		return m, nil
	}

	// `/` enters filter input mode for the focused widget.
	if km.FilterInput.Matches(key) {
		m.filterInputActive = true
		return m, nil
	}

	// gg jump-to-top chord: first 'g' arms the prefix, second 'g' fires.
	// Any other key clears the prefix so unrelated input doesn't trigger
	// the jump on the next 'g'.
	if km.WidgetScrollTop.Matches(key) {
		if m.gPrefixActive {
			m.gPrefixActive = false
			m.cursorToTop()
			return m, nil
		}
		m.gPrefixActive = true
		return m, nil
	}
	m.gPrefixActive = false

	switch {
	case km.WidgetScrollBottom.Matches(key):
		m.cursorToBottom()
		return m, nil
	case km.WidgetLeft.Matches(key):
		m.focusedIdx = moveFocus(m.widgets, m.focusedIdx, 0, -1)
		return m, nil
	case km.WidgetDown.Matches(key):
		m.focusedIdx = moveFocus(m.widgets, m.focusedIdx, 1, 0)
		return m, nil
	case km.WidgetUp.Matches(key):
		m.focusedIdx = moveFocus(m.widgets, m.focusedIdx, -1, 0)
		return m, nil
	case km.WidgetRight.Matches(key):
		m.focusedIdx = moveFocus(m.widgets, m.focusedIdx, 0, 1)
		return m, nil
	case km.WidgetScrollUp.Matches(key):
		m.moveCursorFocused(-1)
		return m, nil
	case km.WidgetScrollDown.Matches(key):
		m.moveCursorFocused(1)
		return m, nil
	case km.HalfPageUp.Matches(key):
		m.moveCursorFocused(-m.halfPageStep())
		return m, nil
	case km.HalfPageDown.Matches(key):
		m.moveCursorFocused(m.halfPageStep())
		return m, nil
	case km.SubscriptionPicker.Matches(key):
		m.SubOverlay.Open()
		return m, nil
	case km.RefreshScope.Matches(key):
		return m.refreshAll()
	case km.ReloadSubscriptions.Matches(key):
		m.Subscriptions = nil
		m.SubOverlay.Open()
		return m, fetchSubscriptionsCmd(m.service, m.stores.Subscriptions, nil)
	}

	// Per-widget action keybinds. Each widget exposes its own actions
	// for the current cursor row; if any matches the keypress, fire it.
	if m.focusedIdx >= 0 && m.focusedIdx < len(m.widgets) {
		w := m.widgets[m.focusedIdx]
		for _, a := range w.Actions(&m, m.focusedCursor()) {
			if a.Key == key && a.Cmd != nil {
				return m, a.Cmd
			}
		}
	}
	return m, nil
}

// applySortResult mutates the focused widget's view state per the sort
// overlay's outcome. clear means "remove sort entirely" (Default option).
// Cursor + scroll reset so the highlight doesn't dangle on a row that
// just moved.
func (m *Model) applySortResult(res sortResult) {
	if m.focusedIdx < 0 || m.focusedIdx >= len(m.viewStates) {
		return
	}
	vs := m.viewStates[m.focusedIdx]
	if res.clear {
		vs.hasSort = false
		vs.sortField = 0
		vs.sortDesc = false
	} else {
		vs.hasSort = true
		vs.sortField = res.field
		vs.sortDesc = res.desc
	}
	m.viewStates[m.focusedIdx] = vs
	m.cursors[m.focusedIdx] = 0
	m.offsets[m.focusedIdx] = 0
}

// handleFilterKey processes input while the focused widget's filter
// box is open. esc cancels (clears filter), enter accepts (keeps the
// filter, closes the box), backspace deletes, printable chars append.
func (m Model) handleFilterKey(key string) (tea.Model, tea.Cmd) {
	if m.focusedIdx < 0 || m.focusedIdx >= len(m.viewStates) {
		m.filterInputActive = false
		return m, nil
	}
	vs := m.viewStates[m.focusedIdx]
	switch key {
	case "esc":
		vs.filter = ""
		m.filterInputActive = false
	case "enter":
		m.filterInputActive = false
	case "backspace":
		if len(vs.filter) > 0 {
			vs.filter = vs.filter[:len(vs.filter)-1]
		}
	default:
		// Single printable character. Multi-byte input arrives as a
		// >1-rune key string (e.g. "shift+a" = "A"). For our scope
		// (ASCII substring filter) accepting single-rune keys is
		// enough — anything else is treated as a no-op.
		if len(key) == 1 && key[0] >= 32 && key[0] < 127 {
			vs.filter += key
		}
	}
	m.viewStates[m.focusedIdx] = vs
	// Filter changed → reset cursor + scroll so the highlight doesn't
	// dangle past the now-shorter list.
	m.cursors[m.focusedIdx] = 0
	m.offsets[m.focusedIdx] = 0
	return m, nil
}

// refreshAll re-fires every fetch that powers the widgets: namespaces,
// entities per namespace, and topic subscriptions per known topic.
// Existing data stays on screen; each handler replaces its slice when
// new pages arrive, so the refresh is silent from the user's side.
//
// Re-entry is blocked via refreshInFlight so rapid key presses don't
// pile up broker subscribers — each extra subscriber would receive the
// full sequence of pages, flooding the message loop.
func (m Model) refreshAll() (tea.Model, tea.Cmd) {
	if !m.HasSubscription || m.refreshInFlight > 0 {
		return m, nil
	}
	cmds := []tea.Cmd{
		// Wake the spinner so the widget-title indicator animates while
		// the refresh fans out. The TickMsg handler stops when
		// refreshInFlight returns to zero.
		m.Spinner.Tick,
		fetchNamespacesCmd(m.service, m.stores.Namespaces, m.CurrentSub.ID, m.namespaces),
	}
	m.refreshInFlight++
	for _, ns := range m.namespaces {
		cmds = append(cmds, fetchEntitiesCmd(m.service, m.stores.Entities, ns))
		m.refreshInFlight++
		for _, e := range m.entitiesByNS[ns.Name] {
			if e.Kind == servicebus.EntityTopic {
				cmds = append(cmds, fetchTopicSubsCmd(m.service, m.stores.TopicSubs, ns, e.Name))
				m.refreshInFlight++
			}
		}
	}
	return m, tea.Batch(cmds...)
}

// refreshDone decrements refreshInFlight for one finished fetch. Clamped
// at zero so non-refresh fetches (e.g. initial load) can't underflow.
func (m *Model) refreshDone() {
	if m.refreshInFlight > 0 {
		m.refreshInFlight--
	}
}

// recomputeWidgetHeights renders the (cheap) sub + status bars to read
// their actual lipgloss heights, then runs the same row-distribution
// View() uses. Stashed results let scroll math match the renderer
// exactly.
func (m *Model) recomputeWidgetHeights() {
	if m.Width <= 0 || m.Height <= 0 {
		return
	}
	subBar := ui.RenderSubscriptionBar(m.CurrentSub, m.HasSubscription, m.Styles, m.Width)
	statusBar := ui.RenderStatusBar(m.Styles, m.statusBarItems(), "", false, m.Width)
	body := m.Height - lipgloss.Height(subBar) - lipgloss.Height(statusBar)
	if body < 4 {
		body = 4
	}
	rows, _ := gridDims(m.widgets)
	m.rowHeights = computeRowHeights(body, rows)
}
