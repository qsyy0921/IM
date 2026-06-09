package app

import (
	"context"

	"github.com/qsyy0921/IM/services/receipt-service/internal/types"
)

type ReceiptRepository interface {
	MarkRead(ctx context.Context, command types.MarkReadCommand) (types.MarkReadResult, error)
	GetReceiptState(ctx context.Context, command types.GetReceiptStateCommand) (types.GetReceiptStateResult, error)
	ListConversations(ctx context.Context, command types.ListConversationsCommand) (types.ListConversationsResult, error)
	ArchiveConversation(ctx context.Context, command types.ArchiveConversationCommand) (types.ArchiveConversationResult, error)
	PinConversation(ctx context.Context, command types.PinConversationCommand) (types.PinConversationResult, error)
}

type ReceiptAccessPort interface {
	CanMarkRead(ctx context.Context, auth types.AuthContext, conversationID types.ConversationID) (types.ReceiptAccessContext, error)
	CanViewReceiptState(ctx context.Context, auth types.AuthContext, conversationID types.ConversationID) (types.ReceiptAccessContext, error)
}

type DeliveryProjectionRepository interface {
	ProjectDeliveryEvent(ctx context.Context, command types.ProjectDeliveryEventCommand) (types.ProjectDeliveryEventResult, error)
}
