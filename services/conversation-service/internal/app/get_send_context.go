package app

import (
	"context"

	"github.com/qsyy0921/IM/services/conversation-service/internal/types"
)

type GetSendContextUseCase struct {
	repository ConversationRepository
}

func NewGetSendContextUseCase(repository ConversationRepository) *GetSendContextUseCase {
	return &GetSendContextUseCase{repository: repository}
}

func (u *GetSendContextUseCase) Execute(
	ctx context.Context,
	command types.GetSendContextCommand,
) (types.ConversationSendContext, error) {
	if err := command.Validate(); err != nil {
		return types.ConversationSendContext{}, err
	}
	return u.repository.GetSendContext(ctx, command)
}
