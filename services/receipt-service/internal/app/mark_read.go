package app

import (
	"context"

	"github.com/qsyy0921/IM/services/receipt-service/internal/types"
)

type MarkReadUseCase struct {
	repository ReceiptRepository
	access     ReceiptAccessPort
}

func NewMarkReadUseCase(repository ReceiptRepository, access ReceiptAccessPort) *MarkReadUseCase {
	return &MarkReadUseCase{repository: repository, access: access}
}

func (useCase *MarkReadUseCase) Execute(ctx context.Context, command types.MarkReadCommand) (types.MarkReadResult, error) {
	if err := command.Validate(); err != nil {
		return types.MarkReadResult{}, err
	}
	if useCase.access != nil {
		access, err := useCase.access.CanMarkRead(ctx, command.AuthContext, command.ConversationID)
		if err != nil {
			return types.MarkReadResult{}, err
		}
		command.AccessContext = access
	}
	return useCase.repository.MarkRead(ctx, command)
}
