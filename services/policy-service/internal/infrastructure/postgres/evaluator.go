package postgres

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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

	blocked, edgeVersion, err := e.lookupContactBlock(ctx, command)
	if err != nil {
		return types.MessageActionDecision{}, err
	}
	if blocked {
		return types.MessageActionDecision{
			TenantID:          command.AuthContext.TenantID,
			UserID:            command.AuthContext.UserID,
			ConversationID:    command.ConversationID,
			MessageID:         command.MessageID,
			Action:            command.Action,
			Allowed:           false,
			PermissionVersion: edgeVersion,
			Classification:    "CONTACT_BLOCKED",
			Reason:            "contact blocked",
		}, nil
	}

	decision, found, err := e.lookupRule(ctx, command)
	if err != nil {
		return types.MessageActionDecision{}, err
	}
	if found {
		return decision, nil
	}
	decision, found, err = e.lookupTenantRule(ctx, command)
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

func (e MessagePolicyEvaluator) lookupTenantRule(
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
FROM policy_tenant_message_action_rules
WHERE tenant_id = $1
  AND action = $2
`, command.AuthContext.TenantID, command.Action).Scan(
		&decision.Allowed,
		&decision.PermissionVersion,
		&decision.Classification,
		&decision.Reason,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return types.MessageActionDecision{}, false, nil
	}
	if isUndefinedTable(err) {
		return types.MessageActionDecision{}, false, nil
	}
	if err != nil {
		return types.MessageActionDecision{}, false, types.NewDependencyUnavailable("policy tenant rule lookup failed")
	}
	decision.Classification = strings.TrimSpace(decision.Classification)
	decision.Reason = strings.TrimSpace(decision.Reason)
	if decision.PermissionVersion <= 0 || decision.Classification == "" {
		return types.MessageActionDecision{}, false, types.NewDependencyUnavailable("policy tenant rule is invalid")
	}
	if !decision.Allowed && decision.Reason == "" {
		decision.Reason = "policy denied"
	}
	return decision, true, nil
}

func isUndefinedTable(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "42P01"
}

func (e MessagePolicyEvaluator) lookupContactBlock(
	ctx context.Context,
	command types.CheckMessageActionCommand,
) (bool, int64, error) {
	if command.Action != types.MessageActionSend || command.DirectPeerUserID == "" {
		return false, 0, nil
	}
	var status string
	var edgeVersion int64
	err := e.pool.QueryRow(ctx, `
SELECT status, edge_version
FROM policy_contact_edges_projection
WHERE tenant_id = $1
  AND status = $2
  AND (
      (owner_user_id = $3 AND contact_user_id = $4)
      OR (owner_user_id = $4 AND contact_user_id = $3)
  )
ORDER BY edge_version DESC
LIMIT 1
`, command.AuthContext.TenantID, types.ContactEdgeStatusBlocked, command.DirectPeerUserID, command.AuthContext.UserID).Scan(
		&status,
		&edgeVersion,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, 0, nil
	}
	if err != nil {
		return false, 0, types.NewDependencyUnavailable("contact policy lookup failed")
	}
	if status != types.ContactEdgeStatusBlocked {
		return false, 0, nil
	}
	if edgeVersion <= 0 {
		return false, 0, types.NewDependencyUnavailable("contact policy projection is invalid")
	}
	return true, edgeVersion, nil
}
