package app

import (
	"context"

	"github.com/qsyy0921/IM/services/receipt-service/internal/types"
)

type SetConversationTagsUseCase struct {
	repository ReceiptRepository
}

func NewSetConversationTagsUseCase(repository ReceiptRepository) *SetConversationTagsUseCase {
	return &SetConversationTagsUseCase{repository: repository}
}

func (useCase *SetConversationTagsUseCase) Execute(
	ctx context.Context,
	command types.SetConversationTagsCommand,
) (types.SetConversationTagsResult, error) {
	if err := command.Validate(); err != nil {
		return types.SetConversationTagsResult{}, err
	}
	tags, err := types.NormalizeConversationTags(command.Tags)
	if err != nil {
		return types.SetConversationTagsResult{}, err
	}
	command.Tags = tags
	return useCase.repository.SetConversationTags(ctx, command)
}
