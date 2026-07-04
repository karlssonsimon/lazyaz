package servicebus

import (
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
)

func TestPeekedFromReceivedCarriesMetadata(t *testing.T) {
	seq := int64(42)
	enqueued := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
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
}

func TestPeekedFromReceivedNilPointers(t *testing.T) {
	got := peekedFromReceived(&azservicebus.ReceivedMessage{MessageID: "m-2"})
	if got.ContentType != "" || got.DeadLetterReason != "" || got.AppProperties != nil {
		t.Fatalf("nil SDK pointers should map to zero values: %+v", got)
	}
}
