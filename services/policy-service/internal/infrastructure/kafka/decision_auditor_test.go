package kafka

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	policyeventsv1 "github.com/qsyy0921/IM/schemas/kafka/policy/v1"
	"github.com/qsyy0921/IM/services/policy-service/internal/types"
	"google.golang.org/protobuf/proto"
)

func TestDecisionAuditKafkaPublishesPolicyEvent(t *testing.T) {
	publisher := &recordingDecisionAuditPublisher{}
	decidedAt := time.Date(2026, 7, 2, 12, 30, 0, 0, time.UTC)
	observer := &recordingDecisionAuditStageObserver{}
	auditor := NewDecisionAuditKafka(
		publisher,
		WithDecisionAuditKafkaTopic("im.policy.events.test"),
		WithDecisionAuditKafkaEventID(func() (string, error) {
			return "policy-audit-kafka-event-1", nil
		}),
		WithDecisionAuditKafkaClock(func() time.Time {
			return decidedAt
		}),
		WithDecisionAuditKafkaStageObserver(observer),
	)
	command := testDecisionAuditCommand(types.MessageActionSend)
	command.DirectPeerUserID = "peer-policy"
	command.AuthContext.TraceID = "trace-policy-audit"
	command.AuthContext.RequestID = "request-policy-audit"
	command.AuthContext.SessionID = "session-should-not-be-published"
	decision := types.MessageActionDecision{
		TenantID:          command.AuthContext.TenantID,
		UserID:            command.AuthContext.UserID,
		ConversationID:    command.ConversationID,
		Action:            command.Action,
		Allowed:           false,
		PermissionVersion: 12,
		Classification:    "CONTACT_BLOCKED",
		Reason:            "contact blocked",
		DecisionSource:    types.PolicyDecisionSourceContactProjection,
	}

	if err := auditor.RecordPolicyDecision(context.Background(), command, decision); err != nil {
		t.Fatalf("record policy decision: %v", err)
	}
	if auditor.PolicyDecisionAuditStageName() != "decision_audit_kafka" {
		t.Fatalf("unexpected stage name")
	}
	if publisher.topic != "im.policy.events.test" || len(publisher.records) != 1 {
		t.Fatalf("unexpected publish call: topic=%s records=%d", publisher.topic, len(publisher.records))
	}
	expectedConversationKey := policyAuditStableKey(string(command.AuthContext.TenantID), "conversation", string(command.ConversationID))
	expectedPartitionKey := "tenant-policy:" + expectedConversationKey
	if string(publisher.records[0].Key) != expectedPartitionKey {
		t.Fatalf("unexpected kafka key %q want %q", publisher.records[0].Key, expectedPartitionKey)
	}
	var event policyeventsv1.PolicyEvent
	if err := proto.Unmarshal(publisher.records[0].Value, &event); err != nil {
		t.Fatalf("decode policy event: %v", err)
	}
	payload := event.GetMessageActionDecision()
	if event.GetEventId() != "policy-audit-kafka-event-1" ||
		event.GetEventType() != types.PolicyEventMessageActionDecision ||
		event.GetTenantId() != string(command.AuthContext.TenantID) ||
		event.GetAggregateType() != "policy_decision" ||
		event.GetAggregateId() != expectedPartitionKey ||
		event.GetPartitionKey() != expectedPartitionKey ||
		event.GetCorrelationId() != "request-policy-audit" ||
		event.GetTraceId() != "trace-policy-audit" ||
		event.GetProducer() != "policy-service" ||
		payload == nil {
		t.Fatalf("unexpected policy event: %+v", &event)
	}
	expectedActorKey := policyAuditStableKey(string(command.AuthContext.TenantID), "user", string(command.AuthContext.UserID))
	expectedDeviceKey := policyAuditStableKey(string(command.AuthContext.TenantID), "device", string(command.AuthContext.DeviceID))
	expectedDirectPeerKey := policyAuditStableKey(string(command.AuthContext.TenantID), "user", string(command.DirectPeerUserID))
	if payload.GetActorUserKey() != expectedActorKey ||
		payload.GetDeviceKey() != expectedDeviceKey ||
		payload.GetConversationKey() != expectedConversationKey ||
		payload.GetMessageKey() != "" ||
		payload.GetAction() != string(types.MessageActionSend) ||
		payload.GetMessageIdPresent() ||
		!payload.GetDirectPeerContextPresent() ||
		payload.GetDirectPeerKey() != expectedDirectPeerKey ||
		payload.GetAllowed() ||
		payload.GetPermissionVersion() != 12 ||
		payload.GetClassification() != "CONTACT_BLOCKED" ||
		payload.GetReasonCode() != "CONTACT_BLOCKED" ||
		payload.GetDecisionSource() != string(types.PolicyDecisionSourceContactProjection) {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	for _, raw := range []string{"user-policy", "device-policy", "peer-policy", "conv-policy", "session-should-not-be-published", "contact blocked"} {
		if strings.Contains(event.String(), raw) || strings.Contains(string(publisher.records[0].Value), raw) {
			t.Fatalf("audit kafka event leaked raw value %q: %s", raw, event.String())
		}
	}
	observer.assertStages(t, []string{"decision_audit_kafka_build", "decision_audit_kafka_marshal", "decision_audit_kafka_publish"})
}

func TestDecisionAuditKafkaFailsClosedOnPublishError(t *testing.T) {
	auditor := NewDecisionAuditKafka(
		&recordingDecisionAuditPublisher{err: errors.New("kafka unavailable")},
		WithDecisionAuditKafkaEventID(func() (string, error) {
			return "policy-audit-kafka-event-1", nil
		}),
	)
	decision := types.MessageActionDecision{
		TenantID:          "tenant-policy",
		UserID:            "user-policy",
		ConversationID:    "conv-policy",
		Action:            types.MessageActionSend,
		Allowed:           true,
		PermissionVersion: 7,
		Classification:    "POLICY_RPC_ALLOWED",
	}

	err := auditor.RecordPolicyDecision(context.Background(), testDecisionAuditCommand(types.MessageActionSend), decision)
	if !errors.Is(err, types.ErrDependencyUnavailable) {
		t.Fatalf("expected dependency unavailable, got %v", err)
	}
}

func TestDecisionAuditKafkaRejectsInvalidDecision(t *testing.T) {
	auditor := NewDecisionAuditKafka(&recordingDecisionAuditPublisher{})
	err := auditor.RecordPolicyDecision(context.Background(), testDecisionAuditCommand(types.MessageActionSend), types.MessageActionDecision{})
	if !errors.Is(err, types.ErrDependencyUnavailable) {
		t.Fatalf("expected dependency unavailable, got %v", err)
	}
}

type recordingDecisionAuditPublisher struct {
	topic   string
	records []types.KafkaPublishRecord
	err     error
}

func (publisher *recordingDecisionAuditPublisher) PublishBatch(_ context.Context, topic string, records []types.KafkaPublishRecord) error {
	publisher.topic = topic
	publisher.records = append([]types.KafkaPublishRecord(nil), records...)
	return publisher.err
}

type recordingDecisionAuditStageObserver struct {
	mu     sync.Mutex
	stages []string
	failed []bool
}

func (observer *recordingDecisionAuditStageObserver) RecordPolicyDecisionStage(_ types.MessageAction, stage string, failed bool, latencyMS int64) {
	observer.mu.Lock()
	defer observer.mu.Unlock()
	observer.stages = append(observer.stages, stage)
	observer.failed = append(observer.failed, failed)
	if latencyMS < 0 {
		panic("negative latency")
	}
}

func (observer *recordingDecisionAuditStageObserver) assertStages(t *testing.T, expected []string) {
	t.Helper()
	observer.mu.Lock()
	defer observer.mu.Unlock()
	if len(observer.stages) != len(expected) {
		t.Fatalf("expected stages %v, got %v", expected, observer.stages)
	}
	for index, expectedStage := range expected {
		if observer.stages[index] != expectedStage || observer.failed[index] {
			t.Fatalf("unexpected stage at %d: stage=%s failed=%t", index, observer.stages[index], observer.failed[index])
		}
	}
}

func testDecisionAuditCommand(action types.MessageAction) types.CheckMessageActionCommand {
	command := types.CheckMessageActionCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-policy",
			UserID:   "user-policy",
			DeviceID: "device-policy",
		},
		ConversationID: "conv-policy",
		Action:         action,
	}
	if action != types.MessageActionSend {
		command.MessageID = "msg-policy"
	}
	return command
}
