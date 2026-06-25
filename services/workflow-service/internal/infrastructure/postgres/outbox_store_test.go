package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/workflow-service/internal/types"
)

func TestOutboxStorePublishesReadyRowsIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openWorkflowTestPool(t)
	resetWorkflowTables(t, ctx, pool)
	insertWorkflowOutboxRow(t, ctx, pool, "evt-publish-1", "wf-outbox-1", types.WorkflowEventSubmitted, 0, "PENDING", time.Now().UTC().Add(-time.Minute))

	store := NewOutboxStore(pool, WithOutboxClock(func() time.Time {
		return time.Date(2026, 6, 25, 10, 0, 0, 0, time.UTC)
	}))
	stats, err := store.ProcessReadyBatch(ctx, 10, 3, time.Millisecond, func(_ context.Context, messages []types.OutboxMessage) []error {
		if len(messages) != 1 {
			t.Fatalf("expected one message, got %d", len(messages))
		}
		if messages[0].EventID != "evt-publish-1" ||
			messages[0].WorkflowID != "wf-outbox-1" ||
			messages[0].Producer != "workflow-service" {
			t.Fatalf("unexpected message: %+v", messages[0])
		}
		return []error{nil}
	})
	if err != nil {
		t.Fatalf("process ready batch: %v", err)
	}
	if stats.Fetched != 1 || stats.Published != 1 || stats.Retried != 0 || stats.DeadLettered != 0 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	assertWorkflowOutboxState(t, ctx, pool, "evt-publish-1", types.OutboxStatusPublished, 0)
}

func TestOutboxStoreRetriesThenDeadLettersIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openWorkflowTestPool(t)
	resetWorkflowTables(t, ctx, pool)
	insertWorkflowOutboxRow(t, ctx, pool, "evt-retry-1", "wf-outbox-2", types.WorkflowEventSubmitted, 1, "PENDING", time.Now().UTC().Add(-time.Minute))

	store := NewOutboxStore(pool, WithOutboxClock(func() time.Time {
		return time.Date(2026, 6, 25, 10, 0, 0, 0, time.UTC)
	}))
	stats, err := store.ProcessReadyBatch(ctx, 10, 3, time.Millisecond, func(_ context.Context, messages []types.OutboxMessage) []error {
		return []error{errors.New("kafka broker unavailable")}
	})
	if err != nil {
		t.Fatalf("process retry batch: %v", err)
	}
	if stats.Fetched != 1 || stats.Retried != 1 || stats.Published != 0 || stats.DeadLettered != 0 {
		t.Fatalf("unexpected retry stats: %+v", stats)
	}
	assertWorkflowOutboxState(t, ctx, pool, "evt-retry-1", types.OutboxStatusPending, 2)
	if _, err := pool.Exec(ctx, `
UPDATE workflow_outbox SET next_retry_at = now() - interval '1 minute' WHERE event_id = 'evt-retry-1'
`); err != nil {
		t.Fatalf("make retry row ready: %v", err)
	}

	stats, err = store.ProcessReadyBatch(ctx, 10, 3, time.Millisecond, func(_ context.Context, messages []types.OutboxMessage) []error {
		return []error{errors.New("kafka broker unavailable")}
	})
	if err != nil {
		t.Fatalf("process dlq batch: %v", err)
	}
	if stats.Fetched != 1 || stats.DeadLettered != 1 || stats.Published != 0 || stats.Retried != 0 {
		t.Fatalf("unexpected dlq stats: %+v", stats)
	}
	assertWorkflowOutboxState(t, ctx, pool, "evt-retry-1", types.OutboxStatusDLQ, 3)
}

func TestOutboxStoreDLQBlocksLaterWorkflowRowsIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openWorkflowTestPool(t)
	resetWorkflowTables(t, ctx, pool)
	now := time.Now().UTC().Add(-time.Minute)
	insertWorkflowOutboxRow(t, ctx, pool, "evt-blocking-1", "wf-outbox-3", types.WorkflowEventSubmitted, 0, "DLQ", now)
	insertWorkflowOutboxRow(t, ctx, pool, "evt-later-1", "wf-outbox-3", types.WorkflowEventDecisionRecorded, 0, "PENDING", now.Add(time.Second))

	store := NewOutboxStore(pool)
	stats, err := store.ProcessReadyBatch(ctx, 10, 3, time.Millisecond, func(_ context.Context, messages []types.OutboxMessage) []error {
		if len(messages) != 0 {
			t.Fatalf("blocked workflow row should not be fetched: %+v", messages)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("process blocked batch: %v", err)
	}
	if stats.Fetched != 0 || stats.Published != 0 {
		t.Fatalf("unexpected stats for blocked batch: %+v", stats)
	}
	assertWorkflowOutboxState(t, ctx, pool, "evt-later-1", types.OutboxStatusPending, 0)
}

func insertWorkflowOutboxRow(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	eventID string,
	workflowID string,
	eventType string,
	retryCount int,
	status string,
	createdAt time.Time,
) {
	t.Helper()
	_, err := pool.Exec(ctx, `
INSERT INTO workflow_outbox (
    event_id, tenant_id, workflow_id, aggregate_type, aggregate_id, event_type,
    event_version, partition_key, payload_json, status, retry_count,
    available_at, created_at, updated_at
) VALUES (
    $1, 'tenant-workflow-test', $2, 'workflow', $2, $3,
    1, 'tenant-workflow-test:' || $2,
    jsonb_build_object('tenant_id', 'tenant-workflow-test', 'workflow_id', $2, 'workflow_type', 'ACTION_APPROVAL', 'status', 'WAITING_DECISION'),
    $4, $5, now() - interval '1 minute', $6, now()
)
`, eventID, workflowID, eventType, status, retryCount, createdAt)
	if err != nil {
		t.Fatalf("insert workflow outbox row: %v", err)
	}
}

func assertWorkflowOutboxState(t *testing.T, ctx context.Context, pool *pgxpool.Pool, eventID string, wantStatus string, wantRetryCount int) {
	t.Helper()
	var status string
	var retryCount int
	if err := pool.QueryRow(ctx, `
SELECT status, retry_count FROM workflow_outbox WHERE event_id = $1
`, eventID).Scan(&status, &retryCount); err != nil {
		t.Fatalf("query workflow outbox state: %v", err)
	}
	if status != wantStatus || retryCount != wantRetryCount {
		t.Fatalf("unexpected outbox state for %s: status=%s retry=%d", eventID, status, retryCount)
	}
}
