package rebuild

import (
	"context"
	"errors"
	"testing"

	"github.com/qsyy0921/IM/services/vector-index-service/internal/types"
)

func TestWorkerRunOnceCompletesClaimedTasks(t *testing.T) {
	store := &fakeStore{
		tasks: []types.VectorRebuildTask{
			{
				Job: types.VectorIndexJob{
					TenantID: "tenant-vector",
					JobID:    "vjob_rebuild_1",
					JobType:  types.JobTypeRebuild,
					Status:   types.JobStatusVectorUpserting,
				},
				Checkpoint: types.VectorRebuildCheckpoint{
					TenantID:     "tenant-vector",
					RebuildJobID: "vjob_rebuild_1",
					Status:       types.RebuildCheckpointStatusRunning,
				},
				CollectionType: types.CollectionTypeKnowledgeChunk,
			},
		},
	}
	worker := NewWorker(store, Config{BatchSize: 10})
	stats, err := worker.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if stats.Claimed != 1 || stats.Completed != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	if len(store.completed) != 1 || store.completed[0].Job.JobID != "vjob_rebuild_1" {
		t.Fatalf("unexpected completed tasks: %+v", store.completed)
	}
}

func TestWorkerRunOnceRequiresStore(t *testing.T) {
	worker := NewWorker(nil, Config{})
	if _, err := worker.RunOnce(context.Background()); err == nil {
		t.Fatal("expected missing store error")
	}
}

func TestWorkerRunOncePropagatesCompleteError(t *testing.T) {
	store := &fakeStore{
		tasks: []types.VectorRebuildTask{{Job: types.VectorIndexJob{JobID: "vjob_rebuild_1"}}},
		err:   errors.New("complete failed"),
	}
	worker := NewWorker(store, Config{})
	stats, err := worker.RunOnce(context.Background())
	if err == nil {
		t.Fatal("expected complete error")
	}
	if stats.Claimed != 1 || stats.Completed != 0 {
		t.Fatalf("unexpected stats after error: %+v", stats)
	}
}

type fakeStore struct {
	tasks     []types.VectorRebuildTask
	completed []types.VectorRebuildTask
	err       error
}

func (store *fakeStore) ClaimRebuildTasks(_ context.Context, limit int) ([]types.VectorRebuildTask, error) {
	if limit < len(store.tasks) {
		return store.tasks[:limit], nil
	}
	return store.tasks, nil
}

func (store *fakeStore) CompleteRebuildTask(_ context.Context, task types.VectorRebuildTask) error {
	if store.err != nil {
		return store.err
	}
	store.completed = append(store.completed, task)
	return nil
}
