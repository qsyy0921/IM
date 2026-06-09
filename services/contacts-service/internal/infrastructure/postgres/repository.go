package postgres

import (
	"context"

	"github.com/qsyy0921/IM/services/contacts-service/internal/types"
)

type Repository struct{}

func NewRepository() *Repository {
	return &Repository{}
}

func (r *Repository) SendContactRequest(
	context.Context,
	types.SendContactRequestCommand,
) (types.SendContactRequestResult, error) {
	return types.SendContactRequestResult{}, types.NewDBWriteFailed("contacts repository write path is not implemented")
}

func (r *Repository) RespondContactRequest(
	context.Context,
	types.RespondContactRequestCommand,
) (types.RespondContactRequestResult, error) {
	return types.RespondContactRequestResult{}, types.NewDBWriteFailed("contacts repository write path is not implemented")
}

func (r *Repository) ListContacts(
	context.Context,
	types.ListContactsCommand,
) (types.ListContactsResult, error) {
	return types.ListContactsResult{}, types.NewDBReadFailed("contacts repository read path is not implemented")
}

func (r *Repository) GetContactState(
	context.Context,
	types.GetContactStateCommand,
) (types.GetContactStateResult, error) {
	return types.GetContactStateResult{}, types.NewDBReadFailed("contacts repository read path is not implemented")
}
