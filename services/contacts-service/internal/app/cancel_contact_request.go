package app

import (
	"context"

	"github.com/qsyy0921/IM/services/contacts-service/internal/types"
)

type CancelContactRequestUseCase struct {
	repository CancelContactRequestRepository
}

func NewCancelContactRequestUseCase(repository CancelContactRequestRepository) *CancelContactRequestUseCase {
	return &CancelContactRequestUseCase{repository: repository}
}

func (u *CancelContactRequestUseCase) Execute(
	ctx context.Context,
	command types.CancelContactRequestCommand,
) (types.CancelContactRequestResult, error) {
	if err := command.Validate(); err != nil {
		return types.CancelContactRequestResult{}, err
	}
	if u.repository == nil {
		return types.CancelContactRequestResult{}, types.NewDBWriteFailed("cancel contact request repository is not configured")
	}
	return u.repository.CancelContactRequest(ctx, command)
}
