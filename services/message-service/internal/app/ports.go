package app

import (
	"context"

	"github.com/qsyy0921/IM/services/message-service/internal/domain"
	"github.com/qsyy0921/IM/services/message-service/internal/types"
)

type PolicyCheckPort interface {
	CheckSendPermission(ctx context.Context, command types.SendMessageCommand, conversation types.ConversationSendContext) (types.PermissionDecision, error)
	CheckEditPermission(ctx context.Context, command types.EditMessageCommand, conversation types.ConversationSendContext, message types.MessagePolicyContext) (types.PermissionDecision, error)
	CheckRevokePermission(ctx context.Context, command types.RevokeMessageCommand, conversation types.ConversationSendContext, message types.MessagePolicyContext) (types.PermissionDecision, error)
	CheckDeletePermission(ctx context.Context, command types.DeleteMessageCommand, conversation types.ConversationSendContext, message types.MessagePolicyContext) (types.PermissionDecision, error)
}

type ConversationQueryPort interface {
	GetSendContext(ctx context.Context, command types.SendMessageCommand) (types.ConversationSendContext, error)
}

type SequencerPort interface {
	AllocateSeqBlock(ctx context.Context, command types.SendMessageCommand, minimumStartSeq int64) (types.SeqBlock, error)
}

type MessageRepository interface {
	NextConversationSeqFloor(ctx context.Context, tenantID types.TenantID, conversationID types.ConversationID) (int64, error)
	AppendMessage(ctx context.Context, input domain.AppendMessageInput) (domain.AppendMessageResult, error)
	GetMessagePolicyContext(ctx context.Context, tenantID types.TenantID, conversationID types.ConversationID, messageID types.MessageID) (types.MessagePolicyContext, error)
	EditMessage(ctx context.Context, input domain.EditMessageInput) (domain.MessageChangeResult, error)
	RevokeMessage(ctx context.Context, input domain.RevokeMessageInput) (domain.MessageChangeResult, error)
	DeleteMessage(ctx context.Context, input domain.DeleteMessageInput) (domain.MessageChangeResult, error)
}

type AdmissionPort interface {
	AdmitSendMessage(ctx context.Context) (types.AdmissionPermit, error)
}
