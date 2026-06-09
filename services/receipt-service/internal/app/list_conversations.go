package app

import (
	"context"

	"github.com/qsyy0921/IM/services/receipt-service/internal/types"
)

type ListConversationsUseCase struct {
	repository ReceiptRepository
}

func NewListConversationsUseCase(repository ReceiptRepository) *ListConversationsUseCase {
	return &ListConversationsUseCase{repository: repository}
}

func (useCase *ListConversationsUseCase) Execute(
	ctx context.Context,
	command types.ListConversationsCommand,
) (types.ListConversationsResult, error) {
	if err := command.Validate(); err != nil {
		return types.ListConversationsResult{}, err
	}
	return useCase.repository.ListConversations(ctx, command)
}
