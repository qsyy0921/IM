package domain

import (
	"context"
	"strings"

	"github.com/qsyy0921/IM/services/policy-service/internal/types"
)

type StaticMessagePolicy struct {
	Allowed           bool
	PermissionVersion int64
	Classification    string
	Reason            string
}

type StaticToolPolicy struct {
	Allowed           bool
	RequiresApproval  bool
	PermissionVersion int64
	Classification    string
	Reason            string
}

func (p StaticMessagePolicy) DecideMessageAction(
	_ context.Context,
	command types.CheckMessageActionCommand,
) (types.MessageActionDecision, error) {
	permissionVersion := p.PermissionVersion
	if permissionVersion <= 0 {
		permissionVersion = command.ConversationPermissionVersion
	}
	if permissionVersion <= 0 {
		permissionVersion = 1
	}
	classification := strings.TrimSpace(p.Classification)
	if classification == "" {
		classification = "INTERNAL"
	}
	reason := strings.TrimSpace(p.Reason)
	if !p.Allowed && reason == "" {
		reason = "policy denied"
	}
	return types.MessageActionDecision{
		TenantID:          command.AuthContext.TenantID,
		UserID:            command.AuthContext.UserID,
		ConversationID:    command.ConversationID,
		MessageID:         command.MessageID,
		Action:            command.Action,
		Allowed:           p.Allowed,
		PermissionVersion: permissionVersion,
		Classification:    classification,
		Reason:            reason,
		DecisionSource:    types.PolicyDecisionSourceFallback,
	}, nil
}

func (p StaticToolPolicy) DecideToolAction(
	_ context.Context,
	command types.CheckToolActionCommand,
) (types.ToolActionDecision, error) {
	permissionVersion := p.PermissionVersion
	if permissionVersion <= 0 {
		permissionVersion = 1
	}
	classification := strings.TrimSpace(p.Classification)
	if classification == "" {
		if p.Allowed {
			classification = "TOOL_STATIC_ALLOW"
		} else {
			classification = "TOOL_STATIC_DENY"
		}
	}
	reason := strings.TrimSpace(p.Reason)
	if !p.Allowed && reason == "" {
		reason = "tool policy denied"
	}
	if p.RequiresApproval && reason == "" {
		reason = "tool action requires approval"
	}
	return types.ToolActionDecision{
		TenantID:          command.AuthContext.TenantID,
		UserID:            command.AuthContext.UserID,
		ToolName:          strings.TrimSpace(command.ToolName),
		Action:            command.Action,
		ResourceType:      strings.TrimSpace(command.ResourceType),
		ResourceID:        strings.TrimSpace(command.ResourceID),
		RiskLevel:         normalizedRiskLevel(command.RiskLevel),
		Allowed:           p.Allowed,
		RequiresApproval:  p.RequiresApproval,
		PermissionVersion: permissionVersion,
		Classification:    classification,
		Reason:            reason,
		DecisionSource:    types.PolicyDecisionSourceFallback,
	}, nil
}

func normalizedRiskLevel(risk types.ToolRiskLevel) types.ToolRiskLevel {
	if risk == "" {
		return types.ToolRiskLevelLow
	}
	return risk
}
