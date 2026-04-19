package cache

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// DB wraps a SQLite database connection for persistent caching.
type DB struct {
	db *sql.DB
}

// DefaultDBPath returns the default cache database path (~/.lazyaz/cache.db).
func DefaultDBPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".lazyaz", "cache.db"), nil
}

// OpenDB opens or creates a SQLite database at the given path.
// The parent directory is created if it doesn't exist.
func OpenDB(path string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create cache directory: %w", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open cache database: %w", err)
	}
	if _, err := db.Exec("PRAGMA journal_mode=WAL; PRAGMA synchronous=NORMAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set database pragmas: %w", err)
	}
	return &DB{db: db}, nil
}

// Close closes the database connection.
func (d *DB) Close() error {
	return d.db.Close()
}

// preferencesTable is a single key/value table used for app-wide
// preferences that don't fit the per-resource cache model — last used
// subscription, last opened tab kind, etc. Created lazily on first
// access so callers don't pay for it if they never use it.
const preferencesTable = "preferences"

// GetPreference returns the stored value for key and whether it exists.
// Errors are silently treated as "not found" — preferences are
// best-effort UX hints, not load-bearing data.
func (d *DB) GetPreference(key string) (string, bool) {
	if d == nil || d.db == nil {
		return "", false
	}
	d.ensurePreferences()
	var value string
	err := d.db.QueryRow(
		fmt.Sprintf(`SELECT value FROM %q WHERE key = ?`, preferencesTable),
		key,
	).Scan(&value)
	if err != nil {
		return "", false
	}
	return value, true
}

// SetPreference upserts a value for key. Failures are swallowed for the
// same reason as GetPreference — never block UX on a preference write.
func (d *DB) SetPreference(key, value string) {
	if d == nil || d.db == nil {
		return
	}
	d.ensurePreferences()
	d.db.Exec(
		fmt.Sprintf(`INSERT OR REPLACE INTO %q (key, value, updated_at) VALUES (?, ?, ?)`, preferencesTable),
		key, value, time.Now().Unix(),
	)
}

func (d *DB) ensurePreferences() {
	d.db.Exec(fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %q (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL,
		updated_at INTEGER NOT NULL
	)`, preferencesTable))
}

// UsageEntry is one row in the usage stats table — a resource the user
// has touched, with how many times and when last.
type UsageEntry struct {
	ResourceType string
	ResourceKey  string // stable identifier (e.g., "subID/nsName/queueName")
	SubID        string
	Display      string // human label shown in the dashboard widget
	Count        int64
	LastUsedAt   int64 // unix seconds
}

const usageTable = "usage_stats"

// RecordUsage upserts a usage event: increments count, refreshes
// last_used_at, updates the display label (in case it changed). Errors
// are silently ignored — usage tracking is best-effort UX, not critical.
func (d *DB) RecordUsage(resourceType, resourceKey, subID, display string) {
	if d == nil || d.db == nil {
		return
	}
	d.ensureUsage()
	now := time.Now().Unix()
	d.db.Exec(
		fmt.Sprintf(`INSERT INTO %q (resource_type, resource_key, sub_id, display, count, last_used_at)
			VALUES (?, ?, ?, ?, 1, ?)
			ON CONFLICT(resource_type, resource_key) DO UPDATE SET
				count = count + 1,
				last_used_at = excluded.last_used_at,
				display = excluded.display,
				sub_id = excluded.sub_id`, usageTable),
		resourceType, resourceKey, subID, display, now,
	)
}

// TopUsage returns up to limit entries for the given subscription and
// resource type, ordered by count desc with last_used_at as the
// tiebreaker. Empty resourceType returns rows for all types in that
// subscription (useful for combined widgets).
func (d *DB) TopUsage(subID, resourceType string, limit int) []UsageEntry {
	if d == nil || d.db == nil || limit <= 0 {
		return nil
	}
	d.ensureUsage()
	q := fmt.Sprintf(`SELECT resource_type, resource_key, sub_id, display, count, last_used_at
		FROM %q WHERE sub_id = ?`, usageTable)
	args := []any{subID}
	if resourceType != "" {
		q += ` AND resource_type = ?`
		args = append(args, resourceType)
	}
	q += ` ORDER BY count DESC, last_used_at DESC LIMIT ?`
	args = append(args, limit)

	rows, err := d.db.Query(q, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var out []UsageEntry
	for rows.Next() {
		var e UsageEntry
		if err := rows.Scan(&e.ResourceType, &e.ResourceKey, &e.SubID, &e.Display, &e.Count, &e.LastUsedAt); err != nil {
			continue
		}
		out = append(out, e)
	}
	return out
}

// ClearUsage removes all usage rows for a subscription, optionally
// filtered to one resource type. Used by the dashboard's
// "Clear usage stats" action.
func (d *DB) ClearUsage(subID, resourceType string) {
	if d == nil || d.db == nil {
		return
	}
	d.ensureUsage()
	if resourceType == "" {
		d.db.Exec(fmt.Sprintf(`DELETE FROM %q WHERE sub_id = ?`, usageTable), subID)
		return
	}
	d.db.Exec(
		fmt.Sprintf(`DELETE FROM %q WHERE sub_id = ? AND resource_type = ?`, usageTable),
		subID, resourceType,
	)
}

func (d *DB) ensureUsage() {
	d.db.Exec(fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %q (
		resource_type TEXT NOT NULL,
		resource_key TEXT NOT NULL,
		sub_id TEXT NOT NULL,
		display TEXT NOT NULL,
		count INTEGER NOT NULL DEFAULT 0,
		last_used_at INTEGER NOT NULL,
		PRIMARY KEY (resource_type, resource_key)
	)`, usageTable))
	d.db.Exec(fmt.Sprintf(`CREATE INDEX IF NOT EXISTS %s_sub_idx ON %q (sub_id, resource_type, count DESC, last_used_at DESC)`,
		usageTable, usageTable))
}

func (d *DB) createTable(name string) {
	d.db.Exec(fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %q (
		key BLOB PRIMARY KEY,
		data TEXT NOT NULL,
		updated_at INTEGER NOT NULL
	)`, name))
}

// SQLiteStore is a persistent [Store] backed by a SQLite table.
// Each key maps to a JSON-encoded slice of items.
type SQLiteStore[T any] struct {
	db    *DB
	table string
}

// NewStore creates a [SQLiteStore] for the named table.
// The table is created if it does not exist.
func NewStore[T any](db *DB, table string) *SQLiteStore[T] {
	db.createTable(table)
	return &SQLiteStore[T]{db: db, table: table}
}

// Get returns the cached items for the given key and whether they exist.
func (s *SQLiteStore[T]) Get(key string) ([]T, bool) {
	var data string
	err := s.db.db.QueryRow(
		fmt.Sprintf(`SELECT data FROM %q WHERE key = ?`, s.table),
		[]byte(key),
	).Scan(&data)
	if err != nil {
		return nil, false
	}
	var items []T
	if err := json.Unmarshal([]byte(data), &items); err != nil {
		return nil, false
	}
	return items, true
}

// Set stores items under the given key, replacing any previous value.
func (s *SQLiteStore[T]) Set(key string, items []T) {
	data, err := json.Marshal(items)
	if err != nil {
		return
	}
	s.db.db.Exec(
		fmt.Sprintf(`INSERT OR REPLACE INTO %q (key, data, updated_at) VALUES (?, ?, ?)`, s.table),
		[]byte(key), string(data), time.Now().Unix(),
	)
}

// Clear removes all entries from the table.
func (s *SQLiteStore[T]) Clear() {
	s.db.db.Exec(fmt.Sprintf(`DELETE FROM %q`, s.table))
}
