package processing

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/media-service/internal/types"
)

func TestWorkerRunOnceProcessesJobsAndRecordsFailures(t *testing.T) {
	store := &fakeStore{
		jobs: []types.ProcessingJob{
			{TenantID: "tenant-1", JobID: "job-scan", JobType: types.ProcessingJobTypeScan},
			{TenantID: "tenant-1", JobID: "job-thumbnail", JobType: types.ProcessingJobTypeThumbnail},
			{TenantID: "tenant-1", JobID: "job-transcode", JobType: types.ProcessingJobTypeTranscode},
		},
		deadLetterOnFailure: true,
	}
	worker := NewWorker(
		store,
		fakeScanner{},
		fakeThumbnailer{err: errors.New("thumbnail provider unavailable")},
		fakeTranscoder{},
		Config{BatchSize: 10, MaxAttempts: 1, RetryBaseDelay: time.Millisecond},
	)

	stats, err := worker.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if stats.Claimed != 3 || stats.Succeeded != 2 || stats.DeadLettered != 1 || stats.Retried != 0 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	if len(store.succeeded) != 2 || len(store.failed) != 1 {
		t.Fatalf("unexpected store calls: succeeded=%d failed=%d", len(store.succeeded), len(store.failed))
	}
}

func TestWorkerRunOnceFailsWhenProcessorMissing(t *testing.T) {
	store := &fakeStore{
		jobs: []types.ProcessingJob{{TenantID: "tenant-1", JobID: "job-scan", JobType: types.ProcessingJobTypeScan}},
	}
	worker := NewWorker(store, nil, fakeThumbnailer{}, fakeTranscoder{}, Config{MaxAttempts: 3})

	stats, err := worker.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if stats.Claimed != 1 || stats.Retried != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	if len(store.failed) != 1 {
		t.Fatalf("expected failed job to be recorded")
	}
}

type fakeStore struct {
	jobs                []types.ProcessingJob
	succeeded           []types.ProcessingJob
	failed              []types.ProcessingJob
	deadLetterOnFailure bool
}

func (store *fakeStore) ClaimProcessingJobs(context.Context, int) ([]types.ProcessingJob, error) {
	return store.jobs, nil
}

func (store *fakeStore) MarkProcessingJobSucceeded(_ context.Context, job types.ProcessingJob) (types.MediaAsset, error) {
	store.succeeded = append(store.succeeded, job)
	return types.MediaAsset{}, nil
}

func (store *fakeStore) MarkProcessingJobFailed(
	_ context.Context,
	job types.ProcessingJob,
	_ error,
	_ int,
	_ time.Duration,
) (bool, error) {
	store.failed = append(store.failed, job)
	return store.deadLetterOnFailure, nil
}

type fakeScanner struct {
	err error
}

func (scanner fakeScanner) Scan(context.Context, types.MediaAsset) error {
	return scanner.err
}

type fakeThumbnailer struct {
	err error
}

func (thumbnailer fakeThumbnailer) GenerateThumbnail(context.Context, types.MediaAsset) error {
	return thumbnailer.err
}

type fakeTranscoder struct {
	err error
}

func (transcoder fakeTranscoder) Transcode(context.Context, types.MediaAsset) error {
	return transcoder.err
}
