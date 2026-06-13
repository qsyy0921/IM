package app

import (
	"context"

	"github.com/qsyy0921/IM/services/message-service/internal/domain"
	"github.com/qsyy0921/IM/services/message-service/internal/types"
)

type RevokeMessageUseCase struct {
	policy       PolicyCheckPort
	conversation ConversationQueryPort
	messageRepo  MessageRepository
}

func NewRevokeMessageUseCase(
	policy PolicyCheckPort,
	conversation ConversationQueryPort,
	messageRepo MessageRepository,
) *RevokeMessageUseCase {
	return &RevokeMessageUseCase{
		policy:       policy,
		conversation: conversation,
		messageRepo:  messageRepo,
	}
}

func (u *RevokeMessageUseCase) Execute(
	ctx context.Context,
	command types.RevokeMessageCommand,
) (types.MessageChangeResult, error) {
	if err := command.Validate(); err != nil {
		return types.MessageChangeResult{}, err
	}

	conversation, permission, err := u.readConsistentRevokeDependencies(ctx, command)
	if err != nil {
		return types.MessageChangeResult{}, err
	}
	if !permission.Allowed {
		return types.MessageChangeResult{}, types.NewPermissionDenied(permission.Reason)
	}
	if conversation.ConversationMode != types.ConversationModeLocalRowLock {
		return types.MessageChangeResult{}, types.NewSequencerUnavailable("sequencer mode is contract-only in phase 1")
	}

	result, err := u.messageRepo.RevokeMessage(ctx, domain.RevokeMessageInput{
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

func (u *RevokeMessageUseCase) readConsistentRevokeDependencies(
	ctx context.Context,
	command types.RevokeMessageCommand,
) (types.ConversationSendContext, types.PermissionDecision, error) {
	var conversation types.ConversationSendContext
	var permission types.PermissionDecision
	for attempt := 0; attempt < 2; attempt++ {
		var err error
		conversation, err = u.conversation.GetSendContext(ctx, sendContextCommandFromRevoke(command))
		if err != nil {
			return types.ConversationSendContext{}, types.PermissionDecision{}, err
		}
		permission, err = u.policy.CheckRevokePermission(ctx, command, conversation)
		if err != nil {
			return types.ConversationSendContext{}, types.PermissionDecision{}, err
		}
		if !permission.Allowed || permission.PermissionVersion == conversation.PermissionVersion {
			return conversation, permission, nil
		}
	}
	return types.ConversationSendContext{}, types.PermissionDecision{}, types.NewDependencyVersionMismatch(
		"permission version changed during revoke dependency read",
	)
}

func sendContextCommandFromRevoke(command types.RevokeMessageCommand) types.SendMessageCommand {
	return types.SendMessageCommand{
		AuthContext:    command.AuthContext,
		ConversationID: command.ConversationID,
		ClientMsgID:    types.ClientMsgID(command.IdempotencyKey),
		MessageType:    types.MessageTypeText,
		PayloadJSON:    []byte(`{}`),
		ReceivedAt:     command.ReceivedAt,
	}
}
