package app

import (
	"context"

	"github.com/qsyy0921/IM/services/conversation-service/internal/types"
)

type UpdateConversationProfileUseCase struct {
	repository UpdateConversationProfileRepository
}

func NewUpdateConversationProfileUseCase(repository UpdateConversationProfileRepository) *UpdateConversationProfileUseCase {
	return &UpdateConversationProfileUseCase{repository: repository}
}

func (u *UpdateConversationProfileUseCase) Execute(
	ctx context.Context,
	command types.UpdateConversationProfileCommand,
) (types.ConversationProfileResult, error) {
	if err := command.Validate(); err != nil {
		return types.ConversationProfileResult{}, err
	}
	return u.repository.UpdateConversationProfile(ctx, command)
}
