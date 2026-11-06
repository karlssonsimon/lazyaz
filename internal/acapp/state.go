// Package acapp is the Azure App Configuration explorer: stores →
// key-values → value. Read-only. It mirrors kvapp's shell and borrows
// the value pane from sbapp's message preview.
package acapp

import (
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/azure"
	"github.com/karlssonsimon/lazyaz/internal/azure/appconfig"
	"github.com/karlssonsimon/lazyaz/internal/cache"
	"github.com/karlssonsimon/lazyaz/internal/keymap"
	"github.com/karlssonsimon/lazyaz/internal/ui"
	"github.com/karlssonsimon/lazyaz/internal/vim"

	"charm.land/bubbles/v2/cursor"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
)

// Miller column panes, left → right. The value pane is not a list: it
// is a viewport showing the opened key-value, like sbapp's message
// preview.
const (
	storesPane = iota
	settingsPane
	valuePane
)

// InputMode is the computed interaction mode used for key dispatch.
type InputMode int

const (
	ModeNormal      InputMode = iota // Browsing
	ModeOverlay                      // Sub/Theme/Help overlay open
	ModeListFilter                   // Typing a list filter
	ModeCopyPalette                  // Copy palette overlay open
)

func (m Model) inputMode() InputMode {
	switch {
	case m.copyOverlay.Active:
		return ModeCopyPalette
	case m.SubOverlay.Active, m.ThemeOverlay.Active, m.HelpOverlay.Active:
		return ModeOverlay
	case m.focusedListSettingFilter():
		return ModeListFilter
	default:
		return ModeNormal
	}
}

func (mode InputMode) String() string {
	if mode == ModeListFilter {
		return "FILTER"
	}
	return "NORMAL"
}

type Model struct {
	appshell.Model

	service *appconfig.Service

	storesList    ui.List
	settingsList  ui.List
	valueViewport viewport.Model

	focus int

	// vimr resolves multi-key vim chords (gg, z...) and counts.
	vimr vim.Resolver

	stores   []appconfig.Store
	settings []appconfig.Setting

	// Per-scope list state so returning to a scope restores cursor and
	// filter. Keyed like the cache.
	storesHistory   map[string]ui.ListState // by subscription ID
	settingsHistory map[string]ui.ListState // by sub+store

	hasStore     bool
	currentStore appconfig.Store

	// viewingValue means the value pane shows currentSetting. The pane
	// follows the settings cursor while open.
	viewingValue   bool
	currentSetting appconfig.Setting

	pendingNav  PendingNav
	applyingNav bool

	copyOverlay  ui.CopyOverlay
	inspectPanes map[int]bool
	cache        acCache

	clickTracker ui.ClickTracker
	paneWidths   [3]int
	paneHeight   int

	// CredResolver, when set, supplies a credential for the active
	// subscription's tenant. The parent injects it.
	CredResolver interface {
		CredentialFor(tenantID string) (azcore.TokenCredential, error)
	}
}

type storesLoadedMsg struct {
	subscriptionID string
	stores         []appconfig.Store
	done           bool
	err            error
	next           tea.Cmd
}

type settingsLoadedMsg struct {
	store    appconfig.Store
	settings []appconfig.Setting
	done     bool
	err      error
	next     tea.Cmd
}

type clipboardMsg struct {
	text string
	err  error
}

// OpenKeyVaultReferenceMsg asks the parent to open the referenced
// secret in a Key Vault tab. Emitted when the user opens a Key Vault
// reference from the value pane.
type OpenKeyVaultReferenceMsg struct {
	Subscription azure.Subscription
	VaultName    string
	SecretName   string
}

func NewModel(svc *appconfig.Service, cfg ui.Config, db *cache.DB) Model {
	return NewModelWithKeyMap(svc, cfg, keymap.Default(), db)
}

func NewModelWithKeyMap(svc *appconfig.Service, cfg ui.Config, km keymap.Keymap, db *cache.DB) Model {
	if svc == nil {
		svc = appconfig.NewService(nil)
	}
	delegate := list.NewDefaultDelegate()

	newList := func(singular, plural string) ui.List {
		l := ui.NewList([]list.Item{}, delegate, 24, 10)
		l.SetShowTitle(false)
		l.SetShowFilter(false) // filter UI lives in the column footer
		l.SetShowHelp(false)
		l.SetShowPagination(false)
		l.SetShowStatusBar(false)
		l.SetStatusBarItemName(singular, plural)
		l.SetFilteringEnabled(true)
		l.DisableQuitKeybindings()
		l.KeyMap.CursorUp = km.CursorUp.AsBubbleKey()
		l.KeyMap.CursorDown = km.CursorDown.AsBubbleKey()
		l.KeyMap.Filter = km.FilterInput.AsBubbleKey()
		l.Filter = ui.ListFilter
		l.SetScrolloff(cfg.ScrolloffValue())
		return l
	}

	m := Model{
		Model:           appshell.New(cfg, km),
		service:         svc,
		storesList:      newList("store", "stores"),
		settingsList:    newList("setting", "settings"),
		focus:           storesPane,
		cache:           newCache(db),
		storesHistory:   make(map[string]ui.ListState),
		settingsHistory: make(map[string]ui.ListState),
		inspectPanes:    make(map[int]bool),
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
func NewModelWithCache(svc *appconfig.Service, cfg ui.Config, stores ACStores, km keymap.Keymap) Model {
	m := NewModelWithKeyMap(svc, cfg, km, nil)
	m.cache = NewCacheWithStores(stores)
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
	m.Styles.ApplyToLists([]*ui.List{&m.storesList, &m.settingsList}, &m.Spinner)
	m.settingsList.SetDelegate(newSettingDelegate(m.Styles.Delegate, m.Styles))
	if m.viewingValue {
		m.setValueContent()
	}
}

// ApplyScheme applies the given scheme to all lists and the spinner.
func (m *Model) ApplyScheme(scheme ui.Scheme) {
	m.applyScheme(scheme)
}

func (m Model) WithScheme(scheme ui.Scheme) tea.Model {
	m.ApplyScheme(scheme)
	return m
}

// HelpSections returns the help sections for the explorer.
func (m Model) HelpSections() []ui.HelpSection {
	km := m.Keymap
	return []ui.HelpSection{
		{
			Title: "Navigation",
			Items: []string{
				keymap.HelpEntry(km.NextFocus, "focus next column"),
				keymap.HelpEntry(km.PreviousFocus, "focus previous column"),
				keymap.HelpEntry(km.FilterInput, "filter focused column"),
				keymap.HelpEntry(km.OpenFocused, "open selected row / value"),
				keymap.HelpEntry(km.BackspaceUp, "go up/back"),
				keymap.HelpEntry(keymap.New(km.HalfPageDown.Label()+"/"+km.HalfPageUp.Label()), "half-page scroll"),
				"  stores → settings → value  (the value pane follows the cursor)",
			},
		},
		{
			Title: "Settings",
			Items: []string{
				keymap.HelpEntry(km.YankSecret, "yank value to clipboard"),
				keymap.HelpEntry(km.CopyPalette, "copy palette (key, value, label, endpoint)"),
				keymap.HelpEntry(km.OpenFocused, "in the value pane: open a Key Vault reference in a Key Vault tab"),
				keymap.HelpEntry(km.Inspect, "inspect item (label, content type, modified, tags)"),
			},
		},
		{
			Title: "App",
			Items: []string{
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

// SetSubscription overrides the embedded appshell method to also
// hydrate stores from cache.
func (m *Model) SetSubscription(sub azure.Subscription) {
	m.Model.SetSubscription(sub)
	if m.service != nil && m.CredResolver != nil && sub.TenantID != "" {
		if cred, err := m.CredResolver.CredentialFor(sub.TenantID); err == nil {
			m.service.SetCredential(cred)
		} else {
			m.Notify(appshell.LevelError, fmt.Sprintf("Credential for tenant %s: %s", sub.TenantID, err.Error()))
		}
	}
	if cached, ok := m.cache.stores.Get(sub.ID); ok {
		m.stores = cached
		m.storesList.Title = fmt.Sprintf("Stores (%d)", len(cached))
		ui.SetItemsPreserveKey(&m.storesList, storesToItems(cached), storeItemKey)
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
		cmds = append(cmds, fetchStoresCmd(m.service, m.cache.stores, m.CurrentSub.ID, m.stores))
	}
	return tea.Batch(cmds...)
}
