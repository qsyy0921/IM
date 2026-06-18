package app

import (
	"context"

	"github.com/qsyy0921/IM/services/memory-service/internal/domain"
	"github.com/qsyy0921/IM/services/memory-service/internal/types"
)

type QueryMemoryEventsUseCase struct {
	repository MemoryQueryRepository
}

func NewQueryMemoryEventsUseCase(repository MemoryQueryRepository) *QueryMemoryEventsUseCase {
	return &QueryMemoryEventsUseCase{repository: repository}
}

func (usecase *QueryMemoryEventsUseCase) Execute(ctx context.Context, command types.QueryMemoryEventsCommand) (types.QueryMemoryEventsResult, error) {
	if err := command.Validate(); err != nil {
		return types.QueryMemoryEventsResult{}, err
	}
	if usecase == nil || usecase.repository == nil {
		return types.QueryMemoryEventsResult{}, types.ErrMemoryUnavailable
	}
	limit := command.EffectiveLimit()
	items, projectionVersion, err := usecase.repository.QueryMemoryEvents(ctx, command, limit)
	if err != nil {
		return types.QueryMemoryEventsResult{}, err
	}
	return domain.BuildQueryMemoryEventsResult(items, projectionVersion, limit), nil
}
