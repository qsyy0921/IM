package app

import (
	"context"

	"github.com/qsyy0921/IM/services/message-service/internal/domain"
	"github.com/qsyy0921/IM/services/message-service/internal/types"
)

type SendMessageUseCase struct {
	policy       PolicyCheckPort
	conversation ConversationQueryPort
	sequencer    SequencerPort
	messageRepo  MessageRepository
	admission    AdmissionPort
}

type SendMessageUseCaseOption func(*SendMessageUseCase)

func NewSendMessageUseCase(
	policy PolicyCheckPort,
	conversation ConversationQueryPort,
	sequencer SequencerPort,
	messageRepo MessageRepository,
	opts ...SendMessageUseCaseOption,
) *SendMessageUseCase {
	useCase := &SendMessageUseCase{
		policy:       policy,
		conversation: conversation,
		sequencer:    sequencer,
		messageRepo:  messageRepo,
	}
	for _, opt := range opts {
		opt(useCase)
	}
	return useCase
}

func WithAdmission(admission AdmissionPort) SendMessageUseCaseOption {
	return func(useCase *SendMessageUseCase) {
		useCase.admission = admission
	}
}

func (u *SendMessageUseCase) Execute(ctx context.Context, command types.SendMessageCommand) (types.SendMessageResult, error) {
	if err := command.Validate(); err != nil {
		return types.SendMessageResult{}, err
	}
	if u.admission != nil {
		permit, err := u.admission.AdmitSendMessage(ctx)
		if err != nil {
			return types.SendMessageResult{}, err
		}
		if permit != nil {
			defer permit.Release()
		}
	}

	conversation, permission, err := u.readConsistentSendDependencies(ctx, command)
	if err != nil {
		return types.SendMessageResult{}, err
	}
	if !permission.Allowed {
		return types.SendMessageResult{}, types.NewPermissionDenied(permission.Reason)
	}

	if conversation.ConversationMode != types.ConversationModeLocalRowLock {
		return types.SendMessageResult{}, types.NewSequencerUnavailable("sequencer mode is contract-only in phase 1")
	}

	result, err := u.messageRepo.AppendMessage(ctx, domain.AppendMessageInput{
		Command:      command,
		Permission:   permission,
		Conversation: conversation,
	})
	if err != nil {
		return types.SendMessageResult{}, err
	}

	return types.SendMessageResult{
		MessageID:        result.MessageID,
		ConversationID:   command.ConversationID,
		ConversationSeq:  result.ConversationSeq,
		AcceptedAt:       result.AcceptedAt,
		IdempotentReplay: result.IdempotentReplay,
	}, nil
}

func (u *SendMessageUseCase) readConsistentSendDependencies(
	ctx context.Context,
	command types.SendMessageCommand,
) (types.ConversationSendContext, types.PermissionDecision, error) {
	var conversation types.ConversationSendContext
	var permission types.PermissionDecision
	for attempt := 0; attempt < 2; attempt++ {
		var err error
		conversation, err = u.conversation.GetSendContext(ctx, command)
		if err != nil {
			return types.ConversationSendContext{}, types.PermissionDecision{}, err
		}

		permission, err = u.policy.CheckSendPermission(ctx, command)
		if err != nil {
			return types.ConversationSendContext{}, types.PermissionDecision{}, err
		}
		if !permission.Allowed || permission.PermissionVersion == conversation.PermissionVersion {
			return conversation, permission, nil
		}
	}
	return types.ConversationSendContext{}, types.PermissionDecision{}, types.NewDependencyVersionMismatch(
		"permission version changed during send dependency read",
	)
}
