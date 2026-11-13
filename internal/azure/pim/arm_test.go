package pim

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
)

// fakeTransport answers every request with one canned response and records
// the requests it saw. No network traffic leaves the process.
type fakeTransport struct {
	mu     sync.Mutex
	status int
	body   string
	reqs   []*http.Request
	bodies []string
}

func (f *fakeTransport) Do(req *http.Request) (*http.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var body string
	if req.Body != nil {
		b, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		body = string(b)
	}
	f.reqs = append(f.reqs, req)
	f.bodies = append(f.bodies, body)
	return &http.Response{
		StatusCode: f.status,
		Status:     http.StatusText(f.status),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(f.body)),
		Request:    req,
	}, nil
}

// fakeCredential hands out a fixed unsigned JWT and records the last request.
type fakeCredential struct {
	token string
	last  policy.TokenRequestOptions
}

func (c *fakeCredential) GetToken(_ context.Context, opts policy.TokenRequestOptions) (azcore.AccessToken, error) {
	c.last = opts
	return azcore.AccessToken{Token: c.token, ExpiresOn: time.Now().Add(time.Hour)}, nil
}

func fakeJWT(t *testing.T, claims map[string]string) string {
	t.Helper()
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	enc := base64.RawURLEncoding.EncodeToString
	return enc([]byte(`{"alg":"none"}`)) + "." + enc(payload) + ".sig"
}

func newTestARMAPI(t *testing.T, tr *fakeTransport) (*armAPI, *fakeCredential) {
	t.Helper()
	cred := &fakeCredential{token: fakeJWT(t, map[string]string{"oid": "user-oid"})}
	a, err := newARMAPI(cred, &arm.ClientOptions{ClientOptions: policy.ClientOptions{Transport: tr}})
	if err != nil {
		t.Fatalf("newARMAPI: %v", err)
	}
	return a, cred
}

func TestListEligibleRequestsTenantScopeAsTarget(t *testing.T) {
	tr := &fakeTransport{status: http.StatusOK, body: `{"value":[
		{"properties":{
			"scope":"/subscriptions/1",
			"roleDefinitionId":"/subscriptions/1/providers/Microsoft.Authorization/roleDefinitions/rd",
			"roleEligibilityScheduleId":"/subscriptions/1/providers/Microsoft.Authorization/roleEligibilitySchedules/es",
			"expandedProperties":{"scope":{"displayName":"Prod"},"roleDefinition":{"displayName":"Owner"}}}},
		{"properties":{"roleDefinitionId":"no-scope"}},
		{"properties":{"scope":"/subscriptions/2"}},
		{}
	]}`}
	a, _ := newTestARMAPI(t, tr)

	got, err := a.listEligible(context.Background())
	if err != nil {
		t.Fatalf("listEligible: %v", err)
	}
	if len(tr.reqs) != 1 {
		t.Fatalf("requests = %d, want 1", len(tr.reqs))
	}
	u := tr.reqs[0].URL
	if u.Path != "/providers/Microsoft.Authorization/roleEligibilityScheduleInstances" {
		t.Errorf("path = %q, want tenant-scope path without a doubled slash", u.Path)
	}
	if f := u.Query().Get("$filter"); f != "asTarget()" {
		t.Errorf("$filter = %q, want asTarget()", f)
	}
	want := []EligibleInstance{{
		Scope:                 "/subscriptions/1",
		ScopeDisplayName:      "Prod",
		RoleDefinitionID:      "/subscriptions/1/providers/Microsoft.Authorization/roleDefinitions/rd",
		RoleName:              "Owner",
		EligibilityScheduleID: "/subscriptions/1/providers/Microsoft.Authorization/roleEligibilitySchedules/es",
	}}
	if len(got) != 1 || got[0] != want[0] {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestListActiveRebuildsFromTenantScopeRequests(t *testing.T) {
	// rd1 runs for another half hour, rd2 ended long ago, rd3 was
	// deactivated, rd4 runs until a fixed end time.
	started := time.Now().UTC().Add(-30 * time.Minute).Truncate(time.Second)
	ts := started.Format(time.RFC3339)
	tr := &fakeTransport{status: http.StatusOK, body: fmt.Sprintf(`{"value":[
		{"properties":{"scope":"/subscriptions/sub-a","roleDefinitionId":"/providers/Microsoft.Authorization/roleDefinitions/rd1",
			"requestType":"SelfActivate","status":"Provisioned","createdOn":%[1]q,
			"scheduleInfo":{"startDateTime":%[1]q,"expiration":{"type":"AfterDuration","duration":"PT1H"}}}},
		{"properties":{"scope":"/subscriptions/sub-a","roleDefinitionId":"/providers/Microsoft.Authorization/roleDefinitions/rd2",
			"requestType":"SelfActivate","status":"Provisioned","createdOn":"2000-01-01T10:00:00Z",
			"scheduleInfo":{"startDateTime":"2000-01-01T10:00:00Z","expiration":{"type":"AfterDuration","duration":"PT8H"}}}},
		{"properties":{"scope":"/subscriptions/sub-a/resourceGroups/rg-1","roleDefinitionId":"/providers/Microsoft.Authorization/roleDefinitions/rd3",
			"requestType":"SelfActivate","status":"Provisioned","createdOn":"2000-01-01T09:00:00Z",
			"scheduleInfo":{"expiration":{"type":"AfterDateTime","endDateTime":"2099-01-01T12:00:00Z"}}}},
		{"properties":{"scope":"/subscriptions/sub-a/resourceGroups/rg-1","roleDefinitionId":"/providers/Microsoft.Authorization/roleDefinitions/rd3",
			"requestType":"SelfDeactivate","status":"Revoked","createdOn":"2000-01-01T09:30:00Z"}},
		{"properties":{"scope":"/subscriptions/sub-b","roleDefinitionId":"/providers/Microsoft.Authorization/roleDefinitions/rd4",
			"requestType":"SelfActivate","status":"Granted","createdOn":"2000-01-01T09:00:00Z",
			"scheduleInfo":{"expiration":{"type":"AfterDateTime","endDateTime":"2099-01-01T12:00:00Z"}}}},
		{}
	]}`, ts)}
	a, _ := newTestARMAPI(t, tr)

	got, err := a.listActive(context.Background())
	if err != nil {
		t.Fatalf("listActive: %v", err)
	}
	u := tr.reqs[0].URL
	if u.Path != "/providers/Microsoft.Authorization/roleAssignmentScheduleRequests" {
		t.Errorf("path = %q, want tenant-scope requests path", u.Path)
	}
	if f := u.Query().Get("$filter"); f != "asTarget()" {
		t.Errorf("$filter = %q, want asTarget()", f)
	}
	want := []ActiveInstance{
		{Scope: "/subscriptions/sub-a", RoleDefinitionID: "/providers/Microsoft.Authorization/roleDefinitions/rd1", EndTime: started.Add(time.Hour)},
		{Scope: "/subscriptions/sub-b", RoleDefinitionID: "/providers/Microsoft.Authorization/roleDefinitions/rd4", EndTime: time.Date(2099, 1, 1, 12, 0, 0, 0, time.UTC)},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i].Scope != want[i].Scope || got[i].RoleDefinitionID != want[i].RoleDefinitionID || !got[i].EndTime.Equal(want[i].EndTime) {
			t.Errorf("got[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestPolicyRulesReadsEffectiveExpirationRulesForRole(t *testing.T) {
	tr := &fakeTransport{status: http.StatusOK, body: `{"value":[
		{"properties":{"roleDefinitionId":"/subscriptions/1/providers/Microsoft.Authorization/roleDefinitions/other",
			"effectiveRules":[{"ruleType":"RoleManagementPolicyExpirationRule","id":"Expiration_EndUser_Assignment","maximumDuration":"PT1H"}]}},
		{"properties":{"roleDefinitionId":"/subscriptions/1/providers/Microsoft.Authorization/roleDefinitions/RD",
			"effectiveRules":[
				{"ruleType":"RoleManagementPolicyEnablementRule","id":"Enablement_EndUser_Assignment","enabledRules":["Justification"]},
				{"ruleType":"RoleManagementPolicyExpirationRule","id":"Expiration_EndUser_Assignment","maximumDuration":"PT8H"},
				{"ruleType":"RoleManagementPolicyExpirationRule","id":"Expiration_Admin_Eligibility","maximumDuration":"P365D"}
			]}}
	]}`}
	a, _ := newTestARMAPI(t, tr)

	got, err := a.policyRules(context.Background(), "/subscriptions/1", "/subscriptions/1/providers/Microsoft.Authorization/roleDefinitions/rd")
	if err != nil {
		t.Fatalf("policyRules: %v", err)
	}
	if p := tr.reqs[0].URL.Path; p != "/subscriptions/1/providers/Microsoft.Authorization/roleManagementPolicyAssignments" {
		t.Errorf("path = %q", p)
	}
	want := []PolicyRule{
		{ID: "Expiration_EndUser_Assignment", MaximumDuration: "PT8H"},
		{ID: "Expiration_Admin_Eligibility", MaximumDuration: "P365D"},
	}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestActivatePutsSelfActivateRequest(t *testing.T) {
	tr := &fakeTransport{status: http.StatusCreated, body: `{"properties":{"status":"Provisioned"}}`}
	a, _ := newTestARMAPI(t, tr)

	status, err := a.activate(context.Background(), activateRequest{
		Scope:                 "/subscriptions/1",
		RoleDefinitionID:      "/subscriptions/1/providers/Microsoft.Authorization/roleDefinitions/rd",
		EligibilityScheduleID: "/subscriptions/1/providers/Microsoft.Authorization/roleEligibilitySchedules/es",
		PrincipalID:           "user-oid",
		Justification:         "Felsökning",
		Duration:              90 * time.Minute,
	})
	if err != nil {
		t.Fatalf("activate: %v", err)
	}
	if status != "Provisioned" {
		t.Errorf("status = %q, want Provisioned", status)
	}

	req := tr.reqs[0]
	if req.Method != http.MethodPut {
		t.Errorf("method = %s, want PUT", req.Method)
	}
	pathRe := regexp.MustCompile(`^/subscriptions/1/providers/Microsoft\.Authorization/roleAssignmentScheduleRequests/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	if !pathRe.MatchString(req.URL.Path) {
		t.Errorf("path = %q, want .../roleAssignmentScheduleRequests/<uuid>", req.URL.Path)
	}

	var body struct {
		Properties struct {
			RequestType                     string `json:"requestType"`
			PrincipalID                     string `json:"principalId"`
			RoleDefinitionID                string `json:"roleDefinitionId"`
			LinkedRoleEligibilityScheduleID string `json:"linkedRoleEligibilityScheduleId"`
			Justification                   string `json:"justification"`
			ScheduleInfo                    struct {
				StartDateTime string `json:"startDateTime"`
				Expiration    struct {
					Type     string `json:"type"`
					Duration string `json:"duration"`
				} `json:"expiration"`
			} `json:"scheduleInfo"`
		} `json:"properties"`
	}
	if err := json.Unmarshal([]byte(tr.bodies[0]), &body); err != nil {
		t.Fatalf("decode body %q: %v", tr.bodies[0], err)
	}
	p := body.Properties
	if p.RequestType != "SelfActivate" {
		t.Errorf("requestType = %q, want SelfActivate", p.RequestType)
	}
	if p.PrincipalID != "user-oid" {
		t.Errorf("principalId = %q, want user-oid", p.PrincipalID)
	}
	if p.RoleDefinitionID != "/subscriptions/1/providers/Microsoft.Authorization/roleDefinitions/rd" {
		t.Errorf("roleDefinitionId = %q", p.RoleDefinitionID)
	}
	if p.LinkedRoleEligibilityScheduleID != "/subscriptions/1/providers/Microsoft.Authorization/roleEligibilitySchedules/es" {
		t.Errorf("linkedRoleEligibilityScheduleId = %q", p.LinkedRoleEligibilityScheduleID)
	}
	if p.Justification != "Felsökning" {
		t.Errorf("justification = %q, want Felsökning", p.Justification)
	}
	if p.ScheduleInfo.Expiration.Type != "AfterDuration" || p.ScheduleInfo.Expiration.Duration != "PT1H30M" {
		t.Errorf("expiration = %+v, want AfterDuration PT1H30M", p.ScheduleInfo.Expiration)
	}
	start, err := time.Parse(time.RFC3339Nano, p.ScheduleInfo.StartDateTime)
	if err != nil {
		t.Fatalf("startDateTime %q: %v", p.ScheduleInfo.StartDateTime, err)
	}
	if d := time.Since(start).Abs(); d > time.Minute {
		t.Errorf("startDateTime %v is not about now", start)
	}
}

func TestActivateReturnsResponseErrorFromARM(t *testing.T) {
	tr := &fakeTransport{status: http.StatusBadRequest, body: `{"error":{"code":"RoleAssignmentExists","message":"The Role assignment already exists."}}`}
	a, _ := newTestARMAPI(t, tr)

	_, err := a.activate(context.Background(), activateRequest{Scope: "/subscriptions/1", Duration: time.Hour})
	got := classifyActivationError(err)
	if got.Kind != ErrAlreadyActive || got.Message != "The Role assignment already exists." {
		t.Errorf("got %+v, want ErrAlreadyActive with ARM's message", got)
	}
}

func TestPrincipalIDReadsOIDFromManagementTokenForTenant(t *testing.T) {
	a, cred := newTestARMAPI(t, &fakeTransport{status: http.StatusOK})

	oid, err := a.principalID(context.Background(), "t1")
	if err != nil {
		t.Fatalf("principalID: %v", err)
	}
	if oid != "user-oid" {
		t.Errorf("oid = %q, want user-oid", oid)
	}
	if cred.last.TenantID != "t1" {
		t.Errorf("token TenantID = %q, want t1", cred.last.TenantID)
	}
	if len(cred.last.Scopes) != 1 || cred.last.Scopes[0] != "https://management.azure.com/.default" {
		t.Errorf("token scopes = %v", cred.last.Scopes)
	}
}
