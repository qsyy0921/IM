package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/workflow-service/internal/domain"
	"github.com/qsyy0921/IM/services/workflow-service/internal/types"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (repository *Repository) CreateWorkflow(
	ctx context.Context,
	prepared domain.PreparedWorkflow,
) (types.Workflow, bool, error) {
	if repository.pool == nil {
		return types.Workflow{}, false, types.NewDBWriteFailed("workflow repository is not configured")
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return types.Workflow{}, false, types.NewDBWriteFailed(err.Error())
	}
	defer func() { _ = tx.Rollback(ctx) }()

	existing, found, err := findWorkflowByIdempotency(ctx, tx, prepared.Command.AuthContext.TenantID, prepared.Command.RequesterService, prepared.Command.IdempotencyKey)
	if err != nil {
		return types.Workflow{}, false, err
	}
	if found {
		if existing.CommandHash != prepared.CommandHash {
			return types.Workflow{}, false, types.NewAlreadyExists("workflow idempotency conflict")
		}
		if err := tx.Commit(ctx); err != nil {
			return types.Workflow{}, false, types.NewDBWriteFailed(err.Error())
		}
		return existing, true, nil
	}

	workflow := domain.WorkflowFromPrepared(prepared)
	step := domain.StepFromPrepared(prepared)
	if err := insertWorkflow(ctx, tx, workflow); err != nil {
		return types.Workflow{}, false, err
	}
	if err := insertWorkflowStep(ctx, tx, step); err != nil {
		return types.Workflow{}, false, err
	}
	if err := insertWorkflowSubmittedOutbox(ctx, tx, workflow); err != nil {
		return types.Workflow{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return types.Workflow{}, false, types.NewDBWriteFailed(err.Error())
	}
	return workflow, false, nil
}

func (repository *Repository) RecordWorkflowDecision(
	ctx context.Context,
	prepared domain.PreparedDecision,
) (types.Workflow, types.WorkflowDecision, bool, error) {
	if repository.pool == nil {
		return types.Workflow{}, types.WorkflowDecision{}, false, types.NewDBWriteFailed("workflow repository is not configured")
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return types.Workflow{}, types.WorkflowDecision{}, false, types.NewDBWriteFailed(err.Error())
	}
	defer func() { _ = tx.Rollback(ctx) }()

	existingDecision, found, err := findDecisionByIdempotency(ctx, tx, prepared.Command.AuthContext.TenantID, prepared.Command.WorkflowID, prepared.Command.DeciderRef, prepared.Command.IdempotencyKey)
	if err != nil {
		return types.Workflow{}, types.WorkflowDecision{}, false, err
	}
	if found {
		if existingDecision.CommandHash != prepared.CommandHash {
			return types.Workflow{}, types.WorkflowDecision{}, false, types.NewAlreadyExists("workflow decision idempotency conflict")
		}
		workflow, err := getWorkflowForUpdate(ctx, tx, prepared.Command.AuthContext.TenantID, prepared.Command.WorkflowID)
		if err != nil {
			return types.Workflow{}, types.WorkflowDecision{}, false, err
		}
		if err := tx.Commit(ctx); err != nil {
			return types.Workflow{}, types.WorkflowDecision{}, false, types.NewDBWriteFailed(err.Error())
		}
		return workflow, existingDecision, true, nil
	}

	workflow, err := getWorkflowForUpdate(ctx, tx, prepared.Command.AuthContext.TenantID, prepared.Command.WorkflowID)
	if err != nil {
		return types.Workflow{}, types.WorkflowDecision{}, false, err
	}
	if workflow.Status != types.WorkflowStatusWaitingDecision {
		return types.Workflow{}, types.WorkflowDecision{}, false, types.NewFailedPrecondition("workflow is not waiting for decision")
	}
	if prepared.Command.StepID != workflow.CurrentStepID {
		return types.Workflow{}, types.WorkflowDecision{}, false, types.NewFailedPrecondition("workflow step is not current")
	}
	if workflow.RequesterRef == prepared.Command.DeciderRef && (workflow.RiskLevel == types.RiskLevelHigh || workflow.RiskLevel == types.RiskLevelCritical) {
		return types.Workflow{}, types.WorkflowDecision{}, false, types.NewPermissionDenied("workflow decision violates separation of duty")
	}

	decision := domain.DecisionFromPrepared(prepared, workflow.TenantID)
	if err := insertDecision(ctx, tx, decision); err != nil {
		return types.Workflow{}, types.WorkflowDecision{}, false, err
	}
	nextStatus, terminal := domain.StatusAfterDecision(decision.DecisionType)
	if nextStatus == "" {
		return types.Workflow{}, types.WorkflowDecision{}, false, types.NewInvalidArgument("decision_type is unsupported")
	}
	updated, err := updateWorkflowAfterDecision(ctx, tx, workflow, nextStatus, terminal, prepared.CreatedAt)
	if err != nil {
		return types.Workflow{}, types.WorkflowDecision{}, false, err
	}
	if err := insertDecisionRecordedOutbox(ctx, tx, updated, decision); err != nil {
		return types.Workflow{}, types.WorkflowDecision{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return types.Workflow{}, types.WorkflowDecision{}, false, types.NewDBWriteFailed(err.Error())
	}
	return updated, decision, false, nil
}

func (repository *Repository) GetWorkflow(
	ctx context.Context,
	command types.GetWorkflowCommand,
) (types.Workflow, []types.WorkflowDecision, error) {
	if repository.pool == nil {
		return types.Workflow{}, nil, types.NewDBReadFailed("workflow repository is not configured")
	}
	row := repository.pool.QueryRow(ctx, selectWorkflowSQL()+`
WHERE tenant_id = $1 AND workflow_id = $2
`, string(command.AuthContext.TenantID), command.WorkflowID)
	workflow, err := scanWorkflow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return types.Workflow{}, nil, types.NewNotFound("workflow not found")
		}
		return types.Workflow{}, nil, types.NewDBReadFailed(err.Error())
	}
	decisions, err := listDecisions(ctx, repository.pool, command.AuthContext.TenantID, command.WorkflowID)
	if err != nil {
		return types.Workflow{}, nil, err
	}
	return workflow, decisions, nil
}

func (repository *Repository) ListWorkflows(
	ctx context.Context,
	command types.ListWorkflowsCommand,
) ([]types.Workflow, error) {
	if repository.pool == nil {
		return nil, types.NewDBReadFailed("workflow repository is not configured")
	}
	command = command.Normalized()
	conditions := []string{"tenant_id = $1"}
	args := []any{string(command.AuthContext.TenantID)}
	addCondition := func(column string, value string) {
		if value == "" {
			return
		}
		args = append(args, value)
		conditions = append(conditions, fmt.Sprintf("%s = $%d", column, len(args)))
	}
	addCondition("workflow_type", command.WorkflowType)
	addCondition("status", command.Status)
	addCondition("target_service", command.TargetService)
	addCondition("target_operation", command.TargetOperation)
	addCondition("approval_policy_ref", command.ApprovalPolicyRef)
	args = append(args, command.PageSize)
	query := selectWorkflowSQL() + `
WHERE ` + strings.Join(conditions, " AND ") + `
ORDER BY created_at DESC, workflow_id DESC
LIMIT $` + fmt.Sprint(len(args)) + `
`
	rows, err := repository.pool.Query(ctx, query, args...)
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

func findWorkflowByIdempotency(ctx context.Context, tx pgx.Tx, tenantID types.TenantID, requesterService string, key string) (types.Workflow, bool, error) {
	row := tx.QueryRow(ctx, selectWorkflowSQL()+`
WHERE tenant_id = $1 AND requester_service = $2 AND idempotency_key = $3
LIMIT 1
`, string(tenantID), requesterService, key)
	workflow, err := scanWorkflow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return types.Workflow{}, false, nil
		}
		return types.Workflow{}, false, types.NewDBReadFailed(err.Error())
	}
	return workflow, true, nil
}

func findDecisionByIdempotency(ctx context.Context, tx pgx.Tx, tenantID types.TenantID, workflowID string, deciderRef string, key string) (types.WorkflowDecision, bool, error) {
	row := tx.QueryRow(ctx, selectDecisionSQL()+`
WHERE tenant_id = $1 AND workflow_id = $2 AND decider_ref = $3 AND idempotency_key = $4
LIMIT 1
`, string(tenantID), workflowID, deciderRef, key)
	decision, err := scanDecision(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return types.WorkflowDecision{}, false, nil
		}
		return types.WorkflowDecision{}, false, types.NewDBReadFailed(err.Error())
	}
	return decision, true, nil
}

func getWorkflowForUpdate(ctx context.Context, tx pgx.Tx, tenantID types.TenantID, workflowID string) (types.Workflow, error) {
	row := tx.QueryRow(ctx, selectWorkflowSQL()+`
WHERE tenant_id = $1 AND workflow_id = $2
FOR UPDATE
`, string(tenantID), workflowID)
	workflow, err := scanWorkflow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return types.Workflow{}, types.NewNotFound("workflow not found")
		}
		return types.Workflow{}, types.NewDBReadFailed(err.Error())
	}
	return workflow, nil
}

func insertWorkflow(ctx context.Context, tx pgx.Tx, workflow types.Workflow) error {
	evidenceJSON, err := json.Marshal(workflow.EvidenceRefs)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	var completedAt any
	if !workflow.CompletedAt.IsZero() {
		completedAt = workflow.CompletedAt
	}
	_, err = tx.Exec(ctx, `
INSERT INTO workflow_requests (
    tenant_id, workflow_id, idempotency_key, command_hash, workflow_type, risk_level,
    requester_ref, requester_service, target_service, target_operation, target_ref_hash,
    payload_schema_version, payload_ref_hash, approval_policy_ref, timeout_policy_ref,
    compensation_policy_ref, reason_ref, evidence_refs_json, status, current_step_id,
    correlation_id, causation_id, trace_id, created_at, updated_at, completed_at
) VALUES (
    $1, $2, $3, $4, $5, $6,
    $7, $8, $9, $10, $11,
    $12, $13, $14, $15,
    $16, $17, $18::jsonb, $19, $20,
    $21, $22, $23, $24, $25, $26
)
`, string(workflow.TenantID), workflow.WorkflowID, workflow.IdempotencyKey, workflow.CommandHash,
		workflow.WorkflowType, workflow.RiskLevel, workflow.RequesterRef, workflow.RequesterService,
		workflow.TargetService, workflow.TargetOperation, workflow.TargetRefHash,
		workflow.PayloadSchemaVersion, workflow.PayloadRefHash, workflow.ApprovalPolicyRef,
		workflow.TimeoutPolicyRef, workflow.CompensationPolicyRef, workflow.ReasonRef,
		string(evidenceJSON), workflow.Status, workflow.CurrentStepID, workflow.CorrelationID,
		workflow.CausationID, workflow.TraceID, workflow.CreatedAt, workflow.UpdatedAt, completedAt)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func insertWorkflowStep(ctx context.Context, tx pgx.Tx, step types.WorkflowStep) error {
	_, err := tx.Exec(ctx, `
INSERT INTO workflow_steps (
    tenant_id, workflow_id, step_id, step_index, step_type, target_service,
    target_operation, status, created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6,
    $7, $8, $9, $10
)
`, string(step.TenantID), step.WorkflowID, step.StepID, step.StepIndex, step.StepType,
		step.TargetService, step.TargetOperation, step.Status, step.CreatedAt, step.UpdatedAt)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func insertDecision(ctx context.Context, tx pgx.Tx, decision types.WorkflowDecision) error {
	evidenceJSON, err := json.Marshal(decision.EvidenceRefs)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	_, err = tx.Exec(ctx, `
INSERT INTO workflow_decisions (
    tenant_id, workflow_id, decision_id, step_id, idempotency_key, command_hash,
    decider_ref, decision_type, decision_policy_ref, reason_ref, evidence_refs_json,
    created_at
) VALUES (
    $1, $2, $3, $4, $5, $6,
    $7, $8, $9, $10, $11::jsonb,
    $12
)
`, string(decision.TenantID), decision.WorkflowID, decision.DecisionID, decision.StepID,
		decision.IdempotencyKey, decision.CommandHash, decision.DeciderRef, decision.DecisionType,
		decision.DecisionPolicyRef, decision.ReasonRef, string(evidenceJSON), decision.CreatedAt)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func updateWorkflowAfterDecision(ctx context.Context, tx pgx.Tx, workflow types.Workflow, nextStatus string, terminal bool, now time.Time) (types.Workflow, error) {
	var completedAt any
	if terminal {
		completedAt = now
		workflow.CompletedAt = now
	}
	_, err := tx.Exec(ctx, `
UPDATE workflow_requests
SET status = $3, updated_at = $4, completed_at = $5
WHERE tenant_id = $1 AND workflow_id = $2
`, string(workflow.TenantID), workflow.WorkflowID, nextStatus, now, completedAt)
	if err != nil {
		return types.Workflow{}, types.NewDBWriteFailed(err.Error())
	}
	workflow.Status = nextStatus
	workflow.UpdatedAt = now
	return workflow, nil
}

func insertWorkflowSubmittedOutbox(ctx context.Context, tx pgx.Tx, workflow types.Workflow) error {
	payload := workflowPayload(workflow)
	return insertOutbox(ctx, tx, "evt_"+workflow.WorkflowID, workflow, "workflow.submitted.v1", payload)
}

func insertDecisionRecordedOutbox(ctx context.Context, tx pgx.Tx, workflow types.Workflow, decision types.WorkflowDecision) error {
	payload := workflowPayload(workflow)
	payload["decision_id"] = decision.DecisionID
	payload["decision_type"] = decision.DecisionType
	payload["decider_ref_hash"] = domain.HashRef(decision.DeciderRef)
	return insertOutbox(ctx, tx, "evt_"+decision.DecisionID, workflow, "workflow.decision.recorded.v1", payload)
}

func insertOutbox(ctx context.Context, tx pgx.Tx, eventID string, workflow types.Workflow, eventType string, payload map[string]any) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	_, err = tx.Exec(ctx, `
INSERT INTO workflow_outbox (
    event_id, tenant_id, workflow_id, aggregate_type, aggregate_id, event_type,
    event_version, partition_key, payload_json, status, available_at, created_at, updated_at
) VALUES (
    $1, $2, $3, 'workflow', $3, $4,
    1, $5, $6::jsonb, 'PENDING', now(), now(), now()
)
ON CONFLICT (event_id) DO NOTHING
`, eventID, string(workflow.TenantID), workflow.WorkflowID, eventType, string(workflow.TenantID)+":"+workflow.WorkflowID, string(encoded))
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func workflowPayload(workflow types.Workflow) map[string]any {
	return map[string]any{
		"tenant_id":              string(workflow.TenantID),
		"workflow_id":            workflow.WorkflowID,
		"workflow_type":          workflow.WorkflowType,
		"risk_level":             workflow.RiskLevel,
		"status":                 workflow.Status,
		"target_service":         workflow.TargetService,
		"target_operation":       workflow.TargetOperation,
		"target_ref_hash":        workflow.TargetRefHash,
		"payload_schema_version": workflow.PayloadSchemaVersion,
		"payload_ref_hash":       workflow.PayloadRefHash,
		"current_step_id":        workflow.CurrentStepID,
		"correlation_id":         workflow.CorrelationID,
		"causation_id":           workflow.CausationID,
		"trace_id":               workflow.TraceID,
	}
}

func listDecisions(ctx context.Context, querier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, tenantID types.TenantID, workflowID string) ([]types.WorkflowDecision, error) {
	rows, err := querier.Query(ctx, selectDecisionSQL()+`
WHERE tenant_id = $1 AND workflow_id = $2
ORDER BY created_at, decision_id
`, string(tenantID), workflowID)
	if err != nil {
		return nil, types.NewDBReadFailed(err.Error())
	}
	defer rows.Close()

	decisions := []types.WorkflowDecision{}
	for rows.Next() {
		decision, err := scanDecision(rows)
		if err != nil {
			return nil, types.NewDBReadFailed(err.Error())
		}
		decisions = append(decisions, decision)
	}
	if err := rows.Err(); err != nil {
		return nil, types.NewDBReadFailed(err.Error())
	}
	return decisions, nil
}

func selectWorkflowSQL() string {
	return `
SELECT tenant_id, workflow_id, idempotency_key, command_hash, workflow_type, risk_level,
       requester_ref, requester_service, target_service, target_operation, target_ref_hash,
       payload_schema_version, payload_ref_hash, approval_policy_ref, timeout_policy_ref,
       compensation_policy_ref, reason_ref, evidence_refs_json::text, status, current_step_id,
       correlation_id, causation_id, trace_id, created_at, updated_at, completed_at
FROM workflow_requests
`
}

func selectDecisionSQL() string {
	return `
SELECT tenant_id, workflow_id, decision_id, step_id, idempotency_key, command_hash,
       decider_ref, decision_type, decision_policy_ref, reason_ref, evidence_refs_json::text,
       created_at
FROM workflow_decisions
`
}

type scanner interface {
	Scan(dest ...any) error
}

func scanWorkflow(row scanner) (types.Workflow, error) {
	var workflow types.Workflow
	var evidenceJSON string
	var completedAt *time.Time
	err := row.Scan(
		&workflow.TenantID, &workflow.WorkflowID, &workflow.IdempotencyKey, &workflow.CommandHash,
		&workflow.WorkflowType, &workflow.RiskLevel, &workflow.RequesterRef, &workflow.RequesterService,
		&workflow.TargetService, &workflow.TargetOperation, &workflow.TargetRefHash,
		&workflow.PayloadSchemaVersion, &workflow.PayloadRefHash, &workflow.ApprovalPolicyRef,
		&workflow.TimeoutPolicyRef, &workflow.CompensationPolicyRef, &workflow.ReasonRef,
		&evidenceJSON, &workflow.Status, &workflow.CurrentStepID, &workflow.CorrelationID,
		&workflow.CausationID, &workflow.TraceID, &workflow.CreatedAt, &workflow.UpdatedAt,
		&completedAt,
	)
	if err != nil {
		return types.Workflow{}, err
	}
	if err := json.Unmarshal([]byte(evidenceJSON), &workflow.EvidenceRefs); err != nil {
		return types.Workflow{}, err
	}
	if completedAt != nil {
		workflow.CompletedAt = *completedAt
	}
	return workflow, nil
}

func scanDecision(row scanner) (types.WorkflowDecision, error) {
	var decision types.WorkflowDecision
	var evidenceJSON string
	err := row.Scan(
		&decision.TenantID, &decision.WorkflowID, &decision.DecisionID, &decision.StepID,
		&decision.IdempotencyKey, &decision.CommandHash, &decision.DeciderRef,
		&decision.DecisionType, &decision.DecisionPolicyRef, &decision.ReasonRef,
		&evidenceJSON, &decision.CreatedAt,
	)
	if err != nil {
		return types.WorkflowDecision{}, err
	}
	if err := json.Unmarshal([]byte(evidenceJSON), &decision.EvidenceRefs); err != nil {
		return types.WorkflowDecision{}, err
	}
	return decision, nil
}
