package app

import (
	"context"

	"github.com/qsyy0921/IM/services/memory-service/internal/types"
)

type GetMemoryEventUseCase struct {
	repository MemoryQueryRepository
}

func NewGetMemoryEventUseCase(repository MemoryQueryRepository) *GetMemoryEventUseCase {
	return &GetMemoryEventUseCase{repository: repository}
}

func (usecase *GetMemoryEventUseCase) Execute(ctx context.Context, command types.GetMemoryEventCommand) (types.GetMemoryEventResult, error) {
	if err := command.Validate(); err != nil {
		return types.GetMemoryEventResult{}, err
	}
	if usecase == nil || usecase.repository == nil {
		return types.GetMemoryEventResult{}, types.ErrMemoryUnavailable
	}
	item, edges, err := usecase.repository.GetMemoryEvent(ctx, command)
	if err != nil {
		return types.GetMemoryEventResult{}, err
	}
	return types.GetMemoryEventResult{Item: item, GraphEdges: edges}, nil
}
