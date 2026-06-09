package app

import (
	"context"

	"github.com/qsyy0921/IM/services/delivery-service/internal/types"
)

type ProjectTimelineEventUseCase struct {
	repository TimelineProjectionRepository
}

func NewProjectTimelineEventUseCase(repository TimelineProjectionRepository) *ProjectTimelineEventUseCase {
	return &ProjectTimelineEventUseCase{repository: repository}
}

func (useCase *ProjectTimelineEventUseCase) Execute(
	ctx context.Context,
	command types.ProjectTimelineEventCommand,
) (types.ProjectTimelineEventResult, error) {
	if err := command.Validate(); err != nil {
		return types.ProjectTimelineEventResult{}, err
	}
	return useCase.repository.ProjectTimelineEvent(ctx, command)
}
