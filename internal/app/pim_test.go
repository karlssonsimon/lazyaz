package app

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/azure"
	"github.com/karlssonsimon/lazyaz/internal/azure/pim"
	"github.com/karlssonsimon/lazyaz/internal/cache"
	"github.com/karlssonsimon/lazyaz/internal/keymap"

	tea "charm.land/bubbletea/v2"
)

// --- pure helpers ---

func TestResolvePIMDuration(t *testing.T) {
	tests := []struct {
		name       string
		last       string
		hasLast    bool
		cfgDefault string
		max        time.Duration
		want       time.Duration
	}{
		{"last used wins", "3h", true, "2h", 0, 3 * time.Hour},
		{"config default", "", false, "2h", 0, 2 * time.Hour},
		{"fallback", "", false, "", 0, time.Hour},
		{"unparsable last ignored", "junk", true, "2h", 0, 2 * time.Hour},
		{"unparsable config ignored", "", false, "junk", 0, time.Hour},
		{"clamped to max", "8h", true, "2h", 4 * time.Hour, 4 * time.Hour},
		{"fallback clamped", "", false, "", 30 * time.Minute, 30 * time.Minute},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolvePIMDuration(tt.last, tt.hasLast, tt.cfgDefault, tt.max); got != tt.want {
				t.Errorf("resolvePIMDuration(%q, %v, %q, %v) = %v, want %v", tt.last, tt.hasLast, tt.cfgDefault, tt.max, got, tt.want)
			}
		})
	}
}

func TestPIMDurationValidator(t *testing.T) {
	open := pimDurationValidator(0)
	if got := open("2h"); got != "" {
		t.Errorf("validator(0)(2h) = %q, want valid", got)
	}
	if got := open("abc"); got == "" {
		t.Error("validator(0)(abc) accepted an unparsable duration")
	}
	if got := open("4m"); got != "minimum is 5m" {
		t.Errorf("validator(0)(4m) = %q, want %q", got, "minimum is 5m")
	}

	capped := pimDurationValidator(4 * time.Hour)
	if got := capped("4h"); got != "" {
		t.Errorf("validator(4h)(4h) = %q, want valid", got)
	}
	if got := capped("5h"); got != "maximum for this role is 4h" {
		t.Errorf("validator(4h)(5h) = %q, want %q", got, "maximum for this role is 4h")
	}
}

func TestPIMReasonValidator(t *testing.T) {
	for _, v := range []string{"", "   "} {
		if pimReasonValidator(v) == "" {
			t.Errorf("pimReasonValidator(%q) accepted a blank reason", v)
		}
	}
	if got := pimReasonValidator("Felsökning"); got != "" {
		t.Errorf("pimReasonValidator(Felsökning) = %q, want valid", got)
	}
}

func TestPIMDurationHint(t *testing.T) {
	tests := []struct {
		fromLast   bool
		max        time.Duration
		maxUnknown bool
		want       string
	}{
		{true, 8 * time.Hour, false, "last used · max 8h"},
		{false, 8 * time.Hour, false, "max 8h"},
		{true, 0, false, "last used"},
		{false, 0, false, ""},
		{true, 0, true, "last used · max unknown"},
		{false, 0, true, "max unknown"},
	}
	for _, tt := range tests {
		if got := pimDurationHint(tt.fromLast, tt.max, tt.maxUnknown); got != tt.want {
			t.Errorf("pimDurationHint(%v, %v, %v) = %q, want %q", tt.fromLast, tt.max, tt.maxUnknown, got, tt.want)
		}
	}
}

// --- model wiring ---

type pimActivateCall struct {
	cred   azcore.TokenCredential
	role   pim.Role
	reason string
	d      time.Duration
}

// fakePIM records every call and answers from canned fields.
type fakePIM struct {
	mu sync.Mutex

	roles     map[string][]pim.Role // by tenant ID
	listErr   map[string]error      // by tenant ID
	activeErr map[string]error      // by tenant ID
	max       time.Duration
	maxErr    error
	result    pim.ActivationResult
	actErr    error

	listCalls   []string
	listCreds   []azcore.TokenCredential
	activeCalls []string // tenant IDs
	maxCalls    []pim.Role
	maxCreds    []azcore.TokenCredential
	actCalls    []pimActivateCall
}

func (f *fakePIM) ListEligible(_ context.Context, cred azcore.TokenCredential, tenant azure.Tenant) ([]pim.Role, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listCalls = append(f.listCalls, tenant.ID)
	f.listCreds = append(f.listCreds, cred)
	if err := f.listErr[tenant.ID]; err != nil {
		return nil, err
	}
	roles := make([]pim.Role, len(f.roles[tenant.ID]))
	copy(roles, f.roles[tenant.ID])
	for i := range roles {
		roles[i].Active, roles[i].ActiveUntil = false, time.Time{}
	}
	return roles, nil
}

// ListActive answers from the roles marked Active in f.roles, for the
// tenant the credential belongs to.
func (f *fakePIM) ListActive(ctx context.Context, cred azcore.TokenCredential) ([]pim.ActiveInstance, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	tenantID := cred.(testCredential).id
	f.activeCalls = append(f.activeCalls, tenantID)
	if err := f.activeErr[tenantID]; err != nil {
		return nil, err
	}
	var out []pim.ActiveInstance
	for _, r := range f.roles[tenantID] {
		if r.Active {
			out = append(out, pim.ActiveInstance{Scope: r.Scope, RoleDefinitionID: r.RoleDefinitionID, EndTime: r.ActiveUntil})
		}
	}
	return out, nil
}

func (f *fakePIM) MaxDuration(_ context.Context, cred azcore.TokenCredential, role pim.Role) (time.Duration, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.maxCalls = append(f.maxCalls, role)
	f.maxCreds = append(f.maxCreds, cred)
	return f.max, f.maxErr
}

func (f *fakePIM) Activate(_ context.Context, cred azcore.TokenCredential, role pim.Role, reason string, d time.Duration) (pim.ActivationResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.actCalls = append(f.actCalls, pimActivateCall{cred: cred, role: role, reason: reason, d: d})
	return f.result, f.actErr
}

var (
	pimTenantA = azure.Tenant{ID: "tenant-a", DisplayName: "Contoso"}
	pimTenantB = azure.Tenant{ID: "tenant-b", DisplayName: "Fabrikam"}
)

func pimRoleIn(tenant azure.Tenant, role, scope string) pim.Role {
	return pim.Role{
		TenantID: tenant.ID, TenantName: tenant.DisplayName,
		Scope: "/subscriptions/" + scope, ScopeName: scope,
		RoleDefinitionID: "/rd/" + role, RoleName: role,
		EligibilityScheduleID: "/es/" + role,
	}
}

// newPIMTestModel builds a Model whose PIM service, tenant lister and
// credential factory are fakes. db may be nil.
func newPIMTestModel(t *testing.T, fake *fakePIM, db *cache.DB, tenants ...azure.Tenant) Model {
	t.Helper()
	m := NewModel(nil, nil, nil, nil, testConfig(), db, keymap.Default())
	m.credCache = newCredentialCache(func(id string) (azcore.TokenCredential, error) {
		return testCredential{id: id}, nil
	})
	m.pimSvc = fake
	m.pimTenants = func(context.Context) ([]azure.Tenant, error) { return tenants, nil }
	m.width, m.height = 120, 40
	// A fresh dashboard opens its subscription picker, which is a text
	// input that would swallow the global PIM key. Two idle tabs, the
	// second active, so a leaked "1" would visibly switch tabs.
	m.tabs = []Tab{{ID: 100, Model: pimIdleTab{}}, {ID: 101, Model: pimIdleTab{}}}
	m.activeIdx = 1
	// Toast ticks are tea.Tick cmds that sleep; keep them out of the
	// cmds the tests execute.
	m.toastTickActive = true
	return m
}

// pimIdleTab is a tab with no text input or overlay of its own.
type pimIdleTab struct{}

func (pimIdleTab) Init() tea.Cmd                       { return nil }
func (pimIdleTab) Update(tea.Msg) (tea.Model, tea.Cmd) { return pimIdleTab{}, nil }
func (pimIdleTab) View() tea.View                      { return tea.NewView("") }

func pimTestDB(t *testing.T) (*cache.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "c.db")
	db, err := cache.OpenDB(path)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db, path
}

func pimKey(k string) tea.KeyPressMsg {
	switch k {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case " ":
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	}
	r := []rune(k)[0]
	return tea.KeyPressMsg{Code: r, Text: k}
}

// pimUpdate applies msg and returns the model and its cmd unrun.
func pimUpdate(t *testing.T, m Model, msg tea.Msg) (Model, tea.Cmd) {
	t.Helper()
	updated, cmd := m.Update(msg)
	mm, ok := updated.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want Model", updated)
	}
	return mm, cmd
}

// pimDrain runs cmd to completion, feeding PIM messages back into the
// model. Other messages are dropped.
func pimDrain(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		return m
	}
	switch msg := cmd().(type) {
	case tea.BatchMsg:
		for _, c := range msg {
			m = pimDrain(t, m, c)
		}
	case pimTenantsLoadedMsg, pimRolesLoadedMsg, pimActiveLoadedMsg, pimPolicyLoadedMsg, pimActivatedMsg, openPIMOverlayMsg:
		var next tea.Cmd
		m, next = pimUpdate(t, m, msg)
		m = pimDrain(t, m, next)
	}
	return m
}

// pimSend applies msg and drains its cmds.
func pimSend(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()
	m, cmd := pimUpdate(t, m, msg)
	return pimDrain(t, m, cmd)
}

func pimTypeKeys(t *testing.T, m Model, s string) Model {
	t.Helper()
	for _, r := range s {
		m = pimSend(t, m, pimKey(string(r)))
	}
	return m
}

// pimOpenWithRoles presses the PIM key and lets every tenant answer.
func pimOpenWithRoles(t *testing.T, m Model) Model {
	t.Helper()
	m = pimSend(t, m, pimKey("P"))
	if !m.pim.active {
		t.Fatal("PIM overlay not active after pressing P")
	}
	return m
}

// pimClearDuration focuses the Duration field and erases its value.
func pimClearDuration(t *testing.T, m Model) Model {
	t.Helper()
	m = pimSend(t, m, pimKey("tab"))
	for range []rune(m.pimForm.Fields[1].Value) {
		m = pimSend(t, m, pimKey("backspace"))
	}
	return m
}

func lastNotification(t *testing.T, m Model) appshell.Notification {
	t.Helper()
	snap := m.notifier.Snapshot()
	if len(snap) == 0 {
		t.Fatal("no notifications")
	}
	return snap[len(snap)-1]
}

func TestPIMKeyOpensOverlayAndQueriesEveryTenant(t *testing.T) {
	fake := &fakePIM{roles: map[string][]pim.Role{
		pimTenantA.ID: {pimRoleIn(pimTenantA, "Contributor", "sub-a")},
		pimTenantB.ID: {pimRoleIn(pimTenantB, "Reader", "sub-b")},
	}}
	m := newPIMTestModel(t, fake, nil, pimTenantA, pimTenantB)

	m = pimOpenWithRoles(t, m)

	if len(fake.listCalls) != 2 {
		t.Fatalf("ListEligible calls = %v, want one per tenant", fake.listCalls)
	}
	seen := map[string]int{}
	for _, id := range fake.listCalls {
		seen[id]++
	}
	if seen[pimTenantA.ID] != 1 || seen[pimTenantB.ID] != 1 {
		t.Fatalf("ListEligible calls = %v, want tenant-a and tenant-b once each", fake.listCalls)
	}
	tenants := map[string]bool{}
	for _, r := range m.pim.roles {
		tenants[r.TenantID] = true
	}
	if len(m.pim.roles) != 2 || !tenants[pimTenantA.ID] || !tenants[pimTenantB.ID] {
		t.Fatalf("overlay roles = %+v, want one from each tenant", m.pim.roles)
	}
	if m.pim.loading || m.pim.pending != 0 {
		t.Fatalf("overlay still loading: loading=%v pending=%d", m.pim.loading, m.pim.pending)
	}
}

func TestPIMServiceCallsUseTheRolesTenantCredential(t *testing.T) {
	roleB := pimRoleIn(pimTenantB, "Owner", "sub-b")
	fake := &fakePIM{
		roles: map[string][]pim.Role{
			pimTenantA.ID: {pimRoleIn(pimTenantA, "Contributor", "sub-a")},
			pimTenantB.ID: {roleB},
		},
		result: pim.ActivationResult{Status: pim.StatusActive, Until: time.Now().Add(time.Hour)},
	}
	m := newPIMTestModel(t, fake, nil, pimTenantA, pimTenantB)
	m = pimOpenWithRoles(t, m)

	for i, id := range fake.listCalls {
		if fake.listCreds[i] != (testCredential{id: id}) {
			t.Errorf("ListEligible(%s) got credential %#v, want that tenant's", id, fake.listCreds[i])
		}
	}

	m = pimTypeKeys(t, m, "Owner")
	if r, ok := m.pim.selected(); !ok || r.Key() != roleB.Key() {
		t.Fatalf("selected = %+v, want %s", r, roleB.Label())
	}
	m = pimSend(t, m, pimKey("enter"))
	if len(fake.maxCreds) != 1 || fake.maxCreds[0] != (testCredential{id: pimTenantB.ID}) {
		t.Fatalf("MaxDuration credentials = %#v, want tenant-b's", fake.maxCreds)
	}

	m = pimTypeKeys(t, m, "because")
	m = pimSend(t, m, pimKey("enter"))
	if len(fake.actCalls) != 1 || fake.actCalls[0].cred != (testCredential{id: pimTenantB.ID}) {
		t.Fatalf("Activate calls = %#v, want one with tenant-b's credential", fake.actCalls)
	}
}

func TestPIMPaletteCommandOpensOverlay(t *testing.T) {
	m := newPIMTestModel(t, &fakePIM{}, nil, pimTenantA)

	var found *command
	for _, c := range m.buildCommands() {
		if c.name == "PIM: Activate role" {
			c := c
			found = &c
		}
	}
	if found == nil {
		t.Fatal(`buildCommands() has no "PIM: Activate role"`)
	}
	if found.hint != "P" || found.section != "Application" {
		t.Errorf("command hint=%q section=%q, want P / Application", found.hint, found.section)
	}

	action := found.action()
	if action.msg == nil {
		t.Fatal("command action has no message")
	}
	m = pimSend(t, m, action.msg)
	if !m.pim.active {
		t.Fatal("executing the palette command did not open the overlay")
	}
}

func TestPIMTenantFailureIsCountedNotNotified(t *testing.T) {
	fake := &fakePIM{
		roles:   map[string][]pim.Role{pimTenantA.ID: {pimRoleIn(pimTenantA, "Contributor", "sub-a")}},
		listErr: map[string]error{pimTenantB.ID: errors.New("forbidden")},
	}
	m := newPIMTestModel(t, fake, nil, pimTenantA, pimTenantB)
	before := m.notifier.Len()

	m = pimOpenWithRoles(t, m)

	if len(m.pim.failures) != 1 {
		t.Errorf("failures = %v, want 1", m.pim.failures)
	}
	if !pimHasNote(&m.pim, "1 tenant could not be queried — Fabrikam: forbidden") {
		t.Errorf("footer = %v, want the failed tenant and its error", pimLabels(&m.pim))
	}
	if len(m.pim.roles) != 1 || m.pim.roles[0].TenantID != pimTenantA.ID {
		t.Errorf("roles = %+v, want tenant-a's role", m.pim.roles)
	}
	if got := m.notifier.Len(); got != before {
		t.Errorf("notifications = %d, want %d (no notification per failed tenant)", got, before)
	}
}

func TestPIMActiveStateUnknownShownNotNotified(t *testing.T) {
	fake := &fakePIM{
		roles:     map[string][]pim.Role{pimTenantA.ID: {pimRoleIn(pimTenantA, "Contributor", "sub-a")}},
		activeErr: map[string]error{pimTenantA.ID: errors.New("forbidden")},
	}
	m := newPIMTestModel(t, fake, nil, pimTenantA)
	before := m.notifier.Len()

	m = pimOpenWithRoles(t, m)

	if !pimHasNote(&m.pim, "Active state unknown for Contoso: forbidden") {
		t.Errorf("footer = %v, want active state unknown for Contoso", pimLabels(&m.pim))
	}
	if len(m.pim.roles) != 1 || len(m.pim.failures) != 0 {
		t.Errorf("roles = %d failures = %v, want the role listed and no failure", len(m.pim.roles), m.pim.failures)
	}
	if got := m.notifier.Len(); got != before {
		t.Errorf("notifications = %d, want %d", got, before)
	}
}

func TestPIMOverlayCapturesGlobalKeys(t *testing.T) {
	fake := &fakePIM{roles: map[string][]pim.Role{
		pimTenantA.ID: {pimRoleIn(pimTenantA, "Contributor", "sub-a")},
	}}
	m := newPIMTestModel(t, fake, nil, pimTenantA)
	m = pimOpenWithRoles(t, m)
	activeIdx, tabs := m.activeIdx, len(m.tabs)

	for _, k := range []string{"T", "?", "q", "1", "P"} {
		var cmd tea.Cmd
		m, cmd = pimUpdate(t, m, pimKey(k))
		if cmd != nil {
			if _, quit := cmd().(tea.QuitMsg); quit {
				t.Fatalf("key %q quit the app", k)
			}
		}
	}

	if m.pim.query != "T?q1P" {
		t.Errorf("query = %q, want %q", m.pim.query, "T?q1P")
	}
	if !m.pim.active {
		t.Error("overlay closed")
	}
	if m.themeOverlay.Active || m.helpOverlay.Active {
		t.Errorf("theme=%v help=%v, want both inactive", m.themeOverlay.Active, m.helpOverlay.Active)
	}
	if m.activeIdx != activeIdx || len(m.tabs) != tabs {
		t.Errorf("tabs changed: activeIdx %d->%d, len %d->%d", activeIdx, m.activeIdx, tabs, len(m.tabs))
	}
}

func TestPIMPasteGoesToQueryThenFocusedField(t *testing.T) {
	fake := &fakePIM{roles: map[string][]pim.Role{
		pimTenantA.ID: {pimRoleIn(pimTenantA, "Contributor", "sub-a")},
	}}
	m := newPIMTestModel(t, fake, nil, pimTenantA)
	m = pimOpenWithRoles(t, m)

	m = pimSend(t, m, tea.PasteMsg{Content: "Contr"})
	if m.pim.query != "Contr" {
		t.Fatalf("query = %q, want pasted text", m.pim.query)
	}
	m = pimSend(t, m, pimKey("enter"))
	if !m.pimForm.Active {
		t.Fatal("form not open")
	}
	m = pimSend(t, m, tea.PasteMsg{Content: "INC-42"})
	if got := m.pimForm.Fields[0].Value; got != "INC-42" {
		t.Errorf("Reason = %q, want pasted text", got)
	}
	if m.pim.query != "Contr" {
		t.Errorf("query = %q, paste leaked into the list", m.pim.query)
	}
}

func TestPIMEnterOnActiveRoleDoesNotOpenForm(t *testing.T) {
	role := pimRoleIn(pimTenantA, "Contributor", "sub-a")
	role.Active = true
	role.ActiveUntil = time.Now().Add(90 * time.Minute)
	fake := &fakePIM{roles: map[string][]pim.Role{pimTenantA.ID: {role}}}
	m := newPIMTestModel(t, fake, nil, pimTenantA)
	m = pimOpenWithRoles(t, m)
	before := m.notifier.Len()

	m = pimSend(t, m, pimKey("enter"))

	if m.pimForm.Active {
		t.Error("form opened for an active role")
	}
	if !m.pim.active {
		t.Error("overlay closed")
	}
	if got := m.notifier.Len(); got != before+1 {
		t.Fatalf("notifications = %d, want %d", got, before+1)
	}
	n := lastNotification(t, m)
	if n.Level != appshell.LevelInfo || !strings.Contains(n.Message, "already active") || !strings.Contains(n.Message, "left") {
		t.Errorf("notification = %+v, want info about time left", n)
	}
	if len(fake.maxCalls) != 0 {
		t.Errorf("MaxDuration called for an active role")
	}
}

func TestPIMEnterOnExpiredActiveRoleOpensForm(t *testing.T) {
	role := pimRoleIn(pimTenantA, "Contributor", "sub-a")
	role.Active = true
	role.ActiveUntil = time.Now().Add(-time.Minute)
	fake := &fakePIM{roles: map[string][]pim.Role{pimTenantA.ID: {role}}}
	m := newPIMTestModel(t, fake, nil, pimTenantA)
	m = pimOpenWithRoles(t, m)

	m = pimSend(t, m, pimKey("enter"))

	if !m.pimForm.Active {
		t.Error("form not opened for a role whose activation has expired")
	}
}

func TestPIMEnterOnActiveRoleWithoutEndDoesNotOpenForm(t *testing.T) {
	role := pimRoleIn(pimTenantA, "Contributor", "sub-a")
	role.Active = true
	fake := &fakePIM{roles: map[string][]pim.Role{pimTenantA.ID: {role}}}
	m := newPIMTestModel(t, fake, nil, pimTenantA)
	m = pimOpenWithRoles(t, m)

	m = pimSend(t, m, pimKey("enter"))

	if m.pimForm.Active {
		t.Error("form opened for a role active without an end time")
	}
	if n := lastNotification(t, m); !strings.Contains(n.Message, "already active") {
		t.Errorf("notification = %+v, want already active", n)
	}
}

func TestPIMEnterOpensFormWithPrefill(t *testing.T) {
	db, _ := pimTestDB(t)
	role := pimRoleIn(pimTenantA, "Contributor", "sub-foo-prod")
	db.SetPreference(pimDurationPrefKey(role), "3h")
	fake := &fakePIM{roles: map[string][]pim.Role{pimTenantA.ID: {role}}}
	m := newPIMTestModel(t, fake, db, pimTenantA)
	m.cfg.PIM.DefaultDuration = "2h"
	m = pimOpenWithRoles(t, m)

	m = pimSend(t, m, pimKey("enter"))

	if !m.pimForm.Active {
		t.Fatal("form not open")
	}
	if !m.pim.active {
		t.Error("list closed underneath the form")
	}
	if m.pimForm.Title != "Activate Contributor @ sub-foo-prod" {
		t.Errorf("title = %q", m.pimForm.Title)
	}
	if f := m.pimForm.Fields[0]; f.Label != "Reason" || f.Value != "" {
		t.Errorf("field 0 = %q/%q, want empty Reason", f.Label, f.Value)
	}
	if f := m.pimForm.Fields[1]; f.Value != "3h" || !strings.Contains(f.Hint, "last used") {
		t.Errorf("duration = %q hint %q, want 3h with last used", f.Value, f.Hint)
	}
	if len(fake.maxCalls) != 1 || fake.maxCalls[0].Key() != role.Key() {
		t.Errorf("MaxDuration calls = %+v, want one for %s", fake.maxCalls, role.Label())
	}
}

func TestPIMPolicyResultClampsUntouchedPrefill(t *testing.T) {
	fake := &fakePIM{
		roles: map[string][]pim.Role{pimTenantA.ID: {pimRoleIn(pimTenantA, "Contributor", "sub-a")}},
		max:   4 * time.Hour,
	}
	m := newPIMTestModel(t, fake, nil, pimTenantA)
	m.cfg.PIM.DefaultDuration = "8h"
	m = pimOpenWithRoles(t, m)

	m = pimSend(t, m, pimKey("enter")) // opens the form at 8h, policy answers 4h

	f := m.pimForm.Fields[1]
	if f.Value != "4h" || !strings.Contains(f.Hint, "max 4h") {
		t.Fatalf("duration = %q hint %q, want 4h with max 4h", f.Value, f.Hint)
	}
	if m.pimFormMax != 4*time.Hour {
		t.Errorf("pimFormMax = %v, want 4h", m.pimFormMax)
	}

	m = pimTypeKeys(t, m, "why")
	m = pimClearDuration(t, m)
	m = pimTypeKeys(t, m, "5h")
	m = pimSend(t, m, pimKey("enter"))

	if !m.pimForm.Active {
		t.Fatal("form submitted a duration over the policy maximum")
	}
	if got := m.pimForm.Fields[1].Error; got != "maximum for this role is 4h" {
		t.Errorf("duration error = %q", got)
	}
	if len(fake.actCalls) != 0 {
		t.Errorf("Activate called %d times", len(fake.actCalls))
	}
}

func TestPIMPolicyErrorShowsMaxUnknown(t *testing.T) {
	db, _ := pimTestDB(t)
	role := pimRoleIn(pimTenantA, "Contributor", "sub-a")
	db.SetPreference(pimDurationPrefKey(role), "3h")
	fake := &fakePIM{
		roles:  map[string][]pim.Role{pimTenantA.ID: {role}},
		maxErr: errors.New("forbidden"),
	}
	m := newPIMTestModel(t, fake, db, pimTenantA)
	m = pimOpenWithRoles(t, m)

	m = pimSend(t, m, pimKey("enter"))

	f := m.pimForm.Fields[1]
	if f.Value != "3h" || f.Hint != "last used · max unknown" {
		t.Errorf("duration = %q hint %q, want 3h with %q", f.Value, f.Hint, "last used · max unknown")
	}
	if got := f.Validate("24h"); got != "" {
		t.Errorf("validator(24h) = %q, want valid while the maximum is unknown", got)
	}
}

func TestPIMPolicyResultDoesNotOverwriteEditedDuration(t *testing.T) {
	role := pimRoleIn(pimTenantA, "Contributor", "sub-a")
	fake := &fakePIM{roles: map[string][]pim.Role{pimTenantA.ID: {role}}}
	m := newPIMTestModel(t, fake, nil, pimTenantA)
	m.cfg.PIM.DefaultDuration = "8h"
	m = pimOpenWithRoles(t, m)
	m = pimSend(t, m, pimKey("enter"))
	m = pimClearDuration(t, m)
	m = pimTypeKeys(t, m, "6h")

	m = pimSend(t, m, pimPolicyLoadedMsg{gen: m.pim.gen, key: role.Key(), max: 4 * time.Hour})

	f := m.pimForm.Fields[1]
	if f.Value != "6h" {
		t.Errorf("duration = %q, want the user's 6h", f.Value)
	}
	if !strings.Contains(f.Hint, "max 4h") {
		t.Errorf("hint = %q, want max 4h", f.Hint)
	}
}

func TestPIMPolicyResultForOtherRoleIgnored(t *testing.T) {
	roleA := pimRoleIn(pimTenantA, "Contributor", "sub-a")
	roleB := pimRoleIn(pimTenantA, "Reader", "sub-b")
	fake := &fakePIM{roles: map[string][]pim.Role{pimTenantA.ID: {roleA, roleB}}}
	m := newPIMTestModel(t, fake, nil, pimTenantA)
	m.cfg.PIM.DefaultDuration = "8h"
	m = pimOpenWithRoles(t, m)
	m = pimSend(t, m, pimKey("enter"))
	if m.pimFormRole.Key() != roleA.Key() {
		t.Fatalf("form role = %s, want %s", m.pimFormRole.Label(), roleA.Label())
	}
	want := m.pimForm.Fields[1]

	for name, msg := range map[string]pimPolicyLoadedMsg{
		"other role": {gen: m.pim.gen, key: roleB.Key(), max: 4 * time.Hour},
		"old gen":    {gen: m.pim.gen - 1, key: roleA.Key(), max: 4 * time.Hour},
	} {
		got := pimSend(t, m, msg)
		if got.pimFormMax != 0 {
			t.Errorf("%s: pimFormMax = %v, want 0", name, got.pimFormMax)
		}
		if f := got.pimForm.Fields[1]; f.Value != want.Value || f.Hint != want.Hint {
			t.Errorf("%s: duration field = %q/%q, want %q/%q", name, f.Value, f.Hint, want.Value, want.Hint)
		}
	}
}

func TestPIMSubmitRequiresReason(t *testing.T) {
	fake := &fakePIM{roles: map[string][]pim.Role{pimTenantA.ID: {pimRoleIn(pimTenantA, "Contributor", "sub-a")}}}
	m := newPIMTestModel(t, fake, nil, pimTenantA)
	m = pimOpenWithRoles(t, m)
	m = pimSend(t, m, pimKey("enter"))

	m = pimSend(t, m, pimKey("enter"))

	if !m.pimForm.Active {
		t.Fatal("form submitted without a reason")
	}
	if m.pimForm.Fields[0].Error == "" {
		t.Error("Reason has no error")
	}
	if len(fake.actCalls) != 0 {
		t.Errorf("Activate called %d times", len(fake.actCalls))
	}
}

func TestPIMSubmitActivatesAndRemembersDuration(t *testing.T) {
	db, path := pimTestDB(t)
	role := pimRoleIn(pimTenantA, "Contributor", "sub-a")
	fake := &fakePIM{
		roles:  map[string][]pim.Role{pimTenantA.ID: {role}},
		result: pim.ActivationResult{Status: pim.StatusActive, Until: time.Now().Add(2 * time.Hour)},
	}
	m := newPIMTestModel(t, fake, db, pimTenantA)
	m = pimOpenWithRoles(t, m)
	m = pimSend(t, m, pimKey("enter"))

	const reason = "Felsökning av app X"
	for _, r := range reason {
		m = pimSend(t, m, pimKey(string(r)))
	}
	m = pimClearDuration(t, m)
	m = pimTypeKeys(t, m, "2h")
	m = pimSend(t, m, pimKey("enter"))

	if m.pimForm.Active || m.pim.active {
		t.Errorf("form=%v list=%v, want both closed", m.pimForm.Active, m.pim.active)
	}
	if len(fake.actCalls) != 1 {
		t.Fatalf("Activate calls = %d, want 1", len(fake.actCalls))
	}
	call := fake.actCalls[0]
	if call.role.Key() != role.Key() || call.reason != reason || call.d != 2*time.Hour {
		t.Errorf("Activate(%s, %q, %v), want (%s, %q, 2h)", call.role.Label(), call.reason, call.d, role.Label(), reason)
	}
	if got, ok := db.GetPreference(pimDurationPrefKey(role)); !ok || got != "2h" {
		t.Errorf("duration pref = %q ok=%v, want 2h", got, ok)
	}
	n := lastNotification(t, m)
	if n.Level != appshell.LevelSuccess || !strings.Contains(n.Message, "active until") || !strings.Contains(n.Message, "few minutes") {
		t.Errorf("last notification = %+v", n)
	}
	for _, e := range m.notifier.Snapshot() {
		if e.Spinner {
			t.Errorf("spinner still pending: %+v", e)
		}
		if strings.Contains(e.Message, reason) {
			t.Errorf("notification leaks the reason: %q", e.Message)
		}
	}

	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer raw.Close()
	rows, err := raw.Query(`SELECT key, value FROM preferences`)
	if err != nil {
		t.Fatalf("query preferences: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(k, reason) || strings.Contains(v, reason) {
			t.Errorf("preference %q=%q contains the reason", k, v)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}

func TestPIMFormCancelReturnsToList(t *testing.T) {
	fake := &fakePIM{roles: map[string][]pim.Role{pimTenantA.ID: {
		pimRoleIn(pimTenantA, "Contributor", "sub-a"),
		pimRoleIn(pimTenantA, "Contributor", "sub-b"),
		pimRoleIn(pimTenantA, "Reader", "sub-c"),
	}}}
	m := newPIMTestModel(t, fake, nil, pimTenantA)
	m = pimOpenWithRoles(t, m)
	m = pimTypeKeys(t, m, "Contr")
	m = pimSend(t, m, pimKey("down"))
	query, cursor := m.pim.query, m.pim.cursor

	m = pimSend(t, m, pimKey("enter"))
	if !m.pimForm.Active {
		t.Fatal("form not open")
	}
	m = pimSend(t, m, pimKey("esc"))

	if m.pimForm.Active {
		t.Error("form still open after esc")
	}
	if !m.pim.active {
		t.Fatal("list closed with the form")
	}
	if m.pim.query != query || m.pim.cursor != cursor {
		t.Errorf("query/cursor = %q/%d, want %q/%d", m.pim.query, m.pim.cursor, query, cursor)
	}
}

func TestPIMActivationOutcomes(t *testing.T) {
	role := pimRoleIn(pimTenantA, "Contributor", "sub-a")
	tests := []struct {
		name      string
		result    pim.ActivationResult
		err       error
		level     appshell.NotificationLevel
		contains  []string
		savesPref bool
	}{
		{"pending approval", pim.ActivationResult{Status: pim.StatusPendingApproval}, nil,
			appshell.LevelInfo, []string{"pending approval"}, true},
		{"accepted", pim.ActivationResult{Status: pim.StatusAccepted, RawStatus: "Accepted"}, nil,
			appshell.LevelInfo, []string{"Contributor @ sub-a: request accepted (status Accepted)"}, true},
		{"mfa", pim.ActivationResult{}, &pim.ActivationError{Kind: pim.ErrMFARequired, Message: "acrs"},
			appshell.LevelError, []string{pimPortalURL, "MFA", pimPortalURL + " (acrs)"}, false},
		{"ticket", pim.ActivationResult{}, &pim.ActivationError{Kind: pim.ErrTicketRequired, Message: "TicketingRule"},
			appshell.LevelError, []string{pimPortalURL, "ticket", pimPortalURL + " (TicketingRule)"}, false},
		{"already active", pim.ActivationResult{}, &pim.ActivationError{Kind: pim.ErrAlreadyActive, Message: "exists"},
			appshell.LevelInfo, []string{"already active"}, false},
		{"other", pim.ActivationResult{}, errors.New("boom"),
			appshell.LevelError, []string{"boom"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, _ := pimTestDB(t)
			m := newPIMTestModel(t, &fakePIM{}, db)
			id := m.notifier.PushSpinner("Activating " + role.Label() + "…")

			m = pimSend(t, m, pimActivatedMsg{role: role, duration: 2 * time.Hour, result: tt.result, err: tt.err, spinnerID: id})

			n := lastNotification(t, m)
			if n.Level != tt.level {
				t.Errorf("level = %v, want %v", n.Level, tt.level)
			}
			for _, s := range tt.contains {
				if !strings.Contains(n.Message, s) {
					t.Errorf("message %q does not contain %q", n.Message, s)
				}
			}
			_, saved := db.GetPreference(pimDurationPrefKey(role))
			if saved != tt.savesPref {
				t.Errorf("duration pref written = %v, want %v", saved, tt.savesPref)
			}
		})
	}
}

func TestPIMActivationResultHandledAfterOverlayClosed(t *testing.T) {
	role := pimRoleIn(pimTenantA, "Contributor", "sub-a")
	m := newPIMTestModel(t, &fakePIM{}, nil, pimTenantA)
	m.pim.open()
	m.pim.close()
	m.pim.open()
	m.pim.close()
	id := m.notifier.PushSpinner("Activating " + role.Label() + "…")

	m = pimSend(t, m, pimActivatedMsg{
		role: role, duration: time.Hour, spinnerID: id,
		result: pim.ActivationResult{Status: pim.StatusActive, Until: time.Now().Add(time.Hour)},
	})

	for _, e := range m.notifier.Snapshot() {
		if e.Spinner && e.SpinnerID == id {
			t.Fatal("spinner not resolved")
		}
	}
	if n := lastNotification(t, m); n.Level != appshell.LevelSuccess {
		t.Errorf("last notification = %+v, want success", n)
	}
}

func TestPIMTenantListErrorClosesOverlay(t *testing.T) {
	m := newPIMTestModel(t, &fakePIM{}, nil)
	m.pimTenants = func(context.Context) ([]azure.Tenant, error) { return nil, errors.New("no session") }
	before := m.notifier.Len()

	m = pimSend(t, m, pimKey("P"))

	if m.pim.active {
		t.Error("overlay still open")
	}
	if got := m.notifier.Len(); got != before+1 {
		t.Fatalf("notifications = %d, want %d", got, before+1)
	}
	n := lastNotification(t, m)
	if n.Level != appshell.LevelError || !strings.Contains(n.Message, "could not list tenants") {
		t.Errorf("notification = %+v", n)
	}
}

// pimSubTab is an idle tab that reports a current subscription.
type pimSubTab struct {
	pimIdleTab
	sub azure.Subscription
}

func (t pimSubTab) CurrentSubscription() (azure.Subscription, bool) { return t.sub, true }
func (t pimSubTab) WithSubscription(azure.Subscription) tea.Model   { return t }
func (t pimSubTab) WithSubscriptions([]azure.Subscription) tea.Model {
	return t
}
func (t pimSubTab) WithoutSubscription([]azure.Subscription) tea.Model { return t }

func TestPIMOverlayOpensScopedToActiveTabSubscription(t *testing.T) {
	fake := &fakePIM{roles: map[string][]pim.Role{
		pimTenantA.ID: {
			pimRoleIn(pimTenantA, "Contributor", "sub-a"),
			pimRoleIn(pimTenantA, "Contributor", "sub-b"),
		},
	}}
	m := newPIMTestModel(t, fake, nil, pimTenantA)
	m.tabs[1] = Tab{ID: 101, Model: pimSubTab{sub: azure.Subscription{ID: "sub-a", Name: "sub-a", TenantID: pimTenantA.ID}}}
	m = pimOpenWithRoles(t, m)
	if got := pimRoleRows(&m.pim); got != 1 {
		t.Fatalf("rows = %d, want 1 (only sub-a)", got)
	}
}

func TestPIMQueriesActiveOncePerTenant(t *testing.T) {
	roleA := pimRoleIn(pimTenantA, "Contributor", "sub-a")
	roleA.Active, roleA.ActiveUntil = true, time.Now().Add(time.Hour)
	fake := &fakePIM{roles: map[string][]pim.Role{
		pimTenantA.ID: {roleA},
		pimTenantB.ID: {pimRoleIn(pimTenantB, "Reader", "sub-b")},
	}}
	m := newPIMTestModel(t, fake, nil, pimTenantA, pimTenantB)
	m = pimOpenWithRoles(t, m)

	sort.Strings(fake.activeCalls)
	if want := []string{pimTenantA.ID, pimTenantB.ID}; !slices.Equal(fake.activeCalls, want) {
		t.Fatalf("active calls = %v, want %v", fake.activeCalls, want)
	}
	for _, r := range m.pim.roles {
		if r.Active != (r.TenantID == pimTenantA.ID) {
			t.Errorf("role %s in %s: Active = %v", r.RoleName, r.TenantID, r.Active)
		}
	}
	if pimHasNote(&m.pim, "Checking which roles are active…") {
		t.Errorf("still checking after every tenant answered: %v", pimLabels(&m.pim))
	}
}
