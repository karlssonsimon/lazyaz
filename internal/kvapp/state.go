package kvapp

import (
	"azure-storage/internal/azure"
	"azure-storage/internal/azure/keyvault"
	"azure-storage/internal/cache"
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
	service *keyvault.Service

	spinner spinner.Model

	vaultsList   list.Model
	secretsList  list.Model
	versionsList list.Model

	focus int

	subscriptions []azure.Subscription
	vaults        []keyvault.Vault
	secrets       []keyvault.Secret
	versions      []keyvault.SecretVersion

	hasSubscription bool
	currentSub      azure.Subscription
	hasVault        bool
	currentVault    keyvault.Vault
	hasSecret       bool
	currentSecret   keyvault.Secret

	styles ui.Styles
	keymap KeyMap

	schemes      []ui.Scheme
	themeOverlay ui.ThemeOverlayState
	helpOverlay  ui.HelpOverlayState
	subOverlay   ui.SubscriptionOverlayState

	cache kvCache

	// EmbeddedMode suppresses theme/help overlay handling and quit
	// interception so the parent tabapp can own those concerns.
	EmbeddedMode bool

	loading bool
	status  string
	lastErr string

	width      int
	height     int
	paneWidths [3]int // vlt, sec, ver — set by resize
	paneHeight int
}

type subscriptionsLoadedMsg struct {
	subscriptions []azure.Subscription
	done          bool
	err           error
	next          tea.Cmd
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
	return NewModelWithKeyMap(svc, cfg, DefaultKeyMap(), db)
}

func NewModelWithKeyMap(svc *keyvault.Service, cfg ui.Config, keymap KeyMap, db *cache.DB) Model {
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

	spin := spinner.New()
	spin.Spinner = spinner.Dot

	m := Model{
		service:      svc,
		spinner:      spin,
		vaultsList:   vaults,
		secretsList:  secrets,
		versionsList: versionsList,
		focus:        vaultsPane,
		cache:        newCache(db),
		schemes:      cfg.Schemes,
		themeOverlay: ui.ThemeOverlayState{
			ActiveThemeIdx: ui.ActiveSchemeIndex(cfg),
		},
		keymap:  keymap,
		status:  "Loading Azure subscriptions...",
		loading: true,
	}
	m.applyScheme(cfg.ActiveScheme())
	return m
}

// NewModelWithCache creates a Model using pre-built shared cache stores.
func NewModelWithCache(svc *keyvault.Service, cfg ui.Config, stores KVStores) Model {
	m := NewModel(svc, cfg, nil)
	m.cache = NewCacheWithStores(stores)
	return m
}

func (m *Model) applyScheme(scheme ui.Scheme) {
	m.styles = ui.NewStyles(scheme)
	m.styles.ApplyToLists([]*list.Model{
		&m.vaultsList, &m.secretsList, &m.versionsList,
	}, &m.spinner)
}

// ApplyScheme applies the given scheme to all lists and spinner.
func (m *Model) ApplyScheme(scheme ui.Scheme) {
	m.applyScheme(scheme)
}

// HelpSections returns the help sections for the key vault explorer.
func (m Model) HelpSections() []ui.HelpSection {
	return m.keymap.HelpSections()
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
	cmds := []tea.Cmd{spinner.Tick, fetchSubscriptionsCmd(m.service, m.cache.subscriptions)}
	if m.hasSubscription {
		cmds = append(cmds, fetchVaultsCmd(m.service, m.cache.vaults, m.currentSub.ID))
	}
	return tea.Batch(cmds...)
}
