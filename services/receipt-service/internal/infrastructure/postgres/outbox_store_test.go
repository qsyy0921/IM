package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/receipt-service/internal/types"
)

func TestSanitizeReceiptOutboxPublishErrorUsesStablePublicMessages(t *testing.T) {
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
			want: "receipt outbox publish canceled",
		},
		{
			name: "deadline exceeded",
			err:  context.DeadlineExceeded,
			text: "deadline exceeded while publishing request-token=secret-token",
			want: "receipt outbox publish timeout",
		},
		{
			name: "unsupported event",
			err:  errors.New("unsupported event_type=receipt.future.v9 user=user1@example.com"),
			text: "unsupported event_type=receipt.future.v9 user=user1@example.com",
			want: "receipt outbox publish unsupported event",
		},
		{
			name: "invalid payload",
			err:  errors.New("malformed json payload for user=user1@example.com token=secret-token"),
			text: "malformed json payload for user=user1@example.com token=secret-token",
			want: "receipt outbox publish invalid payload",
		},
		{
			name: "broker unavailable",
			err:  errors.New("kafka broker connection refused at 10.0.0.8 token=secret-token"),
			text: "kafka broker connection refused at 10.0.0.8 token=secret-token",
			want: "receipt outbox publish broker unavailable",
		},
		{
			name: "unknown raw error",
			err:  errors.New("provider body user=user1@example.com token=secret-token nonce=secret-nonce"),
			text: "provider body user=user1@example.com token=secret-token nonce=secret-nonce",
			want: "receipt outbox publish failed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sanitizeReceiptOutboxPublishError(tt.err); got != tt.want {
				t.Fatalf("sanitize publish error = %q, want %q", got, tt.want)
			}
			if got := sanitizeReceiptOutboxStoredError(tt.text); got != tt.want {
				t.Fatalf("sanitize stored error = %q, want %q", got, tt.want)
			}
			for _, forbidden := range []string{"user1@example.com", "secret-token", "secret-nonce", "10.0.0.8"} {
				if strings.Contains(tt.want, forbidden) {
					t.Fatalf("stable receipt outbox error leaked sensitive text %q in %q", forbidden, tt.want)
				}
			}
		})
	}
	if got := sanitizeReceiptOutboxStoredError("   "); got != "" {
		t.Fatalf("blank stored error = %q, want empty", got)
	}
}

func TestOutboxStoreAuditOutboxReturnsLatestRowsIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetReceiptTables(t, ctx, pool)

	seedReceiptOutboxWithStatus(t, ctx, pool, "receipt-event-201", "tenant-a", "conversation-a", 11, types.ReceiptEventMessageReceived, types.OutboxStatusPending, 2, "retry later")
	seedReceiptOutboxWithStatus(t, ctx, pool, "receipt-event-202", "tenant-a", "conversation-a", 12, types.ReceiptEventMessageRead, types.OutboxStatusDLQ, 4, "kafka unavailable: broker body user=user1@example.com token=secret-token")

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
	if rows[0].EventID != "receipt-event-202" || rows[0].Status != types.OutboxStatusDLQ || rows[0].RetryCount != 4 || rows[0].LastError != "receipt outbox publish broker unavailable" {
		t.Fatalf("unexpected latest outbox audit row: %+v", rows[0])
	}
	assertReceiptOutboxErrorDoesNotContain(t, rows[0].LastError, "user1@example.com", "secret-token", "broker body")
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

func TestOutboxStoreProcessReadyBatchSanitizesPublishErrorsIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetReceiptTables(t, ctx, pool)
	store := NewOutboxStore(pool, WithOutboxClock(func() time.Time {
		return time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	}))

	seedReceiptOutboxWithStatus(t, ctx, pool, "receipt-event-retry", "tenant-outbox", "conversation-a", 31, types.ReceiptEventMessageReceived, types.OutboxStatusPending, 0, "")
	stats, err := store.ProcessReadyBatch(ctx, 10, 3, time.Millisecond, func(ctx context.Context, messages []types.OutboxMessage) []error {
		return []error{errors.New("kafka unavailable: broker body user=user1@example.com token=secret-token")}
	})
	if err != nil {
		t.Fatalf("process retry outbox: %v", err)
	}
	if stats.Fetched != 1 || stats.Retried != 1 || stats.DeadLettered != 0 {
		t.Fatalf("unexpected retry stats: %+v", stats)
	}
	assertReceiptOutboxState(t, ctx, pool, "receipt-event-retry", types.OutboxStatusPending, 1, "receipt outbox publish broker unavailable")
	assertReceiptOutboxLastErrorDoesNotContain(t, ctx, pool, "receipt-event-retry", "user1@example.com", "secret-token")

	resetReceiptTables(t, ctx, pool)
	seedReceiptOutboxWithStatus(t, ctx, pool, "receipt-event-dlq", "tenant-outbox", "conversation-b", 32, types.ReceiptEventMessageRead, types.OutboxStatusPending, 0, "")
	stats, err = store.ProcessReadyBatch(ctx, 10, 1, time.Millisecond, func(ctx context.Context, messages []types.OutboxMessage) []error {
		return []error{errors.New("malformed payload: provider body token=secret-token")}
	})
	if err != nil {
		t.Fatalf("process dlq outbox: %v", err)
	}
	if stats.Fetched != 1 || stats.DeadLettered != 1 {
		t.Fatalf("unexpected dlq stats: %+v", stats)
	}
	assertReceiptOutboxState(t, ctx, pool, "receipt-event-dlq", types.OutboxStatusDLQ, 1, "receipt outbox publish invalid payload")
	assertReceiptOutboxLastErrorDoesNotContain(t, ctx, pool, "receipt-event-dlq", "malformed payload", "secret-token")
}

func TestOutboxStoreRepairDLQEventResetsStatusAndWritesAuditIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetReceiptTables(t, ctx, pool)
	store := NewOutboxStore(pool)

	seedReceiptOutboxWithStatus(t, ctx, pool, "receipt-event-221", "tenant-repair", "conversation-a", 31, types.ReceiptEventMessageRead, types.OutboxStatusDLQ, 3, "kafka unavailable: broker body user=user1@example.com token=secret-token")

	stats, err := store.RepairDLQEvents(ctx, []string{"receipt-event-221", "receipt-event-221", "missing-event"}, "operator retried after kafka recovery")
	if err != nil {
		t.Fatalf("repair dlq: %v", err)
	}
	if stats.Requested != 2 || stats.Repaired != 1 || stats.Skipped != 1 {
		t.Fatalf("unexpected repair stats: %+v", stats)
	}
	assertReceiptOutboxState(t, ctx, pool, "receipt-event-221", types.OutboxStatusPending, 0, "")
	assertReceiptOutboxRepairAudit(t, ctx, pool, "receipt-event-221", "operator retried after kafka recovery", "receipt outbox publish broker unavailable", "user1@example.com", "secret-token", "broker body")

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
    ('repair-event-1', 'tenant-a', 'DLQ', 1, 'malformed payload user=user1@example.com', now() - interval '1 minute', 'manual audit', now()),
    ('repair-event-2', 'tenant-b', 'DLQ', 2, 'kafka unavailable token=secret-token', now() - interval '2 minutes', 'provider recovered', now() - interval '1 minute')
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
	if rows[0].PreviousLastError != "receipt outbox publish invalid payload" {
		t.Fatalf("unexpected sanitized previous error: %q", rows[0].PreviousLastError)
	}
	assertReceiptOutboxErrorDoesNotContain(t, rows[0].PreviousLastError, "user1@example.com")
}

func TestOutboxStoreCleanupOutboxRepairsDeletesOnlyExpiredRowsIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetReceiptTables(t, ctx, pool)

	_, err := pool.Exec(ctx, `
INSERT INTO receipt_outbox_repair_audit (
    event_id, tenant_id, previous_status, previous_retry_count, previous_last_error, previous_dead_lettered_at, repair_reason, repaired_at
) VALUES
    ('cleanup-event-1', 'tenant-cleanup', 'DLQ', 1, 'publish failed', now() - interval '1 minute', 'manual audit', now() - interval '10 days'),
    ('cleanup-event-2', 'tenant-cleanup', 'DLQ', 2, 'provider unavailable', now() - interval '2 minutes', 'provider recovered', now() - interval '1 day')
`)
	if err != nil {
		t.Fatalf("seed receipt outbox cleanup rows: %v", err)
	}

	stats, err := NewOutboxStore(pool).CleanupOutboxRepairs(ctx, OutboxRepairCleanupOptions{
		TenantID: "tenant-cleanup",
		Cutoff:   time.Now().UTC().Add(-7 * 24 * time.Hour),
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("cleanup receipt outbox repairs: %v", err)
	}
	if stats.Deleted != 1 {
		t.Fatalf("unexpected deleted count: %+v", stats)
	}
	assertReceiptOutboxRepairAuditCount(t, ctx, pool, "tenant-cleanup", 1)
}

func TestOutboxStoreCleanupOutboxRepairsHonorsBatchLimitIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetReceiptTables(t, ctx, pool)

	_, err := pool.Exec(ctx, `
INSERT INTO receipt_outbox_repair_audit (
    event_id, tenant_id, previous_status, previous_retry_count, previous_last_error, previous_dead_lettered_at, repair_reason, repaired_at
) VALUES
    ('cleanup-event-11', 'tenant-limit', 'DLQ', 1, 'publish failed', now() - interval '1 minute', 'manual audit', now() - interval '10 days'),
    ('cleanup-event-12', 'tenant-limit', 'DLQ', 2, 'provider unavailable', now() - interval '2 minutes', 'provider recovered', now() - interval '9 days')
`)
	if err != nil {
		t.Fatalf("seed receipt outbox cleanup rows: %v", err)
	}

	stats, err := NewOutboxStore(pool).CleanupOutboxRepairs(ctx, OutboxRepairCleanupOptions{
		TenantID: "tenant-limit",
		Cutoff:   time.Now().UTC().Add(-7 * 24 * time.Hour),
		Limit:    1,
	})
	if err != nil {
		t.Fatalf("cleanup receipt outbox repairs with batch limit: %v", err)
	}
	if stats.Deleted != 1 {
		t.Fatalf("unexpected deleted count: %+v", stats)
	}
	assertReceiptOutboxRepairAuditCount(t, ctx, pool, "tenant-limit", 1)
}

func TestOutboxStoreCleanupOutboxRepairsFiltersEventAndTenantIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetReceiptTables(t, ctx, pool)

	_, err := pool.Exec(ctx, `
INSERT INTO receipt_outbox_repair_audit (
    event_id, tenant_id, previous_status, previous_retry_count, previous_last_error, previous_dead_lettered_at, repair_reason, repaired_at
) VALUES
    ('cleanup-event-21', 'tenant-filter', 'DLQ', 1, 'publish failed', now() - interval '1 minute', 'manual audit', now() - interval '10 days'),
    ('cleanup-event-22', 'tenant-other', 'DLQ', 2, 'provider unavailable', now() - interval '2 minutes', 'provider recovered', now() - interval '10 days')
`)
	if err != nil {
		t.Fatalf("seed receipt outbox cleanup rows: %v", err)
	}

	stats, err := NewOutboxStore(pool).CleanupOutboxRepairs(ctx, OutboxRepairCleanupOptions{
		EventID:  "cleanup-event-21",
		TenantID: "tenant-filter",
		Cutoff:   time.Now().UTC().Add(-7 * 24 * time.Hour),
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("cleanup receipt outbox repairs with filters: %v", err)
	}
	if stats.Deleted != 1 {
		t.Fatalf("unexpected deleted count: %+v", stats)
	}
	rows, err := NewOutboxStore(pool).AuditOutboxRepairs(ctx, OutboxRepairAuditOptions{Limit: 10})
	if err != nil {
		t.Fatalf("audit receipt outbox repairs after cleanup: %v", err)
	}
	if len(rows) != 1 || rows[0].EventID != "cleanup-event-22" || rows[0].TenantID != "tenant-other" {
		t.Fatalf("unexpected remaining receipt outbox repair audit rows: %+v", rows)
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
    last_error,
    available_at,
    next_retry_at,
    dead_lettered_at,
    published_at
) VALUES (
    $1, $2, '1.0.0', $3, $4, $5, $6, 1, 'corr-' || $1, 'cause-' || $1, 'receipt-service', 'trace-' || $1,
    '{"conversation_id":"`+conversationID+`"}'::jsonb,
    $7, $8, $9, $10, $11, $12, $13
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
		lastError,
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

func assertReceiptOutboxLastErrorDoesNotContain(t *testing.T, ctx context.Context, pool *pgxpool.Pool, eventID string, forbidden ...string) {
	t.Helper()
	var lastError string
	if err := pool.QueryRow(ctx, `
SELECT last_error
FROM receipt_outbox
WHERE event_id = $1
`, eventID).Scan(&lastError); err != nil {
		t.Fatalf("query receipt outbox last_error: %v", err)
	}
	for _, text := range forbidden {
		if text != "" && strings.Contains(lastError, text) {
			t.Fatalf("receipt outbox last_error for %s leaked %q: %q", eventID, text, lastError)
		}
	}
}

func assertReceiptOutboxRepairAudit(t *testing.T, ctx context.Context, pool *pgxpool.Pool, eventID string, wantReason string, wantPreviousError string, forbidden ...string) {
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
	assertReceiptOutboxErrorDoesNotContain(t, previousError, forbidden...)
}

func assertReceiptOutboxErrorDoesNotContain(t *testing.T, lastError string, forbidden ...string) {
	t.Helper()
	for _, text := range forbidden {
		if text != "" && strings.Contains(lastError, text) {
			t.Fatalf("receipt outbox error leaked %q: %q", text, lastError)
		}
	}
}

func assertReceiptOutboxRepairAuditCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tenantID string, want int) {
	t.Helper()
	var got int
	err := pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM receipt_outbox_repair_audit
WHERE tenant_id = $1
`, tenantID).Scan(&got)
	if err != nil {
		t.Fatalf("count receipt outbox repair audit: %v", err)
	}
	if got != want {
		t.Fatalf("expected %d receipt outbox repair audit rows, got %d", want, got)
	}
}
