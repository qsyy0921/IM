package kafka

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/policy-service/internal/types"
)

func TestDecisionAuditKafkaAsyncReturnsAfterEnqueueBeforeKafkaAck(t *testing.T) {
	publisher := newBlockingAsyncDecisionAuditPublisher()
	observer := &recordingDecisionAuditStageObserver{}
	auditor := NewDecisionAuditKafkaAsync(
		publisher,
		WithDecisionAuditKafkaAsyncTopic("im.policy.events.test"),
		WithDecisionAuditKafkaAsyncQueueSize(8),
		WithDecisionAuditKafkaAsyncBatchSize(1),
		WithDecisionAuditKafkaAsyncFlushInterval(time.Hour),
		WithDecisionAuditKafkaAsyncCloseTimeout(time.Second),
		WithDecisionAuditKafkaAsyncEventID(func() (string, error) {
			return "policy-audit-async-event-1", nil
		}),
		WithDecisionAuditKafkaAsyncStageObserver(observer),
	)

	command := testDecisionAuditCommand(types.MessageActionSend)
	decision := testAsyncDecision(command)
	if err := auditor.RecordPolicyDecision(context.Background(), command, decision); err != nil {
		t.Fatalf("record policy decision: %v", err)
	}
	if auditor.PolicyDecisionAuditStageName() != "decision_audit_kafka_async" {
		t.Fatalf("unexpected stage name")
	}
	publisher.waitCalled(t)
	if got := publisher.callCount("im.policy.events.test"); got != 1 {
		t.Fatalf("expected one in-flight kafka publish, got %d", got)
	}
	publisher.release()
	if err := auditor.Close(); err != nil {
		t.Fatalf("close auditor: %v", err)
	}
	observer.assertContainsStage(t, "decision_audit_kafka_async_enqueue", false)
	observer.assertContainsStage(t, "decision_audit_kafka_async_publish", false)
}

func TestDecisionAuditKafkaAsyncQueueFullFailsClosed(t *testing.T) {
	publisher := newBlockingAsyncDecisionAuditPublisher()
	auditor := NewDecisionAuditKafkaAsync(
		publisher,
		WithDecisionAuditKafkaAsyncQueueSize(1),
		WithDecisionAuditKafkaAsyncBatchSize(1),
		WithDecisionAuditKafkaAsyncFlushInterval(time.Hour),
		WithDecisionAuditKafkaAsyncCloseTimeout(time.Second),
		WithDecisionAuditKafkaAsyncEventID(func() (string, error) {
			return "policy-audit-async-event-1", nil
		}),
	)
	command := testDecisionAuditCommand(types.MessageActionSend)
	decision := testAsyncDecision(command)

	if err := auditor.RecordPolicyDecision(context.Background(), command, decision); err != nil {
		t.Fatalf("record first decision: %v", err)
	}
	publisher.waitCalled(t)
	if err := auditor.RecordPolicyDecision(context.Background(), command, decision); err != nil {
		t.Fatalf("record second decision: %v", err)
	}
	err := auditor.RecordPolicyDecision(context.Background(), command, decision)
	if !errors.Is(err, types.ErrDependencyUnavailable) {
		t.Fatalf("expected dependency unavailable when queue is full, got %v", err)
	}
	publisher.release()
	if err := auditor.Close(); err != nil {
		t.Fatalf("close auditor: %v", err)
	}
}

func TestDecisionAuditKafkaAsyncPublishesDLQAfterRetryBudget(t *testing.T) {
	publisher := &topicFailingAsyncDecisionAuditPublisher{
		failTopic: "im.policy.events.test",
		err:       errors.New("kafka unavailable"),
	}
	observer := &recordingDecisionAuditStageObserver{}
	auditor := NewDecisionAuditKafkaAsync(
		publisher,
		WithDecisionAuditKafkaAsyncTopic("im.policy.events.test"),
		WithDecisionAuditKafkaAsyncDLQTopic("im.policy.events.test.dlq"),
		WithDecisionAuditKafkaAsyncQueueSize(8),
		WithDecisionAuditKafkaAsyncBatchSize(1),
		WithDecisionAuditKafkaAsyncFlushInterval(time.Millisecond),
		WithDecisionAuditKafkaAsyncRetry(2, time.Millisecond, time.Millisecond),
		WithDecisionAuditKafkaAsyncCloseTimeout(time.Second),
		WithDecisionAuditKafkaAsyncEventID(func() (string, error) {
			return "policy-audit-async-event-1", nil
		}),
		WithDecisionAuditKafkaAsyncStageObserver(observer),
	)
	command := testDecisionAuditCommand(types.MessageActionSend)
	decision := testAsyncDecision(command)

	if err := auditor.RecordPolicyDecision(context.Background(), command, decision); err != nil {
		t.Fatalf("record policy decision: %v", err)
	}
	if err := auditor.Close(); err != nil {
		t.Fatalf("close auditor: %v", err)
	}
	if got := publisher.callCount("im.policy.events.test"); got != 2 {
		t.Fatalf("main topic attempts = %d, want 2", got)
	}
	if got := publisher.callCount("im.policy.events.test.dlq"); got != 1 {
		t.Fatalf("dlq topic attempts = %d, want 1", got)
	}
	observer.assertContainsStage(t, "decision_audit_kafka_async_publish", true)
	observer.assertContainsStage(t, "decision_audit_kafka_async_retry", true)
	observer.assertContainsStage(t, "decision_audit_kafka_async_dlq_publish", false)
}

type blockingAsyncDecisionAuditPublisher struct {
	mu          sync.Mutex
	calls       []asyncDecisionAuditPublishCall
	called      chan struct{}
	calledOnce  sync.Once
	releaseChan chan struct{}
}

type topicFailingAsyncDecisionAuditPublisher struct {
	mu        sync.Mutex
	calls     []asyncDecisionAuditPublishCall
	failTopic string
	err       error
}

type asyncDecisionAuditPublishCall struct {
	topic   string
	records []types.KafkaPublishRecord
}

func newBlockingAsyncDecisionAuditPublisher() *blockingAsyncDecisionAuditPublisher {
	return &blockingAsyncDecisionAuditPublisher{
		called:      make(chan struct{}),
		releaseChan: make(chan struct{}),
	}
}

func (publisher *blockingAsyncDecisionAuditPublisher) PublishBatch(_ context.Context, topic string, records []types.KafkaPublishRecord) error {
	publisher.mu.Lock()
	publisher.calls = append(publisher.calls, asyncDecisionAuditPublishCall{
		topic:   topic,
		records: cloneKafkaRecords(records),
	})
	publisher.mu.Unlock()
	publisher.calledOnce.Do(func() {
		close(publisher.called)
	})
	<-publisher.releaseChan
	return nil
}

func (publisher *blockingAsyncDecisionAuditPublisher) waitCalled(t *testing.T) {
	t.Helper()
	select {
	case <-publisher.called:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for kafka publish")
	}
}

func (publisher *blockingAsyncDecisionAuditPublisher) release() {
	close(publisher.releaseChan)
}

func (publisher *blockingAsyncDecisionAuditPublisher) callCount(topic string) int {
	publisher.mu.Lock()
	defer publisher.mu.Unlock()
	count := 0
	for _, call := range publisher.calls {
		if call.topic == topic {
			count++
		}
	}
	return count
}

func (publisher *topicFailingAsyncDecisionAuditPublisher) PublishBatch(_ context.Context, topic string, records []types.KafkaPublishRecord) error {
	publisher.mu.Lock()
	publisher.calls = append(publisher.calls, asyncDecisionAuditPublishCall{
		topic:   topic,
		records: cloneKafkaRecords(records),
	})
	publisher.mu.Unlock()
	if topic == publisher.failTopic {
		return publisher.err
	}
	return nil
}

func (publisher *topicFailingAsyncDecisionAuditPublisher) callCount(topic string) int {
	publisher.mu.Lock()
	defer publisher.mu.Unlock()
	count := 0
	for _, call := range publisher.calls {
		if call.topic == topic {
			count++
		}
	}
	return count
}

func cloneKafkaRecords(records []types.KafkaPublishRecord) []types.KafkaPublishRecord {
	cloned := make([]types.KafkaPublishRecord, 0, len(records))
	for _, record := range records {
		cloned = append(cloned, types.KafkaPublishRecord{
			Key:   append([]byte(nil), record.Key...),
			Value: append([]byte(nil), record.Value...),
		})
	}
	return cloned
}

func testAsyncDecision(command types.CheckMessageActionCommand) types.MessageActionDecision {
	return types.MessageActionDecision{
		TenantID:          command.AuthContext.TenantID,
		UserID:            command.AuthContext.UserID,
		ConversationID:    command.ConversationID,
		Action:            command.Action,
		Allowed:           true,
		PermissionVersion: 7,
		Classification:    "POLICY_RPC_ALLOWED",
		DecisionSource:    types.PolicyDecisionSourceStaticDefault,
	}
}

func (observer *recordingDecisionAuditStageObserver) assertContainsStage(t *testing.T, stage string, failed bool) {
	t.Helper()
	observer.mu.Lock()
	defer observer.mu.Unlock()
	for index, actual := range observer.stages {
		if actual == stage && observer.failed[index] == failed {
			return
		}
	}
	t.Fatalf("expected stage=%s failed=%t in stages=%v failures=%v", stage, failed, observer.stages, observer.failed)
}
