package app

import (
	"context"

	"github.com/qsyy0921/IM/services/contacts-service/internal/types"
)

type SendContactRequestRepository interface {
	SendContactRequest(context.Context, types.SendContactRequestCommand) (types.SendContactRequestResult, error)
}

type RespondContactRequestRepository interface {
	RespondContactRequest(context.Context, types.RespondContactRequestCommand) (types.RespondContactRequestResult, error)
}

type ListContactsRepository interface {
	ListContacts(context.Context, types.ListContactsCommand) (types.ListContactsResult, error)
}

type GetContactStateRepository interface {
	GetContactState(context.Context, types.GetContactStateCommand) (types.GetContactStateResult, error)
}
