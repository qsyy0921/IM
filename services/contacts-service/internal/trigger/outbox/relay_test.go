package outbox

import (
	"context"
	"encoding/json"
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

type fakeStore struct {
	messages []types.OutboxMessage
}

func (store *fakeStore) ProcessReadyBatch(
	ctx context.Context,
	limit int,
	maxAttempts int,
	retryBaseDelay time.Duration,
	publish func(context.Context, []types.OutboxMessage) []error,
) (types.OutboxRelayStats, error) {
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
