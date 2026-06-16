package app

import (
	"context"

	"github.com/qsyy0921/IM/services/contacts-service/internal/types"
)

type SendContactRequestRepository interface {
	SendContactRequest(context.Context, types.SendContactRequestCommand) (types.SendContactRequestResult, error)
}

type GetContactPrivacyRepository interface {
	GetContactPrivacy(context.Context, types.GetContactPrivacyCommand) (types.GetContactPrivacyResult, error)
}

type SetContactPrivacyRepository interface {
	SetContactPrivacy(context.Context, types.SetContactPrivacyCommand) (types.SetContactPrivacyResult, error)
}

type SetContactPrivacyExceptionRepository interface {
	SetContactPrivacyException(context.Context, types.SetContactPrivacyExceptionCommand) (types.SetContactPrivacyExceptionResult, error)
}

type ListContactPrivacyExceptionsRepository interface {
	ListContactPrivacyExceptions(context.Context, types.ListContactPrivacyExceptionsCommand) (types.ListContactPrivacyExceptionsResult, error)
}

type DeleteContactPrivacyExceptionRepository interface {
	DeleteContactPrivacyException(context.Context, types.DeleteContactPrivacyExceptionCommand) (types.DeleteContactPrivacyExceptionResult, error)
}

type GetTenantContactPrivacyDefaultRepository interface {
	GetTenantContactPrivacyDefault(context.Context, types.GetTenantContactPrivacyDefaultCommand) (types.GetTenantContactPrivacyDefaultResult, error)
}

type SetTenantContactPrivacyDefaultRepository interface {
	SetTenantContactPrivacyDefault(context.Context, types.SetTenantContactPrivacyDefaultCommand) (types.SetTenantContactPrivacyDefaultResult, error)
}

type GetTenantContactRequestSourcePolicyRepository interface {
	GetTenantContactRequestSourcePolicy(context.Context, types.GetTenantContactRequestSourcePolicyCommand) (types.GetTenantContactRequestSourcePolicyResult, error)
}

type SetTenantContactRequestSourcePolicyRepository interface {
	SetTenantContactRequestSourcePolicy(context.Context, types.SetTenantContactRequestSourcePolicyCommand) (types.SetTenantContactRequestSourcePolicyResult, error)
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
