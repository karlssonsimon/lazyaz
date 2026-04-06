package kvapp

import (
	"azure-storage/internal/appshell"
	"azure-storage/internal/azure/keyvault"
	"azure-storage/internal/cache"
	"azure-storage/internal/keymap"
	"azure-storage/internal/ui"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
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

	vaults   []keyvault.Vault
	secrets  []keyvault.Secret
	versions []keyvault.SecretVersion

	hasVault      bool
	currentVault  keyvault.Vault
	hasSecret     bool
	currentSecret keyvault.Secret

	cache kvCache

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
	vaults.Title = "Vaults"
	vaults.SetShowHelp(false)
	vaults.SetShowPagination(false)
	vaults.SetShowStatusBar(true)
	vaults.SetStatusBarItemName("vault", "vaults")
	vaults.SetFilteringEnabled(true)
	vaults.DisableQuitKeybindings()

	secrets := list.New([]list.Item{}, delegate, 24, 10)
	secrets.Title = "Secrets"
	secrets.SetShowHelp(false)
	secrets.SetShowPagination(false)
	secrets.SetShowStatusBar(true)
	secrets.SetStatusBarItemName("secret", "secrets")
	secrets.SetFilteringEnabled(true)
	secrets.DisableQuitKeybindings()

	versionsList := list.New([]list.Item{}, delegate, 40, 10)
	versionsList.Title = "Versions"
	versionsList.SetShowHelp(false)
	versionsList.SetShowPagination(false)
	versionsList.SetShowStatusBar(true)
	versionsList.SetStatusBarItemName("version", "versions")
	versionsList.SetFilteringEnabled(true)
	versionsList.DisableQuitKeybindings()

	m := Model{
		Model:        appshell.New(cfg, km),
		service:      svc,
		vaultsList:   vaults,
		secretsList:  secrets,
		versionsList: versionsList,
		focus:        vaultsPane,
		cache:        newCache(db),
	}
	m.applyScheme(cfg.ActiveScheme())
	// Hydrate subscriptions from cache without hitting Azure.
	m.HydrateSubscriptionsFromCache(m.cache.subscriptions)
	if !m.HasSubscription {
		m.SubOverlay.Open()
		m.SetLoading(-1)
		m.Status = "Loading Azure subscriptions..."
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
				keymap.HelpEntry(km.YankSecret, "yank secret value to clipboard"),
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

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{spinner.Tick}
	if m.SubOverlay.Active {
		cmds = append(cmds, fetchSubscriptionsCmd(m.service, m.cache.subscriptions, true))
	}
	if m.HasSubscription {
		cmds = append(cmds, fetchVaultsCmd(m.service, m.cache.vaults, m.CurrentSub.ID))
	}
	return tea.Batch(cmds...)
}
