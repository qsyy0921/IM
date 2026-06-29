package app

import (
	"context"

	"github.com/qsyy0921/IM/services/timeline-service/internal/types"
)

type ExpireSeqBlockLeasesUseCase struct {
	repository SeqBlockRepository
}

func NewExpireSeqBlockLeasesUseCase(repository SeqBlockRepository) *ExpireSeqBlockLeasesUseCase {
	return &ExpireSeqBlockLeasesUseCase{repository: repository}
}

func (useCase *ExpireSeqBlockLeasesUseCase) Execute(
	ctx context.Context,
	command types.ExpireLeasesCommand,
) (types.ExpireLeasesResult, error) {
	if useCase == nil || useCase.repository == nil {
		return types.ExpireLeasesResult{}, types.NewDBWriteFailed("seq block repository is not configured")
	}
	if err := command.Validate(); err != nil {
		return types.ExpireLeasesResult{}, err
	}
	return useCase.repository.ExpireSeqBlockLeases(ctx, command)
}

type CreateGapMarkerUseCase struct {
	repository SeqBlockRepository
}

func NewCreateGapMarkerUseCase(repository SeqBlockRepository) *CreateGapMarkerUseCase {
	return &CreateGapMarkerUseCase{repository: repository}
}

func (useCase *CreateGapMarkerUseCase) Execute(
	ctx context.Context,
	command types.GapMarkerCommand,
) (types.GapMarker, error) {
	if useCase == nil || useCase.repository == nil {
		return types.GapMarker{}, types.NewDBWriteFailed("seq block repository is not configured")
	}
	if err := command.Validate(); err != nil {
		return types.GapMarker{}, err
	}
	return useCase.repository.CreateGapMarker(ctx, command)
}

type CloseGapMarkerUseCase struct {
	repository SeqBlockRepository
}

func NewCloseGapMarkerUseCase(repository SeqBlockRepository) *CloseGapMarkerUseCase {
	return &CloseGapMarkerUseCase{repository: repository}
}

func (useCase *CloseGapMarkerUseCase) Execute(
	ctx context.Context,
	command types.CloseGapMarkerCommand,
) (types.GapMarker, error) {
	if useCase == nil || useCase.repository == nil {
		return types.GapMarker{}, types.NewDBWriteFailed("seq block repository is not configured")
	}
	if err := command.Validate(); err != nil {
		return types.GapMarker{}, err
	}
	return useCase.repository.CloseGapMarker(ctx, command)
}
