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
	}, nil
}
