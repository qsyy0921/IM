package app

import (
	"context"

	"github.com/qsyy0921/IM/services/conversation-service/internal/types"
)

type CreateMemberChangeUseCase struct {
	repository CreateMemberChangeRepository
}

func NewCreateMemberChangeUseCase(repository CreateMemberChangeRepository) *CreateMemberChangeUseCase {
	return &CreateMemberChangeUseCase{repository: repository}
}

func (u *CreateMemberChangeUseCase) Execute(
	ctx context.Context,
	command types.CreateMemberChangeCommand,
) (types.MemberChangeResult, error) {
	if err := command.Validate(); err != nil {
		return types.MemberChangeResult{}, err
	}
	return u.repository.CreateMemberChange(ctx, command)
}
