package embedding

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/qsyy0921/IM/services/vector-index-service/internal/types"
)

type TaskSource interface {
	ClaimEmbeddingTasks(ctx context.Context, limit int) ([]types.VectorEmbeddingTask, error)
	CompleteEmbeddingTask(ctx context.Context, task types.VectorEmbeddingTask) error
}

type Embedder interface {
	Embed(ctx context.Context, task types.VectorEmbeddingTask) (types.VectorEmbeddingResult, error)
}

type Upserter interface {
	UpsertVectorItem(ctx context.Context, command types.UpsertVectorItemCommand) error
}

type Worker struct {
	source   TaskSource
	embedder Embedder
	upserter Upserter
	config   Config
}

type Config struct {
	BatchSize    int
	PollInterval time.Duration
	ErrorBackoff time.Duration
	Logf         func(format string, args ...any)
}

func NewWorker(source TaskSource, embedder Embedder, upserter Upserter, config Config) *Worker {
	return &Worker{
		source:   source,
		embedder: embedder,
		upserter: upserter,
		config:   normalizeConfig(config),
	}
}

func (worker *Worker) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		stats, err := worker.RunOnce(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return context.Canceled
			}
			if worker.config.Logf != nil {
				worker.config.Logf("vector-index-service embedding worker retrying after runtime error: %v", err)
			}
			if err := waitForInterval(ctx, worker.config.ErrorBackoff); err != nil {
				return err
			}
			continue
		}
		if stats.Claimed > 0 {
			continue
		}
		if err := waitForInterval(ctx, worker.config.PollInterval); err != nil {
			return err
		}
	}
}

func (worker *Worker) RunOnce(ctx context.Context) (types.EmbeddingWorkerStats, error) {
	if worker == nil || worker.source == nil || worker.embedder == nil || worker.upserter == nil {
		return types.EmbeddingWorkerStats{}, errors.New("vector embedding worker dependencies are not configured")
	}
	tasks, err := worker.source.ClaimEmbeddingTasks(ctx, worker.config.BatchSize)
	if err != nil {
		return types.EmbeddingWorkerStats{}, err
	}
	stats := types.EmbeddingWorkerStats{Claimed: len(tasks)}
	for _, task := range tasks {
		normalized := task.Normalized()
		if err := normalized.Validate(); err != nil {
			return stats, err
		}
		if expected := sha256Ref(normalized.InputText); expected != normalized.InputHash {
			return stats, types.NewInvalidArgument("embedding input hash mismatch")
		}
		result, err := worker.embedder.Embed(ctx, normalized)
		if err != nil {
			return stats, err
		}
		stats.Embedded++
		command := upsertCommandFromEmbedding(normalized, result)
		if err := worker.upserter.UpsertVectorItem(ctx, command); err != nil {
			return stats, err
		}
		stats.Upserted++
		if err := worker.source.CompleteEmbeddingTask(ctx, normalized); err != nil {
			return stats, err
		}
		stats.Completed++
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

func normalizeConfig(config Config) Config {
	if config.BatchSize <= 0 {
		config.BatchSize = 50
	}
	if config.PollInterval <= 0 {
		config.PollInterval = time.Second
	}
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
