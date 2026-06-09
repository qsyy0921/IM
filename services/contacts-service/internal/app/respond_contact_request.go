package app

import (
	"context"

	"github.com/qsyy0921/IM/services/contacts-service/internal/types"
)

type RespondContactRequestUseCase struct {
	repository RespondContactRequestRepository
}

func NewRespondContactRequestUseCase(repository RespondContactRequestRepository) *RespondContactRequestUseCase {
	return &RespondContactRequestUseCase{repository: repository}
}

func (u *RespondContactRequestUseCase) Execute(
	ctx context.Context,
	command types.RespondContactRequestCommand,
) (types.RespondContactRequestResult, error) {
	if err := command.Validate(); err != nil {
		return types.RespondContactRequestResult{}, err
	}
	return u.repository.RespondContactRequest(ctx, command)
}
