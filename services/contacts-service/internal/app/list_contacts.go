package app

import (
	"context"

	"github.com/qsyy0921/IM/services/contacts-service/internal/types"
)

type ListContactsUseCase struct {
	repository ListContactsRepository
}

func NewListContactsUseCase(repository ListContactsRepository) *ListContactsUseCase {
	return &ListContactsUseCase{repository: repository}
}

func (u *ListContactsUseCase) Execute(
	ctx context.Context,
	command types.ListContactsCommand,
) (types.ListContactsResult, error) {
	if err := command.Validate(); err != nil {
		return types.ListContactsResult{}, err
	}
	return u.repository.ListContacts(ctx, command)
}
