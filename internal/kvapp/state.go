package kvapp

import (
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
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

// Miller column panes, left → right. The kindPane is a fixed 3-row
// list (Secrets / Certificates / Keys); selecting a row sets m.kvKind
// and reloads the items column from cache + refetches.
const (
	vaultsPane = iota
	kindPane
	secretsPane // historically "secrets"; renders whichever kvKind is active
	versionsPane
)

// InputMode represents the user's current interaction mode. It is a
// computed property (via inputMode()) derived from existing boolean
// state, used for key dispatch and data handler safety.
type InputMode int

const (
	ModeNormal     InputMode = iota // Browsing lists
	ModeActionMenu                  // Action menu overlay open
	ModeOverlay                     // Sub/Theme/Help overlay open
	ModeListFilter                  // User is typing a list filter
	ModeVisualLine                  // Visual line selection active
	ModeForm                        // Multi-field form overlay open (e.g., create secret)
)

// inputMode returns the current interaction mode by checking state
// flags in priority order. This determines which key handler runs
// and how data handlers should behave.
func (m Model) inputMode() InputMode {
	switch {
	case m.createSecret.Active, m.createKey.Active, m.importCert.Active:
		return ModeForm
	case m.certImportBrowserActive:
		// File browser is its own kind of text input (filter / typing
		// for path navigation). Treat it as a form so the parent tabapp
		// doesn't snatch single-key shortcuts (q-quit, 1–9 tab-jump).
		return ModeForm
	case m.actionMenu.Active:
		return ModeActionMenu
	case m.SubOverlay.Active, m.ThemeOverlay.Active, m.HelpOverlay.Active:
		return ModeOverlay
	case m.focusedListSettingFilter():
		return ModeListFilter
	case m.visualLineMode && m.focus == secretsPane:
		return ModeVisualLine
	default:
		return ModeNormal
	}
}

func (mode InputMode) String() string {
	switch mode {
	case ModeVisualLine:
		return "VISUAL"
	case ModeListFilter:
		return "FILTER"
	default:
		return "NORMAL"
	}
}

// kvKind selects which collection of vault objects the items column
// renders: secrets (default), certificates, or keys. Driven by the
// cursor position in the kindPane (see navigation.go).
type kvKind int

const (
	kvKindSecrets kvKind = iota
	kvKindCertificates
	kvKindKeys
)

func (k kvKind) String() string {
	switch k {
	case kvKindCertificates:
		return "certificates"
	case kvKindKeys:
		return "keys"
	default:
		return "secrets"
	}
}

// titleLabel returns the column heading for the middle pane in this kind.
func (k kvKind) titleLabel() string {
	switch k {
	case kvKindCertificates:
		return "CERTIFICATES"
	case kvKindKeys:
		return "KEYS"
	default:
		return "SECRETS"
	}
}

type Model struct {
	appshell.Model

	service *keyvault.Service

	vaultsList   list.Model
	secretsList  list.Model
	versionsList list.Model
	// kindList is the fixed three-row chooser between vaults and items.
	// Cursor position drives m.kvKind on selection.
	kindList list.Model

	focus int

	vaults         []keyvault.Vault
	secrets        []keyvault.Secret
	versions       []keyvault.SecretVersion
	certs          []keyvault.Certificate
	certVersions   []keyvault.CertificateVersion
	keys           []keyvault.Key
	keyVersions    []keyvault.KeyVersion
	markedSecrets  map[string]keyvault.Secret
	visualLineMode bool
	visualAnchor   string

	// kvKind picks which child collection the items/versions columns
	// surface for the current vault. Driven by the cursor position in
	// the kindPane — selecting a kind row sets this and reloads items.
	// Only the data slices and inspect/action menu vary; the rendered
	// list widgets (secretsList, versionsList) are reused.
	kvKind kvKind

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
	hasCert       bool
	currentCert   keyvault.Certificate
	hasKey        bool
	currentKey    keyvault.Key

	// Pending navigation drives eager/advance drill-in from programmatic
	// openers (today only ctrl+o restore via ApplyNav). See pending_nav.go.
	pendingNav PendingNav

	// applyingNav suppresses RecordJumpMsg emission while ApplyNav is
	// driving navigation. See jump.go.
	applyingNav bool

	actionMenu    actionMenuState
	createSecret  ui.FormOverlayState
	createKey     ui.FormOverlayState
	importCert    ui.FormOverlayState
	confirmModal  ui.ConfirmModalState
	confirmAction func() tea.Cmd
	// certImportBrowser is the local-filesystem picker for selecting the
	// PFX file to import. Mirrors blobapp.uploadBrowser. Populated when
	// the user opens "Import certificate..."; the picked path is stashed
	// on pendingCertPath while the import form collects name/password.
	certImportBrowser       ui.FileBrowserState
	certImportBrowserActive bool
	pendingCertPath         string
	cache                   kvCache

	loadingSpinnerID int

	// Per-pane inspect strip toggle. When inspectPanes[pane] is true, the
	// pane renders an inline detail strip (via ui.RenderInspectStrip) under
	// its list. The strip updates live as the cursor moves so the user can
	// keep browsing while details remain visible. Toggled with K.
	inspectPanes map[int]bool

	// Revealed secret values, keyed by secret name (for the latest version)
	// or by "<secretName>@<version>" (for a specific version). Presence in
	// the map means the inspect strip should render the value in the clear.
	// Cleared when the active subscription or vault changes so values from
	// one tenant/vault don't leak across.
	revealedSecrets  map[string]string
	revealedVersions map[string]string

	clickTracker ui.ClickTracker
	paneWidths   [4]int // vlt, kind, items, ver — set by resize
	paneHeight   int

	// CredResolver, when set, supplies a credential for the active
	// subscription's tenant. The parent injects this; standalone
	// (non-embedded) usage leaves it nil and falls back to the service's
	// existing credential.
	CredResolver interface {
		CredentialFor(tenantID string) (azcore.TokenCredential, error)
	}
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

type certsLoadedMsg struct {
	vault keyvault.Vault
	certs []keyvault.Certificate
	done  bool
	err   error
	next  tea.Cmd
}

type certVersionsLoadedMsg struct {
	vault    keyvault.Vault
	certName string
	versions []keyvault.CertificateVersion
	done     bool
	err      error
	next     tea.Cmd
}

type keysLoadedMsg struct {
	vault keyvault.Vault
	keys  []keyvault.Key
	done  bool
	err   error
	next  tea.Cmd
}

type keyVersionsLoadedMsg struct {
	vault    keyvault.Vault
	keyName  string
	versions []keyvault.KeyVersion
	done     bool
	err      error
	next     tea.Cmd
}

type secretValueYankedMsg struct {
	secretName string
	version    string
	err        error
}

// secretRevealedMsg is the result of fetching a secret value purely for
// on-screen display (no clipboard write). On success the value is stored
// in revealedSecrets/revealedVersions; on error a notification is shown
// and the reveal request is dropped silently.
type secretRevealedMsg struct {
	secretName string
	version    string
	value      string
	err        error
}

func NewModel(svc *keyvault.Service, cfg ui.Config, db *cache.DB) Model {
	return NewModelWithKeyMap(svc, cfg, keymap.Default(), db)
}

func NewModelWithKeyMap(svc *keyvault.Service, cfg ui.Config, km keymap.Keymap, db *cache.DB) Model {
	if svc == nil {
		svc = keyvault.NewService(nil)
	}
	delegate := list.NewDefaultDelegate()

	vaults := list.New([]list.Item{}, delegate, 24, 10)
	vaults.SetShowTitle(false)
	vaults.SetShowFilter(false) // filter UI lives in our SubHeader
	vaults.SetShowHelp(false)
	vaults.SetShowPagination(false)
	vaults.SetShowStatusBar(false)
	vaults.SetStatusBarItemName("vault", "vaults")
	vaults.SetFilteringEnabled(true)
	vaults.DisableQuitKeybindings()

	secrets := list.New([]list.Item{}, delegate, 24, 10)
	secrets.SetShowTitle(false)
	secrets.SetShowFilter(false)
	secrets.SetShowHelp(false)
	secrets.SetShowPagination(false)
	secrets.SetShowStatusBar(false)
	secrets.SetStatusBarItemName("secret", "secrets")
	secrets.SetFilteringEnabled(true)
	secrets.DisableQuitKeybindings()

	versionsList := list.New([]list.Item{}, delegate, 40, 10)
	versionsList.SetShowTitle(false)
	versionsList.SetShowFilter(false)
	versionsList.SetShowHelp(false)
	versionsList.SetShowPagination(false)
	versionsList.SetShowStatusBar(false)
	versionsList.SetStatusBarItemName("version", "versions")
	versionsList.SetFilteringEnabled(true)
	versionsList.DisableQuitKeybindings()

	kindList := list.New(kindItems(), delegate, 24, 10)
	kindList.SetShowTitle(false)
	kindList.SetShowFilter(false)
	kindList.SetShowHelp(false)
	kindList.SetShowPagination(false)
	kindList.SetShowStatusBar(false)
	kindList.SetStatusBarItemName("kind", "kinds")
	kindList.SetFilteringEnabled(true)
	kindList.DisableQuitKeybindings()

	// Override bubbles list cursor bindings so they follow the user's
	// configured CursorUp/CursorDown keys.
	for _, l := range []*list.Model{&vaults, &secrets, &versionsList, &kindList} {
		l.KeyMap.CursorUp = km.CursorUp.AsBubbleKey()
		l.KeyMap.CursorDown = km.CursorDown.AsBubbleKey()
	}

	m := Model{
		Model:           appshell.New(cfg, km),
		service:         svc,
		vaultsList:      vaults,
		secretsList:     secrets,
		versionsList:    versionsList,
		kindList:        kindList,
		markedSecrets:   make(map[string]keyvault.Secret),
		focus:           vaultsPane,
		cache:           newCache(db),
		vaultsHistory:   make(map[string]ui.ListState),
		secretsHistory:  make(map[string]ui.ListState),
		versionsHistory: make(map[string]ui.ListState),
		inspectPanes:     make(map[int]bool),
		revealedSecrets:  make(map[string]string),
		revealedVersions: make(map[string]string),
	}
	m.applyScheme(cfg.ActiveScheme())
	// Hydrate subscriptions from cache without hitting Azure.
	m.HydrateSubscriptionsFromCache(m.cache.subscriptions)
	if !m.HasSubscription {
		m.SubOverlay.Open()
		m.startLoading(-1, "Loading Azure subscriptions...")
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
	m.Styles.ApplyToLists([]*list.Model{
		&m.vaultsList, &m.kindList, &m.secretsList, &m.versionsList,
	}, &m.Spinner)
	d := newSecretDelegate(m.Styles.Delegate, m.Styles)
	d.marked = m.markedSecrets
	d.visual = m.visualSelectionNames()
	m.secretsList.SetDelegate(d)
}

// ApplyScheme applies the given scheme to all lists and spinner.
func (m *Model) ApplyScheme(scheme ui.Scheme) {
	m.applyScheme(scheme)
}

func (m Model) WithScheme(scheme ui.Scheme) tea.Model {
	m.ApplyScheme(scheme)
	return m
}

// HelpSections returns the help sections for the key vault explorer.
func (m Model) HelpSections() []ui.HelpSection {
	km := m.Keymap
	return []ui.HelpSection{
		{
			Title: "Navigation",
			Items: []string{
				keymap.HelpEntry(km.NextFocus, "focus next column"),
				keymap.HelpEntry(km.PreviousFocus, "focus previous column"),
				keymap.HelpEntry(km.FilterInput, "filter focused column"),
				keymap.HelpEntry(km.OpenFocused, "open selected row"),
				keymap.HelpEntry(km.BackspaceUp, "go up/back"),
				keymap.HelpEntry(keymap.New(km.HalfPageDown.Label()+"/"+km.HalfPageUp.Label()), "half-page scroll"),
				"  vaults → kind → items → versions  (kind picks secrets / certificates / keys)",
				"  action menu in items → Create secret / Create key / Import certificate (kind-aware)",
			},
		},
		{
			Title: "Secrets",
			Items: []string{
				keymap.HelpEntry(km.ActionMenu, "action menu"),
				keymap.HelpEntry(km.YankSecret, "yank secret value to clipboard"),
				keymap.HelpEntry(km.RevealSecret, "toggle on-screen secret reveal (note: terminal scrollback may retain it)"),
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
	if m.service != nil && m.CredResolver != nil && sub.TenantID != "" {
		if cred, err := m.CredResolver.CredentialFor(sub.TenantID); err == nil {
			m.service.SetCredential(cred)
		} else {
			m.Notify(appshell.LevelError, fmt.Sprintf("Credential for tenant %s: %s", sub.TenantID, err.Error()))
		}
	}
	if cached, ok := m.cache.vaults.Get(sub.ID); ok {
		m.vaults = cached
		m.vaultsList.Title = fmt.Sprintf("Vaults (%d)", len(cached))
		ui.SetItemsPreserveKey(&m.vaultsList, vaultsToItems(cached), vaultItemKey)
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
		cmds = append(cmds, fetchSubscriptionsCmd(m.service, m.cache.subscriptions, m.Tenant, m.Subscriptions))
	}
	if m.HasSubscription {
		cmds = append(cmds, fetchVaultsCmd(m.service, m.cache.vaults, m.CurrentSub.ID, m.vaults))
	}
	return tea.Batch(cmds...)
}
