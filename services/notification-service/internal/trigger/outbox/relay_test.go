package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	notificationeventsv1 "github.com/qsyy0921/IM/schemas/kafka/notification/v1"
	"github.com/qsyy0921/IM/services/notification-service/internal/types"
	"google.golang.org/protobuf/proto"
)

func TestBuildNotificationEventAccepted(t *testing.T) {
	value, err := BuildKafkaValue(notificationOutboxMessage(types.NotificationEventRequestAccepted, acceptedPayload()))
	if err != nil {
		t.Fatalf("build kafka value: %v", err)
	}
	var event notificationeventsv1.NotificationEvent
	if err := proto.Unmarshal(value, &event); err != nil {
		t.Fatalf("decode notification event: %v", err)
	}
	accepted := event.GetRequestAccepted()
	if accepted == nil {
		t.Fatalf("expected request accepted payload: %+v", &event)
	}
	if event.EventId != "notif-1:accepted" ||
		event.EventType != types.NotificationEventRequestAccepted ||
		event.PartitionKey != "tenant-notification:notif-1" ||
		event.Producer != "notification-service" ||
		event.TraceId != "trace-1" ||
		event.CorrelationId != "corr-1" ||
		event.CausationId != "cause-1" ||
		accepted.RequestId != "notif-1" ||
		accepted.DestinationMasked != "u***@example.com" {
		t.Fatalf("unexpected event: %+v payload=%+v", &event, accepted)
	}
}

func TestBuildNotificationEventRejectsSensitivePayloadFields(t *testing.T) {
	for _, field := range []string{"destination_ref", "destination_hash", "secret_payload_ciphertext", "provider_body", "recovery_code"} {
		payload := acceptedPayload()
		payload[field] = "must-not-leak"
		if _, err := BuildNotificationEvent(notificationOutboxMessage(types.NotificationEventRequestAccepted, payload)); err == nil {
			t.Fatalf("expected sensitive field %s to be rejected", field)
		}
	}
}

func TestBuildNotificationEventRejectsUnsupportedAndMalformed(t *testing.T) {
	if _, err := BuildNotificationEvent(notificationOutboxMessage("notification.future.v9", acceptedPayload())); err == nil {
		t.Fatalf("expected unsupported event type to fail")
	}
	message := notificationOutboxMessage(types.NotificationEventRequestAccepted, acceptedPayload())
	message.PayloadJSON = []byte(`{"tenant_id":`)
	if _, err := BuildNotificationEvent(message); err == nil {
		t.Fatalf("expected malformed payload to fail")
	}
}

func TestRelayRunOncePublishesOnlyBuildableMessages(t *testing.T) {
	store := &fakeStore{
		messages: []types.OutboxMessage{
			notificationOutboxMessage(types.NotificationEventRequestAccepted, acceptedPayload()),
			notificationOutboxMessage("notification.future.v9", map[string]any{"tenant_id": "tenant-notification"}),
		},
	}
	publisher := &fakePublisher{}
	relay := NewRelay(store, publisher, Config{})
	stats, err := relay.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("run relay once: %v", err)
	}
	if stats.Fetched != 2 || stats.Published != 1 || stats.Retried != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	if len(publisher.records) != 1 {
		t.Fatalf("expected one published record, got %d", len(publisher.records))
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
			continue
		}
		stats.Retried++
	}
	if len(errs) != len(store.messages) {
		return types.OutboxRelayStats{}, errors.New("mismatched publish result")
	}
	return stats, nil
}

type fakePublisher struct {
	records []types.KafkaPublishRecord
}

func (publisher *fakePublisher) PublishBatch(_ context.Context, _ string, records []types.KafkaPublishRecord) error {
	publisher.records = append(publisher.records, records...)
	return nil
}

func notificationOutboxMessage(eventType string, payload map[string]any) types.OutboxMessage {
	payloadJSON, _ := json.Marshal(payload)
	return types.OutboxMessage{
		EventID:          "notif-1:accepted",
		TenantID:         "tenant-notification",
		RequestID:        "notif-1",
		EventType:        eventType,
		EventVersion:     1,
		PartitionKey:     "tenant-notification:notif-1",
		Producer:         "notification-service",
		PayloadJSON:      payloadJSON,
		OccurredAt:       time.Date(2026, 6, 20, 8, 0, 0, 0, time.UTC),
		AggregateVersion: 1,
	}
}

func acceptedPayload() map[string]any {
	return map[string]any{
		"tenant_id":          "tenant-notification",
		"request_id":         "notif-1",
		"requester_service":  "identity-service",
		"channel":            "EMAIL",
		"recipient_ref":      "user:user-1",
		"destination_masked": "u***@example.com",
		"template_key":       "verify-email",
		"template_version":   "v1",
		"locale":             "und",
		"priority":           "NORMAL",
		"status":             "ACCEPTED",
		"correlation_id":     "corr-1",
		"causation_id":       "cause-1",
		"trace_id":           "trace-1",
	}
}
