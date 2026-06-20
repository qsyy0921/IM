package processing

import (
	"context"
	"errors"
	"time"

	"github.com/qsyy0921/IM/services/media-service/internal/types"
)

type Store interface {
	ClaimProcessingJobs(ctx context.Context, limit int) ([]types.ProcessingJob, error)
	MarkProcessingJobSucceeded(ctx context.Context, job types.ProcessingJob) (types.MediaAsset, error)
	MarkProcessingJobFailed(ctx context.Context, job types.ProcessingJob, cause error, maxAttempts int, retryDelay time.Duration) (bool, error)
}

type Scanner interface {
	Scan(ctx context.Context, asset types.MediaAsset) error
}

type Thumbnailer interface {
	GenerateThumbnail(ctx context.Context, asset types.MediaAsset) error
}

type Transcoder interface {
	Transcode(ctx context.Context, asset types.MediaAsset) error
}

type Worker struct {
	store      Store
	scanner    Scanner
	thumbnail  Thumbnailer
	transcoder Transcoder
	config     Config
}

type Config struct {
	BatchSize      int
	PollInterval   time.Duration
	MaxAttempts    int
	RetryBaseDelay time.Duration
	ErrorBackoff   time.Duration
	Logf           func(format string, args ...any)
}

func NewWorker(store Store, scanner Scanner, thumbnail Thumbnailer, transcoder Transcoder, config Config) *Worker {
	return &Worker{
		store:      store,
		scanner:    scanner,
		thumbnail:  thumbnail,
		transcoder: transcoder,
		config:     normalizeConfig(config),
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
				worker.config.Logf("media-service processing worker retrying after runtime error: %v", err)
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

func (worker *Worker) RunOnce(ctx context.Context) (types.ProcessingWorkerStats, error) {
	if worker == nil || worker.store == nil {
		return types.ProcessingWorkerStats{}, errors.New("media processing worker store is not configured")
	}
	jobs, err := worker.store.ClaimProcessingJobs(ctx, worker.config.BatchSize)
	if err != nil {
		return types.ProcessingWorkerStats{}, err
	}
	stats := types.ProcessingWorkerStats{Claimed: len(jobs)}
	for _, job := range jobs {
		if err := worker.processJob(ctx, job); err != nil {
			deadLettered, markErr := worker.store.MarkProcessingJobFailed(
				ctx,
				job,
				err,
				worker.config.MaxAttempts,
				worker.config.RetryBaseDelay,
			)
			if markErr != nil {
				return stats, markErr
			}
			if deadLettered {
				stats.DeadLettered++
			} else {
				stats.Retried++
			}
			continue
		}
		if _, err := worker.store.MarkProcessingJobSucceeded(ctx, job); err != nil {
			return stats, err
		}
		stats.Succeeded++
	}
	return stats, nil
}

func (worker *Worker) processJob(ctx context.Context, job types.ProcessingJob) error {
	switch job.JobType {
	case types.ProcessingJobTypeScan:
		if worker.scanner == nil {
			return errors.New("scanner is not configured")
		}
		return worker.scanner.Scan(ctx, job.Asset)
	case types.ProcessingJobTypeThumbnail:
		if worker.thumbnail == nil {
			return errors.New("thumbnailer is not configured")
		}
		return worker.thumbnail.GenerateThumbnail(ctx, job.Asset)
	case types.ProcessingJobTypeTranscode:
		if worker.transcoder == nil {
			return errors.New("transcoder is not configured")
		}
		return worker.transcoder.Transcode(ctx, job.Asset)
	default:
		return errors.New("unsupported processing job type")
	}
}

func normalizeConfig(config Config) Config {
	if config.BatchSize <= 0 {
		config.BatchSize = 50
	}
	if config.PollInterval <= 0 {
		config.PollInterval = time.Second
	}
	if config.MaxAttempts <= 0 {
		config.MaxAttempts = 3
	}
	if config.RetryBaseDelay <= 0 {
		config.RetryBaseDelay = time.Second
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
