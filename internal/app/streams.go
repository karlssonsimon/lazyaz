package app

import (
	"sort"

	"github.com/karlssonsimon/lazyaz/internal/cache"
	"github.com/karlssonsimon/lazyaz/internal/ui"
)

// cancelStreamBinding is a KeyMatcher that matches "x" — the key to
// cancel the currently selected stream in the overlay.
type cancelStreamBinding struct{}

func (cancelStreamBinding) Matches(key string) bool { return key == "x" }

// collectStreams gathers stream info from all shared brokers into a
// flat list suitable for the stream overlay. Active streams come
// first, then finished (most recent first).
func (m *Model) collectStreams() []ui.StreamEntry {
	var all []cache.StreamInfo
	b := &m.brokers
	all = append(all, b.subscriptions.Streams()...)
	all = append(all, b.blobAccounts.Streams()...)
	all = append(all, b.blobContainers.Streams()...)
	all = append(all, b.blobs.Streams()...)
	all = append(all, b.sbNamespaces.Streams()...)
	all = append(all, b.sbEntities.Streams()...)
	all = append(all, b.sbTopicSubs.Streams()...)
	all = append(all, b.kvVaults.Streams()...)
	all = append(all, b.kvSecrets.Streams()...)
	all = append(all, b.kvVersions.Streams()...)

	// Sort: active first, then by start time descending.
	sort.Slice(all, func(i, j int) bool {
		ai := all[i].Status == cache.StreamActive
		aj := all[j].Status == cache.StreamActive
		if ai != aj {
			return ai
		}
		return all[i].StartedAt.After(all[j].StartedAt)
	})

	entries := make([]ui.StreamEntry, len(all))
	for i, s := range all {
		entries[i] = ui.StreamEntry{
			Key:       s.Key,
			Status:    s.Status.String(),
			Items:     s.Items,
			Subs:      s.Subs,
			StartedAt: s.StartedAt,
			EndedAt:   s.EndedAt,
			Err:       s.Err,
		}
	}
	return entries
}

// cancelStream cancels the broker stream identified by the entry. It
// tries each broker in turn — the key is globally unique across
// brokers in practice (different resource types produce different keys).
func (m *Model) cancelStream(entry ui.StreamEntry) {
	b := &m.brokers
	b.subscriptions.Cancel(entry.Key)
	b.blobAccounts.Cancel(entry.Key)
	b.blobContainers.Cancel(entry.Key)
	b.blobs.Cancel(entry.Key)
	b.sbNamespaces.Cancel(entry.Key)
	b.sbEntities.Cancel(entry.Key)
	b.sbTopicSubs.Cancel(entry.Key)
	b.kvVaults.Cancel(entry.Key)
	b.kvSecrets.Cancel(entry.Key)
	b.kvVersions.Cancel(entry.Key)
}
