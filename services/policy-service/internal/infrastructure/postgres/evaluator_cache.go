package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"

	"github.com/qsyy0921/IM/services/policy-service/internal/types"
)

func (e MessagePolicyEvaluator) lookupCacheRevisionToken(
	ctx context.Context,
	command types.CheckMessageActionCommand,
) (string, bool, error) {
	if e.decisionCache == nil || e.decisionCacheTTL <= 0 {
		return "", false, nil
	}
	token, err := e.lookupMessagePolicyRevisionToken(ctx, command)
	if isUndefinedTable(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, types.NewDependencyUnavailable("policy revision lookup failed")
	}
	if strings.TrimSpace(token) == "" {
		return "", false, nil
	}
	return token, true, nil
}

func (e MessagePolicyEvaluator) lookupMessagePolicyRevisionToken(
	ctx context.Context,
	command types.CheckMessageActionCommand,
) (string, error) {
	var token string
	err := e.pool.QueryRow(ctx, `
WITH params AS (
    SELECT
        $1::text AS tenant_id,
        $2::text AS user_id,
        $3::text AS conversation_id,
        $4::text AS action,
        $5::text AS direct_peer_user_id
),
revision_rows AS (
    SELECT r.scope_type, r.scope_id, r.action, r.revision
    FROM policy_revision_state r
    CROSS JOIN params p
    WHERE r.tenant_id = p.tenant_id
      AND (
          (r.scope_type = 'tenant_action' AND r.scope_id = '*' AND r.action = p.action)
          OR (r.scope_type = 'user_action' AND r.scope_id = p.user_id AND r.action = p.action)
          OR (r.scope_type = 'exact_message_action' AND r.scope_id = p.user_id || ':' || p.conversation_id AND r.action = p.action)
          OR (r.scope_type = 'conversation_member' AND r.scope_id = p.conversation_id || ':' || p.user_id AND r.action = '')
          OR (
              p.direct_peer_user_id <> ''
              AND r.scope_type = 'contact_edge'
              AND r.scope_id IN (p.user_id || ':' || p.direct_peer_user_id, p.direct_peer_user_id || ':' || p.user_id)
              AND r.action = 'SEND'
          )
      )
)
SELECT COALESCE(
    string_agg(scope_type || '=' || scope_id || '=' || action || '=' || revision::text, '|' ORDER BY scope_type, scope_id, action),
    'empty'
)
FROM revision_rows
`, command.AuthContext.TenantID, command.AuthContext.UserID, command.ConversationID, command.Action, command.DirectPeerUserID).Scan(&token)
	if err != nil {
		return "", err
	}
	return token, nil
}

func messageDecisionCacheKey(command types.CheckMessageActionCommand, revisionToken string) string {
	parts := []string{
		string(command.AuthContext.TenantID),
		string(command.AuthContext.UserID),
		string(command.ConversationID),
		string(command.Action),
		string(command.DirectPeerUserID),
		strconv.FormatInt(command.ConversationPermissionVersion, 10),
		revisionToken,
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])
}
