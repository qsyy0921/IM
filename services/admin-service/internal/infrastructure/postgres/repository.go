package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/admin-service/internal/domain"
	"github.com/qsyy0921/IM/services/admin-service/internal/types"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (repository *Repository) CreateAdminOperation(
	ctx context.Context,
	prepared domain.PreparedOperation,
) (types.AdminOperation, bool, error) {
	if repository.pool == nil {
		return types.AdminOperation{}, false, types.NewDBWriteFailed("admin repository is not configured")
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return types.AdminOperation{}, false, types.NewDBWriteFailed(err.Error())
	}
	defer func() { _ = tx.Rollback(ctx) }()

	existing, found, err := findOperationByIdempotency(ctx, tx, prepared.Command.AuthContext.TenantID, prepared.Command.OperatorRef, prepared.Command.IdempotencyKey)
	if err != nil {
		return types.AdminOperation{}, false, err
	}
	if found {
		if existing.CommandHash != prepared.CommandHash {
			return types.AdminOperation{}, false, types.NewAlreadyExists("admin operation idempotency conflict")
		}
		if err := tx.Commit(ctx); err != nil {
			return types.AdminOperation{}, false, types.NewDBWriteFailed(err.Error())
		}
		return existing, true, nil
	}

	operation := domain.OperationFromPrepared(prepared)
	if err := insertOperation(ctx, tx, operation); err != nil {
		return types.AdminOperation{}, false, err
	}
	if err := insertOperationOutbox(ctx, tx, "evt_"+operation.OperationID+"_submitted", operation, "admin.operation.submitted.v1", adminOperationPayload(operation)); err != nil {
		return types.AdminOperation{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return types.AdminOperation{}, false, types.NewDBWriteFailed(err.Error())
	}
	return operation, false, nil
}

func (repository *Repository) ApproveAdminOperation(
	ctx context.Context,
	prepared domain.PreparedApproval,
) (types.AdminOperation, types.AdminApproval, bool, error) {
	if repository.pool == nil {
		return types.AdminOperation{}, types.AdminApproval{}, false, types.NewDBWriteFailed("admin repository is not configured")
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return types.AdminOperation{}, types.AdminApproval{}, false, types.NewDBWriteFailed(err.Error())
	}
	defer func() { _ = tx.Rollback(ctx) }()

	existingApproval, found, err := findApprovalByIdempotency(
		ctx,
		tx,
		prepared.Command.AuthContext.TenantID,
		prepared.Command.OperationID,
		prepared.Command.ApproverRef,
		prepared.Command.IdempotencyKey,
	)
	if err != nil {
		return types.AdminOperation{}, types.AdminApproval{}, false, err
	}
	if found {
		if existingApproval.CommandHash != prepared.CommandHash {
			return types.AdminOperation{}, types.AdminApproval{}, false, types.NewAlreadyExists("admin approval idempotency conflict")
		}
		operation, err := getOperationForUpdate(ctx, tx, prepared.Command.AuthContext.TenantID, prepared.Command.OperationID)
		if err != nil {
			return types.AdminOperation{}, types.AdminApproval{}, false, err
		}
		if err := tx.Commit(ctx); err != nil {
			return types.AdminOperation{}, types.AdminApproval{}, false, types.NewDBWriteFailed(err.Error())
		}
		return operation, existingApproval, true, nil
	}

	operation, err := getOperationForUpdate(ctx, tx, prepared.Command.AuthContext.TenantID, prepared.Command.OperationID)
	if err != nil {
		return types.AdminOperation{}, types.AdminApproval{}, false, err
	}
	approval := domain.ApprovalFromPrepared(prepared, operation.TenantID)
	if err := domain.ValidateApprovalTransition(operation, approval); err != nil {
		return types.AdminOperation{}, types.AdminApproval{}, false, err
	}
	if err := insertApproval(ctx, tx, approval); err != nil {
		return types.AdminOperation{}, types.AdminApproval{}, false, err
	}
	updated, err := updateOperationAfterApproval(ctx, tx, operation, approval)
	if err != nil {
		return types.AdminOperation{}, types.AdminApproval{}, false, err
	}
	eventType := "admin.operation.approved.v1"
	if approval.Decision == types.DecisionReject {
		eventType = "admin.operation.rejected.v1"
	}
	payload := adminOperationPayload(updated)
	payload["approval_id"] = approval.ApprovalID
	payload["decision"] = approval.Decision
	payload["approver_ref_hash"] = domain.HashText(approval.ApproverRef)
	if err := insertOperationOutbox(ctx, tx, "evt_"+approval.ApprovalID, updated, eventType, payload); err != nil {
		return types.AdminOperation{}, types.AdminApproval{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return types.AdminOperation{}, types.AdminApproval{}, false, types.NewDBWriteFailed(err.Error())
	}
	return updated, approval, false, nil
}

func (repository *Repository) GetAdminOperation(
	ctx context.Context,
	command types.GetAdminOperationCommand,
) (types.AdminOperation, []types.AdminApproval, error) {
	if repository.pool == nil {
		return types.AdminOperation{}, nil, types.NewDBReadFailed("admin repository is not configured")
	}
	row := repository.pool.QueryRow(ctx, selectOperationSQL()+`
WHERE tenant_id = $1 AND operation_id = $2
`, string(command.AuthContext.TenantID), command.OperationID)
	operation, err := scanOperation(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return types.AdminOperation{}, nil, types.NewNotFound("admin operation not found")
		}
		return types.AdminOperation{}, nil, types.NewDBReadFailed(err.Error())
	}
	approvals, err := listApprovals(ctx, repository.pool, operation.TenantID, operation.OperationID)
	if err != nil {
		return types.AdminOperation{}, nil, err
	}
	return operation, approvals, nil
}

func (repository *Repository) ListAdminOperations(
	ctx context.Context,
	command types.ListAdminOperationsCommand,
) ([]types.AdminOperation, error) {
	if repository.pool == nil {
		return nil, types.NewDBReadFailed("admin repository is not configured")
	}
	if command.PageSize <= 0 {
		command.PageSize = 50
	}
	if command.PageSize > 100 {
		command.PageSize = 100
	}
	query := selectOperationSQL() + `
WHERE tenant_id = $1
  AND ($2 = '' OR status = $2)
  AND ($3 = '' OR operation_type = $3)
ORDER BY updated_at DESC, operation_id
LIMIT $4
`
	rows, err := repository.pool.Query(ctx, query, string(command.AuthContext.TenantID), command.Status, command.OperationType, command.PageSize)
	if err != nil {
		return nil, types.NewDBReadFailed(err.Error())
	}
	defer rows.Close()

	operations := []types.AdminOperation{}
	for rows.Next() {
		operation, err := scanOperation(rows)
		if err != nil {
			return nil, types.NewDBReadFailed(err.Error())
		}
		operations = append(operations, operation)
	}
	if err := rows.Err(); err != nil {
		return nil, types.NewDBReadFailed(err.Error())
	}
	return operations, nil
}

func findOperationByIdempotency(ctx context.Context, tx pgx.Tx, tenantID types.TenantID, operatorRef string, key string) (types.AdminOperation, bool, error) {
	row := tx.QueryRow(ctx, selectOperationSQL()+`
WHERE tenant_id = $1 AND requested_by = $2 AND idempotency_key = $3
LIMIT 1
`, string(tenantID), operatorRef, key)
	operation, err := scanOperation(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return types.AdminOperation{}, false, nil
		}
		return types.AdminOperation{}, false, types.NewDBReadFailed(err.Error())
	}
	return operation, true, nil
}

func findApprovalByIdempotency(ctx context.Context, tx pgx.Tx, tenantID types.TenantID, operationID string, approverRef string, key string) (types.AdminApproval, bool, error) {
	row := tx.QueryRow(ctx, selectApprovalSQL()+`
WHERE tenant_id = $1 AND operation_id = $2 AND approver_ref = $3 AND idempotency_key = $4
LIMIT 1
`, string(tenantID), operationID, approverRef, key)
	approval, err := scanApproval(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return types.AdminApproval{}, false, nil
		}
		return types.AdminApproval{}, false, types.NewDBReadFailed(err.Error())
	}
	return approval, true, nil
}

func getOperationForUpdate(ctx context.Context, tx pgx.Tx, tenantID types.TenantID, operationID string) (types.AdminOperation, error) {
	row := tx.QueryRow(ctx, selectOperationSQL()+`
WHERE tenant_id = $1 AND operation_id = $2
FOR UPDATE
`, string(tenantID), operationID)
	operation, err := scanOperation(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return types.AdminOperation{}, types.NewNotFound("admin operation not found")
		}
		return types.AdminOperation{}, types.NewDBReadFailed(err.Error())
	}
	return operation, nil
}

func insertOperation(ctx context.Context, tx pgx.Tx, operation types.AdminOperation) error {
	evidenceJSON, err := json.Marshal(operation.EvidenceRefs)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	_, err = tx.Exec(ctx, `
INSERT INTO admin_operations (
    tenant_id, operation_id, idempotency_key, command_hash, operation_type,
    target_ref_hash, risk_level, payload_schema_version, payload_json, payload_hash,
    reason_ref, evidence_refs_json, status, requested_by, requested_at,
    approved_by, approved_at, correlation_id, causation_id, trace_id, updated_at
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8, $9::jsonb, $10,
    $11, $12::jsonb, $13, $14, $15,
    $16, $17, $18, $19, $20, $21
)
`, string(operation.TenantID), operation.OperationID, operation.IdempotencyKey, operation.CommandHash,
		operation.OperationType, operation.TargetRefHash, operation.RiskLevel, operation.PayloadSchemaVersion,
		operation.PayloadJSON, operation.PayloadHash, operation.ReasonRef, string(evidenceJSON), operation.Status,
		operation.RequestedBy, operation.RequestedAt, operation.ApprovedBy, nullableTime(operation.ApprovedAt),
		operation.CorrelationID, operation.CausationID, operation.TraceID, operation.UpdatedAt)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func insertApproval(ctx context.Context, tx pgx.Tx, approval types.AdminApproval) error {
	evidenceJSON, err := json.Marshal(approval.EvidenceRefs)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	_, err = tx.Exec(ctx, `
INSERT INTO admin_approvals (
    tenant_id, approval_id, operation_id, idempotency_key, command_hash,
    approver_ref, decision, approval_policy_ref, reason_ref, evidence_refs_json, created_at
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8, $9, $10::jsonb, $11
)
`, string(approval.TenantID), approval.ApprovalID, approval.OperationID, approval.IdempotencyKey,
		approval.CommandHash, approval.ApproverRef, approval.Decision, approval.ApprovalPolicyRef,
		approval.ReasonRef, string(evidenceJSON), approval.CreatedAt)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func updateOperationAfterApproval(ctx context.Context, tx pgx.Tx, operation types.AdminOperation, approval types.AdminApproval) (types.AdminOperation, error) {
	nextStatus := types.OperationStatusApproved
	if approval.Decision == types.DecisionReject {
		nextStatus = types.OperationStatusRejected
	}
	approvedBy := approval.ApproverRef
	approvedAt := approval.CreatedAt
	_, err := tx.Exec(ctx, `
UPDATE admin_operations
SET status = $3, approved_by = $4, approved_at = $5, updated_at = $5
WHERE tenant_id = $1 AND operation_id = $2
`, string(operation.TenantID), operation.OperationID, nextStatus, approvedBy, approvedAt)
	if err != nil {
		return types.AdminOperation{}, types.NewDBWriteFailed(err.Error())
	}
	operation.Status = nextStatus
	operation.ApprovedBy = approvedBy
	operation.ApprovedAt = approvedAt
	operation.UpdatedAt = approvedAt
	return operation, nil
}

func insertOperationOutbox(ctx context.Context, tx pgx.Tx, eventID string, operation types.AdminOperation, eventType string, payload map[string]any) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	_, err = tx.Exec(ctx, `
INSERT INTO admin_outbox (
    event_id, tenant_id, operation_id, aggregate_type, aggregate_id, event_type,
    event_version, partition_key, payload_json, status, available_at, created_at, updated_at
) VALUES (
    $1, $2, $3, 'admin_operation', $4, $5,
    1, $6, $7::jsonb, 'PENDING', now(), now(), now()
)
ON CONFLICT (event_id) DO NOTHING
`, eventID, string(operation.TenantID), operation.OperationID, "admin:"+operation.OperationID,
		eventType, string(operation.TenantID)+":"+operation.OperationID, string(encoded))
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func adminOperationPayload(operation types.AdminOperation) map[string]any {
	return map[string]any{
		"tenant_id":              string(operation.TenantID),
		"operation_id":           operation.OperationID,
		"operation_type":         operation.OperationType,
		"target_ref_hash":        operation.TargetRefHash,
		"risk_level":             operation.RiskLevel,
		"status":                 operation.Status,
		"requested_by_hash":      domain.HashText(operation.RequestedBy),
		"approved_by_hash":       domain.HashText(operation.ApprovedBy),
		"payload_schema_version": operation.PayloadSchemaVersion,
		"payload_hash":           operation.PayloadHash,
		"reason_ref":             operation.ReasonRef,
		"correlation_id":         operation.CorrelationID,
		"causation_id":           operation.CausationID,
		"trace_id":               operation.TraceID,
	}
}

func listApprovals(ctx context.Context, querier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, tenantID types.TenantID, operationID string) ([]types.AdminApproval, error) {
	rows, err := querier.Query(ctx, selectApprovalSQL()+`
WHERE tenant_id = $1 AND operation_id = $2
ORDER BY created_at, approval_id
`, string(tenantID), operationID)
	if err != nil {
		return nil, types.NewDBReadFailed(err.Error())
	}
	defer rows.Close()

	approvals := []types.AdminApproval{}
	for rows.Next() {
		approval, err := scanApproval(rows)
		if err != nil {
			return nil, types.NewDBReadFailed(err.Error())
		}
		approvals = append(approvals, approval)
	}
	if err := rows.Err(); err != nil {
		return nil, types.NewDBReadFailed(err.Error())
	}
	return approvals, nil
}

func selectOperationSQL() string {
	return `
SELECT tenant_id, operation_id, idempotency_key, command_hash, operation_type,
       target_ref_hash, risk_level, payload_schema_version, payload_json::text, payload_hash,
       reason_ref, evidence_refs_json::text, status, requested_by, requested_at,
       approved_by, approved_at, correlation_id, causation_id, trace_id, updated_at
FROM admin_operations
`
}

func selectApprovalSQL() string {
	return `
SELECT tenant_id, approval_id, operation_id, idempotency_key, command_hash,
       approver_ref, decision, approval_policy_ref, reason_ref, evidence_refs_json::text,
       created_at
FROM admin_approvals
`
}

type scanner interface {
	Scan(dest ...any) error
}

func scanOperation(row scanner) (types.AdminOperation, error) {
	var operation types.AdminOperation
	var evidenceJSON string
	var approvedAt *time.Time
	err := row.Scan(
		&operation.TenantID, &operation.OperationID, &operation.IdempotencyKey,
		&operation.CommandHash, &operation.OperationType, &operation.TargetRefHash,
		&operation.RiskLevel, &operation.PayloadSchemaVersion, &operation.PayloadJSON,
		&operation.PayloadHash, &operation.ReasonRef, &evidenceJSON, &operation.Status,
		&operation.RequestedBy, &operation.RequestedAt, &operation.ApprovedBy, &approvedAt,
		&operation.CorrelationID, &operation.CausationID, &operation.TraceID, &operation.UpdatedAt,
	)
	if err != nil {
		return types.AdminOperation{}, err
	}
	if err := json.Unmarshal([]byte(evidenceJSON), &operation.EvidenceRefs); err != nil {
		return types.AdminOperation{}, err
	}
	if approvedAt != nil {
		operation.ApprovedAt = *approvedAt
	}
	return operation, nil
}

func scanApproval(row scanner) (types.AdminApproval, error) {
	var approval types.AdminApproval
	var evidenceJSON string
	err := row.Scan(
		&approval.TenantID, &approval.ApprovalID, &approval.OperationID, &approval.IdempotencyKey,
		&approval.CommandHash, &approval.ApproverRef, &approval.Decision, &approval.ApprovalPolicyRef,
		&approval.ReasonRef, &evidenceJSON, &approval.CreatedAt,
	)
	if err != nil {
		return types.AdminApproval{}, err
	}
	if err := json.Unmarshal([]byte(evidenceJSON), &approval.EvidenceRefs); err != nil {
		return types.AdminApproval{}, err
	}
	return approval, nil
}

func nullableTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value
}
