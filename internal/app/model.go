package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/azure"
	"github.com/karlssonsimon/lazyaz/internal/azure/blob"
	"github.com/karlssonsimon/lazyaz/internal/azure/keyvault"
	"github.com/karlssonsimon/lazyaz/internal/azure/servicebus"
	"github.com/karlssonsimon/lazyaz/internal/blobapp"
	"github.com/karlssonsimon/lazyaz/internal/cache"
	"github.com/karlssonsimon/lazyaz/internal/dashapp"
	"github.com/karlssonsimon/lazyaz/internal/jumplist"
	"github.com/karlssonsimon/lazyaz/internal/keymap"
	"github.com/karlssonsimon/lazyaz/internal/kvapp"
	"github.com/karlssonsimon/lazyaz/internal/sbapp"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	"charm.land/bubbles/v2/cursor"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
)

// toastTickInterval is how often the parent re-renders while toasts
// are visible. 100ms is well below the 3-second toast lifetime, so
// expiry feels instant without burning CPU when nothing's happening
// (the ticker self-extinguishes when no toasts are active).
const toastTickInterval = 100 * time.Millisecond

// Model is the top-level Bubble Tea model that manages tabs.
type Model struct {
	tabs      []Tab
	activeIdx int
	nextID    int

	blobSvc *blob.Service
	sbSvc   *servicebus.Service
	kvSvc   *keyvault.Service

	brokers sharedBrokers
	cfg    ui.Config
	keymap keymap.Keymap

	// db is the persistent cache handle, also used for app-level
	// preferences (last selected subscription, etc.). May be nil if
	// the user opted out of disk cache.
	db *cache.DB

	// lastPersistedSubID tracks what's currently in the preferences
	// table so we only write when the active subscription actually
	// changes. Compared after every Update.
	lastPersistedSubID string

	// jumps is the cross-tab navigation history walked by ctrl+o /
	// ctrl+i. jumpIdx points at the entry the user is currently "on";
	// -1 means no entries yet. See internal/app/jumplist.go for the
	// full semantics.
	jumps   []jumpEntry
	jumpIdx int

	// notifier is the single global notification log shared with every
	// tab. The parent owns it and renders both the toast stack and the
	// history overlay from it.
	notifier *appshell.Notifier

	// toastTickActive is true while a self-extinguishing tea.Tick is
	// running to drive toast expiry re-renders. Set when the first
	// active toast appears, cleared when no toasts are active.
	toastTickActive bool

	styles               ui.Styles
	schemes              []ui.Scheme
	themeOverlay         ui.ThemeOverlayState
	helpOverlay          ui.HelpOverlayState
	notificationsOverlay ui.NotificationsOverlayState
	streamOverlay        ui.StreamOverlayState

	cursor       cursor.Model
	tabPicker    tabPickerState
	tenantPicker tenantPickerState
	cmdPalette   commandPalette

	// pendingLoginMsg is the success toast queued by applyNewCredential,
	// shown once the post-login subscription fetch completes.
	pendingLoginMsg string

	width  int
	height int
}

// lastSubscriptionPrefKey is the preferences key holding the most
// recently selected subscription ID, used as the preferred sub for
// fallback default tabs and for any configured tab that doesn't
// specify its own subscription.
const lastSubscriptionPrefKey = "last_subscription"

// NewModel creates the parent tabbed model.
// If db is non-nil, a persistent SQLite cache is used; otherwise in-memory.
func NewModel(blobSvc *blob.Service, sbSvc *servicebus.Service, kvSvc *keyvault.Service, cfg ui.Config, db *cache.DB, km keymap.Keymap) Model {
	cur := cursor.New()
	cur.SetChar(" ")
	cur.Focus() // pre-focus; Init() returns the blink cmd
	m := Model{
		blobSvc:  blobSvc,
		sbSvc:    sbSvc,
		kvSvc:    kvSvc,
		brokers:  newSharedBrokers(db),
		cfg:      cfg,
		keymap:   km,
		cursor:   cur,
		schemes:  cfg.Schemes,
		notifier: appshell.NewNotifier(1000),
		db:       db,
		jumpIdx:  -1,
		themeOverlay: ui.ThemeOverlayState{
			ActiveThemeIdx: ui.ActiveSchemeIndex(cfg),
		},
	}
	m.styles = ui.NewStyles(cfg.ActiveScheme())

	// Last subscription used in a previous session (best-effort read,
	// missing or empty → no preferred sub). Used for any tab whose
	// config doesn't pin a specific subscription, including the
	// fallback default tab when no tabs are configured at all.
	lastSub, _ := db.GetPreference(lastSubscriptionPrefKey)
	m.lastPersistedSubID = lastSub

	// Resolve configured startup tabs. Invalid kinds are skipped and
	// surfaced as a warning on the first opened tab. If nothing valid
	// remains, fall back to a single dashboard tab — it gives an
	// at-a-glance view of all the data the explorers also surface.
	var warnings []string
	opened := 0
	for _, tc := range cfg.Tabs {
		kind, ok := TabKindFromString(tc.Kind)
		if !ok {
			warnings = append(warnings, fmt.Sprintf("unknown tab kind %q in config", tc.Kind))
			continue
		}
		preferred := tc.Subscription
		if preferred == "" {
			preferred = lastSub
		}
		m.addTab(kind, preferred)
		opened++
	}
	if opened == 0 {
		m.addTab(TabDashboard, lastSub)
	}
	m.activeIdx = 0

	if len(warnings) > 0 && len(m.tabs) > 0 {
		m.setTabStatus(0, "Config: "+strings.Join(warnings, "; "))
	}

	return m
}

// setTabStatus pokes a status string into the given tab's appshell so
// the user sees parent-level warnings (e.g. unknown tab kinds) on the
// child's status bar — the parent has no status bar of its own.
func (m *Model) setTabStatus(idx int, status string) {
	if idx < 0 || idx >= len(m.tabs) {
		return
	}
	switch child := m.tabs[idx].Model.(type) {
	case blobapp.Model:
		child.Notify(appshell.LevelWarn, status)
		m.tabs[idx].Model = child
	case sbapp.Model:
		child.Notify(appshell.LevelWarn, status)
		m.tabs[idx].Model = child
	case kvapp.Model:
		child.Notify(appshell.LevelWarn, status)
		m.tabs[idx].Model = child
	case dashapp.Model:
		child.Notify(appshell.LevelWarn, status)
		m.tabs[idx].Model = child
	}
}

func (m *Model) addTab(kind TabKind, preferredSub string) {
	id := m.nextID
	m.nextID++

	// Inherit the active tab's subscription so new tabs start in context.
	sub, hasSub := m.activeSubscription()

	// applyInitialSub wires up the tab's starting subscription. The
	// inherited active-tab sub takes precedence over the configured
	// preferred sub. If neither matches a known subscription up front,
	// preferredSub is stashed on the model so handleSubscriptionsLoaded
	// can apply it once a fetch completes.
	//
	// Note: SetSubscription must be called on the *outer* app model
	// pointer (e.g. *blobapp.Model), not the embedded *appshell.Model.
	// Each app overrides SetSubscription to also seed the per-resource
	// fetch session — passing the embedded pointer would dispatch to
	// the appshell base method and the override would never run, which
	// silently drops the data on the next fetch.
	applyInitialSub := func(s interface {
		SetSubscription(azure.Subscription)
		SetPreferredSubscription(string)
		TryApplyPreferredSubscription() (azure.Subscription, bool)
	}) {
		if hasSub {
			s.SetSubscription(sub)
			return
		}
		if preferredSub == "" {
			return
		}
		s.SetPreferredSubscription(preferredSub)
		if matched, ok := s.TryApplyPreferredSubscription(); ok {
			s.SetSubscription(matched)
		}
	}

	var child tea.Model
	switch kind {
	case TabBlob:
		bm := blobapp.NewModelWithCache(m.blobSvc, m.cfg, blobapp.BlobStores{
			Subscriptions: m.brokers.subscriptions,
			Accounts:      m.brokers.blobAccounts,
			Containers:    m.brokers.blobContainers,
			Blobs:         m.brokers.blobs,
			Usage:         m.db,
		}, m.keymap)
		bm.EmbeddedMode = true
		bm.Notifier = m.notifier
		applyInitialSub(&bm)
		child = bm
	case TabServiceBus:
		sm := sbapp.NewModelWithCache(m.sbSvc, m.cfg, sbapp.SBStores{
			Subscriptions: m.brokers.subscriptions,
			Namespaces:    m.brokers.sbNamespaces,
			Entities:      m.brokers.sbEntities,
			TopicSubs:     m.brokers.sbTopicSubs,
			Usage:         m.db,
		}, m.keymap)
		sm.EmbeddedMode = true
		sm.Notifier = m.notifier
		applyInitialSub(&sm)
		child = sm
	case TabKeyVault:
		kvm := kvapp.NewModelWithCache(m.kvSvc, m.cfg, kvapp.KVStores{
			Subscriptions: m.brokers.subscriptions,
			Vaults:        m.brokers.kvVaults,
			Secrets:       m.brokers.kvSecrets,
			Versions:      m.brokers.kvVersions,
		}, m.keymap)
		kvm.EmbeddedMode = true
		kvm.Notifier = m.notifier
		applyInitialSub(&kvm)
		child = kvm
	case TabDashboard:
		dm := dashapp.NewModelWithCache(m.sbSvc, m.cfg, dashapp.DashStores{
			Subscriptions: m.brokers.subscriptions,
			Namespaces:    m.brokers.sbNamespaces,
			Entities:      m.brokers.sbEntities,
			TopicSubs:     m.brokers.sbTopicSubs,
			DB:            m.db,
		}, m.keymap)
		dm.EmbeddedMode = true
		dm.Notifier = m.notifier
		applyInitialSub(&dm)
		child = dm
	}

	m.tabs = append(m.tabs, Tab{ID: id, Kind: kind, Model: child})
	m.activeIdx = len(m.tabs) - 1
}

// openSBTabWithNav creates a new Service Bus tab pre-positioned to a
// pending navigation target (namespace, entity, DLQ pane). The dashboard
// emits this when a widget action wants to drill into a specific entity.
// The new tab inherits the requested subscription and runs the same
// init flow as a tab opened via the picker.
//
// SetPendingNav fast-forwards through cached layers, so when the
// dashboard has already warmed the brokers the user lands on the
// destination immediately rather than watching three sequential fetches.
func (m *Model) openSBTabWithNav(sub azure.Subscription, nav sbapp.PendingNav) (tea.Model, tea.Cmd) {
	oldIdx := m.activeIdx
	m.addTab(TabServiceBus, sub.ID)
	// Record the FROM tab so ctrl+o brings the user back from the
	// SB tab they just got dropped into.
	if oldIdx >= 0 && oldIdx < len(m.tabs) {
		if snap := m.tabSnapshotForJump(oldIdx); snap != nil {
			m.recordJump(m.tabs[oldIdx].ID, snap)
		}
	}
	tab := &m.tabs[m.activeIdx]
	var ffCmd tea.Cmd
	if sm, ok := tab.Model.(sbapp.Model); ok {
		// SetSubscription explicitly so the tab is wired to the
		// requested sub even if it isn't in the cached subscriptions
		// list yet (preferredSub-based deferred apply wouldn't fire).
		sm.SetSubscription(sub)
		ffCmd = sm.SetPendingNav(nav)
		tab.Model = sm
	}
	// Record the destination as a jump entry so ctrl+o brings the
	// user back to whatever they were doing before the dashboard
	// drilled them in here.
	m.recordJump(tab.ID, sbapp.NavSnapshotFromPending(nav))
	initCmd := wrapCmd(tab.ID, tab.Model.Init())
	resizeCmd := m.forwardToActive(tea.WindowSizeMsg{
		Width:  m.width,
		Height: m.childHeight(),
	})
	return m, tea.Batch(initCmd, wrapCmd(tab.ID, ffCmd), resizeCmd)
}

// persistActiveSubIfChanged writes the active tab's subscription ID to
// the preferences table when it differs from what's currently there.
// Called after every Update via the Update wrapper. Cheap (one string
// compare in the common case where nothing changed); idempotent. The
// receiver is value but updates lastPersistedSubID via a pointer because
// the wrapper hands us a freshly returned Model copy.
func (m *Model) persistActiveSubIfChanged() {
	if m.db == nil {
		return
	}
	sub, ok := m.activeSubscription()
	if !ok || sub.ID == "" {
		return
	}
	if sub.ID == m.lastPersistedSubID {
		return
	}
	m.db.SetPreference(lastSubscriptionPrefKey, sub.ID)
	m.lastPersistedSubID = sub.ID
}

// openBlobTabWithNav mirrors openSBTabWithNav for Blob storage. Creates
// a blob tab on the requested subscription and stashes a pending
// navigation target. The fast-forward path in SetPendingNav lands the
// user directly on the destination when the cache is warm.
func (m *Model) openBlobTabWithNav(sub azure.Subscription, nav blobapp.PendingNav) (tea.Model, tea.Cmd) {
	oldIdx := m.activeIdx
	m.addTab(TabBlob, sub.ID)
	if oldIdx >= 0 && oldIdx < len(m.tabs) {
		if snap := m.tabSnapshotForJump(oldIdx); snap != nil {
			m.recordJump(m.tabs[oldIdx].ID, snap)
		}
	}
	tab := &m.tabs[m.activeIdx]
	var ffCmd tea.Cmd
	if bm, ok := tab.Model.(blobapp.Model); ok {
		bm.SetSubscription(sub)
		ffCmd = bm.SetPendingNav(nav)
		tab.Model = bm
	}
	m.recordJump(tab.ID, blobapp.NavSnapshotFromPending(nav))
	initCmd := wrapCmd(tab.ID, tab.Model.Init())
	resizeCmd := m.forwardToActive(tea.WindowSizeMsg{
		Width:  m.width,
		Height: m.childHeight(),
	})
	return m, tea.Batch(initCmd, wrapCmd(tab.ID, ffCmd), resizeCmd)
}

// activeSubscription returns the current subscription from the active tab.
func (m *Model) activeSubscription() (azure.Subscription, bool) {
	if len(m.tabs) == 0 {
		return azure.Subscription{}, false
	}
	switch child := m.tabs[m.activeIdx].Model.(type) {
	case blobapp.Model:
		return child.CurrentSubscription()
	case sbapp.Model:
		return child.CurrentSubscription()
	case kvapp.Model:
		return child.CurrentSubscription()
	case dashapp.Model:
		return child.CurrentSubscription()
	}
	return azure.Subscription{}, false
}

// activeChildTextInput returns true when the active tab's child model
// is accepting free-form text input (e.g. list filter). The parent
// uses this to suppress single-key shortcuts so they don't fire while
// the user is typing.
func (m *Model) activeChildTextInput() bool {
	if len(m.tabs) == 0 {
		return false
	}
	switch child := m.tabs[m.activeIdx].Model.(type) {
	case blobapp.Model:
		return child.IsTextInputActive()
	case sbapp.Model:
		return child.IsTextInputActive()
	case kvapp.Model:
		return child.IsTextInputActive()
	case dashapp.Model:
		return child.IsTextInputActive()
	}
	return false
}

func (m *Model) closeTab(idx int) {
	if idx < 0 || idx >= len(m.tabs) {
		return
	}
	closedID := m.tabs[idx].ID
	m.tabs = append(m.tabs[:idx], m.tabs[idx+1:]...)
	if m.activeIdx >= len(m.tabs) {
		m.activeIdx = len(m.tabs) - 1
	}
	if m.activeIdx < 0 {
		m.activeIdx = 0
	}
	// Drop jump-list entries pointing at the now-gone tab so ctrl+o
	// doesn't end up at "random" surviving entries instead of the
	// position the user expected.
	m.cleanupJumpsForTab(closedID)
}

func (m *Model) closeTabByID(id int) {
	for i, t := range m.tabs {
		if t.ID == id {
			m.closeTab(i)
			return
		}
	}
}

func (m *Model) tabIndexByID(id int) int {
	for i, t := range m.tabs {
		if t.ID == id {
			return i
		}
	}
	return -1
}

func (m *Model) applySchemeToAll(scheme ui.Scheme) {
	m.styles = ui.NewStyles(scheme)
	// Keep cfg in sync so newly opened tabs pick up the current theme
	// instead of the one that was active at program start.
	m.cfg.ThemeName = scheme.Name
	for i := range m.tabs {
		switch child := m.tabs[i].Model.(type) {
		case blobapp.Model:
			child.ApplyScheme(scheme)
			m.tabs[i].Model = child
		case sbapp.Model:
			child.ApplyScheme(scheme)
			m.tabs[i].Model = child
		case kvapp.Model:
			child.ApplyScheme(scheme)
			m.tabs[i].Model = child
		case dashapp.Model:
			child.ApplyScheme(scheme)
			m.tabs[i].Model = child
		}
	}
}

func (m Model) Init() tea.Cmd {
	if len(m.tabs) == 0 {
		return cursor.Blink
	}
	// Init every tab so configured startup tabs all kick off their
	// initial fetches in the background — not just the active one.
	// Each tab's commands are wrapped so their results route back to
	// the correct tab via tabMsg.
	cmds := []tea.Cmd{cursor.Blink}
	for _, tab := range m.tabs {
		if c := tab.Model.Init(); c != nil {
			cmds = append(cmds, wrapCmd(tab.ID, c))
		}
	}
	return tea.Batch(cmds...)
}

// Update runs the inner dispatcher and then persists the active
// subscription if it changed. Wrapping at this single point catches
// every state transition (key, mouse, tab switch, child-emitted msg)
// without scattering write calls through the dispatcher body.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	updated, cmd := m.updateInner(msg)
	if mm, ok := updated.(Model); ok {
		mm.persistActiveSubIfChanged()
		return mm, cmd
	}
	return updated, cmd
}

func (m Model) updateInner(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Handle paste before cursor update (which can swallow PasteMsg).
	// Route to whichever overlay or child is active.
	if paste, ok := msg.(tea.PasteMsg); ok {
		text := paste.String()
		switch {
		case m.cmdPalette.active:
			m.cmdPalette.query += text
			m.cmdPalette.refilter()
			return m, nil
		case m.tabPicker.active:
			m.tabPicker.query += text
			m.tabPicker.refilter()
			return m, nil
		case m.tenantPicker.active:
			m.tenantPicker.query += text
			m.tenantPicker.refilter()
			return m, nil
		case m.themeOverlay.Active:
			m.themeOverlay.PasteText(text, m.schemes)
			return m, nil
		case m.helpOverlay.Active:
			m.helpOverlay.PasteText(text)
			return m, nil
		default:
			if len(m.tabs) > 0 {
				return m, m.forwardToActive(msg)
			}
			return m, nil
		}
	}

	// Route all messages to the cursor so both initialBlinkMsg and BlinkMsg work.
	if cursorModel, cursorCmd := m.cursor.Update(msg); cursorCmd != nil {
		m.cursor = cursorModel
		// Also forward to active tab so its cursor blinks.
		var tabCmd tea.Cmd
		if len(m.tabs) > 0 {
			tabCmd = m.forwardToActive(msg)
		}
		return m, tea.Batch(cursorCmd, tabCmd)
	}

	// Handle mouse events: tab bar clicks are consumed here; everything
	// else is forwarded to the active child.
	switch msg := msg.(type) {
	case tea.MouseClickMsg:
		if msg.Y == 0 {
			if idx := tabIndexAtX(m.tabs, m.activeIdx, m.styles.TabBar, msg.X); idx >= 0 && idx != m.activeIdx {
				m.activeIdx = idx
				return m, m.resizeAndTickActive()
			}
			return m, nil
		}
		return m, m.forwardToActive(msg)

	case tea.MouseMotionMsg:
		return m, m.forwardToActive(msg)

	case tea.MouseReleaseMsg:
		return m, m.forwardToActive(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Forward resized dimensions to active tab (subtract tab bar height).
		return m, m.forwardToActive(tea.WindowSizeMsg{
			Width:  msg.Width,
			Height: m.childHeight(),
		})

	case tabMsg:
		// Route to the specific tab.
		idx := m.tabIndexByID(msg.tabID)
		if idx < 0 {
			return m, nil
		}
		updated, cmd := m.tabs[idx].Model.Update(msg.inner)
		m.tabs[idx].Model = updated
		// The child may have published a notification via m.Notify()
		// during this update — start the toast ticker if so and we
		// aren't already ticking.
		toastCmd := m.maybeStartToastTick()
		return m, tea.Batch(wrapCmd(msg.tabID, cmd), toastCmd)

	case closeTabMsg:
		if len(m.tabs) <= 1 {
			return m, tea.Quit
		}
		m.closeTabByID(msg.tabID)
		// Send resize to newly active tab.
		return m, m.forwardToActive(tea.WindowSizeMsg{
			Width:  m.width,
			Height: m.childHeight(),
		})

	case tabPickerMsg:
		m.tabPicker.active = false
		oldIdx := m.activeIdx
		m.addTab(msg.kind, "")
		// Record both the previous tab's position (so ctrl+o returns
		// the user there) and the new tab's home (so ctrl+i can come
		// back forward).
		m.recordTabChange(oldIdx, m.activeIdx)
		// Init the new tab and send it a resize.
		tab := &m.tabs[m.activeIdx]
		initCmd := wrapCmd(tab.ID, tab.Model.Init())
		resizeCmd := m.forwardToActive(tea.WindowSizeMsg{
			Width:  m.width,
			Height: m.childHeight(),
		})
		return m, tea.Batch(initCmd, resizeCmd)

	case jumplist.RecordJumpMsg:
		// Snapshot is owned by the active tab — record it against
		// that tab's ID. Cross-tab open paths record their own jumps
		// after creating the new tab so the destination's tabID is
		// captured correctly.
		if len(m.tabs) > 0 {
			m.recordJump(m.tabs[m.activeIdx].ID, msg.Snap)
		}
		return m, nil

	case dashapp.OpenSBNamespaceMsg:
		return m.openSBTabWithNav(msg.Subscription, sbapp.PendingNav{Namespace: msg.Namespace})

	case dashapp.OpenSBEntityMsg:
		return m.openSBTabWithNav(msg.Subscription, sbapp.PendingNav{
			Namespace:  msg.Namespace,
			EntityName: msg.EntityName,
			SubName:    msg.SubName,
			DeadLetter: msg.DeadLetter,
		})

	case dashapp.OpenBlobAccountMsg:
		return m.openBlobTabWithNav(msg.Subscription, blobapp.PendingNav{
			AccountName: msg.AccountName,
		})

	case dashapp.OpenBlobContainerMsg:
		return m.openBlobTabWithNav(msg.Subscription, blobapp.PendingNav{
			AccountName:   msg.AccountName,
			ContainerName: msg.ContainerName,
		})

	case nextTabMsg:
		// Plain tab cycling (H/L) isn't a vim "jump" — it's :bnext.
		// Don't record so the jump list stays focused on real
		// drill-ins and tab opens.
		if len(m.tabs) > 1 {
			m.activeIdx = (m.activeIdx + 1) % len(m.tabs)
			return m, m.resizeAndTickActive()
		}
		return m, nil

	case prevTabMsg:
		if len(m.tabs) > 1 {
			m.activeIdx = (m.activeIdx - 1 + len(m.tabs)) % len(m.tabs)
			return m, m.resizeAndTickActive()
		}
		return m, nil

	case jumpTabMsg:
		if msg.index >= 0 && msg.index < len(m.tabs) {
			m.activeIdx = msg.index
			return m, m.resizeAndTickActive()
		}
		return m, nil

	case openThemePickerMsg:
		m.themeOverlay.Open()
		return m, nil

	case toggleHelpMsg:
		if m.helpOverlay.Active {
			m.helpOverlay.Close()
		} else {
			m.helpOverlay.Open("Azure TUI Help", m.activeHelpSections())
		}
		return m, nil

	case toggleNotificationsMsg:
		if m.notificationsOverlay.Active {
			m.notificationsOverlay.Close()
		} else {
			m.notificationsOverlay.Open()
		}
		return m, nil

	case toggleStreamsMsg:
		if m.streamOverlay.Active {
			m.streamOverlay.Close()
		} else {
			m.streamOverlay.Open()
		}
		return m, nil

	case openAzLoginMsg:
		updated, cmd := m.handleOpenAzLogin()
		return updated, cmd

	case tenantsLoadedMsg:
		updated, cmd := m.handleTenantsLoaded(msg)
		toastCmd := updated.maybeStartToastTick()
		return updated, tea.Batch(cmd, toastCmd)

	case tenantCredentialMsg:
		updated, cmd := m.handleTenantCredential(msg)
		toastCmd := updated.maybeStartToastTick()
		return updated, tea.Batch(cmd, toastCmd)

	case postLoginSubsMsg:
		updated, cmd := m.handlePostLoginSubs(msg)
		toastCmd := updated.maybeStartToastTick()
		return updated, tea.Batch(cmd, toastCmd)

	case azLoginFinishedMsg:
		updated, cmd := m.handleAzLoginFinished(msg)
		toastCmd := updated.maybeStartToastTick()
		return updated, tea.Batch(cmd, toastCmd)

	case toastTickMsg:
		// Self-extinguishing tick: re-render to drop expired toasts,
		// reschedule only if any are still active.
		if !m.notifier.HasActive(time.Now()) {
			m.toastTickActive = false
			return m, nil
		}
		return m, tea.Tick(toastTickInterval, func(time.Time) tea.Msg {
			return toastTickMsg{}
		})

	case spinner.TickMsg:
		// Forward spinner ticks to active tab only.
		if len(m.tabs) == 0 {
			return m, nil
		}
		tab := &m.tabs[m.activeIdx]
		updated, cmd := tab.Model.Update(msg)
		tab.Model = updated
		return m, wrapCmd(tab.ID, cmd)

	case tea.KeyMsg:
		key := msg.String()

		// Command palette overlay.
		if m.cmdPalette.active {
			return m.handleCommandPalette(key)
		}

		// Tab picker overlay.
		if m.tabPicker.active {
			if kind, ok := m.tabPicker.handleKey(key, ui.ThemeKeyBindings{
				Up: m.keymap.ThemeUp, Down: m.keymap.ThemeDown,
				Apply:  m.keymap.ThemeApply,
				Cancel: m.keymap.Cancel,
				Erase:  m.keymap.BackspaceUp,
			}); ok {
				return m.Update(tabPickerMsg{kind: kind})
			}
			return m, nil
		}

		// Tenant picker overlay.
		if m.tenantPicker.active {
			if tenant, ok := m.tenantPicker.handleKey(key, ui.ThemeKeyBindings{
				Up: m.keymap.ThemeUp, Down: m.keymap.ThemeDown,
				Apply:  m.keymap.ThemeApply,
				Cancel: m.keymap.Cancel,
				Erase:  m.keymap.BackspaceUp,
			}); ok {
				updated, cmd := m.handleTenantSelected(tenant)
				toastCmd := updated.maybeStartToastTick()
				return updated, tea.Batch(cmd, toastCmd)
			}
			return m, nil
		}

		// Help overlay.
		if m.helpOverlay.Active {
			m.helpOverlay.HandleKey(key, ui.HelpKeyBindings{
				Up: m.keymap.ThemeUp, Down: m.keymap.ThemeDown,
				Close:  m.keymap.ToggleHelp,
				Cancel: m.keymap.Cancel,
				Erase:  m.keymap.BackspaceUp,
			})
			return m, nil
		}

		// Stream management overlay.
		if m.streamOverlay.Active {
			streams := m.collectStreams()
			cancelIdx, didCancel := m.streamOverlay.HandleKey(key, ui.StreamKeyBindings{
				Up: m.keymap.ThemeUp, Down: m.keymap.ThemeDown,
				Close:        m.keymap.ToggleStreams,
				Cancel:       m.keymap.Cancel,
				CancelStream: cancelStreamBinding{},
			}, len(streams))
			if didCancel && cancelIdx >= 0 && cancelIdx < len(streams) {
				entry := streams[cancelIdx]
				if entry.Status == "active" {
					m.cancelStream(entry)
				}
			}
			return m, nil
		}

		// Notifications overlay.
		if m.notificationsOverlay.Active {
			m.notificationsOverlay.HandleKey(key, ui.HelpKeyBindings{
				Up: m.keymap.ThemeUp, Down: m.keymap.ThemeDown,
				Close:  m.keymap.ToggleNotifications,
				Cancel: m.keymap.Cancel,
			}, m.notifier.Len())
			return m, nil
		}

		// Theme overlay.
		if m.themeOverlay.Active {
			if m.themeOverlay.HandleKey(key, ui.ThemeKeyBindings{
				Up: m.keymap.ThemeUp, Down: m.keymap.ThemeDown,
				Apply:  m.keymap.ThemeApply,
				Cancel: m.keymap.Cancel,
				Erase:  m.keymap.BackspaceUp,
			}, m.schemes) {
				m.applySchemeToAll(m.schemes[m.themeOverlay.ActiveThemeIdx])
				ui.SaveThemeName(m.schemes[m.themeOverlay.ActiveThemeIdx].Name)
			}
			return m, nil
		}

		// If the active child is accepting text input (list filter,
		// search bar, etc.), skip all single-key shortcuts and forward
		// the key directly so the user can type freely. Only ctrl+c
		// is still allowed to quit.
		if m.activeChildTextInput() {
			if key == "ctrl+c" {
				return m, tea.Quit
			}
			return m, m.forwardToActive(msg)
		}

		// Global tab keys.
		switch {
		case m.keymap.Quit.Matches(key):
			return m, tea.Quit
		case m.keymap.CommandPalette.Matches(key):
			m.cmdPalette.open(m.buildCommands())
			return m, nil
		case m.keymap.NewTab.Matches(key):
			m.tabPicker.open()
			return m, nil
		case m.keymap.CloseTab.Matches(key):
			if len(m.tabs) <= 1 {
				return m, tea.Quit
			}
			m.closeTab(m.activeIdx)
			return m, m.forwardToActive(tea.WindowSizeMsg{
				Width:  m.width,
				Height: m.childHeight(),
			})
		case m.keymap.NextTab.Matches(key):
			// Plain tab cycle — not a "jump" in the vim sense.
			if len(m.tabs) > 1 {
				m.activeIdx = (m.activeIdx + 1) % len(m.tabs)
				return m, m.resizeAndTickActive()
			}
			return m, nil
		case m.keymap.PrevTab.Matches(key):
			if len(m.tabs) > 1 {
				m.activeIdx = (m.activeIdx - 1 + len(m.tabs)) % len(m.tabs)
				return m, m.resizeAndTickActive()
			}
			return m, nil
		case m.keymap.ToggleThemePicker.Matches(key):
			m.themeOverlay.Open()
			return m, nil
		case m.keymap.ToggleHelp.Matches(key):
			m.helpOverlay.Open("Azure TUI Help", m.activeHelpSections())
			return m, nil
		case m.keymap.ToggleNotifications.Matches(key):
			return m.Update(toggleNotificationsMsg{})
		case m.keymap.ToggleStreams.Matches(key):
			return m.Update(toggleStreamsMsg{})
		case m.keymap.JumpBack.Matches(key):
			return m, m.jumpBack()
		case m.keymap.JumpForward.Matches(key):
			return m, m.jumpForward()
		}

		if idx, ok := m.keymap.JumpIndex(key); ok {
			if idx < len(m.tabs) {
				m.activeIdx = idx
				return m, m.resizeAndTickActive()
			}
			return m, nil
		}

		// Forward to active tab.
		return m, m.forwardToActive(msg)
	}

	// Any other message — forward to active tab.
	return m, m.forwardToActive(msg)
}

// maybeStartToastTick starts the self-extinguishing toast ticker if a
// notification just appeared and we aren't already ticking. Returns nil
// if no action is needed. The ticker re-renders the view every
// toastTickInterval until no toasts are active, at which point the
// toastTickMsg handler clears the flag and stops scheduling more.
func (m *Model) maybeStartToastTick() tea.Cmd {
	if m.toastTickActive {
		return nil
	}
	if !m.notifier.HasActive(time.Now()) {
		return nil
	}
	m.toastTickActive = true
	return tea.Tick(toastTickInterval, func(time.Time) tea.Msg {
		return toastTickMsg{}
	})
}

func (m *Model) forwardToActive(msg tea.Msg) tea.Cmd {
	if len(m.tabs) == 0 {
		return nil
	}
	tab := &m.tabs[m.activeIdx]
	updated, cmd := tab.Model.Update(msg)
	tab.Model = updated
	return wrapCmd(tab.ID, cmd)
}

func (m Model) childHeight() int {
	h := m.height - 1 // tab bar takes 1 line
	if h < 1 {
		h = 1
	}
	return h
}

func (m *Model) resizeAndTickActive() tea.Cmd {
	resizeCmd := m.forwardToActive(tea.WindowSizeMsg{
		Width:  m.width,
		Height: m.childHeight(),
	})
	// Send a spinner tick so the active tab picks up spinner animation.
	tickCmd := m.forwardToActive(spinner.TickMsg{})
	return tea.Batch(resizeCmd, tickCmd)
}

func (m *Model) buildCommands() []command {
	cmds := []command{
		{name: "New Tab: Blob Storage", hint: "ctrl+t", action: func() commandAction {
			return commandAction{msg: tabPickerMsg{kind: TabBlob}}
		}},
		{name: "New Tab: Service Bus", hint: "ctrl+t", action: func() commandAction {
			return commandAction{msg: tabPickerMsg{kind: TabServiceBus}}
		}},
		{name: "New Tab: Key Vault", hint: "ctrl+t", action: func() commandAction {
			return commandAction{msg: tabPickerMsg{kind: TabKeyVault}}
		}},
		{name: "New Tab: Dashboard", hint: "ctrl+t", action: func() commandAction {
			return commandAction{msg: tabPickerMsg{kind: TabDashboard}}
		}},
		{name: "Close Tab", hint: "ctrl+w", action: func() commandAction {
			if len(m.tabs) <= 1 {
				return commandAction{quit: true}
			}
			return commandAction{msg: closeTabMsg{tabID: m.tabs[m.activeIdx].ID}}
		}},
		{name: "Next Tab", hint: "L", action: func() commandAction {
			return commandAction{msg: nextTabMsg{}}
		}},
		{name: "Previous Tab", hint: "H", action: func() commandAction {
			return commandAction{msg: prevTabMsg{}}
		}},
	}

	// Jump to specific open tabs.
	for i, t := range m.tabs {
		idx := i // capture
		tab := t
		label := fmt.Sprintf("Go to Tab %d: %s", idx+1, tab.Kind.String())
		hint := fmt.Sprintf("alt+%d", idx+1)
		cmds = append(cmds, command{name: label, hint: hint, action: func() commandAction {
			return commandAction{msg: jumpTabMsg{index: idx}}
		}})
	}

	cmds = append(cmds,
		command{name: "Azure Login / Switch Tenant", action: func() commandAction {
			return commandAction{msg: openAzLoginMsg{}}
		}},
		command{name: "Theme Picker", hint: "T", action: func() commandAction {
			return commandAction{msg: openThemePickerMsg{}}
		}},
		command{name: "Notifications History", hint: "N", action: func() commandAction {
			return commandAction{msg: toggleNotificationsMsg{}}
		}},
		command{name: "Stream Manager", hint: "F", action: func() commandAction {
			return commandAction{msg: toggleStreamsMsg{}}
		}},
		command{name: "Help", hint: "?", action: func() commandAction {
			return commandAction{msg: toggleHelpMsg{}}
		}},
		command{name: "Quit", hint: "ctrl+c", action: func() commandAction {
			return commandAction{quit: true}
		}},
	)

	return cmds
}

func (m Model) handleCommandPalette(key string) (tea.Model, tea.Cmd) {
	cmd, executed, _ := m.cmdPalette.handleKey(key, m.keymap)
	if executed {
		action := cmd.action()
		if action.quit {
			return m, tea.Quit
		}
		if action.msg != nil {
			// Re-inject the message through Update.
			return m.Update(action.msg)
		}
		if action.cmd != nil {
			return m, action.cmd
		}
	}
	return m, nil
}

