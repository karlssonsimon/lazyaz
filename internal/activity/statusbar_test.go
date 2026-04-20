package activity

import (
	"strings"
	"testing"
	"time"
)

func TestStatusBarItemUploadDominant(t *testing.T) {
	r := NewRegistry(NewFakeClock(time.Unix(1000, 0)))
	_ = r.Register(snap(t, "u1", KindUpload, Snapshot{
		Status:      StatusRunning,
		StartedAt:   time.Unix(995, 0),
		TotalBytes:  1000,
		DoneBytes:   400,
		BytesPerSec: 42_000_000, // 42 MB/s
	}))
	_ = r.Register(snap(t, "f1", KindFetch, Snapshot{
		Status:    StatusRunning,
		StartedAt: time.Unix(990, 0),
	}))

	value, ok := StatusBarItem(r, "F")
	if !ok {
		t.Fatalf("want an item; got none")
	}
	if !strings.Contains(value, "MB/s") {
		t.Fatalf("want MB/s in value, got %q", value)
	}
	if !strings.Contains(value, "F") {
		t.Fatalf("want key hint F in value, got %q", value)
	}
}

func TestStatusBarItemCountWithGrace(t *testing.T) {
	clock := NewFakeClock(time.Unix(1000, 0))
	r := NewRegistry(clock)
	_ = r.Register(snap(t, "f1", KindFetch, Snapshot{
		Status:    StatusRunning,
		StartedAt: time.Unix(997, 0),
	}))

	value, ok := StatusBarItem(r, "F")
	if !ok {
		t.Fatalf("want an item; got none")
	}
	if !strings.Contains(value, "1 active") {
		t.Fatalf("want '1 active' in value, got %q", value)
	}
}

func TestStatusBarItemGraceSuppressesShortFetch(t *testing.T) {
	clock := NewFakeClock(time.Unix(1000, 0))
	r := NewRegistry(clock)
	_ = r.Register(snap(t, "f1", KindFetch, Snapshot{
		Status:    StatusRunning,
		StartedAt: time.Unix(1000, 0).Add(-500 * time.Millisecond),
	}))

	_, ok := StatusBarItem(r, "F")
	if ok {
		t.Fatalf("want no item inside grace period")
	}
}

func TestStatusBarItemNothingWhenIdle(t *testing.T) {
	r := NewRegistry(NewFakeClock(time.Unix(1000, 0)))
	if _, ok := StatusBarItem(r, "F"); ok {
		t.Fatalf("want no item when registry empty")
	}
}

// snap is a shorthand to build a fakeActivity with a preset snapshot.
func snap(t *testing.T, id string, k Kind, s Snapshot) *fakeActivity {
	t.Helper()
	a := newFakeActivity(id, k)
	a.setSnap(s)
	return a
}
