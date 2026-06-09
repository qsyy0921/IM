package app

import (
	"context"

	"github.com/qsyy0921/IM/services/receipt-service/internal/types"
)

type MarkReadUseCase struct {
	repository ReceiptRepository
}

func NewMarkReadUseCase(repository ReceiptRepository) *MarkReadUseCase {
	return &MarkReadUseCase{repository: repository}
}

func (useCase *MarkReadUseCase) Execute(ctx context.Context, command types.MarkReadCommand) (types.MarkReadResult, error) {
	if err := command.Validate(); err != nil {
		return types.MarkReadResult{}, err
	}
	return useCase.repository.MarkRead(ctx, command)
}
