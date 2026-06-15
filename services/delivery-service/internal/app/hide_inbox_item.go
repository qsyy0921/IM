package app

import (
	"context"

	"github.com/qsyy0921/IM/services/delivery-service/internal/types"
)

type HideInboxItemUseCase struct {
	repository InboxVisibilityRepository
}

func NewHideInboxItemUseCase(repository InboxVisibilityRepository) *HideInboxItemUseCase {
	return &HideInboxItemUseCase{repository: repository}
}

func (useCase *HideInboxItemUseCase) Execute(
	ctx context.Context,
	command types.HideInboxItemCommand,
) (types.HideInboxItemResult, error) {
	if err := command.Validate(); err != nil {
		return types.HideInboxItemResult{}, err
	}
	return useCase.repository.HideInboxItem(ctx, command)
}
