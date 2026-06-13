package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/receipt-service/internal/types"
)

func TestOutboxStoreAuditOutboxReturnsLatestRowsIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetReceiptTables(t, ctx, pool)

	seedReceiptOutboxWithStatus(t, ctx, pool, "receipt-event-201", "tenant-a", "conversation-a", 11, types.ReceiptEventMessageReceived, types.OutboxStatusPending, 2, "retry later")
	seedReceiptOutboxWithStatus(t, ctx, pool, "receipt-event-202", "tenant-a", "conversation-a", 12, types.ReceiptEventMessageRead, types.OutboxStatusDLQ, 4, "malformed payload")

	rows, err := NewOutboxStore(pool).AuditOutbox(ctx, OutboxAuditOptions{
		TenantID: "tenant-a",
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("audit outbox: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected two rows, got %d", len(rows))
	}
	if rows[0].EventID != "receipt-event-202" || rows[0].Status != types.OutboxStatusDLQ || rows[0].RetryCount != 4 {
		t.Fatalf("unexpected latest outbox audit row: %+v", rows[0])
	}
	if rows[1].EventID != "receipt-event-201" || rows[1].Status != types.OutboxStatusPending || rows[1].RetryCount != 2 {
		t.Fatalf("unexpected older outbox audit row: %+v", rows[1])
	}
}

func TestOutboxStoreAuditOutboxFiltersStatusAndEventTypeIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetReceiptTables(t, ctx, pool)

	seedReceiptOutboxWithStatus(t, ctx, pool, "receipt-event-211", "tenant-b", "conversation-a", 21, types.ReceiptEventMessageReceived, types.OutboxStatusPending, 1, "retry later")
	seedReceiptOutboxWithStatus(t, ctx, pool, "receipt-event-212", "tenant-b", "conversation-a", 22, types.ReceiptEventMessageRead, types.OutboxStatusPublished, 0, "")
	seedReceiptOutboxWithStatus(t, ctx, pool, "receipt-event-213", "tenant-b", "conversation-a", 23, types.ReceiptEventMessageRead, types.OutboxStatusDLQ, 3, "decode failed")

	rows, err := NewOutboxStore(pool).AuditOutbox(ctx, OutboxAuditOptions{
		TenantID:  "tenant-b",
		Status:    types.OutboxStatusDLQ,
		EventType: types.ReceiptEventMessageRead,
		Limit:     10,
	})
	if err != nil {
		t.Fatalf("audit outbox by status and event type: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one row, got %d", len(rows))
	}
	if rows[0].EventID != "receipt-event-213" || rows[0].Status != types.OutboxStatusDLQ || rows[0].EventType != types.ReceiptEventMessageRead {
		t.Fatalf("unexpected filtered outbox audit row: %+v", rows[0])
	}
}

func seedReceiptOutboxWithStatus(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	eventID string,
	tenantID string,
	conversationID string,
	aggregateVersion int64,
	eventType string,
	status string,
	retryCount int,
	lastError string,
) {
	t.Helper()

	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	var publishedAt any
	var deadLetteredAt any
	if status == types.OutboxStatusPublished {
		publishedAt = now
	}
	if status == types.OutboxStatusDLQ {
		deadLetteredAt = now
	}
	_, err := pool.Exec(ctx, `
INSERT INTO receipt_outbox (
    event_id,
    event_type,
    event_version,
    tenant_id,
    conversation_id,
    aggregate_version,
    partition_key,
    mapping_version,
    correlation_id,
    causation_id,
    producer,
    trace_id,
    payload_json,
    status,
    retry_count,
    available_at,
    next_retry_at,
    dead_lettered_at,
    published_at
) VALUES (
    $1, $2, '1.0.0', $3, $4, $5, $6, 1, 'corr-'+$1, 'cause-'+$1, 'receipt-service', 'trace-'+$1,
    '{"conversation_id":"`+conversationID+`"}'::jsonb,
    $7, $8, $9, $10, $11, $12
)
`,
		eventID,
		eventType,
		tenantID,
		conversationID,
		aggregateVersion,
		tenantID+":"+conversationID,
		status,
		retryCount,
		now.Add(-time.Minute),
		now,
		deadLetteredAt,
		publishedAt,
	)
	if err != nil {
		t.Fatalf("seed receipt outbox %s: %v", eventID, err)
	}
}
