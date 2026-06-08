package domain

import (
	"time"

	"github.com/qsyy0921/IM/services/message-service/internal/types"
)

type AppendMessageInput struct {
	Command      types.SendMessageCommand
	Permission   types.PermissionDecision
	Conversation types.ConversationSendContext
}

type AppendMessageResult struct {
	MessageID        types.MessageID
	ConversationSeq  int64
	AcceptedAt       time.Time
	IdempotentReplay bool
}
