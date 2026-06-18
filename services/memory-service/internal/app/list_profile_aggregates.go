package app

import (
	"context"

	"github.com/qsyy0921/IM/services/memory-service/internal/domain"
	"github.com/qsyy0921/IM/services/memory-service/internal/types"
)

type ListProfileAggregatesUseCase struct {
	repository MemoryQueryRepository
}

func NewListProfileAggregatesUseCase(repository MemoryQueryRepository) *ListProfileAggregatesUseCase {
	return &ListProfileAggregatesUseCase{repository: repository}
}

func (usecase *ListProfileAggregatesUseCase) Execute(ctx context.Context, command types.ListProfileAggregatesCommand) (types.ListProfileAggregatesResult, error) {
	if err := command.Validate(); err != nil {
		return types.ListProfileAggregatesResult{}, err
	}
	if usecase == nil || usecase.repository == nil {
		return types.ListProfileAggregatesResult{}, types.ErrMemoryUnavailable
	}
	limit := command.EffectiveLimit()
	items, err := usecase.repository.ListProfileAggregates(ctx, command, limit)
	if err != nil {
		return types.ListProfileAggregatesResult{}, err
	}
	return domain.BuildListProfileAggregatesResult(items, limit), nil
}
