package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/qsyy0921/IM/services/workflow-service/internal/types"
)

func (repository *Repository) UpsertWorkflowCompensationInstructions(
	ctx context.Context,
	instructions []types.WorkflowCompensationInstruction,
) (int, error) {
	if repository.pool == nil {
		return 0, types.NewDBWriteFailed("workflow repository is not configured")
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return 0, types.NewDBWriteFailed(err.Error())
	}
	defer func() { _ = tx.Rollback(ctx) }()

	upserted := 0
	for _, instruction := range instructions {
		normalized := instruction.Normalized()
		if err := normalized.Validate(); err != nil {
			return 0, err
		}
		if err := validateWorkflowCompensationInstructionBinding(ctx, tx, normalized); err != nil {
			return 0, err
		}
		if err := upsertWorkflowCompensationInstruction(ctx, tx, normalized); err != nil {
			return 0, err
		}
		upserted++
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, types.NewDBWriteFailed(err.Error())
	}
	return upserted, nil
}

func (repository *Repository) ResolveControlPlaneRollbackInstruction(
	ctx context.Context,
	compensation types.WorkflowCompensation,
) (types.WorkflowCompensationInstruction, bool, error) {
	if repository.pool == nil {
		return types.WorkflowCompensationInstruction{}, false, types.NewDBReadFailed("workflow repository is not configured")
	}
	row := repository.pool.QueryRow(ctx, `
SELECT tenant_id, instruction_id, workflow_id, payload_ref_hash, target_service, target_operation,
       instruction_type, environment, config_kind, bundle_key, target_version, operator_ref,
       reason_ref, status, created_at, updated_at
FROM workflow_compensation_instructions
WHERE tenant_id = $1
  AND payload_ref_hash = $2
  AND target_service = $3
  AND target_operation = $4
  AND workflow_id = $5
  AND status = $6
ORDER BY created_at DESC, instruction_id DESC
LIMIT 1
`, string(compensation.TenantID), compensation.PayloadRefHash, compensation.TargetService,
		compensation.TargetOperation, compensation.WorkflowID, types.WorkflowCompensationInstructionStatusActive)
	instruction, err := scanWorkflowCompensationInstruction(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return types.WorkflowCompensationInstruction{}, false, nil
	}
	if err != nil {
		return types.WorkflowCompensationInstruction{}, false, types.NewDBReadFailed(err.Error())
	}
	return instruction, true, nil
}

func (repository *Repository) ListWorkflowCompensationInstructions(
	ctx context.Context,
	command types.ListWorkflowCompensationInstructionsCommand,
) ([]types.WorkflowCompensationInstruction, error) {
	if repository.pool == nil {
		return nil, types.NewDBReadFailed("workflow repository is not configured")
	}
	normalized := command.Normalized()
	if err := normalized.Validate(); err != nil {
		return nil, err
	}
	rows, err := repository.pool.Query(ctx, `
SELECT tenant_id, instruction_id, workflow_id, payload_ref_hash, target_service, target_operation,
       instruction_type, environment, config_kind, bundle_key, target_version, operator_ref,
       reason_ref, status, created_at, updated_at
FROM workflow_compensation_instructions
WHERE tenant_id = $1
  AND workflow_id = $2
  AND ($3 = '' OR status = $3)
ORDER BY updated_at DESC, instruction_id DESC
LIMIT $4
`, string(normalized.AuthContext.TenantID), normalized.WorkflowID, normalized.Status, normalized.PageSize)
	if err != nil {
		return nil, types.NewDBReadFailed(err.Error())
	}
	defer rows.Close()

	instructions := make([]types.WorkflowCompensationInstruction, 0, normalized.PageSize)
	for rows.Next() {
		instruction, err := scanWorkflowCompensationInstruction(rows)
		if err != nil {
			return nil, types.NewDBReadFailed(err.Error())
		}
		instructions = append(instructions, instruction)
	}
	if err := rows.Err(); err != nil {
		return nil, types.NewDBReadFailed(err.Error())
	}
	return instructions, nil
}

func validateWorkflowCompensationInstructionBinding(
	ctx context.Context,
	tx pgx.Tx,
	instruction types.WorkflowCompensationInstruction,
) error {
	workflow, err := getWorkflowForUpdate(ctx, tx, instruction.TenantID, instruction.WorkflowID)
	if err != nil {
		return err
	}
	if workflow.WorkflowType != types.WorkflowTypeCompensationRequest {
		return types.NewInvalidArgument("workflow compensation instruction must bind a compensation workflow")
	}
	if workflow.Status != types.WorkflowStatusApproved &&
		workflow.Status != types.WorkflowStatusCompensationPending {
		return types.NewFailedPrecondition("workflow compensation instruction requires approved workflow")
	}
	if workflow.TargetService != instruction.TargetService ||
		workflow.TargetOperation != instruction.TargetOperation ||
		workflow.PayloadRefHash != instruction.PayloadRefHash {
		return types.NewInvalidArgument("workflow compensation instruction does not match workflow refs")
	}
	return nil
}

func upsertWorkflowCompensationInstruction(
	ctx context.Context,
	tx pgx.Tx,
	instruction types.WorkflowCompensationInstruction,
) error {
	_, err := tx.Exec(ctx, `
INSERT INTO workflow_compensation_instructions (
    tenant_id, instruction_id, workflow_id, payload_ref_hash, target_service, target_operation,
    instruction_type, environment, config_kind, bundle_key, target_version,
    operator_ref, reason_ref, status, created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6,
    $7, $8, $9, $10, $11,
    $12, $13, $14, now(), now()
)
ON CONFLICT (tenant_id, instruction_id) DO UPDATE SET
    workflow_id = EXCLUDED.workflow_id,
    payload_ref_hash = EXCLUDED.payload_ref_hash,
    target_service = EXCLUDED.target_service,
    target_operation = EXCLUDED.target_operation,
    instruction_type = EXCLUDED.instruction_type,
    environment = EXCLUDED.environment,
    config_kind = EXCLUDED.config_kind,
    bundle_key = EXCLUDED.bundle_key,
    target_version = EXCLUDED.target_version,
    operator_ref = EXCLUDED.operator_ref,
    reason_ref = EXCLUDED.reason_ref,
    status = EXCLUDED.status,
    updated_at = now()
`, string(instruction.TenantID), instruction.InstructionID, instruction.WorkflowID,
		instruction.PayloadRefHash, instruction.TargetService, instruction.TargetOperation,
		instruction.InstructionType, instruction.Environment, instruction.ConfigKind,
		instruction.BundleKey, instruction.TargetVersion, instruction.OperatorRef,
		instruction.ReasonRef, instruction.Status)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func scanWorkflowCompensationInstruction(row scanner) (types.WorkflowCompensationInstruction, error) {
	var instruction types.WorkflowCompensationInstruction
	err := row.Scan(
		&instruction.TenantID,
		&instruction.InstructionID,
		&instruction.WorkflowID,
		&instruction.PayloadRefHash,
		&instruction.TargetService,
		&instruction.TargetOperation,
		&instruction.InstructionType,
		&instruction.Environment,
		&instruction.ConfigKind,
		&instruction.BundleKey,
		&instruction.TargetVersion,
		&instruction.OperatorRef,
		&instruction.ReasonRef,
		&instruction.Status,
		&instruction.CreatedAt,
		&instruction.UpdatedAt,
	)
	if err != nil {
		return types.WorkflowCompensationInstruction{}, err
	}
	return instruction, nil
}
