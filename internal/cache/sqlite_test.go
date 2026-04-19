package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func openTestDB(t *testing.T) *DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := OpenDB(path)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestSQLiteStoreSatisfiesStore(t *testing.T) {
	db := openTestDB(t)
	var _ Store[string] = NewStore[string](db, "test")
}

func TestSQLiteStoreGetSet(t *testing.T) {
	db := openTestDB(t)
	s := NewStore[string](db, "strings")

	if _, ok := s.Get("missing"); ok {
		t.Fatal("expected miss for empty store")
	}

	s.Set("k", []string{"a", "b"})
	got, ok := s.Get("k")
	if !ok {
		t.Fatal("expected hit after Set")
	}
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("got %v, want [a b]", got)
	}

	s.Set("k", []string{"x"})
	got, _ = s.Get("k")
	if len(got) != 1 || got[0] != "x" {
		t.Fatalf("got %v after overwrite, want [x]", got)
	}
}

func TestSQLiteStoreNullByteKeys(t *testing.T) {
	db := openTestDB(t)
	s := NewStore[string](db, "nullkeys")

	key := Key("sub-123", "account", "container")
	s.Set(key, []string{"blob1", "blob2"})

	got, ok := s.Get(key)
	if !ok {
		t.Fatal("expected hit for null-byte key")
	}
	if len(got) != 2 || got[0] != "blob1" {
		t.Fatalf("got %v, want [blob1 blob2]", got)
	}

	// Different key should not match.
	if _, ok := s.Get(Key("sub-123", "other")); ok {
		t.Fatal("expected miss for different key")
	}
}

type testStruct struct {
	Name      string    `json:"name"`
	Count     int64     `json:"count"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
}

func TestSQLiteStoreStructRoundTrip(t *testing.T) {
	db := openTestDB(t)
	s := NewStore[testStruct](db, "structs")

	now := time.Now().Truncate(time.Second)
	items := []testStruct{
		{Name: "foo", Count: 42, Enabled: true, CreatedAt: now},
		{Name: "bar", Count: 0, Enabled: false, CreatedAt: now.Add(-time.Hour)},
	}

	s.Set("k", items)
	got, ok := s.Get("k")
	if !ok {
		t.Fatal("expected hit")
	}
	if len(got) != 2 {
		t.Fatalf("got %d items, want 2", len(got))
	}
	if got[0].Name != "foo" || got[0].Count != 42 || !got[0].Enabled {
		t.Fatalf("first item mismatch: %+v", got[0])
	}
	if got[1].Name != "bar" || got[1].Count != 0 || got[1].Enabled {
		t.Fatalf("second item mismatch: %+v", got[1])
	}
}

func TestSQLiteStorePersistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "persist.db")

	// Write with one DB connection.
	db1, err := OpenDB(path)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	s1 := NewStore[string](db1, "data")
	s1.Set("key", []string{"persisted"})
	db1.Close()

	// Read with a new DB connection.
	db2, err := OpenDB(path)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db2.Close()
	s2 := NewStore[string](db2, "data")
	got, ok := s2.Get("key")
	if !ok {
		t.Fatal("expected data to persist across connections")
	}
	if len(got) != 1 || got[0] != "persisted" {
		t.Fatalf("got %v, want [persisted]", got)
	}
}

func TestOpenDBCreatesDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "dir")
	path := filepath.Join(dir, "cache.db")

	db, err := OpenDB(path)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Fatal("expected directory to be created")
	}
}

func TestDefaultDBPath(t *testing.T) {
	path, err := DefaultDBPath()
	if err != nil {
		t.Fatalf("DefaultDBPath: %v", err)
	}
	if filepath.Base(path) != "cache.db" {
		t.Fatalf("expected cache.db, got %s", filepath.Base(path))
	}
	if !filepath.IsAbs(path) {
		t.Fatalf("expected absolute path, got %s", path)
	}
}

func TestUsageRecordAndTopOrdering(t *testing.T) {
	db := openTestDB(t)

	// Touch "queue-a" three times, "queue-b" once.
	db.RecordUsage("sb_queue", "sub1/ns/queue-a", "sub1", "ns / queue-a")
	db.RecordUsage("sb_queue", "sub1/ns/queue-a", "sub1", "ns / queue-a")
	db.RecordUsage("sb_queue", "sub1/ns/queue-a", "sub1", "ns / queue-a")
	db.RecordUsage("sb_queue", "sub1/ns/queue-b", "sub1", "ns / queue-b")

	got := db.TopUsage("sub1", "sb_queue", 10)
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2", len(got))
	}
	if got[0].ResourceKey != "sub1/ns/queue-a" || got[0].Count != 3 {
		t.Errorf("first entry wrong: %+v", got[0])
	}
	if got[1].ResourceKey != "sub1/ns/queue-b" || got[1].Count != 1 {
		t.Errorf("second entry wrong: %+v", got[1])
	}
}

func TestUsageScopedBySubAndType(t *testing.T) {
	db := openTestDB(t)
	db.RecordUsage("sb_queue", "k1", "sub1", "d1")
	db.RecordUsage("sb_queue", "k2", "sub2", "d2")
	db.RecordUsage("blob_container", "k3", "sub1", "d3")

	if got := db.TopUsage("sub1", "sb_queue", 10); len(got) != 1 || got[0].ResourceKey != "k1" {
		t.Errorf("sub1 sb_queue: %+v", got)
	}
	if got := db.TopUsage("sub1", "", 10); len(got) != 2 {
		t.Errorf("sub1 all types: %d entries, want 2", len(got))
	}
}

func TestUsageClear(t *testing.T) {
	db := openTestDB(t)
	db.RecordUsage("sb_queue", "k", "sub1", "d")
	db.ClearUsage("sub1", "sb_queue")
	if got := db.TopUsage("sub1", "sb_queue", 10); len(got) != 0 {
		t.Errorf("expected empty after clear, got %+v", got)
	}
}

func TestPreferencesRoundTrip(t *testing.T) {
	db := openTestDB(t)
	if _, ok := db.GetPreference("missing"); ok {
		t.Errorf("expected missing pref to return ok=false")
	}
	db.SetPreference("k", "v1")
	v, ok := db.GetPreference("k")
	if !ok || v != "v1" {
		t.Errorf("got (%q, %v), want (v1, true)", v, ok)
	}
	db.SetPreference("k", "v2")
	v, _ = db.GetPreference("k")
	if v != "v2" {
		t.Errorf("expected upsert to v2, got %q", v)
	}
}
