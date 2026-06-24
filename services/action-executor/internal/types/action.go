package types

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	ToolActionCall    = "CALL"
	ToolActionApprove = "APPROVE"
	ToolActionExecute = "EXECUTE"

	SkillStatusActive   = "ACTIVE"
	SkillStatusDisabled = "DISABLED"

	LocalSafeEchoToolName             = "nexusim.local.echo"
	ConversationNoteCreateToolName    = "conversation.note.create"
	ConversationProfileUpdateToolName = "conversation.profile.update"

	ExecutionStatusRecorded = "RECORDED"
	ExecutionStatusBlocked  = "BLOCKED"
	ExecutionStatusFailed   = "FAILED"

	ResultStatusNotExecuted = "NOT_EXECUTED"
	ResultStatusBlocked     = "BLOCKED"
	ResultStatusSucceeded   = "SUCCEEDED"
	ResultStatusFailed      = "FAILED"

	ProviderFailureStatusRetryPending = "RETRY_PENDING"
	ProviderFailureStatusDLQ          = "DLQ"

	maxInputJSONBytes = 64 * 1024
)

type SkillDefinition struct {
	TenantID         TenantID
	SkillID          string
	Status           string
	ToolName         string
	AllowedActions   []string
	RiskLevel        string
	RequiresApproval bool
	AuditEventType   string
	OwnerService     string
}

type ExecuteApprovedActionCommand struct {
	AuthContext     AuthContext
	ProposalID      string
	ApprovalID      string
	PreparedAuditID string
	SkillID         string
	ToolName        string
	Action          string
	ResourceType    string
	ResourceID      string
	RiskLevel       string
	Intent          string
	InputJSON       string
	IdempotencyKey  string
}

func (command ExecuteApprovedActionCommand) Validate() error {
	if !command.AuthContext.IsValid() {
		return fmt.Errorf("%w: auth_context is required", ErrInvalidArgument)
	}
	if strings.TrimSpace(command.ProposalID) == "" {
		return fmt.Errorf("%w: proposal_id is required", ErrInvalidArgument)
	}
	if strings.TrimSpace(command.ApprovalID) == "" {
		return fmt.Errorf("%w: approval_id is required", ErrInvalidArgument)
	}
	if strings.TrimSpace(command.PreparedAuditID) == "" {
		return fmt.Errorf("%w: prepared_audit_id is required", ErrInvalidArgument)
	}
	if strings.TrimSpace(command.SkillID) == "" {
		return fmt.Errorf("%w: skill_id is required", ErrInvalidArgument)
	}
	if strings.TrimSpace(command.ToolName) == "" {
		return fmt.Errorf("%w: tool_name is required", ErrInvalidArgument)
	}
	if normalizeToolAction(command.Action) != ToolActionExecute {
		return fmt.Errorf("%w: action must be EXECUTE", ErrInvalidArgument)
	}
	if len(command.InputJSON) > maxInputJSONBytes {
		return fmt.Errorf("%w: input_json is too large", ErrInvalidArgument)
	}
	if strings.TrimSpace(command.InputJSON) != "" && !json.Valid([]byte(command.InputJSON)) {
		return fmt.Errorf("%w: input_json must be valid JSON", ErrInvalidArgument)
	}
	return nil
}

func (command ExecuteApprovedActionCommand) Normalized() ExecuteApprovedActionCommand {
	command.ProposalID = strings.TrimSpace(command.ProposalID)
	command.ApprovalID = strings.TrimSpace(command.ApprovalID)
	command.PreparedAuditID = strings.TrimSpace(command.PreparedAuditID)
	command.SkillID = strings.TrimSpace(command.SkillID)
	command.ToolName = strings.TrimSpace(command.ToolName)
	command.Action = normalizeToolAction(command.Action)
	command.ResourceType = strings.TrimSpace(command.ResourceType)
	command.ResourceID = strings.TrimSpace(command.ResourceID)
	command.RiskLevel = strings.ToUpper(strings.TrimSpace(command.RiskLevel))
	command.Intent = strings.TrimSpace(command.Intent)
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	command.InputJSON = strings.TrimSpace(command.InputJSON)
	return command
}

func (command ExecuteApprovedActionCommand) InputSHA256() string {
	if strings.TrimSpace(command.InputJSON) == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(command.InputJSON))
	return hex.EncodeToString(sum[:])
}

type CheckToolActionCommand struct {
	AuthContext  AuthContext
	ToolName     string
	Action       string
	ResourceType string
	ResourceID   string
	RiskLevel    string
	Intent       string
}

type ToolPolicyDecision struct {
	TenantID          TenantID
	UserID            UserID
	ToolName          string
	Action            string
	ResourceType      string
	ResourceID        string
	RiskLevel         string
	Allowed           bool
	RequiresApproval  bool
	PermissionVersion int64
	Classification    string
	Reason            string
	DecisionSource    string
}

type ActionRateLimitCommand struct {
	AuthContext    AuthContext
	ToolName       string
	Action         string
	ResourceType   string
	ResourceID     string
	RiskLevel      string
	Intent         string
	IdempotencyKey string
}

type ActionRateLimitDecision struct {
	Allowed        bool
	LimitKey       string
	Classification string
	Reason         string
	DecisionSource string
}

type VerifyApprovedProposalCommand struct {
	AuthContext     AuthContext
	ProposalID      string
	ApprovalID      string
	PreparedAuditID string
	SkillID         string
	ToolName        string
	ResourceType    string
	ResourceID      string
}

type ApprovedProposal struct {
	ProposalID      string
	ApprovalID      string
	Status          string
	UserID          UserID
	ConversationID  string
	SkillID         string
	PreparedAuditID string
	ToolName        string
	ResourceType    string
	ResourceID      string
	RiskLevel       string
	ApprovedAt      time.Time
}

type ExecutionAudit struct {
	TenantID          TenantID
	ExecutionID       string
	ProposalID        string
	ApprovalID        string
	PreparedAuditID   string
	UserID            UserID
	DeviceID          string
	SessionID         string
	TraceID           string
	RequestID         string
	SkillID           string
	ToolName          string
	Action            string
	ResourceType      string
	ResourceID        string
	RiskLevel         string
	Intent            string
	IdempotencyKey    string
	InputSHA256       string
	Allowed           bool
	RequiresApproval  bool
	PermissionVersion int64
	Classification    string
	Reason            string
	DecisionSource    string
	Status            string
	Executed          bool
	OutputSHA256      string
	CreatedAt         time.Time
}

type ToolResultProjection struct {
	TenantID        TenantID
	ResultID        string
	ExecutionID     string
	ProposalID      string
	ApprovalID      string
	PreparedAuditID string
	UserID          UserID
	SkillID         string
	ToolName        string
	ResourceType    string
	ResourceID      string
	Status          string
	Executed        bool
	ResultRef       string
	OutputSHA256    string
	CreatedAt       time.Time
}

type ProviderFailureProjection struct {
	TenantID          TenantID
	ProviderFailureID string
	ExecutionID       string
	ResultID          string
	ProposalID        string
	ApprovalID        string
	PreparedAuditID   string
	UserID            UserID
	SkillID           string
	ToolName          string
	ResourceType      string
	ResourceID        string
	Classification    string
	Status            string
	Retryable         bool
	RetryCount        int
	NextRetryAt       time.Time
	DeadLetteredAt    time.Time
	FailureRef        string
	CreatedAt         time.Time
}

type ProviderFailureRetryStats struct {
	Fetched      int
	Rescheduled  int
	DeadLettered int
}

type ExecuteApprovedActionResult struct {
	TenantID          TenantID
	UserID            UserID
	ExecutionID       string
	ProposalID        string
	ApprovalID        string
	PreparedAuditID   string
	SkillID           string
	ToolName          string
	Action            string
	ResourceType      string
	ResourceID        string
	RiskLevel         string
	Status            string
	Allowed           bool
	RequiresApproval  bool
	PermissionVersion int64
	Classification    string
	Reason            string
	DecisionSource    string
	Executed          bool
	OutputJSON        string
	ResultID          string
	ResultStatus      string
	ResultRef         string
}

type ToolExecutionCommand struct {
	AuthContext     AuthContext
	Skill           SkillDefinition
	ProposalID      string
	ApprovalID      string
	PreparedAuditID string
	ToolName        string
	Action          string
	ResourceType    string
	ResourceID      string
	RiskLevel       string
	Intent          string
	IdempotencyKey  string
	// InputJSON is only available to first-party business adapters. External providers must receive InputSHA256 only.
	InputJSON   string
	InputSHA256 string
}

type ToolExecutionResult struct {
	Executed   bool
	OutputJSON string
}

func normalizeToolAction(action string) string {
	switch strings.ToUpper(strings.TrimSpace(action)) {
	case ToolActionCall:
		return ToolActionCall
	case ToolActionApprove:
		return ToolActionApprove
	case ToolActionExecute:
		return ToolActionExecute
	default:
		return ""
	}
}

func ToolActionAllowed(allowed []string, requested string) bool {
	requested = normalizeToolAction(requested)
	for _, action := range allowed {
		if normalizeToolAction(action) == requested {
			return true
		}
	}
	return false
}
