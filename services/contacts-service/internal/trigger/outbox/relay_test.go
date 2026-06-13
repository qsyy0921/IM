package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	contacteventsv1 "github.com/qsyy0921/IM/schemas/kafka/contacts/v1"
	"github.com/qsyy0921/IM/services/contacts-service/internal/types"
	"google.golang.org/protobuf/proto"
)

func TestBuildContactEventAccepted(t *testing.T) {
	message := outboxMessage(types.ContactEventRequestAccepted, map[string]any{
		"tenant_id":        "tenant-contacts",
		"request_id":       "request-1",
		"sender_user_id":   "alice",
		"receiver_user_id": "bob",
		"status":           "ACCEPTED",
		"edge_version":     1,
		"occurred_at":      "2026-06-10T08:00:00Z",
	})
	value, err := BuildKafkaValue(message)
	if err != nil {
		t.Fatalf("build kafka value: %v", err)
	}
	var event contacteventsv1.ContactEvent
	if err := proto.Unmarshal(value, &event); err != nil {
		t.Fatalf("decode contact event: %v", err)
	}
	accepted := event.GetRequestAccepted()
	if accepted == nil {
		t.Fatalf("expected accepted payload: %+v", &event)
	}
	if event.EventId != "evt-contact-1" ||
		event.PartitionKey != "tenant-contacts:alice:bob" ||
		accepted.RequestId != "request-1" ||
		accepted.EdgeVersion != 1 {
		t.Fatalf("unexpected event: %+v payload=%+v", &event, accepted)
	}
}

func TestBuildContactEventCanceled(t *testing.T) {
	message := outboxMessage(types.ContactEventRequestCanceled, map[string]any{
		"tenant_id":        "tenant-contacts",
		"request_id":       "request-1",
		"sender_user_id":   "alice",
		"receiver_user_id": "bob",
		"status":           "CANCELED",
		"occurred_at":      "2026-06-10T08:00:00Z",
	})
	value, err := BuildKafkaValue(message)
	if err != nil {
		t.Fatalf("build kafka value: %v", err)
	}
	var event contacteventsv1.ContactEvent
	if err := proto.Unmarshal(value, &event); err != nil {
		t.Fatalf("decode contact event: %v", err)
	}
	canceled := event.GetRequestCanceled()
	if canceled == nil {
		t.Fatalf("expected canceled payload: %+v", &event)
	}
	if event.EventType != types.ContactEventRequestCanceled ||
		canceled.RequestId != "request-1" ||
		canceled.Status != "CANCELED" ||
		canceled.SenderUserId != "alice" ||
		canceled.ReceiverUserId != "bob" {
		t.Fatalf("unexpected canceled event: %+v payload=%+v", &event, canceled)
	}
}

func TestBuildContactEventEdgeBlockedUnblockedAndRemarkUpdated(t *testing.T) {
	blockedMessage := outboxMessage(types.ContactEventEdgeBlocked, map[string]any{
		"tenant_id":       "tenant-contacts",
		"owner_user_id":   "alice",
		"contact_user_id": "bob",
		"previous_status": "ACTIVE",
		"status":          "BLOCKED",
		"edge_version":    2,
		"reason":          "spam",
		"occurred_at":     "2026-06-10T08:00:00Z",
	})
	blockedEvent, err := BuildContactEvent(blockedMessage)
	if err != nil {
		t.Fatalf("build blocked event: %v", err)
	}
	blocked := blockedEvent.GetEdgeBlocked()
	if blocked == nil || blocked.OwnerUserId != "alice" || blocked.ContactUserId != "bob" || blocked.Reason != "spam" || blocked.EdgeVersion != 2 {
		t.Fatalf("unexpected blocked event: %+v payload=%+v", blockedEvent, blocked)
	}

	unblockedMessage := outboxMessage(types.ContactEventEdgeUnblocked, map[string]any{
		"tenant_id":       "tenant-contacts",
		"owner_user_id":   "alice",
		"contact_user_id": "bob",
		"previous_status": "BLOCKED",
		"status":          "ACTIVE",
		"edge_version":    3,
		"occurred_at":     "2026-06-10T08:00:00Z",
	})
	unblockedEvent, err := BuildContactEvent(unblockedMessage)
	if err != nil {
		t.Fatalf("build unblocked event: %v", err)
	}
	unblocked := unblockedEvent.GetEdgeUnblocked()
	if unblocked == nil || unblocked.PreviousStatus != "BLOCKED" || unblocked.Status != "ACTIVE" || unblocked.EdgeVersion != 3 {
		t.Fatalf("unexpected unblocked event: %+v payload=%+v", unblockedEvent, unblocked)
	}

	remarkMessage := outboxMessage(types.ContactEventRemarkUpdated, map[string]any{
		"tenant_id":       "tenant-contacts",
		"owner_user_id":   "alice",
		"contact_user_id": "bob",
		"status":          "ACTIVE",
		"edge_version":    3,
		"remark":          "Bob from school",
		"occurred_at":     "2026-06-10T08:00:00Z",
	})
	remarkEvent, err := BuildContactEvent(remarkMessage)
	if err != nil {
		t.Fatalf("build remark event: %v", err)
	}
	remark := remarkEvent.GetEdgeRemarkUpdated()
	if remark == nil || remark.Remark != "Bob from school" || remark.EdgeVersion != 3 {
		t.Fatalf("unexpected remark event: %+v payload=%+v", remarkEvent, remark)
	}
}

func TestBuildContactEventRejectsMalformedEdgeEvent(t *testing.T) {
	_, err := BuildContactEvent(outboxMessage(types.ContactEventEdgeDeleted, map[string]any{
		"tenant_id":       "tenant-contacts",
		"owner_user_id":   "alice",
		"contact_user_id": "bob",
		"status":          "DELETED",
		"edge_version":    2,
		"occurred_at":     "2026-06-10T08:00:00Z",
	}))
	if err == nil {
		t.Fatal("expected malformed edge deleted event error")
	}
}

func TestBuildContactEventRejectsUnsupportedAndMalformed(t *testing.T) {
	_, err := BuildKafkaValue(outboxMessage("contact.unknown.v1", map[string]any{
		"tenant_id": "tenant-contacts",
	}))
	if err == nil {
		t.Fatal("expected unsupported event error")
	}
	_, err = BuildKafkaValue(outboxMessage(types.ContactEventRequestCreated, map[string]any{
		"tenant_id":   "tenant-contacts",
		"request_id":  "request-1",
		"occurred_at": "bad-time",
	}))
	if err == nil {
		t.Fatal("expected malformed payload error")
	}
}

func TestRelayRunOncePublishesBatch(t *testing.T) {
	store := &fakeStore{
		messages: []types.OutboxMessage{
			outboxMessage(types.ContactEventRequestCreated, map[string]any{
				"tenant_id":        "tenant-contacts",
				"request_id":       "request-1",
				"sender_user_id":   "alice",
				"receiver_user_id": "bob",
				"status":           "PENDING",
				"message":          "hi",
				"occurred_at":      "2026-06-10T08:00:00Z",
			}),
		},
	}
	publisher := &fakePublisher{}
	relay := NewRelay(store, publisher, Config{Topic: "im.contact.events"})
	stats, err := relay.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if stats.Fetched != 1 || stats.Published != 1 || len(publisher.records) != 1 {
		t.Fatalf("unexpected stats=%+v records=%d", stats, len(publisher.records))
	}
}

func TestRelayRetriesTransientRunOnceErrorAndExposesSnapshot(t *testing.T) {
	store := &fakeStore{
		errs: []error{errors.New("temporary store error"), nil},
		messages: []types.OutboxMessage{
			outboxMessage(types.ContactEventRequestCreated, map[string]any{
				"tenant_id":        "tenant-contacts",
				"request_id":       "request-1",
				"sender_user_id":   "alice",
				"receiver_user_id": "bob",
				"status":           "PENDING",
				"message":          "hi",
				"occurred_at":      "2026-06-10T08:00:00Z",
			}),
		},
	}
	publisher := &fakePublisher{}
	relay := NewRelay(store, publisher, Config{
		Topic:        "im.contact.events",
		PollInterval: time.Millisecond,
		ErrorBackoff: time.Millisecond,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		for len(publisher.records) == 0 {
			time.Sleep(time.Millisecond)
		}
		cancel()
	}()

	err := relay.Run(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled after successful retry, got %v", err)
	}
	if store.calls < 2 {
		t.Fatalf("expected retry after transient error, calls=%d", store.calls)
	}
	snapshot := relay.Snapshot()
	if snapshot.TotalErrors != 1 || snapshot.ConsecutiveErrors != 0 {
		t.Fatalf("unexpected relay snapshot: %+v", snapshot)
	}
	if snapshot.LastErrorAtMS == 0 || snapshot.LastSuccessAtMS == 0 || snapshot.LastPublishedAtMS == 0 {
		t.Fatalf("expected timestamps in relay snapshot: %+v", snapshot)
	}
	if snapshot.LastErrorBackoffMS != time.Millisecond.Milliseconds() {
		t.Fatalf("expected backoff %d, got %+v", time.Millisecond.Milliseconds(), snapshot)
	}
}

type fakeStore struct {
	messages []types.OutboxMessage
	errs     []error
	calls    int
}

func (store *fakeStore) ProcessReadyBatch(
	ctx context.Context,
	limit int,
	maxAttempts int,
	retryBaseDelay time.Duration,
	publish func(context.Context, []types.OutboxMessage) []error,
) (types.OutboxRelayStats, error) {
	if err := ctx.Err(); err != nil {
		return types.OutboxRelayStats{}, err
	}
	if store.calls < len(store.errs) {
		err := store.errs[store.calls]
		store.calls++
		if err != nil {
			return types.OutboxRelayStats{}, err
		}
	} else {
		store.calls++
	}
	errs := publish(ctx, store.messages)
	stats := types.OutboxRelayStats{Fetched: len(store.messages)}
	for _, err := range errs {
		if err == nil {
			stats.Published++
		} else {
			stats.Retried++
		}
	}
	return stats, nil
}

type fakePublisher struct {
	records []types.KafkaPublishRecord
	err     error
}

func (publisher *fakePublisher) PublishBatch(ctx context.Context, topic string, records []types.KafkaPublishRecord) error {
	publisher.records = append(publisher.records, records...)
	if publisher.err != nil {
		return publisher.err
	}
	return nil
}

func outboxMessage(eventType string, payload map[string]any) types.OutboxMessage {
	raw, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}
	return types.OutboxMessage{
		ID:               1,
		EventID:          "evt-contact-1",
		TenantID:         "tenant-contacts",
		AggregateType:    "CONTACT_REQUEST",
		AggregateID:      "request-1",
		AggregateVersion: 1,
		EventType:        eventType,
		EventVersion:     "1.0.0",
		PartitionKey:     "tenant-contacts:alice:bob",
		MappingVersion:   1,
		CorrelationID:    "request-1",
		CausationID:      "request-1",
		Producer:         "contacts-service",
		PayloadJSON:      raw,
		TraceID:          "trace-1",
		OccurredAt:       time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC),
	}
}
