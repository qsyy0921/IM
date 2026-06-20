package types

import (
	"strings"
	"time"
)

const (
	WorkflowTypeActionApproval = "ACTION_APPROVAL"
	WorkflowTypeRepairApproval = "REPAIR_APPROVAL"
	WorkflowTypeAdminOperation = "ADMIN_OPERATION"

	RiskLevelLow      = "LOW"
	RiskLevelMedium   = "MEDIUM"
	RiskLevelHigh     = "HIGH"
	RiskLevelCritical = "CRITICAL"

	WorkflowStatusWaitingDecision = "WAITING_DECISION"
	WorkflowStatusApproved        = "APPROVED"
	WorkflowStatusRejected        = "REJECTED"
	WorkflowStatusCanceled        = "CANCELED"

	WorkflowStepTypeApproval = "APPROVAL"
	WorkflowStepStatusReady  = "READY"

	DecisionTypeApprove        = "APPROVE"
	DecisionTypeReject         = "REJECT"
	DecisionTypeRequestChanges = "REQUEST_CHANGES"
	DecisionTypeCancel         = "CANCEL"
)

type CreateWorkflowCommand struct {
	AuthContext           AuthContext
	RequesterRef          string
	RequesterService      string
	WorkflowType          string
	RiskLevel             string
	TargetRefHash         string
	TargetService         string
	TargetOperation       string
	ApprovalPolicyRef     string
	TimeoutPolicyRef      string
	CompensationPolicyRef string
	PayloadSchemaVersion  string
	PayloadRefHash        string
	ReasonRef             string
	EvidenceRefs          []string
	IdempotencyKey        string
	CorrelationID         string
	CausationID           string
	TraceID               string
}

type RecordWorkflowDecisionCommand struct {
	AuthContext       AuthContext
	WorkflowID        string
	StepID            string
	DecisionType      string
	DeciderRef        string
	DecisionPolicyRef string
	ReasonRef         string
	EvidenceRefs      []string
	IdempotencyKey    string
	CorrelationID     string
	CausationID       string
	TraceID           string
}

type GetWorkflowCommand struct {
	AuthContext AuthContext
	WorkflowID  string
}

type Workflow struct {
	TenantID              TenantID
	WorkflowID            string
	IdempotencyKey        string
	CommandHash           string
	WorkflowType          string
	RiskLevel             string
	RequesterRef          string
	RequesterService      string
	TargetService         string
	TargetOperation       string
	TargetRefHash         string
	PayloadSchemaVersion  string
	PayloadRefHash        string
	ApprovalPolicyRef     string
	TimeoutPolicyRef      string
	CompensationPolicyRef string
	ReasonRef             string
	EvidenceRefs          []string
	Status                string
	CurrentStepID         string
	CorrelationID         string
	CausationID           string
	TraceID               string
	CreatedAt             time.Time
	UpdatedAt             time.Time
	CompletedAt           time.Time
}

type WorkflowStep struct {
	TenantID        TenantID
	WorkflowID      string
	StepID          string
	StepIndex       int
	StepType        string
	TargetService   string
	TargetOperation string
	Status          string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type WorkflowDecision struct {
	TenantID          TenantID
	WorkflowID        string
	DecisionID        string
	StepID            string
	IdempotencyKey    string
	CommandHash       string
	DeciderRef        string
	DecisionType      string
	DecisionPolicyRef string
	ReasonRef         string
	EvidenceRefs      []string
	CreatedAt         time.Time
}

func (command CreateWorkflowCommand) Normalized() CreateWorkflowCommand {
	command.AuthContext = command.AuthContext.Normalized()
	command.RequesterRef = strings.TrimSpace(command.RequesterRef)
	command.RequesterService = strings.TrimSpace(command.RequesterService)
	command.WorkflowType = strings.ToUpper(strings.TrimSpace(command.WorkflowType))
	command.RiskLevel = strings.ToUpper(strings.TrimSpace(command.RiskLevel))
	command.TargetRefHash = strings.TrimSpace(command.TargetRefHash)
	command.TargetService = strings.TrimSpace(command.TargetService)
	command.TargetOperation = strings.TrimSpace(command.TargetOperation)
	command.ApprovalPolicyRef = strings.TrimSpace(command.ApprovalPolicyRef)
	command.TimeoutPolicyRef = strings.TrimSpace(command.TimeoutPolicyRef)
	command.CompensationPolicyRef = strings.TrimSpace(command.CompensationPolicyRef)
	command.PayloadSchemaVersion = strings.TrimSpace(command.PayloadSchemaVersion)
	command.PayloadRefHash = strings.TrimSpace(command.PayloadRefHash)
	command.ReasonRef = strings.TrimSpace(command.ReasonRef)
	command.EvidenceRefs = normalizeRefs(command.EvidenceRefs)
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	command.CorrelationID = strings.TrimSpace(command.CorrelationID)
	command.CausationID = strings.TrimSpace(command.CausationID)
	command.TraceID = strings.TrimSpace(command.TraceID)
	return command
}

func (command CreateWorkflowCommand) Validate() error {
	command = command.Normalized()
	if err := command.AuthContext.Validate(); err != nil {
		return err
	}
	if command.RequesterRef == "" || command.RequesterService == "" {
		return NewInvalidArgument("requester_ref and requester_service are required")
	}
	if !isAllowedWorkflowType(command.WorkflowType) {
		return NewInvalidArgument("workflow_type is unsupported")
	}
	if !isAllowedRiskLevel(command.RiskLevel) {
		return NewInvalidArgument("risk_level is unsupported")
	}
	if command.TargetService == "" || command.TargetOperation == "" || command.TargetRefHash == "" {
		return NewInvalidArgument("target refs are required")
	}
	if command.PayloadSchemaVersion == "" || command.PayloadRefHash == "" {
		return NewInvalidArgument("payload schema and hash refs are required")
	}
	if command.IdempotencyKey == "" {
		return NewInvalidArgument("idempotency_key is required")
	}
	if looksSensitive(command.TargetRefHash) || looksSensitive(command.PayloadRefHash) || looksSensitive(command.ReasonRef) {
		return NewInvalidArgument("workflow refs must be low-sensitive refs or hashes")
	}
	for _, ref := range command.EvidenceRefs {
		if looksSensitive(ref) {
			return NewInvalidArgument("evidence_refs must be low-sensitive refs or hashes")
		}
	}
	return nil
}

func (command RecordWorkflowDecisionCommand) Normalized() RecordWorkflowDecisionCommand {
	command.AuthContext = command.AuthContext.Normalized()
	command.WorkflowID = strings.TrimSpace(command.WorkflowID)
	command.StepID = strings.TrimSpace(command.StepID)
	command.DecisionType = strings.ToUpper(strings.TrimSpace(command.DecisionType))
	command.DeciderRef = strings.TrimSpace(command.DeciderRef)
	command.DecisionPolicyRef = strings.TrimSpace(command.DecisionPolicyRef)
	command.ReasonRef = strings.TrimSpace(command.ReasonRef)
	command.EvidenceRefs = normalizeRefs(command.EvidenceRefs)
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	command.CorrelationID = strings.TrimSpace(command.CorrelationID)
	command.CausationID = strings.TrimSpace(command.CausationID)
	command.TraceID = strings.TrimSpace(command.TraceID)
	return command
}

func (command RecordWorkflowDecisionCommand) Validate() error {
	command = command.Normalized()
	if err := command.AuthContext.Validate(); err != nil {
		return err
	}
	if command.WorkflowID == "" || command.StepID == "" {
		return NewInvalidArgument("workflow_id and step_id are required")
	}
	if !isAllowedDecisionType(command.DecisionType) {
		return NewInvalidArgument("decision_type is unsupported")
	}
	if command.DeciderRef == "" {
		return NewInvalidArgument("decider_ref is required")
	}
	if command.IdempotencyKey == "" {
		return NewInvalidArgument("idempotency_key is required")
	}
	if looksSensitive(command.ReasonRef) {
		return NewInvalidArgument("reason_ref must be a low-sensitive ref")
	}
	for _, ref := range command.EvidenceRefs {
		if looksSensitive(ref) {
			return NewInvalidArgument("evidence_refs must be low-sensitive refs or hashes")
		}
	}
	return nil
}

func (command GetWorkflowCommand) Normalized() GetWorkflowCommand {
	command.AuthContext = command.AuthContext.Normalized()
	command.WorkflowID = strings.TrimSpace(command.WorkflowID)
	return command
}

func (command GetWorkflowCommand) Validate() error {
	command = command.Normalized()
	if err := command.AuthContext.Validate(); err != nil {
		return err
	}
	if command.WorkflowID == "" {
		return NewInvalidArgument("workflow_id is required")
	}
	return nil
}

func isAllowedWorkflowType(value string) bool {
	return value == WorkflowTypeActionApproval ||
		value == WorkflowTypeRepairApproval ||
		value == WorkflowTypeAdminOperation
}

func isAllowedRiskLevel(value string) bool {
	switch value {
	case RiskLevelLow, RiskLevelMedium, RiskLevelHigh, RiskLevelCritical:
		return true
	default:
		return false
	}
}

func isAllowedDecisionType(value string) bool {
	switch value {
	case DecisionTypeApprove, DecisionTypeReject, DecisionTypeRequestChanges, DecisionTypeCancel:
		return true
	default:
		return false
	}
}

func normalizeRefs(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}

func looksSensitive(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return false
	}
	for _, marker := range []string{"secret", "token", "api_key", "apikey", "password", "private://", "raw:", "dsn=", "postgres://"} {
		if strings.Contains(value, marker) {
			return true
		}
	}
	return false
}
