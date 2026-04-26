package keyvault

import (
	"context"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
)

type testCredential struct{ id string }

func (testCredential) GetToken(context.Context, policy.TokenRequestOptions) (azcore.AccessToken, error) {
	return azcore.AccessToken{}, nil
}

func TestCredentialReturnsCurrentCredential(t *testing.T) {
	initial := testCredential{id: "initial"}
	replacement := testCredential{id: "replacement"}
	svc := NewService(initial)

	if got := svc.Credential(); got != initial {
		t.Fatalf("Credential() = %#v, want initial credential", got)
	}

	svc.SetCredential(replacement)
	if got := svc.Credential(); got != replacement {
		t.Fatalf("Credential() = %#v, want replacement credential", got)
	}
}
