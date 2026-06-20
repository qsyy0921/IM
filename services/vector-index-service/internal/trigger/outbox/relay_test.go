package outbox

import (
	"context"
	"errors"
	"testing"
	"time"

	vectoreventsv1 "github.com/qsyy0921/IM/schemas/kafka/vector/v1"
	"github.com/qsyy0921/IM/services/vector-index-service/internal/types"
	"google.golang.org/protobuf/proto"
)

func TestBuildVectorEventIndexed(t *testing.T) {
	event, err := BuildVectorEvent(validOutboxMessage("vector.item.indexed.v1"))
	if err != nil {
		t.Fatalf("build vector event: %v", err)
	}
	if event.GetEventId() != "evt_vector_1" ||
		event.GetTenantId() != "tenant-vector" ||
		event.GetAggregateType() != "vector_item" ||
		event.GetProducer() != "vector-index-service" {
		t.Fatalf("unexpected envelope: %+v", event)
	}
	payload := event.GetItemIndexed()
	if payload == nil {
		t.Fatalf("expected indexed payload: %+v", event)
	}
	if payload.GetVectorItemRefHash() != "sha256:vitem" ||
		payload.GetSourceRefHash() != "sha256:source" ||
		payload.GetDimension() != 1536 ||
		payload.GetVisibilityVersion() != 7 {
		t.Fatalf("unexpected indexed payload: %+v", payload)
	}
}

func TestBuildVectorEventTombstoned(t *testing.T) {
	message := validOutboxMessage("vector.item.tombstoned.v1")
	message.PayloadJSON = []byte(`{
		"vector_item_ref_hash":"sha256:vitem",
		"collection_type":"MEMORY_EVENT",
		"source_service":"memory-service",
		"source_ref_hash":"sha256:source",
		"embedding_model_ref":"embed:model:v1",
		"dimension":1536,
		"visibility_version":7,
		"tombstone_status":"TOMBSTONED",
		"delete_proof_id":"delete-proof-1",
		"reason_class":"USER_DELETE"
	}`)
	event, err := BuildVectorEvent(message)
	if err != nil {
		t.Fatalf("build vector event: %v", err)
	}
	payload := event.GetItemTombstoned()
	if payload == nil {
		t.Fatalf("expected tombstoned payload: %+v", event)
	}
	if payload.GetDeleteProofId() != "delete-proof-1" || payload.GetReasonClass() != "USER_DELETE" {
		t.Fatalf("unexpected tombstone payload: %+v", payload)
	}
}

func TestBuildVectorEventRejectsUnsafePayload(t *testing.T) {
	message := validOutboxMessage("vector.item.indexed.v1")
	message.PayloadJSON = []byte(`{"vector_item_ref_hash":"sha256:vitem","embedding_vector":[0.1]}`)
	if _, err := BuildVectorEvent(message); err == nil {
		t.Fatal("expected unsafe payload to fail")
	}
}

func TestBuildVectorEventRejectsUnsupportedType(t *testing.T) {
	message := validOutboxMessage("vector.rebuild.started.v1")
	if _, err := BuildVectorEvent(message); err == nil {
		t.Fatal("expected unsupported event to fail")
	}
}

func TestRelayPublishesKafkaRecord(t *testing.T) {
	store := &fakeStore{
		messages: []types.OutboxMessage{validOutboxMessage("vector.item.indexed.v1")},
	}
	publisher := &fakePublisher{}
	relay := NewRelay(store, publisher, Config{Topic: "im.vector.events"})
	stats, err := relay.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if stats.Published != 1 || publisher.topic != "im.vector.events" || len(publisher.records) != 1 {
		t.Fatalf("unexpected publish result: stats=%+v publisher=%+v", stats, publisher)
	}
	if string(publisher.records[0].Key) != "tenant-vector:sha256:vitem" {
		t.Fatalf("unexpected key: %s", string(publisher.records[0].Key))
	}
	var event vectoreventsv1.VectorEvent
	if err := proto.Unmarshal(publisher.records[0].Value, &event); err != nil {
		t.Fatalf("unmarshal event: %v", err)
	}
	if event.GetItemIndexed() == nil {
		t.Fatalf("missing indexed payload: %+v", &event)
	}
}

func validOutboxMessage(eventType string) types.OutboxMessage {
	return types.OutboxMessage{
		EventID:          "evt_vector_1",
		TenantID:         "tenant-vector",
		AggregateID:      "sha256:vitem",
		EventType:        eventType,
		EventVersion:     1,
		PartitionKey:     "tenant-vector:sha256:vitem",
		Producer:         "vector-index-service",
		RetryCount:       0,
		OccurredAt:       time.Unix(1700000000, 0).UTC(),
		AggregateVersion: 1,
		CorrelationID:    "corr-vector",
		CausationID:      "cause-vector",
		TraceID:          "trace-vector",
		PayloadJSON: []byte(`{
			"vector_item_ref_hash":"sha256:vitem",
			"collection_type":"MEMORY_EVENT",
			"source_service":"memory-service",
			"source_ref_hash":"sha256:source",
			"embedding_model_ref":"embed:model:v1",
			"dimension":1536,
			"visibility_version":7,
			"tombstone_status":"NONE"
		}`),
	}
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
