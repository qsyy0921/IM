package app

import (
	"context"

	"github.com/qsyy0921/IM/services/ai-eval-service/internal/types"
)

type Repository interface {
	RecordEvalRun(ctx context.Context, run types.EvalRun) (types.EvalRun, error)
	GetEvalRun(ctx context.Context, tenantID types.TenantID, runID string) (types.EvalRun, error)
	ListEvalRuns(ctx context.Context, command types.ListEvalRunsCommand, fetchLimit int) ([]types.EvalRun, error)
}
