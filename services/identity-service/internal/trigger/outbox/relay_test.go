package outbox

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	identityeventsv1 "github.com/qsyy0921/IM/schemas/kafka/identity/v1"
	"github.com/qsyy0921/IM/services/identity-service/internal/types"
)

func TestBuildIdentityEventDeviceRevoked(t *testing.T) {
	event, err := BuildIdentityEvent(baseMessage(types.IdentityEventDeviceRevoked, []byte(`{
        "tenant_id":"tenant-1",
        "user_id":"user-1",
        "device_id":"device-1",
        "status":"REVOKED",
        "revoked_by":"admin-1",
        "reason":"lost",
        "revoked_at":"2026-06-12T01:02:03Z"
    }`)))
	if err != nil {
		t.Fatalf("build event: %v", err)
	}
	payload, ok := event.Payload.(*identityeventsv1.IdentityEvent_DeviceRevoked)
	if !ok {
		t.Fatalf("expected device revoked payload, got %T", event.Payload)
	}
	if payload.DeviceRevoked.DeviceId != "device-1" || payload.DeviceRevoked.RevokedBy != "admin-1" {
		t.Fatalf("unexpected payload: %+v", payload.DeviceRevoked)
	}
}

func TestBuildIdentityEventSessionRevoked(t *testing.T) {
	event, err := BuildIdentityEvent(baseMessage(types.IdentityEventSessionRevoked, []byte(`{
        "tenant_id":"tenant-1",
        "user_id":"user-1",
        "device_id":"device-1",
        "session_id":"session-1",
        "status":"REVOKED",
        "revoked_by":"admin-1",
        "reason":"manual",
        "revoked_at":"2026-06-12T01:02:03Z"
    }`)))
	if err != nil {
		t.Fatalf("build event: %v", err)
	}
	payload, ok := event.Payload.(*identityeventsv1.IdentityEvent_SessionRevoked)
	if !ok {
		t.Fatalf("expected session revoked payload, got %T", event.Payload)
	}
	if payload.SessionRevoked.SessionId != "session-1" || payload.SessionRevoked.DeviceId != "device-1" {
		t.Fatalf("unexpected payload: %+v", payload.SessionRevoked)
	}
}

func TestBuildIdentityEventRejectsUnsupportedType(t *testing.T) {
	_, err := BuildIdentityEvent(baseMessage("identity.unknown.v1", []byte(`{}`)))
	if err == nil {
		t.Fatal("expected unsupported event error")
	}
}

func TestRelayRunOnceFailClosedForMalformedPayload(t *testing.T) {
	store := &fakeStore{
		messages: []types.OutboxMessage{
			baseMessage(types.IdentityEventDeviceRevoked, []byte(`{"tenant_id":"tenant-1"}`)),
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
			baseMessage(types.IdentityEventSessionRevoked, []byte(`{
				"tenant_id":"tenant-1",
				"user_id":"user-1",
				"device_id":"device-1",
				"session_id":"session-1",
				"status":"REVOKED",
				"revoked_by":"admin-1",
				"reason":"manual",
				"revoked_at":"2026-06-12T01:02:03Z"
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
			baseMessage(types.IdentityEventSessionRevoked, []byte(`{
				"tenant_id":"tenant-1",
				"user_id":"user-1",
				"device_id":"device-1",
				"session_id":"session-1",
				"status":"REVOKED",
				"revoked_by":"admin-1",
				"reason":"manual",
				"revoked_at":"2026-06-12T01:02:03Z"
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

func baseMessage(eventType string, payload []byte) types.OutboxMessage {
	return types.OutboxMessage{
		ID:               1,
		EventID:          "event-1",
		EventType:        eventType,
		EventVersion:     "v1",
		TenantID:         "tenant-1",
		AggregateType:    "identity_device",
		AggregateID:      "user-1:device-1",
		AggregateVersion: 1,
		PartitionKey:     "tenant-1:user-1:device-1",
		MappingVersion:   1,
		Producer:         "identity-service",
		PayloadJSON:      payload,
		OccurredAt:       time.Date(2026, 6, 12, 1, 2, 3, 0, time.UTC),
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
