package postgres

import (
	"context"

	"github.com/qsyy0921/IM/services/message-service/internal/domain"
)

type MessageRepository struct{}

func NewMessageRepository() *MessageRepository {
	return &MessageRepository{}
}

func (r *MessageRepository) AppendMessage(ctx context.Context, input domain.AppendMessageInput) (domain.AppendMessageResult, error) {
	return domain.AppendMessageResult{}, ErrNotImplemented
}
