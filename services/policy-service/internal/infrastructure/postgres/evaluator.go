package postgres

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/policy-service/internal/types"
)

type fallbackEvaluator interface {
	DecideMessageAction(context.Context, types.CheckMessageActionCommand) (types.MessageActionDecision, error)
}

type MessagePolicyEvaluator struct {
	pool     *pgxpool.Pool
	fallback fallbackEvaluator
}

func NewMessagePolicyEvaluator(pool *pgxpool.Pool, fallback fallbackEvaluator) MessagePolicyEvaluator {
	return MessagePolicyEvaluator{pool: pool, fallback: fallback}
}

func (e MessagePolicyEvaluator) DecideMessageAction(
	ctx context.Context,
	command types.CheckMessageActionCommand,
) (types.MessageActionDecision, error) {
	if e.pool == nil {
		return e.fallbackDecision(ctx, command)
	}

	decision, found, err := e.lookupRule(ctx, command)
	if err != nil {
		return types.MessageActionDecision{}, err
	}
	if found {
		return decision, nil
	}
	return e.fallbackDecision(ctx, command)
}

func (e MessagePolicyEvaluator) fallbackDecision(
	ctx context.Context,
	command types.CheckMessageActionCommand,
) (types.MessageActionDecision, error) {
	if e.fallback == nil {
		return types.MessageActionDecision{}, types.NewDependencyUnavailable("policy fallback is not configured")
	}
	return e.fallback.DecideMessageAction(ctx, command)
}

func (e MessagePolicyEvaluator) lookupRule(
	ctx context.Context,
	command types.CheckMessageActionCommand,
) (types.MessageActionDecision, bool, error) {
	decision := types.MessageActionDecision{
		TenantID:       command.AuthContext.TenantID,
		UserID:         command.AuthContext.UserID,
		ConversationID: command.ConversationID,
		MessageID:      command.MessageID,
		Action:         command.Action,
	}
	err := e.pool.QueryRow(ctx, `
SELECT allowed, permission_version, classification, reason
FROM policy_message_action_rules
WHERE tenant_id = $1
  AND user_id = $2
  AND conversation_id = $3
  AND action = $4
`, command.AuthContext.TenantID, command.AuthContext.UserID, command.ConversationID, command.Action).Scan(
		&decision.Allowed,
		&decision.PermissionVersion,
		&decision.Classification,
		&decision.Reason,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return types.MessageActionDecision{}, false, nil
	}
	if err != nil {
		return types.MessageActionDecision{}, false, types.NewDependencyUnavailable("policy rule lookup failed")
	}
	decision.Classification = strings.TrimSpace(decision.Classification)
	decision.Reason = strings.TrimSpace(decision.Reason)
	if decision.PermissionVersion <= 0 || decision.Classification == "" {
		return types.MessageActionDecision{}, false, types.NewDependencyUnavailable("policy rule is invalid")
	}
	if !decision.Allowed && decision.Reason == "" {
		decision.Reason = "policy denied"
	}
	return decision, true, nil
}
