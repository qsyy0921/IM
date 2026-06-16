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
	MemberWindowIssueActiveWithLeaveSeq = "ACTIVE_WITH_LEAVE_SEQ"

	memberWindowRepairActionClearActiveLeaveSeq = "clear_active_leave_seq"
	memberWindowRepairOutcomeAudited            = "AUDITED"
	memberWindowRepairOutcomeMutated            = "MUTATED"
	memberWindowRepairOutcomeSkipped            = "SKIPPED"
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
	Limit          int
}

type MemberWindowRepairAuditRow struct {
	ID               int64
	TenantID         string
	ConversationID   string
	UserID           string
	IssueClass       string
	RepairAction     string
	RepairOutcome    string
	PreviousJoinSeq  int64
	HasJoinSeq       bool
	PreviousLeaveSeq int64
	HasLeaveSeq      bool
	NewLeaveSeq      int64
	HasNewLeaveSeq   bool
	OperatorID       string
	Reason           string
	DryRun           bool
	RepairedAt       time.Time
}

type memberWindowRepairCandidate struct {
	TenantID         string
	ConversationID   string
	UserID           string
	PreviousJoinSeq  sql.NullInt64
	PreviousLeaveSeq sql.NullInt64
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
			updated, err := clearActiveMemberLeaveSeq(ctx, tx, candidate)
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
    previous_leave_seq,
    new_leave_seq,
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
		var previousLeaveSeq sql.NullInt64
		var newLeaveSeq sql.NullInt64
		if err := rows.Scan(
			&row.ID,
			&row.TenantID,
			&row.ConversationID,
			&row.UserID,
			&row.IssueClass,
			&row.RepairAction,
			&row.RepairOutcome,
			&previousJoinSeq,
			&previousLeaveSeq,
			&newLeaveSeq,
			&row.OperatorID,
			&row.Reason,
			&row.DryRun,
			&row.RepairedAt,
		); err != nil {
			return nil, types.NewDBReadFailed(err.Error())
		}
		row.PreviousJoinSeq = previousJoinSeq.Int64
		row.HasJoinSeq = previousJoinSeq.Valid
		row.PreviousLeaveSeq = previousLeaveSeq.Int64
		row.HasLeaveSeq = previousLeaveSeq.Valid
		row.NewLeaveSeq = newLeaveSeq.Int64
		row.HasNewLeaveSeq = newLeaveSeq.Valid
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, types.NewDBReadFailed(err.Error())
	}
	return result, nil
}

func selectMemberWindowRepairCandidates(ctx context.Context, tx pgx.Tx, options MemberWindowRepairOptions, issueClass string, limit int) ([]memberWindowRepairCandidate, error) {
	if issueClass != MemberWindowIssueActiveWithLeaveSeq {
		return nil, types.NewInvalidArgument("unsupported member window repair issue class")
	}
	args := make([]any, 0, 4)
	clauses := []string{
		"status = 'ACTIVE'",
		"join_seq IS NOT NULL",
		"join_seq > 0",
		"leave_seq IS NOT NULL",
		"leave_seq >= join_seq",
	}
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
	args = append(args, limit)

	rows, err := tx.Query(ctx, `
SELECT tenant_id, conversation_id, user_id, join_seq, leave_seq
FROM conversation_members
WHERE `+strings.Join(clauses, " AND ")+`
ORDER BY updated_at DESC, tenant_id, conversation_id, user_id
LIMIT $`+strconv.Itoa(len(args))+`
FOR UPDATE
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
			&candidate.PreviousLeaveSeq,
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
	_, err := tx.Exec(ctx, `
INSERT INTO conversation_member_window_repair_audit (
    tenant_id,
    conversation_id,
    user_id,
    issue_class,
    repair_action,
    repair_outcome,
    previous_join_seq,
    previous_leave_seq,
    new_leave_seq,
    operator_id,
    repair_reason,
    dry_run
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NULL, $9, $10, $11)
`,
		candidate.TenantID,
		candidate.ConversationID,
		candidate.UserID,
		issueClass,
		memberWindowRepairActionClearActiveLeaveSeq,
		outcome,
		nullableSQLInt64(candidate.PreviousJoinSeq),
		nullableSQLInt64(candidate.PreviousLeaveSeq),
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
	if issueClass == MemberWindowIssueActiveWithLeaveSeq {
		return issueClass
	}
	return ""
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

func ensureSupportedMemberWindowRepairIssueClass(value string) error {
	if normalizeMemberWindowRepairIssueClass(value) == "" {
		return errors.New("unsupported member window repair issue class")
	}
	return nil
}
