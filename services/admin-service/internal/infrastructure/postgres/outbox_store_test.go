package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/admin-service/internal/types"
)

func TestOutboxStoreProcessReadyBatchPublishesSubmittedIntegration(t *testing.T) {
	pool := openAdminTestPool(t)
	ctx := context.Background()
	resetAdminTables(t, ctx, pool)
	repository := NewRepository(pool)

	prepared := prepareAdminOperation(t, "admin-outbox-publish", "admop_outbox_publish", types.RiskLevelMedium)
	operation, _, err := repository.CreateAdminOperation(ctx, prepared)
	if err != nil {
		t.Fatalf("create admin operation: %v", err)
	}

	store := NewOutboxStore(pool)
	var published []string
	stats, err := store.ProcessReadyBatch(ctx, 10, 3, time.Millisecond, func(_ context.Context, messages []types.OutboxMessage) []error {
		errs := make([]error, len(messages))
		for _, message := range messages {
			published = append(published, message.EventID)
			if message.EventType != types.AdminEventOperationSubmitted || message.Producer != "admin-service" {
				t.Fatalf("unexpected outbox message: %+v", message)
			}
		}
		return errs
	})
	if err != nil {
		t.Fatalf("process ready batch: %v", err)
	}
	eventID := "evt_" + operation.OperationID + "_submitted"
	if stats.Fetched != 1 || stats.Published != 1 || len(published) != 1 || published[0] != eventID {
		t.Fatalf("unexpected batch stats=%+v published=%v", stats, published)
	}
	assertAdminOutboxStatus(t, ctx, pool, operation.TenantID, eventID, types.OutboxStatusPublished)
}

func TestOutboxStoreDLQBlocksLaterOperationEventsIntegration(t *testing.T) {
	pool := openAdminTestPool(t)
	ctx := context.Background()
	resetAdminTables(t, ctx, pool)
	repository := NewRepository(pool)

	prepared := prepareAdminOperation(t, "admin-outbox-dlq", "admop_outbox_dlq", types.RiskLevelMedium)
	operation, _, err := repository.CreateAdminOperation(ctx, prepared)
	if err != nil {
		t.Fatalf("create admin operation: %v", err)
	}
	laterEventID := insertAdminOutboxEvent(t, ctx, pool, operation, "admop_outbox_dlq:future", 2)

	store := NewOutboxStore(pool)
	stats, err := store.ProcessReadyBatch(ctx, 10, 1, time.Millisecond, func(_ context.Context, messages []types.OutboxMessage) []error {
		errs := make([]error, len(messages))
		for index := range errs {
			errs[index] = errors.New("kafka broker raw failure with internal detail")
		}
		return errs
	})
	if err != nil {
		t.Fatalf("process failing batch: %v", err)
	}
	eventID := "evt_" + operation.OperationID + "_submitted"
	if stats.Fetched != 1 || stats.Published != 0 || stats.DeadLettered != 1 {
		t.Fatalf("unexpected failing batch stats: %+v", stats)
	}
	assertAdminOutboxStatus(t, ctx, pool, operation.TenantID, eventID, types.OutboxStatusDLQ)
	assertAdminOutboxStatus(t, ctx, pool, operation.TenantID, laterEventID, types.OutboxStatusPending)
	assertAdminOutboxLastError(t, ctx, pool, operation.TenantID, eventID, "admin outbox publish broker unavailable")

	stats, err = store.ProcessReadyBatch(ctx, 10, 3, time.Millisecond, func(_ context.Context, messages []types.OutboxMessage) []error {
		t.Fatalf("later event must stay blocked while prior event is DLQ: %+v", messages)
		return nil
	})
	if err != nil {
		t.Fatalf("process blocked batch: %v", err)
	}
	if stats.Fetched != 0 || stats.Published != 0 {
		t.Fatalf("expected no ready rows while earlier event is DLQ, got %+v", stats)
	}
}

func TestOutboxStoreRetryKeepsStablePublicErrorIntegration(t *testing.T) {
	pool := openAdminTestPool(t)
	ctx := context.Background()
	resetAdminTables(t, ctx, pool)
	repository := NewRepository(pool)

	prepared := prepareAdminOperation(t, "admin-outbox-retry", "admop_outbox_retry", types.RiskLevelMedium)
	operation, _, err := repository.CreateAdminOperation(ctx, prepared)
	if err != nil {
		t.Fatalf("create admin operation: %v", err)
	}

	store := NewOutboxStore(pool, WithOutboxClock(func() time.Time {
		return time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	}))
	stats, err := store.ProcessReadyBatch(ctx, 10, 3, time.Millisecond, func(_ context.Context, messages []types.OutboxMessage) []error {
		errs := make([]error, len(messages))
		for index := range errs {
			errs[index] = errors.New("duplicate key value violates unique constraint admin_secret")
		}
		return errs
	})
	if err != nil {
		t.Fatalf("process retry batch: %v", err)
	}
	eventID := "evt_" + operation.OperationID + "_submitted"
	if stats.Fetched != 1 || stats.Retried != 1 || stats.DeadLettered != 0 {
		t.Fatalf("unexpected retry stats: %+v", stats)
	}
	assertAdminOutboxStatus(t, ctx, pool, operation.TenantID, eventID, types.OutboxStatusPending)
	assertAdminOutboxRetry(t, ctx, pool, operation.TenantID, eventID, 1)
	assertAdminOutboxLastError(t, ctx, pool, operation.TenantID, eventID, "admin outbox publish failed")
}

func insertAdminOutboxEvent(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	operation types.AdminOperation,
	eventID string,
	eventVersion int,
) string {
	t.Helper()
	_, err := pool.Exec(ctx, `
INSERT INTO admin_outbox (
    event_id,
    tenant_id,
    operation_id,
    aggregate_type,
    aggregate_id,
    event_type,
    event_version,
    partition_key,
    payload_json,
    status,
    created_at,
    updated_at
) VALUES ($1, $2, $3, 'admin_operation', $3, 'admin.operation.submitted.v1', $4, $5, $6::jsonb, 'PENDING', now() + interval '1 second', now())
`, eventID, operation.TenantID, operation.OperationID, eventVersion, string(operation.TenantID)+":"+operation.OperationID, adminOutboxPayload(operation))
	if err != nil {
		t.Fatalf("insert admin outbox event: %v", err)
	}
	return eventID
}

func adminOutboxPayload(operation types.AdminOperation) string {
	return `{"tenant_id":"` + string(operation.TenantID) + `","operation_id":"` + operation.OperationID + `","operation_type":"` + operation.OperationType + `","target_ref_hash":"` + operation.TargetRefHash + `","risk_level":"` + operation.RiskLevel + `","status":"` + operation.Status + `","requested_by_hash":"sha256:requester","payload_schema_version":"` + operation.PayloadSchemaVersion + `","payload_hash":"` + operation.PayloadHash + `","reason_ref":"` + operation.ReasonRef + `"}`
}

func assertAdminOutboxStatus(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tenantID types.TenantID, eventID string, want string) {
	t.Helper()
	var got string
	if err := pool.QueryRow(ctx, `SELECT status FROM admin_outbox WHERE tenant_id = $1 AND event_id = $2`, tenantID, eventID).Scan(&got); err != nil {
		t.Fatalf("query admin outbox status: %v", err)
	}
	if got != want {
		t.Fatalf("unexpected admin outbox status for %s: got %s want %s", eventID, got, want)
	}
}

func assertAdminOutboxRetry(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tenantID types.TenantID, eventID string, want int) {
	t.Helper()
	var got int
	if err := pool.QueryRow(ctx, `SELECT retry_count FROM admin_outbox WHERE tenant_id = $1 AND event_id = $2`, tenantID, eventID).Scan(&got); err != nil {
		t.Fatalf("query admin outbox retry count: %v", err)
	}
	if got != want {
		t.Fatalf("unexpected admin outbox retry count for %s: got %d want %d", eventID, got, want)
	}
}

func assertAdminOutboxLastError(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tenantID types.TenantID, eventID string, want string) {
	t.Helper()
	var got string
	if err := pool.QueryRow(ctx, `SELECT last_error FROM admin_outbox WHERE tenant_id = $1 AND event_id = $2`, tenantID, eventID).Scan(&got); err != nil {
		t.Fatalf("query admin outbox last_error: %v", err)
	}
	if got != want {
		t.Fatalf("unexpected admin outbox last_error for %s: got %q want %q", eventID, got, want)
	}
}
