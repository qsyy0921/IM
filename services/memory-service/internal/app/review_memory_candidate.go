package app

import (
	"context"

	"github.com/qsyy0921/IM/services/memory-service/internal/types"
)

type ReviewMemoryCandidateUseCase struct {
	repository MemoryQueryRepository
}

func NewReviewMemoryCandidateUseCase(repository MemoryQueryRepository) *ReviewMemoryCandidateUseCase {
	return &ReviewMemoryCandidateUseCase{repository: repository}
}

func (usecase *ReviewMemoryCandidateUseCase) Execute(ctx context.Context, command types.ReviewMemoryCandidateCommand) (types.ReviewMemoryCandidateResult, error) {
	if err := command.Validate(); err != nil {
		return types.ReviewMemoryCandidateResult{}, err
	}
	if usecase == nil || usecase.repository == nil {
		return types.ReviewMemoryCandidateResult{}, types.ErrMemoryUnavailable
	}
	item, err := usecase.repository.ReviewMemoryCandidate(ctx, command)
	if err != nil {
		return types.ReviewMemoryCandidateResult{}, err
	}
	return types.ReviewMemoryCandidateResult{Item: item}, nil
}
