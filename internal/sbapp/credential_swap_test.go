package sbapp

import (
	"context"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/karlssonsimon/lazyaz/internal/azure/servicebus"
)

type swapTestCredential struct{ id string }

func (swapTestCredential) GetToken(context.Context, policy.TokenRequestOptions) (azcore.AccessToken, error) {
	return azcore.AccessToken{}, nil
}

// Held locks belong to the connections a credential swap replaces, so the
// swap releases them and clears the pane instead of leaving messages on
// screen that can no longer be settled.
func TestSetCredentialDropsHeldLocks(t *testing.T) {
	m := NewModel(servicebus.NewService(swapTestCredential{id: "old"}), testConfig, nil)
	m.hasPeekTarget = true
	m.currentNS = servicebus.Namespace{Name: "ns"}
	m.currentEntity = servicebus.Entity{Name: "queue"}
	m.lockedMessages = &servicebus.ReceivedMessages{}
	m.peekedMessages = []servicebus.PeekedMessage{{MessageID: "m-1"}}
	m.messageList.SetItems(m.messageItems())

	next := swapTestCredential{id: "new"}
	m.SetCredential(next)

	if m.HoldsLocks() {
		t.Error("locks still held after the credential changed")
	}
	if len(m.peekedMessages) != 0 || len(m.messageList.Items()) != 0 {
		t.Errorf("locked view not cleared: %d peeked, %d rows", len(m.peekedMessages), len(m.messageList.Items()))
	}
	if got := m.service.Credential(); got != next {
		t.Errorf("service credential = %#v, want the new one", got)
	}
}

// Without held locks a swap leaves the view alone: peeked messages are
// only a snapshot and stay valid to look at.
func TestSetCredentialKeepsPeekedView(t *testing.T) {
	m := NewModel(servicebus.NewService(swapTestCredential{id: "old"}), testConfig, nil)
	m.peekedMessages = []servicebus.PeekedMessage{{MessageID: "m-1"}}
	m.messageList.SetItems(m.messageItems())

	next := swapTestCredential{id: "new"}
	m.SetCredential(next)

	if len(m.peekedMessages) != 1 || len(m.messageList.Items()) != 1 {
		t.Errorf("peeked view changed: %d peeked, %d rows", len(m.peekedMessages), len(m.messageList.Items()))
	}
	if got := m.service.Credential(); got != next {
		t.Errorf("service credential = %#v, want the new one", got)
	}
}
