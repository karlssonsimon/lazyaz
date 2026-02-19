package blobapp

import (
	"azure-storage/internal/azure"
	"azure-storage/internal/ui"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const defaultDownloadRoot = "downloads"
const defaultBlobPrefixSearchLimit = 500
const defaultHierarchyBlobLoadLimit = 500

const (
	subscriptionsPane = iota
	accountsPane
	containersPane
	blobsPane
	previewPane
)

const (
	colorBorder        = "#4B5563"
	colorBorderFocused = "#22C55E"
	colorText          = "#E5E7EB"
	colorMuted         = "#94A3B8"
	colorAccent        = "#60A5FA"
	colorAccentStrong  = "#38BDF8"
	colorDanger        = "#F87171"
	colorFilterMatch   = "#F59E0B"
	colorSelectedBg    = "#334155"
	colorSelectedText  = "#F8FAFC"
)

func blobPalette() ui.Palette {
	return ui.Palette{
		Border:        colorBorder,
		BorderFocused: colorBorderFocused,
		Text:          colorText,
		Muted:         colorMuted,
		Accent:        colorAccent,
		AccentStrong:  colorAccentStrong,
		Danger:        colorDanger,
		FilterMatch:   colorFilterMatch,
		SelectedBg:    colorSelectedBg,
		SelectedText:  colorSelectedText,
	}
}

func blobSyntaxStyles() ui.SyntaxStyles {
	return ui.NewSyntaxStyles(ui.SyntaxPalette{
		Key:         colorAccent,
		String:      colorAccentStrong,
		Number:      colorFilterMatch,
		Bool:        colorFilterMatch,
		Null:        colorDanger,
		Punctuation: colorMuted,
		XMLTag:      colorAccent,
		XMLAttr:     colorFilterMatch,
		CSVCellA:    colorText,
		CSVCellB:    colorAccentStrong,
	})
}

type Model struct {
	service *azure.Service

	spinner spinner.Model

	subscriptionsList list.Model
	accountsList      list.Model
	containersList    list.Model
	blobsList         list.Model

	focus int

	subscriptions   []azure.Subscription
	accounts        []azure.Account
	containers      []azure.ContainerInfo
	blobs           []azure.BlobEntry
	markedBlobs     map[string]azure.BlobEntry
	visualLineMode  bool
	visualAnchor    string
	hasSubscription bool
	currentSub      azure.Subscription
	hasAccount      bool
	currentAccount  azure.Account
	hasContainer    bool
	containerName   string
	prefix          string
	blobLoadAll     bool
	blobSearchQuery string
	preview         previewState
	pendingPreviewG bool
	keymap          KeyMap
	syntaxStyles    ui.SyntaxStyles

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

type accountsLoadedMsg struct {
	subscriptionID string
	accounts       []azure.Account
	err            error
}

type containersLoadedMsg struct {
	account    azure.Account
	containers []azure.ContainerInfo
	err        error
}

type blobsLoadedMsg struct {
	account   azure.Account
	container string
	prefix    string
	loadAll   bool
	query     string
	blobs     []azure.BlobEntry
	err       error
}

type blobsDownloadedMsg struct {
	destinationRoot string
	total           int
	downloaded      int
	failed          int
	failures        []string
	err             error
}

type previewWindowLoadedMsg struct {
	requestID   int
	account     azure.Account
	container   string
	blobName    string
	blobSize    int64
	contentType string
	windowStart int64
	cursor      int64
	data        []byte
	err         error
}

func NewModel(svc *azure.Service) Model {
	return NewModelWithKeyMap(svc, DefaultKeyMap())
}

func NewModelWithKeyMap(svc *azure.Service, keymap KeyMap) Model {
	palette := blobPalette()
	delegate := ui.NewDefaultDelegate(palette)

	subscriptions := list.New([]list.Item{}, delegate, 28, 10)
	subscriptions.Title = "Subscriptions"
	subscriptions.SetShowHelp(false)
	subscriptions.SetShowPagination(false)
	subscriptions.SetShowStatusBar(true)
	subscriptions.SetStatusBarItemName("subscription", "subscriptions")
	subscriptions.SetFilteringEnabled(true)
	subscriptions.DisableQuitKeybindings()
	ui.StyleList(&subscriptions, palette)

	accounts := list.New([]list.Item{}, delegate, 24, 10)
	accounts.Title = "Storage Accounts"
	accounts.SetShowHelp(false)
	accounts.SetShowPagination(false)
	accounts.SetShowStatusBar(true)
	accounts.SetStatusBarItemName("account", "accounts")
	accounts.SetFilteringEnabled(true)
	accounts.DisableQuitKeybindings()
	ui.StyleList(&accounts, palette)

	containers := list.New([]list.Item{}, delegate, 24, 10)
	containers.Title = "Containers"
	containers.SetShowHelp(false)
	containers.SetShowPagination(false)
	containers.SetShowStatusBar(true)
	containers.SetStatusBarItemName("container", "containers")
	containers.SetFilteringEnabled(true)
	containers.DisableQuitKeybindings()
	ui.StyleList(&containers, palette)

	blobs := list.New([]list.Item{}, delegate, 40, 10)
	blobs.Title = "Blobs"
	blobs.SetShowHelp(false)
	blobs.SetShowPagination(false)
	blobs.SetShowStatusBar(true)
	blobs.SetStatusBarItemName("entry", "entries")
	blobs.SetFilteringEnabled(true)
	blobs.DisableQuitKeybindings()
	ui.StyleList(&blobs, palette)

	spin := spinner.New()
	spin.Spinner = spinner.Dot
	spin.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(palette.AccentStrong))

	return Model{
		service:           svc,
		spinner:           spin,
		subscriptionsList: subscriptions,
		accountsList:      accounts,
		containersList:    containers,
		blobsList:         blobs,
		markedBlobs:       make(map[string]azure.BlobEntry),
		preview:           newPreviewState(),
		keymap:            keymap,
		syntaxStyles:      blobSyntaxStyles(),
		focus:             subscriptionsPane,
		status:            "Loading Azure subscriptions...",
		loading:           true,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(spinner.Tick, loadSubscriptionsCmd(m.service))
}
