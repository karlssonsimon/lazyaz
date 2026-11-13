package pim

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/karlssonsimon/lazyaz/internal/azure"
)

type fakeAPI struct {
	eligible    []EligibleInstance
	eligibleErr error
	active      []ActiveInstance
	activeErr   error
	activeCalls int

	rules         []PolicyRule
	rulesErr      error
	rulesScope    string
	rulesRoleDefn string

	principal       string
	principalErr    error
	principalTenant string

	status        string
	activateErr   error
	activateCalls int
	activated     activateRequest
}

func (f *fakeAPI) listEligible(context.Context) ([]EligibleInstance, error) {
	return f.eligible, f.eligibleErr
}

func (f *fakeAPI) listActive(context.Context) ([]ActiveInstance, error) {
	f.activeCalls++
	return f.active, f.activeErr
}

func (f *fakeAPI) policyRules(_ context.Context, scope, roleDefinitionID string) ([]PolicyRule, error) {
	f.rulesScope, f.rulesRoleDefn = scope, roleDefinitionID
	return f.rules, f.rulesErr
}

func (f *fakeAPI) activate(_ context.Context, req activateRequest) (string, error) {
	f.activateCalls++
	f.activated = req
	return f.status, f.activateErr
}

func (f *fakeAPI) principalID(_ context.Context, tenantID string) (string, error) {
	f.principalTenant = tenantID
	return f.principal, f.principalErr
}

func newFakeService(fake *fakeAPI) *Service {
	return &Service{newAPI: func(azcore.TokenCredential) (api, error) { return fake, nil }}
}

var testRole = Role{
	Scope:                 "/subscriptions/1",
	RoleDefinitionID:      "rd",
	EligibilityScheduleID: "es",
	TenantID:              "t1",
}

func TestListEligibleTagsTenantAndLeavesActiveUnset(t *testing.T) {
	fake := &fakeAPI{
		eligible: []EligibleInstance{
			{Scope: "/subscriptions/1", ScopeDisplayName: "Prod", RoleDefinitionID: "rd-reader", RoleName: "Reader", EligibilityScheduleID: "es1"},
			{Scope: "/subscriptions/2", ScopeDisplayName: "Dev", RoleDefinitionID: "rd-owner", RoleName: "Owner", EligibilityScheduleID: "es2"},
		},
	}
	roles, err := newFakeService(fake).ListEligible(context.Background(), nil, azure.Tenant{ID: "t1", DisplayName: "Contoso"})
	if err != nil {
		t.Fatalf("ListEligible: %v", err)
	}
	if len(roles) != 2 {
		t.Fatalf("got %d roles, want 2", len(roles))
	}
	for _, r := range roles {
		if r.TenantID != "t1" || r.TenantName != "Contoso" {
			t.Errorf("role %q tenant = %q/%q, want t1/Contoso", r.RoleName, r.TenantID, r.TenantName)
		}
		if r.Active {
			t.Errorf("role %q marked active; ListEligible must not query active state", r.RoleName)
		}
	}
	if fake.activeCalls != 0 {
		t.Errorf("listActive called %d times by ListEligible, want 0", fake.activeCalls)
	}
}

func TestListEligibleTenantNameFallsBackToDomainThenID(t *testing.T) {
	fake := &fakeAPI{eligible: []EligibleInstance{{Scope: "/subscriptions/1", RoleDefinitionID: "rd", RoleName: "Reader"}}}
	svc := newFakeService(fake)

	roles, err := svc.ListEligible(context.Background(), nil, azure.Tenant{ID: "t1", Domain: "contoso.com"})
	if err != nil {
		t.Fatalf("ListEligible: %v", err)
	}
	if roles[0].TenantName != "contoso.com" {
		t.Errorf("TenantName = %q, want contoso.com", roles[0].TenantName)
	}

	roles, err = svc.ListEligible(context.Background(), nil, azure.Tenant{ID: "t1"})
	if err != nil {
		t.Fatalf("ListEligible: %v", err)
	}
	if roles[0].TenantName != "t1" {
		t.Errorf("TenantName = %q, want t1", roles[0].TenantName)
	}
}

func TestListEligiblePropagatesError(t *testing.T) {
	boom := errors.New("boom")
	fake := &fakeAPI{eligibleErr: boom}
	_, err := newFakeService(fake).ListEligible(context.Background(), nil, azure.Tenant{ID: "t1"})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
}

// pimUnavailableCodes are the ARM error codes for a tenant without PIM.
var pimUnavailableCodes = []string{"AadPremiumLicenseRequired", "TenantNotOnboarded"}

func TestListEligibleTreatsTenantWithoutPIMAsHavingNoRoles(t *testing.T) {
	for _, code := range pimUnavailableCodes {
		fake := &fakeAPI{eligibleErr: armError(code, "no PIM in this tenant")}
		roles, err := newFakeService(fake).ListEligible(context.Background(), nil, azure.Tenant{ID: "t1"})
		if err != nil || len(roles) != 0 {
			t.Errorf("%s: roles = %v, err = %v; want none, nil", code, roles, err)
		}
	}
}

func TestListActiveReturnsActivations(t *testing.T) {
	want := []ActiveInstance{{Scope: "/subscriptions/1", RoleDefinitionID: "rd"}}
	fake := &fakeAPI{active: want}
	got, err := newFakeService(fake).ListActive(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	if len(got) != 1 || got[0] != want[0] {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestListActiveTenantWithoutPIMIsEmpty(t *testing.T) {
	for _, code := range pimUnavailableCodes {
		fake := &fakeAPI{activeErr: armError(code, "no PIM in this tenant")}
		got, err := newFakeService(fake).ListActive(context.Background(), nil)
		if err != nil || len(got) != 0 {
			t.Errorf("%s: got %v, %v; want none, nil", code, got, err)
		}
	}
}

func TestListActivePropagatesError(t *testing.T) {
	forbidden := errors.New("forbidden")
	fake := &fakeAPI{activeErr: forbidden}
	if _, err := newFakeService(fake).ListActive(context.Background(), nil); !errors.Is(err, forbidden) {
		t.Fatalf("err = %v, want %v", err, forbidden)
	}
}

func TestActiveSetApply(t *testing.T) {
	end := time.Date(2026, 11, 13, 15, 0, 0, 0, time.UTC)
	set := NewActiveSet([]ActiveInstance{
		{Scope: "/Subscriptions/1", RoleDefinitionID: "/providers/x/roleDefinitions/RD", EndTime: end},
	})
	on := Role{Scope: "/subscriptions/1", RoleDefinitionID: "/subscriptions/1/providers/x/roleDefinitions/rd"}
	set.Apply(&on)
	if !on.Active || !on.ActiveUntil.Equal(end) {
		t.Errorf("matching role = %+v, want active until %v", on, end)
	}
	stale := Role{Scope: "/subscriptions/2", RoleDefinitionID: "rd", Active: true, ActiveUntil: end}
	set.Apply(&stale)
	if stale.Active || !stale.ActiveUntil.IsZero() {
		t.Errorf("non-matching role = %+v, want active state cleared", stale)
	}
}

func TestMaxDurationFromRules(t *testing.T) {
	tests := []struct {
		name    string
		rules   []PolicyRule
		want    time.Duration
		wantErr bool
	}{
		{"end user rule", []PolicyRule{{ID: "Expiration_EndUser_Assignment", MaximumDuration: "PT8H"}}, 8 * time.Hour, false},
		{"no end user rule", []PolicyRule{{ID: "Expiration_Admin_Assignment", MaximumDuration: "P180D"}}, 0, false},
		{"empty maximum", []PolicyRule{{ID: "Expiration_EndUser_Assignment"}}, 0, false},
		{"garbage maximum", []PolicyRule{{ID: "Expiration_EndUser_Assignment", MaximumDuration: "garbage"}}, 0, true},
		{"admin eligibility ignored", []PolicyRule{
			{ID: "Expiration_Admin_Eligibility", MaximumDuration: "P365D"},
			{ID: "Expiration_EndUser_Assignment", MaximumDuration: "PT2H"},
		}, 2 * time.Hour, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := MaxDurationFromRules(tt.rules)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMaxDurationQueriesRoleScopeAndDefinition(t *testing.T) {
	fake := &fakeAPI{rules: []PolicyRule{{ID: "Expiration_EndUser_Assignment", MaximumDuration: "PT4H"}}}
	got, err := newFakeService(fake).MaxDuration(context.Background(), nil, testRole)
	if err != nil {
		t.Fatalf("MaxDuration: %v", err)
	}
	if got != 4*time.Hour {
		t.Errorf("got %v, want 4h", got)
	}
	if fake.rulesScope != "/subscriptions/1" || fake.rulesRoleDefn != "rd" {
		t.Errorf("policyRules called with (%q, %q), want (/subscriptions/1, rd)", fake.rulesScope, fake.rulesRoleDefn)
	}
}

func TestActivateSendsSelfActivateRequest(t *testing.T) {
	fake := &fakeAPI{principal: "user-oid", status: "Provisioned"}
	if _, err := newFakeService(fake).Activate(context.Background(), nil, testRole, "Felsökning", 2*time.Hour); err != nil {
		t.Fatalf("Activate: %v", err)
	}
	want := activateRequest{
		Scope:                 "/subscriptions/1",
		RoleDefinitionID:      "rd",
		EligibilityScheduleID: "es",
		PrincipalID:           "user-oid",
		Justification:         "Felsökning",
		Duration:              2 * time.Hour,
	}
	if fake.activated != want {
		t.Errorf("activate request = %+v, want %+v", fake.activated, want)
	}
	if fake.principalTenant != "t1" {
		t.Errorf("principalID tenant = %q, want t1", fake.principalTenant)
	}
}

func TestActivateStatusMapping(t *testing.T) {
	tests := []struct {
		status string
		want   ActivationStatus
	}{
		{"Provisioned", StatusActive},
		{"Granted", StatusActive},
		{"ScheduleCreated", StatusActive},
		{"PendingApproval", StatusPendingApproval},
		{"PendingApprovalProvisioning", StatusPendingApproval},
		{"PendingAdminDecision", StatusPendingApproval},
		{"Accepted", StatusAccepted},
		{"PendingProvisioning", StatusAccepted},
		{"SomethingNew", StatusAccepted},
	}
	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			fake := &fakeAPI{principal: "user-oid", status: tt.status}
			got, err := newFakeService(fake).Activate(context.Background(), nil, testRole, "reason", 2*time.Hour)
			if err != nil {
				t.Fatalf("Activate: %v", err)
			}
			if got.Status != tt.want {
				t.Errorf("Status = %v, want %v", got.Status, tt.want)
			}
			if got.RawStatus != tt.status {
				t.Errorf("RawStatus = %q, want %q", got.RawStatus, tt.status)
			}
			if tt.want == StatusActive {
				if diff := got.Until.Sub(time.Now().Add(2 * time.Hour)).Abs(); diff > time.Minute {
					t.Errorf("Until = %v, want about now+2h (off by %v)", got.Until, diff)
				}
			}
		})
	}
}

func TestActivateTreatsFailedStatusAsError(t *testing.T) {
	for _, status := range []string{"Failed", "FailedAsResourceIsLocked", "Denied", "AdminDenied", "Canceled", "Invalid", "Revoked", "TimedOut"} {
		fake := &fakeAPI{principal: "user-oid", status: status}
		_, err := newFakeService(fake).Activate(context.Background(), nil, testRole, "reason", time.Hour)
		var ae *ActivationError
		if !errors.As(err, &ae) || ae.Kind != ErrOther {
			t.Fatalf("%s: err = %v, want *ActivationError with ErrOther", status, err)
		}
		if !strings.Contains(ae.Message, status) {
			t.Errorf("%s: Message = %q, want it to name the status", status, ae.Message)
		}
	}
}

func TestActivateRejectsEmptyReason(t *testing.T) {
	fake := &fakeAPI{principal: "user-oid", status: "Provisioned"}
	if _, err := newFakeService(fake).Activate(context.Background(), nil, testRole, "  ", time.Hour); err == nil {
		t.Fatal("Activate with blank reason returned nil error")
	}
	if fake.activateCalls != 0 {
		t.Errorf("activate called %d times, want 0", fake.activateCalls)
	}
}

func TestActivateRejectsDurationBelowMinimum(t *testing.T) {
	fake := &fakeAPI{principal: "user-oid", status: "Provisioned"}
	if _, err := newFakeService(fake).Activate(context.Background(), nil, testRole, "reason", time.Minute); err == nil {
		t.Fatal("Activate with 1m returned nil error")
	}
	if fake.activateCalls != 0 {
		t.Errorf("activate called %d times, want 0", fake.activateCalls)
	}
}

func TestActivateClassifiesARMError(t *testing.T) {
	fake := &fakeAPI{principal: "user-oid", activateErr: armError("RoleAssignmentExists", "The Role assignment already exists.")}
	_, err := newFakeService(fake).Activate(context.Background(), nil, testRole, "reason", time.Hour)
	var ae *ActivationError
	if !errors.As(err, &ae) {
		t.Fatalf("err = %T %v, want *ActivationError", err, err)
	}
	if ae.Kind != ErrAlreadyActive || ae.Message != "The Role assignment already exists." {
		t.Errorf("got %+v, want ErrAlreadyActive with ARM's message", ae)
	}
}

// armError builds an *azcore.ResponseError the way the SDK does from an ARM
// error response body.
func armError(code, message string) error {
	body := `{"error":{"code":"` + code + `","message":` + jsonString(message) + `}}`
	req, _ := http.NewRequest(http.MethodPut, "https://management.azure.com/x", nil)
	return runtime.NewResponseError(&http.Response{
		StatusCode: http.StatusBadRequest,
		Status:     "400 Bad Request",
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	})
}

func jsonString(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

func TestClassifyActivationError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantKind ErrorKind
		wantMsg  string
	}{
		{"acrs without body", &azcore.ResponseError{ErrorCode: "RoleAssignmentRequestAcrsValidationFailed"}, ErrMFARequired, ""},
		{"acrs", armError("RoleAssignmentRequestAcrsValidationFailed", "Authentication context required."), ErrMFARequired, "Authentication context required."},
		{"exists", armError("RoleAssignmentExists", "The Role assignment already exists."), ErrAlreadyActive, "The Role assignment already exists."},
		{"mfa rule", armError("RoleAssignmentRequestPolicyValidationFailed", `The following policy rules failed: ["MfaRule"]`), ErrMFARequired, `The following policy rules failed: ["MfaRule"]`},
		{"ticketing rule", armError("RoleAssignmentRequestPolicyValidationFailed", `The following policy rules failed: ["TicketingRule"]`), ErrTicketRequired, `The following policy rules failed: ["TicketingRule"]`},
		{"expiration rule", armError("RoleAssignmentRequestPolicyValidationFailed", `The following policy rules failed: ["ExpirationRule"]`), ErrOther, `The following policy rules failed: ["ExpirationRule"]`},
		{"plain error", errors.New("boom"), ErrOther, "boom"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyActivationError(tt.err)
			if got.Kind != tt.wantKind {
				t.Errorf("Kind = %v, want %v", got.Kind, tt.wantKind)
			}
			if tt.wantMsg != "" && got.Message != tt.wantMsg {
				t.Errorf("Message = %q, want %q", got.Message, tt.wantMsg)
			}
			if got.Error() != got.Message {
				t.Errorf("Error() = %q, want Message %q", got.Error(), got.Message)
			}
		})
	}
}

func TestErrorText(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"arm error", armError("AuthorizationFailed", "The client does not have authorization."), "AuthorizationFailed: The client does not have authorization."},
		{"wrapped arm error", fmt.Errorf("list: %w", armError("AuthorizationFailed", "No access.")), "AuthorizationFailed: No access."},
		{"code without body", &azcore.ResponseError{ErrorCode: "Throttled"}, "Throttled"},
		{"plain error", errors.New("boom\nsecond line"), "boom\nsecond line"},
	}
	for _, tt := range tests {
		if got := ErrorText(tt.err); got != tt.want {
			t.Errorf("%s: ErrorText = %q, want %q", tt.name, got, tt.want)
		}
	}
}
