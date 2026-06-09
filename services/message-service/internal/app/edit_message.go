package app

import (
	"context"

	"github.com/qsyy0921/IM/services/message-service/internal/domain"
	"github.com/qsyy0921/IM/services/message-service/internal/types"
)

type EditMessageUseCase struct {
	policy       PolicyCheckPort
	conversation ConversationQueryPort
	messageRepo  MessageRepository
}

func NewEditMessageUseCase(
	policy PolicyCheckPort,
	conversation ConversationQueryPort,
	messageRepo MessageRepository,
) *EditMessageUseCase {
	return &EditMessageUseCase{
		policy:       policy,
		conversation: conversation,
		messageRepo:  messageRepo,
	}
}

func (u *EditMessageUseCase) Execute(
	ctx context.Context,
	command types.EditMessageCommand,
) (types.MessageChangeResult, error) {
	if err := command.Validate(); err != nil {
		return types.MessageChangeResult{}, err
	}

	conversation, permission, err := u.readConsistentEditDependencies(ctx, command)
	if err != nil {
		return types.MessageChangeResult{}, err
	}
	if !permission.Allowed {
		return types.MessageChangeResult{}, types.NewPermissionDenied(permission.Reason)
	}
	if conversation.ConversationMode != types.ConversationModeLocalRowLock {
		return types.MessageChangeResult{}, types.NewSequencerUnavailable("sequencer mode is contract-only in phase 1")
	}

	result, err := u.messageRepo.EditMessage(ctx, domain.EditMessageInput{
		Command:      command,
		Permission:   permission,
		Conversation: conversation,
	})
	if err != nil {
		return types.MessageChangeResult{}, err
	}
	return types.MessageChangeResult{
		MessageID:        command.MessageID,
		ConversationID:   command.ConversationID,
		ConversationSeq:  result.ConversationSeq,
		ChangeVersion:    result.ChangeVersion,
		AcceptedAt:       result.AcceptedAt,
		IdempotentReplay: result.IdempotentReplay,
	}, nil
}

func (u *EditMessageUseCase) readConsistentEditDependencies(
	ctx context.Context,
	command types.EditMessageCommand,
) (types.ConversationSendContext, types.PermissionDecision, error) {
	var conversation types.ConversationSendContext
	var permission types.PermissionDecision
	for attempt := 0; attempt < 2; attempt++ {
		var err error
		conversation, err = u.conversation.GetSendContext(ctx, sendContextCommandFromEdit(command))
		if err != nil {
			return types.ConversationSendContext{}, types.PermissionDecision{}, err
		}
		permission, err = u.policy.CheckEditPermission(ctx, command)
		if err != nil {
			return types.ConversationSendContext{}, types.PermissionDecision{}, err
		}
		if !permission.Allowed || permission.PermissionVersion == conversation.PermissionVersion {
			return conversation, permission, nil
		}
	}
	return types.ConversationSendContext{}, types.PermissionDecision{}, types.NewDependencyVersionMismatch(
		"permission version changed during edit dependency read",
	)
}

func sendContextCommandFromEdit(command types.EditMessageCommand) types.SendMessageCommand {
	return types.SendMessageCommand{
		AuthContext:    command.AuthContext,
		ConversationID: command.ConversationID,
		ClientMsgID:    types.ClientMsgID(command.IdempotencyKey),
		MessageType:    types.MessageTypeText,
		PayloadJSON:    []byte(`{}`),
		ReceivedAt:     command.ReceivedAt,
	}
}
