package app

import (
	"context"

	"github.com/qsyy0921/IM/services/policy-service/internal/types"
)

type ContactProjectionRepository interface {
	ProjectContactEvent(ctx context.Context, command types.ProjectContactEventCommand) (types.ProjectContactEventResult, error)
}

type ProjectContactEventUseCase struct {
	repository ContactProjectionRepository
}

func NewProjectContactEventUseCase(repository ContactProjectionRepository) *ProjectContactEventUseCase {
	return &ProjectContactEventUseCase{repository: repository}
}

func (useCase *ProjectContactEventUseCase) Execute(ctx context.Context, command types.ProjectContactEventCommand) (types.ProjectContactEventResult, error) {
	if err := command.Validate(); err != nil {
		return types.ProjectContactEventResult{}, err
	}
	return useCase.repository.ProjectContactEvent(ctx, command)
}
