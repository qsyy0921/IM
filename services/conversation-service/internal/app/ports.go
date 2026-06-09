package app

import (
	"context"

	"github.com/qsyy0921/IM/services/conversation-service/internal/types"
)

type ConversationRepository interface {
	GetSendContext(ctx context.Context, command types.GetSendContextCommand) (types.ConversationSendContext, error)
}

type CreateMemberChangeRepository interface {
	CreateMemberChange(ctx context.Context, command types.CreateMemberChangeCommand) (types.MemberChangeResult, error)
}

type GetMemberChangeRepository interface {
	GetMemberChange(ctx context.Context, command types.GetMemberChangeCommand) (types.MemberChangeDetail, error)
}

type MemberChangeProgressRepository interface {
	MarkPublishedMemberChanges(ctx context.Context, limit int) (types.MemberChangePublishProgressStats, error)
}
