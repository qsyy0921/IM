package postgres

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/qsyy0921/IM/services/policy-service/internal/types"
)

type messagePolicyFacts struct {
	ContactBlock             *contactBlockFact
	UserRestriction          *messagePolicyRuleFact
	RoleRule                 *rolePolicyRule
	Member                   *projectedMemberFact
	ReBACRules               []rebacRelationRule
	DirectContactActiveEdges int
	TenantQuota              *tenantQuotaFact
	MessageRule              *messagePolicyRuleFact
}

type contactBlockFact struct {
	Status      string `json:"status"`
	EdgeVersion int64  `json:"edge_version"`
}

type projectedMemberFact struct {
	Role              string `json:"role"`
	Status            string `json:"status"`
	PermissionVersion int64  `json:"permission_version"`
}

type tenantQuotaFact struct {
	MaxDecisions           int    `json:"max_decisions"`
	PermissionVersion      int64  `json:"permission_version"`
	Classification         string `json:"classification"`
	Reason                 string `json:"reason"`
	RecentAllowedDecisions int    `json:"recent_allowed_decisions"`
}

type messagePolicyRuleFact struct {
	Allowed           bool   `json:"allowed"`
	PermissionVersion int64  `json:"permission_version"`
	Classification    string `json:"classification"`
	Reason            string `json:"reason"`
	DecisionSource    string `json:"decision_source"`
}

func (e MessagePolicyEvaluator) lookupMessagePolicyFacts(
	ctx context.Context,
	command types.CheckMessageActionCommand,
) (messagePolicyFacts, error) {
	var contactBlockJSON string
	var userRestrictionJSON string
	var roleRuleJSON string
	var memberJSON string
	var rebacRulesJSON string
	var directContactActiveEdges int
	var tenantQuotaJSON string
	var messageRuleJSON string
	err := e.pool.QueryRow(ctx, `
WITH params AS (
    SELECT
        $1::text AS tenant_id,
        $2::text AS user_id,
        $3::text AS conversation_id,
        $4::text AS action,
        $5::text AS direct_peer_user_id
),
contact_block AS (
    SELECT status, edge_version
    FROM policy_contact_edges_projection e
    CROSS JOIN params p
    WHERE p.action = 'SEND'
      AND p.direct_peer_user_id <> ''
      AND e.tenant_id = p.tenant_id
      AND e.status = 'BLOCKED'
      AND (
          (e.owner_user_id = p.direct_peer_user_id AND e.contact_user_id = p.user_id)
          OR (e.owner_user_id = p.user_id AND e.contact_user_id = p.direct_peer_user_id)
      )
    ORDER BY edge_version DESC
    LIMIT 1
),
user_restriction AS (
    SELECT
        false AS allowed,
        permission_version,
        classification,
        reason,
        'USER_RESTRICTION' AS decision_source
    FROM policy_user_message_action_restrictions r
    CROSS JOIN params p
    WHERE r.tenant_id = p.tenant_id
      AND r.user_id = p.user_id
      AND r.action = p.action
      AND (r.expires_at IS NULL OR r.expires_at > now())
    LIMIT 1
),
role_rule AS (
    SELECT min_role, classification, reason
    FROM policy_conversation_role_action_rules r
    CROSS JOIN params p
    WHERE r.tenant_id = p.tenant_id
      AND r.action = p.action
    LIMIT 1
),
member_projection AS (
    SELECT role, status, permission_version
    FROM policy_conversation_members_projection m
    CROSS JOIN params p
    WHERE m.tenant_id = p.tenant_id
      AND m.conversation_id = p.conversation_id
      AND m.user_id = p.user_id
    LIMIT 1
),
rebac_rules AS (
    SELECT relation_type, conversation_scope, permission_version, classification, reason
    FROM policy_rebac_message_action_rules r
    CROSS JOIN params p
    WHERE r.tenant_id = p.tenant_id
      AND r.action = p.action
      AND r.enabled = true
      AND (
          r.conversation_scope = 'ANY'
          OR r.conversation_scope = CASE WHEN p.direct_peer_user_id <> '' THEN 'DIRECT' ELSE 'GROUP' END
      )
    ORDER BY r.priority ASC, r.updated_at ASC, r.relation_type ASC
),
direct_contact_relation AS (
    SELECT COUNT(*)::int AS active_edges
    FROM policy_contact_edges_projection e
    CROSS JOIN params p
    WHERE p.direct_peer_user_id <> ''
      AND e.tenant_id = p.tenant_id
      AND e.status = 'ACTIVE'
      AND (
          (e.owner_user_id = p.user_id AND e.contact_user_id = p.direct_peer_user_id)
          OR (e.owner_user_id = p.direct_peer_user_id AND e.contact_user_id = p.user_id)
      )
),
tenant_quota AS (
    SELECT
        q.max_decisions,
        q.permission_version,
        q.classification,
        q.reason,
        (
            SELECT COUNT(*)::int
            FROM policy_decision_audit_outbox audit
            WHERE audit.tenant_id = q.tenant_id
              AND audit.action = q.action
              AND audit.allowed = true
              AND audit.created_at >= now() - (q.window_seconds * interval '1 second')
        ) AS recent_allowed_decisions
    FROM policy_tenant_message_action_quotas q
    CROSS JOIN params p
    WHERE q.tenant_id = p.tenant_id
      AND q.action = p.action
      AND q.enabled = true
    LIMIT 1
),
message_rule AS (
    SELECT allowed, permission_version, classification, reason, decision_source
    FROM (
        SELECT
            1 AS precedence,
            r.allowed,
            r.permission_version,
            r.classification,
            r.reason,
            'EXACT_RULE' AS decision_source
        FROM policy_message_action_rules r
        CROSS JOIN params p
        WHERE r.tenant_id = p.tenant_id
          AND r.user_id = p.user_id
          AND r.conversation_id = p.conversation_id
          AND r.action = p.action

        UNION ALL

        SELECT
            2 AS precedence,
            r.allowed,
            r.permission_version,
            r.classification,
            r.reason,
            'TENANT_RULE' AS decision_source
        FROM policy_tenant_message_action_rules r
        CROSS JOIN params p
        WHERE r.tenant_id = p.tenant_id
          AND r.action = p.action
    ) rules
    ORDER BY precedence ASC
    LIMIT 1
)
SELECT
    COALESCE((SELECT to_jsonb(contact_block) FROM contact_block), 'null'::jsonb)::text,
    COALESCE((SELECT to_jsonb(user_restriction) FROM user_restriction), 'null'::jsonb)::text,
    COALESCE((SELECT to_jsonb(role_rule) FROM role_rule), 'null'::jsonb)::text,
    COALESCE((SELECT to_jsonb(member_projection) FROM member_projection), 'null'::jsonb)::text,
    COALESCE((SELECT jsonb_agg(to_jsonb(rebac_rules)) FROM rebac_rules), '[]'::jsonb)::text,
    COALESCE((SELECT active_edges FROM direct_contact_relation), 0),
    COALESCE((SELECT to_jsonb(tenant_quota) FROM tenant_quota), 'null'::jsonb)::text,
    COALESCE((SELECT to_jsonb(message_rule) FROM message_rule), 'null'::jsonb)::text
`, command.AuthContext.TenantID, command.AuthContext.UserID, command.ConversationID, command.Action, command.DirectPeerUserID).Scan(
		&contactBlockJSON,
		&userRestrictionJSON,
		&roleRuleJSON,
		&memberJSON,
		&rebacRulesJSON,
		&directContactActiveEdges,
		&tenantQuotaJSON,
		&messageRuleJSON,
	)
	if err != nil {
		return messagePolicyFacts{}, types.NewDependencyUnavailable("policy facts lookup failed")
	}

	facts := messagePolicyFacts{DirectContactActiveEdges: directContactActiveEdges}
	if facts.ContactBlock, err = decodeOptionalPolicyFact[contactBlockFact](contactBlockJSON); err != nil {
		return messagePolicyFacts{}, types.NewDependencyUnavailable("policy contact block fact is invalid")
	}
	if facts.UserRestriction, err = decodeOptionalPolicyFact[messagePolicyRuleFact](userRestrictionJSON); err != nil {
		return messagePolicyFacts{}, types.NewDependencyUnavailable("policy user restriction fact is invalid")
	}
	if facts.RoleRule, err = decodeOptionalPolicyFact[rolePolicyRule](roleRuleJSON); err != nil {
		return messagePolicyFacts{}, types.NewDependencyUnavailable("policy role rule fact is invalid")
	}
	if facts.Member, err = decodeOptionalPolicyFact[projectedMemberFact](memberJSON); err != nil {
		return messagePolicyFacts{}, types.NewDependencyUnavailable("policy member projection fact is invalid")
	}
	if err := json.Unmarshal([]byte(rebacRulesJSON), &facts.ReBACRules); err != nil {
		return messagePolicyFacts{}, types.NewDependencyUnavailable("policy rebac facts are invalid")
	}
	if facts.TenantQuota, err = decodeOptionalPolicyFact[tenantQuotaFact](tenantQuotaJSON); err != nil {
		return messagePolicyFacts{}, types.NewDependencyUnavailable("policy tenant quota fact is invalid")
	}
	if facts.MessageRule, err = decodeOptionalPolicyFact[messagePolicyRuleFact](messageRuleJSON); err != nil {
		return messagePolicyFacts{}, types.NewDependencyUnavailable("policy message rule fact is invalid")
	}
	return facts, nil
}

func decodeOptionalPolicyFact[T any](raw string) (*T, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return nil, nil
	}
	var value T
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return nil, err
	}
	return &value, nil
}

func (e MessagePolicyEvaluator) decideMessageActionFromFacts(
	ctx context.Context,
	command types.CheckMessageActionCommand,
	facts messagePolicyFacts,
) (types.MessageActionDecision, bool, error) {
	started := time.Now()
	decision, handled, err := e.contactBlockDecision(command, facts)
	e.observeStage(command.Action, "contact_block_lookup", started, err)
	if err != nil || handled {
		return decision, handled && cacheablePolicyDecision(decision, facts), err
	}

	started = time.Now()
	decision, handled, err = e.userRestrictionDecision(command, facts)
	e.observeStage(command.Action, "user_restriction_lookup", started, err)
	if err != nil || handled {
		return decision, handled && cacheablePolicyDecision(decision, facts), err
	}

	started = time.Now()
	decision, handled, err = e.roleGateDecision(command, facts)
	e.observeStage(command.Action, "role_gate", started, err)
	if err != nil || handled {
		return decision, handled && cacheablePolicyDecision(decision, facts), err
	}

	started = time.Now()
	decision, handled, err = e.rebacGateDecision(command, facts)
	e.observeStage(command.Action, "rebac_gate", started, err)
	if err != nil || handled {
		return decision, handled && cacheablePolicyDecision(decision, facts), err
	}

	started = time.Now()
	decision, handled, err = e.tenantQuotaDecision(command, facts)
	e.observeStage(command.Action, "tenant_quota_lookup", started, err)
	if err != nil || handled {
		return decision, false, err
	}

	started = time.Now()
	decision, handled, err = e.messageRuleDecision(command, facts)
	e.observeStage(command.Action, "message_action_rule_lookup", started, err)
	if err != nil || handled {
		return decision, handled && cacheablePolicyDecision(decision, facts), err
	}

	decision, err = e.staticDefaultDecision(ctx, command)
	return decision, false, err
}

func (e MessagePolicyEvaluator) contactBlockDecision(
	command types.CheckMessageActionCommand,
	facts messagePolicyFacts,
) (types.MessageActionDecision, bool, error) {
	if command.Action != types.MessageActionSend || command.DirectPeerUserID == "" || facts.ContactBlock == nil {
		return types.MessageActionDecision{}, false, nil
	}
	status := strings.TrimSpace(facts.ContactBlock.Status)
	if status != types.ContactEdgeStatusBlocked {
		return types.MessageActionDecision{}, false, nil
	}
	if facts.ContactBlock.EdgeVersion <= 0 {
		return types.MessageActionDecision{}, false, types.NewDependencyUnavailable("contact policy projection is invalid")
	}
	return types.MessageActionDecision{
		TenantID:          command.AuthContext.TenantID,
		UserID:            command.AuthContext.UserID,
		ConversationID:    command.ConversationID,
		MessageID:         command.MessageID,
		Action:            command.Action,
		Allowed:           false,
		PermissionVersion: facts.ContactBlock.EdgeVersion,
		Classification:    "CONTACT_BLOCKED",
		Reason:            "contact blocked",
		DecisionSource:    types.PolicyDecisionSourceContactProjection,
	}, true, nil
}

func (e MessagePolicyEvaluator) userRestrictionDecision(
	command types.CheckMessageActionCommand,
	facts messagePolicyFacts,
) (types.MessageActionDecision, bool, error) {
	if facts.UserRestriction == nil {
		return types.MessageActionDecision{}, false, nil
	}
	decision := basePolicyFactDecision(command, *facts.UserRestriction, types.PolicyDecisionSourceUserRestriction)
	if decision.PermissionVersion <= 0 || decision.Classification == "" {
		return types.MessageActionDecision{}, false, types.NewDependencyUnavailable("policy user restriction is invalid")
	}
	if decision.Reason == "" {
		decision.Reason = "user moderation policy denied"
	}
	return decision, true, nil
}

func (e MessagePolicyEvaluator) roleGateDecision(
	command types.CheckMessageActionCommand,
	facts messagePolicyFacts,
) (types.MessageActionDecision, bool, error) {
	if facts.RoleRule == nil {
		return types.MessageActionDecision{}, false, nil
	}
	rule := *facts.RoleRule
	rule.MinRole = strings.TrimSpace(rule.MinRole)
	rule.Classification = strings.TrimSpace(rule.Classification)
	rule.Reason = strings.TrimSpace(rule.Reason)
	if rule.Classification == "" || roleRank(rule.MinRole) == 0 {
		return types.MessageActionDecision{}, false, types.NewDependencyUnavailable("policy role rule is invalid")
	}
	member, err := policyProjectedMemberFromFacts(command, facts)
	if err != nil {
		return types.MessageActionDecision{}, false, err
	}
	decision := types.MessageActionDecision{
		TenantID:          command.AuthContext.TenantID,
		UserID:            command.AuthContext.UserID,
		ConversationID:    command.ConversationID,
		MessageID:         command.MessageID,
		Action:            command.Action,
		PermissionVersion: member.PermissionVersion,
		Classification:    rule.Classification,
		Reason:            rule.Reason,
		DecisionSource:    types.PolicyDecisionSourceConversationRole,
	}
	if member.Status == types.ConversationMemberStatusActive && roleRank(member.Role) >= roleRank(rule.MinRole) {
		return types.MessageActionDecision{}, false, nil
	}
	if decision.Reason == "" {
		decision.Reason = "conversation role policy denied"
	}
	return decision, true, nil
}

func (e MessagePolicyEvaluator) rebacGateDecision(
	command types.CheckMessageActionCommand,
	facts messagePolicyFacts,
) (types.MessageActionDecision, bool, error) {
	for _, rule := range facts.ReBACRules {
		rule.RelationType = strings.TrimSpace(rule.RelationType)
		rule.ConversationScope = strings.TrimSpace(rule.ConversationScope)
		rule.Classification = strings.TrimSpace(rule.Classification)
		rule.Reason = strings.TrimSpace(rule.Reason)
		if !validReBACRelationRule(rule) {
			return types.MessageActionDecision{}, false, types.NewDependencyUnavailable("policy rebac rule is invalid")
		}
		satisfied, err := rebacRelationSatisfiedByFacts(command, facts, rule)
		if err != nil {
			return types.MessageActionDecision{}, false, err
		}
		if satisfied {
			continue
		}
		if rule.Reason == "" {
			rule.Reason = "relationship policy denied"
		}
		return types.MessageActionDecision{
			TenantID:          command.AuthContext.TenantID,
			UserID:            command.AuthContext.UserID,
			ConversationID:    command.ConversationID,
			MessageID:         command.MessageID,
			Action:            command.Action,
			Allowed:           false,
			PermissionVersion: rule.PermissionVersion,
			Classification:    rule.Classification,
			Reason:            rule.Reason,
			DecisionSource:    types.PolicyDecisionSourceReBACRelation,
		}, true, nil
	}
	return types.MessageActionDecision{}, false, nil
}

func rebacRelationSatisfiedByFacts(
	command types.CheckMessageActionCommand,
	facts messagePolicyFacts,
	rule rebacRelationRule,
) (bool, error) {
	switch types.ReBACRelationType(rule.RelationType) {
	case types.ReBACRelationDirectContactActive:
		if command.DirectPeerUserID == "" {
			return false, nil
		}
		return facts.DirectContactActiveEdges >= 2, nil
	case types.ReBACRelationConversationMemberActive:
		member, err := policyProjectedMemberFromFacts(command, facts)
		if err != nil {
			return false, err
		}
		return member.Status == types.ConversationMemberStatusActive, nil
	default:
		return false, types.NewDependencyUnavailable("policy rebac rule is invalid")
	}
}

func (e MessagePolicyEvaluator) tenantQuotaDecision(
	command types.CheckMessageActionCommand,
	facts messagePolicyFacts,
) (types.MessageActionDecision, bool, error) {
	if facts.TenantQuota == nil {
		return types.MessageActionDecision{}, false, nil
	}
	quota := *facts.TenantQuota
	quota.Classification = strings.TrimSpace(quota.Classification)
	quota.Reason = strings.TrimSpace(quota.Reason)
	if quota.MaxDecisions <= 0 || quota.PermissionVersion <= 0 || quota.Classification == "" {
		return types.MessageActionDecision{}, false, types.NewDependencyUnavailable("policy tenant quota is invalid")
	}
	if quota.RecentAllowedDecisions < quota.MaxDecisions {
		return types.MessageActionDecision{}, false, nil
	}
	if quota.Reason == "" {
		quota.Reason = "tenant quota exceeded"
	}
	return types.MessageActionDecision{
		TenantID:          command.AuthContext.TenantID,
		UserID:            command.AuthContext.UserID,
		ConversationID:    command.ConversationID,
		MessageID:         command.MessageID,
		Action:            command.Action,
		Allowed:           false,
		PermissionVersion: quota.PermissionVersion,
		Classification:    quota.Classification,
		Reason:            quota.Reason,
		DecisionSource:    types.PolicyDecisionSourceTenantQuota,
	}, true, nil
}

func (e MessagePolicyEvaluator) messageRuleDecision(
	command types.CheckMessageActionCommand,
	facts messagePolicyFacts,
) (types.MessageActionDecision, bool, error) {
	if facts.MessageRule == nil {
		return types.MessageActionDecision{}, false, nil
	}
	source := types.PolicyDecisionSource("")
	switch facts.MessageRule.DecisionSource {
	case "EXACT_RULE":
		source = types.PolicyDecisionSourceExactRule
	case "TENANT_RULE":
		source = types.PolicyDecisionSourceTenantRule
	default:
		return types.MessageActionDecision{}, false, types.NewDependencyUnavailable("policy message action rule source is invalid")
	}
	decision := basePolicyFactDecision(command, *facts.MessageRule, source)
	if decision.PermissionVersion <= 0 || decision.Classification == "" {
		return types.MessageActionDecision{}, false, types.NewDependencyUnavailable("policy message action rule is invalid")
	}
	if !decision.Allowed && decision.Reason == "" {
		decision.Reason = "policy denied"
	}
	return decision, true, nil
}

func basePolicyFactDecision(
	command types.CheckMessageActionCommand,
	fact messagePolicyRuleFact,
	source types.PolicyDecisionSource,
) types.MessageActionDecision {
	return types.MessageActionDecision{
		TenantID:          command.AuthContext.TenantID,
		UserID:            command.AuthContext.UserID,
		ConversationID:    command.ConversationID,
		MessageID:         command.MessageID,
		Action:            command.Action,
		Allowed:           fact.Allowed,
		PermissionVersion: fact.PermissionVersion,
		Classification:    strings.TrimSpace(fact.Classification),
		Reason:            strings.TrimSpace(fact.Reason),
		DecisionSource:    source,
	}
}

func policyProjectedMemberFromFacts(
	command types.CheckMessageActionCommand,
	facts messagePolicyFacts,
) (projectedMemberFact, error) {
	if command.ConversationPermissionVersion <= 0 {
		return projectedMemberFact{}, types.NewDependencyUnavailable("policy conversation permission version is required")
	}
	if facts.Member == nil {
		return projectedMemberFact{}, types.NewDependencyUnavailable("policy conversation member projection is missing")
	}
	member := *facts.Member
	member.Role = strings.TrimSpace(member.Role)
	member.Status = strings.TrimSpace(member.Status)
	if roleRank(member.Role) == 0 || member.Status == "" || member.PermissionVersion <= 0 {
		return projectedMemberFact{}, types.NewDependencyUnavailable("policy conversation member projection is invalid")
	}
	if member.PermissionVersion != command.ConversationPermissionVersion {
		return projectedMemberFact{}, types.NewDependencyUnavailable("policy conversation member projection is stale")
	}
	return member, nil
}

func cacheablePolicyDecision(decision types.MessageActionDecision, facts messagePolicyFacts) bool {
	if decision.PermissionVersion <= 0 || strings.TrimSpace(decision.Classification) == "" {
		return false
	}
	if facts.TenantQuota != nil {
		return false
	}
	switch decision.DecisionSource {
	case types.PolicyDecisionSourceContactProjection,
		types.PolicyDecisionSourceConversationRole,
		types.PolicyDecisionSourceReBACRelation,
		types.PolicyDecisionSourceExactRule,
		types.PolicyDecisionSourceTenantRule:
		return true
	default:
		return false
	}
}
