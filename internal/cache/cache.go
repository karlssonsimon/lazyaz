// Package cache provides a generic in-memory cache for hierarchical
// Azure resource trees. Each CacheMap stores slices of items keyed by
// a composite string built from parent identifiers, enabling
// stale-while-revalidate patterns across navigation levels.
package cache

// Map is a typed cache for one level of a resource hierarchy.
// Keys are composite strings built from parent identifiers
// (e.g. subscriptionID or subscriptionID + namespaceName).
type Map[T any] struct {
	entries map[string][]T
}

// NewMap creates an empty Map ready for use.
func NewMap[T any]() Map[T] {
	return Map[T]{entries: make(map[string][]T)}
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
