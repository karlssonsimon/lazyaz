package servicebus

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
	"github.com/Azure/azure-sdk-for-go/sdk/monitor/azquery"
)

type testCredential struct{ id string }

func (testCredential) GetToken(context.Context, policy.TokenRequestOptions) (azcore.AccessToken, error) {
	return azcore.AccessToken{}, nil
}

func TestReceivedMessagesCompleteByIDRequiresReceiver(t *testing.T) {
	locked := &ReceivedMessages{messages: []LockedMessage{{ID: "m1", raw: &azservicebus.ReceivedMessage{MessageID: "m1"}}}}

	err := locked.CompleteByID(context.Background(), "m1")
	if err == nil || !strings.Contains(err.Error(), "receiver") {
		t.Fatalf("CompleteByID error = %v, want helpful receiver error", err)
	}
}

func TestReceivedMessagesCompleteByIDReportsMissingID(t *testing.T) {
	locked := &ReceivedMessages{messages: []LockedMessage{{ID: "m1"}}}

	err := locked.CompleteByID(context.Background(), "missing")
	if err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("CompleteByID error = %v, want missing ID error", err)
	}
}

func TestReceivedMessagesCompleteByIDCompletesDuplicateIDsOnceEach(t *testing.T) {
	first := &azservicebus.ReceivedMessage{MessageID: "dup", Body: []byte("first")}
	second := &azservicebus.ReceivedMessage{MessageID: "dup", Body: []byte("second")}
	locked := &ReceivedMessages{messages: []LockedMessage{
		{ID: "dup", raw: first},
		{ID: "dup", raw: nil},
		{ID: "dup", raw: second},
	}}

	var completed []*azservicebus.ReceivedMessage
	complete := func(_ context.Context, msg *azservicebus.ReceivedMessage) error {
		completed = append(completed, msg)
		return nil
	}

	if err := locked.completeByID(context.Background(), "dup", complete); err != nil {
		t.Fatalf("first completeByID failed: %v", err)
	}
	if err := locked.completeByID(context.Background(), "dup", complete); err != nil {
		t.Fatalf("second completeByID failed: %v", err)
	}
	if err := locked.completeByID(context.Background(), "dup", complete); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("third completeByID error = %v, want not found", err)
	}

	if len(completed) != 2 || completed[0] != first || completed[1] != second {
		t.Fatalf("completed = %#v, want first then second distinct raw messages", completed)
	}
	if locked.messages[0].raw != nil || locked.messages[2].raw != nil {
		t.Fatalf("completed raw messages were not marked consumed")
	}
}

func TestReceivedMessagesCompleteByIDUsesLockIDBeforeMessageID(t *testing.T) {
	first := &azservicebus.ReceivedMessage{MessageID: "dup", Body: []byte("first")}
	second := &azservicebus.ReceivedMessage{MessageID: "dup", Body: []byte("second")}
	locked := &ReceivedMessages{messages: []LockedMessage{
		{ID: "dup", LockID: "0", raw: first},
		{ID: "dup", LockID: "1", raw: second},
	}}

	var completed []*azservicebus.ReceivedMessage
	complete := func(_ context.Context, msg *azservicebus.ReceivedMessage) error {
		completed = append(completed, msg)
		return nil
	}

	if err := locked.completeByID(context.Background(), "1", complete); err != nil {
		t.Fatalf("completeByID by LockID failed: %v", err)
	}
	if len(completed) != 1 || completed[0] != second {
		t.Fatalf("completed = %#v, want second raw message", completed)
	}
	if locked.messages[0].raw == nil || locked.messages[1].raw != nil {
		t.Fatalf("completeByID consumed wrong raw messages")
	}
}

func TestReceivedMessagesSnapshotRemoveAndLenUseOperationIDs(t *testing.T) {
	locked := &ReceivedMessages{messages: []LockedMessage{
		{ID: "dup", LockID: "0", raw: &azservicebus.ReceivedMessage{MessageID: "dup"}},
		{ID: "dup", LockID: "1", raw: &azservicebus.ReceivedMessage{MessageID: "dup"}},
	}}

	snapshot := locked.MessagesSnapshot()
	if len(snapshot) != 2 {
		t.Fatalf("MessagesSnapshot length = %d, want 2", len(snapshot))
	}
	snapshot[0].ID = "changed"
	if got := locked.MessagesSnapshot()[0].ID; got != "dup" {
		t.Fatalf("MessagesSnapshot exposed internal slice, got ID %q", got)
	}

	if !locked.RemoveByID("1") {
		t.Fatalf("RemoveByID returned false for existing LockID")
	}
	if got := locked.Len(); got != 1 {
		t.Fatalf("Len after RemoveByID = %d, want 1", got)
	}
	remaining := locked.MessagesSnapshot()
	if len(remaining) != 1 || remaining[0].LockID != "0" {
		t.Fatalf("remaining messages = %#v, want only LockID 0", remaining)
	}
	if locked.RemoveByID("dup") {
		t.Fatalf("RemoveByID by MessageID matched despite LockID being present")
	}
}

func TestReceivedMessagesCompleteByIDSerializesRawMutation(t *testing.T) {
	locked := &ReceivedMessages{messages: []LockedMessage{{
		ID:  "m1",
		raw: &azservicebus.ReceivedMessage{MessageID: "m1"},
	}}}
	ctx := context.Background()
	started := make(chan struct{})
	release := make(chan struct{})

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = locked.completeByID(ctx, "m1", func(context.Context, *azservicebus.ReceivedMessage) error {
			close(started)
			<-release
			return nil
		})
	}()
	<-started

	secondDone := make(chan error, 1)
	go func() {
		secondDone <- locked.completeByID(ctx, "m1", func(context.Context, *azservicebus.ReceivedMessage) error {
			return nil
		})
	}()

	select {
	case err := <-secondDone:
		t.Fatalf("second completeByID finished before first released lock: %v", err)
	case <-time.After(20 * time.Millisecond):
	}

	close(release)
	wg.Wait()
	if err := <-secondDone; err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("second completeByID error = %v, want not found after serialized first completion", err)
	}
}

func TestReceivedMessagesPeekedMessagesPreservesDisplayFields(t *testing.T) {
	enqueued := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	locked := &ReceivedMessages{messages: []LockedMessage{{
		ID:     "m1",
		LockID: "0",
		raw: &azservicebus.ReceivedMessage{
			MessageID:     "m1",
			Body:          []byte("hello"),
			DeliveryCount: 5,
			EnqueuedTime:  &enqueued,
		},
	}}}

	got := locked.PeekedMessages()
	if len(got) != 1 {
		t.Fatalf("PeekedMessages length = %d, want 1", len(got))
	}
	if got[0].MessageID != "m1" || got[0].LockID != "0" || got[0].DeliveryCount != 5 || got[0].FullBody != "hello" || got[0].BodyPreview != "hello" || !got[0].EnqueuedAt.Equal(enqueued) {
		t.Fatalf("PeekedMessages()[0] = %#v, want display fields preserved", got[0])
	}
}

func TestLockedMessagesUseUniqueOperationIDsAcrossSessions(t *testing.T) {
	first := lockReceivedMessages([]*azservicebus.ReceivedMessage{
		{MessageID: "same"},
		{MessageID: "same"},
	})
	second := lockReceivedMessages([]*azservicebus.ReceivedMessage{
		{MessageID: "same"},
		{MessageID: "same"},
	})

	seen := make(map[string]struct{})
	for _, locked := range append(first, second...) {
		if locked.LockID == "" {
			t.Fatalf("LockID is empty for %#v", locked)
		}
		if _, ok := seen[locked.LockID]; ok {
			t.Fatalf("duplicate LockID %q across receive sessions", locked.LockID)
		}
		seen[locked.LockID] = struct{}{}
	}
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

func TestGetMetricsClientRetriesWhenCredentialGenerationChangesDuringCreate(t *testing.T) {
	initial := testCredential{id: "initial"}
	replacement := testCredential{id: "replacement"}
	svc := NewService(initial)
	staleClient := &azquery.MetricsClient{}
	currentClient := &azquery.MetricsClient{}

	originalFactory := newMetricsClient
	defer func() { newMetricsClient = originalFactory }()
	var credentials []azcore.TokenCredential
	newMetricsClient = func(cred azcore.TokenCredential, _ *azquery.MetricsClientOptions) (*azquery.MetricsClient, error) {
		credentials = append(credentials, cred)
		if len(credentials) == 1 {
			svc.SetCredential(replacement)
			return staleClient, nil
		}
		return currentClient, nil
	}

	got, err := svc.getMetricsClient()
	if err != nil {
		t.Fatalf("getMetricsClient failed: %v", err)
	}
	if got != currentClient {
		t.Fatalf("getMetricsClient returned stale client %#v, want current client %#v", got, currentClient)
	}
	if len(credentials) != 2 || credentials[0] != initial || credentials[1] != replacement {
		t.Fatalf("credentials used = %#v, want initial then replacement", credentials)
	}
	if svc.metricsClient != currentClient {
		t.Fatalf("cached metrics client = %#v, want current client", svc.metricsClient)
	}
}

func TestParseResourceGroup(t *testing.T) {
	tests := []struct {
		name string
		id   *string
		want string
	}{
		{
			name: "nil id",
			id:   nil,
			want: "",
		},
		{
			name: "valid id",
			id:   strPtr("/subscriptions/abc/resourceGroups/rg-prod/providers/Microsoft.ServiceBus/namespaces/sb-demo"),
			want: "rg-prod",
		},
		{
			name: "missing resource groups segment",
			id:   strPtr("/subscriptions/abc/providers/Microsoft.ServiceBus/namespaces/sb-demo"),
			want: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseResourceGroup(tc.id); got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestEndpointToFQDN(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		want     string
	}{
		{
			name:     "https with trailing slash",
			endpoint: "https://sb-demo.servicebus.windows.net/",
			want:     "sb-demo.servicebus.windows.net",
		},
		{
			name:     "https with port",
			endpoint: "https://sb-demo.servicebus.windows.net:443",
			want:     "sb-demo.servicebus.windows.net",
		},
		{
			name:     "plain fqdn",
			endpoint: "sb-demo.servicebus.windows.net",
			want:     "sb-demo.servicebus.windows.net",
		},
		{
			name:     "empty",
			endpoint: "",
			want:     "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := endpointToFQDN(tc.endpoint); got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestTruncateBody(t *testing.T) {
	tests := []struct {
		name string
		body []byte
		max  int
		want string
	}{
		{
			name: "empty body",
			body: nil,
			max:  512,
			want: "",
		},
		{
			name: "short body",
			body: []byte("hello"),
			max:  512,
			want: "hello",
		},
		{
			name: "exact limit",
			body: []byte("abc"),
			max:  3,
			want: "abc",
		},
		{
			name: "truncated",
			body: []byte("abcdef"),
			max:  3,
			want: "abc...",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := truncateBody(tc.body, tc.max); got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestIsAuthError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "unrelated error",
			err:  fmt.Errorf("something went wrong"),
			want: false,
		},
		{
			name: "service bus unauthorized access",
			err:  &azservicebus.Error{Code: azservicebus.CodeUnauthorizedAccess},
			want: true,
		},
		{
			name: "service bus other code",
			err:  &azservicebus.Error{Code: azservicebus.CodeNotFound},
			want: false,
		},
		{
			name: "wrapped service bus unauthorized",
			err:  fmt.Errorf("peek failed: %w", &azservicebus.Error{Code: azservicebus.CodeUnauthorizedAccess}),
			want: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isAuthError(tc.err); got != tc.want {
				t.Fatalf("isAuthError() = %v, want %v", got, tc.want)
			}
		})
	}
}

func strPtr(v string) *string {
	return &v
}
