package app

import (
	"context"
	"errors"
	"time"

	"github.com/qsyy0921/IM/services/message-service/internal/domain"
	"github.com/qsyy0921/IM/services/message-service/internal/types"
)

type SendMessageUseCase struct {
	policy       PolicyCheckPort
	conversation ConversationQueryPort
	sequencer    SequencerPort
	messageRepo  MessageRepository
	admission    AdmissionPort
	metrics      types.LatencyRecorder
	seqFloors    *seqFloorCache
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
		metrics:      types.NoopLatencyRecorder{},
		seqFloors:    newSeqFloorCache(),
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

func WithMetrics(metrics types.LatencyRecorder) SendMessageUseCaseOption {
	return func(useCase *SendMessageUseCase) {
		if metrics != nil {
			useCase.metrics = metrics
		}
	}
}

func (u *SendMessageUseCase) Execute(ctx context.Context, command types.SendMessageCommand) (types.SendMessageResult, error) {
	if err := command.Validate(); err != nil {
		return types.SendMessageResult{}, err
	}
	if u.admission != nil {
		started := time.Now()
		permit, err := u.admission.AdmitSendMessage(ctx)
		u.metrics.ObserveSendMessageAdmission(time.Since(started))
		if err != nil {
			return types.SendMessageResult{}, err
		}
		if permit != nil {
			defer permit.Release()
		}
	}

	started := time.Now()
	conversation, permission, err := u.readConsistentSendDependencies(ctx, command)
	u.metrics.ObserveSendMessageDependencyRead(time.Since(started))
	if err != nil {
		return types.SendMessageResult{}, err
	}
	if !permission.Allowed {
		return types.SendMessageResult{}, types.NewPermissionDenied(permission.Reason)
	}

	allocatedSeq, lease, err := u.allocateConversationSeq(ctx, command, conversation)
	if err != nil {
		return types.SendMessageResult{}, err
	}

	started = time.Now()
	result, err := u.messageRepo.AppendMessage(ctx, domain.AppendMessageInput{
		Command:      command,
		Permission:   permission,
		Conversation: conversation,
		AllocatedSeq: allocatedSeq,
		SeqLeaseID:   lease.LeaseID,
		SeqEpoch:     lease.Epoch,
		SeqExpiresAt: lease.ExpiresAt,
	})
	u.metrics.ObserveSendMessageRepositoryAppendCall(time.Since(started))
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
		started := time.Now()
		conversation, err = u.readConversationSendContext(ctx, command)
		u.metrics.ObserveSendMessageConversationContext(time.Since(started))
		if err != nil {
			return types.ConversationSendContext{}, types.PermissionDecision{}, err
		}

		started = time.Now()
		permission, err = u.policy.CheckSendPermission(ctx, command, conversation)
		u.metrics.ObserveSendMessagePolicyCheck(time.Since(started))
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

func (u *SendMessageUseCase) readConversationSendContext(
	ctx context.Context,
	command types.SendMessageCommand,
) (types.ConversationSendContext, error) {
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		conversation, err := u.conversation.GetSendContext(ctx, command)
		if err == nil {
			return conversation, nil
		}
		if !errors.Is(err, types.ErrDependencyUnavailable) {
			return types.ConversationSendContext{}, err
		}
		lastErr = err
	}
	return types.ConversationSendContext{}, lastErr
}

func (u *SendMessageUseCase) allocateConversationSeq(
	ctx context.Context,
	command types.SendMessageCommand,
	conversation types.ConversationSendContext,
) (int64, types.SeqBlock, error) {
	switch conversation.ConversationMode {
	case types.ConversationModeLocalRowLock:
		return 0, types.SeqBlock{}, nil
	case types.ConversationModeSequencerBlock:
		if u.sequencer == nil {
			return 0, types.SeqBlock{}, types.NewSequencerUnavailable("sequencer client is not configured")
		}
		started := time.Now()
		minimumStartSeq, err := u.seqFloors.minimumStartSeq(ctx, command, u.messageRepo.NextConversationSeqFloor)
		u.metrics.ObserveSendMessageSeqFloor(time.Since(started))
		if err != nil {
			return 0, types.SeqBlock{}, err
		}
		started = time.Now()
		block, err := u.sequencer.AllocateSeqBlock(ctx, command, minimumStartSeq)
		u.metrics.ObserveSendMessageSequencerAllocate(time.Since(started))
		if err != nil {
			return 0, types.SeqBlock{}, err
		}
		if block.StartSeq <= 0 || block.EndSeq != block.StartSeq {
			return 0, types.SeqBlock{}, types.NewSequencerUnavailable("sequencer returned invalid single-message block")
		}
		if block.Epoch <= 0 || block.LeaseID == "" || block.ExpiresAt.IsZero() {
			return 0, types.SeqBlock{}, types.NewSequencerUnavailable("sequencer returned incomplete lease metadata")
		}
		return block.StartSeq, block, nil
	default:
		return 0, types.SeqBlock{}, types.NewSequencerUnavailable("conversation mode is not supported")
	}
}
