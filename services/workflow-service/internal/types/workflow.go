package types

import (
	"strings"
	"time"
)

const (
	WorkflowTypeActionApproval      = "ACTION_APPROVAL"
	WorkflowTypeRepairApproval      = "REPAIR_APPROVAL"
	WorkflowTypeAdminOperation      = "ADMIN_OPERATION"
	WorkflowTypeCompensationRequest = "COMPENSATION_REQUEST"

	RiskLevelLow      = "LOW"
	RiskLevelMedium   = "MEDIUM"
	RiskLevelHigh     = "HIGH"
	RiskLevelCritical = "CRITICAL"

	WorkflowStatusWaitingDecision     = "WAITING_DECISION"
	WorkflowStatusApproved            = "APPROVED"
	WorkflowStatusRejected            = "REJECTED"
	WorkflowStatusCanceled            = "CANCELED"
	WorkflowStatusTimedOut            = "TIMED_OUT"
	WorkflowStatusCompensationPending = "COMPENSATION_PENDING"
	WorkflowStatusCompensated         = "COMPENSATED"

	WorkflowStepTypeApproval = "APPROVAL"
	WorkflowStepStatusReady  = "READY"

	WorkflowTimerTypeApprovalTimeout = "APPROVAL_TIMEOUT"
	WorkflowTimerStatusPending       = "PENDING"
	WorkflowTimerStatusFired         = "FIRED"
	WorkflowTimerStatusCanceled      = "CANCELED"

	WorkflowCompensationStatusRequested = "REQUESTED"
	WorkflowCompensationStatusExecuting = "EXECUTING"
	WorkflowCompensationStatusSucceeded = "SUCCEEDED"
	WorkflowCompensationStatusFailed    = "FAILED"

	WorkflowCompensationInstructionTypeControlPlaneRollback = "CONTROL_PLANE_ROLLBACK"
	WorkflowCompensationInstructionStatusActive             = "ACTIVE"
	WorkflowCompensationInstructionStatusDisabled           = "DISABLED"

	WorkflowExternalCallbackDeliveryStatusPending      = "PENDING"
	WorkflowExternalCallbackDeliveryStatusInFlight     = "IN_FLIGHT"
	WorkflowExternalCallbackDeliveryStatusDelivered    = "DELIVERED"
	WorkflowExternalCallbackDeliveryStatusRetryPending = "RETRY_PENDING"
	WorkflowExternalCallbackDeliveryStatusDLQ          = "DLQ"

	WorkflowEventCompensationRequested     = "workflow.compensation.requested.v1"
	WorkflowEventCompensationSucceeded     = "workflow.compensation.succeeded.v1"
	WorkflowEventCompensationFailed        = "workflow.compensation.failed.v1"
	WorkflowEventTimedOut                  = "workflow.timed_out.v1"
	WorkflowEventExternalCallbackDelivered = "workflow.external_callback.delivered.v1"
	WorkflowEventExternalCallbackDLQ       = "workflow.external_callback.dlq.v1"
	WorkflowEventExternalCallbackRedriven  = "workflow.external_callback.redriven.v1"

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

type ListWorkflowsCommand struct {
	AuthContext       AuthContext
	WorkflowType      string
	Status            string
	TargetService     string
	TargetOperation   string
	ApprovalPolicyRef string
	PageSize          int
}

type ListWorkflowCompensationInstructionsCommand struct {
	AuthContext AuthContext
	WorkflowID  string
	Status      string
	PageSize    int
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

type WorkflowTimer struct {
	TenantID   TenantID
	WorkflowID string
	TimerID    string
	StepID     string
	TimerType  string
	DueAt      time.Time
	Status     string
	FiredAt    time.Time
	CreatedAt  time.Time
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

type WorkflowCompensation struct {
	TenantID              TenantID
	WorkflowID            string
	CompensationID        string
	SourceStepID          string
	TargetService         string
	TargetOperation       string
	TargetRefHash         string
	PayloadSchemaVersion  string
	PayloadRefHash        string
	CompensationPolicyRef string
	ReasonRef             string
	DownstreamService     string
	DownstreamRequestRef  string
	Status                string
	FailureClass          string
	PublicError           string
	CreatedAt             time.Time
	UpdatedAt             time.Time
	CompletedAt           time.Time
}

type WorkflowCompensationExecutionResult struct {
	DownstreamService    string
	DownstreamRequestRef string
	Status               string
	FailureClass         string
	PublicError          string
}

type WorkflowCompensationInstruction struct {
	TenantID        TenantID
	InstructionID   string
	WorkflowID      string
	PayloadRefHash  string
	TargetService   string
	TargetOperation string
	InstructionType string
	Environment     string
	ConfigKind      string
	BundleKey       string
	TargetVersion   string
	OperatorRef     string
	ReasonRef       string
	Status          string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type WorkflowExternalCallbackDelivery struct {
	TenantID                     TenantID
	WorkflowID                   string
	DeliveryID                   string
	DeliveryPlanSha256           string
	SourceDecisionManifestSha256 string
	StepID                       string
	WorkflowType                 string
	TargetService                string
	TargetOperation              string
	TargetRefHash                string
	PayloadSchemaVersion         string
	PayloadRefHash               string
	ApprovalPolicyRef            string
	DecisionPolicyRef            string
	CallbackProviderRef          string
	CallbackEndpointRef          string
	DeliveryQueueRef             string
	RetryPolicyRef               string
	BackoffPolicyRef             string
	CallbackTimeoutPolicyRef     string
	CallbackPayloadSchemaVersion string
	CallbackPayloadRefHash       string
	Status                       string
	AttemptCount                 int
	MaxAttempts                  int
	AvailableAt                  time.Time
	LeasedUntil                  time.Time
	LastAttemptAt                time.Time
	DeliveredAt                  time.Time
	LastFailureClass             string
	LastDeliveryResultRef        string
	RedriveCount                 int
	LastRedrivePlanSha256        string
	LastRedriveReasonRef         string
	LastRedrivenAt               time.Time
	CreatedAt                    time.Time
	UpdatedAt                    time.Time
}

type WorkflowExternalCallbackDeliveryResult struct {
	DeliveryResultRef string
	FailureClass      string
}

type WorkflowExternalCallbackRedrivePlan struct {
	TenantID                   TenantID
	RedrivePlanID              string
	RedrivePlanSha256          string
	SourceDeliveryStatusSha256 string
	SourceDeliveryPlanSha256   string
	WorkflowID                 string
	StepID                     string
	WorkflowType               string
	TargetService              string
	TargetOperation            string
	TargetRefHash              string
	PayloadSchemaVersion       string
	PayloadRefHash             string
	ApprovalPolicyRef          string
	DecisionPolicyRef          string
	DeliveryStatus             string
	AttemptNumber              int
	MaxAttempts                int
	DeliveryAttemptRef         string
	FailureClassRef            string
	RedrivePolicyRef           string
	RedriveQueueRef            string
	RedriveReasonRef           string
	OperatorReviewRef          string
	AvailableAt                time.Time
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

func (command ListWorkflowsCommand) Normalized() ListWorkflowsCommand {
	command.AuthContext = command.AuthContext.Normalized()
	command.WorkflowType = strings.ToUpper(strings.TrimSpace(command.WorkflowType))
	command.Status = strings.ToUpper(strings.TrimSpace(command.Status))
	command.TargetService = strings.TrimSpace(command.TargetService)
	command.TargetOperation = strings.TrimSpace(command.TargetOperation)
	command.ApprovalPolicyRef = strings.TrimSpace(command.ApprovalPolicyRef)
	if command.PageSize <= 0 {
		command.PageSize = 50
	}
	if command.PageSize > 200 {
		command.PageSize = 200
	}
	return command
}

func (command ListWorkflowsCommand) Validate() error {
	command = command.Normalized()
	if err := command.AuthContext.Validate(); err != nil {
		return err
	}
	if command.WorkflowType != "" && !isAllowedWorkflowType(command.WorkflowType) {
		return NewInvalidArgument("workflow_type is unsupported")
	}
	if command.Status != "" && !isAllowedWorkflowStatus(command.Status) {
		return NewInvalidArgument("workflow status is unsupported")
	}
	if looksSensitive(command.TargetService) ||
		looksSensitive(command.TargetOperation) ||
		looksSensitive(command.ApprovalPolicyRef) {
		return NewInvalidArgument("workflow list filters must be low-sensitive refs")
	}
	return nil
}

func (command ListWorkflowCompensationInstructionsCommand) Normalized() ListWorkflowCompensationInstructionsCommand {
	command.AuthContext = command.AuthContext.Normalized()
	command.WorkflowID = strings.TrimSpace(command.WorkflowID)
	command.Status = strings.ToUpper(strings.TrimSpace(command.Status))
	if command.PageSize <= 0 {
		command.PageSize = 50
	}
	if command.PageSize > 200 {
		command.PageSize = 200
	}
	return command
}

func (command ListWorkflowCompensationInstructionsCommand) Validate() error {
	command = command.Normalized()
	if err := command.AuthContext.Validate(); err != nil {
		return err
	}
	if command.WorkflowID == "" {
		return NewInvalidArgument("workflow_id is required")
	}
	if command.Status != "" &&
		command.Status != WorkflowCompensationInstructionStatusActive &&
		command.Status != WorkflowCompensationInstructionStatusDisabled {
		return NewInvalidArgument("workflow compensation instruction status is unsupported")
	}
	return nil
}

func (instruction WorkflowCompensationInstruction) Normalized() WorkflowCompensationInstruction {
	instruction.TenantID = TenantID(strings.TrimSpace(string(instruction.TenantID)))
	instruction.InstructionID = strings.TrimSpace(instruction.InstructionID)
	instruction.WorkflowID = strings.TrimSpace(instruction.WorkflowID)
	instruction.PayloadRefHash = strings.TrimSpace(instruction.PayloadRefHash)
	instruction.TargetService = strings.TrimSpace(instruction.TargetService)
	instruction.TargetOperation = strings.TrimSpace(instruction.TargetOperation)
	instruction.InstructionType = strings.ToUpper(strings.TrimSpace(instruction.InstructionType))
	instruction.Environment = strings.TrimSpace(instruction.Environment)
	instruction.ConfigKind = strings.TrimSpace(instruction.ConfigKind)
	instruction.BundleKey = strings.TrimSpace(instruction.BundleKey)
	instruction.TargetVersion = strings.TrimSpace(instruction.TargetVersion)
	instruction.OperatorRef = strings.TrimSpace(instruction.OperatorRef)
	instruction.ReasonRef = strings.TrimSpace(instruction.ReasonRef)
	instruction.Status = strings.ToUpper(strings.TrimSpace(instruction.Status))
	if instruction.Status == "" {
		instruction.Status = WorkflowCompensationInstructionStatusActive
	}
	return instruction
}

func (instruction WorkflowCompensationInstruction) Validate() error {
	instruction = instruction.Normalized()
	if instruction.TenantID == "" ||
		instruction.InstructionID == "" ||
		instruction.WorkflowID == "" ||
		instruction.PayloadRefHash == "" ||
		instruction.TargetService == "" ||
		instruction.TargetOperation == "" ||
		instruction.InstructionType == "" ||
		instruction.OperatorRef == "" ||
		instruction.ReasonRef == "" {
		return NewInvalidArgument("workflow compensation instruction is incomplete")
	}
	if instruction.InstructionType != WorkflowCompensationInstructionTypeControlPlaneRollback {
		return NewInvalidArgument("workflow compensation instruction type is unsupported")
	}
	if instruction.Status != WorkflowCompensationInstructionStatusActive &&
		instruction.Status != WorkflowCompensationInstructionStatusDisabled {
		return NewInvalidArgument("workflow compensation instruction status is unsupported")
	}
	if instruction.TargetService != "control-plane-service" || instruction.TargetOperation != "CONFIG_ROLLBACK" {
		return NewInvalidArgument("workflow compensation instruction target is unsupported")
	}
	if instruction.Environment == "" || instruction.ConfigKind == "" ||
		instruction.BundleKey == "" || instruction.TargetVersion == "" {
		return NewInvalidArgument("control-plane compensation instruction is incomplete")
	}
	if looksSensitive(instruction.PayloadRefHash) ||
		looksSensitive(instruction.OperatorRef) ||
		looksSensitive(instruction.ReasonRef) {
		return NewInvalidArgument("workflow compensation instruction refs must be low-sensitive")
	}
	return nil
}

func (delivery WorkflowExternalCallbackDelivery) Normalized() WorkflowExternalCallbackDelivery {
	delivery.TenantID = TenantID(strings.TrimSpace(string(delivery.TenantID)))
	delivery.WorkflowID = strings.TrimSpace(delivery.WorkflowID)
	delivery.DeliveryID = strings.TrimSpace(delivery.DeliveryID)
	delivery.DeliveryPlanSha256 = strings.TrimSpace(delivery.DeliveryPlanSha256)
	delivery.SourceDecisionManifestSha256 = strings.TrimSpace(delivery.SourceDecisionManifestSha256)
	delivery.StepID = strings.TrimSpace(delivery.StepID)
	delivery.WorkflowType = strings.ToUpper(strings.TrimSpace(delivery.WorkflowType))
	delivery.TargetService = strings.TrimSpace(delivery.TargetService)
	delivery.TargetOperation = strings.TrimSpace(delivery.TargetOperation)
	delivery.TargetRefHash = strings.TrimSpace(delivery.TargetRefHash)
	delivery.PayloadSchemaVersion = strings.TrimSpace(delivery.PayloadSchemaVersion)
	delivery.PayloadRefHash = strings.TrimSpace(delivery.PayloadRefHash)
	delivery.ApprovalPolicyRef = strings.TrimSpace(delivery.ApprovalPolicyRef)
	delivery.DecisionPolicyRef = strings.TrimSpace(delivery.DecisionPolicyRef)
	delivery.CallbackProviderRef = strings.TrimSpace(delivery.CallbackProviderRef)
	delivery.CallbackEndpointRef = strings.TrimSpace(delivery.CallbackEndpointRef)
	delivery.DeliveryQueueRef = strings.TrimSpace(delivery.DeliveryQueueRef)
	delivery.RetryPolicyRef = strings.TrimSpace(delivery.RetryPolicyRef)
	delivery.BackoffPolicyRef = strings.TrimSpace(delivery.BackoffPolicyRef)
	delivery.CallbackTimeoutPolicyRef = strings.TrimSpace(delivery.CallbackTimeoutPolicyRef)
	delivery.CallbackPayloadSchemaVersion = strings.TrimSpace(delivery.CallbackPayloadSchemaVersion)
	delivery.CallbackPayloadRefHash = strings.TrimSpace(delivery.CallbackPayloadRefHash)
	delivery.Status = strings.ToUpper(strings.TrimSpace(delivery.Status))
	if delivery.Status == "" {
		delivery.Status = WorkflowExternalCallbackDeliveryStatusPending
	}
	delivery.LastFailureClass = strings.TrimSpace(delivery.LastFailureClass)
	delivery.LastDeliveryResultRef = strings.TrimSpace(delivery.LastDeliveryResultRef)
	delivery.LastRedrivePlanSha256 = strings.TrimSpace(delivery.LastRedrivePlanSha256)
	delivery.LastRedriveReasonRef = strings.TrimSpace(delivery.LastRedriveReasonRef)
	return delivery
}

func (delivery WorkflowExternalCallbackDelivery) Validate() error {
	delivery = delivery.Normalized()
	if delivery.TenantID == "" ||
		delivery.WorkflowID == "" ||
		delivery.DeliveryID == "" ||
		delivery.DeliveryPlanSha256 == "" ||
		delivery.StepID == "" ||
		delivery.WorkflowType == "" ||
		delivery.TargetService == "" ||
		delivery.TargetOperation == "" ||
		delivery.TargetRefHash == "" ||
		delivery.PayloadSchemaVersion == "" ||
		delivery.PayloadRefHash == "" ||
		delivery.ApprovalPolicyRef == "" ||
		delivery.DecisionPolicyRef == "" ||
		delivery.CallbackProviderRef == "" ||
		delivery.CallbackEndpointRef == "" ||
		delivery.DeliveryQueueRef == "" ||
		delivery.RetryPolicyRef == "" ||
		delivery.BackoffPolicyRef == "" ||
		delivery.CallbackTimeoutPolicyRef == "" ||
		delivery.CallbackPayloadSchemaVersion == "" ||
		delivery.CallbackPayloadRefHash == "" {
		return NewInvalidArgument("workflow external callback delivery is incomplete")
	}
	if !isAllowedWorkflowType(delivery.WorkflowType) {
		return NewInvalidArgument("workflow external callback delivery workflow type is unsupported")
	}
	if !isAllowedExternalCallbackDeliveryStatus(delivery.Status) {
		return NewInvalidArgument("workflow external callback delivery status is unsupported")
	}
	if delivery.MaxAttempts < 1 || delivery.MaxAttempts > 10 {
		return NewInvalidArgument("workflow external callback max attempts must be between 1 and 10")
	}
	for name, value := range map[string]string{
		"delivery_id":                     delivery.DeliveryID,
		"delivery_plan_sha256":            delivery.DeliveryPlanSha256,
		"source_decision_manifest_sha256": delivery.SourceDecisionManifestSha256,
		"target_ref_hash":                 delivery.TargetRefHash,
		"payload_ref_hash":                delivery.PayloadRefHash,
		"approval_policy_ref":             delivery.ApprovalPolicyRef,
		"decision_policy_ref":             delivery.DecisionPolicyRef,
		"callback_provider_ref":           delivery.CallbackProviderRef,
		"callback_endpoint_ref":           delivery.CallbackEndpointRef,
		"delivery_queue_ref":              delivery.DeliveryQueueRef,
		"retry_policy_ref":                delivery.RetryPolicyRef,
		"backoff_policy_ref":              delivery.BackoffPolicyRef,
		"callback_timeout_policy_ref":     delivery.CallbackTimeoutPolicyRef,
		"callback_payload_ref_hash":       delivery.CallbackPayloadRefHash,
		"last_failure_class":              delivery.LastFailureClass,
		"last_delivery_result_ref":        delivery.LastDeliveryResultRef,
		"last_redrive_plan_sha256":        delivery.LastRedrivePlanSha256,
		"last_redrive_reason_ref":         delivery.LastRedriveReasonRef,
	} {
		if looksSensitive(value) {
			return NewInvalidArgument(name + " must be a low-sensitive ref")
		}
	}
	return nil
}

func (plan WorkflowExternalCallbackRedrivePlan) Normalized() WorkflowExternalCallbackRedrivePlan {
	plan.TenantID = TenantID(strings.TrimSpace(string(plan.TenantID)))
	plan.RedrivePlanID = strings.TrimSpace(plan.RedrivePlanID)
	plan.RedrivePlanSha256 = strings.TrimSpace(plan.RedrivePlanSha256)
	plan.SourceDeliveryStatusSha256 = strings.TrimSpace(plan.SourceDeliveryStatusSha256)
	plan.SourceDeliveryPlanSha256 = strings.TrimSpace(plan.SourceDeliveryPlanSha256)
	plan.WorkflowID = strings.TrimSpace(plan.WorkflowID)
	plan.StepID = strings.TrimSpace(plan.StepID)
	plan.WorkflowType = strings.ToUpper(strings.TrimSpace(plan.WorkflowType))
	plan.TargetService = strings.TrimSpace(plan.TargetService)
	plan.TargetOperation = strings.TrimSpace(plan.TargetOperation)
	plan.TargetRefHash = strings.TrimSpace(plan.TargetRefHash)
	plan.PayloadSchemaVersion = strings.TrimSpace(plan.PayloadSchemaVersion)
	plan.PayloadRefHash = strings.TrimSpace(plan.PayloadRefHash)
	plan.ApprovalPolicyRef = strings.TrimSpace(plan.ApprovalPolicyRef)
	plan.DecisionPolicyRef = strings.TrimSpace(plan.DecisionPolicyRef)
	plan.DeliveryStatus = strings.ToUpper(strings.TrimSpace(plan.DeliveryStatus))
	plan.DeliveryAttemptRef = strings.TrimSpace(plan.DeliveryAttemptRef)
	plan.FailureClassRef = strings.TrimSpace(plan.FailureClassRef)
	plan.RedrivePolicyRef = strings.TrimSpace(plan.RedrivePolicyRef)
	plan.RedriveQueueRef = strings.TrimSpace(plan.RedriveQueueRef)
	plan.RedriveReasonRef = strings.TrimSpace(plan.RedriveReasonRef)
	plan.OperatorReviewRef = strings.TrimSpace(plan.OperatorReviewRef)
	return plan
}

func (plan WorkflowExternalCallbackRedrivePlan) Validate() error {
	plan = plan.Normalized()
	if plan.TenantID == "" ||
		plan.RedrivePlanID == "" ||
		plan.RedrivePlanSha256 == "" ||
		plan.SourceDeliveryStatusSha256 == "" ||
		plan.SourceDeliveryPlanSha256 == "" ||
		plan.WorkflowID == "" ||
		plan.StepID == "" ||
		plan.WorkflowType == "" ||
		plan.TargetService == "" ||
		plan.TargetOperation == "" ||
		plan.TargetRefHash == "" ||
		plan.PayloadSchemaVersion == "" ||
		plan.PayloadRefHash == "" ||
		plan.ApprovalPolicyRef == "" ||
		plan.DecisionPolicyRef == "" ||
		plan.DeliveryStatus == "" ||
		plan.DeliveryAttemptRef == "" ||
		plan.FailureClassRef == "" ||
		plan.RedrivePolicyRef == "" ||
		plan.RedriveQueueRef == "" ||
		plan.RedriveReasonRef == "" {
		return NewInvalidArgument("workflow external callback redrive plan is incomplete")
	}
	if !isAllowedWorkflowType(plan.WorkflowType) {
		return NewInvalidArgument("workflow external callback redrive workflow type is unsupported")
	}
	if plan.DeliveryStatus != WorkflowExternalCallbackDeliveryStatusRetryPending &&
		plan.DeliveryStatus != WorkflowExternalCallbackDeliveryStatusDLQ {
		return NewInvalidArgument("workflow external callback redrive source status is unsupported")
	}
	if plan.AttemptNumber < 1 || plan.MaxAttempts < 1 || plan.AttemptNumber > plan.MaxAttempts || plan.MaxAttempts > 10 {
		return NewInvalidArgument("workflow external callback redrive attempts are invalid")
	}
	for name, value := range map[string]string{
		"redrive_plan_id":               plan.RedrivePlanID,
		"redrive_plan_sha256":           plan.RedrivePlanSha256,
		"source_delivery_status_sha256": plan.SourceDeliveryStatusSha256,
		"source_delivery_plan_sha256":   plan.SourceDeliveryPlanSha256,
		"target_ref_hash":               plan.TargetRefHash,
		"payload_ref_hash":              plan.PayloadRefHash,
		"approval_policy_ref":           plan.ApprovalPolicyRef,
		"decision_policy_ref":           plan.DecisionPolicyRef,
		"delivery_attempt_ref":          plan.DeliveryAttemptRef,
		"failure_class_ref":             plan.FailureClassRef,
		"redrive_policy_ref":            plan.RedrivePolicyRef,
		"redrive_queue_ref":             plan.RedriveQueueRef,
		"redrive_reason_ref":            plan.RedriveReasonRef,
		"operator_review_ref":           plan.OperatorReviewRef,
	} {
		if looksSensitive(value) {
			return NewInvalidArgument(name + " must be a low-sensitive ref")
		}
	}
	return nil
}

func isAllowedWorkflowType(value string) bool {
	return value == WorkflowTypeActionApproval ||
		value == WorkflowTypeRepairApproval ||
		value == WorkflowTypeAdminOperation ||
		value == WorkflowTypeCompensationRequest
}

func isAllowedRiskLevel(value string) bool {
	switch value {
	case RiskLevelLow, RiskLevelMedium, RiskLevelHigh, RiskLevelCritical:
		return true
	default:
		return false
	}
}

func isAllowedWorkflowStatus(value string) bool {
	switch value {
	case WorkflowStatusWaitingDecision,
		WorkflowStatusApproved,
		WorkflowStatusRejected,
		WorkflowStatusCanceled,
		WorkflowStatusTimedOut,
		WorkflowStatusCompensationPending,
		WorkflowStatusCompensated:
		return true
	default:
		return false
	}
}

func isAllowedExternalCallbackDeliveryStatus(value string) bool {
	switch value {
	case WorkflowExternalCallbackDeliveryStatusPending,
		WorkflowExternalCallbackDeliveryStatusInFlight,
		WorkflowExternalCallbackDeliveryStatusDelivered,
		WorkflowExternalCallbackDeliveryStatusRetryPending,
		WorkflowExternalCallbackDeliveryStatusDLQ:
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
