package app

import (
	"context"

	"github.com/qsyy0921/IM/services/contacts-service/internal/types"
)

type DeleteContactUseCase struct {
	repository DeleteContactRepository
}

func NewDeleteContactUseCase(repository DeleteContactRepository) *DeleteContactUseCase {
	return &DeleteContactUseCase{repository: repository}
}

func (u *DeleteContactUseCase) Execute(
	ctx context.Context,
	command types.DeleteContactCommand,
) (types.DeleteContactResult, error) {
	if err := command.Validate(); err != nil {
		return types.DeleteContactResult{}, err
	}
	return u.repository.DeleteContact(ctx, command)
}

type BlockContactUseCase struct {
	repository BlockContactRepository
}

func NewBlockContactUseCase(repository BlockContactRepository) *BlockContactUseCase {
	return &BlockContactUseCase{repository: repository}
}

func (u *BlockContactUseCase) Execute(
	ctx context.Context,
	command types.BlockContactCommand,
) (types.BlockContactResult, error) {
	if err := command.Validate(); err != nil {
		return types.BlockContactResult{}, err
	}
	return u.repository.BlockContact(ctx, command)
}

type UnblockContactUseCase struct {
	repository UnblockContactRepository
}

func NewUnblockContactUseCase(repository UnblockContactRepository) *UnblockContactUseCase {
	return &UnblockContactUseCase{repository: repository}
}

func (u *UnblockContactUseCase) Execute(
	ctx context.Context,
	command types.UnblockContactCommand,
) (types.UnblockContactResult, error) {
	if err := command.Validate(); err != nil {
		return types.UnblockContactResult{}, err
	}
	return u.repository.UnblockContact(ctx, command)
}

type UpdateContactRemarkUseCase struct {
	repository UpdateContactRemarkRepository
}

func NewUpdateContactRemarkUseCase(repository UpdateContactRemarkRepository) *UpdateContactRemarkUseCase {
	return &UpdateContactRemarkUseCase{repository: repository}
}

func (u *UpdateContactRemarkUseCase) Execute(
	ctx context.Context,
	command types.UpdateContactRemarkCommand,
) (types.UpdateContactRemarkResult, error) {
	if err := command.Validate(); err != nil {
		return types.UpdateContactRemarkResult{}, err
	}
	return u.repository.UpdateContactRemark(ctx, command)
}
