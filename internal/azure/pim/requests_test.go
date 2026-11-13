package pim

import (
	"testing"
	"time"
)

func TestActiveFromRequests(t *testing.T) {
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	at := func(h, m int) time.Time { return time.Date(2026, 11, 13, h, m, 0, 0, time.UTC) }
	const (
		subA   = "/subscriptions/sub-a"
		rg1    = "/subscriptions/sub-a/resourceGroups/rg-1"
		reader = "/subscriptions/sub-a/providers/Microsoft.Authorization/roleDefinitions/reader-guid"
		contr  = "/providers/Microsoft.Authorization/roleDefinitions/contributor-guid"
	)
	activate := func(scope, rd string, created time.Time, dur string) activationRequest {
		return activationRequest{
			Scope: scope, RoleDefinitionID: rd, Type: "SelfActivate", Status: "Provisioned",
			Created: created, Start: created, ExpirationType: "AfterDuration", Duration: dur,
		}
	}

	tests := []struct {
		name string
		reqs []activationRequest
		want []ActiveInstance
	}{
		{
			name: "granted activation ends start plus duration",
			reqs: []activationRequest{activate(subA, contr, at(11, 30), "PT1H")},
			want: []ActiveInstance{{Scope: subA, RoleDefinitionID: contr, EndTime: at(12, 30)}},
		},
		{
			name: "expired activation is history",
			reqs: []activationRequest{activate(subA, contr, at(3, 0), "PT8H")},
		},
		{
			name: "start falls back to created",
			reqs: []activationRequest{func() activationRequest {
				r := activate(subA, contr, at(11, 30), "PT1H")
				r.Start = time.Time{}
				return r
			}()},
			want: []ActiveInstance{{Scope: subA, RoleDefinitionID: contr, EndTime: at(12, 30)}},
		},
		{
			name: "after date time uses the end",
			reqs: []activationRequest{{
				Scope: rg1, RoleDefinitionID: reader, Type: "SelfActivate", Status: "Granted",
				Created: at(11, 0), Start: at(11, 0), ExpirationType: "AfterDateTime", End: at(13, 0),
			}},
			want: []ActiveInstance{{Scope: rg1, RoleDefinitionID: reader, EndTime: at(13, 0)}},
		},
		{
			name: "no expiration has no end",
			reqs: []activationRequest{{
				Scope: rg1, RoleDefinitionID: reader, Type: "SelfActivate", Status: "ScheduleCreated",
				Created: at(11, 0), ExpirationType: "NoExpiration",
			}},
			want: []ActiveInstance{{Scope: rg1, RoleDefinitionID: reader}},
		},
		{
			name: "pending, denied and failed requests do not activate",
			reqs: func() []activationRequest {
				var out []activationRequest
				for _, s := range []string{"PendingApproval", "Denied", "Failed", "Canceled", "Accepted"} {
					r := activate(subA, contr, at(11, 30), "PT1H")
					r.Status = s
					out = append(out, r)
				}
				return out
			}(),
		},
		{
			name: "later deactivation clears",
			reqs: []activationRequest{
				{Scope: subA, RoleDefinitionID: contr, Type: "SelfDeactivate", Status: "Revoked", Created: at(11, 45)},
				activate(subA, contr, at(11, 30), "PT1H"),
			},
		},
		{
			name: "failed deactivation does not clear",
			reqs: []activationRequest{
				activate(subA, contr, at(11, 30), "PT1H"),
				{Scope: subA, RoleDefinitionID: contr, Type: "SelfDeactivate", Status: "Failed", Created: at(11, 45)},
			},
			want: []ActiveInstance{{Scope: subA, RoleDefinitionID: contr, EndTime: at(12, 30)}},
		},
		{
			name: "activation after a deactivation counts",
			reqs: []activationRequest{
				activate(subA, contr, at(10, 0), "PT1H"),
				{Scope: subA, RoleDefinitionID: contr, Type: "SelfDeactivate", Status: "Revoked", Created: at(10, 30)},
				activate(subA, contr, at(11, 30), "PT2H"),
			},
			want: []ActiveInstance{{Scope: subA, RoleDefinitionID: contr, EndTime: at(13, 30)}},
		},
		{
			name: "latest extension wins",
			reqs: []activationRequest{
				activate(subA, contr, at(11, 0), "PT1H"),
				func() activationRequest {
					r := activate(subA, contr, at(11, 50), "PT2H")
					r.Type, r.Start = "SelfExtend", at(11, 0)
					return r
				}(),
			},
			want: []ActiveInstance{{Scope: subA, RoleDefinitionID: contr, EndTime: at(13, 0)}},
		},
		{
			name: "role definition matched on its GUID and scope case-insensitively",
			reqs: []activationRequest{
				activate(subA, "/subscriptions/sub-a/providers/Microsoft.Authorization/roleDefinitions/CONTRIBUTOR-GUID", at(11, 0), "PT2H"),
				{Scope: "/SUBSCRIPTIONS/SUB-A", RoleDefinitionID: contr, Type: "SelfDeactivate", Status: "Revoked", Created: at(11, 10)},
			},
		},
		{
			name: "unparsable duration is not active",
			reqs: []activationRequest{activate(subA, contr, at(11, 30), "P1W")},
		},
		{
			name: "unparsable later request clears an earlier one",
			reqs: []activationRequest{
				activate(subA, contr, at(11, 0), "PT2H"),
				activate(subA, contr, at(11, 30), "bogus"),
			},
		},
		{
			name: "future start is not active yet",
			reqs: []activationRequest{func() activationRequest {
				r := activate(subA, contr, at(11, 0), "PT2H")
				r.Start = at(12, 30)
				return r
			}()},
		},
		{
			name: "admin requests are ignored",
			reqs: []activationRequest{func() activationRequest {
				r := activate(subA, contr, at(11, 0), "PT2H")
				r.Type = "AdminAssign"
				return r
			}()},
		},
		{
			name: "output sorted by scope then role",
			reqs: []activationRequest{
				activate(subA, reader, at(11, 0), "PT2H"),
				activate(rg1, reader, at(11, 0), "PT2H"),
				activate(subA, contr, at(11, 0), "PT2H"),
			},
			want: []ActiveInstance{
				{Scope: subA, RoleDefinitionID: contr, EndTime: at(13, 0)},
				{Scope: subA, RoleDefinitionID: reader, EndTime: at(13, 0)},
				{Scope: rg1, RoleDefinitionID: reader, EndTime: at(13, 0)},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := activeFromRequests(tt.reqs, now)
			if len(got) != len(tt.want) {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
			for i := range tt.want {
				g, w := got[i], tt.want[i]
				if g.Scope != w.Scope || g.RoleDefinitionID != w.RoleDefinitionID || !g.EndTime.Equal(w.EndTime) {
					t.Errorf("got[%d] = %+v, want %+v", i, g, w)
				}
			}
		})
	}
}
