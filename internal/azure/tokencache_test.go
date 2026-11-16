package azure

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
)

// countingCredential hands out a new token per call and counts the calls.
type countingCredential struct {
	mu       sync.Mutex
	calls    int
	lifetime time.Duration
	err      error
}

func (c *countingCredential) GetToken(context.Context, policy.TokenRequestOptions) (azcore.AccessToken, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	if c.err != nil {
		return azcore.AccessToken{}, c.err
	}
	return azcore.AccessToken{Token: string(rune('a' + c.calls)), ExpiresOn: time.Now().Add(c.lifetime)}, nil
}

func scope(s string) policy.TokenRequestOptions {
	return policy.TokenRequestOptions{Scopes: []string{s}}
}

func TestCachingCredentialReusesValidToken(t *testing.T) {
	inner := &countingCredential{lifetime: time.Hour}
	cred := newCachingCredential(inner)

	first, err := cred.GetToken(context.Background(), scope("sb"))
	if err != nil {
		t.Fatal(err)
	}
	second, _ := cred.GetToken(context.Background(), scope("sb"))

	if inner.calls != 1 || first.Token != second.Token {
		t.Errorf("inner called %d times, tokens %q and %q; want one fetch reused", inner.calls, first.Token, second.Token)
	}
}

// A token is only good for the request it was issued for.
func TestCachingCredentialKeysOnScopeAndTenant(t *testing.T) {
	inner := &countingCredential{lifetime: time.Hour}
	cred := newCachingCredential(inner)
	ctx := context.Background()

	cred.GetToken(ctx, scope("sb"))
	cred.GetToken(ctx, scope("arm"))
	cred.GetToken(ctx, policy.TokenRequestOptions{Scopes: []string{"sb"}, TenantID: "other"})
	cred.GetToken(ctx, policy.TokenRequestOptions{Scopes: []string{"sb"}, EnableCAE: true})

	if inner.calls != 4 {
		t.Errorf("inner called %d times, want 4 distinct fetches", inner.calls)
	}
}

func TestCachingCredentialRefetchesNearExpiry(t *testing.T) {
	inner := &countingCredential{lifetime: tokenRefreshMargin - time.Minute}
	cred := newCachingCredential(inner)

	cred.GetToken(context.Background(), scope("sb"))
	cred.GetToken(context.Background(), scope("sb"))

	if inner.calls != 2 {
		t.Errorf("inner called %d times, want a fetch each time for a token inside the refresh margin", inner.calls)
	}
}

func TestCachingCredentialDoesNotCacheClaimsOrErrors(t *testing.T) {
	inner := &countingCredential{lifetime: time.Hour}
	cred := newCachingCredential(inner)
	ctx := context.Background()

	withClaims := policy.TokenRequestOptions{Scopes: []string{"sb"}, Claims: "challenge"}
	cred.GetToken(ctx, withClaims)
	cred.GetToken(ctx, withClaims)
	if inner.calls != 2 {
		t.Errorf("inner called %d times for a claims challenge, want 2", inner.calls)
	}

	inner.err = errors.New("not logged in")
	if _, err := cred.GetToken(ctx, scope("arm")); err == nil {
		t.Fatal("error not returned")
	}
	inner.err = nil
	if _, err := cred.GetToken(ctx, scope("arm")); err != nil {
		t.Errorf("a failed fetch was cached: %v", err)
	}
}

// Callers that ask together share one fetch.
func TestCachingCredentialSharesConcurrentFetch(t *testing.T) {
	inner := &countingCredential{lifetime: time.Hour}
	cred := newCachingCredential(inner)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cred.GetToken(context.Background(), scope("sb"))
		}()
	}
	wg.Wait()

	if inner.calls != 1 {
		t.Errorf("inner called %d times, want 1", inner.calls)
	}
}
