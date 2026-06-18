package types

import "strings"

type AuthContext struct {
	TenantID  TenantID
	UserID    UserID
	DeviceID  DeviceID
	SessionID string
	TraceID   string
	RequestID string
}

type MessageAction string

const (
	MessageActionSend   MessageAction = "SEND"
	MessageActionEdit   MessageAction = "EDIT"
	MessageActionRevoke MessageAction = "REVOKE"
	MessageActionDelete MessageAction = "DELETE"
)

type PolicyDecisionSource string

const (
	PolicyDecisionSourceFallback          PolicyDecisionSource = "FALLBACK"
	PolicyDecisionSourceExactRule         PolicyDecisionSource = "EXACT_RULE"
	PolicyDecisionSourceTenantRule        PolicyDecisionSource = "TENANT_RULE"
	PolicyDecisionSourceUserRestriction   PolicyDecisionSource = "USER_RESTRICTION"
	PolicyDecisionSourceTenantQuota       PolicyDecisionSource = "TENANT_QUOTA"
	PolicyDecisionSourceReBACRelation     PolicyDecisionSource = "REBAC_RELATION"
	PolicyDecisionSourceConversationRole  PolicyDecisionSource = "CONVERSATION_ROLE"
	PolicyDecisionSourceContactProjection PolicyDecisionSource = "CONTACT_PROJECTION"
	PolicyDecisionSourceOwnershipOverride PolicyDecisionSource = "OWNERSHIP_OVERRIDE"
	PolicyDecisionSourceMessageOwnership  PolicyDecisionSource = "MESSAGE_OWNERSHIP"
	PolicyDecisionSourceContentModeration PolicyDecisionSource = "CONTENT_MODERATION"
	PolicyDecisionSourceToolRule          PolicyDecisionSource = "TOOL_RULE"
)

type ToolAction string

const (
	ToolActionCall    ToolAction = "CALL"
	ToolActionApprove ToolAction = "APPROVE"
	ToolActionExecute ToolAction = "EXECUTE"
)

type ToolRiskLevel string

const (
	ToolRiskLevelLow      ToolRiskLevel = "LOW"
	ToolRiskLevelMedium   ToolRiskLevel = "MEDIUM"
	ToolRiskLevelHigh     ToolRiskLevel = "HIGH"
	ToolRiskLevelCritical ToolRiskLevel = "CRITICAL"
)

type ReBACRelationType string

const (
	ReBACRelationDirectContactActive      ReBACRelationType = "DIRECT_CONTACT_ACTIVE"
	ReBACRelationConversationMemberActive ReBACRelationType = "CONVERSATION_MEMBER_ACTIVE"
)

type ReBACConversationScope string

const (
	ReBACConversationScopeAny    ReBACConversationScope = "ANY"
	ReBACConversationScopeDirect ReBACConversationScope = "DIRECT"
	ReBACConversationScopeGroup  ReBACConversationScope = "GROUP"
)

type CheckMessageActionCommand struct {
	AuthContext                   AuthContext
	ConversationID                ConversationID
	Action                        MessageAction
	MessageID                     MessageID
	DirectPeerUserID              UserID
	MessageSenderUserID           UserID
	MessageText                   string
	ConversationPermissionVersion int64
}

func (c CheckMessageActionCommand) Validate() error {
	if c.AuthContext.TenantID == "" || c.AuthContext.UserID == "" || c.AuthContext.DeviceID == "" {
		return NewInvalidArgument("auth context is required")
	}
	if c.ConversationID == "" {
		return NewInvalidArgument("conversation_id is required")
	}
	switch c.Action {
	case MessageActionSend:
	case MessageActionEdit, MessageActionRevoke, MessageActionDelete:
		if c.MessageID == "" {
			return NewInvalidArgument("message_id is required")
		}
	default:
		return NewInvalidArgument("message action is required")
	}
	if c.DirectPeerUserID != "" && c.DirectPeerUserID == c.AuthContext.UserID {
		return NewInvalidArgument("direct_peer_user_id must not equal auth user")
	}
	return nil
}

type MessageActionDecision struct {
	TenantID          TenantID
	UserID            UserID
	ConversationID    ConversationID
	MessageID         MessageID
	Action            MessageAction
	Allowed           bool
	PermissionVersion int64
	Classification    string
	Reason            string
	OwnershipOverride bool
	DecisionSource    PolicyDecisionSource
}

type CheckToolActionCommand struct {
	AuthContext  AuthContext
	ToolName     string
	Action       ToolAction
	ResourceType string
	ResourceID   string
	RiskLevel    ToolRiskLevel
	Intent       string
}

func (c CheckToolActionCommand) Validate() error {
	if c.AuthContext.TenantID == "" || c.AuthContext.UserID == "" || c.AuthContext.DeviceID == "" {
		return NewInvalidArgument("auth context is required")
	}
	if strings.TrimSpace(c.ToolName) == "" {
		return NewInvalidArgument("tool_name is required")
	}
	if len(c.ToolName) > 128 {
		return NewInvalidArgument("tool_name is too long")
	}
	if strings.TrimSpace(c.ResourceType) == "" {
		return NewInvalidArgument("resource_type is required")
	}
	if len(c.ResourceType) > 64 {
		return NewInvalidArgument("resource_type is too long")
	}
	if len(c.ResourceID) > 256 {
		return NewInvalidArgument("resource_id is too long")
	}
	if len(c.Intent) > 512 {
		return NewInvalidArgument("intent is too long")
	}
	switch c.Action {
	case ToolActionCall, ToolActionApprove, ToolActionExecute:
	default:
		return NewInvalidArgument("tool action is required")
	}
	if c.RiskLevel == "" {
		return nil
	}
	switch c.RiskLevel {
	case ToolRiskLevelLow, ToolRiskLevelMedium, ToolRiskLevelHigh, ToolRiskLevelCritical:
		return nil
	default:
		return NewInvalidArgument("risk_level is invalid")
	}
}

type ToolActionDecision struct {
	TenantID          TenantID
	UserID            UserID
	ToolName          string
	Action            ToolAction
	ResourceType      string
	ResourceID        string
	RiskLevel         ToolRiskLevel
	Allowed           bool
	RequiresApproval  bool
	PermissionVersion int64
	Classification    string
	Reason            string
	DecisionSource    PolicyDecisionSource
}
