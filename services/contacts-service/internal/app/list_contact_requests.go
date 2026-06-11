package app

import (
	"context"

	"github.com/qsyy0921/IM/services/contacts-service/internal/types"
)

type ListContactRequestsUseCase struct {
	repository ListContactRequestsRepository
}

func NewListContactRequestsUseCase(repository ListContactRequestsRepository) *ListContactRequestsUseCase {
	return &ListContactRequestsUseCase{repository: repository}
}

func (u *ListContactRequestsUseCase) Execute(
	ctx context.Context,
	command types.ListContactRequestsCommand,
) (types.ListContactRequestsResult, error) {
	if err := command.Validate(); err != nil {
		return types.ListContactRequestsResult{}, err
	}
	if u.repository == nil {
		return types.ListContactRequestsResult{}, types.NewDBReadFailed("list contact requests repository is not configured")
	}
	return u.repository.ListContactRequests(ctx, command)
}
