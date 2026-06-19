package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"time"

	agenteventsv1 "github.com/qsyy0921/IM/schemas/kafka/agent/v1"
	"github.com/qsyy0921/IM/services/agent-service/internal/types"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const TopicAgentEvents = "im.agent.events"

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
	return &Relay{store: store, publisher: publisher, config: normalizeConfig(config)}
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
				relay.config.Logf("agent-service approval outbox relay retrying after runtime error: %v", err)
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
		return types.OutboxRelayStats{}, errors.New("agent approval outbox relay store is not configured")
	}
	if relay.publisher == nil {
		return types.OutboxRelayStats{}, errors.New("agent approval outbox relay publisher is not configured")
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
		records = append(records, types.KafkaPublishRecord{Key: []byte(message.PartitionKey), Value: value})
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
	event, err := BuildAgentEvent(message)
	if err != nil {
		return nil, err
	}
	return proto.Marshal(event)
}

func BuildAgentEvent(message types.OutboxMessage) (*agenteventsv1.AgentEvent, error) {
	if message.EventID == "" ||
		message.EventType == "" ||
		message.EventVersion == "" ||
		message.TenantID == "" ||
		message.ProposalID == "" ||
		message.ApprovalID == "" ||
		message.PreparedAuditID == "" ||
		message.SkillID == "" ||
		message.ToolName == "" ||
		message.ResourceType == "" ||
		message.PartitionKey == "" ||
		message.MappingVersion <= 0 ||
		message.Producer == "" {
		return nil, errors.New("agent approval outbox envelope is incomplete")
	}
	event := &agenteventsv1.AgentEvent{
		EventId:          message.EventID,
		EventType:        message.EventType,
		EventVersion:     message.EventVersion,
		TenantId:         string(message.TenantID),
		AggregateType:    "agent_proposal",
		AggregateId:      message.ProposalID,
		AggregateVersion: message.ID,
		PartitionKey:     message.PartitionKey,
		MappingVersion:   message.MappingVersion,
		TraceId:          message.TraceID,
		CorrelationId:    message.CorrelationID,
		CausationId:      message.CausationID,
		Producer:         message.Producer,
		OccurredAt:       timestamppb.New(message.OccurredAt),
	}
	switch message.EventType {
	case types.AgentEventProposalApproved:
		payload, err := decodeProposalApprovedPayload(message.PayloadJSON)
		if err != nil {
			return nil, err
		}
		event.Payload = &agenteventsv1.AgentEvent_ProposalApproved{
			ProposalApproved: &agenteventsv1.AgentProposalApprovedV1{
				TenantId:         payload.TenantID,
				ProposalId:       payload.ProposalID,
				ApprovalId:       payload.ApprovalID,
				PreparedAuditId:  payload.PreparedAuditID,
				SkillId:          payload.SkillID,
				ToolName:         payload.ToolName,
				ResourceType:     payload.ResourceType,
				ResourceId:       payload.ResourceID,
				RiskLevel:        payload.RiskLevel,
				ApprovedByUserId: payload.ApprovedByUserID,
				ApprovedAt:       timestamppb.New(time.UnixMilli(payload.ApprovedAtUnixMS).UTC()),
			},
		}
		return event, nil
	default:
		return nil, errors.New("unsupported agent approval outbox event type")
	}
}

type proposalApprovedPayload struct {
	SchemaVersion    int    `json:"schema_version"`
	EventType        string `json:"event_type"`
	TenantID         string `json:"tenant_id"`
	ProposalID       string `json:"proposal_id"`
	ApprovalID       string `json:"approval_id"`
	PreparedAuditID  string `json:"prepared_audit_id"`
	SkillID          string `json:"skill_id"`
	ToolName         string `json:"tool_name"`
	ResourceType     string `json:"resource_type"`
	ResourceID       string `json:"resource_id"`
	RiskLevel        string `json:"risk_level"`
	ApprovedByUserID string `json:"approved_by_user_id"`
	ApprovedAtUnixMS int64  `json:"approved_at_unix_ms"`
}

func decodeProposalApprovedPayload(payloadJSON []byte) (proposalApprovedPayload, error) {
	var payload proposalApprovedPayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return proposalApprovedPayload{}, err
	}
	if payload.SchemaVersion <= 0 ||
		payload.EventType != types.AgentEventProposalApproved ||
		payload.TenantID == "" ||
		payload.ProposalID == "" ||
		payload.ApprovalID == "" ||
		payload.PreparedAuditID == "" ||
		payload.SkillID == "" ||
		payload.ToolName == "" ||
		payload.ResourceType == "" ||
		payload.ApprovedByUserID == "" ||
		payload.ApprovedAtUnixMS <= 0 {
		return proposalApprovedPayload{}, errors.New("agent proposal approved payload is incomplete")
	}
	return payload, nil
}

func normalizeConfig(config Config) Config {
	if config.Topic == "" {
		config.Topic = TopicAgentEvents
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
