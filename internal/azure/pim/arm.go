package pim

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization/v2"
	"github.com/google/uuid"
)

// tenantScope makes the SDK build "/providers/Microsoft.Authorization/...":
// it fills "/{scope}/providers/..." and path.Join drops the empty segment.
const tenantScope = ""

const managementScope = "https://management.azure.com/.default"

// NewService returns a Service that talks to Azure Resource Manager.
func NewService() *Service {
	return &Service{newAPI: func(cred azcore.TokenCredential) (api, error) {
		return newARMAPI(cred, nil)
	}}
}

// armAPI implements api with the armauthorization v2 clients.
type armAPI struct {
	cred     azcore.TokenCredential
	eligible *armauthorization.RoleEligibilityScheduleInstancesClient
	requests *armauthorization.RoleAssignmentScheduleRequestsClient
	policies *armauthorization.RoleManagementPolicyAssignmentsClient
}

func newARMAPI(cred azcore.TokenCredential, opts *arm.ClientOptions) (*armAPI, error) {
	eligible, err := armauthorization.NewRoleEligibilityScheduleInstancesClient(cred, opts)
	if err != nil {
		return nil, err
	}
	requests, err := armauthorization.NewRoleAssignmentScheduleRequestsClient(cred, opts)
	if err != nil {
		return nil, err
	}
	policies, err := armauthorization.NewRoleManagementPolicyAssignmentsClient(cred, opts)
	if err != nil {
		return nil, err
	}
	return &armAPI{cred: cred, eligible: eligible, requests: requests, policies: policies}, nil
}

func (a *armAPI) listEligible(ctx context.Context) ([]EligibleInstance, error) {
	pager := a.eligible.NewListForScopePager(tenantScope, &armauthorization.RoleEligibilityScheduleInstancesClientListForScopeOptions{
		Filter: to.Ptr("asTarget()"),
	})
	var out []EligibleInstance
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, inst := range page.Value {
			if inst == nil || inst.Properties == nil {
				continue
			}
			p := inst.Properties
			if deref(p.Scope) == "" || deref(p.RoleDefinitionID) == "" {
				continue
			}
			e := EligibleInstance{
				Scope:                 *p.Scope,
				RoleDefinitionID:      *p.RoleDefinitionID,
				EligibilityScheduleID: deref(p.RoleEligibilityScheduleID),
			}
			if x := p.ExpandedProperties; x != nil {
				if x.RoleDefinition != nil {
					e.RoleName = deref(x.RoleDefinition.DisplayName)
				}
				if x.Scope != nil {
					e.ScopeDisplayName = deref(x.Scope.DisplayName)
				}
			}
			out = append(out, e)
		}
	}
	return out, nil
}

// listActive returns the caller's activations in effect now, anywhere in
// the tenant, rebuilt from their schedule requests (see activationRequest).
func (a *armAPI) listActive(ctx context.Context) ([]ActiveInstance, error) {
	pager := a.requests.NewListForScopePager(tenantScope, &armauthorization.RoleAssignmentScheduleRequestsClientListForScopeOptions{
		Filter: to.Ptr("asTarget()"),
	})
	var reqs []activationRequest
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, r := range page.Value {
			if r == nil || r.Properties == nil {
				continue
			}
			p := r.Properties
			ar := activationRequest{Scope: deref(p.Scope), RoleDefinitionID: deref(p.RoleDefinitionID)}
			if p.RequestType != nil {
				ar.Type = string(*p.RequestType)
			}
			if p.Status != nil {
				ar.Status = string(*p.Status)
			}
			if p.CreatedOn != nil {
				ar.Created = *p.CreatedOn
			}
			if si := p.ScheduleInfo; si != nil {
				if si.StartDateTime != nil {
					ar.Start = *si.StartDateTime
				}
				if x := si.Expiration; x != nil {
					if x.Type != nil {
						ar.ExpirationType = string(*x.Type)
					}
					ar.Duration = deref(x.Duration)
					if x.EndDateTime != nil {
						ar.End = *x.EndDateTime
					}
				}
			}
			reqs = append(reqs, ar)
		}
	}
	return activeFromRequests(reqs, time.Now()), nil
}

// policyRules returns the effective expiration rules of the policy assigned
// to roleDefinitionID at scope. Other rule types are not needed and are
// skipped. Role definitions are matched on their GUID, case-insensitively,
// so a subscription-scoped and a resource-scoped ID of the same role match.
func (a *armAPI) policyRules(ctx context.Context, scope, roleDefinitionID string) ([]PolicyRule, error) {
	want := lastSegment(roleDefinitionID)
	pager := a.policies.NewListForScopePager(scope, nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, pa := range page.Value {
			if pa == nil || pa.Properties == nil || !strings.EqualFold(lastSegment(deref(pa.Properties.RoleDefinitionID)), want) {
				continue
			}
			var rules []PolicyRule
			for _, r := range pa.Properties.EffectiveRules {
				exp, ok := r.(*armauthorization.RoleManagementPolicyExpirationRule)
				if !ok {
					continue
				}
				rules = append(rules, PolicyRule{ID: deref(exp.ID), MaximumDuration: deref(exp.MaximumDuration)})
			}
			return rules, nil
		}
	}
	return nil, nil
}

func (a *armAPI) activate(ctx context.Context, req activateRequest) (string, error) {
	start := time.Now().UTC()
	resp, err := a.requests.Create(ctx, req.Scope, uuid.NewString(), armauthorization.RoleAssignmentScheduleRequest{
		Properties: &armauthorization.RoleAssignmentScheduleRequestProperties{
			RequestType:                     to.Ptr(armauthorization.RequestTypeSelfActivate),
			PrincipalID:                     to.Ptr(req.PrincipalID),
			RoleDefinitionID:                to.Ptr(req.RoleDefinitionID),
			LinkedRoleEligibilityScheduleID: to.Ptr(req.EligibilityScheduleID),
			Justification:                   to.Ptr(req.Justification),
			ScheduleInfo: &armauthorization.RoleAssignmentScheduleRequestPropertiesScheduleInfo{
				StartDateTime: &start,
				Expiration: &armauthorization.RoleAssignmentScheduleRequestPropertiesScheduleInfoExpiration{
					Type:     to.Ptr(armauthorization.TypeAfterDuration),
					Duration: to.Ptr(FormatISO8601(req.Duration)),
				},
			},
		},
	}, nil)
	if err != nil {
		return "", err
	}
	if resp.Properties == nil || resp.Properties.Status == nil {
		return "", errors.New("activation response carried no status")
	}
	return string(*resp.Properties.Status), nil
}

func (a *armAPI) principalID(ctx context.Context, tenantID string) (string, error) {
	tok, err := a.cred.GetToken(ctx, policy.TokenRequestOptions{
		Scopes:   []string{managementScope},
		TenantID: tenantID,
	})
	if err != nil {
		return "", fmt.Errorf("get management token: %w", err)
	}
	return OIDFromToken(tok.Token)
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func lastSegment(id string) string {
	id = strings.TrimRight(id, "/")
	return id[strings.LastIndex(id, "/")+1:]
}
