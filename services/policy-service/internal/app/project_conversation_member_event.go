package app

import (
	"context"

	"github.com/qsyy0921/IM/services/policy-service/internal/types"
)

type ConversationMemberProjectionRepository interface {
	ProjectConversationMemberEvent(context.Context, types.ProjectConversationMemberEventCommand) (types.ProjectConversationMemberEventResult, error)
}

type ProjectConversationMemberEventUseCase struct {
	repository ConversationMemberProjectionRepository
}

func NewProjectConversationMemberEventUseCase(repository ConversationMemberProjectionRepository) *ProjectConversationMemberEventUseCase {
	return &ProjectConversationMemberEventUseCase{repository: repository}
}

func (useCase *ProjectConversationMemberEventUseCase) Execute(
	ctx context.Context,
	command types.ProjectConversationMemberEventCommand,
) (types.ProjectConversationMemberEventResult, error) {
	if err := command.Validate(); err != nil {
		return types.ProjectConversationMemberEventResult{}, err
	}
	return useCase.repository.ProjectConversationMemberEvent(ctx, command)
}
