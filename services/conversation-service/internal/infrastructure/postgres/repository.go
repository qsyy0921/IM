package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/conversation-service/internal/domain"
	"github.com/qsyy0921/IM/services/conversation-service/internal/types"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) GetSendContext(
	ctx context.Context,
	command types.GetSendContextCommand,
) (types.ConversationSendContext, error) {
	row := r.pool.QueryRow(ctx, `
SELECT
    c.status,
    c.conversation_mode,
    c.fanout_mode,
    c.fanout_policy_version,
    c.member_version,
    c.permission_version,
    c.current_seq_shard,
    COALESCE(m.status, ''),
    COALESCE(m.member_version, 0),
    COALESCE(m.permission_version, 0)
FROM conversations c
LEFT JOIN conversation_members m
  ON m.tenant_id = c.tenant_id
 AND m.conversation_id = c.conversation_id
 AND m.user_id = $3
WHERE c.tenant_id = $1
  AND c.conversation_id = $2
`, command.TenantID, command.ConversationID, command.UserID)

	var conversation domain.Conversation
	var member domain.Member
	conversation.TenantID = command.TenantID
	conversation.ConversationID = command.ConversationID
	member.UserID = command.UserID
	if err := row.Scan(
		&conversation.Status,
		&conversation.ConversationMode,
		&conversation.FanoutMode,
		&conversation.FanoutPolicyVersion,
		&conversation.MemberVersion,
		&conversation.PermissionVersion,
		&conversation.CurrentSeqShard,
		&member.Status,
		&member.MemberVersion,
		&member.PermissionVersion,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return types.ConversationSendContext{}, types.NewConversationNotFound("conversation not found")
		}
		return types.ConversationSendContext{}, types.NewDBReadFailed(err.Error())
	}
	return domain.BuildSendContext(conversation, member)
}
