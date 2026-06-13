package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/qsyy0921/IM/services/message-service/internal/types"
)

func (r *MessageRepository) GetMessagePolicyContext(
	ctx context.Context,
	tenantID types.TenantID,
	conversationID types.ConversationID,
	messageID types.MessageID,
) (types.MessagePolicyContext, error) {
	if r.pool == nil {
		return types.MessagePolicyContext{}, ErrRepositoryNotConfigured
	}
	row := r.pool.QueryRow(ctx, `
SELECT sender_id
FROM message_log
WHERE tenant_id = $1
  AND conversation_id = $2
  AND message_id = $3
`, tenantID, conversationID, messageID)
	var context types.MessagePolicyContext
	if err := row.Scan(&context.SenderUserID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return types.MessagePolicyContext{}, types.NewMessageNotFound("message not found")
		}
		return types.MessagePolicyContext{}, types.NewDBWriteFailed(err.Error())
	}
	return context, nil
}
