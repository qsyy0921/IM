package app

import (
	"context"

	"github.com/qsyy0921/IM/services/push-gateway/internal/types"
)

type SubscribeConversationUseCase struct {
	registry SessionRegistry
}

func NewSubscribeConversationUseCase(registry SessionRegistry) *SubscribeConversationUseCase {
	return &SubscribeConversationUseCase{registry: registry}
}

func (usecase *SubscribeConversationUseCase) Execute(
	ctx context.Context,
	command types.ConversationSubscriptionCommand,
) (types.ConversationSubscriptionResult, error) {
	if err := command.AuthContext.Validate(); err != nil {
		return types.ConversationSubscriptionResult{}, err
	}
	if command.ConversationID == "" {
		return types.ConversationSubscriptionResult{}, types.NewInvalidFrame("conversation_id is required")
	}
	return usecase.registry.SubscribeConversation(ctx, command)
}

type UnsubscribeConversationUseCase struct {
	registry SessionRegistry
}

func NewUnsubscribeConversationUseCase(registry SessionRegistry) *UnsubscribeConversationUseCase {
	return &UnsubscribeConversationUseCase{registry: registry}
}

func (usecase *UnsubscribeConversationUseCase) Execute(
	ctx context.Context,
	command types.ConversationSubscriptionCommand,
) (types.ConversationSubscriptionResult, error) {
	if err := command.AuthContext.Validate(); err != nil {
		return types.ConversationSubscriptionResult{}, err
	}
	if command.ConversationID == "" {
		return types.ConversationSubscriptionResult{}, types.NewInvalidFrame("conversation_id is required")
	}
	return usecase.registry.UnsubscribeConversation(ctx, command)
}
