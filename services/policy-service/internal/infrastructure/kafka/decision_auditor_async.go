package kafka

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/qsyy0921/IM/services/policy-service/internal/types"
	"google.golang.org/protobuf/proto"
)

const (
	defaultDecisionAuditAsyncQueueSize     = 8192
	defaultDecisionAuditAsyncWorkers       = 1
	defaultDecisionAuditAsyncBatchSize     = 100
	defaultDecisionAuditAsyncFlushInterval = 10 * time.Millisecond
	defaultDecisionAuditAsyncMaxAttempts   = 5
	defaultDecisionAuditAsyncRetryBase     = 50 * time.Millisecond
	defaultDecisionAuditAsyncRetryMax      = time.Second
	defaultDecisionAuditAsyncCloseTimeout  = 5 * time.Second
)

type DecisionAuditKafkaAsyncConfig struct {
	Topic         string
	QueueSize     int
	Workers       int
	BatchSize     int
	FlushInterval time.Duration
	MaxAttempts   int
	RetryBase     time.Duration
	RetryMax      time.Duration
	DLQTopic      string
	CloseTimeout  time.Duration
	Logf          func(string, ...any)
	StageObserver DecisionAuditStageObserver
	EventID       func() (string, error)
	Clock         func() time.Time
}

type DecisionAuditKafkaAsync struct {
	syncBuilder *DecisionAuditKafka
	publisher   DecisionAuditPublisher
	topic       string
	dlqTopic    string

	queue         chan decisionAuditKafkaAsyncRecord
	batchSize     int
	flushInterval time.Duration
	maxAttempts   int
	retryBase     time.Duration
	retryMax      time.Duration
	closeTimeout  time.Duration
	logf          func(string, ...any)

	mu       sync.RWMutex
	closed   bool
	wg       sync.WaitGroup
	closeOne sync.Once
}

type decisionAuditKafkaAsyncRecord struct {
	action types.MessageAction
	record types.KafkaPublishRecord
}

type DecisionAuditKafkaAsyncOption func(*DecisionAuditKafkaAsyncConfig)

func NewDecisionAuditKafkaAsync(publisher DecisionAuditPublisher, opts ...DecisionAuditKafkaAsyncOption) *DecisionAuditKafkaAsync {
	config := DecisionAuditKafkaAsyncConfig{
		QueueSize:     defaultDecisionAuditAsyncQueueSize,
		Workers:       defaultDecisionAuditAsyncWorkers,
		BatchSize:     defaultDecisionAuditAsyncBatchSize,
		FlushInterval: defaultDecisionAuditAsyncFlushInterval,
		MaxAttempts:   defaultDecisionAuditAsyncMaxAttempts,
		RetryBase:     defaultDecisionAuditAsyncRetryBase,
		RetryMax:      defaultDecisionAuditAsyncRetryMax,
		CloseTimeout:  defaultDecisionAuditAsyncCloseTimeout,
	}
	for _, opt := range opts {
		opt(&config)
	}
	normalizeDecisionAuditKafkaAsyncConfig(&config)
	syncBuilder := NewDecisionAuditKafka(
		publisher,
		WithDecisionAuditKafkaTopic(config.Topic),
		WithDecisionAuditKafkaStageObserver(config.StageObserver),
	)
	if config.EventID != nil {
		WithDecisionAuditKafkaEventID(config.EventID)(syncBuilder)
	}
	if config.Clock != nil {
		WithDecisionAuditKafkaClock(config.Clock)(syncBuilder)
	}
	auditor := &DecisionAuditKafkaAsync{
		syncBuilder:   syncBuilder,
		publisher:     publisher,
		topic:         config.Topic,
		dlqTopic:      config.DLQTopic,
		queue:         make(chan decisionAuditKafkaAsyncRecord, config.QueueSize),
		batchSize:     config.BatchSize,
		flushInterval: config.FlushInterval,
		maxAttempts:   config.MaxAttempts,
		retryBase:     config.RetryBase,
		retryMax:      config.RetryMax,
		closeTimeout:  config.CloseTimeout,
		logf:          config.Logf,
	}
	for i := 0; i < config.Workers; i++ {
		auditor.wg.Add(1)
		go auditor.runWorker()
	}
	return auditor
}

func WithDecisionAuditKafkaAsyncTopic(topic string) DecisionAuditKafkaAsyncOption {
	return func(config *DecisionAuditKafkaAsyncConfig) {
		if trimmed := strings.TrimSpace(topic); trimmed != "" {
			config.Topic = trimmed
		}
	}
}

func WithDecisionAuditKafkaAsyncDLQTopic(topic string) DecisionAuditKafkaAsyncOption {
	return func(config *DecisionAuditKafkaAsyncConfig) {
		if trimmed := strings.TrimSpace(topic); trimmed != "" {
			config.DLQTopic = trimmed
		}
	}
}

func WithDecisionAuditKafkaAsyncStageObserver(observer DecisionAuditStageObserver) DecisionAuditKafkaAsyncOption {
	return func(config *DecisionAuditKafkaAsyncConfig) {
		config.StageObserver = observer
	}
}

func WithDecisionAuditKafkaAsyncEventID(fn func() (string, error)) DecisionAuditKafkaAsyncOption {
	return func(config *DecisionAuditKafkaAsyncConfig) {
		config.EventID = fn
	}
}

func WithDecisionAuditKafkaAsyncClock(clock func() time.Time) DecisionAuditKafkaAsyncOption {
	return func(config *DecisionAuditKafkaAsyncConfig) {
		config.Clock = clock
	}
}

func WithDecisionAuditKafkaAsyncQueueSize(size int) DecisionAuditKafkaAsyncOption {
	return func(config *DecisionAuditKafkaAsyncConfig) {
		config.QueueSize = size
	}
}

func WithDecisionAuditKafkaAsyncWorkers(workers int) DecisionAuditKafkaAsyncOption {
	return func(config *DecisionAuditKafkaAsyncConfig) {
		config.Workers = workers
	}
}

func WithDecisionAuditKafkaAsyncBatchSize(size int) DecisionAuditKafkaAsyncOption {
	return func(config *DecisionAuditKafkaAsyncConfig) {
		config.BatchSize = size
	}
}

func WithDecisionAuditKafkaAsyncFlushInterval(interval time.Duration) DecisionAuditKafkaAsyncOption {
	return func(config *DecisionAuditKafkaAsyncConfig) {
		config.FlushInterval = interval
	}
}

func WithDecisionAuditKafkaAsyncRetry(maxAttempts int, base time.Duration, max time.Duration) DecisionAuditKafkaAsyncOption {
	return func(config *DecisionAuditKafkaAsyncConfig) {
		config.MaxAttempts = maxAttempts
		config.RetryBase = base
		config.RetryMax = max
	}
}

func WithDecisionAuditKafkaAsyncCloseTimeout(timeout time.Duration) DecisionAuditKafkaAsyncOption {
	return func(config *DecisionAuditKafkaAsyncConfig) {
		config.CloseTimeout = timeout
	}
}

func WithDecisionAuditKafkaAsyncLogf(logf func(string, ...any)) DecisionAuditKafkaAsyncOption {
	return func(config *DecisionAuditKafkaAsyncConfig) {
		config.Logf = logf
	}
}

func (auditor *DecisionAuditKafkaAsync) PolicyDecisionAuditStageName() string {
	return "decision_audit_kafka_async"
}

func (auditor *DecisionAuditKafkaAsync) RecordPolicyDecision(
	ctx context.Context,
	command types.CheckMessageActionCommand,
	decision types.MessageActionDecision,
) error {
	if auditor == nil || auditor.publisher == nil || auditor.syncBuilder == nil {
		return types.NewDependencyUnavailable("policy decision audit async kafka publisher is not configured")
	}
	record, err := auditor.buildAsyncPublishRecord(command, decision)
	if err != nil {
		return err
	}
	return auditor.enqueue(ctx, decisionAuditKafkaAsyncRecord{
		action: command.Action,
		record: record,
	})
}

func (auditor *DecisionAuditKafkaAsync) Close() error {
	if auditor == nil {
		return nil
	}
	auditor.closeOne.Do(func() {
		auditor.mu.Lock()
		auditor.closed = true
		close(auditor.queue)
		auditor.mu.Unlock()
	})
	done := make(chan struct{})
	go func() {
		auditor.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-time.After(auditor.closeTimeout):
		return fmt.Errorf("policy decision audit async kafka close timed out")
	}
}

func (auditor *DecisionAuditKafkaAsync) buildAsyncPublishRecord(
	command types.CheckMessageActionCommand,
	decision types.MessageActionDecision,
) (types.KafkaPublishRecord, error) {
	buildStarted := time.Now()
	event, partitionKey, err := auditor.syncBuilder.buildPolicyEvent(command, decision)
	auditor.recordStage(command.Action, "decision_audit_kafka_async_build", err != nil, buildStarted)
	if err != nil {
		return types.KafkaPublishRecord{}, err
	}
	marshalStarted := time.Now()
	value, err := proto.Marshal(event)
	auditor.recordStage(command.Action, "decision_audit_kafka_async_marshal", err != nil, marshalStarted)
	if err != nil {
		return types.KafkaPublishRecord{}, types.NewDependencyUnavailable("policy decision audit async kafka marshal failed")
	}
	return types.KafkaPublishRecord{
		Key:   []byte(partitionKey),
		Value: value,
	}, nil
}

func (auditor *DecisionAuditKafkaAsync) enqueue(ctx context.Context, record decisionAuditKafkaAsyncRecord) error {
	enqueueStarted := time.Now()
	auditor.mu.RLock()
	if auditor.closed {
		auditor.mu.RUnlock()
		auditor.recordStage(record.action, "decision_audit_kafka_async_enqueue", true, enqueueStarted)
		return types.NewDependencyUnavailable("policy decision audit async kafka queue is closed")
	}
	select {
	case auditor.queue <- record:
		auditor.mu.RUnlock()
		auditor.recordStage(record.action, "decision_audit_kafka_async_enqueue", false, enqueueStarted)
		return nil
	case <-ctx.Done():
		auditor.mu.RUnlock()
		auditor.recordStage(record.action, "decision_audit_kafka_async_enqueue", true, enqueueStarted)
		return types.NewDependencyUnavailable("policy decision audit async kafka enqueue canceled")
	default:
		auditor.mu.RUnlock()
		auditor.recordStage(record.action, "decision_audit_kafka_async_enqueue", true, enqueueStarted)
		return types.NewDependencyUnavailable("policy decision audit async kafka queue is full")
	}
}

func (auditor *DecisionAuditKafkaAsync) runWorker() {
	defer auditor.wg.Done()
	ticker := time.NewTicker(auditor.flushInterval)
	defer ticker.Stop()
	batch := make([]decisionAuditKafkaAsyncRecord, 0, auditor.batchSize)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		auditor.publishBatchWithRetry(batch)
		batch = batch[:0]
	}
	for {
		select {
		case record, ok := <-auditor.queue:
			if !ok {
				flush()
				return
			}
			batch = append(batch, record)
			if len(batch) >= auditor.batchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

func (auditor *DecisionAuditKafkaAsync) publishBatchWithRetry(batch []decisionAuditKafkaAsyncRecord) {
	records := kafkaRecordsFromAsyncBatch(batch)
	for attempt := 1; ; attempt++ {
		started := time.Now()
		err := auditor.publisher.PublishBatch(context.Background(), auditor.topic, records)
		auditor.recordBatchStage(batch, "decision_audit_kafka_async_publish", err != nil, started)
		if err == nil {
			return
		}
		auditor.recordBatchStage(batch, "decision_audit_kafka_async_retry", true, time.Now())
		if auditor.maxAttempts > 0 && attempt >= auditor.maxAttempts {
			auditor.log("policy decision audit async kafka publish failed after attempts=%d topic=%s records=%d: %v", attempt, auditor.topic, len(records), err)
			auditor.publishDLQWithRetry(batch)
			return
		}
		time.Sleep(auditor.retryDelay(attempt))
	}
}

func (auditor *DecisionAuditKafkaAsync) publishDLQWithRetry(batch []decisionAuditKafkaAsyncRecord) {
	records := kafkaRecordsFromAsyncBatch(batch)
	for attempt := 1; ; attempt++ {
		started := time.Now()
		err := auditor.publisher.PublishBatch(context.Background(), auditor.dlqTopic, records)
		auditor.recordBatchStage(batch, "decision_audit_kafka_async_dlq_publish", err != nil, started)
		if err == nil {
			auditor.log("policy decision audit async kafka sent records=%d to dlq topic=%s", len(records), auditor.dlqTopic)
			return
		}
		auditor.log("policy decision audit async kafka dlq publish failed attempt=%d topic=%s records=%d: %v", attempt, auditor.dlqTopic, len(records), err)
		time.Sleep(auditor.retryDelay(attempt))
	}
}

func (auditor *DecisionAuditKafkaAsync) retryDelay(attempt int) time.Duration {
	if attempt <= 0 {
		attempt = 1
	}
	delay := auditor.retryBase
	for i := 1; i < attempt; i++ {
		delay *= 2
		if delay >= auditor.retryMax {
			return auditor.retryMax
		}
	}
	if delay > auditor.retryMax {
		return auditor.retryMax
	}
	return delay
}

func (auditor *DecisionAuditKafkaAsync) recordBatchStage(batch []decisionAuditKafkaAsyncRecord, stage string, failed bool, started time.Time) {
	for _, record := range batch {
		auditor.recordStage(record.action, stage, failed, started)
	}
}

func (auditor *DecisionAuditKafkaAsync) recordStage(action types.MessageAction, stage string, failed bool, started time.Time) {
	if auditor == nil || auditor.syncBuilder == nil {
		return
	}
	auditor.syncBuilder.recordStage(action, stage, failed, started)
}

func (auditor *DecisionAuditKafkaAsync) log(format string, args ...any) {
	if auditor != nil && auditor.logf != nil {
		auditor.logf(format, args...)
	}
}

func kafkaRecordsFromAsyncBatch(batch []decisionAuditKafkaAsyncRecord) []types.KafkaPublishRecord {
	records := make([]types.KafkaPublishRecord, 0, len(batch))
	for _, item := range batch {
		records = append(records, item.record)
	}
	return records
}

func normalizeDecisionAuditKafkaAsyncConfig(config *DecisionAuditKafkaAsyncConfig) {
	if strings.TrimSpace(config.Topic) == "" {
		config.Topic = defaultDecisionAuditTopic
	} else {
		config.Topic = strings.TrimSpace(config.Topic)
	}
	if strings.TrimSpace(config.DLQTopic) == "" {
		config.DLQTopic = config.Topic + ".dlq"
	} else {
		config.DLQTopic = strings.TrimSpace(config.DLQTopic)
	}
	if config.QueueSize <= 0 {
		config.QueueSize = defaultDecisionAuditAsyncQueueSize
	}
	if config.Workers <= 0 {
		config.Workers = defaultDecisionAuditAsyncWorkers
	}
	if config.BatchSize <= 0 {
		config.BatchSize = defaultDecisionAuditAsyncBatchSize
	}
	if config.BatchSize > config.QueueSize {
		config.BatchSize = config.QueueSize
	}
	if config.FlushInterval <= 0 {
		config.FlushInterval = defaultDecisionAuditAsyncFlushInterval
	}
	if config.MaxAttempts < 0 {
		config.MaxAttempts = defaultDecisionAuditAsyncMaxAttempts
	}
	if config.RetryBase <= 0 {
		config.RetryBase = defaultDecisionAuditAsyncRetryBase
	}
	if config.RetryMax <= 0 {
		config.RetryMax = defaultDecisionAuditAsyncRetryMax
	}
	if config.RetryMax < config.RetryBase {
		config.RetryMax = config.RetryBase
	}
	if config.CloseTimeout <= 0 {
		config.CloseTimeout = defaultDecisionAuditAsyncCloseTimeout
	}
}
