package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"github.com/qsyy0921/IM/services/workflow-service/internal/types"
)

type PreparedWorkflow struct {
	Command     types.CreateWorkflowCommand
	CommandHash string
	WorkflowID  string
	StepID      string
	CreatedAt   time.Time
}

type PreparedDecision struct {
	Command     types.RecordWorkflowDecisionCommand
	CommandHash string
	DecisionID  string
	CreatedAt   time.Time
}

func PrepareWorkflow(
	command types.CreateWorkflowCommand,
	workflowID string,
	stepID string,
	now time.Time,
) (PreparedWorkflow, error) {
	normalized := command.Normalized()
	if err := normalized.Validate(); err != nil {
		return PreparedWorkflow{}, err
	}
	hash, err := createWorkflowCommandHash(normalized)
	if err != nil {
		return PreparedWorkflow{}, err
	}
	return PreparedWorkflow{
		Command:     normalized,
		CommandHash: hash,
		WorkflowID:  strings.TrimSpace(workflowID),
		StepID:      strings.TrimSpace(stepID),
		CreatedAt:   now.UTC(),
	}, nil
}

func WorkflowFromPrepared(prepared PreparedWorkflow) types.Workflow {
	command := prepared.Command
	return types.Workflow{
		TenantID:              command.AuthContext.TenantID,
		WorkflowID:            prepared.WorkflowID,
		IdempotencyKey:        command.IdempotencyKey,
		CommandHash:           prepared.CommandHash,
		WorkflowType:          command.WorkflowType,
		RiskLevel:             command.RiskLevel,
		RequesterRef:          command.RequesterRef,
		RequesterService:      command.RequesterService,
		TargetService:         command.TargetService,
		TargetOperation:       command.TargetOperation,
		TargetRefHash:         command.TargetRefHash,
		PayloadSchemaVersion:  command.PayloadSchemaVersion,
		PayloadRefHash:        command.PayloadRefHash,
		ApprovalPolicyRef:     command.ApprovalPolicyRef,
		TimeoutPolicyRef:      command.TimeoutPolicyRef,
		CompensationPolicyRef: command.CompensationPolicyRef,
		ReasonRef:             command.ReasonRef,
		EvidenceRefs:          append([]string(nil), command.EvidenceRefs...),
		Status:                types.WorkflowStatusWaitingDecision,
		CurrentStepID:         prepared.StepID,
		CorrelationID:         command.CorrelationID,
		CausationID:           command.CausationID,
		TraceID:               command.TraceID,
		CreatedAt:             prepared.CreatedAt,
		UpdatedAt:             prepared.CreatedAt,
	}
}

func StepFromPrepared(prepared PreparedWorkflow) types.WorkflowStep {
	command := prepared.Command
	return types.WorkflowStep{
		TenantID:        command.AuthContext.TenantID,
		WorkflowID:      prepared.WorkflowID,
		StepID:          prepared.StepID,
		StepIndex:       1,
		StepType:        types.WorkflowStepTypeApproval,
		TargetService:   command.TargetService,
		TargetOperation: command.TargetOperation,
		Status:          types.WorkflowStepStatusReady,
		CreatedAt:       prepared.CreatedAt,
		UpdatedAt:       prepared.CreatedAt,
	}
}

func PrepareDecision(
	command types.RecordWorkflowDecisionCommand,
	decisionID string,
	now time.Time,
) (PreparedDecision, error) {
	normalized := command.Normalized()
	if err := normalized.Validate(); err != nil {
		return PreparedDecision{}, err
	}
	hash, err := decisionCommandHash(normalized)
	if err != nil {
		return PreparedDecision{}, err
	}
	return PreparedDecision{
		Command:     normalized,
		CommandHash: hash,
		DecisionID:  strings.TrimSpace(decisionID),
		CreatedAt:   now.UTC(),
	}, nil
}

func DecisionFromPrepared(prepared PreparedDecision, tenantID types.TenantID) types.WorkflowDecision {
	command := prepared.Command
	return types.WorkflowDecision{
		TenantID:          tenantID,
		WorkflowID:        command.WorkflowID,
		DecisionID:        prepared.DecisionID,
		StepID:            command.StepID,
		IdempotencyKey:    command.IdempotencyKey,
		CommandHash:       prepared.CommandHash,
		DeciderRef:        command.DeciderRef,
		DecisionType:      command.DecisionType,
		DecisionPolicyRef: command.DecisionPolicyRef,
		ReasonRef:         command.ReasonRef,
		EvidenceRefs:      append([]string(nil), command.EvidenceRefs...),
		CreatedAt:         prepared.CreatedAt,
	}
}

func StatusAfterDecision(decisionType string) (string, bool) {
	switch decisionType {
	case types.DecisionTypeApprove:
		return types.WorkflowStatusApproved, true
	case types.DecisionTypeReject:
		return types.WorkflowStatusRejected, true
	case types.DecisionTypeCancel:
		return types.WorkflowStatusCanceled, true
	case types.DecisionTypeRequestChanges:
		return types.WorkflowStatusWaitingDecision, false
	default:
		return "", false
	}
}

func HashRef(value string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func createWorkflowCommandHash(command types.CreateWorkflowCommand) (string, error) {
	payload := map[string]any{
		"tenant_id":               string(command.AuthContext.TenantID),
		"requester_ref":           command.RequesterRef,
		"requester_service":       command.RequesterService,
		"workflow_type":           command.WorkflowType,
		"risk_level":              command.RiskLevel,
		"target_ref_hash":         command.TargetRefHash,
		"target_service":          command.TargetService,
		"target_operation":        command.TargetOperation,
		"approval_policy_ref":     command.ApprovalPolicyRef,
		"timeout_policy_ref":      command.TimeoutPolicyRef,
		"compensation_policy_ref": command.CompensationPolicyRef,
		"payload_schema_version":  command.PayloadSchemaVersion,
		"payload_ref_hash":        command.PayloadRefHash,
		"reason_ref":              command.ReasonRef,
		"evidence_refs":           command.EvidenceRefs,
		"idempotency_key":         command.IdempotencyKey,
		"correlation_id":          command.CorrelationID,
		"causation_id":            command.CausationID,
		"trace_id":                command.TraceID,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", types.NewInvalidArgument("workflow command hash payload invalid")
	}
	return HashRef(string(encoded)), nil
}

func decisionCommandHash(command types.RecordWorkflowDecisionCommand) (string, error) {
	payload := map[string]any{
		"tenant_id":           string(command.AuthContext.TenantID),
		"workflow_id":         command.WorkflowID,
		"step_id":             command.StepID,
		"decision_type":       command.DecisionType,
		"decider_ref":         command.DeciderRef,
		"decision_policy_ref": command.DecisionPolicyRef,
		"reason_ref":          command.ReasonRef,
		"evidence_refs":       command.EvidenceRefs,
		"idempotency_key":     command.IdempotencyKey,
		"correlation_id":      command.CorrelationID,
		"causation_id":        command.CausationID,
		"trace_id":            command.TraceID,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", types.NewInvalidArgument("workflow decision hash payload invalid")
	}
	return HashRef(string(encoded)), nil
}
