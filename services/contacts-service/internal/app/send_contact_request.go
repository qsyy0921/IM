package app

import (
	"context"

	"github.com/qsyy0921/IM/services/contacts-service/internal/types"
)

type SendContactRequestUseCase struct {
	repository SendContactRequestRepository
}

func NewSendContactRequestUseCase(repository SendContactRequestRepository) *SendContactRequestUseCase {
	return &SendContactRequestUseCase{repository: repository}
}

func (u *SendContactRequestUseCase) Execute(
	ctx context.Context,
	command types.SendContactRequestCommand,
) (types.SendContactRequestResult, error) {
	if err := command.Validate(); err != nil {
		return types.SendContactRequestResult{}, err
	}
	return u.repository.SendContactRequest(ctx, command)
}
