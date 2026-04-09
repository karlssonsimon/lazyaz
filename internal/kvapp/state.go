package kvapp

import (
	"fmt"

	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/azure"
	"github.com/karlssonsimon/lazyaz/internal/azure/keyvault"
	"github.com/karlssonsimon/lazyaz/internal/cache"
	"github.com/karlssonsimon/lazyaz/internal/keymap"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	"charm.land/bubbles/v2/cursor"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
)

const (
	vaultsPane = iota
	secretsPane
	versionsPane
)

type Model struct {
	appshell.Model

	service *keyvault.Service

	vaultsList   list.Model
	secretsList  list.Model
	versionsList list.Model

	focus int

	vaults        []keyvault.Vault
	secrets       []keyvault.Secret
	versions      []keyvault.SecretVersion
	markedSecrets map[string]keyvault.Secret
	visualLineMode bool
	visualAnchor   string

	// Per-scope list state history. When the user navigates between
	// scopes (different subscription, different vault, etc.) the cursor
	// and filter of the previous scope are snapshotted here so that
	// returning to that scope restores the view where it was left.
	// Keyed by the same scope identifiers used for the cache.
	vaultsHistory   map[string]ui.ListState // keyed by subscription ID
	secretsHistory  map[string]ui.ListState // keyed by sub+vault
	versionsHistory map[string]ui.ListState // keyed by sub+vault+secret

	hasVault      bool
	currentVault  keyvault.Vault
	hasSecret     bool
	currentSecret keyvault.Secret

	actionMenu actionMenuState
	cache      kvCache

	loadingSpinnerID int

	// Per-pane inspect strip toggle. When inspectPanes[pane] is true, the
	// pane renders an inline detail strip (via ui.RenderInspectStrip) under
	// its list. The strip updates live as the cursor moves so the user can
	// keep browsing while details remain visible. Toggled with K.
	inspectPanes map[int]bool

	paneWidths [3]int // vlt, sec, ver — set by resize
	paneHeight int
}

type vaultsLoadedMsg struct {
	subscriptionID string
	vaults         []keyvault.Vault
	done           bool
	err            error
	next           tea.Cmd
}

type secretsLoadedMsg struct {
	vault   keyvault.Vault
	secrets []keyvault.Secret
	done    bool
	err     error
	next    tea.Cmd
}

type versionsLoadedMsg struct {
	vault      keyvault.Vault
	secretName string
	versions   []keyvault.SecretVersion
	done       bool
	err        error
	next       tea.Cmd
}

type secretValueYankedMsg struct {
	secretName string
	version    string
	err        error
}

func NewModel(svc *keyvault.Service, cfg ui.Config, db *cache.DB) Model {
	return NewModelWithKeyMap(svc, cfg, keymap.Default(), db)
}

func NewModelWithKeyMap(svc *keyvault.Service, cfg ui.Config, km keymap.Keymap, db *cache.DB) Model {
	delegate := list.NewDefaultDelegate()

	vaults := list.New([]list.Item{}, delegate, 24, 10)
	vaults.SetShowTitle(false) // title is rendered by ui.RenderListPane
	vaults.SetShowHelp(false)
	vaults.SetShowPagination(false)
	vaults.SetShowStatusBar(true)
	vaults.SetStatusBarItemName("vault", "vaults")
	vaults.SetFilteringEnabled(true)
	vaults.DisableQuitKeybindings()

	secrets := list.New([]list.Item{}, delegate, 24, 10)
	secrets.SetShowTitle(false)
	secrets.SetShowHelp(false)
	secrets.SetShowPagination(false)
	secrets.SetShowStatusBar(true)
	secrets.SetStatusBarItemName("secret", "secrets")
	secrets.SetFilteringEnabled(true)
	secrets.DisableQuitKeybindings()

	versionsList := list.New([]list.Item{}, delegate, 40, 10)
	versionsList.SetShowTitle(false)
	versionsList.SetShowHelp(false)
	versionsList.SetShowPagination(false)
	versionsList.SetShowStatusBar(true)
	versionsList.SetStatusBarItemName("version", "versions")
	versionsList.SetFilteringEnabled(true)
	versionsList.DisableQuitKeybindings()

	m := Model{
		Model:           appshell.New(cfg, km),
		service:         svc,
		vaultsList:      vaults,
		secretsList:     secrets,
		versionsList:    versionsList,
		markedSecrets:   make(map[string]keyvault.Secret),
		focus:           vaultsPane,
		cache:           newCache(db),
		vaultsHistory:   make(map[string]ui.ListState),
		secretsHistory:  make(map[string]ui.ListState),
		versionsHistory: make(map[string]ui.ListState),
		inspectPanes:    make(map[int]bool),
	}
	m.applyScheme(cfg.ActiveScheme())
	// Hydrate subscriptions from cache without hitting Azure.
	m.HydrateSubscriptionsFromCache(m.cache.subscriptions)
	if !m.HasSubscription {
		m.SubOverlay.Open()
		m.SetLoading(-1)
		m.loadingSpinnerID = m.NotifySpinner("Loading Azure subscriptions...")
	}
	return m
}

// NewModelWithCache creates a Model using pre-built shared cache stores.
func NewModelWithCache(svc *keyvault.Service, cfg ui.Config, stores KVStores, km keymap.Keymap) Model {
	m := NewModelWithKeyMap(svc, cfg, km, nil)
	m.cache = NewCacheWithStores(stores)
	// Re-hydrate subscriptions from the shared store.
	m.HydrateSubscriptionsFromCache(m.cache.subscriptions)
	return m
}

func (m *Model) applyScheme(scheme ui.Scheme) {
	m.SetScheme(scheme)
	m.Styles.ApplyToLists([]*list.Model{
		&m.vaultsList, &m.secretsList, &m.versionsList,
	}, &m.Spinner)
	m.secretsList.SetDelegate(newSecretDelegate(m.Styles.Delegate, m.Styles))
}

// ApplyScheme applies the given scheme to all lists and spinner.
func (m *Model) ApplyScheme(scheme ui.Scheme) {
	m.applyScheme(scheme)
}

// HelpSections returns the help sections for the key vault explorer.
func (m Model) HelpSections() []ui.HelpSection {
	km := m.Keymap
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
			Title: "Secrets",
			Items: []string{
				keymap.HelpEntry(km.ActionMenu, "action menu"),
				keymap.HelpEntry(km.YankSecret, "yank secret value to clipboard"),
				keymap.HelpEntry(km.ToggleMark, "toggle mark on current secret"),
				keymap.HelpEntry(km.ToggleVisualLine, "start/end visual-line selection"),
				keymap.HelpEntry(km.ExitVisualLine, "exit visual mode / clear marks"),
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

// SetSubscription overrides the embedded appshell.Model method to also
// hydrate vaults from cache. Tabapp calls this after constructing the
// model and before Init() issues the first fetch, so the user sees
// cached vaults instantly while the network call runs in the background.
func (m *Model) SetSubscription(sub azure.Subscription) {
	m.Model.SetSubscription(sub)
	if cached, ok := m.cache.vaults.Get(sub.ID); ok {
		m.vaults = cached
		m.vaultsList.Title = fmt.Sprintf("Vaults (%d)", len(cached))
		ui.SetItemsPreserveKey(&m.vaultsList, vaultsToItems(cached), vaultItemKey)
	}
}

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.Spinner.Tick, cursor.Blink}
	if m.SubOverlay.Active {
		cmds = append(cmds, fetchSubscriptionsCmd(m.service, m.cache.subscriptions, m.Subscriptions))
	}
	if m.HasSubscription {
		cmds = append(cmds, fetchVaultsCmd(m.service, m.cache.vaults, m.CurrentSub.ID, m.vaults))
	}
	return tea.Batch(cmds...)
}
