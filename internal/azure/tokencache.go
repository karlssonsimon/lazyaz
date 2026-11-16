package azure

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
)

// tokenRefreshMargin is how long before expiry a cached token stops being
// handed out. It is wider than the five minutes at which the SDK's own
// policies ask for a replacement, so a token they consider stale is never
// served from here.
const tokenRefreshMargin = 10 * time.Minute

// cachingCredential remembers the tokens of the credential it wraps.
//
// The Azure CLI credential runs `az` for every token, and not every caller
// keeps what it gets: the Service Bus client asks again for each link it
// opens, and an ARM client built per call starts with an empty cache. That
// put a process spawn in front of most actions.
type cachingCredential struct {
	inner azcore.TokenCredential

	mu     sync.Mutex
	tokens map[string]azcore.AccessToken
}

func newCachingCredential(inner azcore.TokenCredential) *cachingCredential {
	return &cachingCredential{inner: inner, tokens: map[string]azcore.AccessToken{}}
}

// GetToken returns the cached token for the request while it is comfortably
// valid, and otherwise fetches one. The lock is held across the fetch so
// callers that arrive together share one fetch instead of each starting
// their own.
func (c *cachingCredential) GetToken(ctx context.Context, opts policy.TokenRequestOptions) (azcore.AccessToken, error) {
	// A claims challenge asks for a token the cached one cannot satisfy.
	if opts.Claims != "" {
		return c.inner.GetToken(ctx, opts)
	}

	key := strings.Join(opts.Scopes, " ") + "|" + opts.TenantID
	if opts.EnableCAE {
		key += "|cae"
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if tok, ok := c.tokens[key]; ok && time.Until(tok.ExpiresOn) > tokenRefreshMargin {
		return tok, nil
	}
	tok, err := c.inner.GetToken(ctx, opts)
	if err != nil {
		return azcore.AccessToken{}, err
	}
	c.tokens[key] = tok
	return tok, nil
}
