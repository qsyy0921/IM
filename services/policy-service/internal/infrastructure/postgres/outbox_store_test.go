package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/policy-service/internal/types"
)

func TestOutboxStorePublishesReadyPolicyAuditIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetPolicyTables(t, ctx, pool)
	id, aggregateVersion := insertPolicyAuditOutboxRow(t, ctx, pool, "policy-audit-store-1", "tenant-policy:conversation-key", types.OutboxStatusPending, 0)

	store := NewOutboxStore(pool)
	var published []types.OutboxMessage
	stats, err := store.ProcessReadyBatch(ctx, 10, 5, time.Millisecond, func(ctx context.Context, messages []types.OutboxMessage) []error {
		published = append(published, messages...)
		return make([]error, len(messages))
	})
	if err != nil {
		t.Fatalf("process ready batch: %v", err)
	}
	if stats.Fetched != 1 || stats.Published != 1 || stats.Retried != 0 || stats.DeadLettered != 0 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	if len(published) != 1 || published[0].ID != id || published[0].AggregateVersion != aggregateVersion {
		t.Fatalf("unexpected published message: %+v", published)
	}
	var status string
	var publishedAt *time.Time
	if err := pool.QueryRow(ctx, `
SELECT status, published_at
FROM policy_decision_audit_outbox
WHERE id = $1
`, id).Scan(&status, &publishedAt); err != nil {
		t.Fatalf("read outbox status: %v", err)
	}
	if status != types.OutboxStatusPublished || publishedAt == nil {
		t.Fatalf("expected published row, got status=%s published_at=%v", status, publishedAt)
	}
}

func TestOutboxStoreRetriesAndDeadLettersPolicyAuditIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetPolicyTables(t, ctx, pool)
	id, _ := insertPolicyAuditOutboxRow(t, ctx, pool, "policy-audit-store-retry", "tenant-policy:conversation-key", types.OutboxStatusPending, 0)

	store := NewOutboxStore(pool)
	publishErr := errors.New("kafka write failed")
	stats, err := store.ProcessReadyBatch(ctx, 10, 2, time.Millisecond, func(ctx context.Context, messages []types.OutboxMessage) []error {
		errs := make([]error, len(messages))
		for i := range errs {
			errs[i] = publishErr
		}
		return errs
	})
	if err != nil {
		t.Fatalf("process retry batch: %v", err)
	}
	if stats.Retried != 1 || stats.DeadLettered != 0 {
		t.Fatalf("unexpected retry stats: %+v", stats)
	}
	assertPolicyAuditOutboxState(t, ctx, pool, id, types.OutboxStatusPending, 1)
	if _, err := pool.Exec(ctx, `UPDATE policy_decision_audit_outbox SET next_retry_at = now() WHERE id = $1`, id); err != nil {
		t.Fatalf("make retry ready: %v", err)
	}

	stats, err = store.ProcessReadyBatch(ctx, 10, 2, time.Millisecond, func(ctx context.Context, messages []types.OutboxMessage) []error {
		errs := make([]error, len(messages))
		for i := range errs {
			errs[i] = publishErr
		}
		return errs
	})
	if err != nil {
		t.Fatalf("process dlq batch: %v", err)
	}
	if stats.DeadLettered != 1 || stats.Retried != 0 {
		t.Fatalf("unexpected dlq stats: %+v", stats)
	}
	assertPolicyAuditOutboxState(t, ctx, pool, id, types.OutboxStatusDLQ, 2)
}

func TestOutboxStoreDLQBlocksHigherVersionPolicyAuditIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetPolicyTables(t, ctx, pool)
	partitionKey := "tenant-policy:conversation-key"
	insertPolicyAuditOutboxRow(t, ctx, pool, "policy-audit-store-low", partitionKey, types.OutboxStatusDLQ, 1)
	insertPolicyAuditOutboxRow(t, ctx, pool, "policy-audit-store-high", partitionKey, types.OutboxStatusPending, 0)

	store := NewOutboxStore(pool)
	stats, err := store.ProcessReadyBatch(ctx, 10, 5, time.Millisecond, func(ctx context.Context, messages []types.OutboxMessage) []error {
		t.Fatalf("publish must not be called while lower DLQ row blocks partition")
		return nil
	})
	if err != nil {
		t.Fatalf("process ready batch: %v", err)
	}
	if stats.Fetched != 0 || stats.Published != 0 {
		t.Fatalf("expected blocked batch, got %+v", stats)
	}
}

func insertPolicyAuditOutboxRow(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	eventID string,
	partitionKey string,
	status string,
	retryCount int,
) (int64, int64) {
	t.Helper()
	var id int64
	var aggregateVersion int64
	err := pool.QueryRow(ctx, `
INSERT INTO policy_decision_audit_outbox (
    event_id,
    tenant_id,
    aggregate_type,
    aggregate_id,
    mapping_version,
    actor_user_key,
    device_key,
    conversation_key,
    message_key,
    action,
    message_id_present,
    direct_peer_context_present,
    direct_peer_key,
    allowed,
    permission_version,
    classification,
    reason_code,
    partition_key,
    correlation_id,
    causation_id,
    trace_id,
    payload_json,
    status,
    retry_count
) VALUES (
    $1,
    'tenant-policy',
    'policy_decision',
    $2,
    1,
    'actor-key',
    'device-key',
    'conversation-key',
    'message-key',
    'SEND',
    true,
    true,
    'peer-key',
    true,
    41,
    'POLICY_RPC_ALLOWED',
    '',
    $2,
    'request-policy',
    'request-policy',
    'trace-policy',
    $3::jsonb,
    $4,
    $5
)
RETURNING id, aggregate_version
`, eventID, partitionKey, `{
	"event_id":"`+eventID+`",
	"tenant_id":"tenant-policy",
	"actor_user_key":"actor-key",
	"device_key":"device-key",
	"conversation_key":"conversation-key",
	"message_key":"message-key",
	"action":"SEND",
	"message_id_present":true,
	"direct_peer_context_present":true,
	"direct_peer_key":"peer-key",
	"allowed":true,
	"permission_version":41,
	"classification":"POLICY_RPC_ALLOWED",
	"reason_code":"",
	"trace_id":"trace-policy",
	"request_id":"request-policy",
	"decided_at":"2026-06-13T01:02:03Z"
}`, status, retryCount).Scan(&id, &aggregateVersion)
	if err != nil {
		t.Fatalf("insert policy audit outbox row: %v", err)
	}
	return id, aggregateVersion
}

func assertPolicyAuditOutboxState(t *testing.T, ctx context.Context, pool *pgxpool.Pool, id int64, wantStatus string, wantRetryCount int) {
	t.Helper()
	var status string
	var retryCount int
	if err := pool.QueryRow(ctx, `
SELECT status, retry_count
FROM policy_decision_audit_outbox
WHERE id = $1
`, id).Scan(&status, &retryCount); err != nil {
		t.Fatalf("read policy audit outbox row: %v", err)
	}
	if status != wantStatus || retryCount != wantRetryCount {
		t.Fatalf("unexpected outbox state: status=%s retry_count=%d want status=%s retry_count=%d", status, retryCount, wantStatus, wantRetryCount)
	}
}
