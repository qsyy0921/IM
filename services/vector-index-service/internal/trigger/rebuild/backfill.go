package rebuild

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	"github.com/qsyy0921/IM/services/vector-index-service/internal/types"
)

type EmbeddingTaskLister interface {
	ListCompletedEmbeddingTasks(ctx context.Context, task types.VectorRebuildTask, limit int) ([]types.VectorEmbeddingTask, error)
}

type Embedder interface {
	Embed(ctx context.Context, task types.VectorEmbeddingTask) (types.VectorEmbeddingResult, error)
}

type Upserter interface {
	UpsertVectorItem(ctx context.Context, command types.UpsertVectorItemCommand) (types.VectorItem, bool, error)
}

type VectorBackend interface {
	UpsertEmbedding(ctx context.Context, task types.VectorEmbeddingTask, result types.VectorEmbeddingResult, item types.VectorItem) error
}

type EmbeddingTaskBackfiller struct {
	lister   EmbeddingTaskLister
	embedder Embedder
	upserter Upserter
	backend  VectorBackend
	config   EmbeddingTaskBackfillerConfig
}

type EmbeddingTaskBackfillerConfig struct {
	BatchSize int
}

func NewEmbeddingTaskBackfiller(
	lister EmbeddingTaskLister,
	embedder Embedder,
	upserter Upserter,
	backend VectorBackend,
	config EmbeddingTaskBackfillerConfig,
) EmbeddingTaskBackfiller {
	if config.BatchSize <= 0 {
		config.BatchSize = 100
	}
	return EmbeddingTaskBackfiller{
		lister:   lister,
		embedder: embedder,
		upserter: upserter,
		backend:  backend,
		config:   config,
	}
}

func (backfiller EmbeddingTaskBackfiller) BackfillRebuildTask(
	ctx context.Context,
	task types.VectorRebuildTask,
) (types.RebuildBackfillStats, error) {
	if backfiller.lister == nil || backfiller.embedder == nil || backfiller.upserter == nil || backfiller.backend == nil {
		return types.RebuildBackfillStats{}, types.NewUnavailable("vector rebuild backfill dependencies are not configured")
	}
	limit := backfiller.config.BatchSize
	if limit <= 0 {
		limit = 100
	}
	tasks, err := backfiller.lister.ListCompletedEmbeddingTasks(ctx, task, limit+1)
	if err != nil {
		return types.RebuildBackfillStats{}, err
	}
	if len(tasks) > limit {
		return types.RebuildBackfillStats{}, types.NewFailedPrecondition("vector rebuild backfill limit exceeded")
	}
	stats := types.RebuildBackfillStats{Backfilled: len(tasks)}
	for _, embeddingTask := range tasks {
		normalized := embeddingTask.Normalized()
		if err := normalized.Validate(); err != nil {
			return stats, err
		}
		if expected := sha256Ref(normalized.InputText); expected != normalized.InputHash {
			return stats, types.NewInvalidArgument("embedding input hash mismatch")
		}
		result, err := backfiller.embedder.Embed(ctx, normalized)
		if err != nil {
			return stats, err
		}
		stats.Embedded++
		item, _, err := backfiller.upserter.UpsertVectorItem(ctx, upsertCommandFromEmbedding(normalized, result))
		if err != nil {
			return stats, err
		}
		if err := backfiller.backend.UpsertEmbedding(ctx, normalized, result, item); err != nil {
			return stats, err
		}
		stats.Upserted++
	}
	return stats, nil
}

func upsertCommandFromEmbedding(task types.VectorEmbeddingTask, result types.VectorEmbeddingResult) types.UpsertVectorItemCommand {
	modelRef := result.ModelID
	if modelRef == "" {
		modelRef = task.EmbeddingModelRef
	}
	dimension := result.Dimension
	if dimension <= 0 {
		dimension = task.Dimension
	}
	return types.UpsertVectorItemCommand{
		AuthContext:         task.AuthContext,
		SourceService:       task.SourceService,
		CollectionType:      task.CollectionType,
		SourceRefHash:       task.SourceRefHash,
		SourceID:            task.SourceID,
		SourceVersion:       task.SourceVersion,
		SourceHash:          task.SourceHash,
		ChunkHash:           task.ChunkHash,
		EmbeddingModelRef:   modelRef,
		EmbeddingVectorHash: result.EmbeddingVectorHash,
		Dimension:           dimension,
		VisibilityScope:     task.VisibilityScope,
		VisibilityVersion:   task.VisibilityVersion,
		PolicyVersion:       task.PolicyVersion,
		DataClass:           task.DataClass,
		DeleteProofID:       task.DeleteProofID,
		RetentionPolicyRef:  task.RetentionPolicyRef,
		IdempotencyKey:      task.IdempotencyKey,
		CorrelationID:       task.CorrelationID,
		CausationID:         firstNonEmpty(task.CausationID, result.InvocationID),
		TraceID:             task.TraceID,
	}
}

func sha256Ref(value string) string {
	sum := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
