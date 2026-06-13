package postgres

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/qsyy0921/IM/services/policy-service/internal/types"
)

type ownershipOverrideRule struct {
	MinRole        string
	Classification string
	Reason         string
}

func (e MessagePolicyEvaluator) DecideMessageOwnershipOverride(
	ctx context.Context,
	command types.CheckMessageActionCommand,
) (types.MessageActionDecision, bool, error) {
	if e.pool == nil {
		return types.MessageActionDecision{}, false, nil
	}
	var rule ownershipOverrideRule
	err := e.pool.QueryRow(ctx, `
SELECT min_role, classification, reason
FROM policy_message_ownership_override_rules
WHERE tenant_id = $1
  AND action = $2
`, command.AuthContext.TenantID, command.Action).Scan(
		&rule.MinRole,
		&rule.Classification,
		&rule.Reason,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return types.MessageActionDecision{}, false, nil
	}
	if isUndefinedTable(err) {
		return types.MessageActionDecision{}, false, nil
	}
	if err != nil {
		return types.MessageActionDecision{}, false, types.NewDependencyUnavailable("policy ownership override rule lookup failed")
	}
	rule.MinRole = strings.TrimSpace(rule.MinRole)
	rule.Classification = strings.TrimSpace(rule.Classification)
	rule.Reason = strings.TrimSpace(rule.Reason)
	if rule.Classification == "" || roleRank(rule.MinRole) < roleRank(types.ConversationMemberRoleAdmin) {
		return types.MessageActionDecision{}, false, types.NewDependencyUnavailable("policy ownership override rule is invalid")
	}
	if command.ConversationPermissionVersion <= 0 {
		return types.MessageActionDecision{}, false, types.NewDependencyUnavailable("policy conversation permission version is required")
	}

	memberRole, memberStatus, permissionVersion, found, err := e.lookupProjectedMember(ctx, command)
	if err != nil {
		return types.MessageActionDecision{}, false, err
	}
	if !found {
		return types.MessageActionDecision{}, false, types.NewDependencyUnavailable("policy conversation member projection is missing")
	}
	if permissionVersion != command.ConversationPermissionVersion {
		return types.MessageActionDecision{}, false, types.NewDependencyUnavailable("policy conversation member projection is stale")
	}
	if memberStatus != types.ConversationMemberStatusActive || roleRank(memberRole) < roleRank(rule.MinRole) {
		return types.MessageActionDecision{}, false, nil
	}
	return types.MessageActionDecision{
		TenantID:          command.AuthContext.TenantID,
		UserID:            command.AuthContext.UserID,
		ConversationID:    command.ConversationID,
		MessageID:         command.MessageID,
		Action:            command.Action,
		Allowed:           true,
		PermissionVersion: permissionVersion,
		Classification:    rule.Classification,
		Reason:            rule.Reason,
		OwnershipOverride: true,
	}, true, nil
}
