package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	outboxtrigger "github.com/qsyy0921/IM/services/policy-service/internal/trigger/outbox"
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

func TestOutboxStoreRepairDLQPolicyAuditIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetPolicyTables(t, ctx, pool)
	store := NewOutboxStore(pool)
	partitionKey := "tenant-policy:conversation-key"
	lowID, _ := insertPolicyAuditOutboxRow(t, ctx, pool, "policy-audit-store-repair-low", partitionKey, types.OutboxStatusDLQ, 3)
	insertPolicyAuditOutboxRow(t, ctx, pool, "policy-audit-store-repair-high", partitionKey, types.OutboxStatusPending, 0)
	if _, err := pool.Exec(ctx, `
UPDATE policy_decision_audit_outbox
SET last_error = 'publish failed',
    dead_lettered_at = now()
WHERE id = $1
`, lowID); err != nil {
		t.Fatalf("mark repair target dlq detail: %v", err)
	}

	stats, err := store.ProcessReadyBatch(ctx, 10, 3, time.Millisecond, func(ctx context.Context, messages []types.OutboxMessage) []error {
		t.Fatalf("publish must not be called while lower DLQ row blocks partition")
		return nil
	})
	if err != nil {
		t.Fatalf("process blocked ready: %v", err)
	}
	if stats.Fetched != 0 {
		t.Fatalf("expected no fetched messages before repair, got %+v", stats)
	}

	repairStats, err := store.RepairDLQEvents(ctx, []string{"policy-audit-store-repair-low", "policy-audit-store-repair-low", "missing-event"}, "policy-operator", "operator retried after kafka recovery", validatePolicyAuditOutboxMessageForTest)
	if err != nil {
		t.Fatalf("repair dlq: %v", err)
	}
	if repairStats.Requested != 2 || repairStats.Repaired != 1 || repairStats.Skipped != 1 || repairStats.Invalid != 0 {
		t.Fatalf("unexpected repair stats: %+v", repairStats)
	}
	assertPolicyAuditOutboxState(t, ctx, pool, lowID, types.OutboxStatusPending, 0)
	assertPolicyAuditOutboxRepairAudit(t, ctx, pool, "policy-audit-store-repair-low", "policy-operator", "operator retried after kafka recovery", "publish failed", 3, "REPAIRED", "")

	var published []string
	stats, err = store.ProcessReadyBatch(ctx, 10, 3, time.Millisecond, func(ctx context.Context, messages []types.OutboxMessage) []error {
		errs := make([]error, len(messages))
		for index, message := range messages {
			published = append(published, message.EventID)
			errs[index] = nil
		}
		return errs
	})
	if err != nil {
		t.Fatalf("process repaired ready: %v", err)
	}
	if stats.Fetched != 1 || stats.Published != 1 || len(published) != 1 || published[0] != "policy-audit-store-repair-low" {
		t.Fatalf("unexpected first publish after repair stats=%+v published=%v", stats, published)
	}
	stats, err = store.ProcessReadyBatch(ctx, 10, 3, time.Millisecond, func(ctx context.Context, messages []types.OutboxMessage) []error {
		errs := make([]error, len(messages))
		for index, message := range messages {
			published = append(published, message.EventID)
			errs[index] = nil
		}
		return errs
	})
	if err != nil {
		t.Fatalf("process unblocked ready: %v", err)
	}
	if stats.Fetched != 1 || stats.Published != 1 || len(published) != 2 || published[1] != "policy-audit-store-repair-high" {
		t.Fatalf("unexpected second publish after repair stats=%+v published=%v", stats, published)
	}
	assertPolicyAuditOutboxStatusCount(t, ctx, pool, types.OutboxStatusPublished, 2)
}

func TestOutboxStoreRepairDLQSkipsInvalidPolicyAuditIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetPolicyTables(t, ctx, pool)
	store := NewOutboxStore(pool)
	partitionKey := "tenant-policy:conversation-key"
	lowID, _ := insertPolicyAuditOutboxRow(t, ctx, pool, "policy-audit-store-invalid-low", partitionKey, types.OutboxStatusDLQ, 4)
	insertPolicyAuditOutboxRow(t, ctx, pool, "policy-audit-store-invalid-high", partitionKey, types.OutboxStatusPending, 0)
	validID, _ := insertPolicyAuditOutboxRow(t, ctx, pool, "policy-audit-store-invalid-mixed-valid", "tenant-policy:conversation-valid-key", types.OutboxStatusDLQ, 2)
	if _, err := pool.Exec(ctx, `
UPDATE policy_decision_audit_outbox
SET payload_json = '{"event_id":"policy-audit-store-invalid-low","unexpected":true}'::jsonb,
    last_error = 'unsupported payload field',
    dead_lettered_at = now()
WHERE id = $1
`, lowID); err != nil {
		t.Fatalf("corrupt repair target payload: %v", err)
	}

	repairStats, err := store.RepairDLQEvents(ctx, []string{"policy-audit-store-invalid-low", "policy-audit-store-invalid-mixed-valid"}, "policy-operator", "operator requested validation", validatePolicyAuditOutboxMessageForTest)
	if err != nil {
		t.Fatalf("repair invalid dlq: %v", err)
	}
	if repairStats.Requested != 2 || repairStats.Repaired != 1 || repairStats.Skipped != 1 || repairStats.Invalid != 1 {
		t.Fatalf("unexpected invalid repair stats: %+v", repairStats)
	}
	assertPolicyAuditOutboxState(t, ctx, pool, lowID, types.OutboxStatusDLQ, 4)
	assertPolicyAuditOutboxState(t, ctx, pool, validID, types.OutboxStatusPending, 0)
	assertPolicyAuditOutboxRepairAudit(t, ctx, pool, "policy-audit-store-invalid-low", "policy-operator", "operator requested validation", "unsupported payload field", 4, "SKIPPED", "validation_failed")
	assertPolicyAuditOutboxRepairAudit(t, ctx, pool, "policy-audit-store-invalid-mixed-valid", "policy-operator", "operator requested validation", "", 2, "REPAIRED", "")

	published := make(map[string]bool)
	stats, err := store.ProcessReadyBatch(ctx, 10, 3, time.Millisecond, func(ctx context.Context, messages []types.OutboxMessage) []error {
		errs := make([]error, len(messages))
		for index, message := range messages {
			published[message.EventID] = true
			errs[index] = nil
		}
		return errs
	})
	if err != nil {
		t.Fatalf("process valid mixed repair: %v", err)
	}
	if stats.Fetched != 1 || stats.Published != 1 || !published["policy-audit-store-invalid-mixed-valid"] {
		t.Fatalf("expected only valid mixed repair to publish, stats=%+v published=%v", stats, published)
	}

	stats, err = store.ProcessReadyBatch(ctx, 10, 3, time.Millisecond, func(ctx context.Context, messages []types.OutboxMessage) []error {
		t.Fatalf("publish must not be called while invalid lower DLQ row still blocks partition")
		return nil
	})
	if err != nil {
		t.Fatalf("process blocked ready after skipped repair: %v", err)
	}
	if stats.Fetched != 0 {
		t.Fatalf("expected no fetched messages after skipped repair, got %+v", stats)
	}
}

func TestOutboxStoreRepairDLQRequiresValidatorIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetPolicyTables(t, ctx, pool)
	store := NewOutboxStore(pool)
	_, err := store.RepairDLQEvents(ctx, []string{"policy-audit-store-repair-low"}, "policy-operator", "operator reason", nil)
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid argument for nil validator, got %v", err)
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

func assertPolicyAuditOutboxRepairAudit(t *testing.T, ctx context.Context, pool *pgxpool.Pool, eventID string, wantOperator string, wantReason string, wantPreviousError string, wantPreviousRetryCount int, wantOutcome string, wantSkipReason string) {
	t.Helper()
	var operator string
	var reason string
	var previousStatus string
	var previousRetryCount int
	var previousError string
	var outcome string
	var skipReason string
	err := pool.QueryRow(ctx, `
SELECT repair_operator, repair_reason, previous_status, previous_retry_count, previous_last_error, repair_outcome, skip_reason
FROM policy_decision_audit_outbox_repair_audit
WHERE tenant_id = 'tenant-policy'
  AND event_id = $1
`, eventID).Scan(&operator, &reason, &previousStatus, &previousRetryCount, &previousError, &outcome, &skipReason)
	if err != nil {
		t.Fatalf("query policy audit outbox repair audit: %v", err)
	}
	if operator != wantOperator || reason != wantReason || previousStatus != types.OutboxStatusDLQ || previousRetryCount != wantPreviousRetryCount || previousError != wantPreviousError || outcome != wantOutcome || skipReason != wantSkipReason {
		t.Fatalf(
			"unexpected repair audit operator=%q reason=%q previous_status=%q previous_retry_count=%d previous_error=%q outcome=%q skip_reason=%q",
			operator,
			reason,
			previousStatus,
			previousRetryCount,
			previousError,
			outcome,
			skipReason,
		)
	}
}

func assertPolicyAuditOutboxStatusCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, status string, want int) {
	t.Helper()
	var got int
	err := pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM policy_decision_audit_outbox
WHERE tenant_id = 'tenant-policy'
  AND status = $1
`, status).Scan(&got)
	if err != nil {
		t.Fatalf("count policy audit outbox status: %v", err)
	}
	if got != want {
		t.Fatalf("expected %d policy audit outbox rows with status %s, got %d", want, status, got)
	}
}

func validatePolicyAuditOutboxMessageForTest(message types.OutboxMessage) error {
	_, err := outboxtrigger.BuildPolicyEvent(message)
	return err
}
