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
