package embedding

import (
	"context"
	"errors"

	"github.com/qsyy0921/IM/services/vector-index-service/internal/types"
)

type TaskQueue interface {
	EnqueueEmbeddingTask(ctx context.Context, task types.VectorEmbeddingTask) (bool, error)
}

type Producer struct {
	source TaskSource
	queue  TaskQueue
	config Config
}

func NewProducer(source TaskSource, queue TaskQueue, config Config) *Producer {
	return &Producer{
		source: source,
		queue:  queue,
		config: normalizeConfig(config),
	}
}

func (producer *Producer) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		stats, err := producer.RunOnce(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return context.Canceled
			}
			if producer.config.Logf != nil {
				producer.config.Logf("vector-index-service embedding producer retrying after runtime error: %v", err)
			}
			if err := waitForInterval(ctx, producer.config.ErrorBackoff); err != nil {
				return err
			}
			continue
		}
		if stats.Claimed > 0 {
			continue
		}
		if err := waitForInterval(ctx, producer.config.PollInterval); err != nil {
			return err
		}
	}
}

func (producer *Producer) RunOnce(ctx context.Context) (types.EmbeddingProducerStats, error) {
	if producer == nil || producer.source == nil || producer.queue == nil {
		return types.EmbeddingProducerStats{}, errors.New("vector embedding producer dependencies are not configured")
	}
	tasks, err := producer.source.ClaimEmbeddingTasks(ctx, producer.config.BatchSize)
	if err != nil {
		return types.EmbeddingProducerStats{}, err
	}
	stats := types.EmbeddingProducerStats{Claimed: len(tasks)}
	for _, task := range tasks {
		normalized := task.Normalized()
		if err := normalized.Validate(); err != nil {
			return stats, err
		}
		if expected := sha256Ref(normalized.InputText); expected != normalized.InputHash {
			return stats, types.NewInvalidArgument("embedding input hash mismatch")
		}
		replayed, err := producer.queue.EnqueueEmbeddingTask(ctx, normalized)
		if err != nil {
			return stats, err
		}
		if replayed {
			stats.Replayed++
		} else {
			stats.Enqueued++
		}
		if err := producer.source.CompleteEmbeddingTask(ctx, normalized); err != nil {
			return stats, err
		}
		stats.Completed++
	}
	return stats, nil
}
