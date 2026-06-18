package outbox

import (
	"context"
	"errors"
	"testing"
	"time"

	policyeventsv1 "github.com/qsyy0921/IM/schemas/kafka/policy/v1"
	"github.com/qsyy0921/IM/services/policy-service/internal/types"
	"google.golang.org/protobuf/proto"
)

func TestBuildPolicyEventMessageActionDecision(t *testing.T) {
	message := testPolicyOutboxMessage(t)
	event, err := BuildPolicyEvent(message)
	if err != nil {
		t.Fatalf("BuildPolicyEvent returned error: %v", err)
	}
	if event.GetEventId() != message.EventID ||
		event.GetEventType() != types.PolicyEventMessageActionDecision ||
		event.GetTenantId() != string(message.TenantID) ||
		event.GetAggregateType() != message.AggregateType ||
		event.GetAggregateId() != message.AggregateID ||
		event.GetAggregateVersion() != message.AggregateVersion ||
		event.GetPartitionKey() != message.PartitionKey ||
		event.GetProducer() != message.Producer {
		t.Fatalf("unexpected envelope: %+v", event)
	}
	payload := event.GetMessageActionDecision()
	if payload == nil {
		t.Fatal("message_action_decision payload is nil")
	}
	if payload.GetActorUserKey() != "actor-key" ||
		payload.GetDeviceKey() != "device-key" ||
		payload.GetConversationKey() != "conversation-key" ||
		payload.GetMessageKey() != "message-key" ||
		payload.GetAction() != "SEND" ||
		!payload.GetMessageIdPresent() ||
		!payload.GetDirectPeerContextPresent() ||
		payload.GetDirectPeerKey() != "peer-key" ||
		!payload.GetAllowed() ||
		payload.GetPermissionVersion() != 41 ||
		payload.GetClassification() != "POLICY_RPC_ALLOWED" ||
		payload.GetDecisionSource() != string(types.PolicyDecisionSourceExactRule) {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}

func TestBuildPolicyEventRejectsUnknownPayloadField(t *testing.T) {
	message := testPolicyOutboxMessage(t)
	message.PayloadJSON = []byte(`{
		"event_id":"policy-audit-event-1",
		"tenant_id":"tenant-policy",
		"actor_user_key":"actor-key",
		"conversation_key":"conversation-key",
		"action":"SEND",
		"allowed":true,
		"permission_version":41,
		"classification":"POLICY_RPC_ALLOWED",
		"decided_at":"2026-06-13T01:02:03Z",
		"raw_user_id":"alice"
	}`)
	if _, err := BuildPolicyEvent(message); err == nil {
		t.Fatal("expected unknown payload field to be rejected")
	}
}

func TestRelayRunOnceRetriesMalformedPayloadWithoutPublishing(t *testing.T) {
	message := testPolicyOutboxMessage(t)
	message.PayloadJSON = []byte(`{"event_id":"policy-audit-event-1"}`)
	store := &fakeStore{messages: []types.OutboxMessage{message}}
	publisher := &fakePublisher{}
	relay := NewRelay(store, publisher, Config{BatchSize: 1, MaxAttempts: 5})

	stats, err := relay.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}
	if stats.Fetched != 1 || stats.Retried != 1 || stats.Published != 0 || stats.DeadLettered != 0 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	if publisher.calls != 0 {
		t.Fatalf("publisher should not be called for malformed payload, got %d calls", publisher.calls)
	}
}

func TestRelayRunOnceDeadLettersAfterMaxAttempts(t *testing.T) {
	message := testPolicyOutboxMessage(t)
	message.RetryCount = 4
	message.PayloadJSON = []byte(`{"event_id":"policy-audit-event-1"}`)
	store := &fakeStore{messages: []types.OutboxMessage{message}}
	relay := NewRelay(store, &fakePublisher{}, Config{BatchSize: 1, MaxAttempts: 5})

	stats, err := relay.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}
	if stats.DeadLettered != 1 || stats.Retried != 0 || stats.Published != 0 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
}

func TestRelayRunOncePublishesPolicyEvent(t *testing.T) {
	store := &fakeStore{messages: []types.OutboxMessage{testPolicyOutboxMessage(t)}}
	publisher := &fakePublisher{}
	relay := NewRelay(store, publisher, Config{Topic: "im.policy.events.test"})

	stats, err := relay.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}
	if stats.Published != 1 || publisher.calls != 1 || publisher.topic != "im.policy.events.test" {
		t.Fatalf("unexpected publish stats/calls: stats=%+v calls=%d topic=%s", stats, publisher.calls, publisher.topic)
	}
	var event policyeventsv1.PolicyEvent
	if err := proto.Unmarshal(publisher.records[0].Value, &event); err != nil {
		t.Fatalf("unmarshal policy event: %v", err)
	}
	if event.GetMessageActionDecision() == nil {
		t.Fatal("published event missing message_action_decision payload")
	}
}

func TestRelayRetriesTransientRunOnceErrorAndExposesSnapshot(t *testing.T) {
	store := &fakeStore{
		errs: []error{errors.New("temporary store error"), nil},
		messages: []types.OutboxMessage{
			testPolicyOutboxMessage(t),
		},
	}
	publisher := &fakePublisher{}
	relay := NewRelay(store, publisher, Config{
		Topic:        "im.policy.events.test",
		PollInterval: time.Millisecond,
		ErrorBackoff: time.Millisecond,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		for publisher.calls == 0 {
			time.Sleep(time.Millisecond)
		}
		cancel()
	}()

	err := relay.Run(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled after successful retry, got %v", err)
	}
	if store.calls < 2 {
		t.Fatalf("expected retry after transient error, calls=%d", store.calls)
	}
	snapshot := relay.Snapshot()
	if snapshot.TotalErrors != 1 || snapshot.ConsecutiveErrors != 0 {
		t.Fatalf("unexpected relay snapshot: %+v", snapshot)
	}
	if snapshot.LastErrorAtMS == 0 || snapshot.LastSuccessAtMS == 0 || snapshot.LastPublishedAtMS == 0 {
		t.Fatalf("expected timestamps in relay snapshot: %+v", snapshot)
	}
	if snapshot.LastErrorBackoffMS != time.Millisecond.Milliseconds() {
		t.Fatalf("expected backoff %d, got %+v", time.Millisecond.Milliseconds(), snapshot)
	}
}

type fakeStore struct {
	messages []types.OutboxMessage
	errs     []error
	calls    int
}

func (store *fakeStore) ProcessReadyBatch(
	ctx context.Context,
	limit int,
	maxAttempts int,
	retryBaseDelay time.Duration,
	publish func(context.Context, []types.OutboxMessage) []error,
) (types.OutboxRelayStats, error) {
	if err := ctx.Err(); err != nil {
		return types.OutboxRelayStats{}, err
	}
	if store.calls < len(store.errs) {
		err := store.errs[store.calls]
		store.calls++
		if err != nil {
			return types.OutboxRelayStats{}, err
		}
	} else {
		store.calls++
	}
	errs := publish(ctx, store.messages)
	if len(errs) != len(store.messages) {
		return types.OutboxRelayStats{}, errors.New("publish result count mismatch")
	}
	stats := types.OutboxRelayStats{Fetched: len(store.messages)}
	for index, err := range errs {
		if err == nil {
			stats.Published++
			continue
		}
		if store.messages[index].RetryCount+1 >= maxAttempts {
			stats.DeadLettered++
			continue
		}
		stats.Retried++
	}
	return stats, nil
}

type fakePublisher struct {
	calls   int
	topic   string
	records []types.KafkaPublishRecord
	err     error
}

func (publisher *fakePublisher) PublishBatch(ctx context.Context, topic string, records []types.KafkaPublishRecord) error {
	publisher.calls++
	publisher.topic = topic
	publisher.records = records
	return publisher.err
}

func testPolicyOutboxMessage(t *testing.T) types.OutboxMessage {
	t.Helper()
	occurredAt := time.Date(2026, 6, 13, 1, 2, 3, 0, time.UTC)
	return types.OutboxMessage{
		ID:               1,
		EventID:          "policy-audit-event-1",
		TenantID:         "tenant-policy",
		AggregateType:    "policy_decision",
		AggregateID:      "tenant-policy:conversation-key",
		AggregateVersion: 1,
		EventType:        types.PolicyEventMessageActionDecision,
		EventVersion:     "v1",
		PartitionKey:     "tenant-policy:conversation-key",
		MappingVersion:   1,
		CorrelationID:    "request-policy",
		CausationID:      "request-policy",
		Producer:         "policy-service",
		TraceID:          "trace-policy",
		RetryCount:       0,
		OccurredAt:       occurredAt,
		PayloadJSON: []byte(`{
			"event_id":"policy-audit-event-1",
			"tenant_id":"tenant-policy",
			"actor_user_key":"actor-key",
			"device_key":"device-key",
			"conversation_key":"conversation-key",
			"message_key":"message-key",
			"action":"SEND",
			"message_id_present":true,
			"direct_peer_context_present":true,
			"direct_peer_key":"peer-key",
			"allowed":true,
			"permission_version":41,
			"classification":"POLICY_RPC_ALLOWED",
			"reason_code":"",
			"decision_source":"EXACT_RULE",
			"trace_id":"trace-policy",
			"request_id":"request-policy",
			"decided_at":"2026-06-13T01:02:03Z"
		}`),
	}
}
