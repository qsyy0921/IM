package app

import (
	"context"

	"github.com/qsyy0921/IM/services/conversation-service/internal/types"
)

type GetMemberChangeUseCase struct {
	repository GetMemberChangeRepository
}

func NewGetMemberChangeUseCase(repository GetMemberChangeRepository) *GetMemberChangeUseCase {
	return &GetMemberChangeUseCase{repository: repository}
}

func (u *GetMemberChangeUseCase) Execute(
	ctx context.Context,
	command types.GetMemberChangeCommand,
) (types.MemberChangeDetail, error) {
	if err := command.Validate(); err != nil {
		return types.MemberChangeDetail{}, err
	}
	return u.repository.GetMemberChange(ctx, command)
}
