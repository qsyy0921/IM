package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/qsyy0921/IM/services/admin-service/internal/domain"
	"github.com/qsyy0921/IM/services/admin-service/internal/types"
)

func (repository *Repository) ClaimApprovedOperations(ctx context.Context, limit int, staleAfter time.Duration) ([]types.AdminOperation, error) {
	if repository.pool == nil {
		return nil, types.NewDBWriteFailed("admin repository is not configured")
	}
	if limit <= 0 {
		limit = 50
	}
	if staleAfter <= 0 {
		staleAfter = 5 * time.Minute
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return nil, types.NewDBWriteFailed(err.Error())
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx, `
WITH candidates AS (
    SELECT tenant_id, operation_id
    FROM admin_operations
    WHERE status = $2
       OR (status = $3 AND updated_at <= now() - ($4::double precision * INTERVAL '1 second'))
    ORDER BY COALESCE(approved_at, updated_at), operation_id
    LIMIT $1
    FOR UPDATE SKIP LOCKED
)
UPDATE admin_operations AS operation
SET status = $3,
    updated_at = now()
FROM candidates
WHERE operation.tenant_id = candidates.tenant_id
  AND operation.operation_id = candidates.operation_id
RETURNING operation.tenant_id, operation.operation_id, operation.idempotency_key, operation.command_hash, operation.operation_type,
       operation.target_ref_hash, operation.risk_level, operation.payload_schema_version, operation.payload_json::text, operation.payload_hash,
       operation.reason_ref, operation.evidence_refs_json::text, operation.status, operation.requested_by, operation.requested_at,
       operation.approved_by, operation.approved_at, operation.correlation_id, operation.causation_id, operation.trace_id, operation.updated_at
`, limit, types.OperationStatusApproved, types.OperationStatusExecuting, staleAfter.Seconds())
	if err != nil {
		return nil, types.NewDBWriteFailed(err.Error())
	}
	defer rows.Close()

	operations := make([]types.AdminOperation, 0, limit)
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
	if err := tx.Commit(ctx); err != nil {
		return nil, types.NewDBWriteFailed(err.Error())
	}
	return operations, nil
}

func (repository *Repository) CompleteAdminOperation(
	ctx context.Context,
	operation types.AdminOperation,
	result types.OperationExecutionResult,
	resultID string,
) (types.AdminOperation, error) {
	if repository.pool == nil {
		return types.AdminOperation{}, types.NewDBWriteFailed("admin repository is not configured")
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return types.AdminOperation{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() { _ = tx.Rollback(ctx) }()

	locked, err := getOperationForUpdate(ctx, tx, operation.TenantID, operation.OperationID)
	if err != nil {
		return types.AdminOperation{}, err
	}
	if locked.Status == types.OperationStatusSucceeded || locked.Status == types.OperationStatusFailed {
		if err := tx.Commit(ctx); err != nil {
			return types.AdminOperation{}, types.NewDBWriteFailed(err.Error())
		}
		return locked, nil
	}
	prepared, err := domain.PrepareOperationResult(locked, result, resultID, time.Now().UTC())
	if err != nil {
		return types.AdminOperation{}, err
	}
	if err := insertOperationResult(ctx, tx, prepared); err != nil {
		return types.AdminOperation{}, err
	}
	completed, err := updateOperationAfterResult(ctx, tx, locked, prepared)
	if err != nil {
		return types.AdminOperation{}, err
	}
	eventType := types.AdminEventOperationExecuted
	if completed.Status == types.OperationStatusFailed {
		eventType = types.AdminEventOperationFailed
	}
	payload := adminOperationPayload(completed)
	payload["result_id"] = prepared.ResultID
	payload["downstream_service"] = prepared.DownstreamService
	payload["downstream_request_ref"] = prepared.DownstreamRequestRef
	if prepared.FailureClass != "" {
		payload["failure_class"] = prepared.FailureClass
	}
	if prepared.PublicError != "" {
		payload["public_error"] = prepared.PublicError
	}
	if err := insertOperationOutbox(ctx, tx, "evt_"+prepared.ResultID, completed, eventType, payload); err != nil {
		return types.AdminOperation{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return types.AdminOperation{}, types.NewDBWriteFailed(err.Error())
	}
	return completed, nil
}

func (repository *Repository) RequestAdminOperationCompensation(
	ctx context.Context,
	command types.RequestAdminOperationCompensationCommand,
) (types.AdminOperation, bool, error) {
	if repository.pool == nil {
		return types.AdminOperation{}, false, types.NewDBWriteFailed("admin repository is not configured")
	}
	if command.TenantID == "" ||
		command.OperationID == "" ||
		command.RequestedBy == "" {
		return types.AdminOperation{}, false, types.NewInvalidArgument("admin compensation request is incomplete")
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return types.AdminOperation{}, false, types.NewDBWriteFailed(err.Error())
	}
	defer func() { _ = tx.Rollback(ctx) }()

	locked, err := getOperationForUpdate(ctx, tx, command.TenantID, command.OperationID)
	if err != nil {
		return types.AdminOperation{}, false, err
	}
	if locked.Status == types.OperationStatusCompensationRequested {
		if err := tx.Commit(ctx); err != nil {
			return types.AdminOperation{}, false, types.NewDBWriteFailed(err.Error())
		}
		return locked, false, nil
	}
	if locked.Status != types.OperationStatusFailed {
		return types.AdminOperation{}, false, types.NewFailedPrecondition("admin operation is not failed")
	}
	if command.DryRun {
		if err := tx.Commit(ctx); err != nil {
			return types.AdminOperation{}, false, types.NewDBWriteFailed(err.Error())
		}
		return locked, false, nil
	}
	if command.CompensationReasonRef == "" {
		return types.AdminOperation{}, false, types.NewInvalidArgument("admin compensation request is incomplete")
	}

	now := time.Now().UTC()
	_, err = tx.Exec(ctx, `
UPDATE admin_operations
SET status = $3,
    updated_at = $4
WHERE tenant_id = $1 AND operation_id = $2
`, string(command.TenantID), command.OperationID, types.OperationStatusCompensationRequested, now)
	if err != nil {
		return types.AdminOperation{}, false, types.NewDBWriteFailed(err.Error())
	}
	locked.Status = types.OperationStatusCompensationRequested
	locked.UpdatedAt = now
	payload := adminOperationPayload(locked)
	payload["compensation_requested_by_hash"] = domain.HashText(command.RequestedBy)
	payload["compensation_reason_ref"] = command.CompensationReasonRef
	if err := insertOperationOutbox(ctx, tx, "evt_"+locked.OperationID+"_compensation_requested", locked, types.AdminEventOperationCompensationRequested, payload); err != nil {
		return types.AdminOperation{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return types.AdminOperation{}, false, types.NewDBWriteFailed(err.Error())
	}
	return locked, true, nil
}

func insertOperationResult(ctx context.Context, tx pgx.Tx, result types.AdminOperationResult) error {
	_, err := tx.Exec(ctx, `
INSERT INTO admin_operation_results (
    tenant_id, result_id, operation_id, downstream_service, downstream_request_ref,
    status, failure_class, public_error, created_at, completed_at
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8, $9, $10
)
`, string(result.TenantID), result.ResultID, result.OperationID, result.DownstreamService, result.DownstreamRequestRef,
		result.Status, result.FailureClass, result.PublicError, result.CreatedAt, nullableTime(result.CompletedAt))
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func updateOperationAfterResult(ctx context.Context, tx pgx.Tx, operation types.AdminOperation, result types.AdminOperationResult) (types.AdminOperation, error) {
	_, err := tx.Exec(ctx, `
UPDATE admin_operations
SET status = $3,
    updated_at = $4
WHERE tenant_id = $1 AND operation_id = $2
`, string(operation.TenantID), operation.OperationID, result.Status, result.CompletedAt)
	if err != nil {
		return types.AdminOperation{}, types.NewDBWriteFailed(err.Error())
	}
	operation.Status = result.Status
	operation.UpdatedAt = result.CompletedAt
	return operation, nil
}
