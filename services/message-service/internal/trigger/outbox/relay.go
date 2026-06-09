package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	conversationtimelinev1 "github.com/qsyy0921/IM/schemas/kafka"
	"github.com/qsyy0921/IM/services/message-service/internal/types"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const TopicConversationTimelineEvents = "conversation.timeline.events"

type Store interface {
	ProcessReady(
		ctx context.Context,
		limit int,
		maxAttempts int,
		retryBaseDelay time.Duration,
		publish func(context.Context, types.OutboxMessage) error,
	) (types.OutboxRelayStats, error)
}

type BatchStore interface {
	ProcessReadyBatch(
		ctx context.Context,
		limit int,
		maxAttempts int,
		retryBaseDelay time.Duration,
		publish func(context.Context, []types.OutboxMessage) []error,
	) (types.OutboxRelayStats, error)
}

type Publisher interface {
	Publish(ctx context.Context, topic string, key []byte, value []byte) error
}

type BatchPublisher interface {
	PublishBatch(ctx context.Context, topic string, records []types.KafkaPublishRecord) error
}

type Relay struct {
	store     Store
	publisher Publisher
	config    Config
}

type Config struct {
	Topic               string
	BatchSize           int
	WorkerCount         int
	DisablePublishBatch bool
	PollInterval        time.Duration
	FailureBackoff      time.Duration
	MaxAttempts         int
	RetryBaseDelay      time.Duration
	Metrics             types.LatencyRecorder
}

func NewRelay(store Store, publisher Publisher, config Config) *Relay {
	return &Relay{
		store:     store,
		publisher: publisher,
		config:    normalizeConfig(config),
	}
}

func (r *Relay) Run(ctx context.Context) error {
	if r.config.WorkerCount <= 1 {
		return r.runWorker(ctx)
	}

	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	for worker := 0; worker < r.config.WorkerCount; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := r.runWorker(workerCtx); err != nil && !errors.Is(err, context.Canceled) {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
					cancel()
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if firstErr != nil {
		return firstErr
	}
	return ctx.Err()
}

func (r *Relay) runWorker(ctx context.Context) error {
	for {
		stats, err := r.RunOnce(ctx)
		if err != nil && !errors.Is(err, context.Canceled) {
			return err
		}
		if stats.Published > 0 {
			continue
		}
		delay := r.config.PollInterval
		if stats.Fetched > 0 {
			delay = r.config.FailureBackoff
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (r *Relay) RunOnce(ctx context.Context) (types.OutboxRelayStats, error) {
	if r.store == nil {
		return types.OutboxRelayStats{}, errors.New("outbox relay store is not configured")
	}
	if r.publisher == nil {
		return types.OutboxRelayStats{}, errors.New("outbox relay publisher is not configured")
	}
	started := time.Now()
	var stats types.OutboxRelayStats
	var err error
	useSingle := r.config.DisablePublishBatch
	if !useSingle {
		if store, ok := r.store.(BatchStore); ok {
			stats, err = store.ProcessReadyBatch(
				ctx,
				r.config.BatchSize,
				r.config.MaxAttempts,
				r.config.RetryBaseDelay,
				r.publishMessages,
			)
		} else {
			useSingle = true
		}
	}
	if useSingle {
		stats, err = r.store.ProcessReady(
			ctx,
			r.config.BatchSize,
			r.config.MaxAttempts,
			r.config.RetryBaseDelay,
			r.publishMessage,
		)
	}
	r.config.Metrics.ObserveOutboxProcessReadyResult(time.Since(started), stats.Fetched)
	return stats, err
}

func (r *Relay) publishMessage(ctx context.Context, message types.OutboxMessage) error {
	value, err := BuildKafkaValue(message)
	if err != nil {
		return err
	}
	started := time.Now()
	err = r.publisher.Publish(ctx, r.config.Topic, []byte(message.PartitionKey), value)
	r.config.Metrics.ObserveKafkaPublishCall(time.Since(started), 1)
	return err
}

func (r *Relay) publishMessages(ctx context.Context, messages []types.OutboxMessage) []error {
	errs := make([]error, len(messages))
	if len(messages) == 0 {
		return errs
	}
	records := make([]types.KafkaPublishRecord, 0, len(messages))
	indexes := make([]int, 0, len(messages))
	for index, message := range messages {
		value, err := BuildKafkaValue(message)
		if err != nil {
			errs[index] = err
			continue
		}
		records = append(records, types.KafkaPublishRecord{
			Key:   []byte(message.PartitionKey),
			Value: value,
		})
		indexes = append(indexes, index)
	}
	if len(records) == 0 {
		return errs
	}

	started := time.Now()
	if publisher, ok := r.publisher.(BatchPublisher); ok {
		err := publisher.PublishBatch(ctx, r.config.Topic, records)
		r.config.Metrics.ObserveKafkaPublishCall(time.Since(started), len(records))
		if err != nil {
			for _, index := range indexes {
				errs[index] = err
			}
		}
		return errs
	}

	for recordIndex, record := range records {
		err := r.publisher.Publish(ctx, r.config.Topic, record.Key, record.Value)
		if err != nil {
			errs[indexes[recordIndex]] = err
		}
	}
	r.config.Metrics.ObserveKafkaPublishCall(time.Since(started), len(records))
	return errs
}

func BuildKafkaValue(message types.OutboxMessage) ([]byte, error) {
	event, err := BuildConversationTimelineEvent(message)
	if err != nil {
		return nil, err
	}
	return proto.Marshal(event)
}

func BuildConversationTimelineEvent(message types.OutboxMessage) (*conversationtimelinev1.ConversationTimelineEvent, error) {
	switch message.EventType {
	case types.TimelineEventMessagePersisted:
		payload, err := decodeMessagePersistedPayload(message.PayloadJSON)
		if err != nil {
			return nil, err
		}
		acceptedAt, err := time.Parse(time.RFC3339Nano, payload.AcceptedAt)
		if err != nil {
			return nil, err
		}
		payloadStruct, err := structFromRawJSON(payload.Payload)
		if err != nil {
			return nil, err
		}
		occurredAt := message.OccurredAt
		if occurredAt.IsZero() {
			occurredAt = acceptedAt
		}
		event := buildTimelineEnvelope(message, occurredAt)
		event.Payload = &conversationtimelinev1.ConversationTimelineEvent_MessagePersisted{
			MessagePersisted: &conversationtimelinev1.MessagePersistedV1{
				MessageId:       payload.MessageID,
				ConversationId:  payload.ConversationID,
				ConversationSeq: payload.ConversationSeq,
				SenderId:        payload.SenderID,
				DeviceId:        payload.DeviceID,
				ClientMsgId:     payload.ClientMsgID,
				CommandHash:     payload.CommandHash,
				MessageType:     payload.MessageType,
				Payload:         payloadStruct,
				AttachmentIds:   payload.AttachmentIDs,
				AcceptedAt:      timestamppb.New(acceptedAt),
			},
		}
		return event, nil
	case types.TimelineEventMessageEdited:
		payload, err := decodeMessageEditedPayload(message.PayloadJSON)
		if err != nil {
			return nil, err
		}
		editedAt, err := time.Parse(time.RFC3339Nano, payload.EditedAt)
		if err != nil {
			return nil, err
		}
		beforePayload, err := structFromRawJSON(payload.BeforePayload)
		if err != nil {
			return nil, err
		}
		afterPayload, err := structFromRawJSON(payload.AfterPayload)
		if err != nil {
			return nil, err
		}
		occurredAt := message.OccurredAt
		if occurredAt.IsZero() {
			occurredAt = editedAt
		}
		event := buildTimelineEnvelope(message, occurredAt)
		event.Payload = &conversationtimelinev1.ConversationTimelineEvent_MessageEdited{
			MessageEdited: &conversationtimelinev1.MessageEditedV1{
				MessageId:       payload.MessageID,
				ConversationId:  payload.ConversationID,
				ConversationSeq: payload.ConversationSeq,
				ChangeVersion:   payload.ChangeVersion,
				EditedBy:        payload.EditedBy,
				BeforePayload:   beforePayload,
				AfterPayload:    afterPayload,
				Reason:          payload.Reason,
				EditedAt:        timestamppb.New(editedAt),
			},
		}
		return event, nil
	case types.TimelineEventMessageRevoked:
		payload, err := decodeMessageRevokedPayload(message.PayloadJSON)
		if err != nil {
			return nil, err
		}
		revokedAt, err := time.Parse(time.RFC3339Nano, payload.RevokedAt)
		if err != nil {
			return nil, err
		}
		occurredAt := message.OccurredAt
		if occurredAt.IsZero() {
			occurredAt = revokedAt
		}
		event := buildTimelineEnvelope(message, occurredAt)
		event.Payload = &conversationtimelinev1.ConversationTimelineEvent_MessageRevoked{
			MessageRevoked: &conversationtimelinev1.MessageRevokedV1{
				MessageId:       payload.MessageID,
				ConversationId:  payload.ConversationID,
				ConversationSeq: payload.ConversationSeq,
				ChangeVersion:   payload.ChangeVersion,
				RevokedBy:       payload.RevokedBy,
				Reason:          payload.Reason,
				RevokedAt:       timestamppb.New(revokedAt),
			},
		}
		return event, nil
	case types.TimelineEventConversationMemberJoined,
		types.TimelineEventConversationMemberLeft,
		types.TimelineEventConversationMemberRemoved,
		types.TimelineEventConversationMemberRoleChanged,
		types.TimelineEventConversationMemberBoundaryCancelled:
		return buildMemberBoundaryTimelineEvent(message)
	default:
		return nil, errors.New("unsupported outbox event type")
	}
}

func buildTimelineEnvelope(message types.OutboxMessage, occurredAt time.Time) *conversationtimelinev1.ConversationTimelineEvent {
	return &conversationtimelinev1.ConversationTimelineEvent{
		EventId:          string(message.EventID),
		EventType:        string(message.EventType),
		EventVersion:     message.EventVersion,
		TenantId:         string(message.TenantID),
		AggregateType:    "conversation",
		AggregateId:      string(message.ConversationID),
		AggregateVersion: message.AggregateVersion,
		PartitionKey:     message.PartitionKey,
		MappingVersion:   message.MappingVersion,
		TraceId:          message.TraceID,
		CorrelationId:    message.CorrelationID,
		CausationId:      message.CausationID,
		Producer:         message.Producer,
		OccurredAt:       timestamppb.New(occurredAt),
		Metadata: &conversationtimelinev1.TimelineMetadata{
			FanoutMode:          string(message.FanoutMode),
			FanoutPolicyVersion: message.FanoutPolicyVersion,
			PermissionVersion:   message.PermissionVersion,
			Classification:      message.Classification,
			MappingVersion:      message.MappingVersion,
		},
	}
}

func buildMemberBoundaryTimelineEvent(message types.OutboxMessage) (*conversationtimelinev1.ConversationTimelineEvent, error) {
	payload, err := decodeMemberBoundaryPayload(message.PayloadJSON)
	if err != nil {
		return nil, err
	}
	occurredAt, err := time.Parse(time.RFC3339Nano, payload.OccurredAt)
	if err != nil {
		return nil, err
	}
	changeType, err := conversationMemberChangeType(payload.ChangeType)
	if err != nil {
		return nil, err
	}
	oldRole, err := conversationMemberRole(payload.OldRole)
	if err != nil {
		return nil, err
	}
	newRole, err := conversationMemberRole(payload.NewRole)
	if err != nil {
		return nil, err
	}
	oldStatus, err := conversationMemberStatus(payload.OldStatus)
	if err != nil {
		return nil, err
	}
	newStatus, err := conversationMemberStatus(payload.NewStatus)
	if err != nil {
		return nil, err
	}

	event := buildTimelineEnvelope(message, occurredAt)
	member := memberBoundaryProtoPayload{
		ChangeID:          payload.ChangeID,
		ConversationID:    payload.ConversationID,
		BoundarySeq:       payload.BoundarySeq,
		TargetUserID:      payload.TargetUserID,
		OperatorUserID:    payload.OperatorUserID,
		ChangeType:        changeType,
		OldRole:           oldRole,
		NewRole:           newRole,
		OldStatus:         oldStatus,
		NewStatus:         newStatus,
		MemberVersion:     payload.MemberVersion,
		PermissionVersion: payload.PermissionVersion,
		Reason:            payload.Reason,
		OccurredAt:        timestamppb.New(occurredAt),
	}
	switch message.EventType {
	case types.TimelineEventConversationMemberJoined:
		event.Payload = &conversationtimelinev1.ConversationTimelineEvent_ConversationMemberJoined{
			ConversationMemberJoined: member.joined(),
		}
	case types.TimelineEventConversationMemberLeft:
		event.Payload = &conversationtimelinev1.ConversationTimelineEvent_ConversationMemberLeft{
			ConversationMemberLeft: member.left(),
		}
	case types.TimelineEventConversationMemberRemoved:
		event.Payload = &conversationtimelinev1.ConversationTimelineEvent_ConversationMemberRemoved{
			ConversationMemberRemoved: member.removed(),
		}
	case types.TimelineEventConversationMemberRoleChanged:
		event.Payload = &conversationtimelinev1.ConversationTimelineEvent_ConversationMemberRoleChanged{
			ConversationMemberRoleChanged: member.roleChanged(),
		}
	case types.TimelineEventConversationMemberBoundaryCancelled:
		event.Payload = &conversationtimelinev1.ConversationTimelineEvent_ConversationMemberBoundaryCancelled{
			ConversationMemberBoundaryCancelled: member.cancelled(),
		}
	default:
		return nil, errors.New("unsupported member boundary event type")
	}
	return event, nil
}

type messagePersistedPayload struct {
	MessageID       string          `json:"message_id"`
	ConversationID  string          `json:"conversation_id"`
	ConversationSeq int64           `json:"conversation_seq"`
	SenderID        string          `json:"sender_id"`
	DeviceID        string          `json:"device_id"`
	ClientMsgID     string          `json:"client_msg_id"`
	CommandHash     string          `json:"command_hash"`
	MessageType     string          `json:"message_type"`
	Payload         json.RawMessage `json:"payload"`
	AttachmentIDs   []string        `json:"attachment_ids"`
	AcceptedAt      string          `json:"accepted_at"`
}

type messageRevokedPayload struct {
	MessageID       string `json:"message_id"`
	ConversationID  string `json:"conversation_id"`
	ConversationSeq int64  `json:"conversation_seq"`
	ChangeVersion   int32  `json:"change_version"`
	RevokedBy       string `json:"revoked_by"`
	Reason          string `json:"reason"`
	RevokedAt       string `json:"revoked_at"`
}

type messageEditedPayload struct {
	MessageID       string          `json:"message_id"`
	ConversationID  string          `json:"conversation_id"`
	ConversationSeq int64           `json:"conversation_seq"`
	ChangeVersion   int32           `json:"change_version"`
	EditedBy        string          `json:"edited_by"`
	BeforePayload   json.RawMessage `json:"before_payload"`
	AfterPayload    json.RawMessage `json:"after_payload"`
	Reason          string          `json:"reason"`
	EditedAt        string          `json:"edited_at"`
}

func decodeMessagePersistedPayload(payloadJSON []byte) (messagePersistedPayload, error) {
	var payload messagePersistedPayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return messagePersistedPayload{}, err
	}
	if payload.MessageID == "" ||
		payload.ConversationID == "" ||
		payload.ConversationSeq <= 0 ||
		payload.CommandHash == "" ||
		len(payload.Payload) == 0 ||
		payload.AcceptedAt == "" {
		return messagePersistedPayload{}, errors.New("message persisted payload is incomplete")
	}
	return payload, nil
}

func decodeMessageRevokedPayload(payloadJSON []byte) (messageRevokedPayload, error) {
	var payload messageRevokedPayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return messageRevokedPayload{}, err
	}
	if payload.MessageID == "" ||
		payload.ConversationID == "" ||
		payload.ConversationSeq <= 0 ||
		payload.ChangeVersion <= 0 ||
		payload.RevokedBy == "" ||
		payload.RevokedAt == "" {
		return messageRevokedPayload{}, errors.New("message revoked payload is incomplete")
	}
	return payload, nil
}

func decodeMessageEditedPayload(payloadJSON []byte) (messageEditedPayload, error) {
	var payload messageEditedPayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return messageEditedPayload{}, err
	}
	if payload.MessageID == "" ||
		payload.ConversationID == "" ||
		payload.ConversationSeq <= 0 ||
		payload.ChangeVersion <= 0 ||
		payload.EditedBy == "" ||
		len(payload.BeforePayload) == 0 ||
		len(payload.AfterPayload) == 0 ||
		payload.EditedAt == "" {
		return messageEditedPayload{}, errors.New("message edited payload is incomplete")
	}
	return payload, nil
}

type memberBoundaryPayload struct {
	ChangeID          string `json:"change_id"`
	ConversationID    string `json:"conversation_id"`
	BoundarySeq       int64  `json:"boundary_seq"`
	TargetUserID      string `json:"target_user_id"`
	OperatorUserID    string `json:"operator_user_id"`
	ChangeType        string `json:"change_type"`
	OldRole           string `json:"old_role"`
	NewRole           string `json:"new_role"`
	OldStatus         string `json:"old_status"`
	NewStatus         string `json:"new_status"`
	MemberVersion     int64  `json:"member_version"`
	PermissionVersion int64  `json:"permission_version"`
	Reason            string `json:"reason"`
	OccurredAt        string `json:"occurred_at"`
}

func decodeMemberBoundaryPayload(payloadJSON []byte) (memberBoundaryPayload, error) {
	var payload memberBoundaryPayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return memberBoundaryPayload{}, err
	}
	if payload.ChangeID == "" ||
		payload.ConversationID == "" ||
		payload.BoundarySeq <= 0 ||
		payload.TargetUserID == "" ||
		payload.OperatorUserID == "" ||
		payload.ChangeType == "" ||
		payload.MemberVersion <= 0 ||
		payload.PermissionVersion <= 0 ||
		payload.OccurredAt == "" {
		return memberBoundaryPayload{}, errors.New("member boundary payload is incomplete")
	}
	return payload, nil
}

type memberBoundaryProtoPayload struct {
	ChangeID          string
	ConversationID    string
	BoundarySeq       int64
	TargetUserID      string
	OperatorUserID    string
	ChangeType        conversationtimelinev1.ConversationMemberChangeType
	OldRole           conversationtimelinev1.ConversationMemberRole
	NewRole           conversationtimelinev1.ConversationMemberRole
	OldStatus         conversationtimelinev1.ConversationMemberStatus
	NewStatus         conversationtimelinev1.ConversationMemberStatus
	MemberVersion     int64
	PermissionVersion int64
	Reason            string
	OccurredAt        *timestamppb.Timestamp
}

func (p memberBoundaryProtoPayload) joined() *conversationtimelinev1.ConversationMemberJoinedV1 {
	return &conversationtimelinev1.ConversationMemberJoinedV1{
		ChangeId:          p.ChangeID,
		ConversationId:    p.ConversationID,
		BoundarySeq:       p.BoundarySeq,
		TargetUserId:      p.TargetUserID,
		OperatorUserId:    p.OperatorUserID,
		ChangeType:        p.ChangeType,
		OldRole:           p.OldRole,
		NewRole:           p.NewRole,
		OldStatus:         p.OldStatus,
		NewStatus:         p.NewStatus,
		MemberVersion:     p.MemberVersion,
		PermissionVersion: p.PermissionVersion,
		Reason:            p.Reason,
		OccurredAt:        p.OccurredAt,
	}
}

func (p memberBoundaryProtoPayload) left() *conversationtimelinev1.ConversationMemberLeftV1 {
	return &conversationtimelinev1.ConversationMemberLeftV1{
		ChangeId:          p.ChangeID,
		ConversationId:    p.ConversationID,
		BoundarySeq:       p.BoundarySeq,
		TargetUserId:      p.TargetUserID,
		OperatorUserId:    p.OperatorUserID,
		ChangeType:        p.ChangeType,
		OldRole:           p.OldRole,
		NewRole:           p.NewRole,
		OldStatus:         p.OldStatus,
		NewStatus:         p.NewStatus,
		MemberVersion:     p.MemberVersion,
		PermissionVersion: p.PermissionVersion,
		Reason:            p.Reason,
		OccurredAt:        p.OccurredAt,
	}
}

func (p memberBoundaryProtoPayload) removed() *conversationtimelinev1.ConversationMemberRemovedV1 {
	return &conversationtimelinev1.ConversationMemberRemovedV1{
		ChangeId:          p.ChangeID,
		ConversationId:    p.ConversationID,
		BoundarySeq:       p.BoundarySeq,
		TargetUserId:      p.TargetUserID,
		OperatorUserId:    p.OperatorUserID,
		ChangeType:        p.ChangeType,
		OldRole:           p.OldRole,
		NewRole:           p.NewRole,
		OldStatus:         p.OldStatus,
		NewStatus:         p.NewStatus,
		MemberVersion:     p.MemberVersion,
		PermissionVersion: p.PermissionVersion,
		Reason:            p.Reason,
		OccurredAt:        p.OccurredAt,
	}
}

func (p memberBoundaryProtoPayload) roleChanged() *conversationtimelinev1.ConversationMemberRoleChangedV1 {
	return &conversationtimelinev1.ConversationMemberRoleChangedV1{
		ChangeId:          p.ChangeID,
		ConversationId:    p.ConversationID,
		BoundarySeq:       p.BoundarySeq,
		TargetUserId:      p.TargetUserID,
		OperatorUserId:    p.OperatorUserID,
		ChangeType:        p.ChangeType,
		OldRole:           p.OldRole,
		NewRole:           p.NewRole,
		OldStatus:         p.OldStatus,
		NewStatus:         p.NewStatus,
		MemberVersion:     p.MemberVersion,
		PermissionVersion: p.PermissionVersion,
		Reason:            p.Reason,
		OccurredAt:        p.OccurredAt,
	}
}

func (p memberBoundaryProtoPayload) cancelled() *conversationtimelinev1.ConversationMemberBoundaryCancelledV1 {
	return &conversationtimelinev1.ConversationMemberBoundaryCancelledV1{
		ChangeId:          p.ChangeID,
		ConversationId:    p.ConversationID,
		BoundarySeq:       p.BoundarySeq,
		TargetUserId:      p.TargetUserID,
		OperatorUserId:    p.OperatorUserID,
		ChangeType:        p.ChangeType,
		OldRole:           p.OldRole,
		NewRole:           p.NewRole,
		OldStatus:         p.OldStatus,
		NewStatus:         p.NewStatus,
		MemberVersion:     p.MemberVersion,
		PermissionVersion: p.PermissionVersion,
		Reason:            p.Reason,
		OccurredAt:        p.OccurredAt,
	}
}

func conversationMemberChangeType(value string) (conversationtimelinev1.ConversationMemberChangeType, error) {
	switch strings.ToUpper(value) {
	case "JOIN":
		return conversationtimelinev1.ConversationMemberChangeType_CONVERSATION_MEMBER_CHANGE_TYPE_JOIN, nil
	case "LEAVE":
		return conversationtimelinev1.ConversationMemberChangeType_CONVERSATION_MEMBER_CHANGE_TYPE_LEAVE, nil
	case "REMOVE":
		return conversationtimelinev1.ConversationMemberChangeType_CONVERSATION_MEMBER_CHANGE_TYPE_REMOVE, nil
	case "ROLE_CHANGED":
		return conversationtimelinev1.ConversationMemberChangeType_CONVERSATION_MEMBER_CHANGE_TYPE_ROLE_CHANGED, nil
	default:
		return conversationtimelinev1.ConversationMemberChangeType_CONVERSATION_MEMBER_CHANGE_TYPE_UNSPECIFIED, errors.New("unknown member change type")
	}
}

func conversationMemberRole(value string) (conversationtimelinev1.ConversationMemberRole, error) {
	switch strings.ToUpper(value) {
	case "":
		return conversationtimelinev1.ConversationMemberRole_CONVERSATION_MEMBER_ROLE_UNSPECIFIED, nil
	case "OWNER":
		return conversationtimelinev1.ConversationMemberRole_CONVERSATION_MEMBER_ROLE_OWNER, nil
	case "ADMIN":
		return conversationtimelinev1.ConversationMemberRole_CONVERSATION_MEMBER_ROLE_ADMIN, nil
	case "MEMBER":
		return conversationtimelinev1.ConversationMemberRole_CONVERSATION_MEMBER_ROLE_MEMBER, nil
	default:
		return conversationtimelinev1.ConversationMemberRole_CONVERSATION_MEMBER_ROLE_UNSPECIFIED, errors.New("unknown member role")
	}
}

func conversationMemberStatus(value string) (conversationtimelinev1.ConversationMemberStatus, error) {
	switch strings.ToUpper(value) {
	case "":
		return conversationtimelinev1.ConversationMemberStatus_CONVERSATION_MEMBER_STATUS_UNSPECIFIED, nil
	case "ACTIVE":
		return conversationtimelinev1.ConversationMemberStatus_CONVERSATION_MEMBER_STATUS_ACTIVE, nil
	case "LEFT":
		return conversationtimelinev1.ConversationMemberStatus_CONVERSATION_MEMBER_STATUS_LEFT, nil
	case "BANNED":
		return conversationtimelinev1.ConversationMemberStatus_CONVERSATION_MEMBER_STATUS_BANNED, nil
	default:
		return conversationtimelinev1.ConversationMemberStatus_CONVERSATION_MEMBER_STATUS_UNSPECIFIED, errors.New("unknown member status")
	}
}

func structFromRawJSON(payload json.RawMessage) (*structpb.Struct, error) {
	var object map[string]any
	if err := json.Unmarshal(payload, &object); err != nil {
		return nil, err
	}
	return structpb.NewStruct(object)
}

func normalizeConfig(config Config) Config {
	if config.Topic == "" {
		config.Topic = TopicConversationTimelineEvents
	}
	if config.BatchSize <= 0 {
		config.BatchSize = 500
	}
	if config.WorkerCount <= 0 {
		config.WorkerCount = 1
	}
	if config.PollInterval <= 0 {
		config.PollInterval = time.Second
	}
	if config.FailureBackoff <= 0 {
		config.FailureBackoff = config.PollInterval
	}
	if config.MaxAttempts <= 0 {
		config.MaxAttempts = 5
	}
	if config.RetryBaseDelay <= 0 {
		config.RetryBaseDelay = time.Second
	}
	if config.Metrics == nil {
		config.Metrics = types.NoopLatencyRecorder{}
	}
	return config
}
