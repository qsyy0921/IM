package app

import (
	"context"

	"github.com/qsyy0921/IM/services/delivery-service/internal/types"
)

type AckDeliveryUseCase struct {
	repository DeliveryCursorRepository
}

func NewAckDeliveryUseCase(repository DeliveryCursorRepository) *AckDeliveryUseCase {
	return &AckDeliveryUseCase{repository: repository}
}

func (useCase *AckDeliveryUseCase) Execute(ctx context.Context, command types.AckDeliveryCommand) (types.AckDeliveryResult, error) {
	if err := command.Validate(); err != nil {
		return types.AckDeliveryResult{}, err
	}
	return useCase.repository.AckDelivery(ctx, command)
}
