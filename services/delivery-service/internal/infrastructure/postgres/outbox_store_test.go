package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/delivery-service/internal/types"
)

func TestSanitizeDeliveryOutboxPublishErrorUsesStablePublicMessages(t *testing.T) {
	tests := []struct {
		name string
		err  error
		text string
		want string
	}{
		{
			name: "context canceled",
			err:  context.Canceled,
			text: "context canceled for user=user1@example.com token=secret-token",
			want: "delivery outbox publish canceled",
		},
		{
			name: "deadline exceeded",
			err:  context.DeadlineExceeded,
			text: "deadline exceeded while publishing request-token=secret-token",
			want: "delivery outbox publish timeout",
		},
		{
			name: "unsupported event",
			err:  errors.New("unsupported event_type=delivery.future.v9 user=user1@example.com"),
			text: "unsupported event_type=delivery.future.v9 user=user1@example.com",
			want: "delivery outbox publish unsupported event",
		},
		{
			name: "invalid payload",
			err:  errors.New("malformed json payload for user=user1@example.com token=secret-token"),
			text: "malformed json payload for user=user1@example.com token=secret-token",
			want: "delivery outbox publish invalid payload",
		},
		{
			name: "broker unavailable",
			err:  errors.New("kafka broker connection refused at 10.0.0.8 token=secret-token"),
			text: "kafka broker connection refused at 10.0.0.8 token=secret-token",
			want: "delivery outbox publish broker unavailable",
		},
		{
			name: "unknown raw error",
			err:  errors.New("provider body user=user1@example.com token=secret-token nonce=secret-nonce"),
			text: "provider body user=user1@example.com token=secret-token nonce=secret-nonce",
			want: "delivery outbox publish failed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sanitizeDeliveryOutboxPublishError(tt.err); got != tt.want {
				t.Fatalf("sanitize publish error = %q, want %q", got, tt.want)
			}
			if got := sanitizeDeliveryOutboxStoredError(tt.text); got != tt.want {
				t.Fatalf("sanitize stored error = %q, want %q", got, tt.want)
			}
			for _, forbidden := range []string{"user1@example.com", "secret-token", "secret-nonce", "10.0.0.8"} {
				if strings.Contains(tt.want, forbidden) {
					t.Fatalf("stable delivery outbox error leaked sensitive text %q in %q", forbidden, tt.want)
				}
			}
		})
	}
	if got := sanitizeDeliveryOutboxStoredError("   "); got != "" {
		t.Fatalf("blank stored error = %q, want empty", got)
	}
}

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
				errs[index] = errors.New("kafka unavailable: broker body user=user1@example.com token=secret-token")
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
	assertDeliveryOutboxLastError(t, ctx, pool, "event-2", "delivery outbox publish broker unavailable", "user1@example.com", "secret-token")
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
	assertDeliveryOutboxLastError(t, ctx, pool, "event-1", "delivery outbox publish invalid payload", "malformed payload")

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
	assertDeliveryOutboxRepairAudit(t, ctx, pool, outboxID, types.OutboxRepairModeAudit, outboxRepairOutcomeAudited, "", types.OutboxStatusDLQ, 1, "delivery outbox publish invalid payload", types.OutboxStatusDLQ, 1, "delivery outbox publish invalid payload", "manual audit")
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
	assertDeliveryOutboxRepairAudit(t, ctx, pool, outboxID, types.OutboxRepairModeRedriveDLQPending, outboxRepairOutcomeMutated, "", types.OutboxStatusDLQ, 1, "delivery outbox publish invalid payload", types.OutboxStatusPending, 0, "", "provider recovered")
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

func TestOutboxStoreAuditOutboxReturnsLatestRowsIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetDeliveryTables(t, ctx, pool)

	seedDeliveryOutboxWithStatus(t, ctx, pool, "event-61", "tenant-f", "conversation-a", 1, types.DeliveryEventInboxItemCreated, types.OutboxStatusPending, 2, "retry later")
	seedDeliveryOutboxWithStatus(t, ctx, pool, "event-62", "tenant-f", "conversation-a", 2, types.DeliveryEventAckRecorded, types.OutboxStatusDLQ, 4, "kafka unavailable: broker body user=user1@example.com token=secret-token")

	store := NewOutboxStore(pool)
	rows, err := store.AuditOutbox(ctx, OutboxAuditOptions{
		TenantID: "tenant-f",
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("audit outbox: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected two rows, got %d", len(rows))
	}
	if rows[0].EventID != "event-62" || rows[0].Status != types.OutboxStatusDLQ || rows[0].RetryCount != 4 || rows[0].LastError != "delivery outbox publish broker unavailable" {
		t.Fatalf("unexpected latest outbox audit row: %+v", rows[0])
	}
	assertNoDeliveryOutboxErrorLeak(t, rows[0].LastError, "user1@example.com", "secret-token", "broker body")
	if rows[1].EventID != "event-61" || rows[1].Status != types.OutboxStatusPending || rows[1].RetryCount != 2 {
		t.Fatalf("unexpected older outbox audit row: %+v", rows[1])
	}
}

func TestOutboxStoreAuditOutboxFiltersStatusIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetDeliveryTables(t, ctx, pool)

	seedDeliveryOutboxWithStatus(t, ctx, pool, "event-71", "tenant-g", "conversation-a", 1, types.DeliveryEventInboxItemCreated, types.OutboxStatusPending, 1, "retry later")
	seedDeliveryOutboxWithStatus(t, ctx, pool, "event-72", "tenant-g", "conversation-a", 2, types.DeliveryEventAckRecorded, types.OutboxStatusPublished, 0, "")
	seedDeliveryOutboxWithStatus(t, ctx, pool, "event-73", "tenant-g", "conversation-a", 3, types.DeliveryEventInboxItemCreated, types.OutboxStatusDLQ, 3, "decode failed")

	store := NewOutboxStore(pool)
	rows, err := store.AuditOutbox(ctx, OutboxAuditOptions{
		TenantID: "tenant-g",
		Status:   types.OutboxStatusDLQ,
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("audit outbox by status: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one row, got %d", len(rows))
	}
	if rows[0].EventID != "event-73" || rows[0].Status != types.OutboxStatusDLQ {
		t.Fatalf("unexpected filtered outbox audit row: %+v", rows[0])
	}
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
     'DLQ', 1, 'malformed payload user=user1@example.com', NULL, now() - interval '1 minute',
     'DLQ', 1, 'malformed payload user=user1@example.com', NULL, now() - interval '1 minute', now() - interval '1 minute'),
    (12, 'event-12', 'tenant-a', 'conversation-b', 2, 'redrive-dlq-pending', 'MUTATED', '', 'operator-b', 'provider recovered', false,
     'DLQ', 2, 'kafka unavailable: broker body token=secret-token', NULL, now() - interval '2 minutes',
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
	if rows[0].BeforeLastError != "delivery outbox publish broker unavailable" || rows[0].AfterLastError != "" {
		t.Fatalf("unexpected sanitized latest repair errors: before=%q after=%q", rows[0].BeforeLastError, rows[0].AfterLastError)
	}
	assertNoDeliveryOutboxErrorLeak(t, rows[0].BeforeLastError, "secret-token", "broker body")
	if rows[1].OutboxID != 11 || rows[1].Mode != types.OutboxRepairModeAudit || rows[1].Outcome != outboxRepairOutcomeAudited || !rows[1].DryRun {
		t.Fatalf("unexpected older outbox repair audit row: %+v", rows[1])
	}
	if rows[1].BeforeLastError != "delivery outbox publish invalid payload" || rows[1].AfterLastError != "delivery outbox publish invalid payload" {
		t.Fatalf("unexpected sanitized older repair errors: before=%q after=%q", rows[1].BeforeLastError, rows[1].AfterLastError)
	}
	assertNoDeliveryOutboxErrorLeak(t, rows[1].BeforeLastError, "user1@example.com")
}

func TestOutboxStoreRepairAuditSanitizesStoredLastErrorIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetDeliveryTables(t, ctx, pool)
	seedDeliveryOutboxWithStatus(t, ctx, pool, "event-91", "tenant-i", "conversation-a", 1, types.DeliveryEventInboxItemCreated, types.OutboxStatusDLQ, 3, "kafka unavailable: broker body user=user1@example.com token=secret-token")
	outboxID := readDeliveryOutboxID(t, ctx, pool, "event-91")

	store := NewOutboxStore(pool)
	repairStats, err := store.RepairOutbox(ctx, types.OutboxRepairOptions{
		OutboxIDs: []int64{outboxID},
		Mode:      types.OutboxRepairModeAudit,
		Operator:  "operator-1",
		Reason:    "inspect dlq",
	})
	if err != nil {
		t.Fatalf("audit outbox with raw stored last_error: %v", err)
	}
	if repairStats.Requested != 1 || repairStats.Audited != 1 {
		t.Fatalf("unexpected repair stats: %+v", repairStats)
	}
	assertDeliveryOutboxRepairAudit(t, ctx, pool, outboxID, types.OutboxRepairModeAudit, outboxRepairOutcomeAudited, "", types.OutboxStatusDLQ, 3, "delivery outbox publish broker unavailable", types.OutboxStatusDLQ, 3, "delivery outbox publish broker unavailable", "inspect dlq", "user1@example.com", "secret-token", "broker body")
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

func TestOutboxStoreCleanupOutboxRepairsDeletesOnlyExpiredRowsIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetDeliveryTables(t, ctx, pool)

	_, err := pool.Exec(ctx, `
INSERT INTO delivery_outbox_repair_audit (
    outbox_id, event_id, tenant_id, conversation_id, aggregate_version, mode, outcome, skip_reason, operator, reason, dry_run,
    before_status, before_retry_count, before_last_error, before_next_retry_at, before_dead_lettered_at,
    after_status, after_retry_count, after_last_error, after_next_retry_at, after_dead_lettered_at, created_at
) VALUES
    (31, 'event-31', 'tenant-c', 'conversation-a', 1, 'audit', 'AUDITED', '', 'operator-a', 'manual audit', true,
     'DLQ', 1, 'malformed payload', NULL, NULL,
     'DLQ', 1, 'malformed payload', NULL, NULL, now() - interval '10 days'),
    (32, 'event-32', 'tenant-c', 'conversation-b', 2, 'redrive-dlq-pending', 'MUTATED', '', 'operator-b', 'provider recovered', false,
     'DLQ', 2, 'provider down', NULL, NULL,
     'PENDING', 0, '', NULL, NULL, now() - interval '1 day')
`)
	if err != nil {
		t.Fatalf("seed outbox repair audit: %v", err)
	}

	store := NewOutboxStore(pool)
	stats, err := store.CleanupOutboxRepairs(ctx, OutboxRepairCleanupOptions{
		TenantID: "tenant-c",
		Cutoff:   time.Now().UTC().Add(-7 * 24 * time.Hour),
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("cleanup outbox repairs: %v", err)
	}
	if stats.Deleted != 1 {
		t.Fatalf("unexpected deleted count: %+v", stats)
	}
	assertDeliveryOutboxRepairAuditCount(t, ctx, pool, "tenant-c", 1)
}

func TestOutboxStoreCleanupOutboxRepairsHonorsBatchLimitIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetDeliveryTables(t, ctx, pool)

	_, err := pool.Exec(ctx, `
INSERT INTO delivery_outbox_repair_audit (
    outbox_id, event_id, tenant_id, conversation_id, aggregate_version, mode, outcome, skip_reason, operator, reason, dry_run,
    before_status, before_retry_count, before_last_error, before_next_retry_at, before_dead_lettered_at,
    after_status, after_retry_count, after_last_error, after_next_retry_at, after_dead_lettered_at, created_at
) VALUES
    (41, 'event-41', 'tenant-d', 'conversation-a', 1, 'audit', 'AUDITED', '', 'operator-a', 'manual audit', true,
     'DLQ', 1, 'malformed payload', NULL, NULL,
     'DLQ', 1, 'malformed payload', NULL, NULL, now() - interval '10 days'),
    (42, 'event-42', 'tenant-d', 'conversation-b', 2, 'redrive-dlq-pending', 'MUTATED', '', 'operator-b', 'provider recovered', false,
     'DLQ', 2, 'provider down', NULL, NULL,
     'PENDING', 0, '', NULL, NULL, now() - interval '9 days')
`)
	if err != nil {
		t.Fatalf("seed outbox repair audit: %v", err)
	}

	store := NewOutboxStore(pool)
	stats, err := store.CleanupOutboxRepairs(ctx, OutboxRepairCleanupOptions{
		TenantID: "tenant-d",
		Cutoff:   time.Now().UTC().Add(-7 * 24 * time.Hour),
		Limit:    1,
	})
	if err != nil {
		t.Fatalf("cleanup outbox repairs: %v", err)
	}
	if stats.Deleted != 1 {
		t.Fatalf("unexpected deleted count: %+v", stats)
	}
	assertDeliveryOutboxRepairAuditCount(t, ctx, pool, "tenant-d", 1)
}

func TestOutboxStoreCleanupOutboxRepairsFiltersModeAndOutcomeIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetDeliveryTables(t, ctx, pool)

	_, err := pool.Exec(ctx, `
INSERT INTO delivery_outbox_repair_audit (
    outbox_id, event_id, tenant_id, conversation_id, aggregate_version, mode, outcome, skip_reason, operator, reason, dry_run,
    before_status, before_retry_count, before_last_error, before_next_retry_at, before_dead_lettered_at,
    after_status, after_retry_count, after_last_error, after_next_retry_at, after_dead_lettered_at, created_at
) VALUES
    (51, 'event-51', 'tenant-e', 'conversation-a', 1, 'audit', 'AUDITED', '', 'operator-a', 'manual audit', true,
     'DLQ', 1, 'malformed payload', NULL, NULL,
     'DLQ', 1, 'malformed payload', NULL, NULL, now() - interval '10 days'),
    (52, 'event-52', 'tenant-e', 'conversation-a', 2, 'redrive-dlq-pending', 'MUTATED', '', 'operator-b', 'provider recovered', false,
     'DLQ', 2, 'provider down', NULL, NULL,
     'PENDING', 0, '', NULL, NULL, now() - interval '10 days')
`)
	if err != nil {
		t.Fatalf("seed outbox repair audit: %v", err)
	}

	store := NewOutboxStore(pool)
	stats, err := store.CleanupOutboxRepairs(ctx, OutboxRepairCleanupOptions{
		TenantID: "tenant-e",
		Mode:     types.OutboxRepairModeAudit,
		Outcome:  outboxRepairOutcomeAudited,
		Cutoff:   time.Now().UTC().Add(-7 * 24 * time.Hour),
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("cleanup outbox repairs with filters: %v", err)
	}
	if stats.Deleted != 1 {
		t.Fatalf("unexpected deleted count: %+v", stats)
	}

	rows, err := store.AuditOutboxRepairs(ctx, OutboxRepairAuditOptions{
		TenantID: "tenant-e",
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("audit outbox repairs after cleanup: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one row after cleanup, got %d", len(rows))
	}
	if rows[0].OutboxID != 52 || rows[0].Mode != types.OutboxRepairModeRedriveDLQPending || rows[0].Outcome != outboxRepairOutcomeMutated {
		t.Fatalf("unexpected remaining outbox repair audit row: %+v", rows[0])
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

func seedDeliveryOutboxWithStatus(t *testing.T, ctx context.Context, pool *pgxpool.Pool, eventID string, tenantID string, conversationID string, version int64, eventType string, status string, retryCount int, lastError string) {
	t.Helper()
	payload := fmt.Sprintf(`{"tenant_id":%q,"user_id":"user-1","device_id":"device-1","conversation_id":%q,"conversation_seq":%d,"source_event_id":%q,"message_id":%q,"last_received_seq":%d}`, tenantID, conversationID, version, "source-"+eventID, "message-"+eventID, version)
	var nextRetryAt any
	var deadLetteredAt any
	var publishedAt any
	switch status {
	case types.OutboxStatusPending:
		nextRetryAt = time.Now().UTC().Add(5 * time.Minute)
	case types.OutboxStatusDLQ:
		deadLetteredAt = time.Now().UTC().Add(-time.Minute)
	case types.OutboxStatusPublished:
		publishedAt = time.Now().UTC().Add(-time.Minute)
	}
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
    retry_count,
    last_error,
    available_at,
    next_retry_at,
    dead_lettered_at,
    published_at
) VALUES ($1, $2, $3, $4, $5, '1.0.0', $2 || ':' || $3, 1, 'request-1', 'source-' || $1, 'delivery-service', 'trace-1', $6::jsonb, $7, $8, $9, now(), $10, $11, $12)
`, eventID, tenantID, conversationID, version, eventType, payload, status, retryCount, lastError, nextRetryAt, deadLetteredAt, publishedAt)
	if err != nil {
		t.Fatalf("seed delivery outbox with status: %v", err)
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

func assertDeliveryOutboxLastError(t *testing.T, ctx context.Context, pool *pgxpool.Pool, eventID string, want string, forbidden ...string) {
	t.Helper()
	var got string
	if err := pool.QueryRow(ctx, `
SELECT last_error
FROM delivery_outbox
WHERE event_id = $1
`, eventID).Scan(&got); err != nil {
		t.Fatalf("read delivery outbox last error: %v", err)
	}
	if got != want {
		t.Fatalf("unexpected last error for %s: got %q want %q", eventID, got, want)
	}
	for _, text := range forbidden {
		if text != "" && strings.Contains(got, text) {
			t.Fatalf("last error for %s leaked %q: %q", eventID, text, got)
		}
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

func assertDeliveryOutboxRepairAudit(t *testing.T, ctx context.Context, pool *pgxpool.Pool, outboxID int64, mode string, outcome string, skipReason string, beforeStatus string, beforeRetry int, beforeLastError string, afterStatus string, afterRetry int, afterLastError string, reason string, forbidden ...string) {
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
	assertNoDeliveryOutboxErrorLeak(t, gotBeforeLastError, forbidden...)
	assertNoDeliveryOutboxErrorLeak(t, gotAfterLastError, forbidden...)
}

func assertNoDeliveryOutboxErrorLeak(t *testing.T, got string, forbidden ...string) {
	t.Helper()
	for _, text := range forbidden {
		if text != "" && strings.Contains(got, text) {
			t.Fatalf("delivery outbox error leaked %q: %q", text, got)
		}
	}
}

func assertDeliveryOutboxRepairAuditCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tenantID string, wantCount int64) {
	t.Helper()
	var gotCount int64
	if err := pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM delivery_outbox_repair_audit
WHERE tenant_id = $1
`, tenantID).Scan(&gotCount); err != nil {
		t.Fatalf("count delivery outbox repair audit rows: %v", err)
	}
	if gotCount != wantCount {
		t.Fatalf("unexpected delivery outbox repair audit count: got=%d want=%d", gotCount, wantCount)
	}
}
