package kvapp

import (
	"time"

	"azure-storage/internal/azure"
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
	keymap keymap.Keymap

	schemes      []ui.Scheme
	themeOverlay ui.ThemeOverlayState
	helpOverlay  ui.HelpOverlayState
	subOverlay   ui.SubscriptionOverlayState

	inspectFields []ui.InspectField
	inspectTitle  string

	cache kvCache

	// EmbeddedMode suppresses theme/help overlay handling and quit
	// interception so the parent tabapp can own those concerns.
	EmbeddedMode bool

	loading          bool
	loadingPane      int
	loadingStartedAt time.Time
	status           string
	lastErr          string

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

	spin := spinner.New()
	spin.Spinner = spinner.Dot

	m := Model{
		service:      svc,
		spinner:      spin,
		vaultsList:   vaults,
		secretsList:  secrets,
		versionsList: versionsList,
		focus:        vaultsPane,
		loadingPane:  -1,
		cache:        newCache(db),
		schemes:      cfg.Schemes,
		themeOverlay: ui.ThemeOverlayState{
			ActiveThemeIdx: ui.ActiveSchemeIndex(cfg),
		},
		keymap: km,
	}
	m.applyScheme(cfg.ActiveScheme())
	// Hydrate subscriptions from cache without hitting Azure.
	if cached, ok := m.cache.subscriptions.Get(""); ok {
		m.subscriptions = cached
	}
	if !m.hasSubscription {
		m.subOverlay.Open()
		m.setLoading(-1)
		m.status = "Loading Azure subscriptions..."
	}
	return m
}

// NewModelWithCache creates a Model using pre-built shared cache stores.
func NewModelWithCache(svc *keyvault.Service, cfg ui.Config, stores KVStores, km keymap.Keymap) Model {
	m := NewModelWithKeyMap(svc, cfg, km, nil)
	m.cache = NewCacheWithStores(stores)
	// Re-hydrate subscriptions from the shared store.
	if cached, ok := m.cache.subscriptions.Get(""); ok {
		m.subscriptions = cached
	}
	return m
}

func (m *Model) setLoading(pane int) {
	if !m.loading {
		m.loadingStartedAt = time.Now()
	}
	m.loading = true
	m.loadingPane = pane
}

func (m *Model) clearLoading() {
	m.loading = false
	m.loadingPane = -1
}

// loadingHoldExpiredMsg is sent after the min-visible spinner hold elapses.
type loadingHoldExpiredMsg struct {
	status string
}

// finishLoading completes a load, holding the spinner visible for at least
// ui.SpinnerMinVisible. If the hold has not yet elapsed, returns a delayed
// command; otherwise clears loading immediately and sets the status.
func (m *Model) finishLoading(status string) tea.Cmd {
	remaining := ui.SpinnerMinVisible - time.Since(m.loadingStartedAt)
	if remaining > 0 {
		return tea.Tick(remaining, func(t time.Time) tea.Msg {
			return loadingHoldExpiredMsg{status: status}
		})
	}
	m.clearLoading()
	m.status = status
	return nil
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
	km := m.keymap
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
	cmds := []tea.Cmd{spinner.Tick}
	if m.subOverlay.Active {
		cmds = append(cmds, fetchSubscriptionsCmd(m.service, m.cache.subscriptions, true))
	}
	if m.hasSubscription {
		cmds = append(cmds, fetchVaultsCmd(m.service, m.cache.vaults, m.currentSub.ID))
	}
	return tea.Batch(cmds...)
}
