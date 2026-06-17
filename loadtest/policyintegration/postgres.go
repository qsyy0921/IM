package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

func readDBStats(ctx context.Context, pool *pgxpool.Pool, cfg config) (dbStats, error) {
	var stats dbStats
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM message_log WHERE tenant_id = $1`, cfg.tenantID).Scan(&stats.MessageLog); err != nil {
		return stats, err
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM conversation_timeline_events WHERE tenant_id = $1`, cfg.tenantID).Scan(&stats.TimelineEvents); err != nil {
		return stats, err
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM message_outbox WHERE tenant_id = $1`, cfg.tenantID).Scan(&stats.MessageOutbox); err != nil {
		return stats, err
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM message_change_history WHERE tenant_id = $1`, cfg.tenantID).Scan(&stats.MessageChangeHistory); err != nil {
		return stats, err
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM message_command_idempotency WHERE tenant_id = $1`, cfg.tenantID).Scan(&stats.CommandIdempotency); err != nil {
		return stats, err
	}
	if err := pool.QueryRow(ctx, `
SELECT COALESCE(MAX(current_seq), 0)
FROM conversation_seq
WHERE tenant_id = $1
  AND conversation_id = $2
`, cfg.tenantID, cfg.conversationID).Scan(&stats.ConversationSeq); err != nil {
		return stats, err
	}
	return stats, nil
}

func readMessageRow(ctx context.Context, pool *pgxpool.Pool, cfg config, messageID string) (messageRow, error) {
	row := pool.QueryRow(ctx, `
SELECT
    ml.message_id,
    ml.conversation_seq,
    ml.status,
    ml.payload_json::text,
    ml.permission_version,
    ml.classification,
    te.permission_version,
    COALESCE(te.classification, ''),
    te.fanout_policy_version,
    mo.status
FROM message_log ml
JOIN conversation_timeline_events te
  ON te.tenant_id = ml.tenant_id
 AND te.conversation_id = ml.conversation_id
 AND te.seq = ml.conversation_seq
LEFT JOIN message_outbox mo
  ON mo.tenant_id = ml.tenant_id
 AND mo.conversation_id = ml.conversation_id
 AND mo.aggregate_version = ml.conversation_seq
WHERE ml.tenant_id = $1
  AND ml.message_id = $2
`, cfg.tenantID, messageID)
	var result messageRow
	if err := row.Scan(
		&result.MessageID,
		&result.ConversationSeq,
		&result.MessageStatus,
		&result.MessagePayload,
		&result.MessagePermissionVersion,
		&result.MessageClassification,
		&result.TimelinePermissionVersion,
		&result.TimelineClassification,
		&result.FanoutPolicyVersion,
		&result.OutboxStatus,
	); err != nil {
		return result, err
	}
	return result, nil
}

func readChangeRow(
	ctx context.Context,
	pool *pgxpool.Pool,
	cfg config,
	messageID string,
	conversationSeq int64,
) (changeRow, error) {
	row := pool.QueryRow(ctx, `
SELECT
    ml.message_id,
    ml.status,
    ml.payload_json::text,
    te.event_type,
    te.permission_version,
    COALESCE(te.classification, ''),
    mo.status,
    (
      SELECT COUNT(*)
      FROM message_change_history mch
      WHERE mch.tenant_id = ml.tenant_id
        AND mch.conversation_id = ml.conversation_id
        AND mch.message_id = ml.message_id
    ),
    COALESCE(latest.change_type, ''),
    COALESCE(latest.before_status, ''),
    COALESCE(latest.after_status, ''),
    ml.edited_at IS NOT NULL,
    ml.revoked_at IS NOT NULL,
    ml.deleted_at IS NOT NULL
FROM message_log ml
JOIN conversation_timeline_events te
  ON te.tenant_id = ml.tenant_id
 AND te.conversation_id = ml.conversation_id
 AND te.seq = $3
LEFT JOIN message_outbox mo
  ON mo.tenant_id = te.tenant_id
 AND mo.conversation_id = te.conversation_id
 AND mo.aggregate_version = te.seq
LEFT JOIN LATERAL (
  SELECT mch.change_type, mch.before_status, mch.after_status
  FROM message_change_history mch
  WHERE mch.tenant_id = ml.tenant_id
    AND mch.conversation_id = ml.conversation_id
    AND mch.message_id = ml.message_id
  ORDER BY mch.change_version DESC
  LIMIT 1
) latest ON TRUE
WHERE ml.tenant_id = $1
  AND ml.message_id = $2
`, cfg.tenantID, messageID, conversationSeq)
	var result changeRow
	if err := row.Scan(
		&result.MessageID,
		&result.MessageStatus,
		&result.MessagePayload,
		&result.TimelineEventType,
		&result.TimelinePermissionVersion,
		&result.TimelineClassification,
		&result.OutboxStatus,
		&result.ChangeHistoryRows,
		&result.ChangeHistoryType,
		&result.ChangeHistoryBeforeStatus,
		&result.ChangeHistoryAfterStatus,
		&result.EditedAtSet,
		&result.RevokedAtSet,
		&result.DeletedAtSet,
	); err != nil {
		return result, err
	}
	return result, nil
}

func readPolicyAudit(ctx context.Context, pool *pgxpool.Pool, cfg config) (policyAudit, error) {
	var result policyAudit
	if err := pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM policy_decision_audit_outbox
WHERE tenant_id = $1
`, cfg.tenantID).Scan(&result.RowCount); err != nil {
		return result, fmt.Errorf("read policy audit count: %w", err)
	}
	if result.RowCount < 1 {
		return result, fmt.Errorf("policy audit row count=%d expected at least 1", result.RowCount)
	}
	if err := pool.QueryRow(ctx, `
SELECT
    event_id,
    action,
    message_id_present,
    message_key <> '',
    allowed,
    permission_version,
    classification,
    reason_code,
    status
FROM policy_decision_audit_outbox
WHERE tenant_id = $1
ORDER BY created_at DESC, id DESC
LIMIT 1
`, cfg.tenantID).Scan(
		&result.EventID,
		&result.Action,
		&result.MessageIDPresent,
		&result.MessageKeyPresent,
		&result.Allowed,
		&result.PermissionVersion,
		&result.Classification,
		&result.ReasonCode,
		&result.Status,
	); err != nil {
		return result, fmt.Errorf("read policy audit row: %w", err)
	}
	return result, nil
}

func cleanupTenant(ctx context.Context, pool *pgxpool.Pool, tenantID string) error {
	for _, statement := range []string{
		`DELETE FROM message_outbox WHERE tenant_id = $1`,
		`DELETE FROM conversation_timeline_events WHERE tenant_id = $1`,
		`DELETE FROM message_change_history WHERE tenant_id = $1`,
		`DELETE FROM message_command_idempotency WHERE tenant_id = $1`,
		`DELETE FROM message_log WHERE tenant_id = $1`,
		`DELETE FROM conversation_seq WHERE tenant_id = $1`,
		`DELETE FROM seq_allocation_journal WHERE tenant_id = $1`,
		`DELETE FROM timeline_gap_markers WHERE tenant_id = $1`,
	} {
		if _, err := pool.Exec(ctx, statement, tenantID); err != nil {
			return fmt.Errorf("cleanup tenant: %w", err)
		}
	}
	return nil
}

func cleanupPolicyAudit(ctx context.Context, pool *pgxpool.Pool, tenantID string) error {
	if _, err := pool.Exec(ctx, `DELETE FROM policy_decision_audit_outbox WHERE tenant_id = $1`, tenantID); err != nil {
		return fmt.Errorf("cleanup policy decision audit outbox: %w", err)
	}
	return nil
}

func seedPolicyRules(ctx context.Context, pool *pgxpool.Pool, cfg config) error {
	if err := cleanupPolicyAudit(ctx, pool, cfg.tenantID); err != nil {
		return err
	}
	if _, err := pool.Exec(ctx, `DELETE FROM policy_message_action_rules WHERE tenant_id = $1`, cfg.tenantID); err != nil {
		return fmt.Errorf("cleanup policy rules: %w", err)
	}
	for _, rule := range expectedPolicyRules(cfg, true, false) {
		if err := seedOnePolicyRule(ctx, pool, rule); err != nil {
			return err
		}
	}
	return nil
}

func seedTenantPolicyRules(ctx context.Context, pool *pgxpool.Pool, cfg config) error {
	if err := cleanupPolicyAudit(ctx, pool, cfg.tenantID); err != nil {
		return err
	}
	if _, err := pool.Exec(ctx, `DELETE FROM policy_tenant_message_action_rules WHERE tenant_id = $1`, cfg.tenantID); err != nil {
		return fmt.Errorf("cleanup tenant policy rules: %w", err)
	}
	for _, rule := range expectedPolicyRules(cfg, false, true) {
		if err := seedOneTenantPolicyRule(ctx, pool, rule); err != nil {
			return err
		}
	}
	return nil
}

func seedConversationRoleGate(ctx context.Context, pool *pgxpool.Pool, cfg config) error {
	if cfg.action != "send" {
		return fmt.Errorf("conversation role gate integration smoke currently supports send only")
	}
	if err := cleanupPolicyAudit(ctx, pool, cfg.tenantID); err != nil {
		return err
	}
	if _, err := pool.Exec(ctx, `DELETE FROM policy_conversation_role_action_rules WHERE tenant_id = $1`, cfg.tenantID); err != nil {
		return fmt.Errorf("cleanup conversation role rules: %w", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM policy_conversation_members_projection WHERE tenant_id = $1`, cfg.tenantID); err != nil {
		return fmt.Errorf("cleanup conversation member projection: %w", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM policy_tenant_message_action_rules WHERE tenant_id = $1`, cfg.tenantID); err != nil {
		return fmt.Errorf("cleanup tenant policy rules: %w", err)
	}
	rule := expectedRoleRule(cfg)
	if _, err := pool.Exec(ctx, `
INSERT INTO policy_conversation_role_action_rules (
    tenant_id,
    action,
    min_role,
    classification,
    reason,
    source
) VALUES ($1, $2, $3, $4, $5, 'policy-message-role-smoke')
ON CONFLICT (tenant_id, action) DO UPDATE
SET min_role = EXCLUDED.min_role,
    classification = EXCLUDED.classification,
    reason = EXCLUDED.reason,
    source = EXCLUDED.source,
    updated_at = now()
`, rule.TenantID, rule.Action, rule.MinRole, rule.Classification, rule.Reason); err != nil {
		return fmt.Errorf("seed conversation role rule: %w", err)
	}
	member := expectedConversationMember(cfg)
	if _, err := pool.Exec(ctx, `
INSERT INTO policy_conversation_members_projection (
    tenant_id,
    conversation_id,
    user_id,
    role,
    status,
    member_version,
    permission_version,
    updated_by_event_id
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (tenant_id, conversation_id, user_id) DO UPDATE
SET role = EXCLUDED.role,
    status = EXCLUDED.status,
    member_version = EXCLUDED.member_version,
    permission_version = EXCLUDED.permission_version,
    updated_by_event_id = EXCLUDED.updated_by_event_id,
    updated_at = now()
`, member.TenantID, member.ConversationID, member.UserID, member.Role, member.Status, member.MemberVersion, member.PermissionVersion, member.UpdatedByEventID); err != nil {
		return fmt.Errorf("seed conversation member projection: %w", err)
	}
	allowRule := tenantPolicyRule(cfg, "SEND", true, cfg.expectedClassification, "")
	if cfg.scenario == "deny" {
		allowRule.Classification = "ROLE_GATE_TENANT_ALLOW_SHOULD_NOT_APPEAR"
		allowRule.Reason = ""
	}
	if err := seedOneTenantPolicyRule(ctx, pool, allowRule); err != nil {
		return err
	}
	return nil
}

func seedOwnershipOverrideRule(ctx context.Context, pool *pgxpool.Pool, cfg config) error {
	if cfg.action == "send" {
		return fmt.Errorf("ownership override integration smoke supports edit/revoke/delete only")
	}
	if err := cleanupPolicyAudit(ctx, pool, cfg.tenantID); err != nil {
		return err
	}
	if _, err := pool.Exec(ctx, `DELETE FROM policy_message_ownership_override_rules WHERE tenant_id = $1`, cfg.tenantID); err != nil {
		return fmt.Errorf("cleanup ownership override rules: %w", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM policy_conversation_members_projection WHERE tenant_id = $1`, cfg.tenantID); err != nil {
		return fmt.Errorf("cleanup conversation member projection: %w", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM policy_tenant_message_action_rules WHERE tenant_id = $1`, cfg.tenantID); err != nil {
		return fmt.Errorf("cleanup tenant policy rules: %w", err)
	}
	rule := expectedOwnershipOverrideRule(cfg)
	if _, err := pool.Exec(ctx, `
INSERT INTO policy_message_ownership_override_rules (
    tenant_id,
    action,
    min_role,
    classification,
    reason,
    source
) VALUES ($1, $2, $3, $4, $5, 'policy-message-ownership-override-smoke')
ON CONFLICT (tenant_id, action) DO UPDATE
SET min_role = EXCLUDED.min_role,
    classification = EXCLUDED.classification,
    reason = EXCLUDED.reason,
    source = EXCLUDED.source,
    updated_at = now()
`, rule.TenantID, rule.Action, rule.MinRole, rule.Classification, rule.Reason); err != nil {
		return fmt.Errorf("seed ownership override rule: %w", err)
	}
	member := expectedConversationMember(cfg)
	if _, err := pool.Exec(ctx, `
INSERT INTO policy_conversation_members_projection (
    tenant_id,
    conversation_id,
    user_id,
    role,
    status,
    member_version,
    permission_version,
    updated_by_event_id
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (tenant_id, conversation_id, user_id) DO UPDATE
SET role = EXCLUDED.role,
    status = EXCLUDED.status,
    member_version = EXCLUDED.member_version,
    permission_version = EXCLUDED.permission_version,
    updated_by_event_id = EXCLUDED.updated_by_event_id,
    updated_at = now()
`, member.TenantID, member.ConversationID, member.UserID, member.Role, member.Status, member.MemberVersion, member.PermissionVersion, member.UpdatedByEventID); err != nil {
		return fmt.Errorf("seed conversation member projection: %w", err)
	}
	allowRule := tenantPolicyRule(cfg, "SEND", true, "POLICY_SEND_SEED", "")
	if err := seedOneTenantPolicyRule(ctx, pool, allowRule); err != nil {
		return err
	}
	return nil
}

func seedOnePolicyRule(ctx context.Context, pool *pgxpool.Pool, rule policyRule) error {
	_, err := pool.Exec(ctx, `
INSERT INTO policy_message_action_rules (
    tenant_id,
    user_id,
    conversation_id,
    action,
    allowed,
    permission_version,
    classification,
    reason,
    source
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'loadtest')
ON CONFLICT (tenant_id, user_id, conversation_id, action) DO UPDATE
SET allowed = EXCLUDED.allowed,
    permission_version = EXCLUDED.permission_version,
    classification = EXCLUDED.classification,
    reason = EXCLUDED.reason,
    source = EXCLUDED.source,
    updated_at = now()
`, rule.TenantID, rule.UserID, rule.ConversationID, rule.Action, rule.Allowed, rule.PermissionVersion, rule.Classification, rule.Reason)
	if err != nil {
		return fmt.Errorf("seed policy rule: %w", err)
	}
	return nil
}

func seedOneTenantPolicyRule(ctx context.Context, pool *pgxpool.Pool, rule policyRule) error {
	_, err := pool.Exec(ctx, `
INSERT INTO policy_tenant_message_action_rules (
    tenant_id,
    action,
    allowed,
    permission_version,
    classification,
    reason,
    source
) VALUES ($1, $2, $3, $4, $5, $6, 'loadtest')
ON CONFLICT (tenant_id, action) DO UPDATE
SET allowed = EXCLUDED.allowed,
    permission_version = EXCLUDED.permission_version,
    classification = EXCLUDED.classification,
    reason = EXCLUDED.reason,
    source = EXCLUDED.source,
    updated_at = now()
`, rule.TenantID, rule.Action, rule.Allowed, rule.PermissionVersion, rule.Classification, rule.Reason)
	if err != nil {
		return fmt.Errorf("seed tenant policy rule: %w", err)
	}
	return nil
}

func expectedPolicyRules(cfg config, includeExact bool, includeTenant bool) []policyRule {
	rules := make([]policyRule, 0, 4)
	if includeTenant && cfg.action != "send" {
		rules = append(rules, tenantPolicyRule(cfg, "SEND", true, "POLICY_SEND_SEED", ""))
	}
	if includeExact && cfg.action != "send" {
		rules = append(rules, exactPolicyRule(cfg, "SEND", true, "POLICY_SEND_SEED", ""))
	}
	allowed := cfg.scenario == "allow"
	reason := cfg.expectedReason
	if !allowed && strings.TrimSpace(reason) == "" {
		reason = "policy denied"
	}
	action := strings.ToUpper(cfg.action)
	if includeTenant {
		rules = append(rules, tenantPolicyRule(cfg, action, allowed, cfg.expectedClassification, reason))
	}
	if includeExact {
		rules = append(rules, exactPolicyRule(cfg, action, allowed, cfg.expectedClassification, reason))
	}
	return rules
}

func expectedRoleRule(cfg config) roleRule {
	return roleRule{
		TenantID:       cfg.tenantID,
		Action:         strings.ToUpper(cfg.action),
		MinRole:        "ADMIN",
		Classification: "CONVERSATION_ROLE_DENIED",
		Reason:         "conversation role policy denied",
	}
}

func expectedOwnershipOverrideRule(cfg config) roleRule {
	return roleRule{
		TenantID:       cfg.tenantID,
		Action:         strings.ToUpper(cfg.action),
		MinRole:        "ADMIN",
		Classification: "MESSAGE_OWNERSHIP_ROLE_OVERRIDE",
		Reason:         "",
	}
}

func expectedConversationMember(cfg config) memberRow {
	role := "ADMIN"
	if cfg.scenario == "deny" {
		role = "MEMBER"
	}
	return memberRow{
		TenantID:          cfg.tenantID,
		ConversationID:    cfg.conversationID,
		UserID:            expectedConversationMemberUserID(cfg),
		Role:              role,
		Status:            "ACTIVE",
		MemberVersion:     cfg.expectedPermissionVer,
		PermissionVersion: cfg.expectedPermissionVer,
		UpdatedByEventID:  expectedConversationMemberEventID(cfg),
	}
}

func expectedConversationMemberUserID(cfg config) string {
	if cfg.seedOwnershipOverride {
		return cfg.changeUserID
	}
	return cfg.userID
}

func expectedConversationMemberEventID(cfg config) string {
	if cfg.seedOwnershipOverride {
		return "policy-message-ownership-override-smoke-" + cfg.scenario
	}
	return "policy-message-role-smoke-" + cfg.scenario
}

func exactPolicyRule(cfg config, action string, allowed bool, classification string, reason string) policyRule {
	rule := tenantPolicyRule(cfg, action, allowed, classification, reason)
	rule.UserID = cfg.userID
	if action != "SEND" {
		rule.UserID = cfg.changeUserID
	}
	rule.ConversationID = cfg.conversationID
	return rule
}

func tenantPolicyRule(cfg config, action string, allowed bool, classification string, reason string) policyRule {
	return policyRule{
		TenantID:          cfg.tenantID,
		Action:            action,
		Allowed:           allowed,
		PermissionVersion: cfg.expectedPermissionVer,
		Classification:    classification,
		Reason:            reason,
	}
}
