package app

import (
	"context"
	"errors"
	"testing"

	"github.com/qsyy0921/IM/services/ai-eval-service/internal/types"
)

func TestRecordEvalRunUseCaseNormalizesAndStoresTenant(t *testing.T) {
	repository := &fakeRepository{}
	useCase := NewRecordEvalRunUseCase(repository)
	result, err := useCase.Execute(context.Background(), types.RecordEvalRunCommand{
		AuthContext: validAuth(),
		Run: types.EvalRun{
			RunID:        " run-1 ",
			SuiteID:      " ai-eval-profile ",
			Stage:        " profile-overgeneralization ",
			Adapter:      " local-fixture ",
			Status:       types.EvalRunStatusPassed,
			CaseCount:    2,
			PassedCount:  2,
			SummaryRef:   " H:/NexusIM/loadtest-results/summary.json ",
			MetadataJSON: `{"source":"test"}`,
		},
	})
	if err != nil {
		t.Fatalf("record eval run: %v", err)
	}
	if result.TenantID != "tenant-1" ||
		result.RunID != "run-1" ||
		result.SuiteID != "ai-eval-profile" ||
		result.SummaryRef != "H:/NexusIM/loadtest-results/summary.json" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if repository.recorded.TenantID != "tenant-1" {
		t.Fatalf("expected usecase to force tenant from auth, got %+v", repository.recorded)
	}
}

func TestRecordEvalRunUseCaseRejectsRawJSONListMetadata(t *testing.T) {
	_, err := NewRecordEvalRunUseCase(&fakeRepository{}).Execute(context.Background(), types.RecordEvalRunCommand{
		AuthContext: validAuth(),
		Run: types.EvalRun{
			RunID:        "run-1",
			SuiteID:      "suite-1",
			Status:       types.EvalRunStatusFailed,
			CaseCount:    1,
			FailedCount:  1,
			MetadataJSON: `[]`,
		},
	})
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid argument, got %v", err)
	}
}

func TestRecordEvalRunUseCaseRejectsImpossibleCounts(t *testing.T) {
	_, err := NewRecordEvalRunUseCase(&fakeRepository{}).Execute(context.Background(), types.RecordEvalRunCommand{
		AuthContext: validAuth(),
		Run: types.EvalRun{
			RunID:       "run-1",
			SuiteID:     "suite-1",
			Status:      types.EvalRunStatusFailed,
			CaseCount:   1,
			PassedCount: 1,
			FailedCount: 1,
		},
	})
	if !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid argument, got %v", err)
	}
}

func TestListEvalRunsUseCaseReturnsCursor(t *testing.T) {
	repository := &fakeRepository{list: []types.EvalRun{
		{RunID: "a"},
		{RunID: "b"},
		{RunID: "c"},
	}}
	result, err := NewListEvalRunsUseCase(repository).Execute(context.Background(), types.ListEvalRunsCommand{
		AuthContext: validAuth(),
		Limit:       2,
	})
	if err != nil {
		t.Fatalf("list eval runs: %v", err)
	}
	if len(result.Runs) != 2 || result.NextCursor != "b" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if repository.fetchLimit != 3 {
		t.Fatalf("expected fetch limit 3, got %d", repository.fetchLimit)
	}
}

func validAuth() types.AuthContext {
	return types.AuthContext{
		TenantID: "tenant-1",
		UserID:   "user-1",
		DeviceID: "device-1",
	}
}

type fakeRepository struct {
	recorded   types.EvalRun
	list       []types.EvalRun
	fetchLimit int
}

func (repository *fakeRepository) RecordEvalRun(
	_ context.Context,
	run types.EvalRun,
) (types.EvalRun, error) {
	repository.recorded = run
	return run, nil
}

func (repository *fakeRepository) GetEvalRun(
	_ context.Context,
	tenantID types.TenantID,
	runID string,
) (types.EvalRun, error) {
	return types.EvalRun{TenantID: tenantID, RunID: runID}, nil
}

func (repository *fakeRepository) ListEvalRuns(
	_ context.Context,
	_ types.ListEvalRunsCommand,
	fetchLimit int,
) ([]types.EvalRun, error) {
	repository.fetchLimit = fetchLimit
	return repository.list, nil
}
