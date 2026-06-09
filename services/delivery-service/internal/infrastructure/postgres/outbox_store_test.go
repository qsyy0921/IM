package postgres

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/delivery-service/internal/types"
)

func TestOutboxStoreProcessReadyBatchPublishesAndRetriesIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetDeliveryTables(t, ctx, pool)
	seedDeliveryOutbox(t, ctx, pool, "event-1", "conversation-a", 1, types.DeliveryEventInboxItemCreated)
	seedDeliveryOutbox(t, ctx, pool, "event-2", "conversation-b", 1, types.DeliveryEventAckRecorded)

	store := NewOutboxStore(pool, WithOutboxClock(func() time.Time {
		return time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	}))
	stats, err := store.ProcessReadyBatch(ctx, 10, 3, time.Millisecond, func(ctx context.Context, messages []types.OutboxMessage) []error {
		errs := make([]error, len(messages))
		for index, message := range messages {
			if message.ConversationID == "conversation-b" {
				errs[index] = errors.New("kafka unavailable")
			}
		}
		return errs
	})
	if err != nil {
		t.Fatalf("process outbox: %v", err)
	}
	if stats.Fetched != 2 || stats.Published != 1 || stats.Retried != 1 || stats.DeadLettered != 0 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	assertDeliveryOutboxStatus(t, ctx, pool, "event-1", types.OutboxStatusPublished, 0)
	assertDeliveryOutboxStatus(t, ctx, pool, "event-2", types.OutboxStatusPending, 1)
}

func TestOutboxStoreProcessReadyBatchDeadLettersAndBlocksLaterVersionIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetDeliveryTables(t, ctx, pool)
	seedDeliveryOutbox(t, ctx, pool, "event-1", "conversation-a", 1, types.DeliveryEventInboxItemCreated)
	seedDeliveryOutbox(t, ctx, pool, "event-2", "conversation-a", 2, types.DeliveryEventInboxItemCreated)

	store := NewOutboxStore(pool, WithOutboxClock(func() time.Time {
		return time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	}))
	stats, err := store.ProcessReadyBatch(ctx, 10, 1, time.Millisecond, func(ctx context.Context, messages []types.OutboxMessage) []error {
		errs := make([]error, len(messages))
		for index := range errs {
			errs[index] = errors.New("malformed payload")
		}
		return errs
	})
	if err != nil {
		t.Fatalf("process outbox: %v", err)
	}
	if stats.Fetched != 1 || stats.DeadLettered != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	assertDeliveryOutboxStatus(t, ctx, pool, "event-1", types.OutboxStatusDLQ, 1)

	stats, err = store.ProcessReadyBatch(ctx, 10, 3, time.Millisecond, func(ctx context.Context, messages []types.OutboxMessage) []error {
		return make([]error, len(messages))
	})
	if err != nil {
		t.Fatalf("process blocked outbox: %v", err)
	}
	if stats.Fetched != 0 {
		t.Fatalf("later version should be blocked by lower DLQ, got stats=%+v", stats)
	}
	assertDeliveryOutboxStatus(t, ctx, pool, "event-2", types.OutboxStatusPending, 0)
}

func seedDeliveryOutbox(t *testing.T, ctx context.Context, pool *pgxpool.Pool, eventID string, conversationID string, version int64, eventType string) {
	t.Helper()
	payload := fmt.Sprintf(`{"tenant_id":"tenant-delivery","user_id":"user-1","device_id":"device-1","conversation_id":%q,"conversation_seq":%d,"source_event_id":%q,"message_id":%q,"last_received_seq":%d}`, conversationID, version, "source-"+eventID, "message-"+eventID, version)
	_, err := pool.Exec(ctx, `
INSERT INTO delivery_outbox (
    event_id,
    tenant_id,
    conversation_id,
    aggregate_version,
    event_type,
    event_version,
    partition_key,
    mapping_version,
    correlation_id,
    causation_id,
    producer,
    trace_id,
    payload_json,
    status,
    available_at
) VALUES ($1, 'tenant-delivery', $2, $3, $4, '1.0.0', 'tenant-delivery:' || $2, 1, 'request-1', 'source-' || $1, 'delivery-service', 'trace-1', $5::jsonb, 'PENDING', now())
`, eventID, conversationID, version, eventType, payload)
	if err != nil {
		t.Fatalf("seed delivery outbox: %v", err)
	}
}

func assertDeliveryOutboxStatus(t *testing.T, ctx context.Context, pool *pgxpool.Pool, eventID string, wantStatus string, wantRetry int) {
	t.Helper()
	var status string
	var retryCount int
	err := pool.QueryRow(ctx, `
SELECT status, retry_count
FROM delivery_outbox
WHERE event_id = $1
`, eventID).Scan(&status, &retryCount)
	if err != nil {
		t.Fatalf("read delivery outbox status: %v", err)
	}
	if status != wantStatus || retryCount != wantRetry {
		t.Fatalf("unexpected status for %s: status=%s retry=%d want status=%s retry=%d", eventID, status, retryCount, wantStatus, wantRetry)
	}
}
