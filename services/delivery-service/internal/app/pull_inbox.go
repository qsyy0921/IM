package app

import (
	"context"

	"github.com/qsyy0921/IM/services/delivery-service/internal/domain"
	"github.com/qsyy0921/IM/services/delivery-service/internal/types"
)

type PullInboxUseCase struct {
	repository InboxRepository
}

func NewPullInboxUseCase(repository InboxRepository) *PullInboxUseCase {
	return &PullInboxUseCase{repository: repository}
}

func (useCase *PullInboxUseCase) Execute(ctx context.Context, command types.PullInboxCommand) (types.PullInboxResult, error) {
	if err := command.Validate(); err != nil {
		return types.PullInboxResult{}, err
	}
	limit := command.EffectiveLimit()
	items, err := useCase.repository.PullInbox(ctx, command, limit+1)
	if err != nil {
		return types.PullInboxResult{}, err
	}
	return domain.BuildPullResult(items, limit), nil
}
