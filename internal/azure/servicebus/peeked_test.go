package servicebus

import (
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
)

func TestPeekedFromReceivedCarriesMetadata(t *testing.T) {
	seq := int64(42)
	enqSeq := int64(41)
	enqueued := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	expires := enqueued.Add(14 * 24 * time.Hour)
	locked := enqueued.Add(time.Minute)
	scheduled := enqueued.Add(-time.Hour)
	ttl := 14 * 24 * time.Hour
	msg := &azservicebus.ReceivedMessage{
		MessageID:                  "m-1",
		Body:                       []byte(`{"ok":true}`),
		DeliveryCount:              3,
		SequenceNumber:             &seq,
		EnqueuedTime:               &enqueued,
		ContentType:                strPtr("application/json"),
		CorrelationID:              strPtr("corr-9"),
		Subject:                    strPtr("order-created"),
		SessionID:                  strPtr("sess-1"),
		DeadLetterReason:           strPtr("MaxDeliveryCountExceeded"),
		DeadLetterErrorDescription: strPtr("gave up after 10 tries"),
		DeadLetterSource:           strPtr("orders"),
		EnqueuedSequenceNumber:     &enqSeq,
		ExpiresAt:                  &expires,
		LockedUntil:                &locked,
		LockToken:                  [16]byte{0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc, 0xde, 0xf0, 0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc, 0xde, 0xf0},
		PartitionKey:               strPtr("pk-1"),
		ReplyTo:                    strPtr("replies"),
		ReplyToSessionID:           strPtr("rsess-1"),
		ScheduledEnqueueTime:       &scheduled,
		TimeToLive:                 &ttl,
		To:                         strPtr("dest"),
		State:                      azservicebus.MessageStateScheduled,
		ApplicationProperties: map[string]any{
			"tenant": "htg",
			"retry":  7,
		},
	}

	got := peekedFromReceived(msg)

	if got.ContentType != "application/json" || got.CorrelationID != "corr-9" ||
		got.Subject != "order-created" || got.SessionID != "sess-1" {
		t.Fatalf("metadata not carried: %+v", got)
	}
	if got.DeadLetterReason != "MaxDeliveryCountExceeded" ||
		got.DeadLetterDescription != "gave up after 10 tries" ||
		got.DeadLetterSource != "orders" {
		t.Fatalf("dead-letter fields not carried: %+v", got)
	}
	if got.AppProperties["tenant"] != "htg" || got.AppProperties["retry"] != "7" {
		t.Fatalf("application properties not stringified: %v", got.AppProperties)
	}
	if got.SequenceNumber != 42 || !got.EnqueuedAt.Equal(enqueued) || got.DeliveryCount != 3 {
		t.Fatalf("core fields regressed: %+v", got)
	}
	if got.EnqueuedSequenceNumber != 41 || !got.ExpiresAt.Equal(expires) || !got.LockedUntil.Equal(locked) ||
		!got.ScheduledEnqueueTime.Equal(scheduled) || got.TimeToLive != ttl {
		t.Fatalf("broker timing fields not carried: %+v", got)
	}
	if got.PartitionKey != "pk-1" || got.ReplyTo != "replies" || got.ReplyToSessionID != "rsess-1" || got.To != "dest" {
		t.Fatalf("broker routing fields not carried: %+v", got)
	}
	if got.LockToken != "12345678-9abc-def0-1234-56789abcdef0" {
		t.Fatalf("lock token = %q", got.LockToken)
	}
	if got.State != "Scheduled" {
		t.Fatalf("state = %q, want Scheduled", got.State)
	}
}

func TestPeekedFromReceivedNilPointers(t *testing.T) {
	got := peekedFromReceived(&azservicebus.ReceivedMessage{MessageID: "m-2"})
	if got.ContentType != "" || got.DeadLetterReason != "" || got.AppProperties != nil {
		t.Fatalf("nil SDK pointers should map to zero values: %+v", got)
	}
	if got.LockToken != "" || !got.ExpiresAt.IsZero() || got.TimeToLive != 0 || got.State != "Active" {
		t.Fatalf("unset broker fields should stay zero (state defaults to Active): %+v", got)
	}
}
