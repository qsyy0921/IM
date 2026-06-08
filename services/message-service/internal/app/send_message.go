package app

import (
	"context"

	"github.com/qsyy0921/IM/services/message-service/internal/domain"
	"github.com/qsyy0921/IM/services/message-service/internal/types"
)

type SendMessageUseCase struct {
	policy        PolicyCheckPort
	conversation  ConversationQueryPort
	sequencer     SequencerPort
	messageRepo   MessageRepository
}

func NewSendMessageUseCase(
	policy PolicyCheckPort,
	conversation ConversationQueryPort,
	sequencer SequencerPort,
	messageRepo MessageRepository,
) *SendMessageUseCase {
	return &SendMessageUseCase{
		policy:       policy,
		conversation: conversation,
		sequencer:    sequencer,
		messageRepo:  messageRepo,
	}
}

func (u *SendMessageUseCase) Execute(ctx context.Context, command types.SendMessageCommand) (types.SendMessageResult, error) {
	if err := command.Validate(); err != nil {
		return types.SendMessageResult{}, err
	}

	permission, err := u.policy.CheckSendPermission(ctx, command)
	if err != nil {
		return types.SendMessageResult{}, err
	}
	if !permission.Allowed {
		return types.SendMessageResult{}, types.NewPermissionDenied(permission.Reason)
	}

	conversation, err := u.conversation.GetSendContext(ctx, command)
	if err != nil {
		return types.SendMessageResult{}, err
	}

	if conversation.ConversationMode != types.ConversationModeLocalRowLock {
		return types.SendMessageResult{}, types.NewSequencerUnavailable("sequencer mode is contract-only in phase 1")
	}

	result, err := u.messageRepo.AppendMessage(ctx, domain.AppendMessageInput{
		Command:          command,
		Permission:       permission,
		Conversation:     conversation,
	})
	if err != nil {
		return types.SendMessageResult{}, err
	}

	return types.SendMessageResult{
		MessageID:       result.MessageID,
		ConversationID:  command.ConversationID,
		ConversationSeq: result.ConversationSeq,
		AcceptedAt:      result.AcceptedAt,
		IdempotentReplay: result.IdempotentReplay,
	}, nil
}
