package outbox

import (
	"context"
	"errors"
	"testing"
	"time"

	conversationtimelinev1 "github.com/qsyy0921/IM/schemas/kafka"
	"github.com/qsyy0921/IM/services/message-service/internal/types"
	"google.golang.org/protobuf/proto"
)

func TestBuildConversationTimelineEventMessagePersisted(t *testing.T) {
	message := testOutboxMessage()

	event, err := BuildConversationTimelineEvent(message)
	if err != nil {
		t.Fatalf("build event: %v", err)
	}
	if event.EventId != string(message.EventID) ||
		event.EventType != string(types.TimelineEventMessagePersisted) ||
		event.AggregateType != "conversation" ||
		event.AggregateVersion != message.AggregateVersion ||
		event.PartitionKey != message.PartitionKey {
		t.Fatalf("unexpected envelope: %+v", event)
	}
	if event.Metadata == nil ||
		event.Metadata.PermissionVersion != message.PermissionVersion ||
		event.Metadata.FanoutMode != string(message.FanoutMode) {
		t.Fatalf("unexpected metadata: %+v", event.Metadata)
	}
	payload := event.GetMessagePersisted()
	if payload == nil {
		t.Fatalf("expected message_persisted payload")
	}
	if payload.CommandHash != "hash-1" ||
		payload.MessageId != "msg-1" ||
		payload.ConversationSeq != 1 ||
		payload.Payload.GetFields()["text"].GetStringValue() != "hello" {
		t.Fatalf("unexpected message payload: %+v", payload)
	}
}

func TestRelayRunOncePublishesKafkaMessage(t *testing.T) {
	store := &fakeStore{messages: []types.OutboxMessage{testOutboxMessage()}}
	publisher := &fakePublisher{}
	relay := NewRelay(store, publisher, Config{Topic: "topic-it", BatchSize: 10})

	stats, err := relay.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("run relay once: %v", err)
	}
	if stats.Fetched != 1 || stats.Published != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	if len(publisher.messages) != 1 {
		t.Fatalf("expected one publish, got %d", len(publisher.messages))
	}
	published := publisher.messages[0]
	if published.topic != "topic-it" || string(published.key) != "tenant-1:conv-1" {
		t.Fatalf("unexpected publish target: %+v", published)
	}
	var event conversationtimelinev1.ConversationTimelineEvent
	if err := proto.Unmarshal(published.value, &event); err != nil {
		t.Fatalf("decode kafka value: %v", err)
	}
	if event.GetMessagePersisted().GetCommandHash() != "hash-1" {
		t.Fatalf("unexpected kafka payload: %+v", event.GetMessagePersisted())
	}
}

func TestRelayRunOnceRecordsPublishFailure(t *testing.T) {
	store := &fakeStore{messages: []types.OutboxMessage{testOutboxMessage()}}
	publisher := &fakePublisher{err: errors.New("kafka unavailable")}
	relay := NewRelay(store, publisher, Config{Topic: "topic-it", BatchSize: 10})

	stats, err := relay.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("run relay once: %v", err)
	}
	if stats.Fetched != 1 || stats.Retried != 1 || stats.Published != 0 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
}

func TestRelayRunContinuesImmediatelyWhenWorkWasFetched(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	store := &countingStore{
		stats: []types.OutboxRelayStats{
			{Fetched: 1, Published: 1},
			{Fetched: 0},
		},
		cancel: cancel,
	}
	relay := NewRelay(store, &fakePublisher{}, Config{PollInterval: time.Hour})

	errCh := make(chan error, 1)
	go func() {
		errCh <- relay.Run(ctx)
	}()

	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("unexpected relay error: %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("relay did not immediately continue after fetched work")
	}
	if store.calls != 2 {
		t.Fatalf("expected two store calls, got %d", store.calls)
	}
}

type publishedMessage struct {
	topic string
	key   []byte
	value []byte
}

type fakePublisher struct {
	err      error
	messages []publishedMessage
}

func (p *fakePublisher) Publish(_ context.Context, topic string, key []byte, value []byte) error {
	if p.err != nil {
		return p.err
	}
	p.messages = append(p.messages, publishedMessage{
		topic: topic,
		key:   append([]byte(nil), key...),
		value: append([]byte(nil), value...),
	})
	return nil
}

type fakeStore struct {
	messages []types.OutboxMessage
}

func (s *fakeStore) ProcessReady(
	ctx context.Context,
	_ int,
	maxAttempts int,
	_ time.Duration,
	publish func(context.Context, types.OutboxMessage) error,
) (types.OutboxRelayStats, error) {
	stats := types.OutboxRelayStats{Fetched: len(s.messages)}
	for _, message := range s.messages {
		if err := publish(ctx, message); err != nil {
			if message.RetryCount+1 >= maxAttempts {
				stats.DeadLettered++
			} else {
				stats.Retried++
			}
			continue
		}
		stats.Published++
	}
	return stats, nil
}

type countingStore struct {
	stats  []types.OutboxRelayStats
	cancel context.CancelFunc
	calls  int
}

func (s *countingStore) ProcessReady(
	context.Context,
	int,
	int,
	time.Duration,
	func(context.Context, types.OutboxMessage) error,
) (types.OutboxRelayStats, error) {
	s.calls++
	index := s.calls - 1
	if index >= len(s.stats) {
		if s.cancel != nil {
			s.cancel()
		}
		return types.OutboxRelayStats{}, nil
	}
	if index == len(s.stats)-1 && s.cancel != nil {
		s.cancel()
	}
	return s.stats[index], nil
}

func testOutboxMessage() types.OutboxMessage {
	occurredAt := time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC)
	return types.OutboxMessage{
		ID:                  1,
		EventID:             "event-1",
		TenantID:            "tenant-1",
		ConversationID:      "conv-1",
		AggregateVersion:    1,
		EventType:           types.TimelineEventMessagePersisted,
		EventVersion:        "v1",
		PartitionKey:        "tenant-1:conv-1",
		MappingVersion:      "message.persisted.v1",
		CorrelationID:       "request-1",
		CausationID:         "client-1",
		Producer:            "message-service",
		TraceID:             "trace-1",
		FanoutMode:          types.FanoutModeWriteFanout,
		FanoutPolicyVersion: 1,
		PermissionVersion:   1,
		Classification:      "INTERNAL",
		OccurredAt:          occurredAt,
		PayloadJSON: []byte(`{
			"message_id":"msg-1",
			"conversation_id":"conv-1",
			"conversation_seq":1,
			"sender_id":"user-1",
			"device_id":"device-1",
			"client_msg_id":"client-1",
			"command_hash":"hash-1",
			"message_type":"TEXT",
			"payload":{"text":"hello"},
			"attachment_ids":["att-1"],
			"accepted_at":"2026-06-08T12:00:00Z"
		}`),
	}
}
