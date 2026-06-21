package postgres

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/admin-service/internal/domain"
	"github.com/qsyy0921/IM/services/admin-service/internal/types"
)

func TestRepositoryAdminFirstPathIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openAdminTestPool(t)
	resetAdminTables(t, ctx, pool)
	repository := NewRepository(pool)

	prepared := prepareAdminOperation(t, "admin-create-idem-1", "admop_test_1", types.RiskLevelHigh)
	operation, replayed, err := repository.CreateAdminOperation(ctx, prepared)
	if err != nil {
		t.Fatalf("create admin operation: %v", err)
	}
	if replayed || operation.Status != types.OperationStatusSubmitted {
		t.Fatalf("unexpected create result: replayed=%v operation=%+v", replayed, operation)
	}
	replayedOperation, replayed, err := repository.CreateAdminOperation(ctx, prepared)
	if err != nil {
		t.Fatalf("replay admin operation: %v", err)
	}
	if !replayed || replayedOperation.OperationID != operation.OperationID {
		t.Fatalf("unexpected replay: replayed=%v operation=%+v", replayed, replayedOperation)
	}
	conflict := prepareAdminOperation(t, "admin-create-idem-1", "admop_conflict", types.RiskLevelHigh)
	conflict.Command.OperationType = "USER_UNBAN"
	conflict.CommandHash = "sha256:different"
	if _, _, err := repository.CreateAdminOperation(ctx, conflict); !errors.Is(err, types.ErrAlreadyExists) {
		t.Fatalf("expected create idempotency conflict, got %v", err)
	}

	selfApproval := prepareApproval(t, operation.OperationID, "admin-approve-self", "admappr_self", operation.RequestedBy, types.DecisionApprove)
	if _, _, _, err := repository.ApproveAdminOperation(ctx, selfApproval); !errors.Is(err, types.ErrPermissionDenied) {
		t.Fatalf("expected separation of duty denial, got %v", err)
	}

	preparedApproval := prepareApproval(t, operation.OperationID, "admin-approve-idem-1", "admappr_test_1", "admin:approver", types.DecisionApprove)
	approved, approval, replayed, err := repository.ApproveAdminOperation(ctx, preparedApproval)
	if err != nil {
		t.Fatalf("approve operation: %v", err)
	}
	if replayed || approved.Status != types.OperationStatusApproved || approval.Decision != types.DecisionApprove {
		t.Fatalf("unexpected approval result: replayed=%v operation=%+v approval=%+v", replayed, approved, approval)
	}
	approvedReplay, approvalReplay, replayed, err := repository.ApproveAdminOperation(ctx, preparedApproval)
	if err != nil {
		t.Fatalf("replay approval: %v", err)
	}
	if !replayed || approvedReplay.OperationID != operation.OperationID || approvalReplay.ApprovalID != approval.ApprovalID {
		t.Fatalf("unexpected approval replay: replayed=%v operation=%+v approval=%+v", replayed, approvedReplay, approvalReplay)
	}

	loaded, approvals, err := repository.GetAdminOperation(ctx, types.GetAdminOperationCommand{
		AuthContext: types.AuthContext{TenantID: "tenant-admin-test"},
		OperationID: operation.OperationID,
	})
	if err != nil {
		t.Fatalf("get operation: %v", err)
	}
	if loaded.Status != types.OperationStatusApproved || len(approvals) != 1 {
		t.Fatalf("unexpected loaded operation: %+v approvals=%+v", loaded, approvals)
	}
	listed, err := repository.ListAdminOperations(ctx, types.ListAdminOperationsCommand{
		AuthContext: types.AuthContext{TenantID: "tenant-admin-test"},
		Status:      types.OperationStatusApproved,
		PageSize:    10,
	})
	if err != nil {
		t.Fatalf("list operations: %v", err)
	}
	if len(listed) != 1 || listed[0].OperationID != operation.OperationID {
		t.Fatalf("unexpected list result: %+v", listed)
	}
	assertAdminOutboxLowSensitive(t, ctx, pool)
}

func TestRepositoryAdminOperationExecutionIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openAdminTestPool(t)
	resetAdminTables(t, ctx, pool)
	repository := NewRepository(pool)

	prepared := prepareAdminOperation(t, "admin-exec-create-idem-1", "admop_exec_test_1", types.RiskLevelMedium)
	operation, _, err := repository.CreateAdminOperation(ctx, prepared)
	if err != nil {
		t.Fatalf("create admin operation: %v", err)
	}
	approval := prepareApproval(t, operation.OperationID, "admin-exec-approve-idem-1", "admappr_exec_test_1", "admin:executor-approver", types.DecisionApprove)
	if _, _, _, err := repository.ApproveAdminOperation(ctx, approval); err != nil {
		t.Fatalf("approve admin operation: %v", err)
	}

	claimed, err := repository.ClaimApprovedOperations(ctx, 10, time.Minute)
	if err != nil {
		t.Fatalf("claim approved operations: %v", err)
	}
	if len(claimed) != 1 || claimed[0].Status != types.OperationStatusExecuting {
		t.Fatalf("unexpected claimed operations: %+v", claimed)
	}
	completed, err := repository.CompleteAdminOperation(ctx, claimed[0], types.OperationExecutionResult{
		DownstreamService:    "local-noop",
		DownstreamRequestRef: "operation:" + claimed[0].OperationID,
		Status:               types.OperationStatusSucceeded,
	}, "admres_exec_test_1")
	if err != nil {
		t.Fatalf("complete admin operation: %v", err)
	}
	if completed.Status != types.OperationStatusSucceeded {
		t.Fatalf("unexpected completed operation: %+v", completed)
	}

	var resultStatus string
	var downstreamService string
	if err := pool.QueryRow(ctx, `
SELECT status, downstream_service
FROM admin_operation_results
WHERE tenant_id = $1 AND result_id = $2
`, string(completed.TenantID), "admres_exec_test_1").Scan(&resultStatus, &downstreamService); err != nil {
		t.Fatalf("query admin result: %v", err)
	}
	if resultStatus != types.OperationStatusSucceeded || downstreamService != "local-noop" {
		t.Fatalf("unexpected result status=%s downstream=%s", resultStatus, downstreamService)
	}
	assertAdminOutboxEventLowSensitive(t, ctx, pool, types.AdminEventOperationExecuted, []string{
		`"result_id"`,
		`"downstream_service"`,
		`"downstream_request_ref"`,
	})
}

func TestRepositoryAdminCompensationRequestIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openAdminTestPool(t)
	resetAdminTables(t, ctx, pool)
	repository := NewRepository(pool)

	prepared := prepareAdminOperation(t, "admin-comp-create-idem-1", "admop_comp_test_1", types.RiskLevelHigh)
	operation, _, err := repository.CreateAdminOperation(ctx, prepared)
	if err != nil {
		t.Fatalf("create admin operation: %v", err)
	}
	approval := prepareApproval(t, operation.OperationID, "admin-comp-approve-idem-1", "admappr_comp_test_1", "admin:comp-approver", types.DecisionApprove)
	if _, _, _, err := repository.ApproveAdminOperation(ctx, approval); err != nil {
		t.Fatalf("approve admin operation: %v", err)
	}
	claimed, err := repository.ClaimApprovedOperations(ctx, 10, time.Minute)
	if err != nil {
		t.Fatalf("claim approved operations: %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("expected one claimed operation, got %+v", claimed)
	}
	failed, err := repository.CompleteAdminOperation(ctx, claimed[0], types.OperationExecutionResult{
		DownstreamService:    "control-plane-service",
		DownstreamRequestRef: "config:local:quota:v1",
		Status:               types.OperationStatusFailed,
		FailureClass:         "DOWNSTREAM_FAILED",
		PublicError:          "admin operation execution failed",
	}, "admres_comp_test_1")
	if err != nil {
		t.Fatalf("complete admin operation as failed: %v", err)
	}
	if failed.Status != types.OperationStatusFailed {
		t.Fatalf("expected failed operation, got %+v", failed)
	}

	dryRun, changed, err := repository.RequestAdminOperationCompensation(ctx, types.RequestAdminOperationCompensationCommand{
		TenantID:              failed.TenantID,
		OperationID:           failed.OperationID,
		RequestedBy:           "operator:compensator",
		CompensationReasonRef: "reason:compensation-1",
		DryRun:                true,
	})
	if err != nil {
		t.Fatalf("dry-run compensation request: %v", err)
	}
	if changed || dryRun.Status != types.OperationStatusFailed {
		t.Fatalf("dry-run should not change operation: changed=%v operation=%+v", changed, dryRun)
	}

	compensating, changed, err := repository.RequestAdminOperationCompensation(ctx, types.RequestAdminOperationCompensationCommand{
		TenantID:              failed.TenantID,
		OperationID:           failed.OperationID,
		RequestedBy:           "operator:compensator",
		CompensationReasonRef: "reason:compensation-1",
	})
	if err != nil {
		t.Fatalf("request compensation: %v", err)
	}
	if !changed || compensating.Status != types.OperationStatusCompensationRequested {
		t.Fatalf("unexpected compensation result changed=%v operation=%+v", changed, compensating)
	}
	replayed, changed, err := repository.RequestAdminOperationCompensation(ctx, types.RequestAdminOperationCompensationCommand{
		TenantID:    failed.TenantID,
		OperationID: failed.OperationID,
		RequestedBy: "operator:compensator",
	})
	if err != nil {
		t.Fatalf("replay compensation request: %v", err)
	}
	if changed || replayed.Status != types.OperationStatusCompensationRequested {
		t.Fatalf("unexpected replay result changed=%v operation=%+v", changed, replayed)
	}
	assertAdminOutboxEventLowSensitive(t, ctx, pool, types.AdminEventOperationCompensationRequested, []string{
		`"compensation_requested_by_hash"`,
		`"compensation_reason_ref"`,
	})
}

func TestRepositoryAdminCompensationRequiresFailedOperationIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openAdminTestPool(t)
	resetAdminTables(t, ctx, pool)
	repository := NewRepository(pool)

	prepared := prepareAdminOperation(t, "admin-comp-precond-create", "admop_comp_precond", types.RiskLevelMedium)
	operation, _, err := repository.CreateAdminOperation(ctx, prepared)
	if err != nil {
		t.Fatalf("create admin operation: %v", err)
	}
	_, _, err = repository.RequestAdminOperationCompensation(ctx, types.RequestAdminOperationCompensationCommand{
		TenantID:              operation.TenantID,
		OperationID:           operation.OperationID,
		RequestedBy:           "operator:compensator",
		CompensationReasonRef: "reason:compensation-1",
	})
	if !errors.Is(err, types.ErrFailedPrecondition) {
		t.Fatalf("expected failed precondition, got %v", err)
	}
}

func prepareAdminOperation(t *testing.T, idempotencyKey string, operationID string, risk string) domain.PreparedOperation {
	t.Helper()
	prepared, err := domain.PrepareCreate(types.CreateAdminOperationCommand{
		AuthContext:          types.AuthContext{TenantID: "tenant-admin-test", ServiceName: "admin-ui"},
		OperatorRef:          "admin:requester",
		OperatorRole:         "ADMIN",
		OperationType:        "USER_BAN",
		TargetRefHash:        "sha256:target-user",
		RiskLevel:            risk,
		PayloadSchemaVersion: "admin.user_ban.v1",
		OperationPayloadJSON: `{"target_user_ref":"user:hashed","repair_mode":"manual-review"}`,
		ReasonRef:            "reason:ticket-1",
		EvidenceRefs:         []string{"evidence:ticket-1"},
		IdempotencyKey:       idempotencyKey,
		CorrelationID:        "corr-admin-test",
	}, operationID, time.Now().UTC())
	if err != nil {
		t.Fatalf("prepare operation: %v", err)
	}
	return prepared
}

func prepareApproval(t *testing.T, operationID string, idempotencyKey string, approvalID string, approver string, decision string) domain.PreparedApproval {
	t.Helper()
	prepared, err := domain.PrepareApproval(types.ApproveAdminOperationCommand{
		AuthContext:       types.AuthContext{TenantID: "tenant-admin-test", ServiceName: "admin-ui"},
		OperationID:       operationID,
		ApproverRef:       approver,
		ApproverRole:      "ADMIN",
		Decision:          decision,
		ApprovalPolicyRef: "policy:admin-two-person",
		ReasonRef:         "reason:approval-1",
		EvidenceRefs:      []string{"evidence:approval-1"},
		IdempotencyKey:    idempotencyKey,
		CorrelationID:     "corr-admin-approval-test",
	}, approvalID, time.Now().UTC())
	if err != nil {
		t.Fatalf("prepare approval: %v", err)
	}
	return prepared
}

func openAdminTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("NEXUSIM_PG_DSN")
	if dsn == "" {
		t.Skip("NEXUSIM_PG_DSN is required for admin postgres integration tests")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("open pg pool: %v", err)
	}
	t.Cleanup(pool.Close)
	applyAdminMigration(t, context.Background(), pool)
	return pool
}

func applyAdminMigration(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	for _, name := range []string{
		"000001_admin_core.sql",
		"000002_admin_outbox_last_error.sql",
	} {
		path := filepath.Join("..", "..", "..", "..", "..", "migrations", "postgres", "admin", name)
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read admin migration %s: %v", name, err)
		}
		if _, err := pool.Exec(ctx, string(content)); err != nil {
			t.Fatalf("apply admin migration %s: %v", name, err)
		}
	}
}

func resetAdminTables(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(ctx, `
TRUNCATE
    admin_outbox,
    admin_operation_results,
    admin_approvals,
    admin_operations
CASCADE
`)
	if err != nil {
		t.Fatalf("reset admin tables: %v", err)
	}
}

func assertAdminOutboxLowSensitive(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	rows, err := pool.Query(ctx, `SELECT aggregate_id, partition_key, payload_json::text FROM admin_outbox WHERE tenant_id = 'tenant-admin-test' ORDER BY event_type`)
	if err != nil {
		t.Fatalf("query admin outbox: %v", err)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		count++
		var aggregateID string
		var partitionKey string
		var payload string
		if err := rows.Scan(&aggregateID, &partitionKey, &payload); err != nil {
			t.Fatalf("scan admin outbox: %v", err)
		}
		for _, forbidden := range []string{
			`"target_user_ref"`,
			`"operation_payload_json"`,
			"password",
			"token",
			"secret",
			"provider body",
			"raw prompt",
			"message body",
			"admin:requester",
			"admin:approver",
		} {
			if strings.Contains(payload, forbidden) || strings.Contains(aggregateID, forbidden) || strings.Contains(partitionKey, forbidden) {
				t.Fatalf("admin outbox leaked forbidden value %q: aggregate=%s partition=%s payload=%s", forbidden, aggregateID, partitionKey, payload)
			}
		}
		if !strings.Contains(payload, "sha256:") {
			t.Fatalf("admin outbox payload missing hash refs: %s", payload)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("admin outbox rows: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected two admin outbox rows, got %d", count)
	}
}

func assertAdminOutboxEventLowSensitive(t *testing.T, ctx context.Context, pool *pgxpool.Pool, eventType string, requiredFields []string) {
	t.Helper()
	var payload string
	if err := pool.QueryRow(ctx, `
SELECT payload_json::text
FROM admin_outbox
WHERE tenant_id = 'tenant-admin-test' AND event_type = $1
`, eventType).Scan(&payload); err != nil {
		t.Fatalf("query admin outbox event %s: %v", eventType, err)
	}
	for _, field := range requiredFields {
		if !strings.Contains(payload, field) {
			t.Fatalf("admin outbox event %s missing %s: %s", eventType, field, payload)
		}
	}
	for _, forbidden := range []string{
		`"target_user_ref"`,
		`"operation_payload_json"`,
		"password",
		"token",
		"secret",
		"provider body",
		"raw prompt",
		"message body",
		"admin:requester",
		"admin:executor-approver",
	} {
		if strings.Contains(payload, forbidden) {
			t.Fatalf("admin outbox event %s leaked forbidden value %q: %s", eventType, forbidden, payload)
		}
	}
}
