package app

import (
	"fmt"
	"strings"

	"github.com/karlssonsimon/lazyaz/internal/azure"
	"github.com/karlssonsimon/lazyaz/internal/azure/blob"
	"github.com/karlssonsimon/lazyaz/internal/azure/keyvault"
	"github.com/karlssonsimon/lazyaz/internal/azure/servicebus"
	"github.com/karlssonsimon/lazyaz/internal/blobapp"
	"github.com/karlssonsimon/lazyaz/internal/cache"
	"github.com/karlssonsimon/lazyaz/internal/keymap"
	"github.com/karlssonsimon/lazyaz/internal/kvapp"
	"github.com/karlssonsimon/lazyaz/internal/sbapp"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

// Model is the top-level Bubble Tea model that manages tabs.
type Model struct {
	tabs      []Tab
	activeIdx int
	nextID    int

	blobSvc *blob.Service
	sbSvc   *servicebus.Service
	kvSvc   *keyvault.Service

	stores sharedStores
	cfg    ui.Config
	keymap keymap.Keymap

	styles       ui.Styles
	schemes      []ui.Scheme
	themeOverlay ui.ThemeOverlayState
	helpOverlay  ui.HelpOverlayState

	tabPicker  tabPickerState
	cmdPalette commandPalette

	width  int
	height int
}

// NewModel creates the parent tabbed model.
// If db is non-nil, a persistent SQLite cache is used; otherwise in-memory.
func NewModel(blobSvc *blob.Service, sbSvc *servicebus.Service, kvSvc *keyvault.Service, cfg ui.Config, db *cache.DB, km keymap.Keymap) Model {
	m := Model{
		blobSvc: blobSvc,
		sbSvc:   sbSvc,
		kvSvc:   kvSvc,
		stores:  newSharedStores(db),
		cfg:     cfg,
		keymap:  km,
		schemes: cfg.Schemes,
		themeOverlay: ui.ThemeOverlayState{
			ActiveThemeIdx: ui.ActiveSchemeIndex(cfg),
		},
	}
	m.styles = ui.NewStyles(cfg.ActiveScheme())

	// Resolve configured startup tabs. Invalid kinds are skipped and
	// surfaced as a warning on the first opened tab. If nothing valid
	// remains, fall back to a single blob tab so the app is usable.
	var warnings []string
	opened := 0
	for _, tc := range cfg.Tabs {
		kind, ok := TabKindFromString(tc.Kind)
		if !ok {
			warnings = append(warnings, fmt.Sprintf("unknown tab kind %q in config", tc.Kind))
			continue
		}
		m.addTab(kind, tc.Subscription)
		opened++
	}
	if opened == 0 {
		m.addTab(TabBlob, "")
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
		child.Status = status
		m.tabs[idx].Model = child
	case sbapp.Model:
		child.Status = status
		m.tabs[idx].Model = child
	case kvapp.Model:
		child.Status = status
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
			Subscriptions: m.stores.subscriptions,
			Accounts:      m.stores.blobAccounts,
			Containers:    m.stores.blobContainers,
			Blobs:         m.stores.blobs,
		}, m.keymap)
		bm.EmbeddedMode = true
		applyInitialSub(&bm)
		child = bm
	case TabServiceBus:
		sm := sbapp.NewModelWithCache(m.sbSvc, m.cfg, sbapp.SBStores{
			Subscriptions: m.stores.subscriptions,
			Namespaces:    m.stores.sbNamespaces,
			Entities:      m.stores.sbEntities,
			TopicSubs:     m.stores.sbTopicSubs,
		}, m.keymap)
		sm.EmbeddedMode = true
		applyInitialSub(&sm)
		child = sm
	case TabKeyVault:
		kvm := kvapp.NewModelWithCache(m.kvSvc, m.cfg, kvapp.KVStores{
			Subscriptions: m.stores.subscriptions,
			Vaults:        m.stores.kvVaults,
			Secrets:       m.stores.kvSecrets,
			Versions:      m.stores.kvVersions,
		}, m.keymap)
		kvm.EmbeddedMode = true
		applyInitialSub(&kvm)
		child = kvm
	}

	m.tabs = append(m.tabs, Tab{ID: id, Kind: kind, Model: child})
	m.activeIdx = len(m.tabs) - 1
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
	}
	return azure.Subscription{}, false
}

func (m *Model) closeTab(idx int) {
	if idx < 0 || idx >= len(m.tabs) {
		return
	}
	m.tabs = append(m.tabs[:idx], m.tabs[idx+1:]...)
	if m.activeIdx >= len(m.tabs) {
		m.activeIdx = len(m.tabs) - 1
	}
	if m.activeIdx < 0 {
		m.activeIdx = 0
	}
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
		}
	}
}

func (m Model) Init() tea.Cmd {
	if len(m.tabs) == 0 {
		return nil
	}
	// Init every tab so configured startup tabs all kick off their
	// initial fetches in the background — not just the active one.
	// Each tab's commands are wrapped so their results route back to
	// the correct tab via tabMsg.
	cmds := make([]tea.Cmd, 0, len(m.tabs))
	for _, tab := range m.tabs {
		if c := tab.Model.Init(); c != nil {
			cmds = append(cmds, wrapCmd(tab.ID, c))
		}
	}
	return tea.Batch(cmds...)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
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
		return m, wrapCmd(msg.tabID, cmd)

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
		m.addTab(msg.kind, "")
		// Init the new tab and send it a resize.
		tab := &m.tabs[m.activeIdx]
		initCmd := wrapCmd(tab.ID, tab.Model.Init())
		resizeCmd := m.forwardToActive(tea.WindowSizeMsg{
			Width:  m.width,
			Height: m.childHeight(),
		})
		return m, tea.Batch(initCmd, resizeCmd)

	case nextTabMsg:
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
				Apply: m.keymap.ThemeApply, Cancel: m.keymap.ThemeCancel,
			}); ok {
				return m.Update(tabPickerMsg{kind: kind})
			}
			return m, nil
		}

		// Help overlay.
		if m.helpOverlay.Active {
			m.helpOverlay.HandleKey(key, ui.HelpKeyBindings{
				Up: m.keymap.ThemeUp, Down: m.keymap.ThemeDown,
				Close: m.keymap.ToggleHelp,
			})
			return m, nil
		}

		// Theme overlay.
		if m.themeOverlay.Active {
			if m.themeOverlay.HandleKey(key, ui.ThemeKeyBindings{
				Up: m.keymap.ThemeUp, Down: m.keymap.ThemeDown,
				Apply: m.keymap.ThemeApply, Cancel: m.keymap.ThemeCancel,
			}, m.schemes) {
				m.applySchemeToAll(m.schemes[m.themeOverlay.ActiveThemeIdx])
				ui.SaveThemeName(m.schemes[m.themeOverlay.ActiveThemeIdx].Name)
			}
			return m, nil
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
	tickCmd := m.forwardToActive(spinner.Tick())
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
		command{name: "Theme Picker", hint: "T", action: func() commandAction {
			return commandAction{msg: openThemePickerMsg{}}
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
	cmd, executed, _ := m.cmdPalette.handleKey(key)
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

