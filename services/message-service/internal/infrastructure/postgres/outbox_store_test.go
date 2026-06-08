package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/message-service/internal/types"
)

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
		return errors.New("kafka unavailable")
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
		!strings.Contains(status.LastError, "kafka unavailable") {
		t.Fatalf("unexpected outbox status: %+v", status)
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
		!strings.Contains(status.LastError, "kafka unavailable") {
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

func resetMessageCoreTables(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(ctx, `
TRUNCATE
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
