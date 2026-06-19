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

	ToolAuditStatusAllowed = "ALLOWED"
	ToolAuditStatusBlocked = "BLOCKED"
	ToolAuditStatusFailed  = "FAILED"

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

type PrepareToolCallCommand struct {
	AuthContext    AuthContext
	SkillID        string
	ToolName       string
	Action         string
	ResourceType   string
	ResourceID     string
	RiskLevel      string
	Intent         string
	InputJSON      string
	IdempotencyKey string
}

func (command PrepareToolCallCommand) Validate() error {
	if !command.AuthContext.IsValid() {
		return fmt.Errorf("%w: auth_context is required", ErrInvalidArgument)
	}
	if strings.TrimSpace(command.SkillID) == "" {
		return fmt.Errorf("%w: skill_id is required", ErrInvalidArgument)
	}
	if strings.TrimSpace(command.ToolName) == "" {
		return fmt.Errorf("%w: tool_name is required", ErrInvalidArgument)
	}
	if !isValidToolAction(command.Action) {
		return fmt.Errorf("%w: action is invalid", ErrInvalidArgument)
	}
	if len(command.InputJSON) > maxInputJSONBytes {
		return fmt.Errorf("%w: input_json is too large", ErrInvalidArgument)
	}
	if strings.TrimSpace(command.InputJSON) != "" && !json.Valid([]byte(command.InputJSON)) {
		return fmt.Errorf("%w: input_json must be valid JSON", ErrInvalidArgument)
	}
	return nil
}

func (command PrepareToolCallCommand) Normalized() PrepareToolCallCommand {
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

func (command PrepareToolCallCommand) InputSHA256() string {
	if strings.TrimSpace(command.InputJSON) == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(command.InputJSON))
	return hex.EncodeToString(sum[:])
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

type CheckToolActionCommand struct {
	AuthContext  AuthContext
	ToolName     string
	Action       string
	ResourceType string
	ResourceID   string
	RiskLevel    string
	Intent       string
}

type ToolCallAudit struct {
	TenantID          TenantID
	AuditID           string
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
	CreatedAt         time.Time
}

type PrepareToolCallResult struct {
	TenantID          TenantID
	UserID            UserID
	SkillID           string
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
	AuditID           string
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

func isValidToolAction(action string) bool {
	switch normalizeToolAction(action) {
	case ToolActionCall, ToolActionApprove, ToolActionExecute:
		return true
	default:
		return false
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
