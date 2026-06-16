package postgres

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/qsyy0921/IM/services/conversation-service/internal/types"
)

const (
	MemberWindowIssueActiveWithoutJoinSeq    = "ACTIVE_WITHOUT_JOIN_SEQ"
	MemberWindowIssueActiveWithLeaveSeq      = "ACTIVE_WITH_LEAVE_SEQ"
	MemberWindowIssueInactiveWithoutLeaveSeq = "INACTIVE_WITHOUT_LEAVE_SEQ"
	MemberWindowIssueLeaveBeforeJoin         = "LEAVE_BEFORE_JOIN"
	MemberWindowIssueMemberVersionAhead      = "MEMBER_VERSION_AHEAD_CONVERSATION"
	MemberWindowIssuePermissionVersionAhead  = "PERMISSION_VERSION_AHEAD_CONVERSATION"
	MemberWindowIssueActiveInInactiveConv    = "ACTIVE_MEMBER_IN_INACTIVE_CONVERSATION"

	memberWindowRepairActionSetActiveJoinSeq     = "set_active_join_seq_from_member_version"
	memberWindowRepairActionClearActiveLeaveSeq  = "clear_active_leave_seq"
	memberWindowRepairActionSetInactiveLeaveSeq  = "set_inactive_leave_seq"
	memberWindowRepairActionClampLeaveToJoinSeq  = "clamp_leave_to_join_seq"
	memberWindowRepairActionRaiseMemberVersion   = "raise_conversation_member_version"
	memberWindowRepairActionRaisePermVersion     = "raise_conversation_permission_version"
	memberWindowRepairActionMarkLeftInactiveConv = "mark_active_member_left_in_inactive_conversation"
	memberWindowRepairOutcomeAudited             = "AUDITED"
	memberWindowRepairOutcomeMutated             = "MUTATED"
	memberWindowRepairOutcomeSkipped             = "SKIPPED"
)

type MemberWindowRepairOptions struct {
	TenantID       string
	ConversationID string
	UserID         string
	IssueClass     string
	OperatorID     string
	Reason         string
	DryRun         bool
	Limit          int
}

type MemberWindowRepairStats struct {
	Requested int
	Repaired  int
	Skipped   int
	DryRun    bool
}

type MemberWindowRepairAuditOptions struct {
	TenantID       string
	ConversationID string
	UserID         string
	IssueClass     string
	Outcome        string
	RepairedAfter  *time.Time
	RepairedBefore *time.Time
	Limit          int
}

type MemberWindowRepairAuditRow struct {
	ID                           int64
	TenantID                     string
	ConversationID               string
	UserID                       string
	IssueClass                   string
	RepairAction                 string
	RepairOutcome                string
	PreviousJoinSeq              int64
	HasJoinSeq                   bool
	NewJoinSeq                   int64
	HasNewJoinSeq                bool
	PreviousLeaveSeq             int64
	HasLeaveSeq                  bool
	NewLeaveSeq                  int64
	HasNewLeaveSeq               bool
	PreviousMemberVersion        int64
	HasPreviousMemberVersion     bool
	NewMemberVersion             int64
	HasNewMemberVersion          bool
	ConversationStatus           string
	PreviousMemberStatus         string
	NewMemberStatus              string
	PreviousPermissionVersion    int64
	HasPreviousPermissionVersion bool
	NewPermissionVersion         int64
	HasNewPermissionVersion      bool
	OperatorID                   string
	Reason                       string
	DryRun                       bool
	RepairedAt                   time.Time
}

type memberWindowRepairCandidate struct {
	TenantID                  string
	ConversationID            string
	UserID                    string
	PreviousJoinSeq           sql.NullInt64
	NewJoinSeq                sql.NullInt64
	PreviousLeaveSeq          sql.NullInt64
	NewLeaveSeq               sql.NullInt64
	PreviousMemberVersion     sql.NullInt64
	NewMemberVersion          sql.NullInt64
	ConversationStatus        sql.NullString
	PreviousMemberStatus      sql.NullString
	NewMemberStatus           sql.NullString
	PreviousPermissionVersion sql.NullInt64
	NewPermissionVersion      sql.NullInt64
}

func (r *Repository) RepairMemberWindows(ctx context.Context, options MemberWindowRepairOptions) (MemberWindowRepairStats, error) {
	if r.pool == nil {
		return MemberWindowRepairStats{}, types.NewDBWriteFailed("repository is not configured")
	}
	issueClass := normalizeMemberWindowRepairIssueClass(options.IssueClass)
	if issueClass == "" {
		return MemberWindowRepairStats{}, types.NewInvalidArgument("unsupported member window repair issue class")
	}
	limit := normalizeMemberWindowRepairLimit(options.Limit)
	operatorID := normalizeMemberWindowRepairText(options.OperatorID, "manual")
	reason := normalizeMemberWindowRepairText(options.Reason, "manual member window repair")

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return MemberWindowRepairStats{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	candidates, err := selectMemberWindowRepairCandidates(ctx, tx, options, issueClass, limit)
	if err != nil {
		return MemberWindowRepairStats{}, err
	}
	stats := MemberWindowRepairStats{
		Requested: len(candidates),
		DryRun:    options.DryRun,
	}
	for _, candidate := range candidates {
		outcome := memberWindowRepairOutcomeAudited
		if !options.DryRun {
			updated, err := repairMemberWindowCandidate(ctx, tx, issueClass, candidate)
			if err != nil {
				return MemberWindowRepairStats{}, err
			}
			if updated {
				outcome = memberWindowRepairOutcomeMutated
				stats.Repaired++
			} else {
				outcome = memberWindowRepairOutcomeSkipped
				stats.Skipped++
			}
		}
		if options.DryRun {
			stats.Skipped++
		}
		if err := insertMemberWindowRepairAudit(ctx, tx, candidate, issueClass, outcome, operatorID, reason, options.DryRun); err != nil {
			return MemberWindowRepairStats{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return MemberWindowRepairStats{}, types.NewDBWriteFailed(err.Error())
	}
	return stats, nil
}

func (r *Repository) AuditMemberWindowRepairs(ctx context.Context, options MemberWindowRepairAuditOptions) ([]MemberWindowRepairAuditRow, error) {
	if r.pool == nil {
		return nil, types.NewDBReadFailed("repository is not configured")
	}
	if options.RepairedAfter != nil && options.RepairedBefore != nil && !options.RepairedAfter.Before(*options.RepairedBefore) {
		return nil, types.NewInvalidArgument("repaired_after must be before repaired_before")
	}
	limit := normalizeMemberWindowRepairLimit(options.Limit)
	args := make([]any, 0, 6)
	clauses := make([]string, 0, 5)
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
	if rawIssueClass := strings.TrimSpace(options.IssueClass); rawIssueClass != "" {
		issueClass := normalizeMemberWindowRepairIssueClass(rawIssueClass)
		if issueClass == "" {
			return nil, types.NewInvalidArgument("unsupported member window repair issue class")
		}
		args = append(args, issueClass)
		clauses = append(clauses, "issue_class = $"+strconv.Itoa(len(args)))
	}
	if rawOutcome := strings.TrimSpace(options.Outcome); rawOutcome != "" {
		outcome := normalizeMemberWindowRepairOutcome(rawOutcome)
		if outcome == "" {
			return nil, types.NewInvalidArgument("unsupported member window repair outcome")
		}
		args = append(args, outcome)
		clauses = append(clauses, "repair_outcome = $"+strconv.Itoa(len(args)))
	}
	if options.RepairedAfter != nil {
		args = append(args, options.RepairedAfter.UTC())
		clauses = append(clauses, "repaired_at >= $"+strconv.Itoa(len(args)))
	}
	if options.RepairedBefore != nil {
		args = append(args, options.RepairedBefore.UTC())
		clauses = append(clauses, "repaired_at < $"+strconv.Itoa(len(args)))
	}
	where := ""
	if len(clauses) > 0 {
		where = "WHERE " + strings.Join(clauses, " AND ")
	}
	args = append(args, limit)

	rows, err := r.pool.Query(ctx, `
SELECT
    id,
    tenant_id,
    conversation_id,
    user_id,
    issue_class,
    repair_action,
    repair_outcome,
	previous_join_seq,
	new_join_seq,
	previous_leave_seq,
	new_leave_seq,
    previous_member_version,
    new_member_version,
    conversation_status,
    previous_member_status,
    new_member_status,
    previous_permission_version,
    new_permission_version,
    operator_id,
    repair_reason,
    dry_run,
    repaired_at
FROM conversation_member_window_repair_audit
`+where+`
ORDER BY repaired_at DESC, id DESC
LIMIT $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return nil, types.NewDBReadFailed(err.Error())
	}
	defer rows.Close()

	result := make([]MemberWindowRepairAuditRow, 0, limit)
	for rows.Next() {
		var row MemberWindowRepairAuditRow
		var previousJoinSeq sql.NullInt64
		var newJoinSeq sql.NullInt64
		var previousLeaveSeq sql.NullInt64
		var newLeaveSeq sql.NullInt64
		var previousMemberVersion sql.NullInt64
		var newMemberVersion sql.NullInt64
		var conversationStatus sql.NullString
		var previousMemberStatus sql.NullString
		var newMemberStatus sql.NullString
		var previousPermissionVersion sql.NullInt64
		var newPermissionVersion sql.NullInt64
		if err := rows.Scan(
			&row.ID,
			&row.TenantID,
			&row.ConversationID,
			&row.UserID,
			&row.IssueClass,
			&row.RepairAction,
			&row.RepairOutcome,
			&previousJoinSeq,
			&newJoinSeq,
			&previousLeaveSeq,
			&newLeaveSeq,
			&previousMemberVersion,
			&newMemberVersion,
			&conversationStatus,
			&previousMemberStatus,
			&newMemberStatus,
			&previousPermissionVersion,
			&newPermissionVersion,
			&row.OperatorID,
			&row.Reason,
			&row.DryRun,
			&row.RepairedAt,
		); err != nil {
			return nil, types.NewDBReadFailed(err.Error())
		}
		row.PreviousJoinSeq = previousJoinSeq.Int64
		row.HasJoinSeq = previousJoinSeq.Valid
		row.NewJoinSeq = newJoinSeq.Int64
		row.HasNewJoinSeq = newJoinSeq.Valid
		row.PreviousLeaveSeq = previousLeaveSeq.Int64
		row.HasLeaveSeq = previousLeaveSeq.Valid
		row.NewLeaveSeq = newLeaveSeq.Int64
		row.HasNewLeaveSeq = newLeaveSeq.Valid
		row.PreviousMemberVersion = previousMemberVersion.Int64
		row.HasPreviousMemberVersion = previousMemberVersion.Valid
		row.NewMemberVersion = newMemberVersion.Int64
		row.HasNewMemberVersion = newMemberVersion.Valid
		row.ConversationStatus = conversationStatus.String
		row.PreviousMemberStatus = previousMemberStatus.String
		row.NewMemberStatus = newMemberStatus.String
		row.PreviousPermissionVersion = previousPermissionVersion.Int64
		row.HasPreviousPermissionVersion = previousPermissionVersion.Valid
		row.NewPermissionVersion = newPermissionVersion.Int64
		row.HasNewPermissionVersion = newPermissionVersion.Valid
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, types.NewDBReadFailed(err.Error())
	}
	return result, nil
}

func selectMemberWindowRepairCandidates(ctx context.Context, tx pgx.Tx, options MemberWindowRepairOptions, issueClass string, limit int) ([]memberWindowRepairCandidate, error) {
	args := make([]any, 0, 4)
	clauses := make([]string, 0, 8)
	selectNewLeaveSeq := "NULL::BIGINT"
	selectNewJoinSeq := "NULL::BIGINT"
	selectPreviousMemberVersion := "NULL::BIGINT"
	selectNewMemberVersion := "NULL::BIGINT"
	selectConversationStatus := "NULL::TEXT"
	selectPreviousMemberStatus := "NULL::TEXT"
	selectNewMemberStatus := "NULL::TEXT"
	selectPreviousPermissionVersion := "NULL::BIGINT"
	selectNewPermissionVersion := "NULL::BIGINT"
	fromClause := "conversation_members cm"
	distinctOn := ""
	orderBy := "cm.updated_at DESC, cm.tenant_id, cm.conversation_id, cm.user_id"
	lockClause := "FOR UPDATE OF cm"
	switch issueClass {
	case MemberWindowIssueActiveWithoutJoinSeq:
		fromClause = `conversation_members cm
JOIN conversations c
  ON c.tenant_id = cm.tenant_id
 AND c.conversation_id = cm.conversation_id`
		clauses = append(clauses,
			"c.status = 'ACTIVE'",
			"cm.status = 'ACTIVE'",
			"(cm.join_seq IS NULL OR cm.join_seq <= 0)",
			"(cm.leave_seq IS NULL OR cm.leave_seq <= 0)",
			"cm.member_version > 0",
			"cm.member_version <= c.member_version",
		)
		selectNewJoinSeq = "cm.member_version"
	case MemberWindowIssueActiveWithLeaveSeq:
		clauses = append(clauses,
			"cm.status = 'ACTIVE'",
			"cm.join_seq IS NOT NULL",
			"cm.join_seq > 0",
			"cm.leave_seq IS NOT NULL",
			"cm.leave_seq >= cm.join_seq",
		)
	case MemberWindowIssueInactiveWithoutLeaveSeq:
		clauses = append(clauses,
			"cm.status IN ('LEFT', 'BANNED')",
			"cm.join_seq IS NOT NULL",
			"cm.join_seq > 0",
			"(cm.leave_seq IS NULL OR cm.leave_seq <= 0)",
			"cm.member_version > 0",
			"cm.member_version >= cm.join_seq",
		)
		selectNewLeaveSeq = "cm.member_version"
	case MemberWindowIssueLeaveBeforeJoin:
		clauses = append(clauses,
			"cm.status IN ('LEFT', 'BANNED')",
			"cm.join_seq IS NOT NULL",
			"cm.join_seq > 0",
			"cm.leave_seq IS NOT NULL",
			"cm.leave_seq > 0",
			"cm.leave_seq < cm.join_seq",
			"cm.member_version >= cm.join_seq",
		)
		selectNewLeaveSeq = "cm.join_seq"
	case MemberWindowIssueMemberVersionAhead:
		fromClause = `conversation_members cm
JOIN conversations c
  ON c.tenant_id = cm.tenant_id
 AND c.conversation_id = cm.conversation_id`
		clauses = append(clauses,
			"cm.member_version > c.member_version",
			"cm.member_version > 0",
		)
		selectPreviousMemberVersion = "c.member_version"
		selectNewMemberVersion = "cm.member_version"
		distinctOn = "DISTINCT ON (cm.tenant_id, cm.conversation_id)"
		orderBy = "cm.tenant_id, cm.conversation_id, cm.member_version DESC, cm.updated_at DESC, cm.user_id"
		lockClause = ""
	case MemberWindowIssuePermissionVersionAhead:
		fromClause = `conversation_members cm
JOIN conversations c
  ON c.tenant_id = cm.tenant_id
 AND c.conversation_id = cm.conversation_id`
		clauses = append(clauses,
			"cm.permission_version > c.permission_version",
			"cm.permission_version > 0",
		)
		selectPreviousPermissionVersion = "c.permission_version"
		selectNewPermissionVersion = "cm.permission_version"
		distinctOn = "DISTINCT ON (cm.tenant_id, cm.conversation_id)"
		orderBy = "cm.tenant_id, cm.conversation_id, cm.permission_version DESC, cm.updated_at DESC, cm.user_id"
		lockClause = ""
	case MemberWindowIssueActiveInInactiveConv:
		fromClause = `conversation_members cm
JOIN conversations c
  ON c.tenant_id = cm.tenant_id
 AND c.conversation_id = cm.conversation_id`
		clauses = append(clauses,
			"c.status <> 'ACTIVE'",
			"cm.status = 'ACTIVE'",
			"cm.join_seq IS NOT NULL",
			"cm.join_seq > 0",
			"(cm.leave_seq IS NULL OR cm.leave_seq <= 0)",
			"cm.member_version > 0",
			"cm.member_version >= cm.join_seq",
		)
		selectNewLeaveSeq = "cm.member_version"
		selectConversationStatus = "c.status"
		selectPreviousMemberStatus = "cm.status"
		selectNewMemberStatus = "'LEFT'::TEXT"
	default:
		return nil, types.NewInvalidArgument("unsupported member window repair issue class")
	}
	if tenantID := strings.TrimSpace(options.TenantID); tenantID != "" {
		args = append(args, tenantID)
		clauses = append(clauses, "cm.tenant_id = $"+strconv.Itoa(len(args)))
	}
	if conversationID := strings.TrimSpace(options.ConversationID); conversationID != "" {
		args = append(args, conversationID)
		clauses = append(clauses, "cm.conversation_id = $"+strconv.Itoa(len(args)))
	}
	if userID := strings.TrimSpace(options.UserID); userID != "" {
		args = append(args, userID)
		clauses = append(clauses, "cm.user_id = $"+strconv.Itoa(len(args)))
	}
	args = append(args, limit)

	rows, err := tx.Query(ctx, `
SELECT `+distinctOn+`
    cm.tenant_id,
    cm.conversation_id,
    cm.user_id,
    cm.join_seq,
    `+selectNewJoinSeq+`,
    cm.leave_seq,
    `+selectNewLeaveSeq+`,
    `+selectPreviousMemberVersion+`,
    `+selectNewMemberVersion+`,
    `+selectConversationStatus+`,
    `+selectPreviousMemberStatus+`,
    `+selectNewMemberStatus+`,
    `+selectPreviousPermissionVersion+`,
    `+selectNewPermissionVersion+`
FROM `+fromClause+`
WHERE `+strings.Join(clauses, " AND ")+`
ORDER BY `+orderBy+`
LIMIT $`+strconv.Itoa(len(args))+`
`+lockClause+`
`, args...)
	if err != nil {
		return nil, types.NewDBWriteFailed(err.Error())
	}
	defer rows.Close()

	candidates := make([]memberWindowRepairCandidate, 0, limit)
	for rows.Next() {
		var candidate memberWindowRepairCandidate
		if err := rows.Scan(
			&candidate.TenantID,
			&candidate.ConversationID,
			&candidate.UserID,
			&candidate.PreviousJoinSeq,
			&candidate.NewJoinSeq,
			&candidate.PreviousLeaveSeq,
			&candidate.NewLeaveSeq,
			&candidate.PreviousMemberVersion,
			&candidate.NewMemberVersion,
			&candidate.ConversationStatus,
			&candidate.PreviousMemberStatus,
			&candidate.NewMemberStatus,
			&candidate.PreviousPermissionVersion,
			&candidate.NewPermissionVersion,
		); err != nil {
			return nil, types.NewDBWriteFailed(err.Error())
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, types.NewDBWriteFailed(err.Error())
	}
	return candidates, nil
}

func repairMemberWindowCandidate(ctx context.Context, tx pgx.Tx, issueClass string, candidate memberWindowRepairCandidate) (bool, error) {
	switch issueClass {
	case MemberWindowIssueActiveWithoutJoinSeq:
		return setActiveMemberJoinSeq(ctx, tx, candidate)
	case MemberWindowIssueActiveWithLeaveSeq:
		return clearActiveMemberLeaveSeq(ctx, tx, candidate)
	case MemberWindowIssueInactiveWithoutLeaveSeq:
		return setInactiveMemberLeaveSeq(ctx, tx, candidate)
	case MemberWindowIssueLeaveBeforeJoin:
		return clampInactiveMemberLeaveSeqToJoinSeq(ctx, tx, candidate)
	case MemberWindowIssueMemberVersionAhead:
		return raiseConversationMemberVersion(ctx, tx, candidate)
	case MemberWindowIssuePermissionVersionAhead:
		return raiseConversationPermissionVersion(ctx, tx, candidate)
	case MemberWindowIssueActiveInInactiveConv:
		return markActiveMemberLeftInInactiveConversation(ctx, tx, candidate)
	default:
		return false, types.NewInvalidArgument("unsupported member window repair issue class")
	}
}

func setActiveMemberJoinSeq(ctx context.Context, tx pgx.Tx, candidate memberWindowRepairCandidate) (bool, error) {
	if !candidate.NewJoinSeq.Valid || candidate.NewJoinSeq.Int64 <= 0 {
		return false, nil
	}
	tag, err := tx.Exec(ctx, `
UPDATE conversation_members cm
SET join_seq = $4,
    updated_at = now()
FROM conversations c
WHERE cm.tenant_id = $1
  AND cm.conversation_id = $2
  AND cm.user_id = $3
  AND c.tenant_id = cm.tenant_id
  AND c.conversation_id = cm.conversation_id
  AND c.status = 'ACTIVE'
  AND cm.status = 'ACTIVE'
  AND (cm.join_seq IS NULL OR cm.join_seq <= 0)
  AND (cm.leave_seq IS NULL OR cm.leave_seq <= 0)
  AND cm.member_version = $4
  AND cm.member_version <= c.member_version
`, candidate.TenantID, candidate.ConversationID, candidate.UserID, candidate.NewJoinSeq.Int64)
	if err != nil {
		return false, types.NewDBWriteFailed(err.Error())
	}
	return tag.RowsAffected() > 0, nil
}

func clearActiveMemberLeaveSeq(ctx context.Context, tx pgx.Tx, candidate memberWindowRepairCandidate) (bool, error) {
	tag, err := tx.Exec(ctx, `
UPDATE conversation_members
SET leave_seq = NULL,
    updated_at = now()
WHERE tenant_id = $1
  AND conversation_id = $2
  AND user_id = $3
  AND status = 'ACTIVE'
  AND join_seq IS NOT NULL
  AND join_seq > 0
  AND leave_seq IS NOT NULL
  AND leave_seq >= join_seq
`, candidate.TenantID, candidate.ConversationID, candidate.UserID)
	if err != nil {
		return false, types.NewDBWriteFailed(err.Error())
	}
	return tag.RowsAffected() > 0, nil
}

func setInactiveMemberLeaveSeq(ctx context.Context, tx pgx.Tx, candidate memberWindowRepairCandidate) (bool, error) {
	if !candidate.NewLeaveSeq.Valid || candidate.NewLeaveSeq.Int64 <= 0 {
		return false, nil
	}
	tag, err := tx.Exec(ctx, `
UPDATE conversation_members
SET leave_seq = $4,
    updated_at = now()
WHERE tenant_id = $1
  AND conversation_id = $2
  AND user_id = $3
  AND status IN ('LEFT', 'BANNED')
  AND join_seq IS NOT NULL
  AND join_seq > 0
  AND (leave_seq IS NULL OR leave_seq <= 0)
  AND member_version = $4
  AND member_version >= join_seq
`, candidate.TenantID, candidate.ConversationID, candidate.UserID, candidate.NewLeaveSeq.Int64)
	if err != nil {
		return false, types.NewDBWriteFailed(err.Error())
	}
	return tag.RowsAffected() > 0, nil
}

func clampInactiveMemberLeaveSeqToJoinSeq(ctx context.Context, tx pgx.Tx, candidate memberWindowRepairCandidate) (bool, error) {
	if !candidate.NewLeaveSeq.Valid || candidate.NewLeaveSeq.Int64 <= 0 {
		return false, nil
	}
	tag, err := tx.Exec(ctx, `
UPDATE conversation_members
SET leave_seq = $4,
    updated_at = now()
WHERE tenant_id = $1
  AND conversation_id = $2
  AND user_id = $3
  AND status IN ('LEFT', 'BANNED')
  AND join_seq = $4
  AND leave_seq IS NOT NULL
  AND leave_seq > 0
  AND leave_seq < join_seq
  AND member_version >= join_seq
`, candidate.TenantID, candidate.ConversationID, candidate.UserID, candidate.NewLeaveSeq.Int64)
	if err != nil {
		return false, types.NewDBWriteFailed(err.Error())
	}
	return tag.RowsAffected() > 0, nil
}

func raiseConversationMemberVersion(ctx context.Context, tx pgx.Tx, candidate memberWindowRepairCandidate) (bool, error) {
	if !candidate.NewMemberVersion.Valid || candidate.NewMemberVersion.Int64 <= 0 {
		return false, nil
	}
	tag, err := tx.Exec(ctx, `
UPDATE conversations
SET member_version = $3,
    updated_at = now()
WHERE tenant_id = $1
  AND conversation_id = $2
  AND member_version < $3
`, candidate.TenantID, candidate.ConversationID, candidate.NewMemberVersion.Int64)
	if err != nil {
		return false, types.NewDBWriteFailed(err.Error())
	}
	return tag.RowsAffected() > 0, nil
}

func raiseConversationPermissionVersion(ctx context.Context, tx pgx.Tx, candidate memberWindowRepairCandidate) (bool, error) {
	if !candidate.NewPermissionVersion.Valid || candidate.NewPermissionVersion.Int64 <= 0 {
		return false, nil
	}
	tag, err := tx.Exec(ctx, `
UPDATE conversations
SET permission_version = $3,
    updated_at = now()
WHERE tenant_id = $1
  AND conversation_id = $2
  AND permission_version < $3
`, candidate.TenantID, candidate.ConversationID, candidate.NewPermissionVersion.Int64)
	if err != nil {
		return false, types.NewDBWriteFailed(err.Error())
	}
	return tag.RowsAffected() > 0, nil
}

func markActiveMemberLeftInInactiveConversation(ctx context.Context, tx pgx.Tx, candidate memberWindowRepairCandidate) (bool, error) {
	if !candidate.NewLeaveSeq.Valid || candidate.NewLeaveSeq.Int64 <= 0 {
		return false, nil
	}
	tag, err := tx.Exec(ctx, `
UPDATE conversation_members cm
SET status = 'LEFT',
    leave_seq = $4,
    updated_at = now()
FROM conversations c
WHERE cm.tenant_id = $1
  AND cm.conversation_id = $2
  AND cm.user_id = $3
  AND c.tenant_id = cm.tenant_id
  AND c.conversation_id = cm.conversation_id
  AND c.status <> 'ACTIVE'
  AND cm.status = 'ACTIVE'
  AND cm.join_seq IS NOT NULL
  AND cm.join_seq > 0
  AND (cm.leave_seq IS NULL OR cm.leave_seq <= 0)
  AND cm.member_version = $4
  AND cm.member_version >= cm.join_seq
`, candidate.TenantID, candidate.ConversationID, candidate.UserID, candidate.NewLeaveSeq.Int64)
	if err != nil {
		return false, types.NewDBWriteFailed(err.Error())
	}
	return tag.RowsAffected() > 0, nil
}

func insertMemberWindowRepairAudit(
	ctx context.Context,
	tx pgx.Tx,
	candidate memberWindowRepairCandidate,
	issueClass string,
	outcome string,
	operatorID string,
	reason string,
	dryRun bool,
) error {
	action := memberWindowRepairActionForIssueClass(issueClass)
	newLeaveSeq := nullableSQLInt64(candidate.NewLeaveSeq)
	if issueClass == MemberWindowIssueActiveWithLeaveSeq {
		newLeaveSeq = nil
	}
	_, err := tx.Exec(ctx, `
INSERT INTO conversation_member_window_repair_audit (
    tenant_id,
    conversation_id,
    user_id,
    issue_class,
    repair_action,
    repair_outcome,
    previous_join_seq,
    new_join_seq,
    previous_leave_seq,
    new_leave_seq,
    previous_member_version,
    new_member_version,
    conversation_status,
    previous_member_status,
    new_member_status,
    previous_permission_version,
    new_permission_version,
    operator_id,
    repair_reason,
    dry_run
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)
`,
		candidate.TenantID,
		candidate.ConversationID,
		candidate.UserID,
		issueClass,
		action,
		outcome,
		nullableSQLInt64(candidate.PreviousJoinSeq),
		nullableSQLInt64(candidate.NewJoinSeq),
		nullableSQLInt64(candidate.PreviousLeaveSeq),
		newLeaveSeq,
		nullableSQLInt64(candidate.PreviousMemberVersion),
		nullableSQLInt64(candidate.NewMemberVersion),
		nullableSQLString(candidate.ConversationStatus),
		nullableSQLString(candidate.PreviousMemberStatus),
		nullableSQLString(candidate.NewMemberStatus),
		nullableSQLInt64(candidate.PreviousPermissionVersion),
		nullableSQLInt64(candidate.NewPermissionVersion),
		operatorID,
		reason,
		dryRun,
	)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func normalizeMemberWindowRepairIssueClass(value string) string {
	issueClass := strings.ToUpper(strings.TrimSpace(value))
	if issueClass == "" {
		return MemberWindowIssueActiveWithLeaveSeq
	}
	switch issueClass {
	case MemberWindowIssueActiveWithoutJoinSeq,
		MemberWindowIssueActiveWithLeaveSeq,
		MemberWindowIssueInactiveWithoutLeaveSeq,
		MemberWindowIssueLeaveBeforeJoin,
		MemberWindowIssueMemberVersionAhead,
		MemberWindowIssuePermissionVersionAhead,
		MemberWindowIssueActiveInInactiveConv:
		return issueClass
	default:
		return ""
	}
}

func memberWindowRepairActionForIssueClass(issueClass string) string {
	switch issueClass {
	case MemberWindowIssueActiveWithoutJoinSeq:
		return memberWindowRepairActionSetActiveJoinSeq
	case MemberWindowIssueInactiveWithoutLeaveSeq:
		return memberWindowRepairActionSetInactiveLeaveSeq
	case MemberWindowIssueLeaveBeforeJoin:
		return memberWindowRepairActionClampLeaveToJoinSeq
	case MemberWindowIssueMemberVersionAhead:
		return memberWindowRepairActionRaiseMemberVersion
	case MemberWindowIssuePermissionVersionAhead:
		return memberWindowRepairActionRaisePermVersion
	case MemberWindowIssueActiveInInactiveConv:
		return memberWindowRepairActionMarkLeftInactiveConv
	default:
		return memberWindowRepairActionClearActiveLeaveSeq
	}
}

func normalizeMemberWindowRepairOutcome(value string) string {
	outcome := strings.ToUpper(strings.TrimSpace(value))
	switch outcome {
	case memberWindowRepairOutcomeAudited, memberWindowRepairOutcomeMutated, memberWindowRepairOutcomeSkipped:
		return outcome
	default:
		return ""
	}
}

func normalizeMemberWindowRepairLimit(limit int) int {
	if limit <= 0 {
		return 20
	}
	if limit > 200 {
		return 200
	}
	return limit
}

func normalizeMemberWindowRepairText(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		value = fallback
	}
	if len(value) > 200 {
		value = value[:200]
	}
	return value
}

func nullableSQLInt64(value sql.NullInt64) any {
	if !value.Valid {
		return nil
	}
	return value.Int64
}

func nullableSQLString(value sql.NullString) any {
	if !value.Valid {
		return nil
	}
	return value.String
}

func ensureSupportedMemberWindowRepairIssueClass(value string) error {
	if normalizeMemberWindowRepairIssueClass(value) == "" {
		return errors.New("unsupported member window repair issue class")
	}
	return nil
}
