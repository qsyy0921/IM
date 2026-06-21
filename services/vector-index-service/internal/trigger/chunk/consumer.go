package chunk

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	knowledgeeventsv1 "github.com/qsyy0921/IM/schemas/kafka/knowledge/v1"
	"github.com/qsyy0921/IM/services/vector-index-service/internal/types"
	"google.golang.org/protobuf/proto"
)

const (
	TopicKnowledgeEvents              = "im.knowledge.events"
	EventKnowledgeSourceCreated       = "knowledge.source.created.v1"
	EventKnowledgeDocumentParsed      = "knowledge.document.parsed.v1"
	EventKnowledgeChunkReady          = "knowledge.chunk.ready.v1"
	EventKnowledgeChunkTombstone      = "knowledge.chunk.tombstoned.v1"
	EventKnowledgeIngestionFailed     = "knowledge.ingestion.failed.v1"
	EventKnowledgeDeleteProofRecorded = "knowledge.delete_proof.recorded.v1"
)

var errIgnoredKnowledgeEvent = errors.New("ignored knowledge event")

type Consumer interface {
	Fetch(context.Context) (types.ChunkEventMessage, error)
	Commit(context.Context, types.ChunkEventMessage) error
}

type ChunkTaskResolver interface {
	ResolveKnowledgeChunkTask(context.Context, types.KnowledgeChunkReadyEvent) (types.VectorEmbeddingTask, error)
}

type TaskQueue interface {
	EnqueueEmbeddingTask(ctx context.Context, task types.VectorEmbeddingTask) (bool, error)
}

type Worker struct {
	consumer Consumer
	resolver ChunkTaskResolver
	queue    TaskQueue
	config   Config
}

type Config struct {
	ErrorBackoff time.Duration
	Logf         func(format string, args ...any)
}

func NewWorker(consumer Consumer, resolver ChunkTaskResolver, queue TaskQueue, config Config) *Worker {
	return &Worker{
		consumer: consumer,
		resolver: resolver,
		queue:    queue,
		config:   normalizeConfig(config),
	}
}

func (worker *Worker) Run(ctx context.Context) error {
	for {
		err := worker.RunOnce(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return context.Canceled
			}
			if isPermanentError(err) {
				return err
			}
			if worker.config.Logf != nil {
				worker.config.Logf("vector-index-service chunk consumer retrying after error: %v", err)
			}
			if err := waitForInterval(ctx, worker.config.ErrorBackoff); err != nil {
				return err
			}
			continue
		}
	}
}

func (worker *Worker) RunOnce(ctx context.Context) error {
	if worker == nil || worker.consumer == nil || worker.resolver == nil || worker.queue == nil {
		return errors.New("vector chunk consumer dependencies are not configured")
	}
	message, err := worker.consumer.Fetch(ctx)
	if err != nil {
		return err
	}
	event, err := DecodeKnowledgeChunkReady(message)
	if err != nil {
		if errors.Is(err, errIgnoredKnowledgeEvent) {
			return worker.consumer.Commit(ctx, message)
		}
		return err
	}
	task, err := worker.resolver.ResolveKnowledgeChunkTask(ctx, event)
	if err != nil {
		return err
	}
	if _, err := worker.queue.EnqueueEmbeddingTask(ctx, task); err != nil {
		return err
	}
	return worker.consumer.Commit(ctx, message)
}

func DecodeKnowledgeChunkReady(message types.ChunkEventMessage) (types.KnowledgeChunkReadyEvent, error) {
	if event, ok, err := decodeKnowledgeChunkReadyProto(message); ok || err != nil {
		return event, err
	}
	eventType := strings.TrimSpace(message.EventType)
	var payload knowledgeChunkReadyPayload
	if err := json.Unmarshal(message.Value, &payload); err != nil {
		return types.KnowledgeChunkReadyEvent{}, err
	}
	if eventType == "" {
		eventType = strings.TrimSpace(payload.EventType)
	}
	if eventType != EventKnowledgeChunkReady {
		if isKnownNonChunkKnowledgeEvent(eventType) {
			return types.KnowledgeChunkReadyEvent{}, errIgnoredKnowledgeEvent
		}
		return types.KnowledgeChunkReadyEvent{}, types.NewInvalidArgument("unsupported knowledge chunk event type")
	}
	event := types.KnowledgeChunkReadyEvent{
		EventID:         firstNonEmpty(payload.EventID, payload.ID),
		TenantID:        types.TenantID(payload.TenantID),
		ChunkID:         payload.ChunkID,
		DocumentID:      payload.DocumentID,
		SourceID:        payload.SourceID,
		SourceVersion:   payload.SourceVersion,
		ChunkIndex:      payload.ChunkIndex,
		ChunkHash:       payload.ChunkHash,
		VisibilityScope: payload.VisibilityScope,
		DataClass:       payload.DataClass,
		PolicyVersion:   payload.PolicyVersion,
		ChunkVersion:    payload.ChunkVersion,
		TombstoneStatus: payload.TombstoneStatus,
		CorrelationID:   payload.CorrelationID,
		CausationID:     payload.CausationID,
		TraceID:         payload.TraceID,
	}.Normalized()
	if err := event.Validate(); err != nil {
		return types.KnowledgeChunkReadyEvent{}, err
	}
	return event, nil
}

func decodeKnowledgeChunkReadyProto(message types.ChunkEventMessage) (types.KnowledgeChunkReadyEvent, bool, error) {
	var envelope knowledgeeventsv1.KnowledgeEvent
	if err := proto.Unmarshal(message.Value, &envelope); err != nil {
		return types.KnowledgeChunkReadyEvent{}, false, nil
	}
	if envelope.GetEventId() == "" && envelope.GetEventType() == "" && envelope.GetPayload() == nil {
		return types.KnowledgeChunkReadyEvent{}, false, nil
	}
	eventType := firstNonEmpty(message.EventType, envelope.GetEventType())
	if eventType != EventKnowledgeChunkReady {
		if isKnownNonChunkKnowledgeEvent(eventType) {
			return types.KnowledgeChunkReadyEvent{}, true, errIgnoredKnowledgeEvent
		}
		return types.KnowledgeChunkReadyEvent{}, true, types.NewInvalidArgument("unsupported knowledge chunk event type")
	}
	payload := envelope.GetChunkReady()
	if payload == nil {
		return types.KnowledgeChunkReadyEvent{}, true, types.NewInvalidArgument("malformed knowledge chunk event payload")
	}
	event := types.KnowledgeChunkReadyEvent{
		EventID:         envelope.GetEventId(),
		TenantID:        types.TenantID(firstNonEmpty(payload.GetTenantId(), envelope.GetTenantId())),
		ChunkID:         payload.GetChunkId(),
		DocumentID:      payload.GetDocumentId(),
		SourceID:        payload.GetSourceId(),
		SourceVersion:   payload.GetSourceVersion(),
		ChunkIndex:      int(payload.GetChunkIndex()),
		ChunkHash:       payload.GetChunkHash(),
		VisibilityScope: payload.GetVisibilityScope(),
		DataClass:       payload.GetDataClass(),
		PolicyVersion:   payload.GetPolicyVersion(),
		ChunkVersion:    payload.GetChunkVersion(),
		TombstoneStatus: payload.GetTombstoneStatus(),
		CorrelationID:   envelope.GetCorrelationId(),
		CausationID:     envelope.GetCausationId(),
		TraceID:         envelope.GetTraceId(),
	}.Normalized()
	if err := event.Validate(); err != nil {
		return types.KnowledgeChunkReadyEvent{}, true, err
	}
	return event, true, nil
}

func isKnownNonChunkKnowledgeEvent(eventType string) bool {
	switch strings.TrimSpace(eventType) {
	case EventKnowledgeSourceCreated,
		EventKnowledgeDocumentParsed,
		EventKnowledgeChunkTombstone,
		EventKnowledgeIngestionFailed,
		EventKnowledgeDeleteProofRecorded:
		return true
	default:
		return false
	}
}

type knowledgeChunkReadyPayload struct {
	EventID         string `json:"event_id"`
	ID              string `json:"id"`
	EventType       string `json:"event_type"`
	TenantID        string `json:"tenant_id"`
	ChunkID         string `json:"chunk_id"`
	DocumentID      string `json:"document_id"`
	SourceID        string `json:"source_id"`
	SourceVersion   string `json:"source_version"`
	ChunkIndex      int    `json:"chunk_index"`
	ChunkHash       string `json:"chunk_hash"`
	VisibilityScope string `json:"visibility_scope"`
	DataClass       string `json:"data_class"`
	PolicyVersion   string `json:"policy_version"`
	ChunkVersion    string `json:"chunk_version"`
	TombstoneStatus string `json:"tombstone_status"`
	CorrelationID   string `json:"correlation_id"`
	CausationID     string `json:"causation_id"`
	TraceID         string `json:"trace_id"`
}

func isPermanentError(err error) bool {
	return errors.Is(err, types.ErrInvalidArgument) ||
		errors.Is(err, types.ErrFailedPrecondition)
}

func normalizeConfig(config Config) Config {
	if config.ErrorBackoff <= 0 {
		config.ErrorBackoff = time.Second
	}
	return config
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
