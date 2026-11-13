package pim

import (
	"sort"
	"strings"
	"time"
)

// Role is one eligible Azure resource role at a scope.
type Role struct {
	TenantID              string
	TenantName            string
	Scope                 string // full ARM scope ID, as returned by ARM
	ScopeName             string // display name, see ScopeDisplayName
	RoleDefinitionID      string // full ARM ID
	RoleName              string
	EligibilityScheduleID string // linked schedule to activate from
	Active                bool
	ActiveUntil           time.Time // zero unless Active
}

// Key identifies a (tenant, scope, role) triple. Lower-cased so ARM casing
// differences do not split one role into two.
func (r Role) Key() string {
	return strings.ToLower(r.TenantID + "|" + r.Scope + "|" + r.RoleDefinitionID)
}

// IsActive reports whether the role is active at now. An active role with
// no end time stays active; one whose end time has passed does not.
func (r Role) IsActive(now time.Time) bool {
	return r.Active && (r.ActiveUntil.IsZero() || r.ActiveUntil.After(now))
}

// Label is "<RoleName> @ <ScopeName>", used in titles and notifications.
func (r Role) Label() string {
	return r.RoleName + " @ " + r.ScopeName
}

// EligibleInstance is one eligibility as returned by ARM.
type EligibleInstance struct {
	Scope, ScopeDisplayName, RoleDefinitionID, RoleName, EligibilityScheduleID string
}

// ActiveInstance is one active assignment as returned by ARM.
type ActiveInstance struct {
	Scope, RoleDefinitionID string
	EndTime                 time.Time
}

// ScopeDisplayName returns expandedName when set, else the last path
// segment of scope.
func ScopeDisplayName(scope, expandedName string) string {
	if expandedName != "" {
		return expandedName
	}
	trimmed := strings.TrimRight(scope, "/")
	if trimmed == "" {
		return scope
	}
	return trimmed[strings.LastIndex(trimmed, "/")+1:]
}

// activeMatchKey matches an active assignment to an eligibility: scope
// case-insensitively and the role definition on its GUID, so differently
// qualified IDs of the same definition match (as in policyRules).
func activeMatchKey(scope, roleDefinitionID string) string {
	return strings.ToLower(scope + "|" + lastSegment(roleDefinitionID))
}

// ActiveSet is the end time of each activated (scope, role definition),
// keyed the way roles are matched to activations.
type ActiveSet map[string]time.Time

// NewActiveSet indexes active; where one role has several activations the
// latest end wins.
func NewActiveSet(active []ActiveInstance) ActiveSet {
	set := make(ActiveSet, len(active))
	for _, a := range active {
		key := activeMatchKey(a.Scope, a.RoleDefinitionID)
		if cur, ok := set[key]; !ok || a.EndTime.After(cur) {
			set[key] = a.EndTime
		}
	}
	return set
}

// Apply sets r's active state from the set, clearing it when r is not in
// the set.
func (s ActiveSet) Apply(r *Role) {
	end, ok := s[activeMatchKey(r.Scope, r.RoleDefinitionID)]
	r.Active, r.ActiveUntil = ok, time.Time{}
	if ok {
		r.ActiveUntil = end
	}
}

// MergeRoles collapses duplicate eligibility paths (direct and via group)
// into one Role per (scope, role), marks the ones that are currently active,
// and sorts by scope name then role name, case-insensitively.
func MergeRoles(tenantID, tenantName string, eligible []EligibleInstance, active []ActiveInstance) []Role {
	activeSet := NewActiveSet(active)

	seen := make(map[string]bool, len(eligible))
	roles := make([]Role, 0, len(eligible))
	for _, e := range eligible {
		r := Role{
			TenantID:              tenantID,
			TenantName:            tenantName,
			Scope:                 e.Scope,
			ScopeName:             ScopeDisplayName(e.Scope, e.ScopeDisplayName),
			RoleDefinitionID:      e.RoleDefinitionID,
			RoleName:              e.RoleName,
			EligibilityScheduleID: e.EligibilityScheduleID,
		}
		key := r.Key()
		if seen[key] {
			continue
		}
		seen[key] = true
		activeSet.Apply(&r)
		roles = append(roles, r)
	}

	sort.SliceStable(roles, func(i, j int) bool {
		si, sj := strings.ToLower(roles[i].ScopeName), strings.ToLower(roles[j].ScopeName)
		if si != sj {
			return si < sj
		}
		return strings.ToLower(roles[i].RoleName) < strings.ToLower(roles[j].RoleName)
	})
	return roles
}
