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

func TestOutboxStoreRepairAuditsDLQIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetDeliveryTables(t, ctx, pool)
	seedDeliveryOutbox(t, ctx, pool, "event-1", "conversation-a", 1, types.DeliveryEventInboxItemCreated)

	store := NewOutboxStore(pool, WithOutboxClock(func() time.Time {
		return time.Date(2026, 6, 14, 10, 0, 0, 0, time.UTC)
	}))
	stats, err := store.ProcessReadyBatch(ctx, 10, 1, time.Millisecond, func(ctx context.Context, messages []types.OutboxMessage) []error {
		return []error{errors.New("malformed payload")}
	})
	if err != nil {
		t.Fatalf("process outbox: %v", err)
	}
	if stats.DeadLettered != 1 {
		t.Fatalf("expected one dead-lettered outbox row, got %+v", stats)
	}
	outboxID := readDeliveryOutboxID(t, ctx, pool, "event-1")

	repairStats, err := store.RepairOutbox(ctx, types.OutboxRepairOptions{
		OutboxIDs: []int64{outboxID, outboxID, 0},
		Mode:      types.OutboxRepairModeAudit,
		Operator:  "operator-1",
		Reason:    "manual audit",
	})
	if err != nil {
		t.Fatalf("audit outbox: %v", err)
	}
	if repairStats.Requested != 1 || repairStats.Audited != 1 || repairStats.Mutated != 0 || repairStats.Skipped != 0 {
		t.Fatalf("unexpected repair stats: %+v", repairStats)
	}
	assertDeliveryOutboxStatus(t, ctx, pool, "event-1", types.OutboxStatusDLQ, 1)
	assertDeliveryOutboxRepairAudit(t, ctx, pool, outboxID, types.OutboxRepairModeAudit, outboxRepairOutcomeAudited, "", types.OutboxStatusDLQ, 1, "malformed payload", types.OutboxStatusDLQ, 1, "malformed payload", "manual audit")
}

func TestOutboxStoreRepairRedrivesDLQIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetDeliveryTables(t, ctx, pool)
	seedDeliveryOutbox(t, ctx, pool, "event-1", "conversation-a", 1, types.DeliveryEventInboxItemCreated)

	store := NewOutboxStore(pool, WithOutboxClock(func() time.Time {
		return time.Date(2026, 6, 14, 10, 0, 0, 0, time.UTC)
	}))
	_, err := store.ProcessReadyBatch(ctx, 10, 1, time.Millisecond, func(ctx context.Context, messages []types.OutboxMessage) []error {
		return []error{errors.New("malformed payload")}
	})
	if err != nil {
		t.Fatalf("process outbox: %v", err)
	}
	outboxID := readDeliveryOutboxID(t, ctx, pool, "event-1")

	repairStats, err := store.RepairOutbox(ctx, types.OutboxRepairOptions{
		OutboxIDs: []int64{outboxID},
		Mode:      types.OutboxRepairModeRedriveDLQPending,
		Operator:  "operator-1",
		Reason:    "provider recovered",
	})
	if err != nil {
		t.Fatalf("redrive outbox: %v", err)
	}
	if repairStats.Requested != 1 || repairStats.Audited != 0 || repairStats.Mutated != 1 || repairStats.Skipped != 0 {
		t.Fatalf("unexpected repair stats: %+v", repairStats)
	}
	assertDeliveryOutboxStatus(t, ctx, pool, "event-1", types.OutboxStatusPending, 0)
	assertDeliveryOutboxRepairAudit(t, ctx, pool, outboxID, types.OutboxRepairModeRedriveDLQPending, outboxRepairOutcomeMutated, "", types.OutboxStatusDLQ, 1, "malformed payload", types.OutboxStatusPending, 0, "", "provider recovered")
}

func TestOutboxStoreRepairSkipsNonDLQIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetDeliveryTables(t, ctx, pool)
	seedDeliveryOutbox(t, ctx, pool, "event-1", "conversation-a", 1, types.DeliveryEventInboxItemCreated)
	outboxID := readDeliveryOutboxID(t, ctx, pool, "event-1")

	store := NewOutboxStore(pool)
	repairStats, err := store.RepairOutbox(ctx, types.OutboxRepairOptions{
		OutboxIDs: []int64{outboxID},
		Mode:      types.OutboxRepairModeRedriveDLQPending,
		Operator:  "operator-1",
		Reason:    "manual retry",
	})
	if err != nil {
		t.Fatalf("repair pending outbox: %v", err)
	}
	if repairStats.Requested != 1 || repairStats.Audited != 0 || repairStats.Mutated != 0 || repairStats.Skipped != 1 {
		t.Fatalf("unexpected repair stats: %+v", repairStats)
	}
	assertDeliveryOutboxStatus(t, ctx, pool, "event-1", types.OutboxStatusPending, 0)
	assertDeliveryOutboxRepairAudit(t, ctx, pool, outboxID, types.OutboxRepairModeRedriveDLQPending, outboxRepairOutcomeSkipped, outboxRepairSkipStatusNotDLQ, types.OutboxStatusPending, 0, "", types.OutboxStatusPending, 0, "", "manual retry")
}

func TestOutboxStoreAuditOutboxRepairsReturnsLatestRowsIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetDeliveryTables(t, ctx, pool)

	_, err := pool.Exec(ctx, `
INSERT INTO delivery_outbox_repair_audit (
    outbox_id, event_id, tenant_id, conversation_id, aggregate_version, mode, outcome, skip_reason, operator, reason, dry_run,
    before_status, before_retry_count, before_last_error, before_next_retry_at, before_dead_lettered_at,
    after_status, after_retry_count, after_last_error, after_next_retry_at, after_dead_lettered_at, created_at
) VALUES
    (11, 'event-11', 'tenant-a', 'conversation-a', 1, 'audit', 'AUDITED', '', 'operator-a', 'manual audit', true,
     'DLQ', 1, 'malformed payload', NULL, now() - interval '1 minute',
     'DLQ', 1, 'malformed payload', NULL, now() - interval '1 minute', now() - interval '1 minute'),
    (12, 'event-12', 'tenant-a', 'conversation-b', 2, 'redrive-dlq-pending', 'MUTATED', '', 'operator-b', 'provider recovered', false,
     'DLQ', 2, 'provider down', NULL, now() - interval '2 minutes',
     'PENDING', 0, '', NULL, NULL, now())
`)
	if err != nil {
		t.Fatalf("seed outbox repair audit: %v", err)
	}

	store := NewOutboxStore(pool)
	rows, err := store.AuditOutboxRepairs(ctx, OutboxRepairAuditOptions{
		TenantID: "tenant-a",
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("audit outbox repairs: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected two rows, got %d", len(rows))
	}
	if rows[0].OutboxID != 12 || rows[0].Mode != types.OutboxRepairModeRedriveDLQPending || rows[0].Outcome != outboxRepairOutcomeMutated {
		t.Fatalf("unexpected latest outbox repair audit row: %+v", rows[0])
	}
	if rows[1].OutboxID != 11 || rows[1].Mode != types.OutboxRepairModeAudit || rows[1].Outcome != outboxRepairOutcomeAudited || !rows[1].DryRun {
		t.Fatalf("unexpected older outbox repair audit row: %+v", rows[1])
	}
}

func TestOutboxStoreAuditOutboxRepairsFiltersModeAndOutcomeIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetDeliveryTables(t, ctx, pool)

	_, err := pool.Exec(ctx, `
INSERT INTO delivery_outbox_repair_audit (
    outbox_id, event_id, tenant_id, conversation_id, aggregate_version, mode, outcome, skip_reason, operator, reason, dry_run,
    before_status, before_retry_count, before_last_error, before_next_retry_at, before_dead_lettered_at,
    after_status, after_retry_count, after_last_error, after_next_retry_at, after_dead_lettered_at, created_at
) VALUES
    (21, 'event-21', 'tenant-b', 'conversation-a', 1, 'audit', 'AUDITED', '', 'operator-a', 'manual audit', true,
     'DLQ', 1, 'malformed payload', NULL, NULL,
     'DLQ', 1, 'malformed payload', NULL, NULL, now()),
    (22, 'event-22', 'tenant-b', 'conversation-a', 2, 'redrive-dlq-pending', 'MUTATED', '', 'operator-b', 'provider recovered', false,
     'DLQ', 2, 'provider down', NULL, NULL,
     'PENDING', 0, '', NULL, NULL, now() - interval '1 minute')
`)
	if err != nil {
		t.Fatalf("seed outbox repair audit: %v", err)
	}

	store := NewOutboxStore(pool)
	rows, err := store.AuditOutboxRepairs(ctx, OutboxRepairAuditOptions{
		TenantID: "tenant-b",
		Mode:     types.OutboxRepairModeAudit,
		Outcome:  outboxRepairOutcomeAudited,
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("audit outbox repairs with filters: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one row, got %d", len(rows))
	}
	if rows[0].OutboxID != 21 || rows[0].Mode != types.OutboxRepairModeAudit || rows[0].Outcome != outboxRepairOutcomeAudited {
		t.Fatalf("unexpected filtered outbox repair audit row: %+v", rows[0])
	}
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

func readDeliveryOutboxID(t *testing.T, ctx context.Context, pool *pgxpool.Pool, eventID string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(ctx, `
SELECT id
FROM delivery_outbox
WHERE event_id = $1
`, eventID).Scan(&id); err != nil {
		t.Fatalf("read delivery outbox id: %v", err)
	}
	return id
}

func assertDeliveryOutboxRepairAudit(t *testing.T, ctx context.Context, pool *pgxpool.Pool, outboxID int64, mode string, outcome string, skipReason string, beforeStatus string, beforeRetry int, beforeLastError string, afterStatus string, afterRetry int, afterLastError string, reason string) {
	t.Helper()
	var gotMode string
	var gotOutcome string
	var gotSkipReason string
	var gotBeforeStatus string
	var gotBeforeRetry int
	var gotBeforeLastError string
	var gotAfterStatus string
	var gotAfterRetry int
	var gotAfterLastError string
	var gotReason string
	if err := pool.QueryRow(ctx, `
SELECT
    mode,
    outcome,
    skip_reason,
    before_status,
    before_retry_count,
    before_last_error,
    after_status,
    after_retry_count,
    after_last_error,
    reason
FROM delivery_outbox_repair_audit
WHERE outbox_id = $1
ORDER BY id DESC
LIMIT 1
`, outboxID).Scan(
		&gotMode,
		&gotOutcome,
		&gotSkipReason,
		&gotBeforeStatus,
		&gotBeforeRetry,
		&gotBeforeLastError,
		&gotAfterStatus,
		&gotAfterRetry,
		&gotAfterLastError,
		&gotReason,
	); err != nil {
		t.Fatalf("read delivery outbox repair audit: %v", err)
	}
	if gotMode != mode ||
		gotOutcome != outcome ||
		gotSkipReason != skipReason ||
		gotBeforeStatus != beforeStatus ||
		gotBeforeRetry != beforeRetry ||
		gotBeforeLastError != beforeLastError ||
		gotAfterStatus != afterStatus ||
		gotAfterRetry != afterRetry ||
		gotAfterLastError != afterLastError ||
		gotReason != reason {
		t.Fatalf("unexpected repair audit row: mode=%s outcome=%s skip=%s before=(%s,%d,%s) after=(%s,%d,%s) reason=%s",
			gotMode, gotOutcome, gotSkipReason, gotBeforeStatus, gotBeforeRetry, gotBeforeLastError, gotAfterStatus, gotAfterRetry, gotAfterLastError, gotReason)
	}
}
