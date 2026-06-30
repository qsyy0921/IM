package outbox

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	conversationtimelinev1 "github.com/qsyy0921/IM/schemas/kafka"
	"github.com/qsyy0921/IM/services/message-service/internal/types"
	"google.golang.org/protobuf/proto"
)

func TestRelayRunOnceUnsupportedEventFailsClosed(t *testing.T) {
	message := testMemberBoundaryOutboxMessage("conversation.member.unknown.v1")
	store := &fakeStore{messages: []types.OutboxMessage{message}}
	publisher := &fakePublisher{}
	relay := NewRelay(store, publisher, Config{Topic: "topic-it", BatchSize: 10})

	stats, err := relay.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("run relay once: %v", err)
	}
	if stats.Fetched != 1 || stats.Published != 0 || stats.Retried != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	if len(publisher.messages) != 0 || len(publisher.batches) != 0 {
		t.Fatalf("unsupported event should not publish: messages=%d batches=%d", len(publisher.messages), len(publisher.batches))
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

func TestRelayRunOncePublishesKafkaBatchWhenStoreSupportsBatch(t *testing.T) {
	first := testOutboxMessage()
	second := testOutboxMessage()
	second.ID = 2
	second.EventID = "event-2"
	second.ConversationID = "conv-2"
	second.PartitionKey = "tenant-1:conv-2"
	second.PayloadJSON = []byte(`{
		"message_id":"msg-2",
		"conversation_id":"conv-2",
		"conversation_seq":1,
		"sender_id":"user-1",
		"device_id":"device-1",
		"client_msg_id":"client-2",
		"command_hash":"hash-2",
		"message_type":"TEXT",
		"payload":{"text":"hello-2"},
		"attachment_ids":["att-2"],
		"accepted_at":"2026-06-08T12:00:00Z"
	}`)
	store := &fakeBatchStore{messages: []types.OutboxMessage{first, second}}
	publisher := &fakePublisher{}
	relay := NewRelay(store, publisher, Config{Topic: "topic-it", BatchSize: 10})

	stats, err := relay.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("run relay once: %v", err)
	}
	if stats.Fetched != 2 || stats.Published != 2 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	if len(publisher.messages) != 0 {
		t.Fatalf("expected batch path, got single publishes: %d", len(publisher.messages))
	}
	if len(publisher.batches) != 1 || len(publisher.batches[0]) != 2 {
		t.Fatalf("unexpected batch publishes: %+v", publisher.batches)
	}
	if string(publisher.batches[0][0].Key) != "tenant-1:conv-1" ||
		string(publisher.batches[0][1].Key) != "tenant-1:conv-2" {
		t.Fatalf("unexpected batch keys: %+v", publisher.batches[0])
	}
}

func TestRelayRunOnceCanDisableKafkaBatch(t *testing.T) {
	first := testOutboxMessage()
	second := testOutboxMessage()
	second.ID = 2
	second.EventID = "event-2"
	second.ConversationID = "conv-2"
	second.PartitionKey = "tenant-1:conv-2"
	store := &fakeBatchStore{messages: []types.OutboxMessage{first, second}}
	publisher := &fakePublisher{}
	relay := NewRelay(store, publisher, Config{
		Topic:               "topic-it",
		BatchSize:           10,
		DisablePublishBatch: true,
	})

	stats, err := relay.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("run relay once: %v", err)
	}
	if stats.Fetched != 2 || stats.Published != 2 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	if len(publisher.batches) != 0 {
		t.Fatalf("expected single path, got batch publishes: %+v", publisher.batches)
	}
	if len(publisher.messages) != 2 {
		t.Fatalf("expected two single publishes, got %d", len(publisher.messages))
	}
}

func TestRelayRunOnceBatchPublishFailureRetriesAllBatchRecords(t *testing.T) {
	first := testOutboxMessage()
	second := testOutboxMessage()
	second.ID = 2
	second.EventID = "event-2"
	second.ConversationID = "conv-2"
	second.PartitionKey = "tenant-1:conv-2"
	store := &fakeBatchStore{messages: []types.OutboxMessage{first, second}}
	publisher := &fakePublisher{batchErr: errors.New("kafka batch unavailable")}
	relay := NewRelay(store, publisher, Config{Topic: "topic-it", BatchSize: 10})

	stats, err := relay.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("run relay once: %v", err)
	}
	if stats.Fetched != 2 || stats.Published != 0 || stats.Retried != 2 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
}

func TestRelayRunOnceBatchPayloadErrorRetriesOnlyInvalidMessage(t *testing.T) {
	valid := testOutboxMessage()
	invalid := testOutboxMessage()
	invalid.ID = 2
	invalid.EventID = "event-2"
	invalid.ConversationID = "conv-2"
	invalid.PartitionKey = "tenant-1:conv-2"
	invalid.PayloadJSON = []byte(`{"message_id":"msg-2"}`)
	store := &fakeBatchStore{messages: []types.OutboxMessage{valid, invalid}}
	publisher := &fakePublisher{}
	relay := NewRelay(store, publisher, Config{Topic: "topic-it", BatchSize: 10})

	stats, err := relay.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("run relay once: %v", err)
	}
	if stats.Fetched != 2 || stats.Published != 1 || stats.Retried != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	if len(publisher.batches) != 1 || len(publisher.batches[0]) != 1 {
		t.Fatalf("expected one valid record in batch, got %+v", publisher.batches)
	}
}

func TestRelayRunOnceBatchPayloadErrorSkipsLaterSameConversationMessages(t *testing.T) {
	first := testOutboxMessage()
	invalid := testOutboxMessage()
	invalid.ID = 2
	invalid.EventID = "event-2"
	invalid.AggregateVersion = 2
	invalid.PayloadJSON = []byte(`{"message_id":"msg-2"}`)
	third := testOutboxMessage()
	third.ID = 3
	third.EventID = "event-3"
	third.AggregateVersion = 3
	third.PayloadJSON = []byte(`{
		"message_id":"msg-3",
		"conversation_id":"conv-1",
		"conversation_seq":3,
		"sender_id":"user-1",
		"device_id":"device-1",
		"client_msg_id":"client-3",
		"command_hash":"hash-3",
		"message_type":"TEXT",
		"payload":{"text":"hello-3"},
		"attachment_ids":["att-3"],
		"accepted_at":"2026-06-08T12:00:00Z"
	}`)
	store := &fakeBatchStore{messages: []types.OutboxMessage{first, invalid, third}}
	publisher := &fakePublisher{}
	relay := NewRelay(store, publisher, Config{Topic: "topic-it", BatchSize: 10})

	stats, err := relay.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("run relay once: %v", err)
	}
	if stats.Fetched != 3 || stats.Published != 1 || stats.Retried != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	if len(publisher.batches) != 1 || len(publisher.batches[0]) != 1 {
		t.Fatalf("expected only the valid prefix to publish, got %+v", publisher.batches)
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

func TestRelayRunOnceRecordsKafkaPublishLatency(t *testing.T) {
	store := &fakeStore{messages: []types.OutboxMessage{testOutboxMessage()}}
	metrics := &fakeMetrics{}
	relay := NewRelay(store, &fakePublisher{}, Config{Metrics: metrics})

	if _, err := relay.RunOnce(context.Background()); err != nil {
		t.Fatalf("run relay once: %v", err)
	}
	if metrics.kafkaCount != 1 {
		t.Fatalf("expected one kafka latency sample, got %d", metrics.kafkaCount)
	}
	if metrics.outboxProcessReadyCount != 1 {
		t.Fatalf("expected one outbox process ready latency sample, got %d", metrics.outboxProcessReadyCount)
	}
	if len(metrics.outboxProcessReadyFetched) != 1 || metrics.outboxProcessReadyFetched[0] != 1 {
		t.Fatalf("unexpected fetched count samples: %+v", metrics.outboxProcessReadyFetched)
	}
}

func TestRelayRetriesTransientRunOnceErrorAndExposesSnapshot(t *testing.T) {
	store := &fakeStore{
		errs:     []error{errors.New("temporary store error"), nil},
		messages: []types.OutboxMessage{testOutboxMessage()},
	}
	publisher := &fakePublisher{}
	relay := NewRelay(store, publisher, Config{
		PollInterval: time.Millisecond,
		ErrorBackoff: time.Millisecond,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		for len(publisher.messages) == 0 && len(publisher.batches) == 0 {
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

func TestRelayRunContinuesImmediatelyWhenWorkWasPublished(t *testing.T) {
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
		t.Fatalf("relay did not immediately continue after published work")
	}
	if store.calls != 2 {
		t.Fatalf("expected two store calls, got %d", store.calls)
	}
}

func TestRelayRunStartsConfiguredWorkers(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	store := newShardedBlockingStore(3)
	relay := NewRelay(store, &fakePublisher{}, Config{
		WorkerCount:  3,
		PollInterval: time.Hour,
	})

	errCh := make(chan error, 1)
	go func() {
		errCh <- relay.Run(ctx)
	}()

	select {
	case <-store.allStarted:
	case <-time.After(500 * time.Millisecond):
		cancel()
		t.Fatalf("relay did not start configured workers")
	}
	cancel()

	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("unexpected relay error: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("relay did not stop after cancel")
	}
	if store.maxActiveCount() < 3 {
		t.Fatalf("expected 3 active workers, got %d", store.maxActiveCount())
	}
	if store.callCount() < 3 {
		t.Fatalf("expected sharded workers to call store, got %d", store.callCount())
	}
}

func TestRelayRunBacksOffWhenFetchedWithoutPublished(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	store := &backoffStore{
		firstCall:  make(chan struct{}),
		secondCall: make(chan struct{}),
	}
	relay := NewRelay(store, &fakePublisher{}, Config{
		PollInterval:   time.Hour,
		FailureBackoff: 500 * time.Millisecond,
	})

	errCh := make(chan error, 1)
	go func() {
		errCh <- relay.Run(ctx)
	}()

	select {
	case <-store.firstCall:
	case <-time.After(200 * time.Millisecond):
		cancel()
		t.Fatalf("relay did not process first batch")
	}

	select {
	case <-store.secondCall:
		cancel()
		t.Fatalf("relay retried failed fetched batch without failure backoff")
	case <-time.After(100 * time.Millisecond):
	}
	cancel()

	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("unexpected relay error: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("relay did not stop after cancel")
	}
}

type publishedMessage struct {
	topic string
	key   []byte
	value []byte
}

type fakePublisher struct {
	err      error
	batchErr error
	messages []publishedMessage
	batches  [][]types.KafkaPublishRecord
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

func (p *fakePublisher) PublishBatch(_ context.Context, _ string, records []types.KafkaPublishRecord) error {
	if p.batchErr != nil {
		return p.batchErr
	}
	copied := make([]types.KafkaPublishRecord, 0, len(records))
	for _, record := range records {
		copied = append(copied, types.KafkaPublishRecord{
			Key:   append([]byte(nil), record.Key...),
			Value: append([]byte(nil), record.Value...),
		})
	}
	p.batches = append(p.batches, copied)
	return nil
}

type fakeMetrics struct {
	seqCount                  int
	kafkaCount                int
	outboxProcessReadyCount   int
	outboxProcessReadyFetched []int
	outboxFetchReadyCount     int
	outboxMarkPublishedCount  int
	outboxCommitCount         int
}

func (m *fakeMetrics) ObserveConversationSeqAlloc(time.Duration) {
	m.seqCount++
}

func (m *fakeMetrics) ObserveSendMessage(time.Duration) {}

func (m *fakeMetrics) ObserveRepositoryAppend(time.Duration) {}

func (m *fakeMetrics) ObserveRepositoryBegin(time.Duration) {}

func (m *fakeMetrics) ObserveRepositoryPoolAcquire(time.Duration) {}

func (m *fakeMetrics) ObserveRepositoryTxBegin(time.Duration) {}

func (m *fakeMetrics) ObserveRepositoryIdempotencyLock(time.Duration) {}

func (m *fakeMetrics) ObserveRepositoryFindExisting(time.Duration) {}

func (m *fakeMetrics) ObserveRepositoryEnsureSeq(time.Duration) {}

func (m *fakeMetrics) ObserveRepositoryAllocateSeq(time.Duration) {}

func (m *fakeMetrics) ObserveRepositoryInsertMessage(time.Duration) {}

func (m *fakeMetrics) ObserveRepositoryInsertTimeline(time.Duration) {}

func (m *fakeMetrics) ObserveRepositoryInsertOutbox(time.Duration) {}

func (m *fakeMetrics) ObserveRepositoryCommit(time.Duration) {}

func (m *fakeMetrics) ObserveKafkaPublish(time.Duration) {
	m.kafkaCount++
}

func (m *fakeMetrics) ObserveKafkaPublishCall(time.Duration, int) {
	m.kafkaCount++
}

func (m *fakeMetrics) ObserveOutboxProcessReady(time.Duration) {
	m.outboxProcessReadyCount++
}

func (m *fakeMetrics) ObserveOutboxProcessReadyResult(_ time.Duration, fetched int) {
	m.outboxProcessReadyCount++
	m.outboxProcessReadyFetched = append(m.outboxProcessReadyFetched, fetched)
}

func (m *fakeMetrics) ObserveOutboxFetchReady(time.Duration) {
	m.outboxFetchReadyCount++
}

func (m *fakeMetrics) ObserveOutboxMarkPublished(time.Duration) {
	m.outboxMarkPublishedCount++
}

func (m *fakeMetrics) ObserveOutboxCommit(time.Duration) {
	m.outboxCommitCount++
}

type fakeStore struct {
	messages []types.OutboxMessage
	errs     []error
	calls    int
}

func (s *fakeStore) ProcessReady(
	ctx context.Context,
	_ int,
	maxAttempts int,
	_ time.Duration,
	publish func(context.Context, types.OutboxMessage) error,
) (types.OutboxRelayStats, error) {
	if err := ctx.Err(); err != nil {
		return types.OutboxRelayStats{}, err
	}
	if s.calls < len(s.errs) {
		err := s.errs[s.calls]
		s.calls++
		if err != nil {
			return types.OutboxRelayStats{}, err
		}
	} else {
		s.calls++
	}
	stats := types.OutboxRelayStats{Fetched: len(s.messages)}
	for _, message := range s.messages {
		if err := publish(ctx, message); err != nil {
			if errors.Is(err, types.ErrOutboxPublishSkipped) {
				continue
			}
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

type fakeBatchStore struct {
	messages []types.OutboxMessage
}

func (s *fakeBatchStore) ProcessReady(
	ctx context.Context,
	_ int,
	maxAttempts int,
	_ time.Duration,
	publish func(context.Context, types.OutboxMessage) error,
) (types.OutboxRelayStats, error) {
	stats := types.OutboxRelayStats{Fetched: len(s.messages)}
	for _, message := range s.messages {
		if err := publish(ctx, message); err != nil {
			if errors.Is(err, types.ErrOutboxPublishSkipped) {
				continue
			}
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

func (s *fakeBatchStore) ProcessReadyBatch(
	ctx context.Context,
	_ int,
	maxAttempts int,
	_ time.Duration,
	publish func(context.Context, []types.OutboxMessage) []error,
) (types.OutboxRelayStats, error) {
	stats := types.OutboxRelayStats{Fetched: len(s.messages)}
	errs := publish(ctx, s.messages)
	if len(errs) != len(s.messages) {
		return types.OutboxRelayStats{}, errors.New("unexpected result count")
	}
	for index, err := range errs {
		if errors.Is(err, types.ErrOutboxPublishSkipped) {
			continue
		}
		if err != nil {
			if s.messages[index].RetryCount+1 >= maxAttempts {
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

type blockingStore struct {
	mu         sync.Mutex
	expected   int
	calls      int
	active     int
	maxActive  int
	allStarted chan struct{}
	closeOnce  sync.Once
}

func newBlockingStore(expected int) *blockingStore {
	return &blockingStore{
		expected:   expected,
		allStarted: make(chan struct{}),
	}
}

func (s *blockingStore) ProcessReady(
	ctx context.Context,
	_ int,
	_ int,
	_ time.Duration,
	_ func(context.Context, types.OutboxMessage) error,
) (types.OutboxRelayStats, error) {
	s.mu.Lock()
	s.calls++
	s.active++
	if s.active > s.maxActive {
		s.maxActive = s.active
	}
	if s.calls >= s.expected {
		s.closeOnce.Do(func() {
			close(s.allStarted)
		})
	}
	s.mu.Unlock()

	<-ctx.Done()

	s.mu.Lock()
	s.active--
	s.mu.Unlock()
	return types.OutboxRelayStats{}, ctx.Err()
}

func (s *blockingStore) maxActiveCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.maxActive
}

type shardedBlockingStore struct {
	*blockingStore
	calls atomic.Int32
}

func newShardedBlockingStore(expected int) *shardedBlockingStore {
	return &shardedBlockingStore{blockingStore: newBlockingStore(expected)}
}

func (s *shardedBlockingStore) ProcessReadyShardBatch(
	ctx context.Context,
	_ int,
	_ int,
	_ time.Duration,
	shardCount int,
	shardID int,
	_ func(context.Context, []types.OutboxMessage) []error,
) (types.OutboxRelayStats, error) {
	if shardCount != s.expected {
		return types.OutboxRelayStats{}, errors.New("unexpected shard count")
	}
	if shardID < 0 || shardID >= shardCount {
		return types.OutboxRelayStats{}, errors.New("unexpected shard id")
	}
	s.calls.Add(1)
	return s.blockingStore.ProcessReady(ctx, 0, 0, 0, nil)
}

func (s *shardedBlockingStore) callCount() int {
	return int(s.calls.Load())
}

type backoffStore struct {
	mu              sync.Mutex
	calls           int
	firstCall       chan struct{}
	secondCall      chan struct{}
	firstCloseOnce  sync.Once
	secondCloseOnce sync.Once
}

func (s *backoffStore) ProcessReady(
	_ context.Context,
	_ int,
	_ int,
	_ time.Duration,
	_ func(context.Context, types.OutboxMessage) error,
) (types.OutboxRelayStats, error) {
	s.mu.Lock()
	s.calls++
	calls := s.calls
	s.mu.Unlock()

	if calls == 1 {
		s.firstCloseOnce.Do(func() {
			close(s.firstCall)
		})
		return types.OutboxRelayStats{Fetched: 1, Retried: 1}, nil
	}
	if calls == 2 {
		s.secondCloseOnce.Do(func() {
			close(s.secondCall)
		})
	}
	return types.OutboxRelayStats{}, nil
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

func testMemberBoundaryOutboxMessage(eventType types.TimelineEventType) types.OutboxMessage {
	occurredAt := time.Date(2026, 6, 9, 9, 30, 0, 0, time.UTC)
	return types.OutboxMessage{
		ID:                  10,
		EventID:             "member-event-1",
		TenantID:            "tenant-1",
		ConversationID:      "conv-1",
		AggregateVersion:    2,
		EventType:           eventType,
		EventVersion:        "v1",
		PartitionKey:        "tenant-1:conv-1",
		MappingVersion:      string(eventType),
		CorrelationID:       "request-1",
		CausationID:         "change-1",
		Producer:            "conversation-service",
		TraceID:             "trace-1",
		FanoutMode:          types.FanoutModeWriteFanout,
		FanoutPolicyVersion: 3,
		PermissionVersion:   8,
		Classification:      "MEMBER_BOUNDARY",
		OccurredAt:          occurredAt,
		PayloadJSON: []byte(`{
			"change_id":"change-1",
			"conversation_id":"conv-1",
			"boundary_seq":2,
			"target_user_id":"user-2",
			"operator_user_id":"owner-1",
			"change_type":"JOIN",
			"old_role":"MEMBER",
			"new_role":"ADMIN",
			"old_status":"LEFT",
			"new_status":"ACTIVE",
			"member_version":7,
			"permission_version":8,
			"reason":"invite accepted",
			"occurred_at":"2026-06-09T09:30:00Z"
		}`),
	}
}

func testOwnerTransferredOutboxMessage() types.OutboxMessage {
	message := testMemberBoundaryOutboxMessage(types.TimelineEventConversationMemberOwnerTransferred)
	message.EventID = "owner-transfer-event-1"
	message.AggregateVersion = 3
	message.PermissionVersion = 9
	message.PayloadJSON = []byte(`{
		"change_id":"change-owner-1",
		"conversation_id":"conv-1",
		"boundary_seq":3,
		"previous_owner_user_id":"owner-1",
		"new_owner_user_id":"user-2",
		"operator_user_id":"owner-1",
		"change_type":"OWNER_TRANSFER",
		"previous_owner_old_role":"OWNER",
		"previous_owner_new_role":"ADMIN",
		"new_owner_old_role":"MEMBER",
		"new_owner_new_role":"OWNER",
		"previous_owner_status":"ACTIVE",
		"new_owner_status":"ACTIVE",
		"member_version":8,
		"permission_version":9,
		"reason":"owner handoff",
		"occurred_at":"2026-06-09T09:31:00Z"
	}`)
	return message
}

func testRevokedOutboxMessage() types.OutboxMessage {
	occurredAt := time.Date(2026, 6, 8, 12, 1, 0, 0, time.UTC)
	return types.OutboxMessage{
		ID:                  2,
		EventID:             "event-revoke-1",
		TenantID:            "tenant-1",
		ConversationID:      "conv-1",
		AggregateVersion:    2,
		EventType:           types.TimelineEventMessageRevoked,
		EventVersion:        "v1",
		PartitionKey:        "tenant-1:conv-1",
		MappingVersion:      "message.revoked.v1",
		CorrelationID:       "request-1",
		CausationID:         "revoke-1",
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
			"conversation_seq":2,
			"change_version":1,
			"revoked_by":"user-1",
			"reason":"mistake",
			"revoked_at":"2026-06-08T12:01:00Z"
		}`),
	}
}

func testEditedOutboxMessage() types.OutboxMessage {
	occurredAt := time.Date(2026, 6, 8, 12, 1, 0, 0, time.UTC)
	return types.OutboxMessage{
		ID:                  3,
		EventID:             "event-edit-1",
		TenantID:            "tenant-1",
		ConversationID:      "conv-1",
		AggregateVersion:    2,
		EventType:           types.TimelineEventMessageEdited,
		EventVersion:        "v1",
		PartitionKey:        "tenant-1:conv-1",
		MappingVersion:      "message.edited.v1",
		CorrelationID:       "request-1",
		CausationID:         "edit-1",
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
			"conversation_seq":2,
			"change_version":1,
			"edited_by":"user-1",
			"before_payload":{"text":"hello"},
			"after_payload":{"text":"hello edited"},
			"reason":"typo",
			"edited_at":"2026-06-08T12:01:00Z"
		}`),
	}
}

func testDeletedOutboxMessage() types.OutboxMessage {
	occurredAt := time.Date(2026, 6, 8, 12, 1, 0, 0, time.UTC)
	return types.OutboxMessage{
		ID:                  4,
		EventID:             "event-delete-1",
		TenantID:            "tenant-1",
		ConversationID:      "conv-1",
		AggregateVersion:    2,
		EventType:           types.TimelineEventMessageDeleted,
		EventVersion:        "v1",
		PartitionKey:        "tenant-1:conv-1",
		MappingVersion:      "message.deleted.v1",
		CorrelationID:       "request-1",
		CausationID:         "delete-1",
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
			"conversation_seq":2,
			"change_version":1,
			"deleted_by":"user-1",
			"delete_scope":"CONVERSATION_VIEW",
			"reason":"cleanup",
			"deleted_at":"2026-06-08T12:01:00Z"
		}`),
	}
}
