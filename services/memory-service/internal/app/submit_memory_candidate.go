package app

import (
	"context"

	"github.com/qsyy0921/IM/services/memory-service/internal/types"
)

type SubmitMemoryCandidateUseCase struct {
	repository MemoryQueryRepository
}

func NewSubmitMemoryCandidateUseCase(repository MemoryQueryRepository) *SubmitMemoryCandidateUseCase {
	return &SubmitMemoryCandidateUseCase{repository: repository}
}

func (usecase *SubmitMemoryCandidateUseCase) Execute(ctx context.Context, command types.SubmitMemoryCandidateCommand) (types.SubmitMemoryCandidateResult, error) {
	if err := command.Validate(); err != nil {
		return types.SubmitMemoryCandidateResult{}, err
	}
	if usecase == nil || usecase.repository == nil {
		return types.SubmitMemoryCandidateResult{}, types.ErrMemoryUnavailable
	}
	item, err := usecase.repository.SubmitMemoryCandidate(ctx, command)
	if err != nil {
		return types.SubmitMemoryCandidateResult{}, err
	}
	return types.SubmitMemoryCandidateResult{Item: item}, nil
}
