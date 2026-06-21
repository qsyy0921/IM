package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"time"

	knowledgeeventsv1 "github.com/qsyy0921/IM/schemas/kafka/knowledge/v1"
	"github.com/qsyy0921/IM/services/knowledge-ingestion-service/internal/types"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	TopicKnowledgeEvents = "im.knowledge.events"

	EventKnowledgeSourceCreated       = "knowledge.source.created.v1"
	EventKnowledgeDocumentParsed      = "knowledge.document.parsed.v1"
	EventKnowledgeChunkReady          = "knowledge.chunk.ready.v1"
	EventKnowledgeChunkTombstoned     = "knowledge.chunk.tombstoned.v1"
	EventKnowledgeIngestionFailed     = "knowledge.ingestion.failed.v1"
	EventKnowledgeDeleteProofRecorded = "knowledge.delete_proof.recorded.v1"
)

type Store interface {
	ProcessReadyBatch(
		ctx context.Context,
		limit int,
		maxAttempts int,
		retryBaseDelay time.Duration,
		publish func(context.Context, []types.OutboxMessage) []error,
	) (types.OutboxRelayStats, error)
}

type Publisher interface {
	PublishBatch(ctx context.Context, topic string, records []types.KafkaPublishRecord) error
}

type Relay struct {
	store     Store
	publisher Publisher
	config    Config
	metrics   relayMetrics
}

type Config struct {
	Topic          string
	BatchSize      int
	PollInterval   time.Duration
	MaxAttempts    int
	RetryBaseDelay time.Duration
	ErrorBackoff   time.Duration
	Logf           func(format string, args ...any)
}

type relayMetrics struct {
	totalErrors        atomic.Uint64
	consecutiveErrors  atomic.Uint64
	lastErrorAtMS      atomic.Int64
	lastSuccessAtMS    atomic.Int64
	lastPublishedAtMS  atomic.Int64
	lastErrorBackoffMS atomic.Int64
}

type Snapshot struct {
	TotalErrors        uint64
	ConsecutiveErrors  uint64
	LastErrorAtMS      int64
	LastSuccessAtMS    int64
	LastPublishedAtMS  int64
	LastErrorBackoffMS int64
}

func NewRelay(store Store, publisher Publisher, config Config) *Relay {
	return &Relay{
		store:     store,
		publisher: publisher,
		config:    normalizeConfig(config),
	}
}

func (relay *Relay) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		stats, err := relay.RunOnce(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return context.Canceled
			}
			if relay.config.Logf != nil {
				relay.config.Logf("knowledge-ingestion-service outbox relay retrying after runtime error: %v", err)
			}
			relay.recordError()
			relay.metrics.lastErrorBackoffMS.Store(relay.config.ErrorBackoff.Milliseconds())
			if err := waitForInterval(ctx, relay.config.ErrorBackoff); err != nil {
				return err
			}
			continue
		}
		relay.recordSuccess(stats)
		if stats.Published > 0 {
			continue
		}
		if err := waitForInterval(ctx, relay.config.PollInterval); err != nil {
			return err
		}
	}
}

func (relay *Relay) Snapshot() Snapshot {
	return Snapshot{
		TotalErrors:        relay.metrics.totalErrors.Load(),
		ConsecutiveErrors:  relay.metrics.consecutiveErrors.Load(),
		LastErrorAtMS:      relay.metrics.lastErrorAtMS.Load(),
		LastSuccessAtMS:    relay.metrics.lastSuccessAtMS.Load(),
		LastPublishedAtMS:  relay.metrics.lastPublishedAtMS.Load(),
		LastErrorBackoffMS: relay.metrics.lastErrorBackoffMS.Load(),
	}
}

func (relay *Relay) RunOnce(ctx context.Context) (types.OutboxRelayStats, error) {
	if relay == nil || relay.store == nil {
		return types.OutboxRelayStats{}, errors.New("knowledge outbox relay store is not configured")
	}
	if relay.publisher == nil {
		return types.OutboxRelayStats{}, errors.New("knowledge outbox relay publisher is not configured")
	}
	return relay.store.ProcessReadyBatch(
		ctx,
		relay.config.BatchSize,
		relay.config.MaxAttempts,
		relay.config.RetryBaseDelay,
		relay.publishMessages,
	)
}

func (relay *Relay) publishMessages(ctx context.Context, messages []types.OutboxMessage) []error {
	errs := make([]error, len(messages))
	records := make([]types.KafkaPublishRecord, 0, len(messages))
	indexes := make([]int, 0, len(messages))
	for index, message := range messages {
		value, err := BuildKafkaValue(message)
		if err != nil {
			errs[index] = err
			continue
		}
		records = append(records, types.KafkaPublishRecord{
			Key:   []byte(message.PartitionKey),
			Value: value,
		})
		indexes = append(indexes, index)
	}
	if len(records) == 0 {
		return errs
	}
	if err := relay.publisher.PublishBatch(ctx, relay.config.Topic, records); err != nil {
		for _, index := range indexes {
			errs[index] = err
		}
	}
	return errs
}

func BuildKafkaValue(message types.OutboxMessage) ([]byte, error) {
	event, err := BuildKnowledgeEvent(message)
	if err != nil {
		return nil, err
	}
	return proto.Marshal(event)
}

func BuildKnowledgeEvent(message types.OutboxMessage) (*knowledgeeventsv1.KnowledgeEvent, error) {
	if strings.TrimSpace(message.EventID) == "" ||
		strings.TrimSpace(message.EventType) == "" ||
		strings.TrimSpace(string(message.TenantID)) == "" ||
		strings.TrimSpace(message.AggregateID) == "" ||
		message.EventVersion <= 0 ||
		strings.TrimSpace(message.PartitionKey) == "" ||
		strings.TrimSpace(message.Producer) == "" {
		return nil, errors.New("knowledge outbox envelope is incomplete")
	}
	payload, err := decodeKnowledgePayload(message.PayloadJSON)
	if err != nil {
		return nil, err
	}
	if err := validateKnowledgePayloadForEvent(message.EventType, payload); err != nil {
		return nil, err
	}
	event := &knowledgeeventsv1.KnowledgeEvent{
		EventId:          message.EventID,
		EventType:        message.EventType,
		EventVersion:     int32(message.EventVersion),
		TenantId:         string(message.TenantID),
		AggregateType:    firstNonEmpty(message.AggregateType, aggregateTypeForEvent(message.EventType)),
		AggregateId:      message.AggregateID,
		AggregateVersion: message.AggregateVersion,
		PartitionKey:     message.PartitionKey,
		TraceId:          firstNonEmpty(message.TraceID, payload.TraceID),
		CorrelationId:    firstNonEmpty(message.CorrelationID, payload.CorrelationID, message.EventID),
		CausationId:      firstNonEmpty(message.CausationID, payload.CausationID, message.EventID),
		Producer:         message.Producer,
		OccurredAt:       timestamppb.New(message.OccurredAt),
	}
	tenantID := string(message.TenantID)
	switch message.EventType {
	case EventKnowledgeSourceCreated:
		event.Payload = &knowledgeeventsv1.KnowledgeEvent_SourceCreated{
			SourceCreated: knowledgeSourceCreated(payload, tenantID),
		}
	case EventKnowledgeDocumentParsed:
		event.Payload = &knowledgeeventsv1.KnowledgeEvent_DocumentParsed{
			DocumentParsed: knowledgeDocumentParsed(payload, tenantID),
		}
	case EventKnowledgeChunkReady:
		event.Payload = &knowledgeeventsv1.KnowledgeEvent_ChunkReady{
			ChunkReady: knowledgeChunkReady(payload, tenantID),
		}
	case EventKnowledgeChunkTombstoned:
		event.Payload = &knowledgeeventsv1.KnowledgeEvent_ChunkTombstoned{
			ChunkTombstoned: knowledgeChunkTombstoned(payload, tenantID),
		}
	case EventKnowledgeIngestionFailed:
		event.Payload = &knowledgeeventsv1.KnowledgeEvent_IngestionFailed{
			IngestionFailed: knowledgeIngestionFailed(payload, tenantID),
		}
	case EventKnowledgeDeleteProofRecorded:
		event.Payload = &knowledgeeventsv1.KnowledgeEvent_DeleteProofRecorded{
			DeleteProofRecorded: knowledgeDeleteProofRecorded(payload, tenantID),
		}
	default:
		return nil, errors.New("unsupported knowledge outbox event type")
	}
	return event, nil
}

type knowledgePayload struct {
	TenantID        string `json:"tenant_id"`
	SourceID        string `json:"source_id"`
	SourceType      string `json:"source_type"`
	SourceRefHash   string `json:"source_ref_hash"`
	VisibilityScope string `json:"visibility_scope"`
	DataClass       string `json:"data_class"`
	ContentHash     string `json:"content_hash"`
	SourceVersion   string `json:"source_version"`
	CorrelationID   string `json:"correlation_id"`
	CausationID     string `json:"causation_id"`
	TraceID         string `json:"trace_id"`
	DocumentID      string `json:"document_id"`
	DocumentHash    string `json:"document_hash"`
	ParserProfile   string `json:"parser_profile"`
	MimeType        string `json:"mime_type"`
	PageCount       int32  `json:"page_count"`
	ChunkCount      int32  `json:"chunk_count"`
	ChunkID         string `json:"chunk_id"`
	ChunkIndex      int32  `json:"chunk_index"`
	ChunkHash       string `json:"chunk_hash"`
	PolicyVersion   string `json:"policy_version"`
	ChunkVersion    string `json:"chunk_version"`
	TombstoneStatus string `json:"tombstone_status"`
	DeleteProofID   string `json:"delete_proof_id"`
	ReasonClass     string `json:"reason_class"`
	JobID           string `json:"job_id"`
	FailureClass    string `json:"failure_class"`
	PublicError     string `json:"public_error"`
	ProofType       string `json:"proof_type"`
	ProofRefHash    string `json:"proof_ref_hash"`
}

func decodeKnowledgePayload(payloadJSON []byte) (knowledgePayload, error) {
	if containsForbiddenPayloadField(payloadJSON) {
		return knowledgePayload{}, errors.New("knowledge payload contains internal field")
	}
	var payload knowledgePayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return knowledgePayload{}, err
	}
	return payload, nil
}

func validateKnowledgePayloadForEvent(eventType string, payload knowledgePayload) error {
	switch eventType {
	case EventKnowledgeSourceCreated:
		if payload.SourceID == "" || payload.SourceType == "" || payload.SourceRefHash == "" ||
			payload.VisibilityScope == "" || payload.DataClass == "" || payload.ContentHash == "" ||
			payload.SourceVersion == "" {
			return errors.New("knowledge source created payload is incomplete")
		}
	case EventKnowledgeDocumentParsed:
		if payload.DocumentID == "" || payload.SourceID == "" || payload.SourceVersion == "" ||
			payload.DocumentHash == "" || payload.ParserProfile == "" {
			return errors.New("knowledge document parsed payload is incomplete")
		}
	case EventKnowledgeChunkReady:
		if payload.ChunkID == "" || payload.SourceID == "" || payload.ChunkHash == "" ||
			payload.VisibilityScope == "" || payload.DataClass == "" || payload.PolicyVersion == "" ||
			payload.ChunkVersion == "" || payload.TombstoneStatus == "" {
			return errors.New("knowledge chunk ready payload is incomplete")
		}
	case EventKnowledgeChunkTombstoned:
		if payload.ChunkID == "" || payload.ChunkHash == "" || payload.DeleteProofID == "" ||
			payload.ReasonClass == "" || payload.TombstoneStatus == "" {
			return errors.New("knowledge chunk tombstoned payload is incomplete")
		}
	case EventKnowledgeIngestionFailed:
		if payload.JobID == "" || payload.SourceID == "" || payload.FailureClass == "" ||
			payload.PublicError == "" {
			return errors.New("knowledge ingestion failed payload is incomplete")
		}
	case EventKnowledgeDeleteProofRecorded:
		if payload.DeleteProofID == "" || payload.ProofType == "" || payload.ProofRefHash == "" ||
			payload.ReasonClass == "" {
			return errors.New("knowledge delete proof payload is incomplete")
		}
	default:
		return errors.New("unsupported knowledge outbox event type")
	}
	return nil
}

func containsForbiddenPayloadField(payloadJSON []byte) bool {
	lowered := strings.ToLower(string(payloadJSON))
	for _, field := range []string{
		"raw_text",
		"chunk_text",
		"chunk_preview",
		"message_body",
		"source_uri",
		"object_key",
		"connector_secret",
		"credential",
		"api_key",
		"authorization",
		"token",
		"password",
		"private_key",
		"dsn",
	} {
		if strings.Contains(lowered, field) {
			return true
		}
	}
	return false
}

func knowledgeSourceCreated(payload knowledgePayload, tenantID string) *knowledgeeventsv1.KnowledgeSourceCreatedV1 {
	return &knowledgeeventsv1.KnowledgeSourceCreatedV1{
		TenantId:        firstNonEmpty(payload.TenantID, tenantID),
		SourceId:        payload.SourceID,
		SourceType:      payload.SourceType,
		SourceRefHash:   payload.SourceRefHash,
		VisibilityScope: payload.VisibilityScope,
		DataClass:       payload.DataClass,
		ContentHash:     payload.ContentHash,
		SourceVersion:   payload.SourceVersion,
	}
}

func knowledgeDocumentParsed(payload knowledgePayload, tenantID string) *knowledgeeventsv1.KnowledgeDocumentParsedV1 {
	return &knowledgeeventsv1.KnowledgeDocumentParsedV1{
		TenantId:      firstNonEmpty(payload.TenantID, tenantID),
		DocumentId:    payload.DocumentID,
		SourceId:      payload.SourceID,
		SourceVersion: payload.SourceVersion,
		DocumentHash:  payload.DocumentHash,
		ParserProfile: payload.ParserProfile,
		MimeType:      payload.MimeType,
		PageCount:     payload.PageCount,
		ChunkCount:    payload.ChunkCount,
	}
}

func knowledgeChunkReady(payload knowledgePayload, tenantID string) *knowledgeeventsv1.KnowledgeChunkReadyV1 {
	return &knowledgeeventsv1.KnowledgeChunkReadyV1{
		TenantId:        firstNonEmpty(payload.TenantID, tenantID),
		ChunkId:         payload.ChunkID,
		DocumentId:      payload.DocumentID,
		SourceId:        payload.SourceID,
		SourceVersion:   payload.SourceVersion,
		ChunkIndex:      payload.ChunkIndex,
		ChunkHash:       payload.ChunkHash,
		VisibilityScope: payload.VisibilityScope,
		DataClass:       payload.DataClass,
		PolicyVersion:   payload.PolicyVersion,
		ChunkVersion:    payload.ChunkVersion,
		TombstoneStatus: payload.TombstoneStatus,
	}
}

func knowledgeChunkTombstoned(payload knowledgePayload, tenantID string) *knowledgeeventsv1.KnowledgeChunkTombstonedV1 {
	return &knowledgeeventsv1.KnowledgeChunkTombstonedV1{
		TenantId:        firstNonEmpty(payload.TenantID, tenantID),
		ChunkId:         payload.ChunkID,
		DocumentId:      payload.DocumentID,
		SourceId:        payload.SourceID,
		ChunkHash:       payload.ChunkHash,
		DeleteProofId:   payload.DeleteProofID,
		ReasonClass:     payload.ReasonClass,
		TombstoneStatus: payload.TombstoneStatus,
	}
}

func knowledgeIngestionFailed(payload knowledgePayload, tenantID string) *knowledgeeventsv1.KnowledgeIngestionFailedV1 {
	return &knowledgeeventsv1.KnowledgeIngestionFailedV1{
		TenantId:      firstNonEmpty(payload.TenantID, tenantID),
		JobId:         payload.JobID,
		SourceId:      payload.SourceID,
		SourceVersion: payload.SourceVersion,
		FailureClass:  payload.FailureClass,
		PublicError:   payload.PublicError,
	}
}

func knowledgeDeleteProofRecorded(payload knowledgePayload, tenantID string) *knowledgeeventsv1.KnowledgeDeleteProofRecordedV1 {
	return &knowledgeeventsv1.KnowledgeDeleteProofRecordedV1{
		TenantId:      firstNonEmpty(payload.TenantID, tenantID),
		DeleteProofId: payload.DeleteProofID,
		SourceId:      payload.SourceID,
		ProofType:     payload.ProofType,
		ProofRefHash:  payload.ProofRefHash,
		ReasonClass:   payload.ReasonClass,
	}
}

func aggregateTypeForEvent(eventType string) string {
	switch eventType {
	case EventKnowledgeSourceCreated:
		return "knowledge_source"
	case EventKnowledgeDocumentParsed:
		return "knowledge_document"
	case EventKnowledgeChunkReady, EventKnowledgeChunkTombstoned:
		return "knowledge_chunk"
	case EventKnowledgeIngestionFailed:
		return "knowledge_ingestion_job"
	case EventKnowledgeDeleteProofRecorded:
		return "knowledge_delete_proof"
	default:
		return "knowledge"
	}
}

func normalizeConfig(config Config) Config {
	if config.Topic == "" {
		config.Topic = TopicKnowledgeEvents
	}
	if config.BatchSize <= 0 {
		config.BatchSize = 500
	}
	if config.PollInterval <= 0 {
		config.PollInterval = time.Second
	}
	if config.MaxAttempts <= 0 {
		config.MaxAttempts = 5
	}
	if config.RetryBaseDelay <= 0 {
		config.RetryBaseDelay = time.Second
	}
	if config.ErrorBackoff <= 0 {
		config.ErrorBackoff = time.Second
	}
	return config
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func (relay *Relay) recordError() {
	relay.metrics.totalErrors.Add(1)
	relay.metrics.consecutiveErrors.Add(1)
	relay.metrics.lastErrorAtMS.Store(time.Now().UnixMilli())
}

func (relay *Relay) recordSuccess(stats types.OutboxRelayStats) {
	relay.metrics.consecutiveErrors.Store(0)
	now := time.Now().UnixMilli()
	relay.metrics.lastSuccessAtMS.Store(now)
	if stats.Published > 0 {
		relay.metrics.lastPublishedAtMS.Store(now)
	}
}

func waitForInterval(ctx context.Context, interval time.Duration) error {
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
