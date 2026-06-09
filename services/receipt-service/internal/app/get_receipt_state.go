package app

import (
	"context"

	"github.com/qsyy0921/IM/services/receipt-service/internal/types"
)

type GetReceiptStateUseCase struct {
	repository ReceiptRepository
}

func NewGetReceiptStateUseCase(repository ReceiptRepository) *GetReceiptStateUseCase {
	return &GetReceiptStateUseCase{repository: repository}
}

func (useCase *GetReceiptStateUseCase) Execute(ctx context.Context, command types.GetReceiptStateCommand) (types.GetReceiptStateResult, error) {
	if err := command.Validate(); err != nil {
		return types.GetReceiptStateResult{}, err
	}
	return useCase.repository.GetReceiptState(ctx, command)
}
