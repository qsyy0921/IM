package app

import (
	"context"

	"github.com/qsyy0921/IM/services/receipt-service/internal/types"
)

type GetReceiptStateUseCase struct {
	repository ReceiptRepository
	access     ReceiptAccessPort
}

func NewGetReceiptStateUseCase(repository ReceiptRepository, access ReceiptAccessPort) *GetReceiptStateUseCase {
	return &GetReceiptStateUseCase{repository: repository, access: access}
}

func (useCase *GetReceiptStateUseCase) Execute(ctx context.Context, command types.GetReceiptStateCommand) (types.GetReceiptStateResult, error) {
	if err := command.Validate(); err != nil {
		return types.GetReceiptStateResult{}, err
	}
	if useCase.access != nil {
		access, err := useCase.access.CanViewReceiptState(ctx, command.AuthContext, command.ConversationID)
		if err != nil {
			return types.GetReceiptStateResult{}, err
		}
		command.AccessContext = access
	}
	return useCase.repository.GetReceiptState(ctx, command)
}
