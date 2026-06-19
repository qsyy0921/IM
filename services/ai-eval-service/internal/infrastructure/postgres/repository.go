package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/ai-eval-service/internal/types"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (repository *Repository) RecordEvalRun(ctx context.Context, run types.EvalRun) (types.EvalRun, error) {
	if repository.pool == nil {
		return types.EvalRun{}, types.NewDBWriteFailed("ai eval repository is not configured")
	}
	row := repository.pool.QueryRow(ctx, `
INSERT INTO ai_eval_runs (
	tenant_id,
	run_id,
	suite_id,
	stage,
	adapter,
	status,
	case_count,
	passed_count,
	failed_count,
	skipped_count,
	summary_ref,
	report_ref,
	metadata_json,
	created_at,
	updated_at,
	completed_at
) VALUES (
	$1, $2, $3, $4, $5, $6,
	$7, $8, $9, $10,
	$11, $12, $13::jsonb,
	now(), now(),
	CASE WHEN $6 IN ('PASSED', 'FAILED') THEN now() ELSE NULL END
)
ON CONFLICT (tenant_id, run_id) DO UPDATE SET
	suite_id = EXCLUDED.suite_id,
	stage = EXCLUDED.stage,
	adapter = EXCLUDED.adapter,
	status = EXCLUDED.status,
	case_count = EXCLUDED.case_count,
	passed_count = EXCLUDED.passed_count,
	failed_count = EXCLUDED.failed_count,
	skipped_count = EXCLUDED.skipped_count,
	summary_ref = EXCLUDED.summary_ref,
	report_ref = EXCLUDED.report_ref,
	metadata_json = EXCLUDED.metadata_json,
	updated_at = now(),
	completed_at = CASE
		WHEN EXCLUDED.status IN ('PASSED', 'FAILED') THEN COALESCE(ai_eval_runs.completed_at, now())
		ELSE NULL
	END
RETURNING
	tenant_id,
	run_id,
	suite_id,
	stage,
	adapter,
	status,
	case_count,
	passed_count,
	failed_count,
	skipped_count,
	summary_ref,
	report_ref,
	metadata_json,
	created_at,
	updated_at,
	completed_at
`, run.TenantID,
		run.RunID,
		run.SuiteID,
		run.Stage,
		run.Adapter,
		run.Status,
		run.CaseCount,
		run.PassedCount,
		run.FailedCount,
		run.SkippedCount,
		run.SummaryRef,
		run.ReportRef,
		run.MetadataJSON,
	)
	result, err := scanEvalRun(row)
	if err != nil {
		return types.EvalRun{}, types.NewDBWriteFailed(err.Error())
	}
	return result, nil
}

func (repository *Repository) GetEvalRun(
	ctx context.Context,
	tenantID types.TenantID,
	runID string,
) (types.EvalRun, error) {
	if repository.pool == nil {
		return types.EvalRun{}, types.NewDBReadFailed("ai eval repository is not configured")
	}
	row := repository.pool.QueryRow(ctx, `
SELECT
	tenant_id,
	run_id,
	suite_id,
	stage,
	adapter,
	status,
	case_count,
	passed_count,
	failed_count,
	skipped_count,
	summary_ref,
	report_ref,
	metadata_json,
	created_at,
	updated_at,
	completed_at
FROM ai_eval_runs
WHERE tenant_id = $1
  AND run_id = $2
`, tenantID, strings.TrimSpace(runID))
	result, err := scanEvalRun(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return types.EvalRun{}, types.NewEvalRunNotFound("eval run not found")
		}
		return types.EvalRun{}, types.NewDBReadFailed(err.Error())
	}
	return result, nil
}

func (repository *Repository) ListEvalRuns(
	ctx context.Context,
	command types.ListEvalRunsCommand,
	fetchLimit int,
) ([]types.EvalRun, error) {
	if repository.pool == nil {
		return nil, types.NewDBReadFailed("ai eval repository is not configured")
	}
	if fetchLimit <= 0 {
		return nil, nil
	}

	args := []any{command.AuthContext.TenantID, fetchLimit}
	filters := []string{"tenant_id = $1"}
	if strings.TrimSpace(command.SuiteID) != "" {
		args = append(args, strings.TrimSpace(command.SuiteID))
		filters = append(filters, fmt.Sprintf("suite_id = $%d", len(args)))
	}
	if command.Status != "" {
		args = append(args, command.Status)
		filters = append(filters, fmt.Sprintf("status = $%d", len(args)))
	}
	if strings.TrimSpace(command.AfterRunID) != "" {
		args = append(args, strings.TrimSpace(command.AfterRunID))
		filters = append(filters, fmt.Sprintf("run_id > $%d", len(args)))
	}

	query := `
SELECT
	tenant_id,
	run_id,
	suite_id,
	stage,
	adapter,
	status,
	case_count,
	passed_count,
	failed_count,
	skipped_count,
	summary_ref,
	report_ref,
	metadata_json,
	created_at,
	updated_at,
	completed_at
FROM ai_eval_runs
WHERE ` + strings.Join(filters, "\n  AND ") + `
ORDER BY run_id ASC
LIMIT $2
`
	rows, err := repository.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, types.NewDBReadFailed(err.Error())
	}
	defer rows.Close()

	items := make([]types.EvalRun, 0, fetchLimit)
	for rows.Next() {
		item, err := scanEvalRun(rows)
		if err != nil {
			return nil, types.NewDBReadFailed(err.Error())
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, types.NewDBReadFailed(err.Error())
	}
	return items, nil
}

type evalRunScanner interface {
	Scan(dest ...any) error
}

func scanEvalRun(scanner evalRunScanner) (types.EvalRun, error) {
	var run types.EvalRun
	var completedAt sql.NullTime
	if err := scanner.Scan(
		&run.TenantID,
		&run.RunID,
		&run.SuiteID,
		&run.Stage,
		&run.Adapter,
		&run.Status,
		&run.CaseCount,
		&run.PassedCount,
		&run.FailedCount,
		&run.SkippedCount,
		&run.SummaryRef,
		&run.ReportRef,
		&run.MetadataJSON,
		&run.CreatedAt,
		&run.UpdatedAt,
		&completedAt,
	); err != nil {
		return types.EvalRun{}, err
	}
	if completedAt.Valid {
		run.CompletedAt = completedAt.Time
	}
	return run, nil
}
