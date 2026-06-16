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

type GetContactPrivacyUseCase struct {
	repository GetContactPrivacyRepository
}

func NewGetContactPrivacyUseCase(repository GetContactPrivacyRepository) *GetContactPrivacyUseCase {
	return &GetContactPrivacyUseCase{repository: repository}
}

func (u *GetContactPrivacyUseCase) Execute(
	ctx context.Context,
	command types.GetContactPrivacyCommand,
) (types.GetContactPrivacyResult, error) {
	if err := command.Validate(); err != nil {
		return types.GetContactPrivacyResult{}, err
	}
	return u.repository.GetContactPrivacy(ctx, command)
}

type SetContactPrivacyUseCase struct {
	repository SetContactPrivacyRepository
}

func NewSetContactPrivacyUseCase(repository SetContactPrivacyRepository) *SetContactPrivacyUseCase {
	return &SetContactPrivacyUseCase{repository: repository}
}

func (u *SetContactPrivacyUseCase) Execute(
	ctx context.Context,
	command types.SetContactPrivacyCommand,
) (types.SetContactPrivacyResult, error) {
	if err := command.Validate(); err != nil {
		return types.SetContactPrivacyResult{}, err
	}
	return u.repository.SetContactPrivacy(ctx, command)
}

type SetContactPrivacyExceptionUseCase struct {
	repository SetContactPrivacyExceptionRepository
}

func NewSetContactPrivacyExceptionUseCase(repository SetContactPrivacyExceptionRepository) *SetContactPrivacyExceptionUseCase {
	return &SetContactPrivacyExceptionUseCase{repository: repository}
}

func (u *SetContactPrivacyExceptionUseCase) Execute(
	ctx context.Context,
	command types.SetContactPrivacyExceptionCommand,
) (types.SetContactPrivacyExceptionResult, error) {
	if err := command.Validate(); err != nil {
		return types.SetContactPrivacyExceptionResult{}, err
	}
	if u.repository == nil {
		return types.SetContactPrivacyExceptionResult{}, types.NewDBWriteFailed("contact privacy exception repository is not configured")
	}
	command.Decision = types.NormalizeContactPrivacyExceptionDecision(command.Decision)
	return u.repository.SetContactPrivacyException(ctx, command)
}

type ListContactPrivacyExceptionsUseCase struct {
	repository ListContactPrivacyExceptionsRepository
}

func NewListContactPrivacyExceptionsUseCase(repository ListContactPrivacyExceptionsRepository) *ListContactPrivacyExceptionsUseCase {
	return &ListContactPrivacyExceptionsUseCase{repository: repository}
}

func (u *ListContactPrivacyExceptionsUseCase) Execute(
	ctx context.Context,
	command types.ListContactPrivacyExceptionsCommand,
) (types.ListContactPrivacyExceptionsResult, error) {
	if err := command.Validate(); err != nil {
		return types.ListContactPrivacyExceptionsResult{}, err
	}
	if u.repository == nil {
		return types.ListContactPrivacyExceptionsResult{}, types.NewDBReadFailed("contact privacy exception repository is not configured")
	}
	return u.repository.ListContactPrivacyExceptions(ctx, command)
}

type DeleteContactPrivacyExceptionUseCase struct {
	repository DeleteContactPrivacyExceptionRepository
}

func NewDeleteContactPrivacyExceptionUseCase(repository DeleteContactPrivacyExceptionRepository) *DeleteContactPrivacyExceptionUseCase {
	return &DeleteContactPrivacyExceptionUseCase{repository: repository}
}

func (u *DeleteContactPrivacyExceptionUseCase) Execute(
	ctx context.Context,
	command types.DeleteContactPrivacyExceptionCommand,
) (types.DeleteContactPrivacyExceptionResult, error) {
	if err := command.Validate(); err != nil {
		return types.DeleteContactPrivacyExceptionResult{}, err
	}
	if u.repository == nil {
		return types.DeleteContactPrivacyExceptionResult{}, types.NewDBWriteFailed("contact privacy exception repository is not configured")
	}
	return u.repository.DeleteContactPrivacyException(ctx, command)
}

type GetTenantContactPrivacyDefaultUseCase struct {
	repository GetTenantContactPrivacyDefaultRepository
}

func NewGetTenantContactPrivacyDefaultUseCase(repository GetTenantContactPrivacyDefaultRepository) *GetTenantContactPrivacyDefaultUseCase {
	return &GetTenantContactPrivacyDefaultUseCase{repository: repository}
}

func (u *GetTenantContactPrivacyDefaultUseCase) Execute(
	ctx context.Context,
	command types.GetTenantContactPrivacyDefaultCommand,
) (types.GetTenantContactPrivacyDefaultResult, error) {
	if err := command.Validate(); err != nil {
		return types.GetTenantContactPrivacyDefaultResult{}, err
	}
	if u.repository == nil {
		return types.GetTenantContactPrivacyDefaultResult{}, types.NewDBReadFailed("tenant contact privacy default repository is not configured")
	}
	return u.repository.GetTenantContactPrivacyDefault(ctx, command)
}

type SetTenantContactPrivacyDefaultUseCase struct {
	repository SetTenantContactPrivacyDefaultRepository
}

func NewSetTenantContactPrivacyDefaultUseCase(repository SetTenantContactPrivacyDefaultRepository) *SetTenantContactPrivacyDefaultUseCase {
	return &SetTenantContactPrivacyDefaultUseCase{repository: repository}
}

func (u *SetTenantContactPrivacyDefaultUseCase) Execute(
	ctx context.Context,
	command types.SetTenantContactPrivacyDefaultCommand,
) (types.SetTenantContactPrivacyDefaultResult, error) {
	if err := command.Validate(); err != nil {
		return types.SetTenantContactPrivacyDefaultResult{}, err
	}
	if u.repository == nil {
		return types.SetTenantContactPrivacyDefaultResult{}, types.NewDBWriteFailed("tenant contact privacy default repository is not configured")
	}
	return u.repository.SetTenantContactPrivacyDefault(ctx, command)
}

type GetTenantContactRequestSourcePolicyUseCase struct {
	repository GetTenantContactRequestSourcePolicyRepository
}

func NewGetTenantContactRequestSourcePolicyUseCase(repository GetTenantContactRequestSourcePolicyRepository) *GetTenantContactRequestSourcePolicyUseCase {
	return &GetTenantContactRequestSourcePolicyUseCase{repository: repository}
}

func (u *GetTenantContactRequestSourcePolicyUseCase) Execute(
	ctx context.Context,
	command types.GetTenantContactRequestSourcePolicyCommand,
) (types.GetTenantContactRequestSourcePolicyResult, error) {
	if err := command.Validate(); err != nil {
		return types.GetTenantContactRequestSourcePolicyResult{}, err
	}
	if u.repository == nil {
		return types.GetTenantContactRequestSourcePolicyResult{}, types.NewDBReadFailed("tenant contact request source policy repository is not configured")
	}
	command.SourceType = command.NormalizedSourceType()
	return u.repository.GetTenantContactRequestSourcePolicy(ctx, command)
}

type SetTenantContactRequestSourcePolicyUseCase struct {
	repository SetTenantContactRequestSourcePolicyRepository
}

func NewSetTenantContactRequestSourcePolicyUseCase(repository SetTenantContactRequestSourcePolicyRepository) *SetTenantContactRequestSourcePolicyUseCase {
	return &SetTenantContactRequestSourcePolicyUseCase{repository: repository}
}

func (u *SetTenantContactRequestSourcePolicyUseCase) Execute(
	ctx context.Context,
	command types.SetTenantContactRequestSourcePolicyCommand,
) (types.SetTenantContactRequestSourcePolicyResult, error) {
	if err := command.Validate(); err != nil {
		return types.SetTenantContactRequestSourcePolicyResult{}, err
	}
	if u.repository == nil {
		return types.SetTenantContactRequestSourcePolicyResult{}, types.NewDBWriteFailed("tenant contact request source policy repository is not configured")
	}
	command.SourceType = command.NormalizedSourceType()
	return u.repository.SetTenantContactRequestSourcePolicy(ctx, command)
}
