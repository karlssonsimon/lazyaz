package blobapp

import (
	"errors"
	"testing"
	"time"

	"github.com/karlssonsimon/lazyaz/internal/azure/blob"
)

// Spinner notifications never auto-expire — every StartLoading must be
// paired with a ResolveSpinner/DismissSpinner or the toast stays on
// screen forever. These tests drive the prefix-search flow end to end
// and assert nothing is left active long after every regular toast
// would have expired.
func assertNoStuckToast(t *testing.T, m Model, flow string) {
	t.Helper()
	farFuture := time.Now().Add(time.Hour)
	if m.Notifier.HasActive(farFuture) {
		t.Fatalf("%s: notification stuck — a spinner was never resolved or dismissed", flow)
	}
}

func TestPrefixSearchDoneResolvesSpinner(t *testing.T) {
	m := NewModel(nil, testConfig, nil)
	m.hasAccount = true
	m.currentAccount = blob.Account{Name: "acct", SubscriptionID: "sub-1"}
	m.hasContainer = true
	m.containerName = "data"
	m.filter.prefixQuery = "logs/"

	cmd := m.firePrefixSearch()
	if cmd == nil || !m.Loading {
		t.Fatal("firePrefixSearch should start a load")
	}

	updated, _ := m.handleBlobsLoaded(blobsLoadedMsg{
		account:   m.currentAccount,
		container: "data",
		query:     "logs/",
		blobs:     []blob.BlobEntry{{Name: "logs/a.txt"}},
		done:      true,
	})
	assertNoStuckToast(t, updated, "prefix search done")
}

func TestPrefixSearchErrorResolvesSpinner(t *testing.T) {
	m := NewModel(nil, testConfig, nil)
	m.hasAccount = true
	m.currentAccount = blob.Account{Name: "acct", SubscriptionID: "sub-1"}
	m.hasContainer = true
	m.containerName = "data"
	m.filter.prefixQuery = "logs/"

	if cmd := m.firePrefixSearch(); cmd == nil {
		t.Fatal("firePrefixSearch should start a load")
	}

	updated, _ := m.handleBlobsLoaded(blobsLoadedMsg{
		account:   m.currentAccount,
		container: "data",
		query:     "logs/",
		err:       errors.New("boom"),
	})
	assertNoStuckToast(t, updated, "prefix search error")
}
