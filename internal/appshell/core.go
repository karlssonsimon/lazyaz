// Package appshell provides the UI scaffolding shared by the three Azure
// explorer apps (blobapp, sbapp, kvapp): subscription picker, theme/help
// overlays, loading spinner state, and the plumbing that ties them into
// Bubble Tea's update/view cycle.
//
// Each app composes an appshell.Model into its own Model via embedding, then
// adds its resource-specific panes, lists, and messages on top. The shell
// owns nothing resource-specific; it's the chrome around the content.
package appshell

import (
	"time"

	"azure-storage/internal/azure"
	"azure-storage/internal/keymap"
	"azure-storage/internal/ui"

	"github.com/charmbracelet/bubbles/spinner"
)

// Model holds the shell state each explorer app composes into its own Model.
// Fields are exported because Go embedding only promotes exported identifiers
// across package boundaries — this is not an invitation for outside packages
// to mutate them directly.
type Model struct {
	// Spinner is the shared loading spinner. Apps must render it via
	// ui.RenderPaneSpinner (or the status bar helper) using LoadingStartedAt.
	Spinner spinner.Model

	// Subscriptions + selection.
	Subscriptions   []azure.Subscription
	HasSubscription bool
	CurrentSub      azure.Subscription

	// Keymap + styling.
	Keymap  keymap.Keymap
	Styles  ui.Styles
	Schemes []ui.Scheme

	// Overlays — all managed by appshell.HandleOverlayKeys / RenderOverlays.
	ThemeOverlay ui.ThemeOverlayState
	HelpOverlay  ui.HelpOverlayState
	SubOverlay   ui.SubscriptionOverlayState

	// EmbeddedMode suppresses theme/help overlay handling and quit
	// interception so a parent tabapp can own those concerns.
	EmbeddedMode bool

	// Loading state. Set via SetLoading / ClearLoading / FinishLoading.
	Loading          bool
	LoadingPane      int
	LoadingStartedAt time.Time

	// Status line + most recent error string.
	Status  string
	LastErr string

	// Terminal size.
	Width  int
	Height int
}

// New builds a Model with the shared defaults all three apps use.
// Apps still need to build their own list.Model instances and wire up the
// rest of their resource-specific state.
func New(cfg ui.Config, km keymap.Keymap) Model {
	spin := spinner.New()
	spin.Spinner = spinner.Dot

	m := Model{
		Spinner:     spin,
		Keymap:      km,
		Schemes:     cfg.Schemes,
		LoadingPane: -1,
		ThemeOverlay: ui.ThemeOverlayState{
			ActiveThemeIdx: ui.ActiveSchemeIndex(cfg),
		},
	}
	m.SetScheme(cfg.ActiveScheme())
	return m
}

// SetScheme updates Styles from the given scheme and repaints the spinner.
// It does NOT touch any resource-specific lists — each app must reapply
// its own list delegates via ui.Styles.ApplyToLists.
func (m *Model) SetScheme(scheme ui.Scheme) {
	m.Styles = ui.NewStyles(scheme)
	m.Spinner.Style = m.Styles.Spinner
}
