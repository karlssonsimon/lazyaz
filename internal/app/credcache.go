package app

import (
	"fmt"
	"sync"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
)

// credentialFactory builds a token credential scoped to the given
// tenant ID. Injected so tests don't hit azidentity.
type credentialFactory func(tenantID string) (azcore.TokenCredential, error)

// credentialCache memoizes one azcore.TokenCredential per tenant ID
// for the lifetime of the process. The cache is never persisted —
// credentials carry tokens and refresh tokens that belong on the
// keyring, not in our SQLite prefs.
type credentialCache struct {
	factory credentialFactory

	mu      sync.Mutex
	entries map[string]*credentialCacheEntry
}

type credentialCacheEntry struct {
	once sync.Once
	cred azcore.TokenCredential
	err  error
}

func newCredentialCache(factory credentialFactory) *credentialCache {
	return &credentialCache{
		factory: factory,
		entries: make(map[string]*credentialCacheEntry),
	}
}

// For returns the credential for tenantID, creating it on first call.
// Repeated calls return the same instance. Concurrent cold-cache
// callers for the same tenant share one factory invocation.
func (c *credentialCache) For(tenantID string) (azcore.TokenCredential, error) {
	c.mu.Lock()
	entry, ok := c.entries[tenantID]
	if !ok {
		entry = &credentialCacheEntry{}
		c.entries[tenantID] = entry
	}
	c.mu.Unlock()

	entry.once.Do(func() {
		cred, err := c.factory(tenantID)
		if err != nil {
			entry.err = fmt.Errorf("credential for tenant %s: %w", tenantID, err)
			return
		}
		entry.cred = cred
	})
	return entry.cred, entry.err
}

// Reset drops every cached credential. Used after az login when the
// underlying session has changed and the old credentials may be invalid.
func (c *credentialCache) Reset() {
	c.mu.Lock()
	c.entries = make(map[string]*credentialCacheEntry)
	c.mu.Unlock()
}
