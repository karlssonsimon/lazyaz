package pim

import (
	"sort"
	"strings"
	"time"
)

// activationRequest is one roleAssignmentScheduleRequest, reduced to what
// active-state reconstruction needs.
//
// Active state is rebuilt from the caller's requests rather than read from
// roleAssignmentScheduleInstances: ARM answers the requests list in about a
// second, while the instances list takes about a minute for a user with
// many permanent assignments, because it cannot be filtered to activations
// server-side.
type activationRequest struct {
	Scope, RoleDefinitionID string
	Type                    string // SelfActivate, SelfDeactivate, SelfExtend, ...
	Status                  string
	Created                 time.Time
	Start                   time.Time // zero: use Created
	ExpirationType          string    // AfterDuration, AfterDateTime, NoExpiration
	Duration                string    // ISO 8601, for AfterDuration
	End                     time.Time // for AfterDateTime
}

// grantedStatuses are the request statuses after which the activation is in
// effect (or, for a deactivation, removed).
var grantedStatuses = map[string]bool{
	"Provisioned":     true,
	"Granted":         true,
	"ScheduleCreated": true,
}

// activeFromRequests replays reqs in creation order and returns the
// activations in effect at now, sorted by scope then role definition.
//
// A granted SelfActivate, SelfExtend or SelfRenew sets a role's window; a
// revoked SelfDeactivate clears it. Requests that were not granted change
// nothing. A granted request whose window cannot be worked out clears the
// role rather than guessing: a role wrongly shown as active cannot be
// activated from the list, while one wrongly shown as inactive only costs
// a rejected request.
func activeFromRequests(reqs []activationRequest, now time.Time) []ActiveInstance {
	ordered := make([]activationRequest, len(reqs))
	copy(ordered, reqs)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Created.Before(ordered[j].Created) })

	type window struct {
		inst  ActiveInstance
		start time.Time
	}
	state := map[string]window{}
	for _, r := range ordered {
		key := activeMatchKey(r.Scope, r.RoleDefinitionID)
		switch r.Type {
		case "SelfActivate", "SelfExtend", "SelfRenew":
			if !grantedStatuses[r.Status] {
				continue
			}
			start := r.Start
			if start.IsZero() {
				start = r.Created
			}
			end, ok := requestEnd(r, start)
			if !ok {
				delete(state, key)
				continue
			}
			state[key] = window{
				inst:  ActiveInstance{Scope: r.Scope, RoleDefinitionID: r.RoleDefinitionID, EndTime: end},
				start: start,
			}
		case "SelfDeactivate":
			if r.Status == "Revoked" || grantedStatuses[r.Status] {
				delete(state, key)
			}
		}
	}

	var out []ActiveInstance
	for _, w := range state {
		if w.start.After(now) || (!w.inst.EndTime.IsZero() && !w.inst.EndTime.After(now)) {
			continue
		}
		out = append(out, w.inst)
	}
	sort.Slice(out, func(i, j int) bool {
		if a, b := strings.ToLower(out[i].Scope), strings.ToLower(out[j].Scope); a != b {
			return a < b
		}
		return strings.ToLower(lastSegment(out[i].RoleDefinitionID)) < strings.ToLower(lastSegment(out[j].RoleDefinitionID))
	})
	return out
}

// requestEnd is when r's activation ends; zero with ok for no expiration.
func requestEnd(r activationRequest, start time.Time) (time.Time, bool) {
	switch r.ExpirationType {
	case "AfterDuration":
		d, err := ParseISO8601(r.Duration)
		if err != nil || d <= 0 {
			return time.Time{}, false
		}
		return start.Add(d), true
	case "AfterDateTime":
		return r.End, !r.End.IsZero()
	case "NoExpiration":
		return time.Time{}, true
	}
	return time.Time{}, false
}
