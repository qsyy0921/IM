package app

import (
	"context"

	"github.com/qsyy0921/IM/services/conversation-service/internal/types"
)

type TransferConversationOwnerUseCase struct {
	repository TransferConversationOwnerRepository
}

func NewTransferConversationOwnerUseCase(repository TransferConversationOwnerRepository) *TransferConversationOwnerUseCase {
	return &TransferConversationOwnerUseCase{repository: repository}
}

func (u *TransferConversationOwnerUseCase) Execute(
	ctx context.Context,
	command types.TransferConversationOwnerCommand,
) (types.TransferConversationOwnerResult, error) {
	if err := command.Validate(); err != nil {
		return types.TransferConversationOwnerResult{}, err
	}
	return u.repository.TransferConversationOwner(ctx, command)
}
