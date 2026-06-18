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
			DecisionSource:    types.PolicyDecisionSourceContactProjection,
		}, nil
	}

	decision, restricted, err := e.lookupUserRestriction(ctx, command)
	if err != nil {
		return types.MessageActionDecision{}, err
	}
	if restricted {
		return decision, nil
	}

	decision, denied, err := e.applyRoleGate(ctx, command)
	if err != nil {
		return types.MessageActionDecision{}, err
	}
	if denied {
		return decision, nil
	}

	decision, quotaExceeded, err := e.applyTenantActionQuota(ctx, command)
	if err != nil {
		return types.MessageActionDecision{}, err
	}
	if quotaExceeded {
		return decision, nil
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

func (e MessagePolicyEvaluator) lookupUserRestriction(
	ctx context.Context,
	command types.CheckMessageActionCommand,
) (types.MessageActionDecision, bool, error) {
	decision := types.MessageActionDecision{
		TenantID:       command.AuthContext.TenantID,
		UserID:         command.AuthContext.UserID,
		ConversationID: command.ConversationID,
		MessageID:      command.MessageID,
		Action:         command.Action,
		Allowed:        false,
		DecisionSource: types.PolicyDecisionSourceUserRestriction,
	}
	err := e.pool.QueryRow(ctx, `
SELECT permission_version, classification, reason
FROM policy_user_message_action_restrictions
WHERE tenant_id = $1
  AND user_id = $2
  AND action = $3
  AND (expires_at IS NULL OR expires_at > now())
`, command.AuthContext.TenantID, command.AuthContext.UserID, command.Action).Scan(
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
		return types.MessageActionDecision{}, false, types.NewDependencyUnavailable("policy user restriction lookup failed")
	}
	decision.Classification = strings.TrimSpace(decision.Classification)
	decision.Reason = strings.TrimSpace(decision.Reason)
	if decision.PermissionVersion <= 0 || decision.Classification == "" {
		return types.MessageActionDecision{}, false, types.NewDependencyUnavailable("policy user restriction is invalid")
	}
	if decision.Reason == "" {
		decision.Reason = "user moderation policy denied"
	}
	return decision, true, nil
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
		DecisionSource: types.PolicyDecisionSourceExactRule,
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
		DecisionSource: types.PolicyDecisionSourceTenantRule,
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

func (e MessagePolicyEvaluator) applyTenantActionQuota(
	ctx context.Context,
	command types.CheckMessageActionCommand,
) (types.MessageActionDecision, bool, error) {
	decision := types.MessageActionDecision{
		TenantID:       command.AuthContext.TenantID,
		UserID:         command.AuthContext.UserID,
		ConversationID: command.ConversationID,
		MessageID:      command.MessageID,
		Action:         command.Action,
		Allowed:        false,
		DecisionSource: types.PolicyDecisionSourceTenantQuota,
	}
	var maxDecisions int
	var recentAllowedDecisions int
	err := e.pool.QueryRow(ctx, `
SELECT
    q.max_decisions,
    q.permission_version,
    q.classification,
    q.reason,
    (
        SELECT COUNT(*)
        FROM policy_decision_audit_outbox audit
        WHERE audit.tenant_id = q.tenant_id
          AND audit.action = q.action
          AND audit.allowed = true
          AND audit.created_at >= now() - (q.window_seconds * interval '1 second')
    ) AS recent_allowed_decisions
FROM policy_tenant_message_action_quotas q
WHERE q.tenant_id = $1
  AND q.action = $2
  AND q.enabled = true
`, command.AuthContext.TenantID, command.Action).Scan(
		&maxDecisions,
		&decision.PermissionVersion,
		&decision.Classification,
		&decision.Reason,
		&recentAllowedDecisions,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return types.MessageActionDecision{}, false, nil
	}
	if isUndefinedTable(err) {
		return types.MessageActionDecision{}, false, nil
	}
	if err != nil {
		return types.MessageActionDecision{}, false, types.NewDependencyUnavailable("policy tenant quota lookup failed")
	}
	decision.Classification = strings.TrimSpace(decision.Classification)
	decision.Reason = strings.TrimSpace(decision.Reason)
	if maxDecisions <= 0 || decision.PermissionVersion <= 0 || decision.Classification == "" {
		return types.MessageActionDecision{}, false, types.NewDependencyUnavailable("policy tenant quota is invalid")
	}
	if recentAllowedDecisions < maxDecisions {
		return types.MessageActionDecision{}, false, nil
	}
	if decision.Reason == "" {
		decision.Reason = "tenant quota exceeded"
	}
	return decision, true, nil
}

type rolePolicyRule struct {
	MinRole        string
	Classification string
	Reason         string
}

func (e MessagePolicyEvaluator) applyRoleGate(
	ctx context.Context,
	command types.CheckMessageActionCommand,
) (types.MessageActionDecision, bool, error) {
	var rule rolePolicyRule
	err := e.pool.QueryRow(ctx, `
SELECT min_role, classification, reason
FROM policy_conversation_role_action_rules
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
		return types.MessageActionDecision{}, false, types.NewDependencyUnavailable("policy role rule lookup failed")
	}
	rule.MinRole = strings.TrimSpace(rule.MinRole)
	rule.Classification = strings.TrimSpace(rule.Classification)
	rule.Reason = strings.TrimSpace(rule.Reason)
	if rule.Classification == "" || roleRank(rule.MinRole) == 0 {
		return types.MessageActionDecision{}, false, types.NewDependencyUnavailable("policy role rule is invalid")
	}
	if command.ConversationPermissionVersion <= 0 {
		return types.MessageActionDecision{}, false, types.NewDependencyUnavailable("policy conversation permission version is required")
	}

	decision := types.MessageActionDecision{
		TenantID:       command.AuthContext.TenantID,
		UserID:         command.AuthContext.UserID,
		ConversationID: command.ConversationID,
		MessageID:      command.MessageID,
		Action:         command.Action,
		Classification: rule.Classification,
		Reason:         rule.Reason,
		DecisionSource: types.PolicyDecisionSourceConversationRole,
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
	decision.PermissionVersion = permissionVersion
	if memberStatus == types.ConversationMemberStatusActive && roleRank(memberRole) >= roleRank(rule.MinRole) {
		return types.MessageActionDecision{}, false, nil
	}
	decision.Allowed = false
	if decision.Reason == "" {
		decision.Reason = "conversation role policy denied"
	}
	return decision, true, nil
}

func (e MessagePolicyEvaluator) lookupProjectedMember(
	ctx context.Context,
	command types.CheckMessageActionCommand,
) (string, string, int64, bool, error) {
	var role string
	var status string
	var permissionVersion int64
	err := e.pool.QueryRow(ctx, `
SELECT role, status, permission_version
FROM policy_conversation_members_projection
WHERE tenant_id = $1
  AND conversation_id = $2
  AND user_id = $3
`, command.AuthContext.TenantID, command.ConversationID, command.AuthContext.UserID).Scan(&role, &status, &permissionVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", 0, false, nil
	}
	if isUndefinedTable(err) {
		return "", "", 0, false, nil
	}
	if err != nil {
		return "", "", 0, false, types.NewDependencyUnavailable("policy conversation member lookup failed")
	}
	role = strings.TrimSpace(role)
	status = strings.TrimSpace(status)
	if roleRank(role) == 0 || status == "" || permissionVersion <= 0 {
		return "", "", 0, false, types.NewDependencyUnavailable("policy conversation member projection is invalid")
	}
	return role, status, permissionVersion, true, nil
}

func roleRank(role string) int {
	switch role {
	case types.ConversationMemberRoleOwner:
		return 3
	case types.ConversationMemberRoleAdmin:
		return 2
	case types.ConversationMemberRoleMember:
		return 1
	default:
		return 0
	}
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
