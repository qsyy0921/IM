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
