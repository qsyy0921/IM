package outbox

import (
	"context"
	"errors"
	"testing"
	"time"

	agenteventsv1 "github.com/qsyy0921/IM/schemas/kafka/agent/v1"
	"github.com/qsyy0921/IM/services/agent-service/internal/types"
	"google.golang.org/protobuf/proto"
)

func TestBuildKafkaValueForProposalApproved(t *testing.T) {
	message := testOutboxMessage(types.AgentEventProposalApproved, []byte(`{
		"schema_version": 1,
		"event_type": "agent.proposal.approved.v1",
		"tenant_id": "tenant-1",
		"proposal_id": "ap-1",
		"approval_id": "approval-1",
		"prepared_audit_id": "mcp-audit-1",
		"skill_id": "conversation.note.create",
		"tool_name": "conversation.note.create",
		"resource_type": "conversation",
		"resource_id": "conv-1",
		"risk_level": "LOW",
		"approved_by_user_id": "approver-1",
		"approved_at_unix_ms": 1710000000000
	}`))
	value, err := BuildKafkaValue(message)
	if err != nil {
		t.Fatalf("BuildKafkaValue() error = %v", err)
	}
	var event agenteventsv1.AgentEvent
	if err := proto.Unmarshal(value, &event); err != nil {
		t.Fatalf("unmarshal event: %v", err)
	}
	payload := event.GetProposalApproved()
	if payload == nil {
		t.Fatal("expected proposal_approved payload")
	}
	if event.EventId != "agent-approval-1" ||
		event.EventType != types.AgentEventProposalApproved ||
		event.PartitionKey != "ap-1" ||
		payload.ApprovalId != "approval-1" ||
		payload.PreparedAuditId != "mcp-audit-1" ||
		payload.ToolName != "conversation.note.create" {
		t.Fatalf("unexpected event: %+v payload=%+v", &event, payload)
	}
}

func TestRelayRunOnceFailClosedForMalformedPayload(t *testing.T) {
	store := &recordingStore{
		messages: []types.OutboxMessage{
			testOutboxMessage(types.AgentEventProposalApproved, []byte(`{"tenant_id":"tenant-1"}`)),
		},
	}
	publisher := &recordingPublisher{}
	relay := NewRelay(store, publisher, Config{MaxAttempts: 2})
	stats, err := relay.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if stats.Fetched != 1 || stats.Published != 0 || stats.Retried != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	if publisher.Calls() != 0 {
		t.Fatalf("malformed payload should not publish")
	}
}

func TestRelayRunOnceFailClosedForUnsupportedEvent(t *testing.T) {
	store := &recordingStore{
		messages: []types.OutboxMessage{
			testOutboxMessage("agent.unsupported.v1", []byte(`{}`)),
		},
	}
	publisher := &recordingPublisher{}
	relay := NewRelay(store, publisher, Config{MaxAttempts: 1})
	stats, err := relay.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if stats.Fetched != 1 || stats.Published != 0 || stats.DeadLettered != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	if publisher.Calls() != 0 {
		t.Fatalf("unsupported event should not publish")
	}
}

func TestRelayRunOnceMapsPublishErrorToRetry(t *testing.T) {
	store := &recordingStore{
		messages: []types.OutboxMessage{
			testOutboxMessage(types.AgentEventProposalApproved, []byte(`{
				"schema_version": 1,
				"event_type": "agent.proposal.approved.v1",
				"tenant_id": "tenant-1",
				"proposal_id": "ap-1",
				"approval_id": "approval-1",
				"prepared_audit_id": "mcp-audit-1",
				"skill_id": "conversation.note.create",
				"tool_name": "conversation.note.create",
				"resource_type": "conversation",
				"approved_by_user_id": "approver-1",
				"approved_at_unix_ms": 1710000000000
			}`)),
		},
	}
	publisher := &recordingPublisher{err: errors.New("kafka unavailable")}
	relay := NewRelay(store, publisher, Config{MaxAttempts: 2})
	stats, err := relay.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if stats.Fetched != 1 || stats.Published != 0 || stats.Retried != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	if publisher.Calls() != 1 {
		t.Fatalf("expected one publish call, got %d", publisher.Calls())
	}
}

func testOutboxMessage(eventType string, payload []byte) types.OutboxMessage {
	return types.OutboxMessage{
		ID:              7,
		EventID:         "agent-approval-1",
		TenantID:        "tenant-1",
		ProposalID:      "ap-1",
		ApprovalID:      "approval-1",
		PreparedAuditID: "mcp-audit-1",
		SkillID:         "conversation.note.create",
		ToolName:        "conversation.note.create",
		ResourceType:    "conversation",
		ResourceID:      "conv-1",
		RiskLevel:       "LOW",
		EventType:       eventType,
		EventVersion:    "v1",
		PartitionKey:    "ap-1",
		MappingVersion:  1,
		Producer:        "agent-service",
		PayloadJSON:     payload,
		OccurredAt:      time.Unix(1710000000, 0).UTC(),
	}
}

type recordingStore struct {
	messages []types.OutboxMessage
}

func (store *recordingStore) ProcessReadyBatch(
	ctx context.Context,
	limit int,
	maxAttempts int,
	retryBaseDelay time.Duration,
	publish func(context.Context, []types.OutboxMessage) []error,
) (types.OutboxRelayStats, error) {
	_ = limit
	_ = maxAttempts
	_ = retryBaseDelay
	stats := types.OutboxRelayStats{Fetched: len(store.messages)}
	results := publish(ctx, store.messages)
	for index, err := range results {
		if err != nil {
			if store.messages[index].RetryCount+1 >= maxAttempts {
				stats.DeadLettered++
				continue
			}
			stats.Retried++
		} else {
			stats.Published++
		}
	}
	return stats, nil
}

type recordingPublisher struct {
	err   error
	calls int
}

func (publisher *recordingPublisher) PublishBatch(context.Context, string, []types.KafkaPublishRecord) error {
	publisher.calls++
	return publisher.err
}

func (publisher *recordingPublisher) Calls() int {
	return publisher.calls
}
