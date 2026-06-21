package rpc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	knowledgev1 "github.com/qsyy0921/IM/api/proto/nexusim/knowledge/v1"
	"github.com/qsyy0921/IM/services/vector-index-service/internal/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

type KnowledgeChunkTaskSource struct {
	client    knowledgev1.KnowledgeIngestionServiceClient
	config    KnowledgeChunkSourceConfig
	timeout   time.Duration
	mu        sync.Mutex
	pageToken string
	drained   bool
	processed map[string]struct{}
}

type KnowledgeChunkResolver struct {
	client  knowledgev1.KnowledgeIngestionServiceClient
	config  KnowledgeChunkResolverConfig
	timeout time.Duration
}

type KnowledgeChunkSourceConfig struct {
	TenantID          string
	SourceID          string
	DocumentID        string
	PageSize          int
	EmbeddingModelRef string
	Dimension         int
	VisibilityVersion int64
	TraceID           string
}

type KnowledgeChunkResolverConfig struct {
	PageSize          int
	EmbeddingModelRef string
	Dimension         int
	VisibilityVersion int64
	TraceID           string
}

func NewKnowledgeChunkTaskSource(
	client knowledgev1.KnowledgeIngestionServiceClient,
	config KnowledgeChunkSourceConfig,
	timeout time.Duration,
) (*KnowledgeChunkTaskSource, error) {
	if client == nil {
		return nil, errors.New("knowledge-ingestion client is required")
	}
	config = normalizeKnowledgeChunkSourceConfig(config)
	if err := validateKnowledgeChunkSourceConfig(config); err != nil {
		return nil, err
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &KnowledgeChunkTaskSource{
		client:    client,
		config:    config,
		timeout:   timeout,
		processed: map[string]struct{}{},
	}, nil
}

func DialKnowledgeChunkTaskSource(
	_ context.Context,
	addr string,
	config KnowledgeChunkSourceConfig,
	timeout time.Duration,
) (*KnowledgeChunkTaskSource, func() error, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return nil, nil, errors.New("knowledge-ingestion-service address is required")
	}
	conn, err := grpc.NewClient(
		"passthrough:///"+addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, nil, err
	}
	source, err := NewKnowledgeChunkTaskSource(knowledgev1.NewKnowledgeIngestionServiceClient(conn), config, timeout)
	if err != nil {
		_ = conn.Close()
		return nil, nil, err
	}
	return source, conn.Close, nil
}

func NewKnowledgeChunkResolver(
	client knowledgev1.KnowledgeIngestionServiceClient,
	config KnowledgeChunkResolverConfig,
	timeout time.Duration,
) (*KnowledgeChunkResolver, error) {
	if client == nil {
		return nil, errors.New("knowledge-ingestion client is required")
	}
	config = normalizeKnowledgeChunkResolverConfig(config)
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &KnowledgeChunkResolver{client: client, config: config, timeout: timeout}, nil
}

func DialKnowledgeChunkResolver(
	_ context.Context,
	addr string,
	config KnowledgeChunkResolverConfig,
	timeout time.Duration,
) (*KnowledgeChunkResolver, func() error, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return nil, nil, errors.New("knowledge-ingestion-service address is required")
	}
	conn, err := grpc.NewClient(
		"passthrough:///"+addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, nil, err
	}
	resolver, err := NewKnowledgeChunkResolver(knowledgev1.NewKnowledgeIngestionServiceClient(conn), config, timeout)
	if err != nil {
		_ = conn.Close()
		return nil, nil, err
	}
	return resolver, conn.Close, nil
}

func (source *KnowledgeChunkTaskSource) ClaimEmbeddingTasks(
	ctx context.Context,
	limit int,
) ([]types.VectorEmbeddingTask, error) {
	source.mu.Lock()
	defer source.mu.Unlock()
	if source.drained {
		return nil, nil
	}
	if limit <= 0 {
		limit = source.config.PageSize
	}
	tasks := make([]types.VectorEmbeddingTask, 0, limit)
	for len(tasks) < limit && !source.drained {
		pageSize := source.config.PageSize
		remaining := limit - len(tasks)
		if remaining < pageSize {
			pageSize = remaining
		}
		callCtx, cancel := context.WithTimeout(ctx, source.timeout)
		response, err := source.client.ListKnowledgeChunks(callCtx, &knowledgev1.ListKnowledgeChunksRequest{
			AuthContext: &knowledgev1.AuthContext{
				TenantId:    source.config.TenantID,
				ServiceName: types.AllowedCallerVectorIndex,
				InstanceRef: "vector-embedding-worker",
				TraceId:     source.config.TraceID,
				RequestId:   "vector-embedding-list-chunks",
			},
			SourceId:   source.config.SourceID,
			DocumentId: source.config.DocumentID,
			PageSize:   int32(pageSize),
			PageToken:  source.pageToken,
		})
		cancel()
		if err != nil {
			return nil, mapKnowledgeError(err)
		}
		for _, chunk := range response.GetChunks() {
			task, ok := vectorEmbeddingTaskFromKnowledgeChunk(chunk, knowledgeChunkTaskConfig{
				TenantID:          source.config.TenantID,
				EmbeddingModelRef: source.config.EmbeddingModelRef,
				Dimension:         source.config.Dimension,
				VisibilityVersion: source.config.VisibilityVersion,
				TraceID:           source.config.TraceID,
				InstanceRef:       "vector-embedding-worker",
			})
			if !ok {
				continue
			}
			if _, seen := source.processed[task.IdempotencyKey]; seen {
				continue
			}
			tasks = append(tasks, task)
			if len(tasks) >= limit {
				break
			}
		}
		source.pageToken = strings.TrimSpace(response.GetNextPageToken())
		if source.pageToken == "" {
			source.drained = true
		}
		if len(response.GetChunks()) == 0 {
			break
		}
	}
	return tasks, nil
}

func (source *KnowledgeChunkTaskSource) CompleteEmbeddingTask(
	_ context.Context,
	task types.VectorEmbeddingTask,
) error {
	source.mu.Lock()
	defer source.mu.Unlock()
	source.processed[strings.TrimSpace(task.IdempotencyKey)] = struct{}{}
	return nil
}

func (resolver *KnowledgeChunkResolver) ResolveKnowledgeChunkTask(
	ctx context.Context,
	event types.KnowledgeChunkReadyEvent,
) (types.VectorEmbeddingTask, error) {
	if resolver == nil || resolver.client == nil {
		return types.VectorEmbeddingTask{}, types.NewUnavailable("knowledge chunk resolver is not configured")
	}
	event = event.Normalized()
	if err := event.Validate(); err != nil {
		return types.VectorEmbeddingTask{}, err
	}
	pageToken := ""
	for {
		callCtx, cancel := context.WithTimeout(ctx, resolver.timeout)
		response, err := resolver.client.ListKnowledgeChunks(callCtx, &knowledgev1.ListKnowledgeChunksRequest{
			AuthContext: &knowledgev1.AuthContext{
				TenantId:    string(event.TenantID),
				ServiceName: types.AllowedCallerVectorIndex,
				InstanceRef: "vector-chunk-consumer",
				TraceId:     firstNonEmpty(event.TraceID, resolver.config.TraceID),
				RequestId:   "vector-chunk-consumer-" + event.ChunkID,
			},
			SourceId:   event.SourceID,
			DocumentId: event.DocumentID,
			PageSize:   int32(resolver.config.PageSize),
			PageToken:  pageToken,
		})
		cancel()
		if err != nil {
			return types.VectorEmbeddingTask{}, mapKnowledgeError(err)
		}
		for _, chunk := range response.GetChunks() {
			if strings.TrimSpace(chunk.GetChunkId()) != event.ChunkID {
				continue
			}
			if strings.TrimSpace(chunk.GetChunkHash()) != event.ChunkHash {
				return types.VectorEmbeddingTask{}, types.NewFailedPrecondition("knowledge chunk hash mismatch")
			}
			task, ok := vectorEmbeddingTaskFromKnowledgeChunk(chunk, knowledgeChunkTaskConfig{
				TenantID:          string(event.TenantID),
				EmbeddingModelRef: resolver.config.EmbeddingModelRef,
				Dimension:         resolver.config.Dimension,
				VisibilityVersion: resolver.config.VisibilityVersion,
				TraceID:           firstNonEmpty(event.TraceID, resolver.config.TraceID),
				InstanceRef:       "vector-chunk-consumer",
			})
			if !ok {
				return types.VectorEmbeddingTask{}, types.NewFailedPrecondition("knowledge chunk preview is not available")
			}
			task.CorrelationID = firstNonEmpty(event.CorrelationID, task.CorrelationID)
			task.CausationID = firstNonEmpty(event.EventID, event.CausationID, task.CausationID)
			task.TraceID = firstNonEmpty(event.TraceID, task.TraceID)
			return task.Normalized(), nil
		}
		pageToken = strings.TrimSpace(response.GetNextPageToken())
		if pageToken == "" {
			break
		}
	}
	return types.VectorEmbeddingTask{}, types.NewNotFound("knowledge chunk not found")
}

type knowledgeChunkTaskConfig struct {
	TenantID          string
	EmbeddingModelRef string
	Dimension         int
	VisibilityVersion int64
	TraceID           string
	InstanceRef       string
}

func vectorEmbeddingTaskFromKnowledgeChunk(
	chunk *knowledgev1.KnowledgeChunk,
	config knowledgeChunkTaskConfig,
) (types.VectorEmbeddingTask, bool) {
	if chunk == nil || strings.TrimSpace(chunk.GetChunkPreviewRedacted()) == "" {
		return types.VectorEmbeddingTask{}, false
	}
	sourceID := strings.TrimSpace(chunk.GetSourceId())
	documentID := strings.TrimSpace(chunk.GetDocumentId())
	chunkID := strings.TrimSpace(chunk.GetChunkId())
	taskID := sourceID + ":" + documentID + ":" + chunkID
	sourceVersion := sourceVersionNumber(chunk.GetSourceVersion(), chunk.GetUpdatedAtUnixMs(), int64(chunk.GetChunkIndex()+1))
	preview := strings.TrimSpace(chunk.GetChunkPreviewRedacted())
	return types.VectorEmbeddingTask{
		AuthContext: types.AuthContext{
			TenantID:    types.TenantID(config.TenantID),
			ServiceName: types.AllowedCallerVectorIndex,
			InstanceRef: firstNonEmpty(config.InstanceRef, "vector-embedding-worker"),
			TraceID:     firstNonEmpty(config.TraceID, "trace-vector-embedding"),
			RequestID:   "vector-embedding-" + chunkID,
		},
		SourceService:      types.AllowedCallerKnowledgeIngestion,
		CollectionType:     types.CollectionTypeKnowledgeChunk,
		SourceRefHash:      sha256Ref(taskID),
		SourceID:           taskID,
		SourceVersion:      sourceVersion,
		SourceHash:         sha256Ref(sourceID + ":" + documentID + ":" + chunk.GetSourceVersion()),
		ChunkHash:          chunk.GetChunkHash(),
		InputText:          preview,
		InputHash:          sha256Ref(preview),
		InputSchemaVersion: 1,
		EmbeddingModelRef:  config.EmbeddingModelRef,
		Dimension:          config.Dimension,
		VisibilityScope:    chunk.GetVisibilityScope(),
		VisibilityVersion:  config.VisibilityVersion,
		PolicyVersion:      chunk.GetPolicyVersion(),
		DataClass:          chunk.GetDataClass(),
		DeleteProofID:      chunk.GetDeleteProofId(),
		IdempotencyKey:     "knowledge-chunk:" + chunkID + ":" + config.EmbeddingModelRef,
		CorrelationID:      "knowledge-chunk:" + chunkID,
		CausationID:        chunkID,
		TraceID:            config.TraceID,
	}, true
}

func normalizeKnowledgeChunkSourceConfig(config KnowledgeChunkSourceConfig) KnowledgeChunkSourceConfig {
	config.TenantID = strings.TrimSpace(config.TenantID)
	config.SourceID = strings.TrimSpace(config.SourceID)
	config.DocumentID = strings.TrimSpace(config.DocumentID)
	config.EmbeddingModelRef = strings.TrimSpace(config.EmbeddingModelRef)
	if config.EmbeddingModelRef == "" {
		config.EmbeddingModelRef = "deterministic-embedding-v1"
	}
	if config.Dimension <= 0 {
		config.Dimension = 8
	}
	if config.PageSize <= 0 {
		config.PageSize = 50
	}
	if config.VisibilityVersion <= 0 {
		config.VisibilityVersion = 1
	}
	config.TraceID = strings.TrimSpace(config.TraceID)
	return config
}

func normalizeKnowledgeChunkResolverConfig(config KnowledgeChunkResolverConfig) KnowledgeChunkResolverConfig {
	config.EmbeddingModelRef = strings.TrimSpace(config.EmbeddingModelRef)
	if config.EmbeddingModelRef == "" {
		config.EmbeddingModelRef = "deterministic-embedding-v1"
	}
	if config.Dimension <= 0 {
		config.Dimension = 8
	}
	if config.PageSize <= 0 {
		config.PageSize = 50
	}
	if config.VisibilityVersion <= 0 {
		config.VisibilityVersion = 1
	}
	config.TraceID = strings.TrimSpace(config.TraceID)
	return config
}

func validateKnowledgeChunkSourceConfig(config KnowledgeChunkSourceConfig) error {
	if config.TenantID == "" {
		return errors.New("knowledge chunk task source tenant id is required")
	}
	if config.SourceID == "" && config.DocumentID == "" {
		return errors.New("knowledge chunk task source requires source id or document id")
	}
	if config.EmbeddingModelRef == "" || config.Dimension <= 0 {
		return errors.New("knowledge chunk task source embedding model and dimension are required")
	}
	return nil
}

func mapKnowledgeError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return types.NewUnavailable("knowledge-ingestion temporarily unavailable")
	}
	st, ok := status.FromError(err)
	if !ok {
		return types.NewUnavailable("knowledge-ingestion temporarily unavailable")
	}
	switch st.Code() {
	case codes.InvalidArgument:
		return types.NewInvalidArgument("knowledge chunk request invalid")
	case codes.PermissionDenied:
		return types.NewPermissionDenied("knowledge chunk permission denied")
	case codes.NotFound:
		return types.NewNotFound("knowledge chunk source not found")
	case codes.FailedPrecondition:
		return types.NewFailedPrecondition("knowledge chunk precondition failed")
	case codes.Unavailable, codes.DeadlineExceeded:
		return types.NewUnavailable("knowledge-ingestion temporarily unavailable")
	default:
		return types.NewUnavailable("knowledge-ingestion temporarily unavailable")
	}
}

func sha256Ref(value string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func sourceVersionNumber(value string, updatedAt int64, fallback int64) int64 {
	value = strings.TrimSpace(value)
	if parsed, err := strconv.ParseInt(value, 10, 64); err == nil && parsed > 0 {
		return parsed
	}
	if updatedAt > 0 {
		return updatedAt
	}
	if fallback > 0 {
		return fallback
	}
	return 1
}
