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

type CancelContactRequestRepository interface {
	CancelContactRequest(context.Context, types.CancelContactRequestCommand) (types.CancelContactRequestResult, error)
}

type ListContactRequestsRepository interface {
	ListContactRequests(context.Context, types.ListContactRequestsCommand) (types.ListContactRequestsResult, error)
}

type ListContactsRepository interface {
	ListContacts(context.Context, types.ListContactsCommand) (types.ListContactsResult, error)
}

type GetContactStateRepository interface {
	GetContactState(context.Context, types.GetContactStateCommand) (types.GetContactStateResult, error)
}

type DeleteContactRepository interface {
	DeleteContact(context.Context, types.DeleteContactCommand) (types.DeleteContactResult, error)
}

type BlockContactRepository interface {
	BlockContact(context.Context, types.BlockContactCommand) (types.BlockContactResult, error)
}

type UnblockContactRepository interface {
	UnblockContact(context.Context, types.UnblockContactCommand) (types.UnblockContactResult, error)
}

type UpdateContactRemarkRepository interface {
	UpdateContactRemark(context.Context, types.UpdateContactRemarkCommand) (types.UpdateContactRemarkResult, error)
}

type UpdateContactGroupRepository interface {
	UpdateContactGroup(context.Context, types.UpdateContactGroupCommand) (types.UpdateContactGroupResult, error)
}
