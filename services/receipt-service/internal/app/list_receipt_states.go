package app

import (
	"context"

	"github.com/qsyy0921/IM/services/receipt-service/internal/types"
)

type ListReceiptStatesUseCase struct {
	repository ReceiptRepository
	access     ReceiptAccessPort
}

func NewListReceiptStatesUseCase(repository ReceiptRepository, access ReceiptAccessPort) *ListReceiptStatesUseCase {
	return &ListReceiptStatesUseCase{repository: repository, access: access}
}

func (useCase *ListReceiptStatesUseCase) Execute(
	ctx context.Context,
	command types.ListReceiptStatesCommand,
) (types.ListReceiptStatesResult, error) {
	if err := command.Validate(); err != nil {
		return types.ListReceiptStatesResult{}, err
	}
	if useCase.access != nil {
		access, err := useCase.access.CanViewReceiptState(ctx, command.AuthContext, command.ConversationID)
		if err != nil {
			return types.ListReceiptStatesResult{}, err
		}
		command.AccessContext = access
	}
	return useCase.repository.ListReceiptStates(ctx, command)
}
