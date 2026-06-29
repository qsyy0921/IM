package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	kafkainfra "github.com/qsyy0921/IM/services/delivery-service/internal/infrastructure/kafka"
	"github.com/qsyy0921/IM/services/delivery-service/internal/trigger/timeline"
	"github.com/qsyy0921/IM/services/delivery-service/internal/types"
)

type timelineProjectionRuntime struct {
	workers []*timeline.Worker
}

func newTimelineProjectionRuntime(
	workerCount int,
	readerConfig kafkainfra.ReaderConfig,
	projector timeline.Projector,
	consumerGroup string,
	recorder timeline.FailureRecorder,
	config timeline.Config,
) (*timelineProjectionRuntime, []*kafkainfra.ReaderConsumer, error) {
	if workerCount <= 0 {
		return nil, nil, errors.New("delivery timeline consumer worker count must be positive")
	}
	consumers := make([]*kafkainfra.ReaderConsumer, 0, workerCount)
	workers := make([]*timeline.Worker, 0, workerCount)
	for index := 0; index < workerCount; index++ {
		consumer, err := kafkainfra.NewReaderConsumer(readerConfig)
		if err != nil {
			closeTimelineConsumers(consumers)
			return nil, nil, fmt.Errorf("create delivery timeline consumer worker %d: %w", index, err)
		}
		consumers = append(consumers, consumer)
		workers = append(workers, timeline.NewWorker(
			consumer,
			projector,
			consumerGroup,
			recorder,
			config,
		))
	}
	return &timelineProjectionRuntime{workers: workers}, consumers, nil
}

func (runtime *timelineProjectionRuntime) Run(ctx context.Context) error {
	if runtime == nil || len(runtime.workers) == 0 {
		return errors.New("delivery timeline projection runtime has no workers")
	}
	if len(runtime.workers) == 1 {
		return runtime.workers[0].Run(ctx)
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	errCh := make(chan error, len(runtime.workers))
	for _, worker := range runtime.workers {
		go func(worker *timeline.Worker) {
			errCh <- worker.Run(runCtx)
		}(worker)
	}
	var firstErr error
	for range runtime.workers {
		err := <-errCh
		if err == nil || errors.Is(err, context.Canceled) {
			continue
		}
		if firstErr == nil {
			firstErr = err
			cancel()
		}
	}
	if firstErr != nil {
		return firstErr
	}
	return context.Canceled
}

func (runtime *timelineProjectionRuntime) Snapshot() types.ProjectionWorkerSnapshot {
	if runtime == nil {
		return types.ProjectionWorkerSnapshot{}
	}
	var snapshot types.ProjectionWorkerSnapshot
	for _, worker := range runtime.workers {
		if worker == nil {
			continue
		}
		workerSnapshot := worker.Snapshot()
		snapshot.TotalErrors += workerSnapshot.TotalErrors
		if workerSnapshot.ConsecutiveErrors > snapshot.ConsecutiveErrors {
			snapshot.ConsecutiveErrors = workerSnapshot.ConsecutiveErrors
		}
		if workerSnapshot.LastErrorAtMS > snapshot.LastErrorAtMS {
			snapshot.LastErrorAtMS = workerSnapshot.LastErrorAtMS
		}
		if workerSnapshot.LastSuccessAtMS > snapshot.LastSuccessAtMS {
			snapshot.LastSuccessAtMS = workerSnapshot.LastSuccessAtMS
		}
		if workerSnapshot.LastCommitAtMS > snapshot.LastCommitAtMS {
			snapshot.LastCommitAtMS = workerSnapshot.LastCommitAtMS
		}
		if workerSnapshot.LastErrorBackoffMS > snapshot.LastErrorBackoffMS {
			snapshot.LastErrorBackoffMS = workerSnapshot.LastErrorBackoffMS
		}
	}
	return snapshot
}

func deliveryTimelineConsumerWorkerCountFromEnv() (int, error) {
	raw := strings.TrimSpace(os.Getenv("NEXUSIM_DELIVERY_TIMELINE_CONSUMER_WORKERS"))
	if raw == "" {
		return 1, nil
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed <= 0 {
		return 0, errors.New("NEXUSIM_DELIVERY_TIMELINE_CONSUMER_WORKERS must be a positive integer")
	}
	return parsed, nil
}

func closeTimelineConsumers(consumers []*kafkainfra.ReaderConsumer) {
	for _, consumer := range consumers {
		if err := consumer.Close(); err != nil {
			log.Printf("delivery-service timeline consumer close failed: %v", err)
		}
	}
}
