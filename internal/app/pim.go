package app

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/karlssonsimon/lazyaz/internal/appshell"
	"github.com/karlssonsimon/lazyaz/internal/azure"
	"github.com/karlssonsimon/lazyaz/internal/azure/pim"
	"github.com/karlssonsimon/lazyaz/internal/ui"

	tea "charm.land/bubbletea/v2"
)

// pimService is the part of *pim.Service the app uses. Every call must
// get the credential for the tenant it targets, never the default one.
type pimService interface {
	ListEligible(ctx context.Context, cred azcore.TokenCredential, tenant azure.Tenant) ([]pim.Role, error)
	ListActive(ctx context.Context, cred azcore.TokenCredential) ([]pim.ActiveInstance, error)
	MaxDuration(ctx context.Context, cred azcore.TokenCredential, role pim.Role) (time.Duration, error)
	Activate(ctx context.Context, cred azcore.TokenCredential, role pim.Role, reason string, d time.Duration) (pim.ActivationResult, error)
}

const pimPortalURL = "https://portal.azure.com/#view/Microsoft_Azure_PIMCommon/ActivationMenuBlade/~/azurerbac"

// pimFallbackDuration prefills the form when neither a last-used value
// nor pim.default_duration parses.
const pimFallbackDuration = time.Hour

// pimFetchTimeout bounds every PIM call to ARM.
const pimFetchTimeout = 60 * time.Second

// Field order in m.pimForm.
const (
	pimFieldReason = iota
	pimFieldDuration
)

// pimDurationShapeRe mirrors pim.ParseDuration's accepted shape so the
// validator can tell a too-short duration from an unparsable one.
var pimDurationShapeRe = regexp.MustCompile(`^(\d+h)?(\d+m)?$`)

// defaultPIMTenants lists the signed-in user's tenants with the default
// (multi-tenant) credential. This is the only PIM path that may use it.
func defaultPIMTenants(ctx context.Context) ([]azure.Tenant, error) {
	cred, err := azure.NewDefaultCredential()
	if err != nil {
		return nil, err
	}
	return azure.ListTenants(ctx, cred)
}

func pimDurationPrefKey(r pim.Role) string {
	return "pim.duration." + r.Key()
}

// resolvePIMDuration picks the prefill: last used for the role, then the
// configured default, then pimFallbackDuration, clamped to max when max
// is known (> 0).
func resolvePIMDuration(last string, hasLast bool, cfgDefault string, max time.Duration) time.Duration {
	d := pimFallbackDuration
	if v, err := pim.ParseDuration(last); hasLast && err == nil {
		d = v
	} else if v, err := pim.ParseDuration(cfgDefault); err == nil {
		d = v
	}
	if max > 0 && d > max {
		return max
	}
	return d
}

// pimDurationValidator returns a form validator; max 0 means unknown.
func pimDurationValidator(max time.Duration) func(string) string {
	return func(value string) string {
		d, err := pim.ParseDuration(value)
		if err != nil {
			s := strings.ToLower(strings.TrimSpace(value))
			if raw, perr := time.ParseDuration(s); perr == nil && pimDurationShapeRe.MatchString(s) && raw < pim.MinDuration {
				return "minimum is " + pim.FormatDuration(pim.MinDuration)
			}
			return "use e.g. 30m, 2h or 1h30m"
		}
		if max > 0 && d > max {
			return "maximum for this role is " + pim.FormatDuration(max)
		}
		return ""
	}
}

func pimReasonValidator(value string) string {
	if strings.TrimSpace(value) == "" {
		return "a reason is required"
	}
	return ""
}

// pimDurationHint builds the Duration field hint. maxUnknown means the
// policy lookup failed; max is then ignored.
func pimDurationHint(fromLast bool, max time.Duration, maxUnknown bool) string {
	var parts []string
	if fromLast {
		parts = append(parts, "last used")
	}
	switch {
	case maxUnknown:
		parts = append(parts, "max unknown")
	case max > 0:
		parts = append(parts, "max "+pim.FormatDuration(max))
	}
	return strings.Join(parts, " · ")
}

// openPIMOverlay opens the role list and starts listing tenants.
func (m *Model) openPIMOverlay() tea.Cmd {
	gen := m.pim.open()
	m.pim.setSubscription(m.activeSubscription())
	list := m.pimTenants
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), pimFetchTimeout)
		defer cancel()
		tenants, err := list(ctx)
		return pimTenantsLoadedMsg{gen: gen, tenants: tenants, err: err}
	}
}

func (m *Model) handlePIMTenantsLoaded(msg pimTenantsLoadedMsg) tea.Cmd {
	if !m.pim.accepts(msg.gen) {
		return nil
	}
	if msg.err != nil {
		m.pim.close()
		m.notifier.Push(appshell.LevelError, "PIM: could not list tenants: "+msg.err.Error())
		return m.maybeStartToastTick()
	}
	m.pim.setTenantCount(msg.gen, len(msg.tenants))
	// Per tenant: eligible roles and activations as separate cmds, so a
	// slow or failing active-state query never holds back the list.
	cmds := make([]tea.Cmd, 0, 2*len(msg.tenants))
	for _, t := range msg.tenants {
		cmds = append(cmds, m.pimListEligibleCmd(msg.gen, t))
		cmds = append(cmds, m.pimListActiveCmd(msg.gen, t))
	}
	m.pim.expectActive(msg.gen, len(msg.tenants))
	return tea.Batch(cmds...)
}

// pimListEligibleCmd lists tenant's roles with that tenant's credential. A
// credential error counts as the tenant failing.
func (m *Model) pimListEligibleCmd(gen int, tenant azure.Tenant) tea.Cmd {
	svc, creds := m.pimSvc, m.credCache
	return func() tea.Msg {
		msg := pimRolesLoadedMsg{gen: gen, tenantName: pim.TenantName(tenant)}
		cred, err := creds.For(tenant.ID)
		if err != nil {
			msg.err = err
			return msg
		}
		ctx, cancel := context.WithTimeout(context.Background(), pimFetchTimeout)
		defer cancel()
		msg.roles, msg.err = svc.ListEligible(ctx, cred, tenant)
		return msg
	}
}

// pimListActiveCmd lists tenant's activations with that tenant's
// credential.
func (m *Model) pimListActiveCmd(gen int, tenant azure.Tenant) tea.Cmd {
	svc, creds := m.pimSvc, m.credCache
	return func() tea.Msg {
		msg := pimActiveLoadedMsg{gen: gen, tenantID: tenant.ID, tenantName: pim.TenantName(tenant)}
		cred, err := creds.For(tenant.ID)
		if err != nil {
			msg.err = err
			return msg
		}
		ctx, cancel := context.WithTimeout(context.Background(), pimFetchTimeout)
		defer cancel()
		msg.active, msg.err = svc.ListActive(ctx, cred)
		return msg
	}
}

func (m *Model) handlePIMRolesLoaded(msg pimRolesLoadedMsg) {
	if msg.err != nil {
		m.pim.tenantFailed(msg.gen, msg.tenantName, msg.err)
		return
	}
	m.pim.addRoles(msg.gen, msg.roles)
}

func (m *Model) handlePIMActiveLoaded(msg pimActiveLoadedMsg) {
	if msg.err != nil {
		m.pim.activeStateUnknown(msg.gen, msg.tenantName, msg.err)
		return
	}
	m.pim.activeLoaded(msg.gen, msg.tenantID, msg.active)
}

func (m *Model) handlePIMListKey(key string) tea.Cmd {
	switch m.pim.handleKey(key, m.keymap) {
	case pimActionClose:
		m.pim.close()
	case pimActionSelect:
		return m.selectPIMRole()
	}
	return nil
}

// selectPIMRole opens the activation form for the selected role, or
// reports that it is already active. The list stays open underneath.
func (m *Model) selectPIMRole() tea.Cmd {
	role, ok := m.pim.selected()
	if !ok {
		return nil
	}
	if role.IsActive(time.Now()) {
		text := role.Label() + " is already active"
		if left := time.Until(role.ActiveUntil).Truncate(time.Minute); left > 0 {
			text += " (" + pim.FormatDuration(left) + " left)"
		}
		m.notifier.Push(appshell.LevelInfo, text)
		return m.maybeStartToastTick()
	}

	last, hasLast := m.db.GetPreference(pimDurationPrefKey(role))
	_, lastErr := pim.ParseDuration(last)
	fromLast := hasLast && lastErr == nil
	prefill := pim.FormatDuration(resolvePIMDuration(last, hasLast, m.cfg.PIM.DefaultDuration, 0))

	m.pimForm.Open("Activate "+role.Label(), []ui.FormField{
		pimFieldReason: {
			Label:       "Reason",
			Placeholder: "Why you need this role",
			Validate:    pimReasonValidator,
		},
		pimFieldDuration: {
			Label:    "Duration",
			Value:    prefill,
			Validate: pimDurationValidator(0),
			Hint:     pimDurationHint(fromLast, 0, false),
			Help:     "e.g. 30m, 2h, 1h30m",
		},
	})
	m.pimFormRole = role
	m.pimFormPrefill = prefill
	m.pimFormFromLast = fromLast
	m.pimFormMax = 0
	return m.pimMaxDurationCmd(m.pim.gen, role)
}

// pimMaxDurationCmd looks up the role's policy maximum with the role's
// tenant credential.
func (m *Model) pimMaxDurationCmd(gen int, role pim.Role) tea.Cmd {
	svc, creds := m.pimSvc, m.credCache
	return func() tea.Msg {
		msg := pimPolicyLoadedMsg{gen: gen, key: role.Key()}
		cred, err := creds.For(role.TenantID)
		if err != nil {
			msg.err = err
			return msg
		}
		ctx, cancel := context.WithTimeout(context.Background(), pimFetchTimeout)
		defer cancel()
		msg.max, msg.err = svc.MaxDuration(ctx, cred, role)
		return msg
	}
}

// handlePIMPolicyLoaded applies a known policy maximum to the open form.
// The duration is clamped only while the user has not edited it. A failed
// lookup only marks the maximum as unknown in the hint.
func (m *Model) handlePIMPolicyLoaded(msg pimPolicyLoadedMsg) {
	if !m.pimForm.Active || msg.gen != m.pim.gen || msg.key != m.pimFormRole.Key() {
		return
	}
	f := &m.pimForm.Fields[pimFieldDuration]
	if msg.err != nil {
		f.Hint = pimDurationHint(m.pimFormFromLast, 0, true)
		return
	}
	if msg.max <= 0 {
		return
	}
	m.pimFormMax = msg.max
	f.Validate = pimDurationValidator(msg.max)
	f.Hint = pimDurationHint(m.pimFormFromLast, msg.max, false)
	if f.Value != m.pimFormPrefill {
		return
	}
	clamped := pim.FormatDuration(resolvePIMDuration(f.Value, true, m.cfg.PIM.DefaultDuration, msg.max))
	f.Value = clamped
	f.Cursor = len([]rune(clamped))
	m.pimFormPrefill = clamped
}

func (m *Model) handlePIMFormKey(key string) tea.Cmd {
	if key == "ctrl+v" {
		if text := ui.ReadClipboard(); text != "" {
			if f := m.pimForm.FocusedField(); f != nil {
				f.Insert(text)
			}
		}
		return nil
	}
	res := m.pimForm.HandleKey(key)
	if res.Action != ui.FormActionSubmit {
		// Cancel closes the form; the list underneath stays open.
		return nil
	}
	role := m.pimFormRole
	d, err := pim.ParseDuration(res.Values[pimFieldDuration])
	if err != nil {
		// The validator rejects this before submit; kept as a guard.
		m.notifier.Push(appshell.LevelError, "PIM: "+err.Error())
		return m.maybeStartToastTick()
	}
	m.pim.close()
	id := m.notifier.PushSpinner("Activating " + role.Label() + "…")
	return tea.Batch(m.pimActivateCmd(role, res.Values[pimFieldReason], d, id), m.maybeStartToastTick())
}

// pimActivateCmd activates role with the role's tenant credential. The
// reason goes to ARM only; it is never stored or shown.
func (m *Model) pimActivateCmd(role pim.Role, reason string, d time.Duration, spinnerID int) tea.Cmd {
	svc, creds := m.pimSvc, m.credCache
	return func() tea.Msg {
		msg := pimActivatedMsg{role: role, duration: d, spinnerID: spinnerID}
		cred, err := creds.For(role.TenantID)
		if err != nil {
			msg.err = err
			return msg
		}
		ctx, cancel := context.WithTimeout(context.Background(), pimFetchTimeout)
		defer cancel()
		msg.result, msg.err = svc.Activate(ctx, cred, role, reason, d)
		return msg
	}
}

// handlePIMActivated resolves the activation spinner. It is not gated on
// the overlay: the result arrives after the overlay has closed.
func (m *Model) handlePIMActivated(msg pimActivatedMsg) tea.Cmd {
	label := msg.role.Label()
	kind, armMsg := pim.ErrOther, ""
	if aerr := (*pim.ActivationError)(nil); errors.As(msg.err, &aerr) {
		kind = aerr.Kind
		if aerr.Message != "" {
			armMsg = " (" + aerr.Message + ")"
		}
	}
	level, text := appshell.LevelError, ""
	switch {
	case msg.err == nil && msg.result.Status == pim.StatusPendingApproval:
		m.savePIMDuration(msg.role, msg.duration)
		level, text = appshell.LevelInfo, label+": activation requested, pending approval"
	case msg.err == nil && msg.result.Status == pim.StatusAccepted:
		m.savePIMDuration(msg.role, msg.duration)
		level, text = appshell.LevelInfo, label+": request accepted (status "+msg.result.RawStatus+")"
	case msg.err == nil:
		m.savePIMDuration(msg.role, msg.duration)
		level = appshell.LevelSuccess
		text = fmt.Sprintf("%s active until %s — may take a few minutes to take effect", label, msg.result.Until.Local().Format("15:04"))
	case kind == pim.ErrMFARequired:
		text = label + " requires MFA and must be activated in the portal: " + pimPortalURL + armMsg
	case kind == pim.ErrTicketRequired:
		text = label + " requires ticket information and must be activated in the portal: " + pimPortalURL + armMsg
	case kind == pim.ErrAlreadyActive:
		level, text = appshell.LevelInfo, label+" is already active"
	default:
		text = label + ": activation failed: " + msg.err.Error()
	}
	m.notifier.ResolveSpinner(msg.spinnerID, level, text)
	return m.maybeStartToastTick()
}

func (m *Model) savePIMDuration(role pim.Role, d time.Duration) {
	if m.db == nil {
		return
	}
	m.db.SetPreference(pimDurationPrefKey(role), pim.FormatDuration(d))
}
