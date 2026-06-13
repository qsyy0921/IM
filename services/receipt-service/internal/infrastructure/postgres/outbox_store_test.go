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

func TestOutboxStoreRepairDLQEventResetsStatusAndWritesAuditIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetReceiptTables(t, ctx, pool)
	store := NewOutboxStore(pool)

	seedReceiptOutboxWithStatus(t, ctx, pool, "receipt-event-221", "tenant-repair", "conversation-a", 31, types.ReceiptEventMessageRead, types.OutboxStatusDLQ, 3, "publish failed")

	stats, err := store.RepairDLQEvents(ctx, []string{"receipt-event-221", "receipt-event-221", "missing-event"}, "operator retried after kafka recovery")
	if err != nil {
		t.Fatalf("repair dlq: %v", err)
	}
	if stats.Requested != 2 || stats.Repaired != 1 || stats.Skipped != 1 {
		t.Fatalf("unexpected repair stats: %+v", stats)
	}
	assertReceiptOutboxState(t, ctx, pool, "receipt-event-221", types.OutboxStatusPending, 0, "")
	assertReceiptOutboxRepairAudit(t, ctx, pool, "receipt-event-221", "operator retried after kafka recovery", "publish failed")

	publisherCalled := false
	relayStats, err := store.ProcessReadyBatch(ctx, 10, 5, time.Millisecond, func(ctx context.Context, messages []types.OutboxMessage) []error {
		publisherCalled = true
		if len(messages) != 1 || messages[0].EventID != "receipt-event-221" {
			t.Fatalf("unexpected repaired batch: %+v", messages)
		}
		return []error{nil}
	})
	if err != nil {
		t.Fatalf("publish repaired row: %v", err)
	}
	if !publisherCalled || relayStats.Fetched != 1 || relayStats.Published != 1 {
		t.Fatalf("unexpected relay stats after repair: %+v called=%t", relayStats, publisherCalled)
	}
}

func TestOutboxStoreAuditOutboxRepairsFiltersEventAndTenantIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetReceiptTables(t, ctx, pool)

	_, err := pool.Exec(ctx, `
INSERT INTO receipt_outbox_repair_audit (
    event_id, tenant_id, previous_status, previous_retry_count, previous_last_error, previous_dead_lettered_at, repair_reason, repaired_at
) VALUES
    ('repair-event-1', 'tenant-a', 'DLQ', 1, 'publish failed', now() - interval '1 minute', 'manual audit', now()),
    ('repair-event-2', 'tenant-b', 'DLQ', 2, 'provider unavailable', now() - interval '2 minutes', 'provider recovered', now() - interval '1 minute')
`)
	if err != nil {
		t.Fatalf("seed receipt outbox repair audit: %v", err)
	}

	rows, err := NewOutboxStore(pool).AuditOutboxRepairs(ctx, OutboxRepairAuditOptions{
		EventID:  "repair-event-1",
		TenantID: "tenant-a",
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("audit receipt outbox repairs with filters: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one row, got %d", len(rows))
	}
	if rows[0].EventID != "repair-event-1" || rows[0].TenantID != "tenant-a" || rows[0].Reason != "manual audit" {
		t.Fatalf("unexpected filtered receipt outbox repair audit row: %+v", rows[0])
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

func assertReceiptOutboxState(t *testing.T, ctx context.Context, pool *pgxpool.Pool, eventID string, wantStatus string, wantRetryCount int, wantLastError string) {
	t.Helper()
	var status string
	var retryCount int
	var lastError string
	err := pool.QueryRow(ctx, `
SELECT status, retry_count, last_error
FROM receipt_outbox
WHERE event_id = $1
`, eventID).Scan(&status, &retryCount, &lastError)
	if err != nil {
		t.Fatalf("query receipt outbox state: %v", err)
	}
	if status != wantStatus || retryCount != wantRetryCount || lastError != wantLastError {
		t.Fatalf("unexpected receipt outbox state status=%q retry_count=%d last_error=%q", status, retryCount, lastError)
	}
}

func assertReceiptOutboxRepairAudit(t *testing.T, ctx context.Context, pool *pgxpool.Pool, eventID string, wantReason string, wantPreviousError string) {
	t.Helper()
	var reason string
	var previousStatus string
	var previousRetryCount int
	var previousError string
	err := pool.QueryRow(ctx, `
SELECT repair_reason, previous_status, previous_retry_count, previous_last_error
FROM receipt_outbox_repair_audit
WHERE event_id = $1
`, eventID).Scan(&reason, &previousStatus, &previousRetryCount, &previousError)
	if err != nil {
		t.Fatalf("query receipt outbox repair audit: %v", err)
	}
	if reason != wantReason || previousStatus != types.OutboxStatusDLQ || previousRetryCount != 3 || previousError != wantPreviousError {
		t.Fatalf(
			"unexpected receipt outbox repair audit reason=%q previous_status=%q previous_retry_count=%d previous_error=%q",
			reason,
			previousStatus,
			previousRetryCount,
			previousError,
		)
	}
}
