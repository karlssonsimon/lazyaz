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

// DefaultDBPath returns the default cache database path (~/.aztui/cache.db).
func DefaultDBPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".aztui", "cache.db"), nil
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
