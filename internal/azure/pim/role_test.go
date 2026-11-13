package pim

import (
	"strings"
	"testing"
	"time"
)

func TestScopeDisplayName(t *testing.T) {
	cases := []struct{ scope, expanded, want string }{
		{"/subscriptions/1111", "sub-foo-prod", "sub-foo-prod"},
		{"/subscriptions/1111", "", "1111"},
		{"/subscriptions/1111/resourceGroups/rg-baz", "", "rg-baz"},
		{"/subscriptions/1/resourceGroups/rg/providers/Microsoft.KeyVault/vaults/kv1", "", "kv1"},
		{"/providers/Microsoft.Management/managementGroups/mg-root", "", "mg-root"},
		{"/", "", "/"},
		{"", "", ""},
	}
	for _, c := range cases {
		if got := ScopeDisplayName(c.scope, c.expanded); got != c.want {
			t.Errorf("ScopeDisplayName(%q, %q) = %q, want %q", c.scope, c.expanded, got, c.want)
		}
	}
}

func TestRoleKeyIsCaseInsensitive(t *testing.T) {
	a := Role{TenantID: "T1", Scope: "/subscriptions/ABC", RoleDefinitionID: "/providers/X/roleDefinitions/R1"}
	b := Role{TenantID: "t1", Scope: "/Subscriptions/abc", RoleDefinitionID: "/providers/x/roledefinitions/r1"}
	if a.Key() != b.Key() {
		t.Errorf("keys differ: %q vs %q", a.Key(), b.Key())
	}
}

func TestRoleKeyIncludesTenant(t *testing.T) {
	a := Role{TenantID: "t1", Scope: "/s/1", RoleDefinitionID: "/r/1"}
	b := Role{TenantID: "t2", Scope: "/s/1", RoleDefinitionID: "/r/1"}
	if a.Key() == b.Key() {
		t.Errorf("same key %q for roles in different tenants", a.Key())
	}
	if got := a.Key(); got != "t1|/s/1|/r/1" {
		t.Errorf("Key() = %q, want t1|/s/1|/r/1", got)
	}
}

func TestRoleIsActive(t *testing.T) {
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		role Role
		want bool
	}{
		{"not active", Role{ActiveUntil: now.Add(time.Hour)}, false},
		{"active with time left", Role{Active: true, ActiveUntil: now.Add(time.Hour)}, true},
		{"active without end", Role{Active: true}, true},
		{"expired", Role{Active: true, ActiveUntil: now.Add(-time.Second)}, false},
		{"ends now", Role{Active: true, ActiveUntil: now}, false},
	}
	for _, tt := range tests {
		if got := tt.role.IsActive(now); got != tt.want {
			t.Errorf("%s: IsActive = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestRoleLabel(t *testing.T) {
	r := Role{RoleName: "Contributor", ScopeName: "sub-prod"}
	if got := r.Label(); got != "Contributor @ sub-prod" {
		t.Errorf("Label() = %q", got)
	}
}

func TestMergeRolesCollapsesDuplicatePaths(t *testing.T) {
	got := MergeRoles("t", "T", []EligibleInstance{
		{Scope: "/subscriptions/1", RoleDefinitionID: "/r/1", RoleName: "Reader", EligibilityScheduleID: "direct"},
		{Scope: "/subscriptions/1", RoleDefinitionID: "/r/1", RoleName: "Reader", EligibilityScheduleID: "via-group"},
	}, nil)
	if len(got) != 1 {
		t.Fatalf("got %d roles, want 1", len(got))
	}
	if got[0].EligibilityScheduleID != "direct" {
		t.Errorf("EligibilityScheduleID = %q, want direct", got[0].EligibilityScheduleID)
	}
}

func TestMergeRolesMatchesActiveCaseInsensitively(t *testing.T) {
	end := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	got := MergeRoles("t", "T",
		[]EligibleInstance{{Scope: "/subscriptions/ABC", RoleDefinitionID: "/r/1", RoleName: "Reader"}},
		[]ActiveInstance{{Scope: "/subscriptions/abc", RoleDefinitionID: "/R/1", EndTime: end}})
	if len(got) != 1 || !got[0].Active || !got[0].ActiveUntil.Equal(end) {
		t.Fatalf("got %+v, want active until %v", got, end)
	}
}

func TestMergeRolesMatchesActiveByRoleDefinitionGUID(t *testing.T) {
	end := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	got := MergeRoles("t", "T",
		[]EligibleInstance{{
			Scope:            "/subscriptions/ABC",
			RoleDefinitionID: "/subscriptions/ABC/providers/Microsoft.Authorization/roleDefinitions/B24988AC-6180-42A0-AB88-20F7382DD24C",
			RoleName:         "Contributor",
		}},
		[]ActiveInstance{{
			Scope:            "/subscriptions/abc",
			RoleDefinitionID: "/providers/Microsoft.Authorization/roleDefinitions/b24988ac-6180-42a0-ab88-20f7382dd24c",
			EndTime:          end,
		}})
	if len(got) != 1 || !got[0].Active || !got[0].ActiveUntil.Equal(end) {
		t.Fatalf("got %+v, want active until %v", got, end)
	}
}

func TestMergeRolesSortsByScopeNameThenRoleName(t *testing.T) {
	got := MergeRoles("t", "T", []EligibleInstance{
		{Scope: "/s/2", ScopeDisplayName: "beta", RoleDefinitionID: "/r/1", RoleName: "Reader"},
		{Scope: "/s/1", ScopeDisplayName: "Alpha", RoleDefinitionID: "/r/2", RoleName: "owner"},
		{Scope: "/s/1", ScopeDisplayName: "Alpha", RoleDefinitionID: "/r/3", RoleName: "Contributor"},
	}, nil)
	want := []string{"Contributor @ Alpha", "owner @ Alpha", "Reader @ beta"}
	if len(got) != len(want) {
		t.Fatalf("got %d roles, want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i].Label() != w {
			t.Errorf("got[%d] = %q, want %q", i, got[i].Label(), w)
		}
	}
}

func TestMergeRolesSetsTenant(t *testing.T) {
	got := MergeRoles("tid", "tname", []EligibleInstance{
		{Scope: "/s/1", RoleDefinitionID: "/r/1"}, {Scope: "/s/2", RoleDefinitionID: "/r/1"},
	}, nil)
	for _, r := range got {
		if r.TenantID != "tid" || r.TenantName != "tname" {
			t.Errorf("tenant = %q/%q", r.TenantID, r.TenantName)
		}
		if !strings.HasPrefix(r.Key(), "tid|") {
			t.Errorf("Key() = %q, want it to carry the tenant", r.Key())
		}
	}
}
