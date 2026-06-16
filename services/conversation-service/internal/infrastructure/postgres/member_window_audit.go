package postgres

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/qsyy0921/IM/services/conversation-service/internal/types"
)

type MemberWindowAuditOptions struct {
	TenantID       string
	ConversationID string
	UserID         string
	Role           string
	Status         string
	IssueClass     string
	Limit          int
}

type MemberWindowAuditRow struct {
	TenantID                      string
	ConversationID                string
	UserID                        string
	Role                          string
	Status                        string
	JoinSeq                       int64
	HasJoinSeq                    bool
	LeaveSeq                      int64
	HasLeaveSeq                   bool
	MemberVersion                 int64
	PermissionVersion             int64
	ConversationMemberVersion     int64
	ConversationPermissionVersion int64
	ConversationStatus            string
	IssueClass                    string
	UpdatedAt                     time.Time
}

func (r *Repository) AuditMemberWindows(ctx context.Context, options MemberWindowAuditOptions) ([]MemberWindowAuditRow, error) {
	if r.pool == nil {
		return nil, types.NewDBReadFailed("repository is not configured")
	}
	limit := options.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}

	args := make([]any, 0, 8)
	clauses := []string{"issue_class <> ''"}
	if tenantID := strings.TrimSpace(options.TenantID); tenantID != "" {
		args = append(args, tenantID)
		clauses = append(clauses, "tenant_id = $"+strconv.Itoa(len(args)))
	}
	if conversationID := strings.TrimSpace(options.ConversationID); conversationID != "" {
		args = append(args, conversationID)
		clauses = append(clauses, "conversation_id = $"+strconv.Itoa(len(args)))
	}
	if userID := strings.TrimSpace(options.UserID); userID != "" {
		args = append(args, userID)
		clauses = append(clauses, "user_id = $"+strconv.Itoa(len(args)))
	}
	if rawRole := strings.TrimSpace(options.Role); rawRole != "" {
		role := normalizeMemberWindowAuditRole(rawRole)
		if role == "" {
			return nil, errors.New("unsupported member role")
		}
		args = append(args, role)
		clauses = append(clauses, "role = $"+strconv.Itoa(len(args)))
	}
	if rawStatus := strings.TrimSpace(options.Status); rawStatus != "" {
		status := normalizeMemberWindowAuditStatus(rawStatus)
		if status == "" {
			return nil, errors.New("unsupported member status")
		}
		args = append(args, status)
		clauses = append(clauses, "status = $"+strconv.Itoa(len(args)))
	}
	if rawIssueClass := strings.TrimSpace(options.IssueClass); rawIssueClass != "" {
		issueClass := normalizeMemberWindowIssueClass(rawIssueClass)
		if issueClass == "" {
			return nil, errors.New("unsupported member window issue class")
		}
		args = append(args, issueClass)
		clauses = append(clauses, "issue_class = $"+strconv.Itoa(len(args)))
	}
	args = append(args, limit)

	rows, err := r.pool.Query(ctx, `
WITH member_windows AS (
    SELECT
        cm.tenant_id,
        cm.conversation_id,
        cm.user_id,
        cm.role,
        cm.status,
        cm.join_seq,
        cm.leave_seq,
        cm.member_version,
        cm.permission_version,
        c.member_version AS conversation_member_version,
        c.permission_version AS conversation_permission_version,
        c.status AS conversation_status,
        cm.updated_at,
        CASE
            WHEN cm.status = 'ACTIVE' AND (cm.join_seq IS NULL OR cm.join_seq <= 0) THEN 'ACTIVE_WITHOUT_JOIN_SEQ'
            WHEN cm.status = 'ACTIVE' AND cm.leave_seq IS NOT NULL THEN 'ACTIVE_WITH_LEAVE_SEQ'
            WHEN cm.status IN ('LEFT', 'BANNED') AND (cm.leave_seq IS NULL OR cm.leave_seq <= 0) THEN 'INACTIVE_WITHOUT_LEAVE_SEQ'
            WHEN cm.join_seq IS NOT NULL AND cm.leave_seq IS NOT NULL AND cm.leave_seq < cm.join_seq THEN 'LEAVE_BEFORE_JOIN'
            WHEN cm.member_version > c.member_version THEN 'MEMBER_VERSION_AHEAD_CONVERSATION'
            WHEN cm.permission_version > c.permission_version THEN 'PERMISSION_VERSION_AHEAD_CONVERSATION'
            WHEN c.status <> 'ACTIVE' AND cm.status = 'ACTIVE' THEN 'ACTIVE_MEMBER_IN_INACTIVE_CONVERSATION'
            ELSE ''
        END AS issue_class
    FROM conversation_members cm
    JOIN conversations c
      ON c.tenant_id = cm.tenant_id
     AND c.conversation_id = cm.conversation_id
),
active_owner_counts AS (
    SELECT
        c.tenant_id,
        c.conversation_id,
        c.member_version AS conversation_member_version,
        c.permission_version AS conversation_permission_version,
        c.status AS conversation_status,
        COUNT(*) FILTER (WHERE cm.status = 'ACTIVE' AND cm.role = 'OWNER') AS active_owner_count,
        COALESCE(MAX(cm.updated_at), now()) AS updated_at
    FROM conversations c
    LEFT JOIN conversation_members cm
      ON cm.tenant_id = c.tenant_id
     AND cm.conversation_id = c.conversation_id
    WHERE c.status = 'ACTIVE'
    GROUP BY c.tenant_id, c.conversation_id, c.member_version, c.permission_version, c.status
),
owner_count_issues AS (
    SELECT
        tenant_id,
        conversation_id,
        '' AS user_id,
        '' AS role,
        '' AS status,
        NULL::bigint AS join_seq,
        NULL::bigint AS leave_seq,
        0::bigint AS member_version,
        0::bigint AS permission_version,
        conversation_member_version,
        conversation_permission_version,
        conversation_status,
        updated_at,
        'ACTIVE_CONVERSATION_WITHOUT_OWNER' AS issue_class
    FROM active_owner_counts
    WHERE active_owner_count = 0
    UNION ALL
    SELECT
        cm.tenant_id,
        cm.conversation_id,
        cm.user_id,
        cm.role,
        cm.status,
        cm.join_seq,
        cm.leave_seq,
        cm.member_version,
        cm.permission_version,
        aoc.conversation_member_version,
        aoc.conversation_permission_version,
        aoc.conversation_status,
        cm.updated_at,
        'ACTIVE_CONVERSATION_WITH_MULTIPLE_OWNERS' AS issue_class
    FROM active_owner_counts aoc
    JOIN conversation_members cm
      ON cm.tenant_id = aoc.tenant_id
     AND cm.conversation_id = aoc.conversation_id
    WHERE aoc.active_owner_count > 1
      AND cm.status = 'ACTIVE'
      AND cm.role = 'OWNER'
),
all_issues AS (
    SELECT * FROM member_windows
    UNION ALL
    SELECT * FROM owner_count_issues
)
SELECT
    tenant_id,
    conversation_id,
    user_id,
    role,
    status,
    join_seq,
    leave_seq,
    member_version,
    permission_version,
    conversation_member_version,
    conversation_permission_version,
    conversation_status,
    issue_class,
    updated_at
FROM all_issues
WHERE `+strings.Join(clauses, " AND ")+`
ORDER BY updated_at DESC, tenant_id, conversation_id, user_id
LIMIT $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return nil, types.NewDBReadFailed(err.Error())
	}
	defer rows.Close()

	result := make([]MemberWindowAuditRow, 0, limit)
	for rows.Next() {
		var row MemberWindowAuditRow
		var joinSeq sql.NullInt64
		var leaveSeq sql.NullInt64
		if err := rows.Scan(
			&row.TenantID,
			&row.ConversationID,
			&row.UserID,
			&row.Role,
			&row.Status,
			&joinSeq,
			&leaveSeq,
			&row.MemberVersion,
			&row.PermissionVersion,
			&row.ConversationMemberVersion,
			&row.ConversationPermissionVersion,
			&row.ConversationStatus,
			&row.IssueClass,
			&row.UpdatedAt,
		); err != nil {
			return nil, types.NewDBReadFailed(err.Error())
		}
		row.JoinSeq = joinSeq.Int64
		row.HasJoinSeq = joinSeq.Valid
		row.LeaveSeq = leaveSeq.Int64
		row.HasLeaveSeq = leaveSeq.Valid
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, types.NewDBReadFailed(err.Error())
	}
	return result, nil
}

func normalizeMemberWindowAuditRole(value string) string {
	role := strings.ToUpper(strings.TrimSpace(value))
	switch types.MemberRole(role) {
	case types.MemberRoleOwner, types.MemberRoleAdmin, types.MemberRoleMember:
		return role
	default:
		return ""
	}
}

func normalizeMemberWindowAuditStatus(value string) string {
	status := strings.ToUpper(strings.TrimSpace(value))
	switch types.MemberStatus(status) {
	case types.MemberStatusActive, types.MemberStatusLeft, types.MemberStatusBanned:
		return status
	default:
		return ""
	}
}

func normalizeMemberWindowIssueClass(value string) string {
	issueClass := strings.ToUpper(strings.TrimSpace(value))
	switch issueClass {
	case "ACTIVE_WITHOUT_JOIN_SEQ",
		"ACTIVE_WITH_LEAVE_SEQ",
		"INACTIVE_WITHOUT_LEAVE_SEQ",
		"LEAVE_BEFORE_JOIN",
		"MEMBER_VERSION_AHEAD_CONVERSATION",
		"PERMISSION_VERSION_AHEAD_CONVERSATION",
		"ACTIVE_MEMBER_IN_INACTIVE_CONVERSATION",
		"ACTIVE_CONVERSATION_WITHOUT_OWNER",
		"ACTIVE_CONVERSATION_WITH_MULTIPLE_OWNERS":
		return issueClass
	default:
		return ""
	}
}
