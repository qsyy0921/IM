package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"time"

	admineventsv1 "github.com/qsyy0921/IM/schemas/kafka/admin/v1"
	"github.com/qsyy0921/IM/services/admin-service/internal/types"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const TopicAdminEvents = "im.admin.events"

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

type Snapshot struct {
	TotalErrors        uint64
	ConsecutiveErrors  uint64
	LastErrorAtMS      int64
	LastSuccessAtMS    int64
	LastPublishedAtMS  int64
	LastErrorBackoffMS int64
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
				relay.config.Logf("admin-service outbox relay retrying after runtime error: %v", err)
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

func (relay *Relay) Snapshot() Snapshot {
	return Snapshot{
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
		return types.OutboxRelayStats{}, errors.New("admin outbox relay store is not configured")
	}
	if relay.publisher == nil {
		return types.OutboxRelayStats{}, errors.New("admin outbox relay publisher is not configured")
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
	event, err := BuildAdminEvent(message)
	if err != nil {
		return nil, err
	}
	return proto.Marshal(event)
}

func BuildAdminEvent(message types.OutboxMessage) (*admineventsv1.AdminEvent, error) {
	if strings.TrimSpace(message.EventID) == "" ||
		strings.TrimSpace(message.EventType) == "" ||
		strings.TrimSpace(string(message.TenantID)) == "" ||
		strings.TrimSpace(message.OperationID) == "" ||
		message.EventVersion <= 0 ||
		strings.TrimSpace(message.PartitionKey) == "" ||
		strings.TrimSpace(message.Producer) == "" {
		return nil, errors.New("admin outbox envelope is incomplete")
	}
	payload, err := decodeAdminPayload(message.PayloadJSON)
	if err != nil {
		return nil, err
	}
	if err := validateAdminPayloadForEvent(message.EventType, payload); err != nil {
		return nil, err
	}
	traceID := firstNonEmpty(message.TraceID, payload.TraceID)
	correlationID := firstNonEmpty(message.CorrelationID, payload.CorrelationID, message.EventID)
	causationID := firstNonEmpty(message.CausationID, payload.CausationID, message.EventID)
	event := &admineventsv1.AdminEvent{
		EventId:          message.EventID,
		EventType:        message.EventType,
		EventVersion:     int32(message.EventVersion),
		TenantId:         string(message.TenantID),
		AggregateType:    "admin_operation",
		AggregateId:      message.OperationID,
		AggregateVersion: message.AggregateVersion,
		PartitionKey:     message.PartitionKey,
		TraceId:          traceID,
		CorrelationId:    correlationID,
		CausationId:      causationID,
		Producer:         message.Producer,
		OccurredAt:       timestamppb.New(message.OccurredAt),
	}
	switch message.EventType {
	case types.AdminEventOperationSubmitted:
		event.Payload = &admineventsv1.AdminEvent_OperationSubmitted{
			OperationSubmitted: adminOperationSubmitted(payload),
		}
	case types.AdminEventOperationApproved:
		event.Payload = &admineventsv1.AdminEvent_OperationApproved{
			OperationApproved: adminOperationApproved(payload),
		}
	case types.AdminEventOperationRejected:
		event.Payload = &admineventsv1.AdminEvent_OperationRejected{
			OperationRejected: adminOperationRejected(payload),
		}
	default:
		return nil, errors.New("unsupported admin outbox event type")
	}
	return event, nil
}

type adminPayload struct {
	TenantID             string `json:"tenant_id"`
	OperationID          string `json:"operation_id"`
	OperationType        string `json:"operation_type"`
	TargetRefHash        string `json:"target_ref_hash"`
	RiskLevel            string `json:"risk_level"`
	Status               string `json:"status"`
	RequestedByHash      string `json:"requested_by_hash"`
	ApprovedByHash       string `json:"approved_by_hash"`
	ApprovalID           string `json:"approval_id"`
	Decision             string `json:"decision"`
	PayloadSchemaVersion string `json:"payload_schema_version"`
	PayloadHash          string `json:"payload_hash"`
	ReasonRef            string `json:"reason_ref"`
	CorrelationID        string `json:"correlation_id"`
	CausationID          string `json:"causation_id"`
	TraceID              string `json:"trace_id"`
}

func decodeAdminPayload(payloadJSON []byte) (adminPayload, error) {
	if containsForbiddenPayloadField(payloadJSON) {
		return adminPayload{}, errors.New("admin payload contains internal field")
	}
	var payload adminPayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return adminPayload{}, err
	}
	return payload, nil
}

func validateAdminPayloadForEvent(eventType string, payload adminPayload) error {
	if payload.TenantID == "" ||
		payload.OperationID == "" ||
		payload.OperationType == "" ||
		payload.TargetRefHash == "" ||
		payload.RiskLevel == "" ||
		payload.Status == "" ||
		payload.RequestedByHash == "" ||
		payload.PayloadHash == "" {
		return errors.New("admin payload is incomplete")
	}
	switch eventType {
	case types.AdminEventOperationSubmitted:
		if payload.PayloadSchemaVersion == "" {
			return errors.New("admin operation submitted payload is incomplete")
		}
	case types.AdminEventOperationApproved, types.AdminEventOperationRejected:
		if payload.ApprovedByHash == "" || payload.ApprovalID == "" || payload.Decision == "" {
			return errors.New("admin operation approval payload is incomplete")
		}
	}
	return nil
}

func containsForbiddenPayloadField(payloadJSON []byte) bool {
	lowered := strings.ToLower(string(payloadJSON))
	for _, field := range []string{
		"payload_json",
		"operation_payload_json",
		"operator_ref",
		"approver_ref",
		"password",
		"token",
		"totp",
		"recovery_code",
		"secret",
		"private_key",
		"object_key",
		"dsn",
		"provider_body",
		"downstream_response",
		"raw_prompt",
		"message_body",
		"evidence_pack",
	} {
		if strings.Contains(lowered, field) {
			return true
		}
	}
	return false
}

func adminOperationSubmitted(payload adminPayload) *admineventsv1.AdminOperationSubmittedV1 {
	return &admineventsv1.AdminOperationSubmittedV1{
		TenantId:             payload.TenantID,
		OperationId:          payload.OperationID,
		OperationType:        payload.OperationType,
		TargetRefHash:        payload.TargetRefHash,
		RiskLevel:            payload.RiskLevel,
		Status:               payload.Status,
		RequestedByHash:      payload.RequestedByHash,
		PayloadSchemaVersion: payload.PayloadSchemaVersion,
		PayloadHash:          payload.PayloadHash,
		ReasonRef:            payload.ReasonRef,
	}
}

func adminOperationApproved(payload adminPayload) *admineventsv1.AdminOperationApprovedV1 {
	return &admineventsv1.AdminOperationApprovedV1{
		TenantId:        payload.TenantID,
		OperationId:     payload.OperationID,
		OperationType:   payload.OperationType,
		TargetRefHash:   payload.TargetRefHash,
		RiskLevel:       payload.RiskLevel,
		Status:          payload.Status,
		RequestedByHash: payload.RequestedByHash,
		ApprovedByHash:  payload.ApprovedByHash,
		ApprovalId:      payload.ApprovalID,
		Decision:        payload.Decision,
		PayloadHash:     payload.PayloadHash,
	}
}

func adminOperationRejected(payload adminPayload) *admineventsv1.AdminOperationRejectedV1 {
	return &admineventsv1.AdminOperationRejectedV1{
		TenantId:        payload.TenantID,
		OperationId:     payload.OperationID,
		OperationType:   payload.OperationType,
		TargetRefHash:   payload.TargetRefHash,
		RiskLevel:       payload.RiskLevel,
		Status:          payload.Status,
		RequestedByHash: payload.RequestedByHash,
		ApprovedByHash:  payload.ApprovedByHash,
		ApprovalId:      payload.ApprovalID,
		Decision:        payload.Decision,
		PayloadHash:     payload.PayloadHash,
	}
}

func normalizeConfig(config Config) Config {
	if config.Topic == "" {
		config.Topic = TopicAdminEvents
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
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
