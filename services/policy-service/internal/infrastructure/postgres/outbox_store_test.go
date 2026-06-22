package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	outboxtrigger "github.com/qsyy0921/IM/services/policy-service/internal/trigger/outbox"
	"github.com/qsyy0921/IM/services/policy-service/internal/types"
)

func TestSanitizePolicyOutboxPublishErrorUsesStablePublicMessages(t *testing.T) {
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
			want: "policy audit outbox publish canceled",
		},
		{
			name: "deadline exceeded",
			err:  context.DeadlineExceeded,
			text: "deadline exceeded while publishing request-token=secret-token",
			want: "policy audit outbox publish timeout",
		},
		{
			name: "unsupported event",
			err:  errors.New("unsupported event_type=policy.future.v9 user=user1@example.com"),
			text: "unsupported event_type=policy.future.v9 user=user1@example.com",
			want: "policy audit outbox publish unsupported event",
		},
		{
			name: "invalid payload",
			err:  errors.New("malformed json payload for user=user1@example.com token=secret-token"),
			text: "malformed json payload for user=user1@example.com token=secret-token",
			want: "policy audit outbox publish invalid payload",
		},
		{
			name: "broker unavailable",
			err:  errors.New("kafka broker connection refused at 10.0.0.8 token=secret-token"),
			text: "kafka broker connection refused at 10.0.0.8 token=secret-token",
			want: "policy audit outbox publish broker unavailable",
		},
		{
			name: "unknown raw error",
			err:  errors.New("provider body user=user1@example.com token=secret-token nonce=secret-nonce"),
			text: "provider body user=user1@example.com token=secret-token nonce=secret-nonce",
			want: "policy audit outbox publish failed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sanitizePolicyOutboxPublishError(tt.err); got != tt.want {
				t.Fatalf("sanitize publish error = %q, want %q", got, tt.want)
			}
			if got := sanitizePolicyOutboxStoredError(tt.text); got != tt.want {
				t.Fatalf("sanitize stored error = %q, want %q", got, tt.want)
			}
			for _, forbidden := range []string{"user1@example.com", "secret-token", "secret-nonce", "10.0.0.8"} {
				if strings.Contains(tt.want, forbidden) {
					t.Fatalf("stable policy audit outbox error leaked sensitive text %q in %q", forbidden, tt.want)
				}
			}
		})
	}
	if got := sanitizePolicyOutboxStoredError("   "); got != "" {
		t.Fatalf("blank stored error = %q, want empty", got)
	}
}

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
	publishErr := errors.New("kafka write failed: broker body user=user1@example.com token=secret-token")
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
	assertPolicyAuditOutboxLastError(t, ctx, pool, id, "policy audit outbox publish broker unavailable", "user1@example.com", "secret-token", "broker body")
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
	assertPolicyAuditOutboxLastError(t, ctx, pool, id, "policy audit outbox publish broker unavailable", "user1@example.com", "secret-token", "broker body")
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
SET last_error = 'kafka unavailable: broker body user=user1@example.com token=secret-token',
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

	repairStats, err := store.RepairDLQEvents(ctx, []string{"policy-audit-store-repair-low", "policy-audit-store-repair-low", "missing-event"}, "policy-operator", "operator retried after kafka repair", validatePolicyAuditOutboxMessageForTest)
	if err != nil {
		t.Fatalf("repair dlq: %v", err)
	}
	if repairStats.Requested != 2 || repairStats.Repaired != 1 || repairStats.Skipped != 1 || repairStats.Invalid != 0 {
		t.Fatalf("unexpected repair stats: %+v", repairStats)
	}
	assertPolicyAuditOutboxState(t, ctx, pool, lowID, types.OutboxStatusPending, 0)
	assertPolicyAuditOutboxRepairAudit(t, ctx, pool, "policy-audit-store-repair-low", "policy-operator", "operator retried after kafka repair", "policy audit outbox publish broker unavailable", 3, "REPAIRED", "", "user1@example.com", "secret-token", "broker body")

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
    last_error = 'unsupported payload field user=user1@example.com token=secret-token',
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
	assertPolicyAuditOutboxRepairAudit(t, ctx, pool, "policy-audit-store-invalid-low", "policy-operator", "operator requested validation", "policy audit outbox publish unsupported event", 4, "SKIPPED", "validation_failed", "user1@example.com", "secret-token")
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

func TestOutboxStoreAuditOutboxReturnsLatestRowsIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetPolicyTables(t, ctx, pool)

	oldID, oldVersion := insertPolicyAuditOutboxRow(t, ctx, pool, "policy-audit-outbox-old", "tenant-policy:conversation-old", types.OutboxStatusPublished, 0)
	newID, newVersion := insertPolicyAuditOutboxRow(t, ctx, pool, "policy-audit-outbox-new", "tenant-policy:conversation-new", types.OutboxStatusDLQ, 3)
	if _, err := pool.Exec(ctx, `
UPDATE policy_decision_audit_outbox
SET created_at = now() - interval '2 minutes',
    published_at = now() - interval '90 seconds'
WHERE id = $1
`, oldID); err != nil {
		t.Fatalf("age old policy outbox row: %v", err)
	}
	if _, err := pool.Exec(ctx, `
UPDATE policy_decision_audit_outbox
SET last_error = 'relay failed: broker body user=user1@example.com token=secret-token',
    dead_lettered_at = now() - interval '30 seconds'
WHERE id = $1
`, newID); err != nil {
		t.Fatalf("mark new policy outbox row dlq detail: %v", err)
	}

	store := NewOutboxStore(pool)
	rows, err := store.AuditOutbox(ctx, OutboxAuditOptions{
		TenantID: "tenant-policy",
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("audit policy outbox: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected two rows, got %d", len(rows))
	}
	if rows[0].ID != newID || rows[0].EventID != "policy-audit-outbox-new" || rows[0].AggregateVersion != newVersion || rows[0].Status != types.OutboxStatusDLQ || rows[0].RetryCount != 3 || rows[0].LastError != "policy audit outbox publish broker unavailable" || rows[0].DeadLetteredAt == nil {
		t.Fatalf("unexpected latest policy outbox audit row: %+v", rows[0])
	}
	assertPolicyAuditOutboxErrorDoesNotContain(t, rows[0].LastError, "user1@example.com", "secret-token", "broker body")
	if rows[1].ID != oldID || rows[1].EventID != "policy-audit-outbox-old" || rows[1].AggregateVersion != oldVersion || rows[1].Status != types.OutboxStatusPublished || rows[1].PublishedAt == nil {
		t.Fatalf("unexpected older policy outbox audit row: %+v", rows[1])
	}
}

func TestOutboxStoreAuditOutboxFiltersStatusAndEventTypeIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetPolicyTables(t, ctx, pool)

	matchedID, matchedVersion := insertPolicyAuditOutboxRow(t, ctx, pool, "policy-audit-outbox-match", "tenant-policy:conversation-match", types.OutboxStatusDLQ, 2)
	insertPolicyAuditOutboxRow(t, ctx, pool, "policy-audit-outbox-pending", "tenant-policy:conversation-pending", types.OutboxStatusPending, 0)
	insertPolicyAuditOutboxRow(t, ctx, pool, "policy-audit-outbox-other-type", "tenant-policy:conversation-other-type", types.OutboxStatusDLQ, 1)
	if _, err := pool.Exec(ctx, `
UPDATE policy_decision_audit_outbox
SET event_type = 'policy.other_event.v1',
    last_error = 'other event failed',
    dead_lettered_at = now()
WHERE event_id = 'policy-audit-outbox-other-type'
`); err != nil {
		t.Fatalf("change policy outbox event type: %v", err)
	}
	if _, err := pool.Exec(ctx, `
UPDATE policy_decision_audit_outbox
SET last_error = 'malformed payload user=user1@example.com token=secret-token',
    dead_lettered_at = now()
WHERE id = $1
`, matchedID); err != nil {
		t.Fatalf("mark matched policy outbox dlq detail: %v", err)
	}

	store := NewOutboxStore(pool)
	rows, err := store.AuditOutbox(ctx, OutboxAuditOptions{
		TenantID:  "tenant-policy",
		Status:    "dlq",
		EventType: types.PolicyEventMessageActionDecision,
		Limit:     10,
	})
	if err != nil {
		t.Fatalf("audit filtered policy outbox: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one filtered row, got %d", len(rows))
	}
	if rows[0].ID != matchedID || rows[0].EventID != "policy-audit-outbox-match" || rows[0].AggregateVersion != matchedVersion || rows[0].Status != types.OutboxStatusDLQ || rows[0].EventType != types.PolicyEventMessageActionDecision || rows[0].LastError != "policy audit outbox publish invalid payload" {
		t.Fatalf("unexpected filtered policy outbox row: %+v", rows[0])
	}
	assertPolicyAuditOutboxErrorDoesNotContain(t, rows[0].LastError, "user1@example.com", "secret-token")
}

func TestOutboxStoreAuditOutboxFiltersCreatedAtIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetPolicyTables(t, ctx, pool)

	oldID, _ := insertPolicyAuditOutboxRow(t, ctx, pool, "policy-audit-created-old", "tenant-policy:conversation-old", types.OutboxStatusPending, 0)
	hitID, _ := insertPolicyAuditOutboxRow(t, ctx, pool, "policy-audit-created-hit", "tenant-policy:conversation-hit", types.OutboxStatusPublished, 0)
	newID, _ := insertPolicyAuditOutboxRow(t, ctx, pool, "policy-audit-created-new", "tenant-policy:conversation-new", types.OutboxStatusDLQ, 1)
	base := time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC)
	if _, err := pool.Exec(ctx, `
UPDATE policy_decision_audit_outbox
SET created_at = CASE id
    WHEN $1 THEN $4
    WHEN $2 THEN $5
    WHEN $3 THEN $6
    ELSE created_at
END
WHERE id IN ($1, $2, $3)
`, oldID, hitID, newID, base.Add(-time.Hour), base, base.Add(time.Hour)); err != nil {
		t.Fatalf("set policy outbox created_at values: %v", err)
	}

	store := NewOutboxStore(pool)
	rows, err := store.AuditOutbox(ctx, OutboxAuditOptions{
		TenantID:      "tenant-policy",
		CreatedAfter:  ptrTime(base.Add(-time.Minute)),
		CreatedBefore: ptrTime(base.Add(time.Minute)),
		Limit:         10,
	})
	if err != nil {
		t.Fatalf("audit policy outbox by created_at: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one row, got %d", len(rows))
	}
	if rows[0].ID != hitID || !rows[0].CreatedAt.Equal(base) {
		t.Fatalf("unexpected created_at filtered row: %+v", rows[0])
	}

	_, err = store.AuditOutbox(ctx, OutboxAuditOptions{
		TenantID:      "tenant-policy",
		CreatedAfter:  ptrTime(base),
		CreatedBefore: ptrTime(base),
		Limit:         10,
	})
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid created_at window, got %v", err)
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

func assertPolicyAuditOutboxLastError(t *testing.T, ctx context.Context, pool *pgxpool.Pool, id int64, want string, forbidden ...string) {
	t.Helper()
	var got string
	err := pool.QueryRow(ctx, `
SELECT last_error
FROM policy_decision_audit_outbox
WHERE id = $1
`, id).Scan(&got)
	if err != nil {
		t.Fatalf("read policy audit outbox last_error: %v", err)
	}
	if got != want {
		t.Fatalf("expected policy audit outbox last_error %q, got %q", want, got)
	}
	assertPolicyAuditOutboxErrorDoesNotContain(t, got, forbidden...)
}

func assertPolicyAuditOutboxErrorDoesNotContain(t *testing.T, value string, forbidden ...string) {
	t.Helper()
	for _, fragment := range forbidden {
		if fragment == "" {
			continue
		}
		if strings.Contains(value, fragment) {
			t.Fatalf("policy audit outbox error %q contains sensitive fragment %q", value, fragment)
		}
	}
}

func assertPolicyAuditOutboxRepairAudit(t *testing.T, ctx context.Context, pool *pgxpool.Pool, eventID string, wantOperator string, wantReason string, wantPreviousError string, wantPreviousRetryCount int, wantOutcome string, wantSkipReason string, forbidden ...string) {
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
	assertPolicyAuditOutboxErrorDoesNotContain(t, previousError, forbidden...)
}

func TestOutboxStoreAuditOutboxRepairsReturnsLatestRowsIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetPolicyTables(t, ctx, pool)

	_, err := pool.Exec(ctx, `
INSERT INTO policy_decision_audit_outbox_repair_audit (
    event_id, tenant_id, previous_status, previous_retry_count, previous_last_error, previous_dead_lettered_at,
    repair_operator, repair_reason, repair_outcome, skip_reason, repaired_at
) VALUES
    ('event-11', 'tenant-a', 'DLQ', 1, 'malformed payload user=user1@example.com', now() - interval '1 minute', 'operator-a', 'manual audit', 'REPAIRED', '', now() - interval '1 minute'),
    ('event-12', 'tenant-a', 'DLQ', 2, 'kafka unavailable token=secret-token broker body', now() - interval '2 minutes', 'operator-b', 'manual validation', 'SKIPPED', 'validation_failed', now())
`)
	if err != nil {
		t.Fatalf("seed policy outbox repair audit: %v", err)
	}

	store := NewOutboxStore(pool)
	rows, err := store.AuditOutboxRepairs(ctx, OutboxRepairAuditOptions{
		TenantID: "tenant-a",
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("audit policy outbox repairs: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected two rows, got %d", len(rows))
	}
	if rows[0].EventID != "event-12" || rows[0].Operator != "operator-b" || rows[0].Outcome != "SKIPPED" || rows[0].PreviousLastError != "policy audit outbox publish broker unavailable" {
		t.Fatalf("unexpected latest policy outbox repair row: %+v", rows[0])
	}
	assertPolicyAuditOutboxErrorDoesNotContain(t, rows[0].PreviousLastError, "secret-token", "broker body")
	if rows[1].EventID != "event-11" || rows[1].Operator != "operator-a" || rows[1].Outcome != "REPAIRED" || rows[1].PreviousLastError != "policy audit outbox publish invalid payload" {
		t.Fatalf("unexpected older policy outbox repair row: %+v", rows[1])
	}
	assertPolicyAuditOutboxErrorDoesNotContain(t, rows[1].PreviousLastError, "user1@example.com")
}

func TestOutboxStoreAuditOutboxRepairsFiltersOperatorAndOutcomeIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetPolicyTables(t, ctx, pool)

	_, err := pool.Exec(ctx, `
INSERT INTO policy_decision_audit_outbox_repair_audit (
    event_id, tenant_id, previous_status, previous_retry_count, previous_last_error, previous_dead_lettered_at,
    repair_operator, repair_reason, repair_outcome, skip_reason, repaired_at
) VALUES
    ('event-21', 'tenant-b', 'DLQ', 1, 'publish failed', now() - interval '1 minute', 'operator-a', 'manual audit', 'REPAIRED', '', now()),
    ('event-22', 'tenant-b', 'DLQ', 2, 'unsupported payload user=user1@example.com token=secret-token', now() - interval '2 minutes', 'operator-b', 'manual validation', 'SKIPPED', 'validation_failed', now() - interval '1 minute')
`)
	if err != nil {
		t.Fatalf("seed policy outbox repair audit: %v", err)
	}

	store := NewOutboxStore(pool)
	rows, err := store.AuditOutboxRepairs(ctx, OutboxRepairAuditOptions{
		TenantID: "tenant-b",
		Operator: "operator-b",
		Outcome:  "SKIPPED",
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("audit policy outbox repairs with filters: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one row, got %d", len(rows))
	}
	if rows[0].EventID != "event-22" || rows[0].Operator != "operator-b" || rows[0].Outcome != "SKIPPED" || rows[0].PreviousLastError != "policy audit outbox publish unsupported event" {
		t.Fatalf("unexpected filtered policy outbox repair row: %+v", rows[0])
	}
	assertPolicyAuditOutboxErrorDoesNotContain(t, rows[0].PreviousLastError, "user1@example.com", "secret-token")
}

func TestOutboxStoreAuditOutboxRepairsFiltersRepairedAtIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetPolicyTables(t, ctx, pool)

	base := time.Date(2026, 6, 16, 2, 0, 0, 0, time.UTC)
	_, err := pool.Exec(ctx, `
INSERT INTO policy_decision_audit_outbox_repair_audit (
    event_id, tenant_id, previous_status, previous_retry_count, previous_last_error, previous_dead_lettered_at,
    repair_operator, repair_reason, repair_outcome, skip_reason, repaired_at
) VALUES
    ('event-window-old', 'tenant-window', 'DLQ', 1, 'publish failed', $1::timestamptz, 'operator-a', 'manual old', 'REPAIRED', '', $2::timestamptz),
    ('event-window-hit', 'tenant-window', 'DLQ', 2, 'provider unavailable', $1::timestamptz, 'operator-b', 'manual hit', 'SKIPPED', 'validation_failed', $3::timestamptz),
    ('event-window-new', 'tenant-window', 'DLQ', 3, 'provider unavailable', $1::timestamptz, 'operator-c', 'manual new', 'REPAIRED', '', $4::timestamptz)
`, base, base.Add(-time.Hour), base, base.Add(time.Hour))
	if err != nil {
		t.Fatalf("seed policy outbox repair audit window rows: %v", err)
	}

	store := NewOutboxStore(pool)
	rows, err := store.AuditOutboxRepairs(ctx, OutboxRepairAuditOptions{
		TenantID:       "tenant-window",
		RepairedAfter:  ptrTime(base.Add(-30 * time.Minute)),
		RepairedBefore: ptrTime(base.Add(30 * time.Minute)),
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("audit policy outbox repairs by repaired_at: %v", err)
	}
	if len(rows) != 1 || rows[0].EventID != "event-window-hit" {
		t.Fatalf("unexpected repaired_at window rows: %+v", rows)
	}

	_, err = store.AuditOutboxRepairs(ctx, OutboxRepairAuditOptions{
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
	pool := openTestPool(t)
	ctx := context.Background()
	resetPolicyTables(t, ctx, pool)

	_, err := pool.Exec(ctx, `
INSERT INTO policy_decision_audit_outbox_repair_audit (
    event_id, tenant_id, previous_status, previous_retry_count, previous_last_error, previous_dead_lettered_at,
    repair_operator, repair_reason, repair_outcome, skip_reason, repaired_at
) VALUES
    ('cleanup-policy-event-old-1', 'tenant-cleanup', 'DLQ', 1, 'publish failed', now() - interval '2 hours', 'operator-a', 'manual repair', 'REPAIRED', '', now() - interval '2 hours'),
    ('cleanup-policy-event-old-2', 'tenant-cleanup', 'DLQ', 2, 'validation failed', now() - interval '90 minutes', 'operator-b', 'manual validation', 'SKIPPED', 'validation_failed', now() - interval '90 minutes'),
    ('cleanup-policy-event-fresh', 'tenant-cleanup', 'DLQ', 1, 'recent failure', now() - interval '10 minutes', 'operator-a', 'manual repair', 'REPAIRED', '', now() - interval '10 minutes')
`)
	if err != nil {
		t.Fatalf("seed cleanup policy outbox repair audit: %v", err)
	}

	store := NewOutboxStore(pool)
	stats, err := store.CleanupOutboxRepairs(ctx, OutboxRepairCleanupOptions{
		TenantID: "tenant-cleanup",
		Cutoff:   time.Now().UTC().Add(-30 * time.Minute),
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("cleanup policy outbox repairs: %v", err)
	}
	if stats.Deleted != 2 {
		t.Fatalf("expected 2 deleted rows, got %+v", stats)
	}
	assertPolicyAuditOutboxRepairAuditCount(t, ctx, pool, "tenant-cleanup", 1)
}

func TestOutboxStoreCleanupOutboxRepairsDryRunDoesNotDeleteIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetPolicyTables(t, ctx, pool)

	_, err := pool.Exec(ctx, `
INSERT INTO policy_decision_audit_outbox_repair_audit (
    event_id, tenant_id, previous_status, previous_retry_count, previous_last_error, previous_dead_lettered_at,
    repair_operator, repair_reason, repair_outcome, skip_reason, repaired_at
) VALUES
    ('cleanup-policy-dry-run-old-1', 'tenant-cleanup-dry-run', 'DLQ', 1, 'publish failed', now() - interval '2 hours', 'operator-a', 'manual repair', 'REPAIRED', '', now() - interval '2 hours'),
    ('cleanup-policy-dry-run-old-2', 'tenant-cleanup-dry-run', 'DLQ', 2, 'validation failed', now() - interval '90 minutes', 'operator-b', 'manual validation', 'SKIPPED', 'validation_failed', now() - interval '90 minutes'),
    ('cleanup-policy-dry-run-fresh', 'tenant-cleanup-dry-run', 'DLQ', 1, 'recent failure', now() - interval '10 minutes', 'operator-a', 'manual repair', 'REPAIRED', '', now() - interval '10 minutes')
`)
	if err != nil {
		t.Fatalf("seed dry-run cleanup policy outbox repair audit: %v", err)
	}

	store := NewOutboxStore(pool)
	stats, err := store.CleanupOutboxRepairs(ctx, OutboxRepairCleanupOptions{
		TenantID: "tenant-cleanup-dry-run",
		Cutoff:   time.Now().UTC().Add(-30 * time.Minute),
		Limit:    10,
		DryRun:   true,
	})
	if err != nil {
		t.Fatalf("dry-run cleanup policy outbox repairs: %v", err)
	}
	if stats.Deleted != 2 {
		t.Fatalf("expected 2 dry-run deleted rows, got %+v", stats)
	}
	assertPolicyAuditOutboxRepairAuditCount(t, ctx, pool, "tenant-cleanup-dry-run", 3)
}

func ptrTime(value time.Time) *time.Time {
	return &value
}

func TestOutboxStoreCleanupOutboxRepairsHonorsBatchLimitIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetPolicyTables(t, ctx, pool)

	_, err := pool.Exec(ctx, `
INSERT INTO policy_decision_audit_outbox_repair_audit (
    event_id, tenant_id, previous_status, previous_retry_count, previous_last_error, previous_dead_lettered_at,
    repair_operator, repair_reason, repair_outcome, skip_reason, repaired_at
) VALUES
    ('cleanup-policy-batch-1', 'tenant-batch', 'DLQ', 1, 'error 1', now() - interval '4 hours', 'operator-a', 'manual repair', 'REPAIRED', '', now() - interval '4 hours'),
    ('cleanup-policy-batch-2', 'tenant-batch', 'DLQ', 1, 'error 2', now() - interval '3 hours', 'operator-a', 'manual repair', 'REPAIRED', '', now() - interval '3 hours'),
    ('cleanup-policy-batch-3', 'tenant-batch', 'DLQ', 1, 'error 3', now() - interval '2 hours', 'operator-a', 'manual repair', 'REPAIRED', '', now() - interval '2 hours')
`)
	if err != nil {
		t.Fatalf("seed batch cleanup policy outbox repair audit: %v", err)
	}

	store := NewOutboxStore(pool)
	stats, err := store.CleanupOutboxRepairs(ctx, OutboxRepairCleanupOptions{
		TenantID: "tenant-batch",
		Cutoff:   time.Now().UTC().Add(-30 * time.Minute),
		Limit:    2,
	})
	if err != nil {
		t.Fatalf("cleanup policy outbox repairs with limit: %v", err)
	}
	if stats.Deleted != 2 {
		t.Fatalf("expected 2 deleted rows, got %+v", stats)
	}
	assertPolicyAuditOutboxRepairAuditCount(t, ctx, pool, "tenant-batch", 1)
}

func TestOutboxStoreCleanupOutboxRepairsFiltersOperatorAndOutcomeIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetPolicyTables(t, ctx, pool)

	_, err := pool.Exec(ctx, `
INSERT INTO policy_decision_audit_outbox_repair_audit (
    event_id, tenant_id, previous_status, previous_retry_count, previous_last_error, previous_dead_lettered_at,
    repair_operator, repair_reason, repair_outcome, skip_reason, repaired_at
) VALUES
    ('cleanup-policy-filter-match', 'tenant-filter', 'DLQ', 1, 'publish failed', now() - interval '2 hours', 'operator-match', 'manual repair', 'SKIPPED', 'validation_failed', now() - interval '2 hours'),
    ('cleanup-policy-filter-operator', 'tenant-filter', 'DLQ', 1, 'publish failed', now() - interval '2 hours', 'operator-other', 'manual repair', 'SKIPPED', 'validation_failed', now() - interval '2 hours'),
    ('cleanup-policy-filter-outcome', 'tenant-filter', 'DLQ', 1, 'publish failed', now() - interval '2 hours', 'operator-match', 'manual repair', 'REPAIRED', '', now() - interval '2 hours')
`)
	if err != nil {
		t.Fatalf("seed filtered cleanup policy outbox repair audit: %v", err)
	}

	store := NewOutboxStore(pool)
	stats, err := store.CleanupOutboxRepairs(ctx, OutboxRepairCleanupOptions{
		TenantID: "tenant-filter",
		Operator: "operator-match",
		Outcome:  "skipped",
		Cutoff:   time.Now().UTC().Add(-30 * time.Minute),
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("cleanup policy outbox repairs with filters: %v", err)
	}
	if stats.Deleted != 1 {
		t.Fatalf("expected 1 deleted row, got %+v", stats)
	}

	var remaining []string
	rows, err := pool.Query(ctx, `
SELECT event_id
FROM policy_decision_audit_outbox_repair_audit
WHERE tenant_id = 'tenant-filter'
ORDER BY event_id
`)
	if err != nil {
		t.Fatalf("query remaining cleanup rows: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var eventID string
		if err := rows.Scan(&eventID); err != nil {
			t.Fatalf("scan remaining cleanup row: %v", err)
		}
		remaining = append(remaining, eventID)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate remaining cleanup rows: %v", err)
	}
	if len(remaining) != 2 || remaining[0] != "cleanup-policy-filter-operator" || remaining[1] != "cleanup-policy-filter-outcome" {
		t.Fatalf("unexpected remaining cleanup rows: %v", remaining)
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

func assertPolicyAuditOutboxRepairAuditCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tenantID string, want int) {
	t.Helper()
	var got int
	err := pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM policy_decision_audit_outbox_repair_audit
WHERE tenant_id = $1
`, tenantID).Scan(&got)
	if err != nil {
		t.Fatalf("count policy outbox repair audit: %v", err)
	}
	if got != want {
		t.Fatalf("expected %d policy outbox repair audit rows, got %d", want, got)
	}
}

func validatePolicyAuditOutboxMessageForTest(message types.OutboxMessage) error {
	_, err := outboxtrigger.BuildPolicyEvent(message)
	return err
}
