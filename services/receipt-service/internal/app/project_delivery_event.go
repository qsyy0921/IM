package app

import (
	"context"

	"github.com/qsyy0921/IM/services/receipt-service/internal/types"
)

type ProjectDeliveryEventUseCase struct {
	repository DeliveryProjectionRepository
}

func NewProjectDeliveryEventUseCase(repository DeliveryProjectionRepository) *ProjectDeliveryEventUseCase {
	return &ProjectDeliveryEventUseCase{repository: repository}
}

func (useCase *ProjectDeliveryEventUseCase) Execute(ctx context.Context, command types.ProjectDeliveryEventCommand) (types.ProjectDeliveryEventResult, error) {
	if err := command.Validate(); err != nil {
		return types.ProjectDeliveryEventResult{}, err
	}
	return useCase.repository.ProjectDeliveryEvent(ctx, command)
}
