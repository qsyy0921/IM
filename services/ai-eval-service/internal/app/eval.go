package app

import (
	"context"

	"github.com/qsyy0921/IM/services/ai-eval-service/internal/types"
)

type RecordEvalRunUseCase struct {
	repository Repository
}

func NewRecordEvalRunUseCase(repository Repository) *RecordEvalRunUseCase {
	return &RecordEvalRunUseCase{repository: repository}
}

func (useCase *RecordEvalRunUseCase) Execute(
	ctx context.Context,
	command types.RecordEvalRunCommand,
) (types.EvalRun, error) {
	if err := command.Validate(); err != nil {
		return types.EvalRun{}, err
	}
	run := command.Run.Normalized()
	run.TenantID = command.AuthContext.TenantID
	return useCase.repository.RecordEvalRun(ctx, run)
}

type GetEvalRunUseCase struct {
	repository Repository
}

func NewGetEvalRunUseCase(repository Repository) *GetEvalRunUseCase {
	return &GetEvalRunUseCase{repository: repository}
}

func (useCase *GetEvalRunUseCase) Execute(
	ctx context.Context,
	command types.GetEvalRunCommand,
) (types.EvalRun, error) {
	if err := command.Validate(); err != nil {
		return types.EvalRun{}, err
	}
	return useCase.repository.GetEvalRun(ctx, command.AuthContext.TenantID, command.RunID)
}

type ListEvalRunsUseCase struct {
	repository Repository
}

func NewListEvalRunsUseCase(repository Repository) *ListEvalRunsUseCase {
	return &ListEvalRunsUseCase{repository: repository}
}

func (useCase *ListEvalRunsUseCase) Execute(
	ctx context.Context,
	command types.ListEvalRunsCommand,
) (types.ListEvalRunsResult, error) {
	if err := command.Validate(); err != nil {
		return types.ListEvalRunsResult{}, err
	}
	limit := command.EffectiveLimit()
	items, err := useCase.repository.ListEvalRuns(ctx, command, limit+1)
	if err != nil {
		return types.ListEvalRunsResult{}, err
	}
	result := types.ListEvalRunsResult{Runs: items}
	if len(result.Runs) > limit {
		result.NextCursor = result.Runs[limit-1].RunID
		result.Runs = result.Runs[:limit]
	}
	return result, nil
}
