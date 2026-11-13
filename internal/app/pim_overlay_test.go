package app

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/cursor"
	"github.com/charmbracelet/x/ansi"
	"github.com/karlssonsimon/lazyaz/internal/azure"
	"github.com/karlssonsimon/lazyaz/internal/azure/pim"
	"github.com/karlssonsimon/lazyaz/internal/keymap"
	"github.com/karlssonsimon/lazyaz/internal/ui"
)

func pimTestRole(tenant, role, scope string) pim.Role {
	return pim.Role{
		TenantID: strings.ToLower(tenant), TenantName: tenant,
		Scope: "/subscriptions/" + scope, ScopeName: scope,
		RoleDefinitionID: "/rd/" + role, RoleName: role,
	}
}

func pimLabels(p *pimOverlay) []string {
	out := make([]string, len(p.visible))
	for i, v := range p.visible {
		out[i] = v.Label
	}
	return out
}

// pimHasLabel reports a tenant header with label.
func pimHasLabel(p *pimOverlay, label string) bool {
	for _, v := range p.visible {
		if v.IsHeader && v.Label == label {
			return true
		}
	}
	return false
}

// pimHasNote reports a muted status row (Loading…, failures, empty states)
// with label.
func pimHasNote(p *pimOverlay, label string) bool {
	for _, v := range p.visible {
		if v.IsNote && !v.IsHeader && v.Label == label {
			return true
		}
	}
	return false
}

func pimType(p *pimOverlay, s string) {
	km := keymap.Default()
	for _, r := range s {
		p.handleKey(string(r), km)
	}
}

func pimRoleRows(p *pimOverlay) int {
	n := 0
	for _, idx := range p.roleIndex {
		if idx >= 0 {
			n++
		}
	}
	return n
}

func TestPIMOverlayOpenBumpsGeneration(t *testing.T) {
	var p pimOverlay
	g1 := p.open()
	p.close()
	g2 := p.open()
	if g2 == g1 {
		t.Fatalf("generation not bumped: %d", g2)
	}
	if !p.active {
		t.Fatal("overlay not active after open")
	}
}

func TestPIMOverlayDropsStaleGeneration(t *testing.T) {
	var p pimOverlay
	g1 := p.open()
	p.close()
	g2 := p.open()
	p.setTenantCount(g2, 1)
	one := []pim.Role{pimTestRole("Contoso", "Contributor", "sub-a")}

	p.addRoles(g1, one)
	if len(p.roles) != 0 || p.pending != 1 {
		t.Fatalf("stale addRoles applied: roles=%d pending=%d", len(p.roles), p.pending)
	}
	p.tenantFailed(g1, "Contoso", errors.New("forbidden"))
	if len(p.failures) != 0 || p.pending != 1 {
		t.Fatalf("stale tenantFailed applied: failures=%v pending=%d", p.failures, p.pending)
	}
	p.activeStateUnknown(g1, "Contoso", errors.New("forbidden"))
	if len(p.activeUnknown) != 0 {
		t.Fatalf("stale activeStateUnknown applied: %v", p.activeUnknown)
	}
	p.setTenantCount(g1, 9)
	if p.pending != 1 {
		t.Fatalf("stale setTenantCount applied: pending=%d", p.pending)
	}

	p.close()
	p.addRoles(g2, one)
	if len(p.roles) != 0 {
		t.Fatalf("addRoles applied to closed overlay")
	}
}

func TestPIMOverlayIncrementalFillKeepsQueryAndCursor(t *testing.T) {
	var p pimOverlay
	g := p.open()
	p.setTenantCount(g, 2)
	p.addRoles(g, []pim.Role{pimTestRole("A", "Contributor", "sub-b")})
	pimType(&p, "contrib")
	if r, ok := p.selected(); !ok || r.ScopeName != "sub-b" {
		t.Fatalf("selected = %+v, %v", r, ok)
	}

	p.addRoles(g, []pim.Role{pimTestRole("A", "Contributor", "sub-a")})
	if p.query != "contrib" {
		t.Fatalf("query = %q", p.query)
	}
	if r, ok := p.selected(); !ok || r.ScopeName != "sub-b" {
		t.Fatalf("cursor moved: selected = %+v, %v", r, ok)
	}
	if pimRoleRows(&p) != 2 {
		t.Fatalf("role rows = %d, want 2", pimRoleRows(&p))
	}
}

func TestPIMOverlayFilterMatchesRoleScopeAndTenant(t *testing.T) {
	var p pimOverlay
	g := p.open()
	p.setTenantCount(g, 2)
	p.addRoles(g, []pim.Role{pimTestRole("Contoso", "Contributor", "sub-foo-prod")})
	p.addRoles(g, []pim.Role{pimTestRole("Fabrikam", "Key Vault Administrator", "rg-baz")})

	cases := []struct{ query, role string }{
		{"vault", "Key Vault Administrator"},
		{"foo", "Contributor"},
		{"fabrikam", "Key Vault Administrator"},
	}
	for _, c := range cases {
		p.query, p.queryCaret = "", 0
		pimType(&p, c.query)
		if pimRoleRows(&p) != 1 {
			t.Fatalf("query %q: %d role rows, labels %v", c.query, pimRoleRows(&p), pimLabels(&p))
		}
		if r, _ := p.selected(); r.RoleName != c.role {
			t.Fatalf("query %q: selected %q, want %q", c.query, r.RoleName, c.role)
		}
	}
}

func TestPIMOverlayTenantHeaders(t *testing.T) {
	var p pimOverlay
	g := p.open()
	p.setTenantCount(g, 2)
	p.addRoles(g, []pim.Role{pimTestRole("Contoso", "Reader", "sub-a")})
	if pimHasLabel(&p, "CONTOSO") {
		t.Fatalf("single tenant shows header: %v", pimLabels(&p))
	}

	p.addRoles(g, []pim.Role{pimTestRole("Fabrikam", "Reader", "sub-b")})
	labels := pimLabels(&p)
	if !pimHasLabel(&p, "CONTOSO") || !pimHasLabel(&p, "FABRIKAM") {
		t.Fatalf("missing headers: %v", labels)
	}
	if !p.visible[0].IsHeader || p.visible[0].Label != "CONTOSO" || p.roleIndex[0] != -1 {
		t.Fatalf("first row not CONTOSO header: %v", labels)
	}
	if p.visible[1].IsHeader || !p.visible[2].IsHeader || p.visible[2].Label != "FABRIKAM" || p.visible[3].IsHeader {
		t.Fatalf("headers not before their rows: %v", labels)
	}
}

func TestPIMOverlayTenantHeadersKeyedByTenantID(t *testing.T) {
	idA, idB := "aaaaaaaa-1111-1111-1111-111111111111", "bbbbbbbb-2222-2222-2222-222222222222"
	role := func(id, scope string) pim.Role {
		r := pimTestRole("Contoso", "Reader", scope)
		r.TenantID = id
		return r
	}
	var p pimOverlay
	g := p.open()
	p.setTenantCount(g, 2)
	p.addRoles(g, []pim.Role{role(idA, "sub-a"), role(idA, "sub-c")})
	p.addRoles(g, []pim.Role{role(idB, "sub-b")})

	labels := pimLabels(&p)
	want := []string{"CONTOSO (aaaaaaaa)", "Reader  sub-a", "Reader  sub-c", "CONTOSO (bbbbbbbb)", "Reader  sub-b"}
	if len(labels) != len(want) {
		t.Fatalf("labels = %q, want %q", labels, want)
	}
	for i := range want {
		if labels[i] != want[i] {
			t.Fatalf("labels = %q, want %q", labels, want)
		}
	}
	if !p.visible[0].IsHeader || !p.visible[3].IsHeader {
		t.Fatalf("headers not marked: %+v", p.visible)
	}
}

func TestPIMOverlayOrdersTenantsCaseInsensitively(t *testing.T) {
	var p pimOverlay
	g := p.open()
	p.setTenantCount(g, 2)
	p.addRoles(g, []pim.Role{pimTestRole("zeta", "Reader", "s")})
	p.addRoles(g, []pim.Role{pimTestRole("Alpha", "Reader", "s")})
	if got := pimLabels(&p)[0]; got != "ALPHA" {
		t.Fatalf("first header = %q, want ALPHA", got)
	}
}

func TestPIMOverlayEmptyStates(t *testing.T) {
	km := keymap.Default()
	var p pimOverlay
	g := p.open()
	if !pimHasNote(&p, "Loading…") {
		t.Fatalf("no loading row: %v", pimLabels(&p))
	}
	if _, ok := p.selected(); ok {
		t.Fatal("selected ok while loading")
	}
	if a := p.handleKey("enter", km); a != pimActionNone {
		t.Fatalf("enter while loading = %v", a)
	}

	p.setTenantCount(g, 0)
	if !pimHasNote(&p, "No eligible roles") || pimHasNote(&p, "Loading…") {
		t.Fatalf("zero tenants: %v", pimLabels(&p))
	}

	g = p.open()
	p.setTenantCount(g, 2)
	p.tenantFailed(g, "Contoso", errors.New("AuthorizationFailed: no access\nmore detail"))
	p.tenantFailed(g, "Fabrikam", errors.New("timeout"))
	if !pimHasNote(&p, "2 tenants could not be queried — Contoso: AuthorizationFailed: no access") || pimHasNote(&p, "No eligible roles") {
		t.Fatalf("two failures: %v", pimLabels(&p))
	}

	g = p.open()
	p.setTenantCount(g, 1)
	p.tenantFailed(g, "Contoso", errors.New("forbidden"))
	if !pimHasNote(&p, "1 tenant could not be queried — Contoso: forbidden") {
		t.Fatalf("one failure: %v", pimLabels(&p))
	}

	g = p.open()
	p.setTenantCount(g, 2)
	p.addRoles(g, []pim.Role{pimTestRole("A", "Reader", "s")})
	if !pimHasNote(&p, "Loading…") {
		t.Fatalf("pending tenant should keep Loading…: %v", pimLabels(&p))
	}
	p.addRoles(g, nil)
	pimType(&p, "zzz")
	if !pimHasNote(&p, "No matches") || pimHasNote(&p, "Loading…") {
		t.Fatalf("no match: %v", pimLabels(&p))
	}
	if _, ok := p.selected(); ok {
		t.Fatal("selected ok on status row")
	}
}

func TestPIMOverlayFooterRowsAreNotes(t *testing.T) {
	var p pimOverlay
	g := p.open()
	p.setTenantCount(g, 2)
	p.tenantFailed(g, "Contoso", errors.New("forbidden"))
	for i, v := range p.visible {
		if p.roleIndex[i] != -1 {
			continue
		}
		if !v.IsNote || v.IsHeader {
			t.Errorf("footer row %q: IsNote=%v IsHeader=%v, want a note", v.Label, v.IsNote, v.IsHeader)
		}
	}
}

func TestPIMOverlayActiveStateUnknown(t *testing.T) {
	var p pimOverlay
	g := p.open()
	p.setTenantCount(g, 1)
	p.activeStateUnknown(g, "Contoso", errors.New("forbidden\nsecond line"))
	p.addRoles(g, []pim.Role{pimTestRole("Contoso", "Reader", "s")})
	if !pimHasNote(&p, "Active state unknown for Contoso: forbidden") {
		t.Fatalf("no active-state-unknown row: %v", pimLabels(&p))
	}
	if pimRoleRows(&p) != 1 {
		t.Fatalf("role rows = %d, want 1", pimRoleRows(&p))
	}
}

func TestPIMErrorLine(t *testing.T) {
	if got := pimErrorLine(errors.New("first\nsecond")); got != "first" {
		t.Errorf("multi-line: got %q, want first", got)
	}
	long := strings.Repeat("x", 300)
	got := pimErrorLine(errors.New(long))
	if n := len([]rune(got)); n != pimErrorLineMax || !strings.HasSuffix(got, "…") {
		t.Errorf("long: %d runes %q, want %d ending in …", n, got, pimErrorLineMax)
	}
}

func TestPIMOverlayCursorSkipsNonSelectableRows(t *testing.T) {
	km := keymap.Default()
	var p pimOverlay
	g := p.open()
	p.setTenantCount(g, 2)
	p.addRoles(g, []pim.Role{pimTestRole("Contoso", "Reader", "sub-a")})
	p.addRoles(g, []pim.Role{pimTestRole("Fabrikam", "Owner", "sub-b")})
	if r, ok := p.selected(); !ok || r.TenantName != "Contoso" {
		t.Fatalf("initial cursor not on first role: %+v %v (cursor %d)", r, ok, p.cursor)
	}
	p.handleKey("down", km)
	if r, ok := p.selected(); !ok || r.TenantName != "Fabrikam" {
		t.Fatalf("down landed on %+v %v", r, ok)
	}
	p.handleKey("down", km)
	if _, ok := p.selected(); !ok {
		t.Fatal("down past end left cursor on non-selectable row")
	}
	p.handleKey("up", km)
	if r, _ := p.selected(); r.TenantName != "Contoso" {
		t.Fatalf("up landed on %+v", r)
	}
}

func TestPIMOverlayCancelClearsQueryThenCloses(t *testing.T) {
	km := keymap.Default()
	var p pimOverlay
	g := p.open()
	p.setTenantCount(g, 1)
	p.addRoles(g, []pim.Role{pimTestRole("A", "Reader", "s")})
	pimType(&p, "x")
	if a := p.handleKey("esc", km); a != pimActionNone || p.query != "" {
		t.Fatalf("first esc: action %v query %q", a, p.query)
	}
	if a := p.handleKey("esc", km); a != pimActionClose {
		t.Fatalf("second esc = %v", a)
	}
}

func TestPIMOverlaySelectReturnsRole(t *testing.T) {
	km := keymap.Default()
	var p pimOverlay
	g := p.open()
	p.setTenantCount(g, 1)
	p.addRoles(g, []pim.Role{pimTestRole("A", "Reader", "s")})
	if a := p.handleKey("enter", km); a != pimActionSelect {
		t.Fatalf("enter = %v", a)
	}
	if r, ok := p.selected(); !ok || r.RoleName != "Reader" {
		t.Fatalf("selected = %+v %v", r, ok)
	}
}

func TestPIMOverlayInsertTextFilters(t *testing.T) {
	var p pimOverlay
	g := p.open()
	p.setTenantCount(g, 1)
	p.addRoles(g, []pim.Role{pimTestRole("A", "Reader", "s"), pimTestRole("A", "Owner", "s")})
	p.insertText("owner")
	if p.query != "owner" || pimRoleRows(&p) != 1 {
		t.Fatalf("query %q rows %d", p.query, pimRoleRows(&p))
	}
}

func TestPIMRowHint(t *testing.T) {
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		role pim.Role
		want string
	}{
		{"time left", pim.Role{Active: true, ActiveUntil: now.Add(time.Hour + 42*time.Minute + 30*time.Second)}, "active 1h42m"},
		{"inactive", pim.Role{ActiveUntil: now.Add(time.Hour)}, ""},
		{"expired", pim.Role{Active: true, ActiveUntil: now.Add(-time.Minute)}, ""},
		{"no end time", pim.Role{Active: true}, "active"},
		{"under a minute", pim.Role{Active: true, ActiveUntil: now.Add(30 * time.Second)}, "active <1m"},
	}
	for _, tt := range tests {
		if got := pimRowHint(tt.role, now); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestRenderPIMOverlayShowsRowsAndTitle(t *testing.T) {
	km := keymap.Default()
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	var p pimOverlay
	g := p.open()
	p.setTenantCount(g, 1)
	r := pimTestRole("Contoso", "Contributor", "sub-a")
	r.Active, r.ActiveUntil = true, now.Add(90*time.Minute)
	p.addRoles(g, []pim.Role{r})

	styles := ui.NewStyles(ui.Scheme{})
	out := renderPIMOverlay(&p, "esc close", cursor.New(), styles, &km, now, 100, 30, "")
	for _, want := range []string{"PIM — ELIGIBLE ROLES", "Contributor", "sub-a", "active 1h30m"} {
		if !strings.Contains(ansi.Strip(out), want) {
			t.Fatalf("render missing %q:\n%s", want, out)
		}
	}
}

// pimScopedRoles returns roles in two subscriptions of Contoso (one at a
// resource group below sub-a), one at a Contoso management group, and one
// in Fabrikam.
func pimScopedRoles() []pim.Role {
	rg := pimTestRole("Contoso", "Reader", "sub-a")
	rg.Scope, rg.ScopeName = "/subscriptions/sub-a/resourceGroups/rg-1", "rg-1"
	mg := pimTestRole("Contoso", "Owner", "mg")
	mg.Scope, mg.ScopeName = "/providers/Microsoft.Management/managementGroups/mg-root", "mg-root"
	return []pim.Role{
		pimTestRole("Contoso", "Contributor", "sub-a"),
		rg,
		pimTestRole("Contoso", "Contributor", "sub-b"),
		mg,
		pimTestRole("Fabrikam", "Contributor", "sub-a"),
	}
}

func pimSubA() azure.Subscription {
	return azure.Subscription{ID: "SUB-A", Name: "sub-a-prod", TenantID: "contoso"}
}

func pimRowLabels(p *pimOverlay) []string {
	var out []string
	for i, idx := range p.roleIndex {
		if idx >= 0 {
			out = append(out, p.visible[i].Label)
		}
	}
	return out
}

func TestPIMOverlayScopesToSubscription(t *testing.T) {
	var p pimOverlay
	g := p.open()
	p.setSubscription(pimSubA(), true)
	p.setTenantCount(g, 2)
	p.addRoles(g, pimScopedRoles())

	// sub-a itself, a resource group under it, and a management group in
	// the same tenant (which may cover it); not sub-b, not Fabrikam.
	rows := pimRowLabels(&p)
	if len(rows) != 3 {
		t.Fatalf("rows = %q, want 3 (sub-a, rg-1, mg-root)", rows)
	}
	for _, row := range rows {
		if strings.Contains(row, "sub-b") {
			t.Fatalf("sub-b listed while scoped to sub-a: %q", rows)
		}
	}
	if pimHasLabel(&p, "FABRIKAM") {
		t.Fatal("other tenant listed while scoped to sub-a")
	}
}

func TestPIMOverlayTabTogglesAllSubscriptions(t *testing.T) {
	km := keymap.Default()
	var p pimOverlay
	g := p.open()
	p.setSubscription(pimSubA(), true)
	p.setTenantCount(g, 2)
	p.addRoles(g, pimScopedRoles())
	pimType(&p, "contrib")

	if got := pimRoleRows(&p); got != 1 {
		t.Fatalf("scoped rows = %d, want 1", got)
	}
	p.handleKey("tab", km)
	if got := pimRoleRows(&p); got != 3 {
		t.Fatalf("all-subscription rows = %d, want 3", got)
	}
	if p.query != "contrib" {
		t.Fatalf("query = %q, toggle must keep it", p.query)
	}
	p.handleKey("tab", km)
	if got := pimRoleRows(&p); got != 1 {
		t.Fatalf("rows after toggling back = %d, want 1", got)
	}
}

func TestPIMOverlayWithoutSubscriptionShowsAll(t *testing.T) {
	km := keymap.Default()
	var p pimOverlay
	g := p.open()
	p.setSubscription(azure.Subscription{}, false)
	p.setTenantCount(g, 2)
	p.addRoles(g, pimScopedRoles())
	if got := pimRoleRows(&p); got != 5 {
		t.Fatalf("rows = %d, want all 5", got)
	}
	p.handleKey("tab", km)
	if got := pimRoleRows(&p); got != 5 {
		t.Fatalf("tab without a subscription changed rows to %d", got)
	}
}

func TestPIMOverlayScopedNoMatchesPointsAtToggle(t *testing.T) {
	var p pimOverlay
	g := p.open()
	p.setSubscription(pimSubA(), true)
	p.setTenantCount(g, 2)
	p.addRoles(g, pimScopedRoles())
	pimType(&p, "fabrikam")
	if !pimHasNote(&p, "No matches in sub-a-prod — tab searches all subscriptions") {
		t.Fatalf("notes = %q", pimLabels(&p))
	}
}

func TestPIMOverlayScopedNoRolesPointsAtToggle(t *testing.T) {
	var p pimOverlay
	g := p.open()
	p.setSubscription(azure.Subscription{ID: "sub-z", Name: "sub-z", TenantID: "contoso"}, true)
	p.setTenantCount(g, 1)
	mgless := []pim.Role{pimTestRole("Contoso", "Contributor", "sub-a")}
	p.addRoles(g, mgless)
	if !pimHasNote(&p, "No eligible roles in sub-z — tab shows all subscriptions") {
		t.Fatalf("notes = %q", pimLabels(&p))
	}
}

func TestPIMOverlaySpaceSeparatedTermsAllMustMatch(t *testing.T) {
	var p pimOverlay
	g := p.open()
	p.setTenantCount(g, 1)
	p.addRoles(g, []pim.Role{
		pimTestRole("Contoso", "Contributor", "sub-prod"),
		pimTestRole("Contoso", "Contributor", "sub-test"),
		pimTestRole("Contoso", "Reader", "sub-prod"),
	})
	p.insertText("contributor prod")
	rows := pimRowLabels(&p)
	if len(rows) != 1 || !strings.Contains(rows[0], "sub-prod") || !strings.HasPrefix(rows[0], "Contributor") {
		t.Fatalf("rows = %q, want only Contributor @ sub-prod", rows)
	}
}

func TestRenderPIMOverlayGrowsWithTerminal(t *testing.T) {
	km := keymap.Default()
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	var p pimOverlay
	g := p.open()
	p.setTenantCount(g, 1)
	var roles []pim.Role
	for i := 0; i < 60; i++ {
		roles = append(roles, pimTestRole("Contoso", "Storage Blob Data Contributor", fmt.Sprintf("example-subscription-%02d", i)))
	}
	p.addRoles(g, roles)

	styles := ui.NewStyles(ui.Scheme{})
	out := ansi.Strip(renderPIMOverlay(&p, "esc", cursor.New(), styles, &km, now, 200, 60, ""))
	// Full scope names fit and well over the default 20 rows are shown.
	if !strings.Contains(out, "example-subscription-00") {
		t.Fatalf("scope name truncated in wide terminal:\n%s", out)
	}
	if n := strings.Count(out, "example-subscription-"); n < 30 {
		t.Fatalf("visible rows = %d in a 60-line terminal, want at least 30", n)
	}
	// And it still fits a small terminal.
	small := ansi.Strip(renderPIMOverlay(&p, "esc", cursor.New(), styles, &km, now, 80, 24, ""))
	if lines := strings.Count(small, "\n") + 1; lines > 24 {
		t.Fatalf("render is %d lines in a 24-line terminal", lines)
	}
}

func TestRenderPIMOverlayShowsToggleHint(t *testing.T) {
	km := keymap.Default()
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	var p pimOverlay
	g := p.open()
	p.setSubscription(pimSubA(), true)
	p.setTenantCount(g, 1)
	p.addRoles(g, pimScopedRoles())
	styles := ui.NewStyles(ui.Scheme{})
	out := ansi.Strip(renderPIMOverlay(&p, "esc", cursor.New(), styles, &km, now, 160, 40, ""))
	for _, want := range []string{"sub-a-prod", "tab", "all subscriptions"} {
		if !strings.Contains(out, want) {
			t.Fatalf("render missing %q:\n%s", want, out)
		}
	}
	p.handleKey("tab", km)
	out = ansi.Strip(renderPIMOverlay(&p, "esc", cursor.New(), styles, &km, now, 160, 40, ""))
	if !strings.Contains(out, "only sub-a-prod") {
		t.Fatalf("render after toggle missing %q:\n%s", "only sub-a-prod", out)
	}
}

func pimActiveFor(r pim.Role, until time.Time) pim.ActiveInstance {
	return pim.ActiveInstance{Scope: r.Scope, RoleDefinitionID: r.RoleDefinitionID, EndTime: until}
}

func TestPIMOverlayShowsRolesBeforeActiveStateArrives(t *testing.T) {
	var p pimOverlay
	g := p.open()
	p.setTenantCount(g, 1)
	p.expectActive(g, 1)
	r := pimTestRole("Contoso", "Contributor", "sub-a")
	p.addRoles(g, []pim.Role{r})

	if pimRoleRows(&p) != 1 {
		t.Fatalf("rows = %d, want the role listed while active state is pending", pimRoleRows(&p))
	}
	if !pimHasNote(&p, "Checking which roles are active…") {
		t.Fatalf("notes = %q, want a checking-active row", pimLabels(&p))
	}
	if pimHasNote(&p, "Loading…") {
		t.Fatalf("notes = %q: eligible roles are loaded, Loading… must be gone", pimLabels(&p))
	}

	until := time.Now().Add(time.Hour)
	p.activeLoaded(g, "contoso", []pim.ActiveInstance{pimActiveFor(r, until)})
	if !p.roles[0].Active || !p.roles[0].ActiveUntil.Equal(until) {
		t.Fatalf("role = %+v, want active until %v", p.roles[0], until)
	}
	if pimHasNote(&p, "Checking which roles are active…") {
		t.Fatalf("notes = %q, checking row must go once active state is in", pimLabels(&p))
	}
}

func TestPIMOverlayActiveBeforeRolesStillApplies(t *testing.T) {
	var p pimOverlay
	g := p.open()
	p.setTenantCount(g, 1)
	p.expectActive(g, 1)
	r := pimTestRole("Contoso", "Contributor", "sub-a")
	until := time.Now().Add(time.Hour)
	p.activeLoaded(g, "contoso", []pim.ActiveInstance{pimActiveFor(r, until)})
	p.addRoles(g, []pim.Role{r})
	if !p.roles[0].Active {
		t.Fatalf("role = %+v, want active state applied to roles arriving later", p.roles[0])
	}
}

func TestPIMOverlayActiveFailureNotesAndStopsChecking(t *testing.T) {
	var p pimOverlay
	g := p.open()
	p.setTenantCount(g, 1)
	p.expectActive(g, 1)
	p.addRoles(g, []pim.Role{pimTestRole("Contoso", "Contributor", "sub-a")})
	p.activeStateUnknown(g, "Contoso", errors.New("forbidden"))
	if !pimHasNote(&p, "Active state unknown for Contoso: forbidden") {
		t.Fatalf("notes = %q", pimLabels(&p))
	}
	if pimHasNote(&p, "Checking which roles are active…") {
		t.Fatalf("still checking after the only tenant answered: %q", pimLabels(&p))
	}
}

func TestPIMOverlayDropsStaleActive(t *testing.T) {
	var p pimOverlay
	g1 := p.open()
	p.close()
	g2 := p.open()
	p.setTenantCount(g2, 1)
	p.expectActive(g1, 1)
	p.expectActive(g2, 1)
	r := pimTestRole("Contoso", "Contributor", "sub-a")
	p.addRoles(g2, []pim.Role{r})
	p.activeLoaded(g1, "contoso", []pim.ActiveInstance{pimActiveFor(r, time.Now().Add(time.Hour))})
	if p.roles[0].Active {
		t.Fatal("stale active answer applied")
	}
	if p.activePending != 1 {
		t.Fatalf("activePending = %d, stale answer must not count", p.activePending)
	}
}

func TestPIMOverlayWordsMatchAsWholeText(t *testing.T) {
	var p pimOverlay
	g := p.open()
	p.setTenantCount(g, 1)
	p.addRoles(g, []pim.Role{
		pimTestRole("Contoso", "Azure Kubernetes Service RBAC Reader", "aks-example"),
		pimTestRole("Contoso", "Storage Account Contributor", "shared-data-archive-tier"),
		pimTestRole("Contoso", "Storage File Data Privileged Contributor", "example-records-prod"),
		pimTestRole("Contoso", "Backup Reader", "example-api-prod"),
	})
	p.insertText("RBAC Reader")
	rows := pimRowLabels(&p)
	if len(rows) != 1 || !strings.HasPrefix(rows[0], "Azure Kubernetes Service RBAC Reader") {
		t.Fatalf("rows = %q, want only the RBAC Reader role", rows)
	}
	p.close()
	g = p.open()
	p.setTenantCount(g, 1)
	p.addRoles(g, []pim.Role{pimTestRole("Contoso", "Storage Blob Data Contributor", "sub-a")})
	p.insertText("'sbdc")
	if pimRoleRows(&p) != 1 {
		t.Fatalf("'sbdc should fuzzy-match Storage Blob Data Contributor: %q", pimLabels(&p))
	}
}

// pimActivate marks r active for d from now; d 0 means no end time.
func pimActivate(r pim.Role, d time.Duration) pim.Role {
	r.Active = true
	if d != 0 {
		r.ActiveUntil = time.Now().Add(d)
	}
	return r
}

// pimActiveViewRoles returns, in Contoso: Contributor on sub-b active for
// 2h, Reader on rg-1 (under sub-a) active for 20m, Contributor on sub-a
// not active, Owner on mg-root whose activation has ended; in Fabrikam:
// Contributor on sub-a active with no end time and Reader on sub-c active
// for 5h.
func pimActiveViewRoles() []pim.Role {
	rg := pimTestRole("Contoso", "Reader", "sub-a")
	rg.Scope, rg.ScopeName = "/subscriptions/sub-a/resourceGroups/rg-1", "rg-1"
	mg := pimTestRole("Contoso", "Owner", "mg")
	mg.Scope, mg.ScopeName = "/providers/Microsoft.Management/managementGroups/mg-root", "mg-root"
	mg.Active, mg.ActiveUntil = true, time.Now().Add(-time.Minute)
	return []pim.Role{
		pimActivate(pimTestRole("Contoso", "Contributor", "sub-b"), 2*time.Hour),
		pimActivate(rg, 20*time.Minute),
		pimTestRole("Contoso", "Contributor", "sub-a"),
		mg,
		pimActivate(pimTestRole("Fabrikam", "Contributor", "sub-a"), 0),
		pimActivate(pimTestRole("Fabrikam", "Reader", "sub-c"), 5*time.Hour),
	}
}

func TestPIMOverlayActiveViewListsActiveRolesSoonestFirst(t *testing.T) {
	km := keymap.Default()
	var p pimOverlay
	g := p.open()
	p.setSubscription(pimSubA(), true) // ignored by the active view
	p.setTenantCount(g, 2)
	p.addRoles(g, pimActiveViewRoles())

	p.handleKey("ctrl+a", km)

	// Every tenant and subscription; per tenant, least time left first and
	// no end time last; no inactive or ended roles.
	want := []string{"CONTOSO", "Reader", "Contributor", "FABRIKAM", "Reader", "Contributor"}
	var got []string
	for _, v := range p.visible {
		if v.IsNote {
			continue
		}
		got = append(got, strings.Fields(v.Label)[0])
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("active view rows = %q, want %q", pimLabels(&p), want)
	}
	for _, row := range pimRowLabels(&p) {
		if strings.Contains(row, "mg-root") {
			t.Fatalf("ended activation listed: %q", pimRowLabels(&p))
		}
	}

	p.handleKey("ctrl+a", km)
	if got := pimRoleRows(&p); got != 3 {
		t.Fatalf("rows after switching back = %d, want the 3 eligible roles scoped to sub-a", got)
	}
}

func TestPIMOverlayActiveViewToggleKeepsQueryAndCaret(t *testing.T) {
	km := keymap.Default()
	var p pimOverlay
	g := p.open()
	p.setTenantCount(g, 2)
	p.addRoles(g, pimActiveViewRoles())
	pimType(&p, "rea")
	p.handleKey("ctrl+a", km) // ours, not the text input's start-of-line
	pimType(&p, "der")
	if p.query != "reader" {
		t.Fatalf("query = %q, want reader", p.query)
	}
	if got := pimRoleRows(&p); got != 2 {
		t.Fatalf("active readers = %d, want 2 (%q)", got, pimRowLabels(&p))
	}
	pimType(&p, " nothing")
	if !pimHasNote(&p, "No matches") {
		t.Fatalf("notes = %q, want No matches", pimLabels(&p))
	}
}

func TestPIMOverlayActiveViewIgnoresTab(t *testing.T) {
	km := keymap.Default()
	var p pimOverlay
	g := p.open()
	p.setSubscription(pimSubA(), true)
	p.setTenantCount(g, 2)
	p.addRoles(g, pimActiveViewRoles())
	p.handleKey("ctrl+a", km)
	before := pimRowLabels(&p)
	p.handleKey("tab", km)
	if after := pimRowLabels(&p); strings.Join(after, "|") != strings.Join(before, "|") {
		t.Fatalf("tab changed the active view: %q -> %q", before, after)
	}
	p.handleKey("ctrl+a", km)
	if got := pimRoleRows(&p); got != 3 {
		t.Fatalf("eligible rows = %d, want still scoped to sub-a (3)", got)
	}
}

func TestPIMOverlayActiveViewEmptyStates(t *testing.T) {
	km := keymap.Default()
	var p pimOverlay
	g := p.open()
	p.setTenantCount(g, 1)
	p.expectActive(g, 1)
	p.addRoles(g, []pim.Role{pimTestRole("Contoso", "Contributor", "sub-a")})
	p.handleKey("ctrl+a", km)
	if !pimHasNote(&p, "Checking which roles are active…") || pimHasNote(&p, "No active roles") {
		t.Fatalf("notes while checking = %q, want only the checking row", pimLabels(&p))
	}
	p.activeLoaded(g, "contoso", nil)
	if !pimHasNote(&p, "No active roles") {
		t.Fatalf("notes = %q, want No active roles", pimLabels(&p))
	}
}

func TestRenderPIMOverlayActiveViewTitleAndHints(t *testing.T) {
	km := keymap.Default()
	now := time.Now()
	var p pimOverlay
	g := p.open()
	p.setSubscription(pimSubA(), true)
	p.setTenantCount(g, 2)
	p.addRoles(g, pimActiveViewRoles())
	styles := ui.NewStyles(ui.Scheme{})

	out := ansi.Strip(renderPIMOverlay(&p, "esc", cursor.New(), styles, &km, now, 160, 40, ""))
	if !strings.Contains(out, "ctrl+a active roles") {
		t.Fatalf("eligible view missing the active-view hint:\n%s", out)
	}

	p.handleKey("ctrl+a", km)
	out = ansi.Strip(renderPIMOverlay(&p, "esc", cursor.New(), styles, &km, now, 160, 40, ""))
	for _, want := range []string{"PIM — ACTIVE ROLES", "ctrl+a eligible roles", "active 20m"} {
		if !strings.Contains(out, want) {
			t.Fatalf("active view missing %q:\n%s", want, out)
		}
	}
	for _, unwanted := range []string{"all subscriptions", "sub-a-prod"} {
		if strings.Contains(out, unwanted) {
			t.Fatalf("active view shows subscription toggle %q:\n%s", unwanted, out)
		}
	}
}
