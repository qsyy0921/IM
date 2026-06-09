package app

import (
	"context"

	"github.com/qsyy0921/IM/services/receipt-service/internal/types"
)

type ArchiveConversationUseCase struct {
	repository ReceiptRepository
}

func NewArchiveConversationUseCase(repository ReceiptRepository) *ArchiveConversationUseCase {
	return &ArchiveConversationUseCase{repository: repository}
}

func (useCase *ArchiveConversationUseCase) Execute(
	ctx context.Context,
	command types.ArchiveConversationCommand,
) (types.ArchiveConversationResult, error) {
	if err := command.Validate(); err != nil {
		return types.ArchiveConversationResult{}, err
	}
	return useCase.repository.ArchiveConversation(ctx, command)
}
