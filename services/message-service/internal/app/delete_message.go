package app

import (
	"context"

	"github.com/qsyy0921/IM/services/message-service/internal/domain"
	"github.com/qsyy0921/IM/services/message-service/internal/types"
)

type DeleteMessageUseCase struct {
	policy       PolicyCheckPort
	conversation ConversationQueryPort
	messageRepo  MessageRepository
}

func NewDeleteMessageUseCase(
	policy PolicyCheckPort,
	conversation ConversationQueryPort,
	messageRepo MessageRepository,
) *DeleteMessageUseCase {
	return &DeleteMessageUseCase{
		policy:       policy,
		conversation: conversation,
		messageRepo:  messageRepo,
	}
}

func (u *DeleteMessageUseCase) Execute(
	ctx context.Context,
	command types.DeleteMessageCommand,
) (types.MessageChangeResult, error) {
	if err := command.Validate(); err != nil {
		return types.MessageChangeResult{}, err
	}
	if command.DeleteScope != types.DeleteScopeConversationView {
		return types.MessageChangeResult{}, types.NewUnsupportedMessageType("delete_scope is not supported in phase 1")
	}

	conversation, permission, err := u.readConsistentDeleteDependencies(ctx, command)
	if err != nil {
		return types.MessageChangeResult{}, err
	}
	if !permission.Allowed {
		return types.MessageChangeResult{}, types.NewPermissionDenied(permission.Reason)
	}
	if conversation.ConversationMode != types.ConversationModeLocalRowLock {
		return types.MessageChangeResult{}, types.NewSequencerUnavailable("sequencer mode is contract-only in phase 1")
	}

	result, err := u.messageRepo.DeleteMessage(ctx, domain.DeleteMessageInput{
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

func (u *DeleteMessageUseCase) readConsistentDeleteDependencies(
	ctx context.Context,
	command types.DeleteMessageCommand,
) (types.ConversationSendContext, types.PermissionDecision, error) {
	var conversation types.ConversationSendContext
	var permission types.PermissionDecision
	var message types.MessagePolicyContext
	for attempt := 0; attempt < 2; attempt++ {
		var err error
		conversation, err = u.conversation.GetSendContext(ctx, sendContextCommandFromDelete(command))
		if err != nil {
			return types.ConversationSendContext{}, types.PermissionDecision{}, err
		}
		message, err = u.messageRepo.GetMessagePolicyContext(ctx, command.AuthContext.TenantID, command.ConversationID, command.MessageID)
		if err != nil {
			return types.ConversationSendContext{}, types.PermissionDecision{}, err
		}
		permission, err = u.policy.CheckDeletePermission(ctx, command, conversation, message)
		if err != nil {
			return types.ConversationSendContext{}, types.PermissionDecision{}, err
		}
		if !permission.Allowed || permission.PermissionVersion == conversation.PermissionVersion {
			return conversation, permission, nil
		}
	}
	return types.ConversationSendContext{}, types.PermissionDecision{}, types.NewDependencyVersionMismatch(
		"permission version changed during delete dependency read",
	)
}

func sendContextCommandFromDelete(command types.DeleteMessageCommand) types.SendMessageCommand {
	return types.SendMessageCommand{
		AuthContext:    command.AuthContext,
		ConversationID: command.ConversationID,
		ClientMsgID:    types.ClientMsgID(command.IdempotencyKey),
		MessageType:    types.MessageTypeText,
		PayloadJSON:    []byte(`{}`),
		ReceivedAt:     command.ReceivedAt,
	}
}
