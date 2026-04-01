// Package cache provides a pluggable caching layer for hierarchical
// Azure resource trees. The [Store] interface defines the contract;
// [Map] is a simple in-memory implementation.
package cache

// Store is the interface for a single-level cache in the resource hierarchy.
// Implementations must be safe for sequential use within a Bubble Tea
// update loop (concurrent safety is not required).
type Store[T any] interface {
	Get(key string) ([]T, bool)
	Set(key string, items []T)
}

// Map is an in-memory [Store] backed by a plain Go map.
type Map[T any] struct {
	entries map[string][]T
}

// NewMap creates an empty in-memory Store.
func NewMap[T any]() *Map[T] {
	return &Map[T]{entries: make(map[string][]T)}
}

// Get returns the cached items for the given key and whether they exist.
func (c *Map[T]) Get(key string) ([]T, bool) {
	items, ok := c.entries[key]
	return items, ok
}

// Set stores items under the given key, replacing any previous value.
func (c *Map[T]) Set(key string, items []T) {
	c.entries[key] = items
}

// Key joins segments with a null byte separator to form a cache key.
func Key(parts ...string) string {
	if len(parts) == 0 {
		return ""
	}
	if len(parts) == 1 {
		return parts[0]
	}
	n := len(parts) - 1
	for _, p := range parts {
		n += len(p)
	}
	b := make([]byte, 0, n)
	for i, p := range parts {
		if i > 0 {
			b = append(b, 0)
		}
		b = append(b, p...)
	}
	return string(b)
}
