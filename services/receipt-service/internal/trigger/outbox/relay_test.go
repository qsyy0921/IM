package outbox

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	receipteventsv1 "github.com/qsyy0921/IM/schemas/kafka/receipt/v1"
	"github.com/qsyy0921/IM/services/receipt-service/internal/types"
	"google.golang.org/protobuf/proto"
)

func TestBuildReceiptEventMessageReceived(t *testing.T) {
	message := testOutboxMessage(types.ReceiptEventMessageReceived, []byte(`{
		"tenant_id":"tenant-1",
		"conversation_id":"conversation-1",
		"conversation_seq":12,
		"message_id":"message-1",
		"user_id":"user-1",
		"device_id":"device-1",
		"cursor_seq":12,
		"source_event_id":"delivery-event-1"
	}`))
	value, err := BuildKafkaValue(message)
	if err != nil {
		t.Fatalf("build kafka value: %v", err)
	}
	var event receipteventsv1.ReceiptEvent
	if err := proto.Unmarshal(value, &event); err != nil {
		t.Fatalf("unmarshal receipt event: %v", err)
	}
	payload := event.GetMessageReceived()
	if payload == nil {
		t.Fatalf("expected received payload")
	}
	if event.EventId != message.EventID ||
		event.EventType != types.ReceiptEventMessageReceived ||
		event.PartitionKey != message.PartitionKey ||
		payload.MessageId != "message-1" ||
		payload.CursorSeq != 12 ||
		payload.DeviceId != "device-1" {
		t.Fatalf("unexpected event=%+v payload=%+v", &event, payload)
	}
}

func TestBuildReceiptEventMessageRead(t *testing.T) {
	message := testOutboxMessage(types.ReceiptEventMessageRead, []byte(`{
		"tenant_id":"tenant-1",
		"conversation_id":"conversation-1",
		"conversation_seq":12,
		"message_id":"message-1",
		"user_id":"user-1",
		"device_id":"device-1",
		"cursor_seq":12,
		"source_event_id":"request-1"
	}`))
	event, err := BuildReceiptEvent(message)
	if err != nil {
		t.Fatalf("build receipt event: %v", err)
	}
	payload := event.GetMessageRead()
	if payload == nil {
		t.Fatalf("expected read payload")
	}
	if payload.UserId != "user-1" ||
		payload.MessageId != "message-1" ||
		payload.CursorSeq != 12 {
		t.Fatalf("unexpected read payload: %+v", payload)
	}
}

func TestRelayRunOnceFailClosedForMalformedPayload(t *testing.T) {
	store := &fakeStore{
		messages: []types.OutboxMessage{
			testOutboxMessage(types.ReceiptEventMessageReceived, []byte(`{"tenant_id":"tenant-1"}`)),
		},
	}
	publisher := &recordingPublisher{}
	relay := NewRelay(store, publisher, Config{BatchSize: 10})
	stats, err := relay.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if stats.Fetched != 1 || stats.Published != 0 || stats.Retried != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	if publisher.Calls() != 0 {
		t.Fatalf("malformed payload must not be published")
	}
}

func TestRelayRunOnceMapsBatchPublishErrorToRetry(t *testing.T) {
	store := &fakeStore{
		messages: []types.OutboxMessage{
			testOutboxMessage(types.ReceiptEventMessageRead, []byte(`{
				"tenant_id":"tenant-1",
				"conversation_id":"conversation-1",
				"conversation_seq":12,
				"message_id":"message-1",
				"user_id":"user-1",
				"device_id":"device-1",
				"cursor_seq":12,
				"source_event_id":"request-1"
			}`)),
		},
	}
	publisher := &recordingPublisher{err: errors.New("kafka unavailable")}
	relay := NewRelay(store, publisher, Config{BatchSize: 10})
	stats, err := relay.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if stats.Fetched != 1 || stats.Published != 0 || stats.Retried != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	if publisher.Calls() != 1 {
		t.Fatalf("expected one publish call, got %d", publisher.Calls())
	}
}

func TestRelayRetriesTransientRunOnceErrorAndExposesSnapshot(t *testing.T) {
	store := &transientErrorStore{
		messages: []types.OutboxMessage{
			testOutboxMessage(types.ReceiptEventMessageRead, []byte(`{
				"tenant_id":"tenant-1",
				"conversation_id":"conversation-1",
				"conversation_seq":12,
				"message_id":"message-1",
				"user_id":"user-1",
				"device_id":"device-1",
				"cursor_seq":12,
				"source_event_id":"request-1"
			}`)),
		},
	}
	publisher := &recordingPublisher{}
	relay := NewRelay(store, publisher, Config{
		BatchSize:    10,
		PollInterval: time.Hour,
		ErrorBackoff: time.Millisecond,
	})
	ctx, cancel := context.WithCancel(context.Background())
	publisher.onPublish = cancel
	done := make(chan error, 1)
	go func() {
		done <- relay.Run(ctx)
	}()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled run, got %v", err)
	}
	cancel()
	if publisher.Calls() != 1 {
		t.Fatalf("expected relay to publish after transient error, got %d calls", publisher.Calls())
	}
	if store.calls.Load() < 2 {
		t.Fatalf("expected relay to retry after transient error")
	}
	snapshot := relay.Snapshot()
	if snapshot.TotalErrors == 0 || snapshot.ConsecutiveErrors != 0 {
		t.Fatalf("unexpected snapshot after recovery: %+v", snapshot)
	}
	if snapshot.LastSuccessAtMS == 0 || snapshot.LastPublishedAtMS == 0 {
		t.Fatalf("expected success/published timestamps in snapshot: %+v", snapshot)
	}
	if snapshot.LastErrorBackoffMS != time.Millisecond.Milliseconds() {
		t.Fatalf("unexpected error backoff snapshot: %+v", snapshot)
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
		if err != nil {
			stats.Retried++
		} else {
			stats.Published++
		}
	}
	return stats, nil
}

type recordingPublisher struct {
	calls     atomic.Int32
	err       error
	onPublish func()
}

func (publisher *recordingPublisher) PublishBatch(ctx context.Context, topic string, records []types.KafkaPublishRecord) error {
	publisher.calls.Add(1)
	if publisher.onPublish != nil {
		publisher.onPublish()
	}
	return publisher.err
}

func (publisher *recordingPublisher) Calls() int {
	return int(publisher.calls.Load())
}

type transientErrorStore struct {
	calls    atomic.Int32
	messages []types.OutboxMessage
}

func (store *transientErrorStore) ProcessReadyBatch(
	ctx context.Context,
	limit int,
	maxAttempts int,
	retryBaseDelay time.Duration,
	publish func(context.Context, []types.OutboxMessage) []error,
) (types.OutboxRelayStats, error) {
	if store.calls.Add(1) == 1 {
		return types.OutboxRelayStats{}, errors.New("temporary store failure")
	}
	errs := publish(ctx, store.messages)
	stats := types.OutboxRelayStats{Fetched: len(store.messages)}
	for _, err := range errs {
		if err != nil {
			stats.Retried++
		} else {
			stats.Published++
		}
	}
	return stats, nil
}

func testOutboxMessage(eventType string, payload []byte) types.OutboxMessage {
	return types.OutboxMessage{
		ID:               1,
		EventID:          "receipt-event-1",
		TenantID:         "tenant-1",
		ConversationID:   "conversation-1",
		AggregateVersion: 12,
		EventType:        eventType,
		EventVersion:     "1.0.0",
		PartitionKey:     "tenant-1:conversation-1",
		MappingVersion:   1,
		CorrelationID:    "request-1",
		CausationID:      "delivery-event-1",
		Producer:         "receipt-service",
		PayloadJSON:      payload,
		TraceID:          "trace-1",
		OccurredAt:       time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC),
	}
}
