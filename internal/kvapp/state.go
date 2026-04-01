package kvapp

import (
	"azure-storage/internal/azure"
	"azure-storage/internal/azure/keyvault"
	"azure-storage/internal/ui"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

const (
	subscriptionsPane = iota
	vaultsPane
	secretsPane
	versionsPane
)

type Model struct {
	service *keyvault.Service

	spinner spinner.Model

	subscriptionsList list.Model
	vaultsList        list.Model
	secretsList       list.Model
	versionsList      list.Model

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

	palette      ui.Palette
	syntaxStyles ui.SyntaxStyles
	keymap       KeyMap

	appName      string
	themes       []ui.Theme
	themeOverlay ui.ThemeOverlayState
	helpOverlay  ui.HelpOverlayState

	cache kvCache

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

type vaultsLoadedMsg struct {
	subscriptionID string
	vaults         []keyvault.Vault
	err            error
}

type secretsLoadedMsg struct {
	vault   keyvault.Vault
	secrets []keyvault.Secret
	err     error
}

type versionsLoadedMsg struct {
	vault      keyvault.Vault
	secretName string
	versions   []keyvault.SecretVersion
	err        error
}

type secretValueYankedMsg struct {
	secretName string
	version    string
	err        error
}

func NewModel(svc *keyvault.Service, cfg ui.Config) Model {
	return NewModelWithKeyMap(svc, cfg, DefaultKeyMap())
}

func NewModelWithKeyMap(svc *keyvault.Service, cfg ui.Config, keymap KeyMap) Model {
	delegate := list.NewDefaultDelegate()

	subscriptions := list.New([]list.Item{}, delegate, 28, 10)
	subscriptions.Title = "Subscriptions"
	subscriptions.SetShowHelp(false)
	subscriptions.SetShowPagination(false)
	subscriptions.SetShowStatusBar(true)
	subscriptions.SetStatusBarItemName("subscription", "subscriptions")
	subscriptions.SetFilteringEnabled(true)
	subscriptions.DisableQuitKeybindings()

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
		service:           svc,
		spinner:           spin,
		subscriptionsList: subscriptions,
		vaultsList:        vaults,
		secretsList:       secrets,
		versionsList:      versionsList,
		focus:             subscriptionsPane,
		cache:             newCache(),
		appName:           cfg.AppName,
		themes:            cfg.Themes,
		themeOverlay: ui.ThemeOverlayState{
			ActiveThemeIdx: ui.ActiveThemeIndex(cfg),
		},
		keymap:  keymap,
		status:  "Loading Azure subscriptions...",
		loading: true,
	}
	m.applyTheme(cfg.ActiveTheme())
	return m
}

func (m *Model) applyTheme(theme ui.Theme) {
	m.palette, m.syntaxStyles = ui.ApplyThemeToLists(theme, []*list.Model{
		&m.subscriptionsList, &m.vaultsList, &m.secretsList, &m.versionsList,
	}, &m.spinner)
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(spinner.Tick, loadSubscriptionsCmd(m.service))
}
