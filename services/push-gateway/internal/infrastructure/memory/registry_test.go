package memory

import (
	"context"
	"testing"

	"github.com/qsyy0921/IM/services/push-gateway/internal/types"
)

func TestRegistryEnqueueNotificationDeduplicatesPerSession(t *testing.T) {
	registry := NewRegistry()
	outbound := make(chan types.ServerFrame, 2)
	auth := types.AuthContext{TenantID: "tenant-1", UserID: "user-1", DeviceID: "device-1"}
	if err := registry.Register(context.Background(), types.SessionRegistration{
		AuthContext: auth,
		SessionID:   "session-1",
		Outbound:    outbound,
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	notification := testNotification()
	first, err := registry.EnqueueNotification(context.Background(), notification)
	if err != nil {
		t.Fatalf("first enqueue: %v", err)
	}
	second, err := registry.EnqueueNotification(context.Background(), notification)
	if err != nil {
		t.Fatalf("second enqueue: %v", err)
	}
	if first.Enqueued != 1 || second.Enqueued != 0 || len(outbound) != 1 {
		t.Fatalf("unexpected first=%+v second=%+v queue=%d", first, second, len(outbound))
	}
}

func TestRegistryEnqueueNotificationFailsClosedWhenQueueFull(t *testing.T) {
	registry := NewRegistry()
	outbound := make(chan types.ServerFrame)
	evicted := make(chan types.SessionEviction, 1)
	auth := types.AuthContext{TenantID: "tenant-1", UserID: "user-1", DeviceID: "device-1"}
	if err := registry.Register(context.Background(), types.SessionRegistration{
		AuthContext: auth,
		SessionID:   "session-1",
		Outbound:    outbound,
		Evicted:     evicted,
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	result, err := registry.EnqueueNotification(context.Background(), testNotification())
	if err != nil {
		t.Fatalf("queue full should evict session and let consumer commit: result=%+v err=%v", result, err)
	}
	if result.Dropped != 1 || result.Enqueued != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}
	select {
	case eviction := <-evicted:
		if eviction.Reason != "slow_session" ||
			len(eviction.Conversations) != 0 {
			t.Fatalf("unexpected eviction: %+v", eviction)
		}
	default:
		t.Fatalf("expected eviction signal")
	}
	next, err := registry.EnqueueNotification(context.Background(), testNotification())
	if err != nil {
		t.Fatalf("second enqueue: %v", err)
	}
	if next.MatchedSessions != 0 {
		t.Fatalf("full queue session should have been evicted: %+v", next)
	}
}

func testNotification() types.DeliveryNotification {
	return types.DeliveryNotification{
		EventID:         "delivery-event-1",
		TenantID:        "tenant-1",
		UserID:          "user-1",
		ConversationID:  "conversation-1",
		ConversationSeq: 7,
		SourceEventID:   "timeline-event-1",
		MessageID:       "message-1",
	}
}
