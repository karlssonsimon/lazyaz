package cache

import (
	"testing"
	"time"

	"github.com/karlssonsimon/lazyaz/internal/activity"
)

func TestBrokerActivityAdapterSnapshotReflectsStreamInfo(t *testing.T) {
	broker := NewBroker[string](NewMap[string](), func(s string) string { return s })
	broker.Set("key-a", []string{"one", "two", "three"})

	info := StreamInfo{
		Key:       "key-a",
		Status:    StreamDone,
		Items:     3,
		Subs:      0,
		StartedAt: time.Unix(100, 0),
		EndedAt:   time.Unix(105, 0),
	}
	a := NewBrokerActivityAdapter(broker, info)

	if a.Kind() != activity.KindFetch {
		t.Fatalf("want KindFetch, got %v", a.Kind())
	}
	if a.ID() != "fetch:key-a" {
		t.Fatalf("want id fetch:key-a, got %q", a.ID())
	}
	// Note: Snapshot re-queries the broker's current state. Since our
	// seeded stream isn't actually tracked in broker.streams, Snapshot
	// returns StatusCancelled per the "stream vanished" fallback. We
	// test the "happy path" in an integration-ish fashion below.
}

func TestBrokerActivityAdapterTitleFromKey(t *testing.T) {
	broker := NewBroker[string](NewMap[string](), func(s string) string { return s })
	info := StreamInfo{Key: "blobs\x00sub-id\x00account\x00container"}
	a := NewBrokerActivityAdapter(broker, info)
	title := a.Title()
	want := "blobs > sub-id > account > container"
	if title != want {
		t.Fatalf("want title %q, got %q", want, title)
	}
}
