package memberchange

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/conversation-service/internal/types"
)

func TestProgressWorkerRunOnceExecutesUseCase(t *testing.T) {
	executor := &fakeProgressExecutor{
		stats: types.MemberChangePublishProgressStats{Advanced: 2},
	}
	worker := NewProgressWorker(executor, ProgressConfig{})

	stats, err := worker.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if stats.Advanced != 2 || executor.calls != 1 {
		t.Fatalf("unexpected stats=%+v calls=%d", stats, executor.calls)
	}
}

func TestProgressWorkerRunOnceRequiresExecutor(t *testing.T) {
	worker := NewProgressWorker(nil, ProgressConfig{})

	_, err := worker.RunOnce(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestProgressWorkerRunRequiresExecutor(t *testing.T) {
	worker := NewProgressWorker(nil, ProgressConfig{PollInterval: time.Millisecond})

	err := worker.Run(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestProgressWorkerRunContinuesImmediatelyWhenAdvanced(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	executor := &fakeProgressExecutor{
		results: []types.MemberChangePublishProgressStats{
			{Advanced: 1},
			{Advanced: 0},
		},
		afterCalls: 2,
		cancel:     cancel,
	}
	worker := NewProgressWorker(executor, ProgressConfig{PollInterval: time.Hour})

	err := worker.Run(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled, got %v", err)
	}
	if executor.calls < 2 {
		t.Fatalf("expected immediate second call, got %d", executor.calls)
	}
}

func TestProgressWorkerRunRetriesAfterError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	executor := &fakeProgressExecutor{
		errs: []error{
			errors.New("temporary db error"),
			nil,
		},
		results: []types.MemberChangePublishProgressStats{
			{},
			{},
		},
		afterCalls: 2,
		cancel:     cancel,
	}
	worker := NewProgressWorker(executor, ProgressConfig{
		PollInterval: time.Hour,
		ErrorBackoff: time.Millisecond,
	})

	err := worker.Run(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled, got %v", err)
	}
	if executor.calls < 2 {
		t.Fatalf("expected retry after error, got %d calls", executor.calls)
	}
	snapshot := worker.Snapshot()
	if snapshot.TotalErrors != 1 || snapshot.ConsecutiveErrors != 0 {
		t.Fatalf("unexpected worker snapshot after retry: %+v", snapshot)
	}
	if snapshot.LastErrorBackoffMS != time.Millisecond.Milliseconds() {
		t.Fatalf("unexpected error backoff snapshot: %+v", snapshot)
	}
}

func TestProgressWorkerSnapshotTracksAdvancedRuns(t *testing.T) {
	worker := NewProgressWorker(&fakeProgressExecutor{
		stats: types.MemberChangePublishProgressStats{Advanced: 3},
	}, ProgressConfig{})

	if _, err := worker.RunOnce(context.Background()); err != nil {
		t.Fatalf("run once: %v", err)
	}
	worker.recordSuccess(types.MemberChangePublishProgressStats{Advanced: 3})

	snapshot := worker.Snapshot()
	if snapshot.LastSuccessAtMS == 0 || snapshot.LastAdvancedAtMS == 0 || snapshot.LastAdvancedCount != 3 {
		t.Fatalf("unexpected worker snapshot: %+v", snapshot)
	}
}

type fakeProgressExecutor struct {
	calls      int
	stats      types.MemberChangePublishProgressStats
	results    []types.MemberChangePublishProgressStats
	err        error
	errs       []error
	afterCalls int
	cancel     context.CancelFunc
}

func (f *fakeProgressExecutor) Execute(context.Context) (types.MemberChangePublishProgressStats, error) {
	f.calls++
	if f.afterCalls > 0 && f.calls >= f.afterCalls && f.cancel != nil {
		defer f.cancel()
	}
	if len(f.errs) >= f.calls && f.errs[f.calls-1] != nil {
		return types.MemberChangePublishProgressStats{}, f.errs[f.calls-1]
	}
	if f.err != nil {
		return types.MemberChangePublishProgressStats{}, f.err
	}
	if len(f.results) >= f.calls {
		return f.results[f.calls-1], nil
	}
	return f.stats, nil
}
