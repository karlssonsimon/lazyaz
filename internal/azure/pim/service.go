package pim

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/karlssonsimon/lazyaz/internal/azure"
)

// api is the seam over ARM. One instance per credential.
type api interface {
	listEligible(ctx context.Context) ([]EligibleInstance, error)
	// listActive lists the caller's activations in effect now, tenant-wide.
	listActive(ctx context.Context) ([]ActiveInstance, error)
	// listActiveAt lists the caller's activations in effect at scope, from
	// ARM's own record of them.
	listActiveAt(ctx context.Context, scope string) ([]ActiveInstance, error)
	// policyRules returns the effective rules for (scope, roleDefinitionID).
	policyRules(ctx context.Context, scope, roleDefinitionID string) ([]PolicyRule, error)
	activate(ctx context.Context, req activateRequest) (status string, err error)
	principalID(ctx context.Context, tenantID string) (string, error)
}

// PolicyRule is one effective rule of a role management policy.
type PolicyRule struct {
	ID              string // e.g. "Expiration_EndUser_Assignment"
	MaximumDuration string // ISO 8601; empty for non-expiration rules
}

type activateRequest struct {
	Scope, RoleDefinitionID, EligibilityScheduleID, PrincipalID, Justification string
	Duration                                                                   time.Duration
}

// Service lists and activates PIM-eligible Azure resource roles.
type Service struct {
	newAPI func(azcore.TokenCredential) (api, error)
}

// ActivationStatus is the outcome of an accepted activation request.
type ActivationStatus int

const (
	StatusActive ActivationStatus = iota
	StatusPendingApproval
	// StatusAccepted: ARM accepted the request with a status that is
	// neither known-active, pending approval nor failed. See RawStatus.
	StatusAccepted
)

// ActivationResult describes an accepted activation request.
type ActivationResult struct {
	Status    ActivationStatus
	Until     time.Time // start + d; meaningful when StatusActive
	RawStatus string    // the status ARM returned
}

// ErrorKind classifies a failed activation so the UI can explain it.
type ErrorKind int

const (
	ErrOther ErrorKind = iota
	ErrMFARequired
	ErrTicketRequired
	ErrAlreadyActive
)

// ActivationError is returned by Activate when ARM rejects the request.
type ActivationError struct {
	Kind    ErrorKind
	Message string // ARM's message, unmodified
}

func (e *ActivationError) Error() string { return e.Message }

const endUserExpirationRuleID = "Expiration_EndUser_Assignment"

// ListEligible returns the caller's eligible roles in tenant, with no
// active state: that is a separate query (ListActive). A
// tenant without PIM yields no roles and no error. ARM is queried
// in the tenant cred is bound to, so cred must be the credential for
// tenant.ID.
func (s *Service) ListEligible(ctx context.Context, cred azcore.TokenCredential, tenant azure.Tenant) ([]Role, error) {
	a, err := s.newAPI(cred)
	if err != nil {
		return nil, err
	}
	eligible, err := a.listEligible(ctx)
	if isPIMUnavailable(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return MergeRoles(tenant.ID, TenantName(tenant), eligible, nil), nil
}

// ListActive returns the caller's activations in effect now, anywhere in
// the tenant cred is bound to. A tenant without PIM yields none and no
// error.
func (s *Service) ListActive(ctx context.Context, cred azcore.TokenCredential) ([]ActiveInstance, error) {
	a, err := s.newAPI(cred)
	if err != nil {
		return nil, err
	}
	active, err := a.listActive(ctx)
	if isPIMUnavailable(err) {
		return nil, nil
	}
	return active, err
}

// ListActiveAt returns the caller's activations in effect at scope and
// below it, plus those scope inherits from above, read from ARM's record
// of them rather than rebuilt from requests as ListActive does.
//
// It is exact where ListActive is an estimate: it has the window ARM
// really granted, which starts when ARM provisions the assignment and so,
// for a request that waited on approval, later than the request. ARM
// creates the record a few seconds after the request, so a fresh
// activation can be missing from it. It is also the listing that takes
// about a minute across a whole tenant, so it is for one subscription at
// a time. cred must be the credential for the scope's tenant.
func (s *Service) ListActiveAt(ctx context.Context, cred azcore.TokenCredential, scope string) ([]ActiveInstance, error) {
	a, err := s.newAPI(cred)
	if err != nil {
		return nil, err
	}
	active, err := a.listActiveAt(ctx, scope)
	if isPIMUnavailable(err) {
		return nil, nil
	}
	return active, err
}

// isPIMUnavailable reports whether err is ARM saying the tenant has no
// PIM: no license for it, or not onboarded to it. Such a tenant cannot hold
// eligible roles or activations, so it is a tenant with nothing to list
// rather than one that failed.
func isPIMUnavailable(err error) bool {
	var re *azcore.ResponseError
	if !errors.As(err, &re) {
		return false
	}
	switch re.ErrorCode {
	case "AadPremiumLicenseRequired", "TenantNotOnboarded":
		return true
	}
	return false
}

// TenantName is the name roles of t are tagged with: its display name,
// else its domain, else its ID.
func TenantName(t azure.Tenant) string {
	switch {
	case t.DisplayName != "":
		return t.DisplayName
	case t.Domain != "":
		return t.Domain
	default:
		return t.ID
	}
}

// MaxDuration returns 0, nil when the policy has no end-user expiration rule.
// cred must be the credential for role.TenantID.
func (s *Service) MaxDuration(ctx context.Context, cred azcore.TokenCredential, role Role) (time.Duration, error) {
	a, err := s.newAPI(cred)
	if err != nil {
		return 0, err
	}
	rules, err := a.policyRules(ctx, role.Scope, role.RoleDefinitionID)
	if err != nil {
		return 0, err
	}
	return MaxDurationFromRules(rules)
}

// MaxDurationFromRules returns the maximum activation duration set by the
// end-user assignment expiration rule, or 0 when there is none.
func MaxDurationFromRules(rules []PolicyRule) (time.Duration, error) {
	for _, r := range rules {
		if r.ID != endUserExpirationRuleID {
			continue
		}
		if r.MaximumDuration == "" {
			return 0, nil
		}
		d, err := ParseISO8601(r.MaximumDuration)
		if err != nil {
			return 0, fmt.Errorf("policy rule %s: %w", r.ID, err)
		}
		return d, nil
	}
	return 0, nil
}

// Activate self-activates role for d with reason as justification. ARM
// rejections are returned as *ActivationError. cred must be the credential
// for role.TenantID.
func (s *Service) Activate(ctx context.Context, cred azcore.TokenCredential, role Role, reason string, d time.Duration) (ActivationResult, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return ActivationResult{}, errors.New("a reason is required to activate a role")
	}
	if d < MinDuration {
		return ActivationResult{}, fmt.Errorf("duration %s is below the minimum of %s", FormatDuration(d), FormatDuration(MinDuration))
	}
	a, err := s.newAPI(cred)
	if err != nil {
		return ActivationResult{}, err
	}
	principal, err := a.principalID(ctx, role.TenantID)
	if err != nil {
		return ActivationResult{}, fmt.Errorf("resolve signed-in user: %w", err)
	}
	start := time.Now()
	status, err := a.activate(ctx, activateRequest{
		Scope:                 role.Scope,
		RoleDefinitionID:      role.RoleDefinitionID,
		EligibilityScheduleID: role.EligibilityScheduleID,
		PrincipalID:           principal,
		Justification:         reason,
		Duration:              d,
	})
	if err != nil {
		return ActivationResult{}, classifyActivationError(err)
	}
	switch status {
	case "Provisioned", "Granted", "ScheduleCreated":
		return ActivationResult{Status: StatusActive, Until: start.Add(d), RawStatus: status}, nil
	case "PendingApproval", "PendingApprovalProvisioning", "PendingAdminDecision":
		return ActivationResult{Status: StatusPendingApproval, RawStatus: status}, nil
	case "Failed", "FailedAsResourceIsLocked", "Denied", "AdminDenied", "Canceled", "Invalid", "Revoked", "TimedOut":
		return ActivationResult{}, &ActivationError{Kind: ErrOther, Message: "activation request ended with status " + status}
	}
	return ActivationResult{Status: StatusAccepted, RawStatus: status}, nil
}

// classifyActivationError maps an activation failure to an ErrorKind,
// keeping ARM's own message where one is available.
func classifyActivationError(err error) *ActivationError {
	out := &ActivationError{Kind: ErrOther, Message: err.Error()}
	var re *azcore.ResponseError
	if !errors.As(err, &re) {
		return out
	}
	if msg := armErrorMessage(re); msg != "" {
		out.Message = msg
	}
	switch re.ErrorCode {
	case "RoleAssignmentRequestAcrsValidationFailed":
		out.Kind = ErrMFARequired
	case "RoleAssignmentExists":
		out.Kind = ErrAlreadyActive
	case "RoleAssignmentRequestPolicyValidationFailed":
		switch {
		case strings.Contains(out.Message, "MfaRule"):
			out.Kind = ErrMFARequired
		case strings.Contains(out.Message, "TicketingRule"):
			out.Kind = ErrTicketRequired
		}
	}
	return out
}

// ErrorText describes err for display. For an ARM error it is
// "<code>: <message>" from the response body instead of the SDK's
// multi-line dump, which starts with the request URL. For a refused
// sign-in it is the reason and, where signing in again helps, the command.
func ErrorText(err error) string {
	var re *azcore.ResponseError
	if !errors.As(err, &re) {
		if text, ok := signInErrorText(err.Error()); ok {
			return text
		}
		return err.Error()
	}
	msg := armErrorMessage(re)
	switch {
	case re.ErrorCode != "" && msg != "":
		return re.ErrorCode + ": " + msg
	case msg != "":
		return msg
	case re.ErrorCode != "":
		return re.ErrorCode
	}
	return err.Error()
}

var (
	// An Entra sign-in error as the Azure CLI prints it: the code, then the
	// message up to the trace IDs that follow it on the same line.
	entraErrorRE = regexp.MustCompile(`AADSTS(\d+): ([^\n]*?)(?: Trace ID:|\n|$)`)
	// The tenant in the az login command the CLI suggests.
	azLoginTenantRE = regexp.MustCompile(`az login --tenant "([^"]+)"`)
)

// signInErrorText describes a credential failure caused by Entra refusing
// the sign-in, or reports false when text holds no such error.
//
// The credential's own text opens with "failed to acquire a token" and
// buries the reason several lines down, under every credential type it
// tried. The codes with a known cause get a short reason; the rest keep
// Entra's message.
func signInErrorText(text string) (string, bool) {
	m := entraErrorRE.FindStringSubmatch(text)
	if m == nil {
		return "", false
	}
	code, message := m[1], strings.TrimSpace(m[2])

	var reason string
	switch code {
	case "53003":
		// The policy refuses the token outright, so signing in again
		// does not help.
		return "blocked by a Conditional Access policy", true
	case "50076", "50079":
		reason = "multi-factor authentication required"
	case "50173", "70043", "700082":
		reason = "sign-in expired"
	default:
		reason = "AADSTS" + code + ": " + message
	}
	if t := azLoginTenantRE.FindStringSubmatch(text); t != nil {
		reason += " — run: az login --tenant " + t[1]
	}
	return reason, true
}

// armErrorMessage returns error.message from an ARM error body, or "".
func armErrorMessage(re *azcore.ResponseError) string {
	if re.RawResponse == nil {
		return ""
	}
	body, err := runtime.Payload(re.RawResponse)
	if err != nil {
		return ""
	}
	var parsed struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &parsed) != nil {
		return ""
	}
	return parsed.Error.Message
}
