package app

import (
	"context"

	"github.com/qsyy0921/IM/services/conversation-service/internal/types"
)

type ConversationRepository interface {
	GetSendContext(ctx context.Context, command types.GetSendContextCommand) (types.ConversationSendContext, error)
}

type MemberChangeRepository interface {
	CreateMemberChange(ctx context.Context, command types.CreateMemberChangeCommand) (types.MemberChangeResult, error)
}
