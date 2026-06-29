package app

import (
	"context"
	"time"

	"github.com/qsyy0921/IM/services/timeline-service/internal/types"
)

type AllocateSeqBlockUseCase struct {
	repository   SeqBlockRepository
	maxBlockSize int
	leaseTTL     time.Duration
}

func NewAllocateSeqBlockUseCase(
	repository SeqBlockRepository,
	maxBlockSize int,
	leaseTTL time.Duration,
) *AllocateSeqBlockUseCase {
	return &AllocateSeqBlockUseCase{
		repository:   repository,
		maxBlockSize: maxBlockSize,
		leaseTTL:     leaseTTL,
	}
}

func (useCase *AllocateSeqBlockUseCase) Execute(
	ctx context.Context,
	command types.AllocateSeqBlockCommand,
) (types.SeqBlockLease, error) {
	if useCase == nil || useCase.repository == nil {
		return types.SeqBlockLease{}, types.NewDBWriteFailed("seq block repository is not configured")
	}
	if err := command.Validate(useCase.maxBlockSize); err != nil {
		return types.SeqBlockLease{}, err
	}
	leaseTTL := useCase.leaseTTL
	if leaseTTL <= 0 {
		return types.SeqBlockLease{}, types.NewInvalidArgument("lease ttl must be positive")
	}
	return useCase.repository.AllocateSeqBlock(ctx, command, leaseTTL)
}
