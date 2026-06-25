package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/qsyy0921/IM/services/workflow-service/internal/types"
)

const (
	defaultCompensationRequestLimit = 50
	defaultCompensationStaleAfter   = 5 * time.Minute
)

func (repository *Repository) RequestApprovedCompensations(
	ctx context.Context,
	limit int,
) ([]types.WorkflowCompensation, error) {
	if repository.pool == nil {
		return nil, types.NewDBWriteFailed("workflow repository is not configured")
	}
	if limit <= 0 {
		limit = defaultCompensationRequestLimit
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return nil, types.NewDBWriteFailed(err.Error())
	}
	defer func() { _ = tx.Rollback(ctx) }()

	workflows, err := listApprovedCompensationWorkflowsForUpdate(ctx, tx, limit)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	compensations := make([]types.WorkflowCompensation, 0, len(workflows))
	for _, workflow := range workflows {
		compensation := types.WorkflowCompensation{
			TenantID:              workflow.TenantID,
			WorkflowID:            workflow.WorkflowID,
			CompensationID:        "wfc_" + workflow.WorkflowID,
			SourceStepID:          workflow.CurrentStepID,
			TargetService:         workflow.TargetService,
			TargetOperation:       workflow.TargetOperation,
			TargetRefHash:         workflow.TargetRefHash,
			PayloadSchemaVersion:  workflow.PayloadSchemaVersion,
			PayloadRefHash:        workflow.PayloadRefHash,
			CompensationPolicyRef: workflow.CompensationPolicyRef,
			ReasonRef:             workflow.ReasonRef,
			Status:                types.WorkflowCompensationStatusRequested,
			CreatedAt:             now,
			UpdatedAt:             now,
		}
		if err := insertWorkflowCompensation(ctx, tx, compensation); err != nil {
			return nil, err
		}
		updated, err := updateWorkflowCompensationPending(ctx, tx, workflow, now)
		if err != nil {
			return nil, err
		}
		if err := insertCompensationRequestedOutbox(ctx, tx, updated, compensation); err != nil {
			return nil, err
		}
		compensations = append(compensations, compensation)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, types.NewDBWriteFailed(err.Error())
	}
	return compensations, nil
}

func (repository *Repository) ClaimRequestedCompensations(
	ctx context.Context,
	limit int,
	staleAfter time.Duration,
) ([]types.WorkflowCompensation, error) {
	if repository.pool == nil {
		return nil, types.NewDBWriteFailed("workflow repository is not configured")
	}
	if limit <= 0 {
		limit = defaultCompensationRequestLimit
	}
	if staleAfter <= 0 {
		staleAfter = defaultCompensationStaleAfter
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return nil, types.NewDBWriteFailed(err.Error())
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx, `
WITH candidates AS (
    SELECT tenant_id, workflow_id, compensation_id
    FROM workflow_compensations
    WHERE status = $2
       OR (status = $3 AND updated_at <= now() - ($4::double precision * INTERVAL '1 second'))
    ORDER BY created_at, compensation_id
    LIMIT $1
    FOR UPDATE SKIP LOCKED
)
UPDATE workflow_compensations AS compensation
SET status = $3,
    updated_at = now()
FROM candidates
WHERE compensation.tenant_id = candidates.tenant_id
  AND compensation.workflow_id = candidates.workflow_id
  AND compensation.compensation_id = candidates.compensation_id
RETURNING `+selectCompensationColumns("compensation"),
		limit,
		types.WorkflowCompensationStatusRequested,
		types.WorkflowCompensationStatusExecuting,
		staleAfter.Seconds())
	if err != nil {
		return nil, types.NewDBWriteFailed(err.Error())
	}
	defer rows.Close()

	compensations := make([]types.WorkflowCompensation, 0, limit)
	for rows.Next() {
		compensation, err := scanWorkflowCompensation(rows)
		if err != nil {
			return nil, types.NewDBReadFailed(err.Error())
		}
		compensations = append(compensations, compensation)
	}
	if err := rows.Err(); err != nil {
		return nil, types.NewDBReadFailed(err.Error())
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, types.NewDBWriteFailed(err.Error())
	}
	return compensations, nil
}

func (repository *Repository) CompleteWorkflowCompensation(
	ctx context.Context,
	compensation types.WorkflowCompensation,
	result types.WorkflowCompensationExecutionResult,
) (types.WorkflowCompensation, error) {
	if repository.pool == nil {
		return types.WorkflowCompensation{}, types.NewDBWriteFailed("workflow repository is not configured")
	}
	if result.Status != types.WorkflowCompensationStatusSucceeded &&
		result.Status != types.WorkflowCompensationStatusFailed {
		return types.WorkflowCompensation{}, types.NewInvalidArgument("workflow compensation result status is unsupported")
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return types.WorkflowCompensation{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() { _ = tx.Rollback(ctx) }()

	locked, err := getCompensationForUpdate(ctx, tx, compensation.TenantID, compensation.WorkflowID, compensation.CompensationID)
	if err != nil {
		return types.WorkflowCompensation{}, err
	}
	if locked.Status == types.WorkflowCompensationStatusSucceeded ||
		locked.Status == types.WorkflowCompensationStatusFailed {
		if err := tx.Commit(ctx); err != nil {
			return types.WorkflowCompensation{}, types.NewDBWriteFailed(err.Error())
		}
		return locked, nil
	}
	if locked.Status != types.WorkflowCompensationStatusExecuting {
		return types.WorkflowCompensation{}, types.NewFailedPrecondition("workflow compensation is not executing")
	}
	workflow, err := getWorkflowForUpdate(ctx, tx, locked.TenantID, locked.WorkflowID)
	if err != nil {
		return types.WorkflowCompensation{}, err
	}
	now := time.Now().UTC()
	completed, err := updateWorkflowCompensationResult(ctx, tx, locked, result, now)
	if err != nil {
		return types.WorkflowCompensation{}, err
	}
	eventType := types.WorkflowEventCompensationFailed
	if result.Status == types.WorkflowCompensationStatusSucceeded {
		workflow, err = updateWorkflowCompensated(ctx, tx, workflow, now)
		if err != nil {
			return types.WorkflowCompensation{}, err
		}
		eventType = types.WorkflowEventCompensationSucceeded
	}
	if err := insertCompensationResultOutbox(ctx, tx, workflow, completed, eventType); err != nil {
		return types.WorkflowCompensation{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return types.WorkflowCompensation{}, types.NewDBWriteFailed(err.Error())
	}
	return completed, nil
}

func (repository *Repository) ListWorkflowCompensations(
	ctx context.Context,
	command types.ListWorkflowCompensationsCommand,
) ([]types.WorkflowCompensation, error) {
	if repository.pool == nil {
		return nil, types.NewDBReadFailed("workflow repository is not configured")
	}
	normalized := command.Normalized()
	if err := normalized.Validate(); err != nil {
		return nil, err
	}
	rows, err := repository.pool.Query(ctx, `
SELECT `+selectCompensationColumns("")+`
FROM workflow_compensations
WHERE tenant_id = $1
  AND workflow_id = $2
  AND ($3 = '' OR status = $3)
ORDER BY updated_at DESC, compensation_id DESC
LIMIT $4
`, string(normalized.AuthContext.TenantID), normalized.WorkflowID, normalized.Status, normalized.PageSize)
	if err != nil {
		return nil, types.NewDBReadFailed(err.Error())
	}
	defer rows.Close()

	compensations := make([]types.WorkflowCompensation, 0, normalized.PageSize)
	for rows.Next() {
		compensation, err := scanWorkflowCompensation(rows)
		if err != nil {
			return nil, types.NewDBReadFailed(err.Error())
		}
		compensations = append(compensations, compensation)
	}
	if err := rows.Err(); err != nil {
		return nil, types.NewDBReadFailed(err.Error())
	}
	return compensations, nil
}

func listApprovedCompensationWorkflowsForUpdate(ctx context.Context, tx pgx.Tx, limit int) ([]types.Workflow, error) {
	rows, err := tx.Query(ctx, selectWorkflowSQL()+`
WHERE workflow_type = $1 AND status = $2
ORDER BY created_at, workflow_id
FOR UPDATE SKIP LOCKED
LIMIT $3
`, types.WorkflowTypeCompensationRequest, types.WorkflowStatusApproved, limit)
	if err != nil {
		return nil, types.NewDBReadFailed(err.Error())
	}
	defer rows.Close()

	workflows := []types.Workflow{}
	for rows.Next() {
		workflow, err := scanWorkflow(rows)
		if err != nil {
			return nil, types.NewDBReadFailed(err.Error())
		}
		workflows = append(workflows, workflow)
	}
	if err := rows.Err(); err != nil {
		return nil, types.NewDBReadFailed(err.Error())
	}
	return workflows, nil
}

func insertWorkflowCompensation(ctx context.Context, tx pgx.Tx, compensation types.WorkflowCompensation) error {
	var completedAt any
	if !compensation.CompletedAt.IsZero() {
		completedAt = compensation.CompletedAt
	}
	_, err := tx.Exec(ctx, `
INSERT INTO workflow_compensations (
    tenant_id, workflow_id, compensation_id, source_step_id, target_service,
    target_operation, target_ref_hash, payload_schema_version, payload_ref_hash,
    compensation_policy_ref, reason_ref, downstream_service, downstream_request_ref,
    status, failure_class, public_error, created_at, updated_at, completed_at
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8, $9,
    $10, $11, $12, $13,
    $14, $15, $16, $17, $18, $19
)
ON CONFLICT (tenant_id, workflow_id, compensation_id) DO NOTHING
`, string(compensation.TenantID), compensation.WorkflowID, compensation.CompensationID,
		compensation.SourceStepID, compensation.TargetService, compensation.TargetOperation,
		compensation.TargetRefHash, compensation.PayloadSchemaVersion, compensation.PayloadRefHash,
		compensation.CompensationPolicyRef, compensation.ReasonRef, compensation.DownstreamService,
		compensation.DownstreamRequestRef, compensation.Status, compensation.FailureClass,
		compensation.PublicError, compensation.CreatedAt, compensation.UpdatedAt, completedAt)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func updateWorkflowCompensationPending(ctx context.Context, tx pgx.Tx, workflow types.Workflow, now time.Time) (types.Workflow, error) {
	tag, err := tx.Exec(ctx, `
UPDATE workflow_requests
SET status = $3, updated_at = $4, completed_at = NULL
WHERE tenant_id = $1 AND workflow_id = $2 AND status = $5
`, string(workflow.TenantID), workflow.WorkflowID, types.WorkflowStatusCompensationPending, now, types.WorkflowStatusApproved)
	if err != nil {
		return types.Workflow{}, types.NewDBWriteFailed(err.Error())
	}
	if tag.RowsAffected() != 1 {
		return types.Workflow{}, types.NewFailedPrecondition("workflow compensation state changed")
	}
	workflow.Status = types.WorkflowStatusCompensationPending
	workflow.UpdatedAt = now
	workflow.CompletedAt = time.Time{}
	return workflow, nil
}

func insertCompensationRequestedOutbox(
	ctx context.Context,
	tx pgx.Tx,
	workflow types.Workflow,
	compensation types.WorkflowCompensation,
) error {
	payload := workflowPayload(workflow)
	payload["compensation_id"] = compensation.CompensationID
	payload["compensation_status"] = compensation.Status
	payload["source_step_id"] = compensation.SourceStepID
	payload["compensation_policy_ref"] = compensation.CompensationPolicyRef
	return insertOutbox(ctx, tx, "evt_"+compensation.CompensationID+"_requested", workflow, types.WorkflowEventCompensationRequested, payload)
}

func getCompensationForUpdate(
	ctx context.Context,
	tx pgx.Tx,
	tenantID types.TenantID,
	workflowID string,
	compensationID string,
) (types.WorkflowCompensation, error) {
	row := tx.QueryRow(ctx, `
SELECT `+selectCompensationColumns("")+`
FROM workflow_compensations
WHERE tenant_id = $1 AND workflow_id = $2 AND compensation_id = $3
FOR UPDATE
`, string(tenantID), workflowID, compensationID)
	compensation, err := scanWorkflowCompensation(row)
	if err != nil {
		return types.WorkflowCompensation{}, types.NewNotFound("workflow compensation not found")
	}
	return compensation, nil
}

func updateWorkflowCompensationResult(
	ctx context.Context,
	tx pgx.Tx,
	compensation types.WorkflowCompensation,
	result types.WorkflowCompensationExecutionResult,
	now time.Time,
) (types.WorkflowCompensation, error) {
	tag, err := tx.Exec(ctx, `
UPDATE workflow_compensations
SET status = $4,
    downstream_service = $5,
    downstream_request_ref = $6,
    failure_class = $7,
    public_error = $8,
    completed_at = $9,
    updated_at = $9
WHERE tenant_id = $1 AND workflow_id = $2 AND compensation_id = $3
`, string(compensation.TenantID), compensation.WorkflowID, compensation.CompensationID,
		result.Status, result.DownstreamService, result.DownstreamRequestRef,
		result.FailureClass, result.PublicError, now)
	if err != nil {
		return types.WorkflowCompensation{}, types.NewDBWriteFailed(err.Error())
	}
	if tag.RowsAffected() != 1 {
		return types.WorkflowCompensation{}, types.NewNotFound("workflow compensation not found")
	}
	compensation.Status = result.Status
	compensation.DownstreamService = result.DownstreamService
	compensation.DownstreamRequestRef = result.DownstreamRequestRef
	compensation.FailureClass = result.FailureClass
	compensation.PublicError = result.PublicError
	compensation.CompletedAt = now
	compensation.UpdatedAt = now
	return compensation, nil
}

func updateWorkflowCompensated(ctx context.Context, tx pgx.Tx, workflow types.Workflow, now time.Time) (types.Workflow, error) {
	tag, err := tx.Exec(ctx, `
UPDATE workflow_requests
SET status = $3, updated_at = $4, completed_at = $4
WHERE tenant_id = $1 AND workflow_id = $2 AND status = $5
`, string(workflow.TenantID), workflow.WorkflowID, types.WorkflowStatusCompensated, now, types.WorkflowStatusCompensationPending)
	if err != nil {
		return types.Workflow{}, types.NewDBWriteFailed(err.Error())
	}
	if tag.RowsAffected() != 1 {
		return types.Workflow{}, types.NewFailedPrecondition("workflow compensation state changed")
	}
	workflow.Status = types.WorkflowStatusCompensated
	workflow.UpdatedAt = now
	workflow.CompletedAt = now
	return workflow, nil
}

func insertCompensationResultOutbox(
	ctx context.Context,
	tx pgx.Tx,
	workflow types.Workflow,
	compensation types.WorkflowCompensation,
	eventType string,
) error {
	payload := workflowPayload(workflow)
	payload["compensation_id"] = compensation.CompensationID
	payload["compensation_status"] = compensation.Status
	payload["source_step_id"] = compensation.SourceStepID
	payload["downstream_service"] = compensation.DownstreamService
	payload["downstream_request_ref"] = compensation.DownstreamRequestRef
	if compensation.FailureClass != "" {
		payload["failure_class"] = compensation.FailureClass
	}
	if compensation.PublicError != "" {
		payload["public_error"] = compensation.PublicError
	}
	suffix := "_failed"
	if eventType == types.WorkflowEventCompensationSucceeded {
		suffix = "_succeeded"
	}
	return insertOutbox(ctx, tx, "evt_"+compensation.CompensationID+suffix, workflow, eventType, payload)
}

func scanWorkflowCompensation(row scanner) (types.WorkflowCompensation, error) {
	var compensation types.WorkflowCompensation
	var completedAt *time.Time
	err := row.Scan(
		&compensation.TenantID,
		&compensation.WorkflowID,
		&compensation.CompensationID,
		&compensation.SourceStepID,
		&compensation.TargetService,
		&compensation.TargetOperation,
		&compensation.TargetRefHash,
		&compensation.PayloadSchemaVersion,
		&compensation.PayloadRefHash,
		&compensation.CompensationPolicyRef,
		&compensation.ReasonRef,
		&compensation.DownstreamService,
		&compensation.DownstreamRequestRef,
		&compensation.Status,
		&compensation.FailureClass,
		&compensation.PublicError,
		&compensation.CreatedAt,
		&compensation.UpdatedAt,
		&completedAt,
	)
	if err != nil {
		return types.WorkflowCompensation{}, err
	}
	if completedAt != nil {
		compensation.CompletedAt = *completedAt
	}
	return compensation, nil
}

func selectCompensationColumns(alias string) string {
	prefix := ""
	if alias != "" {
		prefix = alias + "."
	}
	return prefix + `tenant_id, ` +
		prefix + `workflow_id, ` +
		prefix + `compensation_id, ` +
		prefix + `source_step_id, ` +
		prefix + `target_service, ` +
		prefix + `target_operation, ` +
		prefix + `target_ref_hash, ` +
		prefix + `payload_schema_version, ` +
		prefix + `payload_ref_hash, ` +
		prefix + `compensation_policy_ref, ` +
		prefix + `reason_ref, ` +
		prefix + `downstream_service, ` +
		prefix + `downstream_request_ref, ` +
		prefix + `status, ` +
		prefix + `failure_class, ` +
		prefix + `public_error, ` +
		prefix + `created_at, ` +
		prefix + `updated_at, ` +
		prefix + `completed_at`
}
