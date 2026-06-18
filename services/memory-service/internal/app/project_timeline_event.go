package app

import (
	"context"

	"github.com/qsyy0921/IM/services/memory-service/internal/types"
)

type ProjectTimelineEventUseCase struct {
	repository TimelineProjectionRepository
}

func NewProjectTimelineEventUseCase(repository TimelineProjectionRepository) *ProjectTimelineEventUseCase {
	return &ProjectTimelineEventUseCase{repository: repository}
}

func (usecase *ProjectTimelineEventUseCase) Execute(ctx context.Context, command types.ProjectTimelineEventCommand) (types.ProjectTimelineEventResult, error) {
	if err := command.Validate(); err != nil {
		return types.ProjectTimelineEventResult{}, err
	}
	if usecase == nil || usecase.repository == nil {
		return types.ProjectTimelineEventResult{}, types.ErrMemoryUnavailable
	}
	return usecase.repository.ProjectTimelineEvent(ctx, command)
}
