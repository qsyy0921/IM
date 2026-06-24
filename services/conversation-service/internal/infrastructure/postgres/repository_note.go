package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/qsyy0921/IM/services/conversation-service/internal/types"
)

func (r *Repository) CreateConversationNote(
	ctx context.Context,
	command types.CreateConversationNoteCommand,
) (types.ConversationNoteResult, error) {
	if r.pool == nil {
		return types.ConversationNoteResult{}, types.NewDBWriteFailed("repository is not configured")
	}
	if err := command.Validate(); err != nil {
		return types.ConversationNoteResult{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return types.ConversationNoteResult{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	var conversationStatus types.ConversationStatus
	var authStatus types.MemberStatus
	if err := tx.QueryRow(ctx, `
SELECT
    c.status,
    COALESCE(auth_member.status, '')
FROM conversations c
LEFT JOIN conversation_members auth_member
  ON auth_member.tenant_id = c.tenant_id
 AND auth_member.conversation_id = c.conversation_id
 AND auth_member.user_id = $3
WHERE c.tenant_id = $1
  AND c.conversation_id = $2
FOR UPDATE OF c
`, command.AuthContext.TenantID, command.ConversationID, command.AuthContext.UserID).Scan(
		&conversationStatus,
		&authStatus,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return types.ConversationNoteResult{}, types.NewConversationNotFound("conversation not found")
		}
		return types.ConversationNoteResult{}, types.NewDBReadFailed(err.Error())
	}
	if conversationStatus != types.ConversationStatusActive {
		return types.ConversationNoteResult{}, types.NewConversationNotFound("conversation not found")
	}
	if authStatus != types.MemberStatusActive {
		return types.ConversationNoteResult{}, types.NewMemberNotActive("conversation member is not active")
	}

	result, found, err := queryConversationNoteByIdempotency(ctx, tx, command)
	if err != nil {
		return types.ConversationNoteResult{}, types.NewDBReadFailed(err.Error())
	}
	if found {
		if err := tx.Commit(ctx); err != nil {
			return types.ConversationNoteResult{}, types.NewDBWriteFailed(err.Error())
		}
		result.IdempotentReplay = true
		return result, nil
	}

	noteID, err := newUUIDString()
	if err != nil {
		return types.ConversationNoteResult{}, types.NewDBWriteFailed(err.Error())
	}
	result.NoteID = types.NoteID("cnote_" + noteID)
	if err := tx.QueryRow(ctx, `
INSERT INTO conversation_notes (
    tenant_id,
    conversation_id,
    note_id,
    author_user_id,
    body,
    source_tool_name,
    source_proposal_id,
    source_approval_id,
    source_execution_id,
    idempotency_key,
    created_at,
    updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $11)
RETURNING
    tenant_id,
    conversation_id,
    note_id,
    author_user_id,
    body,
    source_tool_name,
    source_proposal_id,
    source_approval_id,
    source_execution_id,
    created_at
`,
		command.AuthContext.TenantID,
		command.ConversationID,
		result.NoteID,
		command.AuthContext.UserID,
		command.NormalizedBody(),
		command.NormalizedSourceToolName(),
		command.NormalizedSourceProposalID(),
		command.NormalizedSourceApprovalID(),
		command.NormalizedSourceExecutionID(),
		command.NormalizedIdempotencyKey(),
		r.now(),
	).Scan(
		&result.TenantID,
		&result.ConversationID,
		&result.NoteID,
		&result.AuthorUserID,
		&result.Body,
		&result.SourceToolName,
		&result.SourceProposalID,
		&result.SourceApprovalID,
		&result.SourceExecutionID,
		&result.CreatedAt,
	); err != nil {
		return types.ConversationNoteResult{}, types.NewDBWriteFailed(err.Error())
	}
	if err := tx.Commit(ctx); err != nil {
		return types.ConversationNoteResult{}, types.NewDBWriteFailed(err.Error())
	}
	return result, nil
}

type noteQueryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func queryConversationNoteByIdempotency(
	ctx context.Context,
	queryer noteQueryer,
	command types.CreateConversationNoteCommand,
) (types.ConversationNoteResult, bool, error) {
	var result types.ConversationNoteResult
	err := queryer.QueryRow(ctx, `
SELECT
    tenant_id,
    conversation_id,
    note_id,
    author_user_id,
    body,
    source_tool_name,
    source_proposal_id,
    source_approval_id,
    source_execution_id,
    created_at
FROM conversation_notes
WHERE tenant_id = $1
  AND conversation_id = $2
  AND author_user_id = $3
  AND idempotency_key = $4
`, command.AuthContext.TenantID, command.ConversationID, command.AuthContext.UserID, command.NormalizedIdempotencyKey()).Scan(
		&result.TenantID,
		&result.ConversationID,
		&result.NoteID,
		&result.AuthorUserID,
		&result.Body,
		&result.SourceToolName,
		&result.SourceProposalID,
		&result.SourceApprovalID,
		&result.SourceExecutionID,
		&result.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return types.ConversationNoteResult{}, false, nil
	}
	if err != nil {
		return types.ConversationNoteResult{}, false, err
	}
	return result, true, nil
}
