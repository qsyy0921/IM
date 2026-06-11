package app

import (
	"context"

	"github.com/qsyy0921/IM/services/receipt-service/internal/types"
)

type MuteConversationUseCase struct {
	repository ReceiptRepository
}

func NewMuteConversationUseCase(repository ReceiptRepository) *MuteConversationUseCase {
	return &MuteConversationUseCase{repository: repository}
}

func (useCase *MuteConversationUseCase) Execute(
	ctx context.Context,
	command types.MuteConversationCommand,
) (types.MuteConversationResult, error) {
	if err := command.Validate(); err != nil {
		return types.MuteConversationResult{}, err
	}
	return useCase.repository.MuteConversation(ctx, command)
}
