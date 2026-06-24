package app

import (
	"context"

	"github.com/qsyy0921/IM/services/conversation-service/internal/types"
)

type CreateConversationNoteUseCase struct {
	repository CreateConversationNoteRepository
}

func NewCreateConversationNoteUseCase(repository CreateConversationNoteRepository) CreateConversationNoteUseCase {
	return CreateConversationNoteUseCase{repository: repository}
}

func (useCase CreateConversationNoteUseCase) Execute(
	ctx context.Context,
	command types.CreateConversationNoteCommand,
) (types.ConversationNoteResult, error) {
	if err := command.Validate(); err != nil {
		return types.ConversationNoteResult{}, err
	}
	if useCase.repository == nil {
		return types.ConversationNoteResult{}, types.NewDBWriteFailed("repository is not configured")
	}
	return useCase.repository.CreateConversationNote(ctx, command)
}
