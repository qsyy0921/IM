package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/workflow-service/internal/types"
)

func TestBuildKafkaValueBuildsWorkflowEnvelope(t *testing.T) {
	message := validOutboxMessage()
	value, err := BuildKafkaValue(message)
	if err != nil {
		t.Fatalf("build kafka value: %v", err)
	}

	var envelope workflowEventEnvelope
	if err := json.Unmarshal(value, &envelope); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	if envelope.SchemaVersion != "nexusim.workflow.event_envelope.v1" ||
		envelope.EventID != message.EventID ||
		envelope.EventType != message.EventType ||
		envelope.TenantID != string(message.TenantID) ||
		envelope.WorkflowID != message.WorkflowID ||
		envelope.PartitionKey != message.PartitionKey ||
		envelope.Producer != message.Producer {
		t.Fatalf("unexpected envelope: %+v", envelope)
	}
	if string(envelope.PayloadJSON) != string(message.PayloadJSON) {
		t.Fatalf("payload changed: %s", envelope.PayloadJSON)
	}
}

func TestBuildKafkaValueRejectsSensitivePayload(t *testing.T) {
	message := validOutboxMessage()
	message.PayloadJSON = []byte(`{"tenant_id":"tenant-workflow","workflow_id":"workflow-1","raw_payload":"secret"}`)
	if _, err := BuildKafkaValue(message); err == nil {
		t.Fatal("expected sensitive payload to be rejected")
	}
}

func TestRelayRunOncePublishesValidAndRetriesInvalid(t *testing.T) {
	store := &fakeStore{
		messages: []types.OutboxMessage{
			validOutboxMessage(),
			func() types.OutboxMessage {
				message := validOutboxMessage()
				message.EventID = "evt-bad"
				message.PayloadJSON = []byte(`{"tenant_id":"tenant-workflow","workflow_id":"workflow-1","provider_body":"raw"}`)
				return message
			}(),
		},
	}
	publisher := &fakePublisher{}
	relay := NewRelay(store, publisher, Config{Topic: TopicWorkflowEvents})

	stats, err := relay.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("run relay once: %v", err)
	}
	if stats.Fetched != 2 || stats.Published != 1 || stats.Retried != 1 || stats.DeadLettered != 0 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	if publisher.topic != TopicWorkflowEvents || len(publisher.records) != 1 {
		t.Fatalf("unexpected published records: topic=%s count=%d", publisher.topic, len(publisher.records))
	}
}

func TestRelayRunOnceMarksPublisherFailureForPublishedCandidates(t *testing.T) {
	store := &fakeStore{messages: []types.OutboxMessage{validOutboxMessage()}}
	publisher := &fakePublisher{err: errors.New("broker unavailable")}
	relay := NewRelay(store, publisher, Config{})

	stats, err := relay.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("run relay once: %v", err)
	}
	if stats.Fetched != 1 || stats.Published != 0 || stats.Retried != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
}

func validOutboxMessage() types.OutboxMessage {
	return types.OutboxMessage{
		EventID:       "evt-workflow-1",
		TenantID:      "tenant-workflow",
		WorkflowID:    "workflow-1",
		AggregateType: "workflow",
		AggregateID:   "workflow-1",
		EventType:     types.WorkflowEventSubmitted,
		EventVersion:  1,
		PartitionKey:  "tenant-workflow:workflow-1",
		Producer:      "workflow-service",
		PayloadJSON:   []byte(`{"tenant_id":"tenant-workflow","workflow_id":"workflow-1","workflow_type":"ACTION_APPROVAL","status":"WAITING_DECISION"}`),
		OccurredAt:    time.Date(2026, 6, 25, 10, 0, 0, 0, time.UTC),
	}
}

type fakeStore struct {
	messages []types.OutboxMessage
}

func (store *fakeStore) ProcessReadyBatch(
	ctx context.Context,
	_ int,
	maxAttempts int,
	_ time.Duration,
	publish func(context.Context, []types.OutboxMessage) []error,
) (types.OutboxRelayStats, error) {
	errs := publish(ctx, store.messages)
	stats := types.OutboxRelayStats{Fetched: len(store.messages)}
	for _, err := range errs {
		if err == nil {
			stats.Published++
			continue
		}
		if maxAttempts <= 1 {
			stats.DeadLettered++
			continue
		}
		stats.Retried++
	}
	return stats, nil
}

type fakePublisher struct {
	topic   string
	records []types.KafkaPublishRecord
	err     error
}

func (publisher *fakePublisher) PublishBatch(_ context.Context, topic string, records []types.KafkaPublishRecord) error {
	publisher.topic = topic
	publisher.records = records
	return publisher.err
}
