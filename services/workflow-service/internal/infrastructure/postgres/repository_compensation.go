package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/qsyy0921/IM/services/workflow-service/internal/types"
)

const defaultCompensationRequestLimit = 50

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
			TenantID:        workflow.TenantID,
			WorkflowID:      workflow.WorkflowID,
			CompensationID:  "wfc_" + workflow.WorkflowID,
			SourceStepID:    workflow.CurrentStepID,
			TargetService:   workflow.TargetService,
			TargetOperation: workflow.TargetOperation,
			TargetRefHash:   workflow.TargetRefHash,
			Status:          types.WorkflowCompensationStatusRequested,
			CreatedAt:       now,
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
    target_operation, target_ref_hash, status, failure_class, public_error,
    created_at, completed_at
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8, $9, $10,
    $11, $12
)
ON CONFLICT (tenant_id, workflow_id, compensation_id) DO NOTHING
`, string(compensation.TenantID), compensation.WorkflowID, compensation.CompensationID,
		compensation.SourceStepID, compensation.TargetService, compensation.TargetOperation,
		compensation.TargetRefHash, compensation.Status, compensation.FailureClass,
		compensation.PublicError, compensation.CreatedAt, completedAt)
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
	return insertOutbox(ctx, tx, "evt_"+compensation.CompensationID+"_requested", workflow, types.WorkflowEventCompensationRequested, payload)
}
