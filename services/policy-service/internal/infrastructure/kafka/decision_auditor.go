package kafka

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	policyeventsv1 "github.com/qsyy0921/IM/schemas/kafka/policy/v1"
	"github.com/qsyy0921/IM/services/policy-service/internal/types"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	defaultDecisionAuditTopic = "im.policy.events"
)

type DecisionAuditPublisher interface {
	PublishBatch(ctx context.Context, topic string, records []types.KafkaPublishRecord) error
}

type DecisionAuditStageObserver interface {
	RecordPolicyDecisionStage(action types.MessageAction, stage string, failed bool, latencyMS int64)
}

type DecisionAuditKafka struct {
	publisher     DecisionAuditPublisher
	topic         string
	eventID       func() (string, error)
	now           func() time.Time
	stageObserver DecisionAuditStageObserver
}

type DecisionAuditKafkaOption func(*DecisionAuditKafka)

func NewDecisionAuditKafka(publisher DecisionAuditPublisher, opts ...DecisionAuditKafkaOption) *DecisionAuditKafka {
	auditor := &DecisionAuditKafka{
		publisher: publisher,
		topic:     defaultDecisionAuditTopic,
		eventID:   newPolicyAuditEventID,
		now:       func() time.Time { return time.Now().UTC() },
	}
	for _, opt := range opts {
		opt(auditor)
	}
	return auditor
}

func WithDecisionAuditKafkaTopic(topic string) DecisionAuditKafkaOption {
	return func(auditor *DecisionAuditKafka) {
		if trimmed := strings.TrimSpace(topic); trimmed != "" {
			auditor.topic = trimmed
		}
	}
}

func WithDecisionAuditKafkaEventID(fn func() (string, error)) DecisionAuditKafkaOption {
	return func(auditor *DecisionAuditKafka) {
		if fn != nil {
			auditor.eventID = fn
		}
	}
}

func WithDecisionAuditKafkaClock(clock func() time.Time) DecisionAuditKafkaOption {
	return func(auditor *DecisionAuditKafka) {
		if clock != nil {
			auditor.now = clock
		}
	}
}

func WithDecisionAuditKafkaStageObserver(observer DecisionAuditStageObserver) DecisionAuditKafkaOption {
	return func(auditor *DecisionAuditKafka) {
		auditor.stageObserver = observer
	}
}

func (auditor *DecisionAuditKafka) PolicyDecisionAuditStageName() string {
	return "decision_audit_kafka"
}

func (auditor *DecisionAuditKafka) RecordPolicyDecision(
	ctx context.Context,
	command types.CheckMessageActionCommand,
	decision types.MessageActionDecision,
) error {
	if auditor == nil || auditor.publisher == nil {
		return types.NewDependencyUnavailable("policy decision audit kafka publisher is not configured")
	}
	buildStarted := time.Now()
	event, partitionKey, err := auditor.buildPolicyEvent(command, decision)
	auditor.recordStage(command.Action, "decision_audit_kafka_build", err != nil, buildStarted)
	if err != nil {
		return err
	}
	marshalStarted := time.Now()
	value, err := proto.Marshal(event)
	auditor.recordStage(command.Action, "decision_audit_kafka_marshal", err != nil, marshalStarted)
	if err != nil {
		return types.NewDependencyUnavailable("policy decision audit kafka marshal failed")
	}

	publishStarted := time.Now()
	err = auditor.publisher.PublishBatch(ctx, auditor.topic, []types.KafkaPublishRecord{{
		Key:   []byte(partitionKey),
		Value: value,
	}})
	auditor.recordStage(command.Action, "decision_audit_kafka_publish", err != nil, publishStarted)
	if err != nil {
		return types.NewDependencyUnavailable("policy decision audit kafka publish failed")
	}
	return nil
}

func (auditor *DecisionAuditKafka) buildPolicyEvent(
	command types.CheckMessageActionCommand,
	decision types.MessageActionDecision,
) (*policyeventsv1.PolicyEvent, string, error) {
	if decision.PermissionVersion <= 0 || strings.TrimSpace(decision.Classification) == "" {
		return nil, "", types.NewDependencyUnavailable("policy decision audit payload is invalid")
	}
	eventID, err := auditor.eventID()
	if err != nil {
		return nil, "", types.NewDependencyUnavailable("policy decision audit event id failed")
	}
	decidedAt := auditor.now()
	classification := truncateAuditField(strings.TrimSpace(decision.Classification), 128)
	reasonCode := policyDecisionReasonCode(decision)
	decisionSource := policyDecisionSource(decision)
	traceID := truncateAuditField(strings.TrimSpace(command.AuthContext.TraceID), 128)
	requestID := truncateAuditField(strings.TrimSpace(command.AuthContext.RequestID), 128)
	messageIDPresent := decision.MessageID != ""
	directPeerContextPresent := command.DirectPeerUserID != ""
	actorUserKey := policyAuditStableKey(string(decision.TenantID), "user", string(decision.UserID))
	deviceKey := policyAuditStableKey(string(decision.TenantID), "device", string(command.AuthContext.DeviceID))
	conversationKey := policyAuditStableKey(string(decision.TenantID), "conversation", string(decision.ConversationID))
	messageKey := ""
	if messageIDPresent {
		messageKey = policyAuditStableKey(string(decision.TenantID), "message", string(decision.MessageID))
	}
	directPeerKey := ""
	if directPeerContextPresent {
		directPeerKey = policyAuditStableKey(string(decision.TenantID), "user", string(command.DirectPeerUserID))
	}
	partitionKey := policyDecisionPartitionKey(decision.TenantID, conversationKey)
	event := &policyeventsv1.PolicyEvent{
		EventId:          eventID,
		EventType:        types.PolicyEventMessageActionDecision,
		EventVersion:     "v1",
		TenantId:         string(decision.TenantID),
		AggregateType:    "policy_decision",
		AggregateId:      partitionKey,
		AggregateVersion: 1,
		PartitionKey:     partitionKey,
		MappingVersion:   1,
		TraceId:          traceID,
		CorrelationId:    requestID,
		CausationId:      requestID,
		Producer:         "policy-service",
		OccurredAt:       timestamppb.New(decidedAt),
		Payload: &policyeventsv1.PolicyEvent_MessageActionDecision{
			MessageActionDecision: &policyeventsv1.PolicyMessageActionDecisionV1{
				TenantId:                 string(decision.TenantID),
				ActorUserKey:             actorUserKey,
				DeviceKey:                deviceKey,
				ConversationKey:          conversationKey,
				MessageKey:               messageKey,
				Action:                   string(decision.Action),
				MessageIdPresent:         messageIDPresent,
				DirectPeerContextPresent: directPeerContextPresent,
				DirectPeerKey:            directPeerKey,
				Allowed:                  decision.Allowed,
				PermissionVersion:        decision.PermissionVersion,
				Classification:           classification,
				ReasonCode:               reasonCode,
				DecidedAt:                timestamppb.New(decidedAt),
				DecisionSource:           decisionSource,
			},
		},
	}
	return event, partitionKey, nil
}

func (auditor *DecisionAuditKafka) recordStage(action types.MessageAction, stage string, failed bool, started time.Time) {
	if auditor == nil || auditor.stageObserver == nil {
		return
	}
	auditor.stageObserver.RecordPolicyDecisionStage(action, stage, failed, time.Since(started).Milliseconds())
}

func policyDecisionPartitionKey(tenantID types.TenantID, conversationKey string) string {
	return fmt.Sprintf("%s:%s", tenantID, conversationKey)
}

func truncateAuditField(value string, max int) string {
	if max <= 0 || len(value) <= max {
		return value
	}
	return value[:max]
}

func newPolicyAuditEventID() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		bytes[0:4],
		bytes[4:6],
		bytes[6:8],
		bytes[8:10],
		bytes[10:16],
	), nil
}

func policyDecisionReasonCode(decision types.MessageActionDecision) string {
	if decision.Allowed {
		return ""
	}
	classification := strings.TrimSpace(decision.Classification)
	if classification != "" {
		return truncateAuditField(classification, 128)
	}
	return "POLICY_DENIED"
}

func policyDecisionSource(decision types.MessageActionDecision) string {
	source := strings.TrimSpace(string(decision.DecisionSource))
	if source == "" {
		return "UNSPECIFIED"
	}
	return truncateAuditField(source, 128)
}

func policyAuditStableKey(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:16])
}
