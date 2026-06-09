package app

import (
	"context"

	"github.com/qsyy0921/IM/services/receipt-service/internal/types"
)

type PinConversationUseCase struct {
	repository ReceiptRepository
}

func NewPinConversationUseCase(repository ReceiptRepository) *PinConversationUseCase {
	return &PinConversationUseCase{repository: repository}
}

func (useCase *PinConversationUseCase) Execute(
	ctx context.Context,
	command types.PinConversationCommand,
) (types.PinConversationResult, error) {
	if err := command.Validate(); err != nil {
		return types.PinConversationResult{}, err
	}
	return useCase.repository.PinConversation(ctx, command)
}
