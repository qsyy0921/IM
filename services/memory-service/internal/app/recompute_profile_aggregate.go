package app

import (
	"context"

	"github.com/qsyy0921/IM/services/memory-service/internal/types"
)

type RecomputeProfileAggregateUseCase struct {
	repository MemoryQueryRepository
}

func NewRecomputeProfileAggregateUseCase(repository MemoryQueryRepository) *RecomputeProfileAggregateUseCase {
	return &RecomputeProfileAggregateUseCase{repository: repository}
}

func (usecase *RecomputeProfileAggregateUseCase) Execute(ctx context.Context, command types.RecomputeProfileAggregateCommand) (types.RecomputeProfileAggregateResult, error) {
	if err := command.Validate(); err != nil {
		return types.RecomputeProfileAggregateResult{}, err
	}
	if usecase == nil || usecase.repository == nil {
		return types.RecomputeProfileAggregateResult{}, types.ErrMemoryUnavailable
	}
	item, supportCount, active, err := usecase.repository.RecomputeProfileAggregate(ctx, command)
	if err != nil {
		return types.RecomputeProfileAggregateResult{}, err
	}
	return types.RecomputeProfileAggregateResult{
		Item:         item,
		SupportCount: supportCount,
		Active:       active,
	}, nil
}
