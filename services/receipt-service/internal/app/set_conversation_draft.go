package app

import (
	"context"

	"github.com/qsyy0921/IM/services/receipt-service/internal/types"
)

type SetConversationDraftUseCase struct {
	repository ReceiptRepository
}

func NewSetConversationDraftUseCase(repository ReceiptRepository) *SetConversationDraftUseCase {
	return &SetConversationDraftUseCase{repository: repository}
}

func (useCase *SetConversationDraftUseCase) Execute(
	ctx context.Context,
	command types.SetConversationDraftCommand,
) (types.SetConversationDraftResult, error) {
	if err := command.Validate(); err != nil {
		return types.SetConversationDraftResult{}, err
	}
	draftText, err := types.NormalizeConversationDraft(command.DraftText)
	if err != nil {
		return types.SetConversationDraftResult{}, err
	}
	command.DraftText = draftText
	return useCase.repository.SetConversationDraft(ctx, command)
}
