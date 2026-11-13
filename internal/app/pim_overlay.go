package app

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/cursor"
	"github.com/karlssonsimon/lazyaz/internal/azure"
	"github.com/karlssonsimon/lazyaz/internal/azure/pim"
	"github.com/karlssonsimon/lazyaz/internal/fuzzy"
	"github.com/karlssonsimon/lazyaz/internal/keymap"
	"github.com/karlssonsimon/lazyaz/internal/ui"
)

// pimOverlay is the searchable list of eligible PIM roles. Its active view
// (activeOnly) lists just the roles active now, in every subscription and
// tenant, soonest to expire first.
//
// Like commandPalette, rendering and cursor work on `visible`, which
// interleaves tenant headers, role rows and trailing status rows. roleIndex
// is parallel to visible: -1 for non-selectable rows, otherwise an index
// into roles.
type pimOverlay struct {
	active     bool
	gen        int // bumped on every open; async results carry it
	query      string
	queryCaret int
	cursor     int
	loading    bool            // tenant list not yet back
	pending    int             // tenants still being queried
	failures   []pimTenantNote // tenants that errored, in arrival order
	// tenants whose roles were listed but whose active assignments could
	// not be read, in arrival order
	activeUnknown []pimTenantNote
	// Active state arrives separately from the roles. activeByTenant holds
	// each tenant's activations by lower-cased tenant ID, and is applied to
	// roles arriving later. activePending counts unanswered tenants.
	activeByTenant map[string][]pim.ActiveInstance
	activePending  int
	roles          []pim.Role
	// sub is the active tab's subscription when hasSub; the list then
	// shows only roles that can cover it, unless showAll.
	sub        azure.Subscription
	hasSub     bool
	showAll    bool
	activeOnly bool
	visible    []ui.OverlayItem
	roleIndex  []int // parallel to visible; -1 for headers/footer rows
}

// pimTenantNote is a tenant's name and a one-line error detail.
type pimTenantNote struct {
	tenant, detail string
}

// pimErrorLineMax caps error detail in the overlay footer, in runes.
const pimErrorLineMax = 120

// pimErrorLine is the first line of err's display text, capped at
// pimErrorLineMax runes.
func pimErrorLine(err error) string {
	s := pim.ErrorText(err)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > pimErrorLineMax {
		s = string(r[:pimErrorLineMax-1]) + "…"
	}
	return s
}

type pimOverlayAction int

const (
	pimActionNone pimOverlayAction = iota
	pimActionClose
	pimActionSelect
)

// open activates the overlay with empty state and returns the new
// generation, which async results must carry back.
func (p *pimOverlay) open() int {
	gen := p.gen + 1
	*p = pimOverlay{active: true, gen: gen, loading: true}
	p.refilter()
	return gen
}

func (p *pimOverlay) close() {
	p.active = false
	p.query = ""
	p.queryCaret = 0
	p.cursor = 0
	p.loading = false
	p.pending = 0
	p.failures = nil
	p.activeUnknown = nil
	p.activeByTenant = nil
	p.activePending = 0
	p.roles = nil
	p.sub, p.hasSub, p.showAll = azure.Subscription{}, false, false
	p.activeOnly = false
	p.visible = nil
	p.roleIndex = nil
}

// setSubscription scopes the list to sub, the subscription the user is
// working in. ok=false (no subscription) lists everything.
func (p *pimOverlay) setSubscription(sub azure.Subscription, ok bool) {
	p.sub, p.hasSub, p.showAll = sub, ok, false
	p.refilter()
}

// scoped reports whether the list is currently limited to p.sub. The
// active view never is.
func (p *pimOverlay) scoped() bool { return p.hasSub && !p.showAll && !p.activeOnly }

func (p *pimOverlay) subName() string {
	if p.sub.Name != "" {
		return p.sub.Name
	}
	return p.sub.ID
}

// coversSub reports whether r can grant access in p.sub: a role at the
// subscription or below it, or one above any subscription (management
// group or root) in the same tenant, which may include it.
func (p *pimOverlay) coversSub(r pim.Role) bool {
	if !strings.EqualFold(r.TenantID, p.sub.TenantID) {
		return false
	}
	scope := strings.ToLower(r.Scope)
	sub := "/subscriptions/" + strings.ToLower(p.sub.ID)
	if scope == sub || strings.HasPrefix(scope, sub+"/") {
		return true
	}
	return !strings.HasPrefix(scope, "/subscriptions/")
}

func (p *pimOverlay) accepts(gen int) bool {
	return p.active && gen == p.gen
}

func (p *pimOverlay) setTenantCount(gen, n int) {
	if !p.accepts(gen) {
		return
	}
	p.loading = false
	p.pending = n
	p.refilter()
}

func (p *pimOverlay) addRoles(gen int, roles []pim.Role) {
	if !p.accepts(gen) {
		return
	}
	p.pending = max(0, p.pending-1)
	prevKey := p.selectedKey() // before roles reorder under roleIndex
	for i := range roles {
		if active, ok := p.activeByTenant[strings.ToLower(roles[i].TenantID)]; ok {
			pim.NewActiveSet(active).Apply(&roles[i])
		}
	}
	p.roles = append(p.roles, roles...)
	sort.SliceStable(p.roles, func(i, j int) bool {
		a, b := p.roles[i], p.roles[j]
		if ta, tb := strings.ToLower(a.TenantName), strings.ToLower(b.TenantName); ta != tb {
			return ta < tb
		}
		// Keeps tenants that share a display name in separate groups.
		if a.TenantID != b.TenantID {
			return a.TenantID < b.TenantID
		}
		if sa, sb := strings.ToLower(a.ScopeName), strings.ToLower(b.ScopeName); sa != sb {
			return sa < sb
		}
		return strings.ToLower(a.RoleName) < strings.ToLower(b.RoleName)
	})
	p.refilterKeeping(prevKey)
}

// tenantFailed records that tenantName could not be queried, keeping the
// first line of err for the footer.
func (p *pimOverlay) tenantFailed(gen int, tenantName string, err error) {
	if !p.accepts(gen) {
		return
	}
	p.pending = max(0, p.pending-1)
	p.failures = append(p.failures, pimTenantNote{tenant: tenantName, detail: pimErrorLine(err)})
	p.refilter()
}

// expectActive records how many tenants' active state is coming.
func (p *pimOverlay) expectActive(gen, tenants int) {
	if !p.accepts(gen) {
		return
	}
	p.activePending = tenants
	p.refilter()
}

// activeLoaded records tenantID's activations and re-marks its roles.
func (p *pimOverlay) activeLoaded(gen int, tenantID string, active []pim.ActiveInstance) {
	if !p.accepts(gen) {
		return
	}
	p.activePending = max(0, p.activePending-1)
	if p.activeByTenant == nil {
		p.activeByTenant = map[string][]pim.ActiveInstance{}
	}
	key := strings.ToLower(tenantID)
	p.activeByTenant[key] = active
	set := pim.NewActiveSet(active)
	for i := range p.roles {
		if strings.EqualFold(p.roles[i].TenantID, tenantID) {
			set.Apply(&p.roles[i])
		}
	}
	p.refilter()
}

// checkingActive reports whether some tenant's active state is still
// unknown, so rows may lack their active marker.
func (p *pimOverlay) checkingActive() bool { return p.activePending > 0 }

// activeStateUnknown records that tenantName's activations could not be
// read. Its roles stay listed without active markers.
func (p *pimOverlay) activeStateUnknown(gen int, tenantName string, err error) {
	if !p.accepts(gen) {
		return
	}
	p.activePending = max(0, p.activePending-1)
	p.activeUnknown = append(p.activeUnknown, pimTenantNote{tenant: tenantName, detail: pimErrorLine(err)})
	p.refilter()
}

func (p *pimOverlay) insertText(text string) {
	ti := ui.TextInput{Value: p.query, Cursor: p.queryCaret}
	ti.Insert(text)
	p.query = ti.Value
	p.queryCaret = ti.Cursor
	p.refilter()
}

func (p *pimOverlay) selected() (pim.Role, bool) {
	if p.cursor < 0 || p.cursor >= len(p.roleIndex) {
		return pim.Role{}, false
	}
	idx := p.roleIndex[p.cursor]
	if idx < 0 {
		return pim.Role{}, false
	}
	return p.roles[idx], true
}

// refilter rebuilds visible/roleIndex from roles and the query, keeping the
// cursor on the same role when it is still visible and otherwise on the
// first selectable row.
func (p *pimOverlay) refilter() {
	p.refilterKeeping(p.selectedKey())
}

func (p *pimOverlay) selectedKey() string {
	if r, ok := p.selected(); ok {
		return r.Key()
	}
	return ""
}

func (p *pimOverlay) refilterKeeping(prevKey string) {
	now := time.Now()
	// candidates are indices into roles that the view allows, in display
	// order.
	candidates := make([]int, 0, len(p.roles))
	for i, r := range p.roles {
		switch {
		case p.activeOnly:
			if r.IsActive(now) {
				candidates = append(candidates, i)
			}
		case !p.scoped() || p.coversSub(r):
			candidates = append(candidates, i)
		}
	}
	// Exact mode: with role and scope names this long, fuzzy single letters
	// match almost every row ("rbac" hits R…b…a…c across two fields).
	hits := fuzzy.FilterExact(p.query, candidates, func(i int) string {
		r := p.roles[i]
		return r.RoleName + " " + r.ScopeName + " " + r.TenantName
	})
	matched := make([]int, len(hits))
	for n, h := range hits {
		matched[n] = candidates[h]
	}
	// roles is already in display order; fuzzy returns best-score first.
	sort.Ints(matched)
	if p.activeOnly {
		// Within each tenant, least time left first; no end time last.
		sort.SliceStable(matched, func(i, j int) bool {
			a, b := p.roles[matched[i]], p.roles[matched[j]]
			if a.TenantID != b.TenantID {
				return matched[i] < matched[j] // roles keeps tenants grouped
			}
			switch {
			case a.ActiveUntil.IsZero():
				return false
			case b.ActiveUntil.IsZero():
				return true
			}
			return a.ActiveUntil.Before(b.ActiveUntil)
		})
	}

	tenants := map[string]bool{}
	idsByName := map[string]map[string]bool{}
	for _, i := range candidates {
		r := p.roles[i]
		tenants[r.TenantID] = true
		name := strings.ToLower(r.TenantName)
		if idsByName[name] == nil {
			idsByName[name] = map[string]bool{}
		}
		idsByName[name][r.TenantID] = true
	}
	showHeaders := len(tenants) > 1
	header := func(r pim.Role) string {
		label := strings.ToUpper(r.TenantName)
		if len(idsByName[strings.ToLower(r.TenantName)]) > 1 {
			id := r.TenantID
			if len(id) > 8 {
				id = id[:8]
			}
			label += " (" + id + ")"
		}
		return label
	}

	nameW := 0
	for _, idx := range matched {
		nameW = max(nameW, len([]rune(p.roles[idx].RoleName)))
	}

	p.visible = p.visible[:0]
	p.roleIndex = p.roleIndex[:0]

	lastTenant := ""
	for n, idx := range matched {
		r := p.roles[idx]
		if showHeaders && (n == 0 || r.TenantID != lastTenant) {
			p.visible = append(p.visible, ui.OverlayItem{Label: header(r), IsHeader: true})
			p.roleIndex = append(p.roleIndex, -1)
		}
		lastTenant = r.TenantID
		pad := strings.Repeat(" ", nameW-len([]rune(r.RoleName)))
		p.visible = append(p.visible, ui.OverlayItem{
			Label: r.RoleName + pad + "  " + r.ScopeName,
			Hint:  pimRowHint(r, now),
		})
		p.roleIndex = append(p.roleIndex, idx)
	}

	footer := func(label string) {
		p.visible = append(p.visible, ui.OverlayItem{Label: label, IsNote: true})
		p.roleIndex = append(p.roleIndex, -1)
	}
	if p.loading || p.pending > 0 {
		footer("Loading…")
	} else if (len(p.roles) > 0 || p.activeOnly) && p.checkingActive() {
		footer("Checking which roles are active…")
	}
	if n := len(p.failures); n > 0 {
		count := "1 tenant"
		if n > 1 {
			count = fmt.Sprintf("%d tenants", n)
		}
		f := p.failures[0]
		footer(fmt.Sprintf("%s could not be queried — %s: %s", count, f.tenant, f.detail))
	}
	if len(p.activeUnknown) > 0 {
		u := p.activeUnknown[0]
		footer("Active state unknown for " + u.tenant + ": " + u.detail)
	}
	settled := !p.loading && p.pending == 0
	switch {
	case p.activeOnly && settled && !p.checkingActive() && len(candidates) == 0:
		footer("No active roles")
	case p.activeOnly:
		if len(candidates) > 0 && len(matched) == 0 {
			footer("No matches")
		}
	case settled && len(p.failures) == 0 && len(p.roles) == 0:
		footer("No eligible roles")
	case p.scoped() && settled && len(p.roles) > 0 && len(candidates) == 0:
		footer("No eligible roles in " + p.subName() + " — tab shows all subscriptions")
	case p.scoped() && len(candidates) > 0 && len(matched) == 0:
		footer("No matches in " + p.subName() + " — tab searches all subscriptions")
	case len(candidates) > 0 && len(matched) == 0:
		footer("No matches")
	}

	p.cursor = p.firstSelectable()
	if prevKey != "" {
		for i, idx := range p.roleIndex {
			if idx >= 0 && p.roles[idx].Key() == prevKey {
				p.cursor = i
				break
			}
		}
	}
}

func (p *pimOverlay) firstSelectable() int {
	for i, idx := range p.roleIndex {
		if idx >= 0 {
			return i
		}
	}
	return 0
}

// moveCursor advances by delta, skipping non-selectable rows.
func (p *pimOverlay) moveCursor(delta int) {
	for next := p.cursor + delta; next >= 0 && next < len(p.roleIndex); next += delta {
		if p.roleIndex[next] >= 0 {
			p.cursor = next
			return
		}
	}
}

func (p *pimOverlay) handleKey(key string, km keymap.Keymap) pimOverlayAction {
	switch {
	case km.Cancel.Matches(key):
		if p.query != "" {
			p.query = ""
			p.queryCaret = 0
			p.refilter()
			return pimActionNone
		}
		return pimActionClose
	case km.ThemeUp.Matches(key):
		p.moveCursor(-1)
		return pimActionNone
	case km.ThemeDown.Matches(key):
		p.moveCursor(1)
		return pimActionNone
	case km.PIMToggleActive.Matches(key):
		p.activeOnly = !p.activeOnly
		p.refilter()
		return pimActionNone
	case km.NextFocus.Matches(key):
		if p.hasSub && !p.activeOnly {
			p.showAll = !p.showAll
			p.refilter()
		}
		return pimActionNone
	case km.OpenFocused.Matches(key):
		if _, ok := p.selected(); ok {
			return pimActionSelect
		}
		return pimActionNone
	case key == "ctrl+v":
		if text := ui.ReadClipboard(); text != "" {
			p.insertText(text)
		}
		return pimActionNone
	}
	ti := ui.TextInput{Value: p.query, Cursor: p.queryCaret}
	if ti.HandleKey(key) {
		changed := ti.Value != p.query
		p.query = ti.Value
		p.queryCaret = ti.Cursor
		if changed {
			p.refilter()
		}
	}
	return pimActionNone
}

// pimRowHint is "active 1h42m" for a role that is active at now, "active
// <1m" when under a minute is left, "active" when it has no end time, and
// "" otherwise.
func pimRowHint(r pim.Role, now time.Time) string {
	if !r.IsActive(now) {
		return ""
	}
	if r.ActiveUntil.IsZero() {
		return "active"
	}
	left := r.ActiveUntil.Sub(now).Truncate(time.Minute)
	if left <= 0 {
		return "active <1m"
	}
	return "active " + pim.FormatDuration(left)
}

func renderPIMOverlay(p *pimOverlay, closeHint string, cur cursor.Model, styles ui.Styles, km *keymap.Keymap, now time.Time, width, height int, base string) string {
	// Hints are computed against the caller's clock, not the one used at
	// filter time, so rows count down between refilters.
	items := make([]ui.OverlayItem, len(p.visible))
	copy(items, p.visible)
	for i, idx := range p.roleIndex {
		if idx >= 0 {
			items[i].Hint = pimRowHint(p.roles[idx], now)
		}
	}
	// Unlike the other pickers this list can hold hundreds of rows with
	// long scope names, so it takes most of the terminal.
	innerW := min(max(width-16, 60), 160)
	maxVis := min(max(height-height/5-10, 8), 60)

	// The header names what is listed; the footer names what tab and the
	// active-view key switch to.
	title := "PIM — eligible roles"
	var extra []ui.StatusAction
	switch {
	case p.activeOnly:
		title = "PIM — active roles"
		extra = append(extra, ui.StatusAction{Key: km.PIMToggleActive.Short(), Label: "eligible roles"})
	default:
		if p.hasSub {
			showing, other := p.subName(), "all subscriptions"
			if p.showAll {
				showing, other = "all subscriptions", "only "+p.subName()
			}
			closeHint = showing + "  " + closeHint
			extra = append(extra, ui.StatusAction{Key: km.NextFocus.Short(), Label: other})
		}
		extra = append(extra, ui.StatusAction{Key: km.PIMToggleActive.Short(), Label: "active roles"})
	}
	cfg := ui.OverlayListConfig{
		Title:          title,
		Query:          p.query,
		QueryCursor:    p.queryCaret,
		Cursor:         cur,
		CloseHint:      closeHint,
		ExactMatch:     true,
		InnerWidth:     innerW,
		MaxVisible:     maxVis,
		ExtraActions:   extra,
		NoActiveMarker: true,
		Bindings: &ui.OverlayBindings{
			MoveUp:   km.ThemeUp,
			MoveDown: km.ThemeDown,
			Apply:    km.OpenFocused,
			Cancel:   km.Cancel,
			Erase:    km.BackspaceUp,
		},
	}
	return ui.RenderOverlayList(cfg, items, p.cursor, styles, width, height, base)
}
