package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/action-executor/internal/types"
)

type Repository struct {
	pool *pgxpool.Pool
}

type providerFailureRetryRow struct {
	TenantID          string
	ProviderFailureID string
	RetryCount        int
}

func NewRepository(pool *pgxpool.Pool) Repository {
	return Repository{pool: pool}
}

func (repository Repository) RecordExecution(
	ctx context.Context,
	audit types.ExecutionAudit,
	projection types.ToolResultProjection,
	providerFailure *types.ProviderFailureProjection,
) error {
	if repository.pool == nil {
		return errors.Join(types.ErrExecutionAuditFailed, errors.New("nil pg pool"))
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("%w: %v", types.ErrExecutionAuditFailed, err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()
	if err := insertExecutionAudit(ctx, tx, audit); err != nil {
		return err
	}
	if err := insertToolResultProjection(ctx, tx, projection); err != nil {
		return err
	}
	if providerFailure != nil {
		if err := insertProviderFailureProjection(ctx, tx, *providerFailure); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("%w: %v", types.ErrExecutionAuditFailed, err)
	}
	return nil
}

func insertExecutionAudit(ctx context.Context, tx pgx.Tx, audit types.ExecutionAudit) error {
	_, err := tx.Exec(ctx, `
INSERT INTO action_executor_execution_audits (
    tenant_id,
    execution_id,
    proposal_id,
    approval_id,
    prepared_audit_id,
    user_id,
    device_id,
    session_id,
    trace_id,
    request_id,
    skill_id,
    tool_name,
    tool_action,
    resource_type,
    resource_id,
    risk_level,
    intent,
    idempotency_key,
    input_sha256,
    allowed,
    requires_approval,
    permission_version,
    classification,
    reason,
    decision_source,
    status,
    executed,
    output_sha256
) VALUES (
    $1, $2, $3, $4, $5, $6, $7,
    $8, $9, $10, $11, $12, $13, $14,
    $15, $16, $17, $18, $19, $20, $21,
    $22, $23, $24, $25, $26, $27, $28
)`,
		string(audit.TenantID),
		audit.ExecutionID,
		audit.ProposalID,
		audit.ApprovalID,
		audit.PreparedAuditID,
		string(audit.UserID),
		audit.DeviceID,
		audit.SessionID,
		audit.TraceID,
		audit.RequestID,
		audit.SkillID,
		audit.ToolName,
		audit.Action,
		audit.ResourceType,
		audit.ResourceID,
		audit.RiskLevel,
		audit.Intent,
		audit.IdempotencyKey,
		audit.InputSHA256,
		audit.Allowed,
		audit.RequiresApproval,
		audit.PermissionVersion,
		truncateLowSensitive(audit.Classification, 128),
		truncateLowSensitive(audit.Reason, 512),
		truncateLowSensitive(audit.DecisionSource, 128),
		audit.Status,
		audit.Executed,
		audit.OutputSHA256,
	)
	if err != nil {
		return fmt.Errorf("%w: %v", types.ErrExecutionAuditFailed, err)
	}
	return nil
}

func insertToolResultProjection(ctx context.Context, tx pgx.Tx, projection types.ToolResultProjection) error {
	_, err := tx.Exec(ctx, `
INSERT INTO action_executor_tool_results (
    tenant_id,
    result_id,
    execution_id,
    proposal_id,
    approval_id,
    prepared_audit_id,
    user_id,
    skill_id,
    tool_name,
    resource_type,
    resource_id,
    status,
    executed,
    result_ref,
    output_sha256
) VALUES (
    $1, $2, $3, $4, $5, $6, $7,
    $8, $9, $10, $11, $12, $13, $14, $15
)`,
		string(projection.TenantID),
		projection.ResultID,
		projection.ExecutionID,
		projection.ProposalID,
		projection.ApprovalID,
		projection.PreparedAuditID,
		string(projection.UserID),
		projection.SkillID,
		projection.ToolName,
		projection.ResourceType,
		projection.ResourceID,
		projection.Status,
		projection.Executed,
		projection.ResultRef,
		projection.OutputSHA256,
	)
	if err != nil {
		return fmt.Errorf("%w: %v", types.ErrExecutionAuditFailed, err)
	}
	return nil
}

func insertProviderFailureProjection(
	ctx context.Context,
	tx pgx.Tx,
	projection types.ProviderFailureProjection,
) error {
	_, err := tx.Exec(ctx, `
INSERT INTO action_executor_provider_failures (
    tenant_id,
    provider_failure_id,
    execution_id,
    result_id,
    proposal_id,
    approval_id,
    prepared_audit_id,
    user_id,
    skill_id,
    tool_name,
    resource_type,
    resource_id,
    classification,
    status,
    retryable,
    retry_count,
    next_retry_at,
    dead_lettered_at,
    failure_ref,
    created_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7,
    $8, $9, $10, $11, $12, $13, $14,
    $15, $16, $17, $18, $19, COALESCE($20, now())
)`,
		string(projection.TenantID),
		projection.ProviderFailureID,
		projection.ExecutionID,
		projection.ResultID,
		projection.ProposalID,
		projection.ApprovalID,
		projection.PreparedAuditID,
		string(projection.UserID),
		projection.SkillID,
		projection.ToolName,
		projection.ResourceType,
		projection.ResourceID,
		truncateLowSensitive(projection.Classification, 128),
		projection.Status,
		projection.Retryable,
		projection.RetryCount,
		nullableTime(projection.NextRetryAt),
		nullableTime(projection.DeadLetteredAt),
		truncateLowSensitive(projection.FailureRef, 512),
		nullableTime(projection.CreatedAt),
	)
	if err != nil {
		return fmt.Errorf("%w: %v", types.ErrExecutionAuditFailed, err)
	}
	return nil
}

func (repository Repository) ProcessDueProviderFailures(
	ctx context.Context,
	limit int,
	maxAttempts int,
	retryBaseDelay time.Duration,
	now time.Time,
) (types.ProviderFailureRetryStats, error) {
	if repository.pool == nil {
		return types.ProviderFailureRetryStats{}, errors.Join(types.ErrExecutionAuditFailed, errors.New("nil pg pool"))
	}
	limit, maxAttempts, retryBaseDelay, now = normalizeProviderFailureRetryConfig(limit, maxAttempts, retryBaseDelay, now)
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return types.ProviderFailureRetryStats{}, fmt.Errorf("%w: %v", types.ErrExecutionAuditFailed, err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()
	rows, err := tx.Query(ctx, `
SELECT tenant_id, provider_failure_id, retry_count
FROM action_executor_provider_failures
WHERE status = 'RETRY_PENDING'
  AND retryable = TRUE
  AND next_retry_at IS NOT NULL
  AND next_retry_at <= $1
ORDER BY next_retry_at, created_at, provider_failure_id
LIMIT $2
FOR UPDATE SKIP LOCKED`, now, limit)
	if err != nil {
		return types.ProviderFailureRetryStats{}, fmt.Errorf("%w: %v", types.ErrExecutionAuditFailed, err)
	}
	ready := make([]providerFailureRetryRow, 0, limit)
	for rows.Next() {
		var row providerFailureRetryRow
		if err := rows.Scan(&row.TenantID, &row.ProviderFailureID, &row.RetryCount); err != nil {
			return types.ProviderFailureRetryStats{}, fmt.Errorf("%w: %v", types.ErrExecutionAuditFailed, err)
		}
		ready = append(ready, row)
	}
	if err := rows.Err(); err != nil {
		return types.ProviderFailureRetryStats{}, fmt.Errorf("%w: %v", types.ErrExecutionAuditFailed, err)
	}
	rows.Close()

	stats := types.ProviderFailureRetryStats{Fetched: len(ready)}
	for _, row := range ready {
		nextRetryCount := row.RetryCount + 1
		if nextRetryCount >= maxAttempts {
			if _, err := tx.Exec(ctx, `
UPDATE action_executor_provider_failures
SET retry_count = $3,
    status = 'DLQ',
    retryable = FALSE,
    next_retry_at = NULL,
    dead_lettered_at = $4
WHERE tenant_id = $1 AND provider_failure_id = $2`,
				row.TenantID,
				row.ProviderFailureID,
				nextRetryCount,
				now,
			); err != nil {
				return types.ProviderFailureRetryStats{}, fmt.Errorf("%w: %v", types.ErrExecutionAuditFailed, err)
			}
			stats.DeadLettered++
			continue
		}
		if _, err := tx.Exec(ctx, `
UPDATE action_executor_provider_failures
SET retry_count = $3,
    next_retry_at = $4
WHERE tenant_id = $1 AND provider_failure_id = $2`,
			row.TenantID,
			row.ProviderFailureID,
			nextRetryCount,
			now.Add(providerFailureRetryDelay(retryBaseDelay, nextRetryCount)),
		); err != nil {
			return types.ProviderFailureRetryStats{}, fmt.Errorf("%w: %v", types.ErrExecutionAuditFailed, err)
		}
		stats.Rescheduled++
	}
	if err := tx.Commit(ctx); err != nil {
		return types.ProviderFailureRetryStats{}, fmt.Errorf("%w: %v", types.ErrExecutionAuditFailed, err)
	}
	return stats, nil
}

func truncateLowSensitive(value string, max int) string {
	value = strings.TrimSpace(value)
	if max <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max])
}

func nullableTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value
}

func normalizeProviderFailureRetryConfig(
	limit int,
	maxAttempts int,
	retryBaseDelay time.Duration,
	now time.Time,
) (int, int, time.Duration, time.Time) {
	if limit <= 0 {
		limit = 100
	}
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	if retryBaseDelay <= 0 {
		retryBaseDelay = 30 * time.Second
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return limit, maxAttempts, retryBaseDelay, now.UTC()
}

func providerFailureRetryDelay(base time.Duration, nextRetryCount int) time.Duration {
	if nextRetryCount <= 1 {
		return base
	}
	shift := nextRetryCount - 1
	if shift > 6 {
		shift = 6
	}
	return base * time.Duration(1<<shift)
}
