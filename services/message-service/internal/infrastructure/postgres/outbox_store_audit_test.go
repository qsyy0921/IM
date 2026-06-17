package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/message-service/internal/types"
)

func TestOutboxStoreAuditOutboxReturnsLatestRowsIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openIntegrationPool(t, ctx)
	defer pool.Close()
	applyMessageMigration(t, ctx, pool)
	resetMessageCoreTables(t, ctx, pool)

	tenantID := types.TenantID(fmt.Sprintf("tenant-outbox-audit-%d", time.Now().UnixNano()))
	repo := NewMessageRepository(pool)
	appendConversationMessages(t, ctx, repo, tenantID, "conversation-a", 2)
	updateMessageOutboxAuditState(t, ctx, pool, tenantID, "conversation-a", 1, types.OutboxStatusPending, 1, "kafka unavailable: broker body user=user1@example.com token=secret-token", false, false)
	updateMessageOutboxAuditState(t, ctx, pool, tenantID, "conversation-a", 2, types.OutboxStatusPublished, 0, "", true, false)

	rows, err := NewOutboxStore(pool).AuditOutbox(ctx, OutboxAuditOptions{
		TenantID: string(tenantID),
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("audit message outbox: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 outbox rows, got %d", len(rows))
	}
	if rows[0].AggregateVersion != 2 || rows[0].Status != types.OutboxStatusPublished || rows[0].EventID == "" {
		t.Fatalf("unexpected latest outbox row: %+v", rows[0])
	}
	if rows[1].AggregateVersion != 1 || rows[1].Status != types.OutboxStatusPending || rows[1].RetryCount != 1 || rows[1].LastError != "outbox publish broker unavailable" {
		t.Fatalf("unexpected older outbox row: %+v", rows[1])
	}
	if strings.Contains(rows[1].LastError, "user1@example.com") ||
		strings.Contains(rows[1].LastError, "secret-token") ||
		strings.Contains(rows[1].LastError, "broker body") {
		t.Fatalf("message outbox audit leaked raw last_error: %q", rows[1].LastError)
	}
}

func TestOutboxStoreAuditOutboxFiltersStatusAndEventTypeIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openIntegrationPool(t, ctx)
	defer pool.Close()
	applyMessageMigration(t, ctx, pool)
	resetMessageCoreTables(t, ctx, pool)

	tenantID := types.TenantID(fmt.Sprintf("tenant-outbox-audit-filter-%d", time.Now().UnixNano()))
	repo := NewMessageRepository(pool)
	appendConversationMessages(t, ctx, repo, tenantID, "conversation-a", 1)
	appendConversationMessages(t, ctx, repo, tenantID, "conversation-b", 1)
	updateMessageOutboxAuditState(t, ctx, pool, tenantID, "conversation-a", 1, types.OutboxStatusDLQ, 3, "invalid json payload with token=secret-token", false, true)
	updateMessageOutboxEventType(t, ctx, pool, tenantID, "conversation-a", 1, "message.deleted.v1")
	updateMessageOutboxAuditState(t, ctx, pool, tenantID, "conversation-b", 1, types.OutboxStatusPublished, 0, "", true, false)

	rows, err := NewOutboxStore(pool).AuditOutbox(ctx, OutboxAuditOptions{
		TenantID:  string(tenantID),
		Status:    "dlq",
		EventType: "message.deleted.v1",
		Limit:     10,
	})
	if err != nil {
		t.Fatalf("audit filtered message outbox: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 outbox row, got %d", len(rows))
	}
	if rows[0].ConversationID != "conversation-a" || rows[0].Status != types.OutboxStatusDLQ || rows[0].EventType != "message.deleted.v1" {
		t.Fatalf("unexpected filtered outbox row: %+v", rows[0])
	}
}

func TestOutboxStoreAuditOutboxFiltersCreatedAtIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openIntegrationPool(t, ctx)
	defer pool.Close()
	applyMessageMigration(t, ctx, pool)
	resetMessageCoreTables(t, ctx, pool)

	tenantID := types.TenantID(fmt.Sprintf("tenant-outbox-audit-created-%d", time.Now().UnixNano()))
	repo := NewMessageRepository(pool)
	appendConversationMessages(t, ctx, repo, tenantID, "conversation-created", 3)
	base := time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC)
	_, err := pool.Exec(ctx, `
UPDATE message_outbox
SET created_at = CASE aggregate_version
    WHEN 1 THEN $2
    WHEN 2 THEN $3
    WHEN 3 THEN $4
    ELSE created_at
END
WHERE tenant_id = $1
`, string(tenantID), base.Add(-time.Hour), base, base.Add(time.Hour))
	if err != nil {
		t.Fatalf("set message outbox created_at values: %v", err)
	}

	rows, err := NewOutboxStore(pool).AuditOutbox(ctx, OutboxAuditOptions{
		TenantID:      string(tenantID),
		CreatedAfter:  ptrTime(base.Add(-time.Minute)),
		CreatedBefore: ptrTime(base.Add(time.Minute)),
		Limit:         10,
	})
	if err != nil {
		t.Fatalf("audit message outbox by created_at: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one outbox row, got %d", len(rows))
	}
	if rows[0].AggregateVersion != 2 || !rows[0].CreatedAt.Equal(base) {
		t.Fatalf("unexpected created_at filtered row: %+v", rows[0])
	}

	_, err = NewOutboxStore(pool).AuditOutbox(ctx, OutboxAuditOptions{
		TenantID:      string(tenantID),
		CreatedAfter:  ptrTime(base),
		CreatedBefore: ptrTime(base),
		Limit:         10,
	})
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid created_at window, got %v", err)
	}
}

func TestOutboxStoreRepairDLQEventsResetsMessageOutboxStateIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openIntegrationPool(t, ctx)
	defer pool.Close()
	applyMessageMigration(t, ctx, pool)
	resetMessageCoreTables(t, ctx, pool)

	tenantID := types.TenantID(fmt.Sprintf("tenant-outbox-repair-%d", time.Now().UnixNano()))
	repo := NewMessageRepository(pool)
	appendConversationMessages(t, ctx, repo, tenantID, "conversation-a", 1)
	appendConversationMessages(t, ctx, repo, tenantID, "conversation-b", 1)
	updateMessageOutboxAuditState(t, ctx, pool, tenantID, "conversation-a", 1, types.OutboxStatusDLQ, 3, "kafka unavailable: broker body user=user1@example.com token=secret-token", false, true)
	updateMessageOutboxAuditState(t, ctx, pool, tenantID, "conversation-b", 1, types.OutboxStatusPublished, 0, "", true, false)

	stats, err := NewOutboxStore(pool).RepairDLQEvents(ctx, []string{"", "missing-event", readMessageOutboxEventID(t, ctx, pool, tenantID, "conversation-a", 1)}, "manual repair")
	if err != nil {
		t.Fatalf("repair message outbox dlq: %v", err)
	}
	if stats.Requested != 2 || stats.Repaired != 1 || stats.Skipped != 1 {
		t.Fatalf("unexpected repair stats: %+v", stats)
	}
	assertMessageOutboxState(t, ctx, pool, tenantID, "conversation-a", 1, types.OutboxStatusPending, 0, "")
	rows, err := NewOutboxStore(pool).AuditOutboxRepairs(ctx, OutboxRepairAuditOptions{
		TenantID: string(tenantID),
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("audit message outbox repairs: %v", err)
	}
	if len(rows) != 1 ||
		rows[0].ConversationID != "conversation-a" ||
		rows[0].Reason != "manual repair" ||
		rows[0].PreviousStatus != types.OutboxStatusDLQ ||
		rows[0].PreviousLastError != "outbox publish broker unavailable" {
		t.Fatalf("unexpected message outbox repair audit rows: %+v", rows)
	}
	if strings.Contains(rows[0].PreviousLastError, "user1@example.com") ||
		strings.Contains(rows[0].PreviousLastError, "secret-token") ||
		strings.Contains(rows[0].PreviousLastError, "broker body") {
		t.Fatalf("message outbox repair audit leaked raw previous_last_error: %q", rows[0].PreviousLastError)
	}
}

func TestOutboxStoreAuditOutboxRepairsFiltersConversationIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openIntegrationPool(t, ctx)
	defer pool.Close()
	applyMessageMigration(t, ctx, pool)
	resetMessageCoreTables(t, ctx, pool)

	_, err := pool.Exec(ctx, `
INSERT INTO message_outbox_repair_audit (
    event_id, tenant_id, conversation_id, previous_status, previous_retry_count, previous_last_error, previous_dead_lettered_at, repair_reason, repaired_at
) VALUES
    ('repair-event-1', 'tenant-a', 'conversation-a', 'DLQ', 2, 'publish failed', now() - interval '1 minute', 'manual audit', now() - interval '2 minutes'),
    ('repair-event-2', 'tenant-a', 'conversation-b', 'DLQ', 1, 'provider unavailable', now() - interval '3 minutes', 'operator replay', now() - interval '1 minute')
`)
	if err != nil {
		t.Fatalf("seed message outbox repair audit rows: %v", err)
	}

	rows, err := NewOutboxStore(pool).AuditOutboxRepairs(ctx, OutboxRepairAuditOptions{
		TenantID:       "tenant-a",
		ConversationID: "conversation-a",
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("audit filtered message outbox repairs: %v", err)
	}
	if len(rows) != 1 || rows[0].EventID != "repair-event-1" || rows[0].ConversationID != "conversation-a" || rows[0].Reason != "manual audit" {
		t.Fatalf("unexpected filtered message outbox repair rows: %+v", rows)
	}
}

func TestOutboxStoreAuditOutboxRepairsFiltersRepairedAtIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openIntegrationPool(t, ctx)
	defer pool.Close()
	applyMessageMigration(t, ctx, pool)
	resetMessageCoreTables(t, ctx, pool)

	base := time.Date(2026, 6, 16, 2, 0, 0, 0, time.UTC)
	_, err := pool.Exec(ctx, `
INSERT INTO message_outbox_repair_audit (
    event_id, tenant_id, conversation_id, previous_status, previous_retry_count, previous_last_error, previous_dead_lettered_at, repair_reason, repaired_at
) VALUES
    ('repair-window-old', 'tenant-window', 'conversation-a', 'DLQ', 1, 'publish failed', $1::timestamptz, 'manual old', $2::timestamptz),
    ('repair-window-hit', 'tenant-window', 'conversation-a', 'DLQ', 2, 'provider unavailable', $1::timestamptz, 'manual hit', $3::timestamptz),
    ('repair-window-new', 'tenant-window', 'conversation-a', 'DLQ', 3, 'provider unavailable', $1::timestamptz, 'manual new', $4::timestamptz)
`, base, base.Add(-time.Hour), base, base.Add(time.Hour))
	if err != nil {
		t.Fatalf("seed message outbox repair audit window rows: %v", err)
	}

	rows, err := NewOutboxStore(pool).AuditOutboxRepairs(ctx, OutboxRepairAuditOptions{
		TenantID:       "tenant-window",
		RepairedAfter:  ptrTime(base.Add(-30 * time.Minute)),
		RepairedBefore: ptrTime(base.Add(30 * time.Minute)),
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("audit message outbox repairs by repaired_at: %v", err)
	}
	if len(rows) != 1 || rows[0].EventID != "repair-window-hit" {
		t.Fatalf("unexpected repaired_at window rows: %+v", rows)
	}

	_, err = NewOutboxStore(pool).AuditOutboxRepairs(ctx, OutboxRepairAuditOptions{
		TenantID:       "tenant-window",
		RepairedAfter:  ptrTime(base),
		RepairedBefore: ptrTime(base),
		Limit:          10,
	})
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid repaired_at window, got %v", err)
	}
}

func TestOutboxStoreCleanupOutboxRepairsDeletesOnlyExpiredRowsIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openIntegrationPool(t, ctx)
	defer pool.Close()
	applyMessageMigration(t, ctx, pool)
	resetMessageCoreTables(t, ctx, pool)

	_, err := pool.Exec(ctx, `
INSERT INTO message_outbox_repair_audit (
    event_id, tenant_id, conversation_id, previous_status, previous_retry_count, previous_last_error, previous_dead_lettered_at, repair_reason, repaired_at
) VALUES
    ('cleanup-event-1', 'tenant-cleanup', 'conversation-a', 'DLQ', 1, 'publish failed', now() - interval '1 minute', 'manual audit', now() - interval '10 days'),
    ('cleanup-event-2', 'tenant-cleanup', 'conversation-a', 'DLQ', 2, 'provider unavailable', now() - interval '2 minutes', 'provider recovered', now() - interval '1 day')
`)
	if err != nil {
		t.Fatalf("seed message outbox cleanup rows: %v", err)
	}

	stats, err := NewOutboxStore(pool).CleanupOutboxRepairs(ctx, OutboxRepairCleanupOptions{
		TenantID: "tenant-cleanup",
		Cutoff:   time.Now().UTC().Add(-7 * 24 * time.Hour),
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("cleanup message outbox repairs: %v", err)
	}
	if stats.Deleted != 1 {
		t.Fatalf("unexpected deleted count: %+v", stats)
	}
	assertMessageOutboxRepairAuditCount(t, ctx, pool, "tenant-cleanup", "", 1)
}

func TestOutboxStoreCleanupOutboxRepairsDryRunDoesNotDeleteIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openIntegrationPool(t, ctx)
	defer pool.Close()
	applyMessageMigration(t, ctx, pool)
	resetMessageCoreTables(t, ctx, pool)

	_, err := pool.Exec(ctx, `
INSERT INTO message_outbox_repair_audit (
    event_id, tenant_id, conversation_id, previous_status, previous_retry_count, previous_last_error, previous_dead_lettered_at, repair_reason, repaired_at
) VALUES
    ('cleanup-dry-run-event-1', 'tenant-cleanup-dry-run', 'conversation-a', 'DLQ', 1, 'publish failed', now() - interval '1 minute', 'manual audit', now() - interval '10 days'),
    ('cleanup-dry-run-event-2', 'tenant-cleanup-dry-run', 'conversation-a', 'DLQ', 2, 'provider unavailable', now() - interval '2 minutes', 'provider recovered', now() - interval '9 days'),
    ('cleanup-dry-run-event-3', 'tenant-cleanup-dry-run', 'conversation-a', 'DLQ', 3, 'recent failure', now() - interval '3 minutes', 'recent repair', now() - interval '1 day')
`)
	if err != nil {
		t.Fatalf("seed message outbox dry-run cleanup rows: %v", err)
	}

	stats, err := NewOutboxStore(pool).CleanupOutboxRepairs(ctx, OutboxRepairCleanupOptions{
		TenantID: "tenant-cleanup-dry-run",
		Cutoff:   time.Now().UTC().Add(-7 * 24 * time.Hour),
		Limit:    10,
		DryRun:   true,
	})
	if err != nil {
		t.Fatalf("dry-run cleanup message outbox repairs: %v", err)
	}
	if stats.Deleted != 2 {
		t.Fatalf("unexpected dry-run deleted count: %+v", stats)
	}
	assertMessageOutboxRepairAuditCount(t, ctx, pool, "tenant-cleanup-dry-run", "", 3)
}

func ptrTime(value time.Time) *time.Time {
	return &value
}

func TestOutboxStoreCleanupOutboxRepairsFiltersConversationIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openIntegrationPool(t, ctx)
	defer pool.Close()
	applyMessageMigration(t, ctx, pool)
	resetMessageCoreTables(t, ctx, pool)

	_, err := pool.Exec(ctx, `
INSERT INTO message_outbox_repair_audit (
    event_id, tenant_id, conversation_id, previous_status, previous_retry_count, previous_last_error, previous_dead_lettered_at, repair_reason, repaired_at
) VALUES
    ('cleanup-event-21', 'tenant-filter', 'conversation-a', 'DLQ', 1, 'publish failed', now() - interval '1 minute', 'manual audit', now() - interval '10 days'),
    ('cleanup-event-22', 'tenant-filter', 'conversation-b', 'DLQ', 2, 'provider unavailable', now() - interval '2 minutes', 'provider recovered', now() - interval '10 days')
`)
	if err != nil {
		t.Fatalf("seed message outbox cleanup rows: %v", err)
	}

	stats, err := NewOutboxStore(pool).CleanupOutboxRepairs(ctx, OutboxRepairCleanupOptions{
		TenantID:       "tenant-filter",
		ConversationID: "conversation-a",
		Cutoff:         time.Now().UTC().Add(-7 * 24 * time.Hour),
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("cleanup message outbox repairs with conversation filter: %v", err)
	}
	if stats.Deleted != 1 {
		t.Fatalf("unexpected deleted count: %+v", stats)
	}
	rows, err := NewOutboxStore(pool).AuditOutboxRepairs(ctx, OutboxRepairAuditOptions{
		TenantID: "tenant-filter",
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("audit message outbox repairs after cleanup: %v", err)
	}
	if len(rows) != 1 || rows[0].EventID != "cleanup-event-22" || rows[0].ConversationID != "conversation-b" {
		t.Fatalf("unexpected remaining message outbox repair rows: %+v", rows)
	}
}
