package timeline

import (
	"context"
	"errors"

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
}

func NewWorker(consumer Consumer, projector Projector, consumerGroup string, recorder ...FailureRecorder) *Worker {
	worker := &Worker{consumer: consumer, projector: projector, consumerGroup: consumerGroup}
	if len(recorder) > 0 {
		worker.recorder = recorder[0]
	}
	return worker
}

func (worker *Worker) Run(ctx context.Context) error {
	for {
		message, err := worker.consumer.Fetch(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return context.Canceled
			}
			return err
		}
		command, err := buildCommand(worker.consumerGroup, message)
		if err != nil {
			return worker.recordAndReturn(ctx, message, err)
		}
		if _, err := worker.projector.Execute(ctx, command); err != nil {
			return worker.recordAndReturn(ctx, message, err)
		}
		if err := worker.consumer.Commit(ctx, message); err != nil {
			return err
		}
	}
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
