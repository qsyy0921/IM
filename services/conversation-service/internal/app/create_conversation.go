package app

import (
	"context"

	"github.com/qsyy0921/IM/services/conversation-service/internal/types"
)

type CreateConversationUseCase struct {
	repository CreateConversationRepository
}

func NewCreateConversationUseCase(repository CreateConversationRepository) *CreateConversationUseCase {
	return &CreateConversationUseCase{repository: repository}
}

func (u *CreateConversationUseCase) Execute(
	ctx context.Context,
	command types.CreateConversationCommand,
) (types.CreateConversationResult, error) {
	if err := command.Validate(); err != nil {
		return types.CreateConversationResult{}, err
	}
	if u == nil || u.repository == nil {
		return types.CreateConversationResult{}, types.NewDBWriteFailed("repository is not configured")
	}
	return u.repository.CreateConversation(ctx, command)
}
