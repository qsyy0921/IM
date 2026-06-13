package timeline

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	conversationtimelinev1 "github.com/qsyy0921/IM/schemas/kafka"
	"github.com/qsyy0921/IM/services/delivery-service/internal/types"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

type Consumer interface {
	Fetch(context.Context) (types.TimelineMessage, error)
	Commit(context.Context, types.TimelineMessage) error
}

type Projector interface {
	Execute(context.Context, types.ProjectTimelineEventCommand) (types.ProjectTimelineEventResult, error)
}

type Worker struct {
	consumer      Consumer
	projector     Projector
	consumerGroup string
	recorder      FailureRecorder
	config        Config
	metrics       workerMetrics
}

type Config struct {
	ErrorBackoff time.Duration
	Logf         func(format string, args ...any)
}

type workerMetrics struct {
	totalErrors        atomic.Uint64
	consecutiveErrors  atomic.Uint64
	lastErrorAtMS      atomic.Int64
	lastSuccessAtMS    atomic.Int64
	lastCommitAtMS     atomic.Int64
	lastErrorBackoffMS atomic.Int64
}

func NewWorker(consumer Consumer, projector Projector, consumerGroup string, recorder FailureRecorder, configs ...Config) *Worker {
	var config Config
	if len(configs) > 0 {
		config = configs[0]
	}
	return &Worker{
		consumer:      consumer,
		projector:     projector,
		consumerGroup: consumerGroup,
		recorder:      recorder,
		config:        normalizeConfig(config),
	}
}

func (worker *Worker) Run(ctx context.Context) error {
	if worker.consumer == nil {
		return errors.New("delivery timeline consumer is not configured")
	}
	if worker.projector == nil {
		return errors.New("delivery timeline projector is not configured")
	}
	for {
		retryable, err := worker.runOnce(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return context.Canceled
			}
			if !retryable {
				return err
			}
			if worker.config.Logf != nil {
				worker.config.Logf("delivery-service timeline projection worker retrying after runtime error: %v", err)
			}
			worker.recordError()
			worker.metrics.lastErrorBackoffMS.Store(worker.config.ErrorBackoff.Milliseconds())
			if err := waitForInterval(ctx, worker.config.ErrorBackoff); err != nil {
				return err
			}
			continue
		}
		worker.recordSuccess()
	}
}

func (worker *Worker) Snapshot() types.ProjectionWorkerSnapshot {
	return types.ProjectionWorkerSnapshot{
		TotalErrors:        worker.metrics.totalErrors.Load(),
		ConsecutiveErrors:  worker.metrics.consecutiveErrors.Load(),
		LastErrorAtMS:      worker.metrics.lastErrorAtMS.Load(),
		LastSuccessAtMS:    worker.metrics.lastSuccessAtMS.Load(),
		LastCommitAtMS:     worker.metrics.lastCommitAtMS.Load(),
		LastErrorBackoffMS: worker.metrics.lastErrorBackoffMS.Load(),
	}
}

func (worker *Worker) runOnce(ctx context.Context) (bool, error) {
	message, err := worker.consumer.Fetch(ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return false, context.Canceled
		}
		return true, err
	}
	command, err := buildCommand(worker.consumerGroup, message)
	if err != nil {
		return false, worker.recordAndReturn(ctx, message, err)
	}
	if _, err := worker.projector.Execute(ctx, command); err != nil {
		return false, worker.recordAndReturn(ctx, message, err)
	}
	if err := worker.consumer.Commit(ctx, message); err != nil {
		if errors.Is(err, context.Canceled) {
			return false, context.Canceled
		}
		return true, err
	}
	return false, nil
}

func (worker *Worker) recordAndReturn(ctx context.Context, message types.TimelineMessage, err error) error {
	if worker.recorder == nil {
		return err
	}
	recordErr := worker.recorder.RecordFailure(ctx, bestEffortProjectionFailureRecord(worker.consumerGroup, message, err))
	if recordErr != nil {
		return errors.Join(err, recordErr)
	}
	return err
}

func normalizeConfig(config Config) Config {
	if config.ErrorBackoff <= 0 {
		config.ErrorBackoff = time.Second
	}
	return config
}

func (worker *Worker) recordError() {
	worker.metrics.totalErrors.Add(1)
	worker.metrics.consecutiveErrors.Add(1)
	worker.metrics.lastErrorAtMS.Store(time.Now().UnixMilli())
}

func (worker *Worker) recordSuccess() {
	worker.metrics.consecutiveErrors.Store(0)
	now := time.Now().UnixMilli()
	worker.metrics.lastSuccessAtMS.Store(now)
	worker.metrics.lastCommitAtMS.Store(now)
}

func waitForInterval(ctx context.Context, interval time.Duration) error {
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func buildCommand(consumerGroup string, message types.TimelineMessage) (types.ProjectTimelineEventCommand, error) {
	var event conversationtimelinev1.ConversationTimelineEvent
	if err := proto.Unmarshal(message.Value, &event); err != nil {
		return types.ProjectTimelineEventCommand{}, err
	}
	command := types.ProjectTimelineEventCommand{
		TenantID:        types.TenantID(event.GetTenantId()),
		EventID:         event.GetEventId(),
		EventType:       event.GetEventType(),
		ConversationID:  types.ConversationID(event.GetAggregateId()),
		ConversationSeq: event.GetAggregateVersion(),
		ConsumerGroup:   consumerGroup,
		Topic:           message.Topic,
		PartitionID:     int32(message.Partition),
		OffsetValue:     message.Offset + 1,
		TraceID:         event.GetTraceId(),
		CorrelationID:   event.GetCorrelationId(),
		CausationID:     event.GetCausationId(),
	}
	if metadata := event.GetMetadata(); metadata != nil {
		command.FanoutMode = metadata.GetFanoutMode()
		command.PermissionVersion = metadata.GetPermissionVersion()
	}
	switch payload := event.GetPayload().(type) {
	case *conversationtimelinev1.ConversationTimelineEvent_MessagePersisted:
		fillMessagePersisted(&command, payload.MessagePersisted)
	case *conversationtimelinev1.ConversationTimelineEvent_MessageEdited:
		fillMessageEdited(&command, payload.MessageEdited)
	case *conversationtimelinev1.ConversationTimelineEvent_MessageRevoked:
		fillMessageRevoked(&command, payload.MessageRevoked)
	case *conversationtimelinev1.ConversationTimelineEvent_MessageDeleted:
		fillMessageDeleted(&command, payload.MessageDeleted)
	case *conversationtimelinev1.ConversationTimelineEvent_ConversationMemberJoined:
		fillMemberBoundary(&command, payload.ConversationMemberJoined.GetTargetUserId(), payload.ConversationMemberJoined.GetNewRole(), payload.ConversationMemberJoined.GetNewStatus(), payload.ConversationMemberJoined.GetMemberVersion(), payload.ConversationMemberJoined.GetPermissionVersion())
	case *conversationtimelinev1.ConversationTimelineEvent_ConversationMemberLeft:
		fillMemberBoundary(&command, payload.ConversationMemberLeft.GetTargetUserId(), payload.ConversationMemberLeft.GetNewRole(), payload.ConversationMemberLeft.GetNewStatus(), payload.ConversationMemberLeft.GetMemberVersion(), payload.ConversationMemberLeft.GetPermissionVersion())
	case *conversationtimelinev1.ConversationTimelineEvent_ConversationMemberRemoved:
		fillMemberBoundary(&command, payload.ConversationMemberRemoved.GetTargetUserId(), payload.ConversationMemberRemoved.GetNewRole(), payload.ConversationMemberRemoved.GetNewStatus(), payload.ConversationMemberRemoved.GetMemberVersion(), payload.ConversationMemberRemoved.GetPermissionVersion())
	case *conversationtimelinev1.ConversationTimelineEvent_ConversationMemberRoleChanged:
		fillMemberBoundary(&command, payload.ConversationMemberRoleChanged.GetTargetUserId(), payload.ConversationMemberRoleChanged.GetNewRole(), payload.ConversationMemberRoleChanged.GetNewStatus(), payload.ConversationMemberRoleChanged.GetMemberVersion(), payload.ConversationMemberRoleChanged.GetPermissionVersion())
	case *conversationtimelinev1.ConversationTimelineEvent_ConversationMemberBoundaryCancelled:
		fillMemberBoundary(&command, payload.ConversationMemberBoundaryCancelled.GetTargetUserId(), payload.ConversationMemberBoundaryCancelled.GetNewRole(), payload.ConversationMemberBoundaryCancelled.GetNewStatus(), payload.ConversationMemberBoundaryCancelled.GetMemberVersion(), payload.ConversationMemberBoundaryCancelled.GetPermissionVersion())
	case *conversationtimelinev1.ConversationTimelineEvent_ConversationMemberOwnerTransferred:
		fillOwnerTransferred(&command, payload.ConversationMemberOwnerTransferred)
	default:
		return types.ProjectTimelineEventCommand{}, types.NewInvalidArgument("unsupported timeline payload")
	}
	return command, nil
}

func fillOwnerTransferred(command *types.ProjectTimelineEventCommand, payload *conversationtimelinev1.ConversationMemberOwnerTransferredV1) {
	if payload == nil {
		return
	}
	command.PreviousOwnerUserID = types.UserID(payload.GetPreviousOwnerUserId())
	command.PreviousOwnerNewRole = memberRole(payload.GetPreviousOwnerNewRole())
	command.PreviousOwnerStatus = memberStatus(payload.GetPreviousOwnerStatus())
	command.NewOwnerUserID = types.UserID(payload.GetNewOwnerUserId())
	command.NewOwnerNewRole = memberRole(payload.GetNewOwnerNewRole())
	command.NewOwnerStatus = memberStatus(payload.GetNewOwnerStatus())
	command.MemberVersion = payload.GetMemberVersion()
	command.PermissionVersion = payload.GetPermissionVersion()
}

func fillMessagePersisted(command *types.ProjectTimelineEventCommand, payload *conversationtimelinev1.MessagePersistedV1) {
	if payload == nil {
		return
	}
	command.MessageID = payload.GetMessageId()
	command.SenderID = types.UserID(payload.GetSenderId())
	if payload.GetPayload() == nil {
		command.PayloadJSON = []byte(`{}`)
		return
	}
	command.PayloadJSON, _ = protojson.Marshal(payload.GetPayload())
}

func fillMessageRevoked(command *types.ProjectTimelineEventCommand, payload *conversationtimelinev1.MessageRevokedV1) {
	if payload == nil {
		return
	}
	command.MessageID = payload.GetMessageId()
	command.SenderID = types.UserID(payload.GetRevokedBy())
	command.PayloadJSON, _ = protojson.Marshal(payload)
}

func fillMessageEdited(command *types.ProjectTimelineEventCommand, payload *conversationtimelinev1.MessageEditedV1) {
	if payload == nil {
		return
	}
	command.MessageID = payload.GetMessageId()
	command.SenderID = types.UserID(payload.GetEditedBy())
	command.PayloadJSON, _ = protojson.Marshal(payload)
}

func fillMessageDeleted(command *types.ProjectTimelineEventCommand, payload *conversationtimelinev1.MessageDeletedV1) {
	if payload == nil {
		return
	}
	command.MessageID = payload.GetMessageId()
	command.SenderID = types.UserID(payload.GetDeletedBy())
	command.PayloadJSON, _ = protojson.Marshal(payload)
}

func fillMemberBoundary(
	command *types.ProjectTimelineEventCommand,
	userID string,
	role conversationtimelinev1.ConversationMemberRole,
	status conversationtimelinev1.ConversationMemberStatus,
	memberVersion int64,
	permissionVersion int64,
) {
	command.MemberUserID = types.UserID(userID)
	command.MemberRole = memberRole(role)
	command.MemberStatus = memberStatus(status)
	command.MemberVersion = memberVersion
	command.PermissionVersion = permissionVersion
}

func memberRole(role conversationtimelinev1.ConversationMemberRole) string {
	switch role {
	case conversationtimelinev1.ConversationMemberRole_CONVERSATION_MEMBER_ROLE_OWNER:
		return "OWNER"
	case conversationtimelinev1.ConversationMemberRole_CONVERSATION_MEMBER_ROLE_ADMIN:
		return "ADMIN"
	case conversationtimelinev1.ConversationMemberRole_CONVERSATION_MEMBER_ROLE_MEMBER:
		return "MEMBER"
	default:
		return ""
	}
}

func memberStatus(status conversationtimelinev1.ConversationMemberStatus) string {
	switch status {
	case conversationtimelinev1.ConversationMemberStatus_CONVERSATION_MEMBER_STATUS_ACTIVE:
		return types.DeliveryMemberStatusActive
	case conversationtimelinev1.ConversationMemberStatus_CONVERSATION_MEMBER_STATUS_LEFT:
		return types.DeliveryMemberStatusLeft
	case conversationtimelinev1.ConversationMemberStatus_CONVERSATION_MEMBER_STATUS_BANNED:
		return types.DeliveryMemberStatusBanned
	default:
		return ""
	}
}
