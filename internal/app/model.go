package app

import (
	"azure-storage/internal/azure/blob"
	"azure-storage/internal/azure/keyvault"
	"azure-storage/internal/azure/servicebus"
	"azure-storage/internal/blobapp"
	"azure-storage/internal/kvapp"
	"azure-storage/internal/sbapp"
	"azure-storage/internal/ui"

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
	keymap tabKeyMap

	palette      ui.Palette
	themes       []ui.Theme
	themeOverlay ui.ThemeOverlayState
	helpOverlay  ui.HelpOverlayState

	tabPicker bool // true when the new-tab picker overlay is showing

	width  int
	height int
}

// NewModel creates the parent tabbed model.
func NewModel(blobSvc *blob.Service, sbSvc *servicebus.Service, kvSvc *keyvault.Service, cfg ui.Config) Model {
	m := Model{
		blobSvc: blobSvc,
		sbSvc:   sbSvc,
		kvSvc:   kvSvc,
		stores:  newSharedStores(),
		cfg:     cfg,
		keymap:  defaultTabKeyMap(),
		themes:  cfg.Themes,
		themeOverlay: ui.ThemeOverlayState{
			ActiveThemeIdx: ui.ActiveThemeIndex(cfg),
		},
	}
	theme := cfg.ActiveTheme()
	m.palette = theme.Colors

	// Open a default blob tab.
	m.addTab(TabBlob)
	return m
}

func (m *Model) addTab(kind TabKind) {
	id := m.nextID
	m.nextID++

	var child tea.Model
	switch kind {
	case TabBlob:
		bm := blobapp.NewModelWithCache(m.blobSvc, m.cfg, blobapp.BlobStores{
			Subscriptions: m.stores.subscriptions,
			Accounts:      m.stores.blobAccounts,
			Containers:    m.stores.blobContainers,
			Blobs:         m.stores.blobs,
		})
		bm.EmbeddedMode = true
		child = bm
	case TabServiceBus:
		sm := sbapp.NewModelWithCache(m.sbSvc, m.cfg, sbapp.SBStores{
			Subscriptions: m.stores.subscriptions,
			Namespaces:    m.stores.sbNamespaces,
			Entities:      m.stores.sbEntities,
			TopicSubs:     m.stores.sbTopicSubs,
		})
		sm.EmbeddedMode = true
		child = sm
	case TabKeyVault:
		km := kvapp.NewModelWithCache(m.kvSvc, m.cfg, kvapp.KVStores{
			Subscriptions: m.stores.subscriptions,
			Vaults:        m.stores.kvVaults,
			Secrets:       m.stores.kvSecrets,
			Versions:      m.stores.kvVersions,
		})
		km.EmbeddedMode = true
		child = km
	}

	m.tabs = append(m.tabs, Tab{ID: id, Kind: kind, Model: child})
	m.activeIdx = len(m.tabs) - 1
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

func (m *Model) applyThemeToAll(theme ui.Theme) {
	m.palette = theme.Colors
	for i := range m.tabs {
		switch child := m.tabs[i].Model.(type) {
		case blobapp.Model:
			child.ApplyTheme(theme)
			m.tabs[i].Model = child
		case sbapp.Model:
			child.ApplyTheme(theme)
			m.tabs[i].Model = child
		case kvapp.Model:
			child.ApplyTheme(theme)
			m.tabs[i].Model = child
		}
	}
}

func (m Model) Init() tea.Cmd {
	if len(m.tabs) == 0 {
		return nil
	}
	// Init the first tab and wrap its commands.
	cmd := m.tabs[0].Model.Init()
	return wrapCmd(m.tabs[0].ID, cmd)
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
		m.tabPicker = false
		m.addTab(msg.kind)
		// Init the new tab and send it a resize.
		tab := &m.tabs[m.activeIdx]
		initCmd := wrapCmd(tab.ID, tab.Model.Init())
		resizeCmd := m.forwardToActive(tea.WindowSizeMsg{
			Width:  m.width,
			Height: m.childHeight(),
		})
		return m, tea.Batch(initCmd, resizeCmd)

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

		// Tab picker overlay.
		if m.tabPicker {
			return m.handleTabPicker(key)
		}

		// Help overlay.
		if m.helpOverlay.Active {
			switch {
			case m.keymap.ToggleHelp.Matches(key), key == "esc":
				m.helpOverlay.Close()
				return m, nil
			default:
				return m, nil
			}
		}

		// Theme overlay.
		if m.themeOverlay.Active {
			if m.themeOverlay.HandleKey(key, ui.ThemeKeyBindings{
				Up: m.keymap.ThemeUp, Down: m.keymap.ThemeDown,
				Apply: m.keymap.ThemeApply, Cancel: m.keymap.ThemeCancel,
			}, m.themes) {
				m.applyThemeToAll(m.themes[m.themeOverlay.ActiveThemeIdx])
				ui.SaveThemeName(m.cfg.AppName, m.themes[m.themeOverlay.ActiveThemeIdx].Name)
			}
			return m, nil
		}

		// Global tab keys.
		switch {
		case m.keymap.Quit.Matches(key):
			return m, tea.Quit
		case m.keymap.NewTab.Matches(key):
			m.tabPicker = true
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
		case m.keymap.ThemePick.Matches(key):
			m.themeOverlay.Open()
			return m, nil
		case m.keymap.ToggleHelp.Matches(key):
			m.helpOverlay.Toggle()
			return m, nil
		}

		if idx, ok := m.keymap.jumpIndex(key); ok {
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

func (m Model) handleTabPicker(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "1", "b", "B":
		return m, func() tea.Msg { return tabPickerMsg{kind: TabBlob} }
	case "2", "s", "S":
		return m, func() tea.Msg { return tabPickerMsg{kind: TabServiceBus} }
	case "3", "k", "K":
		return m, func() tea.Msg { return tabPickerMsg{kind: TabKeyVault} }
	case "esc", "ctrl+c":
		m.tabPicker = false
		return m, nil
	}
	return m, nil
}
