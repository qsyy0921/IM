package timeline

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	conversationtimelinev1 "github.com/qsyy0921/IM/schemas/kafka"
	"github.com/qsyy0921/IM/services/memory-service/internal/types"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

const TopicConversationTimelineEvents = "conversation.timeline.events"

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
	config        Config
}

type Config struct {
	ErrorBackoff time.Duration
	Logf         func(format string, args ...any)
}

func NewWorker(consumer Consumer, projector Projector, consumerGroup string, configs ...Config) *Worker {
	var config Config
	if len(configs) > 0 {
		config = configs[0]
	}
	if config.ErrorBackoff <= 0 {
		config.ErrorBackoff = time.Second
	}
	return &Worker{
		consumer:      consumer,
		projector:     projector,
		consumerGroup: consumerGroup,
		config:        config,
	}
}

func (worker *Worker) Run(ctx context.Context) error {
	for {
		err := worker.RunOnce(ctx)
		if err == nil {
			continue
		}
		if errors.Is(err, context.Canceled) {
			return context.Canceled
		}
		if worker.config.Logf != nil {
			worker.config.Logf("memory-service timeline projection worker retrying after error: %v", err)
		}
		timer := time.NewTimer(worker.config.ErrorBackoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (worker *Worker) RunOnce(ctx context.Context) error {
	if worker == nil || worker.consumer == nil {
		return errors.New("memory timeline consumer is not configured")
	}
	if worker.projector == nil {
		return errors.New("memory timeline projector is not configured")
	}
	message, err := worker.consumer.Fetch(ctx)
	if err != nil {
		return err
	}
	command, err := buildCommand(worker.consumerGroup, message)
	if err != nil {
		return err
	}
	if _, err := worker.projector.Execute(ctx, command); err != nil {
		return err
	}
	return worker.consumer.Commit(ctx, message)
}

func buildCommand(consumerGroup string, message types.TimelineMessage) (types.ProjectTimelineEventCommand, error) {
	var event conversationtimelinev1.ConversationTimelineEvent
	if err := proto.Unmarshal(message.Value, &event); err != nil {
		return types.ProjectTimelineEventCommand{}, err
	}
	command := types.ProjectTimelineEventCommand{
		TenantID:          types.TenantID(event.GetTenantId()),
		EventID:           event.GetEventId(),
		EventType:         event.GetEventType(),
		ConversationID:    types.ConversationID(event.GetAggregateId()),
		ConversationSeq:   event.GetAggregateVersion(),
		MemoryEventID:     event.GetEventId(),
		ExtractionVersion: "rules-v0.1",
		ConsumerGroup:     consumerGroup,
		Topic:             message.Topic,
		PartitionID:       int32(message.Partition),
		OffsetValue:       message.Offset + 1,
		TraceID:           event.GetTraceId(),
		CorrelationID:     event.GetCorrelationId(),
		CausationID:       event.GetCausationId(),
	}
	if metadata := event.GetMetadata(); metadata != nil {
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
		return types.ProjectTimelineEventCommand{}, types.NewUnsupportedPayload("unsupported timeline payload")
	}
	if err := command.Validate(); err != nil {
		return types.ProjectTimelineEventCommand{}, err
	}
	return command, nil
}

func fillMessagePersisted(command *types.ProjectTimelineEventCommand, payload *conversationtimelinev1.MessagePersistedV1) {
	if payload == nil {
		return
	}
	command.MessageID = payload.GetMessageId()
	command.SenderID = types.UserID(payload.GetSenderId())
	command.FactText = durableFactTextFromStruct(payload.GetPayload())
}

func fillMessageEdited(command *types.ProjectTimelineEventCommand, payload *conversationtimelinev1.MessageEditedV1) {
	if payload == nil {
		return
	}
	command.MessageID = payload.GetMessageId()
	command.SenderID = types.UserID(payload.GetEditedBy())
	command.FactText = durableFactTextFromStruct(payload.GetAfterPayload())
}

func fillMessageRevoked(command *types.ProjectTimelineEventCommand, payload *conversationtimelinev1.MessageRevokedV1) {
	if payload == nil {
		return
	}
	command.MessageID = payload.GetMessageId()
	command.SenderID = types.UserID(payload.GetRevokedBy())
}

func fillMessageDeleted(command *types.ProjectTimelineEventCommand, payload *conversationtimelinev1.MessageDeletedV1) {
	if payload == nil {
		return
	}
	command.MessageID = payload.GetMessageId()
	command.SenderID = types.UserID(payload.GetDeletedBy())
}

func fillMemberBoundary(command *types.ProjectTimelineEventCommand, userID string, role conversationtimelinev1.ConversationMemberRole, status conversationtimelinev1.ConversationMemberStatus, memberVersion int64, permissionVersion int64) {
	command.TargetUserID = types.UserID(userID)
	command.MemberRole = memberRole(role)
	command.MemberStatus = memberStatus(status)
	command.MemberVersion = memberVersion
	command.PermissionVersion = permissionVersion
}

func fillOwnerTransferred(command *types.ProjectTimelineEventCommand, payload *conversationtimelinev1.ConversationMemberOwnerTransferredV1) {
	if payload == nil {
		return
	}
	command.PreviousOwnerUserID = types.UserID(payload.GetPreviousOwnerUserId())
	command.PreviousOwnerRole = memberRole(payload.GetPreviousOwnerNewRole())
	command.PreviousOwnerStatus = memberStatus(payload.GetPreviousOwnerStatus())
	command.NewOwnerUserID = types.UserID(payload.GetNewOwnerUserId())
	command.NewOwnerRole = memberRole(payload.GetNewOwnerNewRole())
	command.NewOwnerStatus = memberStatus(payload.GetNewOwnerStatus())
	command.MemberVersion = payload.GetMemberVersion()
	command.PermissionVersion = payload.GetPermissionVersion()
}

func memberRole(role conversationtimelinev1.ConversationMemberRole) string {
	switch role {
	case conversationtimelinev1.ConversationMemberRole_CONVERSATION_MEMBER_ROLE_OWNER:
		return types.MemoryMemberRoleOwner
	case conversationtimelinev1.ConversationMemberRole_CONVERSATION_MEMBER_ROLE_ADMIN:
		return types.MemoryMemberRoleAdmin
	case conversationtimelinev1.ConversationMemberRole_CONVERSATION_MEMBER_ROLE_MEMBER:
		return types.MemoryMemberRoleMember
	default:
		return ""
	}
}

func memberStatus(status conversationtimelinev1.ConversationMemberStatus) string {
	switch status {
	case conversationtimelinev1.ConversationMemberStatus_CONVERSATION_MEMBER_STATUS_ACTIVE:
		return types.MemoryMemberStatusActive
	case conversationtimelinev1.ConversationMemberStatus_CONVERSATION_MEMBER_STATUS_LEFT:
		return types.MemoryMemberStatusLeft
	case conversationtimelinev1.ConversationMemberStatus_CONVERSATION_MEMBER_STATUS_BANNED:
		return types.MemoryMemberStatusBanned
	default:
		return ""
	}
}

func durableFactTextFromStruct(payload *structpb.Struct) string {
	if payload == nil {
		return ""
	}
	for _, key := range []string{"memory_fact", "decision", "task", "status", "text", "content", "body", "title"} {
		if value := strings.TrimSpace(payload.GetFields()[key].GetStringValue()); value != "" {
			return value
		}
	}
	var values []string
	collectStructStrings(&values, payload.AsMap())
	return strings.TrimSpace(strings.Join(values, "\n"))
}

func collectStructStrings(values *[]string, value any) {
	switch typed := value.(type) {
	case string:
		if text := strings.TrimSpace(typed); text != "" {
			*values = append(*values, text)
		}
	case []any:
		for _, item := range typed {
			collectStructStrings(values, item)
		}
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			collectStructStrings(values, typed[key])
		}
	}
}
