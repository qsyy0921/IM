package app

import (
	"context"

	"github.com/qsyy0921/IM/services/conversation-service/internal/types"
)

type ListConversationMembersUseCase struct {
	repository ListConversationMembersRepository
}

func NewListConversationMembersUseCase(repository ListConversationMembersRepository) *ListConversationMembersUseCase {
	return &ListConversationMembersUseCase{repository: repository}
}

func (u *ListConversationMembersUseCase) Execute(
	ctx context.Context,
	command types.ListConversationMembersCommand,
) (types.ListConversationMembersResult, error) {
	if err := command.Validate(); err != nil {
		return types.ListConversationMembersResult{}, err
	}
	return u.repository.ListConversationMembers(ctx, command)
}
