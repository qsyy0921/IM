package app

import (
	"context"

	"github.com/qsyy0921/IM/services/contacts-service/internal/types"
)

type GetContactStateUseCase struct {
	repository GetContactStateRepository
}

func NewGetContactStateUseCase(repository GetContactStateRepository) *GetContactStateUseCase {
	return &GetContactStateUseCase{repository: repository}
}

func (u *GetContactStateUseCase) Execute(
	ctx context.Context,
	command types.GetContactStateCommand,
) (types.GetContactStateResult, error) {
	if err := command.Validate(); err != nil {
		return types.GetContactStateResult{}, err
	}
	return u.repository.GetContactState(ctx, command)
}
