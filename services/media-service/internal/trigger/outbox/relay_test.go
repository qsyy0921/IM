package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	mediaeventsv1 "github.com/qsyy0921/IM/schemas/kafka/media/v1"
	"github.com/qsyy0921/IM/services/media-service/internal/types"
	"google.golang.org/protobuf/proto"
)

func TestBuildMediaEventReady(t *testing.T) {
	value, err := BuildKafkaValue(outboxMessage(types.MediaEventAssetReady, map[string]any{
		"tenant_id":       "tenant-media",
		"asset_id":        "asset-1",
		"conversation_id": "conv-1",
		"media_kind":      "IMAGE",
		"content_type":    "image/png",
		"size_bytes":      64,
		"sha256":          strings.Repeat("a", 64),
		"status":          "READY",
	}))
	if err != nil {
		t.Fatalf("build kafka value: %v", err)
	}
	var event mediaeventsv1.MediaEvent
	if err := proto.Unmarshal(value, &event); err != nil {
		t.Fatalf("decode media event: %v", err)
	}
	ready := event.GetAssetReady()
	if ready == nil {
		t.Fatalf("expected ready payload: %+v", &event)
	}
	if event.EventId != "evt-media-1" ||
		event.EventType != types.MediaEventAssetReady ||
		event.PartitionKey != "tenant-media:asset-1" ||
		event.Producer != "media-service" ||
		ready.AssetId != "asset-1" ||
		ready.Status != "READY" {
		t.Fatalf("unexpected event: %+v payload=%+v", &event, ready)
	}
}

func TestBuildMediaEventRejectsInternalPayloadFields(t *testing.T) {
	_, err := BuildMediaEvent(outboxMessage(types.MediaEventAssetReady, map[string]any{
		"tenant_id":       "tenant-media",
		"asset_id":        "asset-1",
		"conversation_id": "conv-1",
		"media_kind":      "IMAGE",
		"content_type":    "image/png",
		"size_bytes":      64,
		"sha256":          strings.Repeat("a", 64),
		"status":          "READY",
		"object_key":      "tenant-media/conv-1/object-secret",
	}))
	if err == nil {
		t.Fatalf("expected internal object_key payload to be rejected")
	}
}

func TestBuildMediaEventRejectsUnsupportedAndMalformed(t *testing.T) {
	if _, err := BuildMediaEvent(outboxMessage("media.asset.future.v9", map[string]any{
		"tenant_id":       "tenant-media",
		"asset_id":        "asset-1",
		"conversation_id": "conv-1",
		"media_kind":      "IMAGE",
		"content_type":    "image/png",
		"size_bytes":      64,
		"sha256":          strings.Repeat("a", 64),
		"status":          "READY",
	})); err == nil {
		t.Fatalf("expected unsupported event type to fail")
	}
	message := outboxMessage(types.MediaEventAssetReady, map[string]any{
		"tenant_id": "tenant-media",
	})
	message.PayloadJSON = []byte(`{"tenant_id":`)
	if _, err := BuildMediaEvent(message); err == nil {
		t.Fatalf("expected malformed payload to fail")
	}
}

func TestRelayRunOncePublishesOnlyBuildableMessages(t *testing.T) {
	store := &fakeStore{
		messages: []types.OutboxMessage{
			outboxMessage(types.MediaEventAssetReady, map[string]any{
				"tenant_id":       "tenant-media",
				"asset_id":        "asset-1",
				"conversation_id": "conv-1",
				"media_kind":      "IMAGE",
				"content_type":    "image/png",
				"size_bytes":      64,
				"sha256":          strings.Repeat("a", 64),
				"status":          "READY",
			}),
			outboxMessage("media.asset.future.v9", map[string]any{"tenant_id": "tenant-media"}),
		},
	}
	publisher := &fakePublisher{}
	relay := NewRelay(store, publisher, Config{})
	stats, err := relay.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("run relay once: %v", err)
	}
	if stats.Fetched != 2 || stats.Published != 1 || stats.Retried != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	if len(publisher.records) != 1 {
		t.Fatalf("expected one published record, got %d", len(publisher.records))
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
	stats := types.OutboxRelayStats{Fetched: len(store.messages)}
	for _, err := range errs {
		if err == nil {
			stats.Published++
			continue
		}
		stats.Retried++
	}
	if len(errs) != len(store.messages) {
		return types.OutboxRelayStats{}, errors.New("mismatched publish result")
	}
	return stats, nil
}

type fakePublisher struct {
	records []types.KafkaPublishRecord
}

func (publisher *fakePublisher) PublishBatch(_ context.Context, _ string, records []types.KafkaPublishRecord) error {
	publisher.records = append(publisher.records, records...)
	return nil
}

func outboxMessage(eventType string, payload map[string]any) types.OutboxMessage {
	payloadJSON, _ := json.Marshal(payload)
	return types.OutboxMessage{
		ID:               1,
		EventID:          "evt-media-1",
		TenantID:         "tenant-media",
		AssetID:          "asset-1",
		EventType:        eventType,
		EventVersion:     1,
		PartitionKey:     "tenant-media:asset-1",
		Producer:         "media-service",
		PayloadJSON:      payloadJSON,
		OccurredAt:       time.Date(2026, 6, 20, 8, 0, 0, 0, time.UTC),
		AggregateVersion: 1,
	}
}
