package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/contacts-service/internal/types"
)

func TestOutboxStoreProcessReadyBatchMarksPublishedIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetContactsTables(t, ctx, pool)
	repository := newTestRepository(pool)
	store := NewOutboxStore(pool, WithOutboxClock(func() time.Time {
		return time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)
	}))

	sendResult, err := repository.SendContactRequest(ctx, sendCommand("alice", "bob", "send-1", "hello"))
	if err != nil {
		t.Fatalf("send contact request: %v", err)
	}
	if _, err := repository.RespondContactRequest(ctx, respondCommand("bob", sendResult.RequestID, "respond-1", types.ContactDecisionAccept)); err != nil {
		t.Fatalf("accept contact request: %v", err)
	}

	var published []string
	stats, err := store.ProcessReadyBatch(ctx, 10, 3, time.Millisecond, func(ctx context.Context, messages []types.OutboxMessage) []error {
		errs := make([]error, len(messages))
		for index, message := range messages {
			published = append(published, message.EventType)
			errs[index] = nil
		}
		return errs
	})
	if err != nil {
		t.Fatalf("process first ready: %v", err)
	}
	if stats.Fetched != 1 || stats.Published != 1 || len(published) != 1 || published[0] != types.ContactEventRequestCreated {
		t.Fatalf("unexpected stats=%+v published=%v", stats, published)
	}
	stats, err = store.ProcessReadyBatch(ctx, 10, 3, time.Millisecond, func(ctx context.Context, messages []types.OutboxMessage) []error {
		errs := make([]error, len(messages))
		for index, message := range messages {
			published = append(published, message.EventType)
			errs[index] = nil
		}
		return errs
	})
	if err != nil {
		t.Fatalf("process second ready: %v", err)
	}
	if stats.Fetched != 1 || stats.Published != 1 || len(published) != 2 || published[1] != types.ContactEventRequestAccepted {
		t.Fatalf("unexpected second stats=%+v published=%v", stats, published)
	}
	assertContactsOutboxStatusCount(t, ctx, pool, types.OutboxStatusPublished, 2)
	assertContactsOutboxStatusCount(t, ctx, pool, types.OutboxStatusPending, 0)
}

func TestOutboxStoreRetriesAndDLQIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetContactsTables(t, ctx, pool)
	repository := newTestRepository(pool)
	store := NewOutboxStore(pool)

	if _, err := repository.SendContactRequest(ctx, sendCommand("alice", "bob", "send-1", "hello")); err != nil {
		t.Fatalf("send contact request: %v", err)
	}
	stats, err := store.ProcessReadyBatch(ctx, 10, 3, time.Millisecond, func(ctx context.Context, messages []types.OutboxMessage) []error {
		return []error{errors.New("publish failed")}
	})
	if err != nil {
		t.Fatalf("process retry: %v", err)
	}
	if stats.Retried != 1 {
		t.Fatalf("expected retry stats, got %+v", stats)
	}
	assertContactsOutboxStatusCount(t, ctx, pool, types.OutboxStatusPending, 1)

	_, err = pool.Exec(ctx, `
UPDATE contacts_outbox
SET next_retry_at = now() - interval '1 second',
    retry_count = 2
WHERE tenant_id = 'tenant-contacts'
`)
	if err != nil {
		t.Fatalf("make outbox ready for dlq: %v", err)
	}
	stats, err = store.ProcessReadyBatch(ctx, 10, 3, time.Millisecond, func(ctx context.Context, messages []types.OutboxMessage) []error {
		return []error{errors.New("publish failed again")}
	})
	if err != nil {
		t.Fatalf("process dlq: %v", err)
	}
	if stats.DeadLettered != 1 {
		t.Fatalf("expected dlq stats, got %+v", stats)
	}
	assertContactsOutboxStatusCount(t, ctx, pool, types.OutboxStatusDLQ, 1)
}

func TestOutboxStoreBlocksHigherVersionByPartitionKeyIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetContactsTables(t, ctx, pool)
	repository := newTestRepository(pool)
	store := NewOutboxStore(pool)

	sendResult, err := repository.SendContactRequest(ctx, sendCommand("alice", "bob", "send-1", "hello"))
	if err != nil {
		t.Fatalf("send contact request: %v", err)
	}
	if _, err := repository.RespondContactRequest(ctx, respondCommand("bob", sendResult.RequestID, "respond-1", types.ContactDecisionAccept)); err != nil {
		t.Fatalf("accept contact request: %v", err)
	}
	_, err = pool.Exec(ctx, `
UPDATE contacts_outbox
SET status = 'DLQ'
WHERE event_type = $1
`, types.ContactEventRequestCreated)
	if err != nil {
		t.Fatalf("mark created dlq: %v", err)
	}
	stats, err := store.ProcessReadyBatch(ctx, 10, 3, time.Millisecond, func(ctx context.Context, messages []types.OutboxMessage) []error {
		t.Fatalf("publish should not be called while lower version is DLQ: %+v", messages)
		return nil
	})
	if err != nil {
		t.Fatalf("process ready: %v", err)
	}
	if stats.Fetched != 0 {
		t.Fatalf("expected no fetched messages, got %+v", stats)
	}
}

func assertContactsOutboxStatusCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, status string, want int) {
	t.Helper()
	var got int
	err := pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM contacts_outbox
WHERE tenant_id = 'tenant-contacts'
  AND status = $1
`, status).Scan(&got)
	if err != nil {
		t.Fatalf("count contacts outbox status: %v", err)
	}
	if got != want {
		t.Fatalf("expected %d contacts outbox rows with status %s, got %d", want, status, got)
	}
}
