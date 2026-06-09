package app

import (
	"context"

	"github.com/qsyy0921/IM/services/message-service/internal/domain"
	"github.com/qsyy0921/IM/services/message-service/internal/types"
)

type PolicyCheckPort interface {
	CheckSendPermission(ctx context.Context, command types.SendMessageCommand) (types.PermissionDecision, error)
	CheckRevokePermission(ctx context.Context, command types.RevokeMessageCommand) (types.PermissionDecision, error)
}

type ConversationQueryPort interface {
	GetSendContext(ctx context.Context, command types.SendMessageCommand) (types.ConversationSendContext, error)
}

type SequencerPort interface {
	AllocateSeqBlock(ctx context.Context, command types.SendMessageCommand) (types.SeqBlock, error)
}

type MessageRepository interface {
	AppendMessage(ctx context.Context, input domain.AppendMessageInput) (domain.AppendMessageResult, error)
	RevokeMessage(ctx context.Context, input domain.RevokeMessageInput) (domain.MessageChangeResult, error)
}

type AdmissionPort interface {
	AdmitSendMessage(ctx context.Context) (types.AdmissionPermit, error)
}
