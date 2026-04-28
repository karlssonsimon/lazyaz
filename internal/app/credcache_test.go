package app

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
)

type fakeCred struct{ tenant string }

func (fakeCred) GetToken(_ context.Context, _ policy.TokenRequestOptions) (azcore.AccessToken, error) {
	return azcore.AccessToken{}, nil
}

func TestCredentialCache_ForReusesInstance(t *testing.T) {
	calls := int32(0)
	c := newCredentialCache(func(id string) (azcore.TokenCredential, error) {
		atomic.AddInt32(&calls, 1)
		return fakeCred{tenant: id}, nil
	})
	a, err := c.For("t1")
	if err != nil {
		t.Fatalf("first For: %v", err)
	}
	b, err := c.For("t1")
	if err != nil {
		t.Fatalf("second For: %v", err)
	}
	if a != b {
		t.Fatalf("expected same instance, got %p vs %p", a, b)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("expected factory called once, got %d", calls)
	}
}

func TestCredentialCache_ForDifferentTenants(t *testing.T) {
	c := newCredentialCache(func(id string) (azcore.TokenCredential, error) {
		return fakeCred{tenant: id}, nil
	})
	a, _ := c.For("t1")
	b, _ := c.For("t2")
	if a == b {
		t.Fatalf("expected distinct instances for different tenants")
	}
}

func TestCredentialCache_FactoryError(t *testing.T) {
	wantErr := errors.New("boom")
	c := newCredentialCache(func(id string) (azcore.TokenCredential, error) {
		return nil, wantErr
	})
	_, err := c.For("t1")
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected wrapped factory error, got %v", err)
	}
}

func TestCredentialCache_ResetClears(t *testing.T) {
	calls := int32(0)
	c := newCredentialCache(func(id string) (azcore.TokenCredential, error) {
		atomic.AddInt32(&calls, 1)
		return fakeCred{tenant: id}, nil
	})
	_, _ = c.For("t1")
	c.Reset()
	_, _ = c.For("t1")
	if atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("expected factory called twice after Reset, got %d", calls)
	}
}

func TestCredentialCache_ConcurrentColdCacheSingleCreate(t *testing.T) {
	calls := int32(0)
	c := newCredentialCache(func(id string) (azcore.TokenCredential, error) {
		atomic.AddInt32(&calls, 1)
		return fakeCred{tenant: id}, nil
	})
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, _ = c.For("t1") }()
	}
	wg.Wait()
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("expected factory called exactly once under concurrency, got %d", calls)
	}
}
