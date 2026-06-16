package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"time"

	contacteventsv1 "github.com/qsyy0921/IM/schemas/kafka/contacts/v1"
	"github.com/qsyy0921/IM/services/contacts-service/internal/types"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const TopicContactEvents = "im.contact.events"

type Store interface {
	ProcessReadyBatch(
		ctx context.Context,
		limit int,
		maxAttempts int,
		retryBaseDelay time.Duration,
		publish func(context.Context, []types.OutboxMessage) []error,
	) (types.OutboxRelayStats, error)
}

type Publisher interface {
	PublishBatch(ctx context.Context, topic string, records []types.KafkaPublishRecord) error
}

type Relay struct {
	store     Store
	publisher Publisher
	config    Config
	metrics   relayMetrics
}

type Config struct {
	Topic          string
	BatchSize      int
	PollInterval   time.Duration
	MaxAttempts    int
	RetryBaseDelay time.Duration
	ErrorBackoff   time.Duration
	Logf           func(format string, args ...any)
}

type relayMetrics struct {
	totalErrors        atomic.Uint64
	consecutiveErrors  atomic.Uint64
	lastErrorAtMS      atomic.Int64
	lastSuccessAtMS    atomic.Int64
	lastPublishedAtMS  atomic.Int64
	lastErrorBackoffMS atomic.Int64
}

func NewRelay(store Store, publisher Publisher, config Config) *Relay {
	return &Relay{
		store:     store,
		publisher: publisher,
		config:    normalizeConfig(config),
	}
}

func (relay *Relay) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		stats, err := relay.RunOnce(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return context.Canceled
			}
			if relay.config.Logf != nil {
				relay.config.Logf("contacts-service outbox relay retrying after runtime error: %v", err)
			}
			relay.recordError()
			relay.metrics.lastErrorBackoffMS.Store(relay.config.ErrorBackoff.Milliseconds())
			if err := waitForInterval(ctx, relay.config.ErrorBackoff); err != nil {
				return err
			}
			continue
		}
		relay.recordSuccess(stats)
		if stats.Published > 0 {
			continue
		}
		if err := waitForInterval(ctx, relay.config.PollInterval); err != nil {
			return err
		}
	}
}

func (relay *Relay) Snapshot() types.OutboxRelayWorkerSnapshot {
	return types.OutboxRelayWorkerSnapshot{
		TotalErrors:        relay.metrics.totalErrors.Load(),
		ConsecutiveErrors:  relay.metrics.consecutiveErrors.Load(),
		LastErrorAtMS:      relay.metrics.lastErrorAtMS.Load(),
		LastSuccessAtMS:    relay.metrics.lastSuccessAtMS.Load(),
		LastPublishedAtMS:  relay.metrics.lastPublishedAtMS.Load(),
		LastErrorBackoffMS: relay.metrics.lastErrorBackoffMS.Load(),
	}
}

func (relay *Relay) RunOnce(ctx context.Context) (types.OutboxRelayStats, error) {
	if relay == nil || relay.store == nil {
		return types.OutboxRelayStats{}, errors.New("contacts outbox relay store is not configured")
	}
	if relay.publisher == nil {
		return types.OutboxRelayStats{}, errors.New("contacts outbox relay publisher is not configured")
	}
	return relay.store.ProcessReadyBatch(
		ctx,
		relay.config.BatchSize,
		relay.config.MaxAttempts,
		relay.config.RetryBaseDelay,
		relay.publishMessages,
	)
}

func (relay *Relay) publishMessages(ctx context.Context, messages []types.OutboxMessage) []error {
	errs := make([]error, len(messages))
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
	if err := relay.publisher.PublishBatch(ctx, relay.config.Topic, records); err != nil {
		for _, index := range indexes {
			errs[index] = err
		}
	}
	return errs
}

func BuildKafkaValue(message types.OutboxMessage) ([]byte, error) {
	event, err := BuildContactEvent(message)
	if err != nil {
		return nil, err
	}
	return proto.Marshal(event)
}

func BuildContactEvent(message types.OutboxMessage) (*contacteventsv1.ContactEvent, error) {
	if message.EventID == "" ||
		message.EventType == "" ||
		message.EventVersion == "" ||
		message.TenantID == "" ||
		message.AggregateType == "" ||
		message.AggregateID == "" ||
		message.AggregateVersion <= 0 ||
		message.PartitionKey == "" ||
		message.MappingVersion <= 0 ||
		message.Producer == "" {
		return nil, errors.New("contacts outbox envelope is incomplete")
	}
	event := &contacteventsv1.ContactEvent{
		EventId:          message.EventID,
		EventType:        message.EventType,
		EventVersion:     message.EventVersion,
		TenantId:         string(message.TenantID),
		AggregateType:    message.AggregateType,
		AggregateId:      message.AggregateID,
		AggregateVersion: message.AggregateVersion,
		PartitionKey:     message.PartitionKey,
		MappingVersion:   message.MappingVersion,
		TraceId:          message.TraceID,
		CorrelationId:    message.CorrelationID,
		CausationId:      message.CausationID,
		Producer:         message.Producer,
		OccurredAt:       timestamppb.New(message.OccurredAt),
	}
	switch message.EventType {
	case types.ContactEventRequestCreated:
		payload, err := decodeContactRequestPayload(message.PayloadJSON)
		if err != nil {
			return nil, err
		}
		event.Payload = &contacteventsv1.ContactEvent_RequestCreated{
			RequestCreated: &contacteventsv1.ContactRequestCreatedV1{
				TenantId:       payload.TenantID,
				RequestId:      payload.RequestID,
				SenderUserId:   payload.SenderUserID,
				ReceiverUserId: payload.ReceiverUserID,
				Status:         payload.Status,
				Message:        payload.Message,
				OccurredAt:     payload.Timestamp(),
				SourceType:     payload.SourceType,
				SourceRef:      payload.SourceRef,
				RiskLevel:      payload.RiskLevelValue(),
				ReviewRequired: payload.ReviewRequired,
			},
		}
		return event, nil
	case types.ContactEventRequestAccepted:
		payload, err := decodeContactResponsePayload(message.PayloadJSON, true)
		if err != nil {
			return nil, err
		}
		event.Payload = &contacteventsv1.ContactEvent_RequestAccepted{
			RequestAccepted: &contacteventsv1.ContactRequestAcceptedV1{
				TenantId:       payload.TenantID,
				RequestId:      payload.RequestID,
				SenderUserId:   payload.SenderUserID,
				ReceiverUserId: payload.ReceiverUserID,
				Status:         payload.Status,
				EdgeVersion:    payload.EdgeVersion,
				OccurredAt:     payload.Timestamp(),
			},
		}
		return event, nil
	case types.ContactEventRequestDeclined:
		payload, err := decodeContactResponsePayload(message.PayloadJSON, false)
		if err != nil {
			return nil, err
		}
		event.Payload = &contacteventsv1.ContactEvent_RequestDeclined{
			RequestDeclined: &contacteventsv1.ContactRequestDeclinedV1{
				TenantId:       payload.TenantID,
				RequestId:      payload.RequestID,
				SenderUserId:   payload.SenderUserID,
				ReceiverUserId: payload.ReceiverUserID,
				Status:         payload.Status,
				OccurredAt:     payload.Timestamp(),
			},
		}
		return event, nil
	case types.ContactEventRequestCanceled:
		payload, err := decodeContactResponsePayload(message.PayloadJSON, false)
		if err != nil {
			return nil, err
		}
		event.Payload = &contacteventsv1.ContactEvent_RequestCanceled{
			RequestCanceled: &contacteventsv1.ContactRequestCanceledV1{
				TenantId:       payload.TenantID,
				RequestId:      payload.RequestID,
				SenderUserId:   payload.SenderUserID,
				ReceiverUserId: payload.ReceiverUserID,
				Status:         payload.Status,
				OccurredAt:     payload.Timestamp(),
			},
		}
		return event, nil
	case types.ContactEventEdgeDeleted:
		payload, err := decodeContactEdgePayload(message.PayloadJSON, true)
		if err != nil {
			return nil, err
		}
		event.Payload = &contacteventsv1.ContactEvent_EdgeDeleted{
			EdgeDeleted: &contacteventsv1.ContactEdgeDeletedV1{
				TenantId:       payload.TenantID,
				OwnerUserId:    payload.OwnerUserID,
				ContactUserId:  payload.ContactUserID,
				PreviousStatus: payload.PreviousStatus,
				Status:         payload.Status,
				EdgeVersion:    payload.EdgeVersion,
				OccurredAt:     payload.Timestamp(),
			},
		}
		return event, nil
	case types.ContactEventEdgeBlocked:
		payload, err := decodeContactEdgePayload(message.PayloadJSON, true)
		if err != nil {
			return nil, err
		}
		event.Payload = &contacteventsv1.ContactEvent_EdgeBlocked{
			EdgeBlocked: &contacteventsv1.ContactEdgeBlockedV1{
				TenantId:       payload.TenantID,
				OwnerUserId:    payload.OwnerUserID,
				ContactUserId:  payload.ContactUserID,
				PreviousStatus: payload.PreviousStatus,
				Status:         payload.Status,
				EdgeVersion:    payload.EdgeVersion,
				Reason:         payload.Reason,
				OccurredAt:     payload.Timestamp(),
			},
		}
		return event, nil
	case types.ContactEventEdgeUnblocked:
		payload, err := decodeContactEdgePayload(message.PayloadJSON, true)
		if err != nil {
			return nil, err
		}
		event.Payload = &contacteventsv1.ContactEvent_EdgeUnblocked{
			EdgeUnblocked: &contacteventsv1.ContactEdgeUnblockedV1{
				TenantId:       payload.TenantID,
				OwnerUserId:    payload.OwnerUserID,
				ContactUserId:  payload.ContactUserID,
				PreviousStatus: payload.PreviousStatus,
				Status:         payload.Status,
				EdgeVersion:    payload.EdgeVersion,
				OccurredAt:     payload.Timestamp(),
			},
		}
		return event, nil
	case types.ContactEventRemarkUpdated:
		payload, err := decodeContactEdgePayload(message.PayloadJSON, false)
		if err != nil {
			return nil, err
		}
		event.Payload = &contacteventsv1.ContactEvent_EdgeRemarkUpdated{
			EdgeRemarkUpdated: &contacteventsv1.ContactEdgeRemarkUpdatedV1{
				TenantId:      payload.TenantID,
				OwnerUserId:   payload.OwnerUserID,
				ContactUserId: payload.ContactUserID,
				Status:        payload.Status,
				EdgeVersion:   payload.EdgeVersion,
				Remark:        payload.Remark,
				OccurredAt:    payload.Timestamp(),
			},
		}
		return event, nil
	case types.ContactEventGroupUpdated:
		payload, err := decodeContactEdgePayload(message.PayloadJSON, false)
		if err != nil {
			return nil, err
		}
		event.Payload = &contacteventsv1.ContactEvent_EdgeGroupUpdated{
			EdgeGroupUpdated: &contacteventsv1.ContactEdgeGroupUpdatedV1{
				TenantId:      payload.TenantID,
				OwnerUserId:   payload.OwnerUserID,
				ContactUserId: payload.ContactUserID,
				Status:        payload.Status,
				EdgeVersion:   payload.EdgeVersion,
				GroupName:     payload.GroupName,
				OccurredAt:    payload.Timestamp(),
			},
		}
		return event, nil
	case types.ContactEventPrivacyUpdated:
		payload, err := decodeContactPrivacyPayload(message.PayloadJSON)
		if err != nil {
			return nil, err
		}
		event.Payload = &contacteventsv1.ContactEvent_PrivacyUpdated{
			PrivacyUpdated: &contacteventsv1.ContactPrivacyUpdatedV1{
				TenantId:                   payload.TenantID,
				UserId:                     payload.UserID,
				AllowContactRequests:       payload.AllowContactRequests,
				AllowSearchContactRequests: payload.AllowSearchContactRequestsValue(),
				AllowProfileVisibility:     payload.AllowProfileVisibilityValue(),
				ProfileVisibilityFields:    payload.ProfileVisibilityFieldsValue(),
				PrivacyVersion:             payload.PrivacyVersion,
				OccurredAt:                 payload.Timestamp(),
			},
		}
		return event, nil
	case types.ContactEventPrivacyExceptionUpdated:
		payload, err := decodeContactPrivacyExceptionPayload(message.PayloadJSON)
		if err != nil {
			return nil, err
		}
		event.Payload = &contacteventsv1.ContactEvent_PrivacyExceptionUpdated{
			PrivacyExceptionUpdated: &contacteventsv1.ContactPrivacyExceptionUpdatedV1{
				TenantId:         payload.TenantID,
				OwnerUserId:      payload.OwnerUserID,
				OtherUserId:      payload.OtherUserID,
				Decision:         payload.Decision,
				ExceptionVersion: payload.ExceptionVersion,
				OccurredAt:       payload.Timestamp(),
			},
		}
		return event, nil
	case types.ContactEventPrivacyExceptionDeleted:
		payload, err := decodeContactPrivacyExceptionDeletedPayload(message.PayloadJSON)
		if err != nil {
			return nil, err
		}
		event.Payload = &contacteventsv1.ContactEvent_PrivacyExceptionDeleted{
			PrivacyExceptionDeleted: &contacteventsv1.ContactPrivacyExceptionDeletedV1{
				TenantId:                 payload.TenantID,
				OwnerUserId:              payload.OwnerUserID,
				OtherUserId:              payload.OtherUserID,
				PreviousExceptionVersion: payload.PreviousExceptionVersion,
				OccurredAt:               payload.Timestamp(),
			},
		}
		return event, nil
	default:
		return nil, errors.New("unsupported contacts outbox event type")
	}
}

type contactPayload struct {
	TenantID                   string   `json:"tenant_id"`
	RequestID                  string   `json:"request_id"`
	SenderUserID               string   `json:"sender_user_id"`
	ReceiverUserID             string   `json:"receiver_user_id"`
	Status                     string   `json:"status"`
	Message                    string   `json:"message"`
	SourceType                 string   `json:"source_type"`
	SourceRef                  string   `json:"source_ref"`
	RiskLevel                  string   `json:"risk_level"`
	ReviewRequired             bool     `json:"review_required"`
	EdgeVersion                int64    `json:"edge_version"`
	OccurredAt                 string   `json:"occurred_at"`
	OwnerUserID                string   `json:"owner_user_id"`
	ContactUserID              string   `json:"contact_user_id"`
	PreviousStatus             string   `json:"previous_status"`
	Reason                     string   `json:"reason"`
	Remark                     string   `json:"remark"`
	GroupName                  string   `json:"group_name"`
	UserID                     string   `json:"user_id"`
	PrivacyVersion             int64    `json:"privacy_version"`
	AllowContactRequests       bool     `json:"allow_contact_requests"`
	AllowSearchContactRequests *bool    `json:"allow_search_contact_requests"`
	AllowProfileVisibility     *bool    `json:"allow_profile_visibility"`
	ProfileVisibilityFields    []string `json:"profile_visibility_fields"`
	OtherUserID                string   `json:"other_user_id"`
	Decision                   string   `json:"decision"`
	ExceptionVersion           int64    `json:"exception_version"`
	PreviousExceptionVersion   int64    `json:"previous_exception_version"`
}

func (payload contactPayload) Timestamp() *timestamppb.Timestamp {
	occurredAt, _ := time.Parse(time.RFC3339Nano, payload.OccurredAt)
	return timestamppb.New(occurredAt)
}

func (payload contactPayload) RiskLevelValue() string {
	if payload.RiskLevel == "" {
		return "LOW"
	}
	return payload.RiskLevel
}

func (payload contactPayload) AllowSearchContactRequestsValue() bool {
	if payload.AllowSearchContactRequests == nil {
		return true
	}
	return *payload.AllowSearchContactRequests
}

func (payload contactPayload) AllowProfileVisibilityValue() bool {
	if payload.AllowProfileVisibility == nil {
		return true
	}
	return *payload.AllowProfileVisibility
}

func (payload contactPayload) ProfileVisibilityFieldsValue() []string {
	if payload.ProfileVisibilityFields != nil {
		return append([]string(nil), payload.ProfileVisibilityFields...)
	}
	if !payload.AllowProfileVisibilityValue() {
		return nil
	}
	return []string{"DISPLAY_NAME", "AVATAR", "ORGANIZATION", "TITLE"}
}

func decodeContactRequestPayload(payloadJSON []byte) (contactPayload, error) {
	var payload contactPayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return contactPayload{}, err
	}
	if payload.TenantID == "" ||
		payload.RequestID == "" ||
		payload.SenderUserID == "" ||
		payload.ReceiverUserID == "" ||
		payload.Status == "" ||
		payload.OccurredAt == "" {
		return contactPayload{}, errors.New("contact request payload is incomplete")
	}
	if _, err := time.Parse(time.RFC3339Nano, payload.OccurredAt); err != nil {
		return contactPayload{}, errors.New("contact request payload occurred_at is invalid")
	}
	return payload, nil
}

func decodeContactResponsePayload(payloadJSON []byte, requireEdgeVersion bool) (contactPayload, error) {
	payload, err := decodeContactRequestPayload(payloadJSON)
	if err != nil {
		return contactPayload{}, err
	}
	if requireEdgeVersion && payload.EdgeVersion <= 0 {
		return contactPayload{}, errors.New("contact accepted payload is incomplete")
	}
	return payload, nil
}

func decodeContactEdgePayload(payloadJSON []byte, requirePreviousStatus bool) (contactPayload, error) {
	var payload contactPayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return contactPayload{}, err
	}
	if payload.TenantID == "" ||
		payload.OwnerUserID == "" ||
		payload.ContactUserID == "" ||
		payload.Status == "" ||
		payload.EdgeVersion <= 0 ||
		payload.OccurredAt == "" {
		return contactPayload{}, errors.New("contact edge payload is incomplete")
	}
	if requirePreviousStatus && payload.PreviousStatus == "" {
		return contactPayload{}, errors.New("contact edge payload previous_status is incomplete")
	}
	if _, err := time.Parse(time.RFC3339Nano, payload.OccurredAt); err != nil {
		return contactPayload{}, errors.New("contact edge payload occurred_at is invalid")
	}
	return payload, nil
}

func decodeContactPrivacyPayload(payloadJSON []byte) (contactPayload, error) {
	var payload contactPayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return contactPayload{}, err
	}
	if payload.TenantID == "" ||
		payload.UserID == "" ||
		payload.PrivacyVersion <= 0 ||
		payload.OccurredAt == "" {
		return contactPayload{}, errors.New("contact privacy payload is incomplete")
	}
	if _, err := time.Parse(time.RFC3339Nano, payload.OccurredAt); err != nil {
		return contactPayload{}, errors.New("contact privacy payload occurred_at is invalid")
	}
	return payload, nil
}

func decodeContactPrivacyExceptionPayload(payloadJSON []byte) (contactPayload, error) {
	var payload contactPayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return contactPayload{}, err
	}
	if payload.TenantID == "" ||
		payload.OwnerUserID == "" ||
		payload.OtherUserID == "" ||
		payload.Decision == "" ||
		payload.ExceptionVersion <= 0 ||
		payload.OccurredAt == "" {
		return contactPayload{}, errors.New("contact privacy exception payload is incomplete")
	}
	if payload.Decision != "ALLOW" && payload.Decision != "DENY" {
		return contactPayload{}, errors.New("contact privacy exception decision is invalid")
	}
	if _, err := time.Parse(time.RFC3339Nano, payload.OccurredAt); err != nil {
		return contactPayload{}, errors.New("contact privacy exception payload occurred_at is invalid")
	}
	return payload, nil
}

func decodeContactPrivacyExceptionDeletedPayload(payloadJSON []byte) (contactPayload, error) {
	var payload contactPayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return contactPayload{}, err
	}
	if payload.TenantID == "" ||
		payload.OwnerUserID == "" ||
		payload.OtherUserID == "" ||
		payload.PreviousExceptionVersion <= 0 ||
		payload.OccurredAt == "" {
		return contactPayload{}, errors.New("contact privacy exception deleted payload is incomplete")
	}
	if _, err := time.Parse(time.RFC3339Nano, payload.OccurredAt); err != nil {
		return contactPayload{}, errors.New("contact privacy exception deleted payload occurred_at is invalid")
	}
	return payload, nil
}

func normalizeConfig(config Config) Config {
	if config.Topic == "" {
		config.Topic = TopicContactEvents
	}
	if config.BatchSize <= 0 {
		config.BatchSize = 500
	}
	if config.PollInterval <= 0 {
		config.PollInterval = time.Second
	}
	if config.MaxAttempts <= 0 {
		config.MaxAttempts = 5
	}
	if config.RetryBaseDelay <= 0 {
		config.RetryBaseDelay = time.Second
	}
	if config.ErrorBackoff <= 0 {
		config.ErrorBackoff = time.Second
	}
	return config
}

func (relay *Relay) recordError() {
	relay.metrics.totalErrors.Add(1)
	relay.metrics.consecutiveErrors.Add(1)
	relay.metrics.lastErrorAtMS.Store(time.Now().UnixMilli())
}

func (relay *Relay) recordSuccess(stats types.OutboxRelayStats) {
	relay.metrics.consecutiveErrors.Store(0)
	now := time.Now().UnixMilli()
	relay.metrics.lastSuccessAtMS.Store(now)
	if stats.Published > 0 {
		relay.metrics.lastPublishedAtMS.Store(now)
	}
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
