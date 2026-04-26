package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/azure"
	"github.com/karlssonsimon/lazyaz/internal/fuzzy"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	tea "charm.land/bubbletea/v2"
)

// tenantPickerState manages the overlay for choosing a tenant before
// running az login --tenant <id>.
type tenantPickerState struct {
	active   bool
	loading  bool
	tenants  []azure.Tenant
	cursor   int
	query    string
	filtered []int
}

func (s *tenantPickerState) open() {
	s.active = true
	s.loading = true
	s.tenants = nil
	s.cursor = 0
	s.query = ""
	s.filtered = nil
}

func (s *tenantPickerState) close() {
	s.active = false
	s.loading = false
}

func (s *tenantPickerState) refilter() {
	s.filtered = fuzzy.Filter(s.query, s.tenants, func(t azure.Tenant) string {
		return t.DisplayName + " " + t.Domain + " " + t.ID
	})
	if s.cursor >= len(s.filtered) {
		s.cursor = max(0, len(s.filtered)-1)
	}
}

func (s *tenantPickerState) visibleItems() []ui.OverlayItem {
	items := make([]ui.OverlayItem, len(s.filtered))
	for ci, ti := range s.filtered {
		t := s.tenants[ti]
		label := t.DisplayName
		if label == "" {
			label = t.ID
		}
		items[ci] = ui.OverlayItem{
			Label: " " + label,
			Desc:  "  " + t.Domain,
		}
	}
	return items
}

func (s *tenantPickerState) selected() (azure.Tenant, bool) {
	if len(s.filtered) == 0 || s.cursor >= len(s.filtered) {
		return azure.Tenant{}, false
	}
	return s.tenants[s.filtered[s.cursor]], true
}

func (s *tenantPickerState) handleKey(key string, bindings ui.ThemeKeyBindings) (azure.Tenant, bool) {
	if len(s.tenants) == 0 && !s.loading {
		s.close()
		return azure.Tenant{}, false
	}

	count := len(s.filtered)

	switch {
	case bindings.Up.Matches(key):
		if s.cursor > 0 {
			s.cursor--
		}
	case bindings.Down.Matches(key):
		if s.cursor < count-1 {
			s.cursor++
		}
	case bindings.Apply.Matches(key):
		if t, ok := s.selected(); ok {
			return t, true
		}
	case bindings.Cancel.Matches(key):
		if s.query != "" {
			s.query = ""
			s.refilter()
		} else {
			s.close()
		}
	case bindings.Erase != nil && bindings.Erase.Matches(key):
		if len(s.query) > 0 {
			s.query = s.query[:len(s.query)-1]
			s.refilter()
		}
	default:
		if len(key) == 1 && key[0] >= 32 && key[0] < 127 {
			s.query += key
			s.refilter()
		}
	}
	return azure.Tenant{}, false
}

// -- Messages --

// tenantsLoadedMsg is sent when tenant listing completes.
type tenantsLoadedMsg struct {
	tenants []azure.Tenant
	err     error
}

// azLoginFinishedMsg is sent when the az login process exits.
type azLoginFinishedMsg struct {
	err error
}

// tenantCredentialMsg carries a freshly created credential scoped to
// the chosen tenant. Produced by switchTenantCmd.
type tenantCredentialMsg struct {
	cred azcore.TokenCredential
	err  error
}

// postLoginSubsMsg carries the subscription list fetched after a
// credential swap, so tabs can be updated without the empty-overlay flash.
type postLoginSubsMsg struct {
	subs []azure.Subscription
	err  error
}

// -- Commands --

func listTenantsCmd(cred azcore.TokenCredential) tea.Cmd {
	return func() tea.Msg {
		tenants, err := azure.ListTenants(context.Background(), cred)
		return tenantsLoadedMsg{tenants: tenants, err: err}
	}
}

// switchTenantCmd creates a new credential scoped to the given tenant.
// No browser sign-in needed — it reuses the existing az login session.
func switchTenantCmd(tenantID string) tea.Cmd {
	return func() tea.Msg {
		cred, err := azure.NewCredentialForTenant(tenantID)
		return tenantCredentialMsg{cred: cred, err: err}
	}
}

func fetchPostLoginSubsCmd(cred azcore.TokenCredential) tea.Cmd {
	return func() tea.Msg {
		var all []azure.Subscription
		err := azure.ListSubscriptions(context.Background(), cred, func(batch []azure.Subscription) {
			all = append(all, batch...)
		})
		return postLoginSubsMsg{subs: all, err: err}
	}
}

func azLoginCmd() tea.Cmd {
	c := exec.Command("az", "login")
	// Disable the interactive subscription selector introduced in az CLI v2 —
	// the app has its own picker. This overrides the core.login_experience_v2
	// config setting via the env var that knack's CLIConfig reads.
	c.Env = append(os.Environ(), "AZURE_CORE_LOGIN_EXPERIENCE_V2=false")
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return azLoginFinishedMsg{err: err}
	})
}

// openAzLoginMsg triggers the az login / tenant picker flow.
type openAzLoginMsg struct{}

// -- Model methods --

// handleOpenAzLogin opens the tenant picker and starts fetching tenants.
func (m *Model) handleOpenAzLogin() (Model, tea.Cmd) {
	if m.blobSvc == nil {
		m.tenantPicker.close()
		m.notifier.Push(appshell.LevelError, "Azure credential unavailable")
		return *m, nil
	}
	m.tenantPicker.open()
	return *m, listTenantsCmd(m.blobSvc.Credential())
}

// handleTenantsLoaded processes the tenant list result.
func (m *Model) handleTenantsLoaded(msg tenantsLoadedMsg) (Model, tea.Cmd) {
	m.tenantPicker.loading = false
	if msg.err != nil {
		// Can't list tenants — not logged in. Run az login directly.
		m.tenantPicker.close()
		m.notifier.Push(appshell.LevelInfo, "Opening az login...")
		return *m, azLoginCmd()
	}
	m.tenantPicker.tenants = msg.tenants
	m.tenantPicker.refilter()
	return *m, nil
}

// handleTenantSelected creates a credential for the chosen tenant.
// No browser needed — reuses the existing az login session.
func (m *Model) handleTenantSelected(tenant azure.Tenant) (Model, tea.Cmd) {
	m.tenantPicker.close()
	m.notifier.Push(appshell.LevelInfo, fmt.Sprintf("Switching to %s...", tenant.DisplayName))
	return *m, switchTenantCmd(tenant.ID)
}

// handleTenantCredential swaps the tenant-scoped credential into all
// services, resets caches, and opens the subscription picker.
func (m *Model) handleTenantCredential(msg tenantCredentialMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		m.notifier.Push(appshell.LevelError, fmt.Sprintf("Failed to switch tenant: %s", msg.err))
		return *m, nil
	}
	return m.applyNewCredential(msg.cred, "Switched tenant")
}

// handleAzLoginFinished re-initializes credentials after az login exits.
func (m *Model) handleAzLoginFinished(msg azLoginFinishedMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		m.notifier.Push(appshell.LevelError, fmt.Sprintf("az login failed: %s", msg.err))
		return *m, nil
	}
	cred, err := azure.NewDefaultCredential()
	if err != nil {
		m.notifier.Push(appshell.LevelError, fmt.Sprintf("Failed to create credential: %s", err))
		return *m, nil
	}
	return m.applyNewCredential(cred, "Logged in successfully")
}

// applyNewCredential swaps a credential into all services, resets caches,
// and starts a subscription fetch. Tabs are not touched yet — that happens
// in handlePostLoginSubs once we know which subscriptions are available.
func (m *Model) applyNewCredential(cred azcore.TokenCredential, successMsg string) (Model, tea.Cmd) {
	if m.blobSvc != nil {
		m.blobSvc.SetCredential(cred)
	}
	if m.sbSvc != nil {
		m.sbSvc.SetCredential(cred)
	}
	if m.kvSvc != nil {
		m.kvSvc.SetCredential(cred)
	}

	m.brokers.resetAll()
	m.pendingLoginMsg = successMsg

	return *m, fetchPostLoginSubsCmd(cred)
}

// handlePostLoginSubs runs after the subscription fetch completes. For each
// tab it checks whether the current subscription still exists in the new
// tenant. If so, the subscription is silently re-applied (triggering a
// resource refresh with the new credential). Otherwise the subscription
// picker opens with the list already populated — no empty flash.
func (m *Model) handlePostLoginSubs(msg postLoginSubsMsg) (Model, tea.Cmd) {
	successMsg := m.pendingLoginMsg
	m.pendingLoginMsg = ""

	if msg.err != nil {
		m.notifier.Push(appshell.LevelError, fmt.Sprintf("Failed to load subscriptions: %s", msg.err))
		return *m, nil
	}

	// Seed the broker cache so child tabs don't re-fetch.
	m.brokers.subscriptions.Set("", msg.subs)

	// Build a set for fast lookup.
	subsByID := make(map[string]azure.Subscription, len(msg.subs))
	for _, s := range msg.subs {
		subsByID[s.ID] = s
	}

	var cmds []tea.Cmd
	for i := range m.tabs {
		if child, ok := m.tabs[i].Model.(credentialTab); ok {
			m.tabs[i].Model = child.WithCredential(m.credentialForTabKind(m.tabs[i].Kind))
		}

		child, ok := m.tabs[i].Model.(subscriptionTab)
		if !ok {
			continue
		}
		prevSub, _ := child.CurrentSubscription()

		if matched, ok := subsByID[prevSub.ID]; ok {
			// The tab's subscription still exists in the new tenant.
			// Re-apply the matched subscription so the tab's private
			// service is scoped to that subscription's tenant.
			updated := child.WithSubscriptions(msg.subs)
			if subChild, ok := updated.(subscriptionTab); ok {
				updated = subChild.WithSubscription(matched)
			}
			m.tabs[i].Model = updated
		} else {
			// Subscription gone — clear it and open the picker
			// with data already populated.
			m.tabs[i].Model = child.WithoutSubscription(msg.subs)
			if c := m.tabs[i].Model.Init(); c != nil {
				cmds = append(cmds, wrapCmd(m.tabs[i].ID, c))
			}
		}
	}

	cmds = append(cmds, m.forwardToActive(tea.WindowSizeMsg{
		Width:  m.width,
		Height: m.childHeight(),
	}))

	m.notifier.Push(appshell.LevelSuccess, successMsg)
	return *m, tea.Batch(cmds...)
}

func (m *Model) credentialForTabKind(kind TabKind) azcore.TokenCredential {
	switch kind {
	case TabBlob:
		if m.blobSvc != nil {
			return m.blobSvc.Credential()
		}
	case TabServiceBus, TabDashboard:
		if m.sbSvc != nil {
			return m.sbSvc.Credential()
		}
	case TabKeyVault:
		if m.kvSvc != nil {
			return m.kvSvc.Credential()
		}
	}
	return nil
}
