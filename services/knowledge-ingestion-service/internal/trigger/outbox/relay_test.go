package outbox

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	knowledgeeventsv1 "github.com/qsyy0921/IM/schemas/kafka/knowledge/v1"
	"github.com/qsyy0921/IM/services/knowledge-ingestion-service/internal/types"
	"google.golang.org/protobuf/proto"
)

func TestBuildKnowledgeEventSourceCreated(t *testing.T) {
	event, err := BuildKnowledgeEvent(validOutboxMessage(EventKnowledgeSourceCreated))
	if err != nil {
		t.Fatalf("build source created: %v", err)
	}
	if event.GetEventId() != "evt_knowledge_1" ||
		event.GetTenantId() != "tenant-knowledge" ||
		event.GetAggregateType() != "knowledge_source" ||
		event.GetProducer() != "knowledge-ingestion-service" {
		t.Fatalf("unexpected envelope: %+v", event)
	}
	payload := event.GetSourceCreated()
	if payload == nil {
		t.Fatalf("expected source created payload: %+v", event)
	}
	if payload.GetSourceRefHash() != "sha256:source-ref" ||
		payload.GetContentHash() != "sha256:source-content" ||
		payload.GetSourceVersion() != "v1" {
		t.Fatalf("unexpected source payload: %+v", payload)
	}
}

func TestBuildKnowledgeEventDocumentParsed(t *testing.T) {
	event, err := BuildKnowledgeEvent(validOutboxMessage(EventKnowledgeDocumentParsed))
	if err != nil {
		t.Fatalf("build document parsed: %v", err)
	}
	payload := event.GetDocumentParsed()
	if payload == nil {
		t.Fatalf("expected document parsed payload: %+v", event)
	}
	if payload.GetDocumentHash() != "sha256:document" ||
		payload.GetParserProfile() != "local-manifest-v1" ||
		payload.GetChunkCount() != 2 {
		t.Fatalf("unexpected document payload: %+v", payload)
	}
}

func TestBuildKnowledgeEventChunkReady(t *testing.T) {
	event, err := BuildKnowledgeEvent(validOutboxMessage(EventKnowledgeChunkReady))
	if err != nil {
		t.Fatalf("build chunk ready: %v", err)
	}
	payload := event.GetChunkReady()
	if payload == nil {
		t.Fatalf("expected chunk ready payload: %+v", event)
	}
	if payload.GetChunkId() != "chunk-1" ||
		payload.GetChunkHash() != "sha256:chunk-1" ||
		payload.GetTombstoneStatus() != "ACTIVE" {
		t.Fatalf("unexpected chunk payload: %+v", payload)
	}
}

func TestBuildKnowledgeEventRejectsUnsafePayload(t *testing.T) {
	message := validOutboxMessage(EventKnowledgeChunkReady)
	message.PayloadJSON = []byte(`{"chunk_id":"chunk-1","chunk_preview_redacted":"should not be published"}`)
	if _, err := BuildKnowledgeEvent(message); err == nil {
		t.Fatal("expected unsafe payload to fail")
	}
}

func TestBuildKnowledgeEventRejectsUnsupportedType(t *testing.T) {
	message := validOutboxMessage("knowledge.unknown.v1")
	if _, err := BuildKnowledgeEvent(message); err == nil {
		t.Fatal("expected unsupported event to fail")
	}
}

func TestRelayPublishesKafkaRecord(t *testing.T) {
	store := &fakeStore{
		messages: []types.OutboxMessage{validOutboxMessage(EventKnowledgeChunkReady)},
	}
	publisher := &fakePublisher{}
	relay := NewRelay(store, publisher, Config{Topic: TopicKnowledgeEvents})
	stats, err := relay.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if stats.Published != 1 || publisher.topic != TopicKnowledgeEvents || len(publisher.records) != 1 {
		t.Fatalf("unexpected publish result: stats=%+v publisher=%+v", stats, publisher)
	}
	if string(publisher.records[0].Key) != "tenant-knowledge:chunk-1" {
		t.Fatalf("unexpected key: %s", string(publisher.records[0].Key))
	}
	var event knowledgeeventsv1.KnowledgeEvent
	if err := proto.Unmarshal(publisher.records[0].Value, &event); err != nil {
		t.Fatalf("unmarshal event: %v", err)
	}
	if event.GetChunkReady() == nil {
		t.Fatalf("missing chunk ready payload: %+v", &event)
	}
	if strings.Contains(string(publisher.records[0].Value), "redacted preview") {
		t.Fatal("protobuf event should not include chunk preview")
	}
}

func validOutboxMessage(eventType string) types.OutboxMessage {
	message := types.OutboxMessage{
		EventID:          "evt_knowledge_1",
		TenantID:         "tenant-knowledge",
		AggregateType:    aggregateTypeForEvent(eventType),
		AggregateID:      "chunk-1",
		EventType:        eventType,
		EventVersion:     1,
		PartitionKey:     "tenant-knowledge:chunk-1",
		Producer:         "knowledge-ingestion-service",
		RetryCount:       0,
		OccurredAt:       time.Unix(1700000000, 0).UTC(),
		AggregateVersion: 1,
		CorrelationID:    "corr-knowledge",
		CausationID:      "cause-knowledge",
		TraceID:          "trace-knowledge",
	}
	switch eventType {
	case EventKnowledgeSourceCreated:
		message.AggregateID = "source-1"
		message.PartitionKey = "tenant-knowledge:source-1"
		message.PayloadJSON = []byte(`{
			"tenant_id":"tenant-knowledge",
			"source_id":"source-1",
			"source_type":"MANUAL_MARKDOWN",
			"source_ref_hash":"sha256:source-ref",
			"visibility_scope":"tenant:tenant-knowledge",
			"data_class":"BUSINESS_INTERNAL",
			"content_hash":"sha256:source-content",
			"source_version":"v1",
			"correlation_id":"corr-knowledge",
			"causation_id":"cause-knowledge",
			"trace_id":"trace-knowledge"
		}`)
	case EventKnowledgeDocumentParsed:
		message.AggregateID = "document-1"
		message.PartitionKey = "tenant-knowledge:document-1"
		message.PayloadJSON = []byte(`{
			"tenant_id":"tenant-knowledge",
			"document_id":"document-1",
			"source_id":"source-1",
			"source_version":"v1",
			"document_hash":"sha256:document",
			"parser_profile":"local-manifest-v1",
			"mime_type":"text/markdown",
			"page_count":1,
			"chunk_count":2
		}`)
	default:
		message.PayloadJSON = []byte(`{
			"tenant_id":"tenant-knowledge",
			"chunk_id":"chunk-1",
			"document_id":"document-1",
			"source_id":"source-1",
			"source_version":"v1",
			"chunk_index":0,
			"chunk_hash":"sha256:chunk-1",
			"visibility_scope":"tenant:tenant-knowledge",
			"data_class":"BUSINESS_INTERNAL",
			"policy_version":"policy-local-v1",
			"chunk_version":"v1",
			"tombstone_status":"ACTIVE"
		}`)
	}
	return message
}

type fakeStore struct {
	messages []types.OutboxMessage
}

func (store *fakeStore) ProcessReadyBatch(
	ctx context.Context,
	limit int,
	maxAttempts int,
	retryBaseDelay time.Duration,
	publish func(context.Context, []types.OutboxMessage) []error,
) (types.OutboxRelayStats, error) {
	errs := publish(ctx, store.messages)
	if len(errs) != len(store.messages) {
		return types.OutboxRelayStats{}, errors.New("bad publish result")
	}
	stats := types.OutboxRelayStats{Fetched: len(store.messages)}
	for _, err := range errs {
		if err == nil {
			stats.Published++
		}
	}
	return stats, nil
}

type fakePublisher struct {
	topic   string
	records []types.KafkaPublishRecord
}

func (publisher *fakePublisher) PublishBatch(_ context.Context, topic string, records []types.KafkaPublishRecord) error {
	publisher.topic = topic
	publisher.records = append(publisher.records, records...)
	return nil
}
