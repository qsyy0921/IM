package app

import (
	"context"

	"github.com/qsyy0921/IM/services/conversation-service/internal/types"
)

type GetConversationProfileUseCase struct {
	repository GetConversationProfileRepository
}

func NewGetConversationProfileUseCase(repository GetConversationProfileRepository) *GetConversationProfileUseCase {
	return &GetConversationProfileUseCase{repository: repository}
}

func (u *GetConversationProfileUseCase) Execute(
	ctx context.Context,
	command types.GetConversationProfileCommand,
) (types.ConversationProfileResult, error) {
	if err := command.Validate(); err != nil {
		return types.ConversationProfileResult{}, err
	}
	return u.repository.GetConversationProfile(ctx, command)
}
