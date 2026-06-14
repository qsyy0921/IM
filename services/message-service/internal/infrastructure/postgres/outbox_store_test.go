package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/message-service/internal/types"
)

func TestSanitizeOutboxPublishErrorUsesStablePublicMessages(t *testing.T) {
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
			want: "outbox publish canceled",
		},
		{
			name: "deadline exceeded",
			err:  context.DeadlineExceeded,
			text: "deadline exceeded while publishing request-token=secret-token",
			want: "outbox publish timeout",
		},
		{
			name: "unsupported event",
			err:  errors.New("unsupported event_type=message.future.v9 user=user1@example.com"),
			text: "unsupported event_type=message.future.v9 user=user1@example.com",
			want: "outbox publish unsupported event",
		},
		{
			name: "invalid event",
			err:  errors.New("malformed json payload for user=user1@example.com token=secret-token"),
			text: "malformed json payload for user=user1@example.com token=secret-token",
			want: "outbox publish invalid event",
		},
		{
			name: "broker unavailable",
			err:  errors.New("kafka broker connection refused at 10.0.0.8 token=secret-token"),
			text: "kafka broker connection refused at 10.0.0.8 token=secret-token",
			want: "outbox publish broker unavailable",
		},
		{
			name: "unknown raw error",
			err:  errors.New("provider body user=user1@example.com token=secret-token nonce=secret-nonce"),
			text: "provider body user=user1@example.com token=secret-token nonce=secret-nonce",
			want: "outbox publish failed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sanitizeOutboxPublishError(tt.err); got != tt.want {
				t.Fatalf("sanitize publish error = %q, want %q", got, tt.want)
			}
			if got := sanitizeOutboxStoredError(tt.text); got != tt.want {
				t.Fatalf("sanitize stored error = %q, want %q", got, tt.want)
			}
			for _, forbidden := range []string{"user1@example.com", "secret-token", "secret-nonce", "10.0.0.8"} {
				if strings.Contains(tt.want, forbidden) {
					t.Fatalf("stable outbox error leaked sensitive text %q in %q", forbidden, tt.want)
				}
			}
		})
	}
	if got := sanitizeOutboxStoredError("   "); got != "" {
		t.Fatalf("blank stored error = %q, want empty", got)
	}
}

func TestOutboxStoreProcessReadyPublishesAndMarksPublished(t *testing.T) {
	ctx := context.Background()
	pool := openIntegrationPool(t, ctx)
	defer pool.Close()
	applyMessageMigration(t, ctx, pool)
	resetMessageCoreTables(t, ctx, pool)

	input := testAppendInput(types.TenantID(fmt.Sprintf("tenant-outbox-publish-%d", time.Now().UnixNano())), "client-1", []byte(`{"text":"hello"}`))
	repo := NewMessageRepository(pool)
	if _, err := repo.AppendMessage(ctx, input); err != nil {
		t.Fatalf("append message: %v", err)
	}

	store := NewOutboxStore(pool, WithOutboxClock(func() time.Time {
		return time.Date(2026, 6, 8, 12, 1, 0, 0, time.UTC)
	}))
	published := make([]types.OutboxMessage, 0, 1)
	stats, err := store.ProcessReady(ctx, 10, 3, time.Millisecond, func(_ context.Context, message types.OutboxMessage) error {
		published = append(published, message)
		return nil
	})
	if err != nil {
		t.Fatalf("process outbox: %v", err)
	}
	if stats.Fetched != 1 || stats.Published != 1 || len(published) != 1 {
		t.Fatalf("unexpected stats=%+v published=%d", stats, len(published))
	}
	if published[0].PartitionKey != string(input.Command.AuthContext.TenantID)+":"+string(input.Command.ConversationID) ||
		published[0].PermissionVersion != input.Permission.PermissionVersion ||
		!strings.Contains(string(published[0].PayloadJSON), `"command_hash"`) {
		t.Fatalf("unexpected published message: %+v", published[0])
	}

	status := readOutboxStatus(t, ctx, pool, input.Command.AuthContext.TenantID)
	if status.Status != types.OutboxStatusPublished || !status.Published || status.RetryCount != 0 || status.LastError != "" {
		t.Fatalf("unexpected outbox status: %+v", status)
	}
}

func TestOutboxStoreProcessReadyBatchMarksPublished(t *testing.T) {
	ctx := context.Background()
	pool := openIntegrationPool(t, ctx)
	defer pool.Close()
	applyMessageMigration(t, ctx, pool)
	resetMessageCoreTables(t, ctx, pool)

	tenantID := types.TenantID(fmt.Sprintf("tenant-outbox-batch-publish-%d", time.Now().UnixNano()))
	repo := NewMessageRepository(pool)
	appendConversationMessages(t, ctx, repo, tenantID, "conversation-a", 1)
	appendConversationMessages(t, ctx, repo, tenantID, "conversation-b", 1)

	store := NewOutboxStore(pool, WithOutboxClock(func() time.Time {
		return time.Date(2026, 6, 8, 12, 1, 0, 0, time.UTC)
	}))
	var published int
	stats, err := store.ProcessReady(ctx, 10, 3, time.Millisecond, func(context.Context, types.OutboxMessage) error {
		published++
		return nil
	})
	if err != nil {
		t.Fatalf("process outbox: %v", err)
	}
	if stats.Fetched != 2 || stats.Published != 2 || published != 2 {
		t.Fatalf("unexpected stats=%+v published=%d", stats, published)
	}
	assertOutboxStatusCounts(t, ctx, pool, tenantID, 2, 0, 0)
}

func TestOutboxStoreProcessReadyBatchMarksPublishedAndRetriesFailures(t *testing.T) {
	ctx := context.Background()
	pool := openIntegrationPool(t, ctx)
	defer pool.Close()
	applyMessageMigration(t, ctx, pool)
	resetMessageCoreTables(t, ctx, pool)

	tenantID := types.TenantID(fmt.Sprintf("tenant-outbox-batch-mixed-%d", time.Now().UnixNano()))
	repo := NewMessageRepository(pool)
	appendConversationMessages(t, ctx, repo, tenantID, "conversation-a", 1)
	appendConversationMessages(t, ctx, repo, tenantID, "conversation-b", 1)

	store := NewOutboxStore(pool, WithOutboxClock(func() time.Time {
		return time.Date(2026, 6, 8, 12, 1, 0, 0, time.UTC)
	}))
	stats, err := store.ProcessReady(ctx, 10, 3, time.Millisecond, func(_ context.Context, message types.OutboxMessage) error {
		if message.ConversationID == "conversation-b" {
			return errors.New("kafka unavailable")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("process mixed outbox batch: %v", err)
	}
	if stats.Fetched != 2 || stats.Published != 1 || stats.Retried != 1 || stats.DeadLettered != 0 {
		t.Fatalf("unexpected stats=%+v", stats)
	}

	published := readOutboxStatusByConversation(t, ctx, pool, tenantID, "conversation-a")
	if published.Status != types.OutboxStatusPublished || !published.Published || published.RetryCount != 0 {
		t.Fatalf("expected conversation-a published, got %+v", published)
	}
	retried := readOutboxStatusByConversation(t, ctx, pool, tenantID, "conversation-b")
	if retried.Status != types.OutboxStatusPending ||
		retried.Published ||
		retried.RetryCount != 1 ||
		!retried.NextRetry ||
		retried.LastError != "outbox publish broker unavailable" {
		t.Fatalf("expected conversation-b retried, got %+v", retried)
	}
	assertOutboxStatusCounts(t, ctx, pool, tenantID, 1, 1, 0)
}

func TestOutboxStoreProcessReadyBatchSkipsPublishWhenEmpty(t *testing.T) {
	ctx := context.Background()
	pool := openIntegrationPool(t, ctx)
	defer pool.Close()
	applyMessageMigration(t, ctx, pool)
	resetMessageCoreTables(t, ctx, pool)

	store := NewOutboxStore(pool)
	called := false
	stats, err := store.ProcessReadyBatch(ctx, 10, 3, time.Millisecond, func(context.Context, []types.OutboxMessage) []error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("process empty batch: %v", err)
	}
	if called {
		t.Fatalf("publish callback should not be called for empty batch")
	}
	if stats.Fetched != 0 || stats.Published != 0 || stats.Retried != 0 || stats.DeadLettered != 0 {
		t.Fatalf("unexpected empty stats: %+v", stats)
	}
}

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

func TestOutboxStoreProcessReadyBatchDirectlyMarksPublishedAndRetriesFailures(t *testing.T) {
	ctx := context.Background()
	pool := openIntegrationPool(t, ctx)
	defer pool.Close()
	applyMessageMigration(t, ctx, pool)
	resetMessageCoreTables(t, ctx, pool)

	tenantID := types.TenantID(fmt.Sprintf("tenant-outbox-batch-direct-%d", time.Now().UnixNano()))
	repo := NewMessageRepository(pool)
	appendConversationMessages(t, ctx, repo, tenantID, "conversation-a", 1)
	appendConversationMessages(t, ctx, repo, tenantID, "conversation-b", 1)

	store := NewOutboxStore(pool, WithOutboxClock(func() time.Time {
		return time.Date(2026, 6, 8, 12, 1, 0, 0, time.UTC)
	}))
	stats, err := store.ProcessReadyBatch(ctx, 10, 3, time.Millisecond, func(_ context.Context, messages []types.OutboxMessage) []error {
		errs := make([]error, len(messages))
		for i, message := range messages {
			if message.ConversationID == "conversation-b" {
				errs[i] = errors.New("kafka unavailable")
			}
		}
		return errs
	})
	if err != nil {
		t.Fatalf("process mixed outbox batch directly: %v", err)
	}
	if stats.Fetched != 2 || stats.Published != 1 || stats.Retried != 1 || stats.DeadLettered != 0 {
		t.Fatalf("unexpected stats=%+v", stats)
	}

	published := readOutboxStatusByConversation(t, ctx, pool, tenantID, "conversation-a")
	if published.Status != types.OutboxStatusPublished || !published.Published || published.RetryCount != 0 {
		t.Fatalf("expected conversation-a published, got %+v", published)
	}
	retried := readOutboxStatusByConversation(t, ctx, pool, tenantID, "conversation-b")
	if retried.Status != types.OutboxStatusPending ||
		retried.Published ||
		retried.RetryCount != 1 ||
		!retried.NextRetry ||
		retried.LastError != "outbox publish broker unavailable" {
		t.Fatalf("expected conversation-b retried, got %+v", retried)
	}
	assertOutboxStatusCounts(t, ctx, pool, tenantID, 1, 1, 0)
}

func TestOutboxStoreProcessReadyRetriesOnPublishFailure(t *testing.T) {
	ctx := context.Background()
	pool := openIntegrationPool(t, ctx)
	defer pool.Close()
	applyMessageMigration(t, ctx, pool)
	resetMessageCoreTables(t, ctx, pool)

	input := testAppendInput(types.TenantID(fmt.Sprintf("tenant-outbox-retry-%d", time.Now().UnixNano())), "client-1", []byte(`{"text":"hello"}`))
	repo := NewMessageRepository(pool)
	if _, err := repo.AppendMessage(ctx, input); err != nil {
		t.Fatalf("append message: %v", err)
	}

	store := NewOutboxStore(pool, WithOutboxClock(func() time.Time {
		return time.Date(2026, 6, 8, 12, 1, 0, 0, time.UTC)
	}))
	stats, err := store.ProcessReady(ctx, 10, 3, time.Millisecond, func(context.Context, types.OutboxMessage) error {
		return errors.New("kafka unavailable: broker body user=user1@example.com token=secret-token")
	})
	if err != nil {
		t.Fatalf("process outbox: %v", err)
	}
	if stats.Fetched != 1 || stats.Retried != 1 || stats.Published != 0 {
		t.Fatalf("unexpected stats: %+v", stats)
	}

	status := readOutboxStatus(t, ctx, pool, input.Command.AuthContext.TenantID)
	if status.Status != types.OutboxStatusPending ||
		status.RetryCount != 1 ||
		!status.NextRetry ||
		status.Published ||
		status.LastError != "outbox publish broker unavailable" {
		t.Fatalf("unexpected outbox status: %+v", status)
	}
	if strings.Contains(status.LastError, "user1@example.com") ||
		strings.Contains(status.LastError, "secret-token") ||
		strings.Contains(status.LastError, "broker body") {
		t.Fatalf("outbox last_error leaked publisher text: %q", status.LastError)
	}
}

func TestOutboxStoreProcessReadyDeadLettersAfterMaxAttempts(t *testing.T) {
	ctx := context.Background()
	pool := openIntegrationPool(t, ctx)
	defer pool.Close()
	applyMessageMigration(t, ctx, pool)
	resetMessageCoreTables(t, ctx, pool)

	input := testAppendInput(types.TenantID(fmt.Sprintf("tenant-outbox-dlq-%d", time.Now().UnixNano())), "client-1", []byte(`{"text":"hello"}`))
	repo := NewMessageRepository(pool)
	if _, err := repo.AppendMessage(ctx, input); err != nil {
		t.Fatalf("append message: %v", err)
	}

	store := NewOutboxStore(pool, WithOutboxClock(func() time.Time {
		return time.Date(2026, 6, 8, 12, 1, 0, 0, time.UTC)
	}))
	stats, err := store.ProcessReady(ctx, 10, 1, time.Millisecond, func(context.Context, types.OutboxMessage) error {
		return errors.New("kafka unavailable")
	})
	if err != nil {
		t.Fatalf("process outbox: %v", err)
	}
	if stats.Fetched != 1 || stats.DeadLettered != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}

	status := readOutboxStatus(t, ctx, pool, input.Command.AuthContext.TenantID)
	if status.Status != types.OutboxStatusDLQ ||
		status.RetryCount != 1 ||
		!status.DeadLettered ||
		status.NextRetry ||
		status.LastError != "outbox publish broker unavailable" {
		t.Fatalf("unexpected outbox status: %+v", status)
	}
}

func TestOutboxStoreProcessReadyBlocksLaterConversationVersionWhenLowerDLQ(t *testing.T) {
	ctx := context.Background()
	pool := openIntegrationPool(t, ctx)
	defer pool.Close()
	applyMessageMigration(t, ctx, pool)
	resetMessageCoreTables(t, ctx, pool)

	tenantID := types.TenantID(fmt.Sprintf("tenant-outbox-order-%d", time.Now().UnixNano()))
	repo := NewMessageRepository(pool)
	first := testAppendInput(tenantID, "client-1", []byte(`{"text":"first"}`))
	second := testAppendInput(tenantID, "client-2", []byte(`{"text":"second"}`))
	if _, err := repo.AppendMessage(ctx, first); err != nil {
		t.Fatalf("append first: %v", err)
	}
	if _, err := repo.AppendMessage(ctx, second); err != nil {
		t.Fatalf("append second: %v", err)
	}

	store := NewOutboxStore(pool, WithOutboxClock(func() time.Time {
		return time.Date(2026, 6, 8, 12, 1, 0, 0, time.UTC)
	}))
	if _, err := store.ProcessReady(ctx, 10, 1, time.Millisecond, func(context.Context, types.OutboxMessage) error {
		return errors.New("kafka unavailable")
	}); err != nil {
		t.Fatalf("dlq first event: %v", err)
	}

	publishCount := 0
	stats, err := store.ProcessReady(ctx, 10, 3, time.Millisecond, func(context.Context, types.OutboxMessage) error {
		publishCount++
		return nil
	})
	if err != nil {
		t.Fatalf("process blocked outbox: %v", err)
	}
	if stats.Fetched != 0 || publishCount != 0 {
		t.Fatalf("later event should be blocked by lower DLQ: stats=%+v publishCount=%d", stats, publishCount)
	}

	if status := readOutboxStatusByVersion(t, ctx, pool, tenantID, 1); status.Status != types.OutboxStatusDLQ {
		t.Fatalf("first event should be DLQ: %+v", status)
	}
	if status := readOutboxStatusByVersion(t, ctx, pool, tenantID, 2); status.Status != types.OutboxStatusPending {
		t.Fatalf("second event should remain pending: %+v", status)
	}
}

func TestOutboxStoreProcessReadyConcurrentWorkersKeepConversationOrder(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool := openIntegrationPool(t, ctx)
	defer pool.Close()
	applyMessageMigration(t, ctx, pool)
	resetMessageCoreTables(t, ctx, pool)

	tenantID := types.TenantID(fmt.Sprintf("tenant-outbox-concurrent-%d", time.Now().UnixNano()))
	repo := NewMessageRepository(pool)
	appendConversationMessages(t, ctx, repo, tenantID, "conversation-a", 3)
	appendConversationMessages(t, ctx, repo, tenantID, "conversation-b", 3)

	store := NewOutboxStore(pool)
	recorder := newOutboxPublishRecorder()
	releaseFirstBatch := make(chan struct{})
	releaseOnce := sync.Once{}
	release := func() {
		releaseOnce.Do(func() {
			close(releaseFirstBatch)
		})
	}
	defer release()

	firstBatchErr := make(chan error, 1)
	go func() {
		firstBatchErr <- runConcurrentProcessReady(
			ctx,
			store,
			4,
			func(ctx context.Context, message types.OutboxMessage) error {
				recorder.record(message)
				recorder.enter()
				defer recorder.leave()
				select {
				case <-releaseFirstBatch:
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			},
		)
	}()

	select {
	case <-recorder.twoActive:
	case <-time.After(2 * time.Second):
		release()
		t.Fatalf("expected two conversations to be processed concurrently")
	}
	release()
	if err := <-firstBatchErr; err != nil {
		t.Fatalf("process first concurrent batch: %v", err)
	}
	if recorder.maxActiveCount() < 2 {
		t.Fatalf("expected cross-conversation concurrency, max active=%d", recorder.maxActiveCount())
	}

	drainOutboxConcurrently(t, ctx, store, 4, func(_ context.Context, message types.OutboxMessage) error {
		recorder.record(message)
		return nil
	})

	recorder.assertConversationOrder(t, "conversation-a", []int64{1, 2, 3})
	recorder.assertConversationOrder(t, "conversation-b", []int64{1, 2, 3})
	assertOutboxStatusCounts(t, ctx, pool, tenantID, 6, 0, 0)
}

type outboxStatus struct {
	Status       string
	RetryCount   int
	LastError    string
	NextRetry    bool
	Published    bool
	DeadLettered bool
}

func readOutboxStatus(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tenantID types.TenantID) outboxStatus {
	t.Helper()
	return readOutboxStatusWhere(t, ctx, pool, "tenant_id = $1", tenantID)
}

func readOutboxStatusByVersion(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tenantID types.TenantID, version int64) outboxStatus {
	t.Helper()
	return readOutboxStatusWhere(t, ctx, pool, "tenant_id = $1 AND aggregate_version = $2", tenantID, version)
}

func readOutboxStatusByConversation(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tenantID types.TenantID, conversationID types.ConversationID) outboxStatus {
	t.Helper()
	return readOutboxStatusWhere(t, ctx, pool, "tenant_id = $1 AND conversation_id = $2", tenantID, conversationID)
}

func readOutboxStatusWhere(t *testing.T, ctx context.Context, pool *pgxpool.Pool, where string, args ...any) outboxStatus {
	t.Helper()
	query := `
SELECT
    status,
    retry_count,
    COALESCE(last_error, ''),
    next_retry_at IS NOT NULL,
    published_at IS NOT NULL,
    dead_lettered_at IS NOT NULL
FROM message_outbox
WHERE ` + where + `
ORDER BY aggregate_version
LIMIT 1
`
	var status outboxStatus
	if err := pool.QueryRow(ctx, query, args...).Scan(
		&status.Status,
		&status.RetryCount,
		&status.LastError,
		&status.NextRetry,
		&status.Published,
		&status.DeadLettered,
	); err != nil {
		t.Fatalf("read outbox status: %v", err)
	}
	return status
}

func appendConversationMessages(
	t *testing.T,
	ctx context.Context,
	repo *MessageRepository,
	tenantID types.TenantID,
	conversationID types.ConversationID,
	count int,
) {
	t.Helper()
	for i := 1; i <= count; i++ {
		input := testAppendInput(
			tenantID,
			types.ClientMsgID(fmt.Sprintf("client-%s-%d", conversationID, i)),
			[]byte(fmt.Sprintf(`{"text":"%s-%d"}`, conversationID, i)),
		)
		input.Command.ConversationID = conversationID
		input.Command.AuthContext.TraceID = fmt.Sprintf("trace-%s-%d", conversationID, i)
		input.Command.AuthContext.RequestID = fmt.Sprintf("request-%s-%d", conversationID, i)
		if _, err := repo.AppendMessage(ctx, input); err != nil {
			t.Fatalf("append %s message %d: %v", conversationID, i, err)
		}
	}
}

func runConcurrentProcessReady(
	ctx context.Context,
	store *OutboxStore,
	workers int,
	publish func(context.Context, types.OutboxMessage) error,
) error {
	errCh := make(chan error, workers)
	for i := 0; i < workers; i++ {
		go func() {
			_, err := store.ProcessReady(ctx, 1, 3, time.Millisecond, publish)
			errCh <- err
		}()
	}
	for i := 0; i < workers; i++ {
		if err := <-errCh; err != nil {
			return err
		}
	}
	return nil
}

func drainOutboxConcurrently(
	t *testing.T,
	ctx context.Context,
	store *OutboxStore,
	workers int,
	publish func(context.Context, types.OutboxMessage) error,
) {
	t.Helper()
	for round := 0; round < 20; round++ {
		statsCh := make(chan types.OutboxRelayStats, workers)
		errCh := make(chan error, workers)
		for i := 0; i < workers; i++ {
			go func() {
				stats, err := store.ProcessReady(ctx, 1, 3, time.Millisecond, publish)
				statsCh <- stats
				errCh <- err
			}()
		}
		var fetched int
		for i := 0; i < workers; i++ {
			if err := <-errCh; err != nil {
				t.Fatalf("drain outbox concurrently: %v", err)
			}
			fetched += (<-statsCh).Fetched
		}
		if fetched == 0 {
			return
		}
	}
	t.Fatalf("outbox did not drain within expected rounds")
}

type outboxPublishRecorder struct {
	mu        sync.Mutex
	order     map[types.ConversationID][]int64
	active    int
	maxActive int
	twoActive chan struct{}
	closeOnce sync.Once
}

func newOutboxPublishRecorder() *outboxPublishRecorder {
	return &outboxPublishRecorder{
		order:     map[types.ConversationID][]int64{},
		twoActive: make(chan struct{}),
	}
}

func (r *outboxPublishRecorder) record(message types.OutboxMessage) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.order[message.ConversationID] = append(r.order[message.ConversationID], message.AggregateVersion)
}

func (r *outboxPublishRecorder) enter() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.active++
	if r.active > r.maxActive {
		r.maxActive = r.active
	}
	if r.active >= 2 {
		r.closeOnce.Do(func() {
			close(r.twoActive)
		})
	}
}

func (r *outboxPublishRecorder) leave() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.active--
}

func (r *outboxPublishRecorder) maxActiveCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.maxActive
}

func (r *outboxPublishRecorder) assertConversationOrder(t *testing.T, conversationID types.ConversationID, want []int64) {
	t.Helper()
	r.mu.Lock()
	got := append([]int64(nil), r.order[conversationID]...)
	r.mu.Unlock()
	if len(got) != len(want) {
		t.Fatalf("unexpected publish count for %s: got %v want %v", conversationID, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected publish order for %s: got %v want %v", conversationID, got, want)
		}
	}
}

func assertOutboxStatusCounts(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	tenantID types.TenantID,
	wantPublished int64,
	wantPending int64,
	wantDLQ int64,
) {
	t.Helper()
	var published, pending, dlq int64
	if err := pool.QueryRow(ctx, `
SELECT
    count(*) FILTER (WHERE status = 'PUBLISHED')::bigint,
    count(*) FILTER (WHERE status = 'PENDING' AND published_at IS NULL)::bigint,
    count(*) FILTER (WHERE status = 'DLQ')::bigint
FROM message_outbox
WHERE tenant_id = $1
`, tenantID).Scan(&published, &pending, &dlq); err != nil {
		t.Fatalf("read outbox status counts: %v", err)
	}
	if published != wantPublished || pending != wantPending || dlq != wantDLQ {
		t.Fatalf(
			"unexpected outbox status counts: published=%d pending=%d dlq=%d want published=%d pending=%d dlq=%d",
			published,
			pending,
			dlq,
			wantPublished,
			wantPending,
			wantDLQ,
		)
	}
}

func updateMessageOutboxAuditState(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	tenantID types.TenantID,
	conversationID types.ConversationID,
	aggregateVersion int64,
	status string,
	retryCount int,
	lastError string,
	published bool,
	deadLettered bool,
) {
	t.Helper()
	now := time.Date(2026, 6, 14, 13, 0, 0, 0, time.UTC)
	var publishedAt any
	var deadLetteredAt any
	var nextRetryAt any
	if published {
		publishedAt = now
	}
	if deadLettered {
		deadLetteredAt = now
	}
	if status == types.OutboxStatusPending && retryCount > 0 {
		nextRetryAt = now.Add(time.Minute)
	}
	_, err := pool.Exec(ctx, `
UPDATE message_outbox
SET status = $4,
    retry_count = $5,
    last_error = $6,
    next_retry_at = $7,
    dead_lettered_at = $8,
    published_at = $9
WHERE tenant_id = $1
  AND conversation_id = $2
  AND aggregate_version = $3
`, tenantID, conversationID, aggregateVersion, status, retryCount, lastError, nextRetryAt, deadLetteredAt, publishedAt)
	if err != nil {
		t.Fatalf("update message outbox audit state: %v", err)
	}
}

func updateMessageOutboxEventType(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	tenantID types.TenantID,
	conversationID types.ConversationID,
	aggregateVersion int64,
	eventType string,
) {
	t.Helper()
	_, err := pool.Exec(ctx, `
UPDATE message_outbox
SET event_type = $4
WHERE tenant_id = $1
  AND conversation_id = $2
  AND aggregate_version = $3
`, tenantID, conversationID, aggregateVersion, eventType)
	if err != nil {
		t.Fatalf("update message outbox event type: %v", err)
	}
}

func readMessageOutboxEventID(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	tenantID types.TenantID,
	conversationID types.ConversationID,
	aggregateVersion int64,
) string {
	t.Helper()
	var eventID string
	if err := pool.QueryRow(ctx, `
SELECT event_id
FROM message_outbox
WHERE tenant_id = $1
  AND conversation_id = $2
  AND aggregate_version = $3
`, tenantID, conversationID, aggregateVersion).Scan(&eventID); err != nil {
		t.Fatalf("read message outbox event id: %v", err)
	}
	return eventID
}

func assertMessageOutboxState(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	tenantID types.TenantID,
	conversationID types.ConversationID,
	aggregateVersion int64,
	wantStatus string,
	wantRetryCount int,
	wantLastError string,
) {
	t.Helper()
	var status string
	var retryCount int
	var lastError string
	if err := pool.QueryRow(ctx, `
SELECT status, retry_count, COALESCE(last_error, '')
FROM message_outbox
WHERE tenant_id = $1
  AND conversation_id = $2
  AND aggregate_version = $3
`, tenantID, conversationID, aggregateVersion).Scan(&status, &retryCount, &lastError); err != nil {
		t.Fatalf("read message outbox state: %v", err)
	}
	if status != wantStatus || retryCount != wantRetryCount || lastError != wantLastError {
		t.Fatalf(
			"unexpected message outbox state: status=%s retry=%d last_error=%q want status=%s retry=%d last_error=%q",
			status,
			retryCount,
			lastError,
			wantStatus,
			wantRetryCount,
			wantLastError,
		)
	}
}

func assertMessageOutboxRepairAuditCount(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	tenantID string,
	conversationID string,
	want int64,
) {
	t.Helper()
	query := `
SELECT COUNT(*)
FROM message_outbox_repair_audit
WHERE tenant_id = $1
`
	args := []any{tenantID}
	if conversationID != "" {
		query += " AND conversation_id = $2"
		args = append(args, conversationID)
	}
	var got int64
	if err := pool.QueryRow(ctx, query, args...).Scan(&got); err != nil {
		t.Fatalf("count message outbox repair audit: %v", err)
	}
	if got != want {
		t.Fatalf("unexpected message outbox repair audit count: got %d want %d", got, want)
	}
}

func resetMessageCoreTables(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(ctx, `
TRUNCATE
    message_outbox_repair_audit,
    message_outbox,
    conversation_timeline_events,
    message_log,
    conversation_seq,
    message_change_history,
    message_command_idempotency,
    seq_allocation_journal,
    timeline_gap_markers
RESTART IDENTITY
`)
	if err != nil {
		t.Fatalf("reset message core tables: %v", err)
	}
}
