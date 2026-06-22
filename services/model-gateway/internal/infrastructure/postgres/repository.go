package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/model-gateway/internal/domain"
	"github.com/qsyy0921/IM/services/model-gateway/internal/types"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (repository *Repository) StartTextInvocation(
	ctx context.Context,
	prepared domain.PreparedTextGeneration,
) (types.ModelInvocation, bool, error) {
	return repository.startInvocation(ctx, prepared.Command.AuthContext.TenantID, prepared.Command.CallerService, prepared.Command.IdempotencyKey, prepared.CommandHash, domain.InvocationFromStart(prepared))
}

func (repository *Repository) CompleteTextInvocation(ctx context.Context, invocation types.ModelInvocation) error {
	return repository.completeInvocation(ctx, invocation)
}

func (repository *Repository) StartEmbeddingInvocation(
	ctx context.Context,
	prepared domain.PreparedEmbedding,
) (types.ModelInvocation, bool, error) {
	return repository.startInvocation(ctx, prepared.Command.AuthContext.TenantID, prepared.Command.CallerService, prepared.Command.IdempotencyKey, prepared.CommandHash, domain.InvocationFromEmbeddingStart(prepared))
}

func (repository *Repository) CompleteEmbeddingInvocation(ctx context.Context, invocation types.ModelInvocation) error {
	return repository.completeInvocation(ctx, invocation)
}

func (repository *Repository) startInvocation(
	ctx context.Context,
	tenantID types.TenantID,
	callerService string,
	idempotencyKey string,
	commandHash string,
	invocation types.ModelInvocation,
) (types.ModelInvocation, bool, error) {
	if repository.pool == nil {
		return types.ModelInvocation{}, false, types.NewDBWriteFailed("model repository is not configured")
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return types.ModelInvocation{}, false, types.NewDBWriteFailed(err.Error())
	}
	defer func() { _ = tx.Rollback(ctx) }()

	existing, found, err := findInvocationByIdempotency(ctx, tx, tenantID, callerService, idempotencyKey)
	if err != nil {
		return types.ModelInvocation{}, false, err
	}
	if found {
		if existing.CommandHash != commandHash {
			return types.ModelInvocation{}, false, types.NewFailedPrecondition("idempotency command conflict")
		}
		if err := tx.Commit(ctx); err != nil {
			return types.ModelInvocation{}, false, types.NewDBWriteFailed(err.Error())
		}
		return existing, true, nil
	}

	if err := insertInvocation(ctx, tx, invocation); err != nil {
		return types.ModelInvocation{}, false, types.NewDBWriteFailed(err.Error())
	}
	if err := tx.Commit(ctx); err != nil {
		return types.ModelInvocation{}, false, types.NewDBWriteFailed(err.Error())
	}
	return invocation, false, nil
}

func (repository *Repository) completeInvocation(ctx context.Context, invocation types.ModelInvocation) error {
	if repository.pool == nil {
		return types.NewDBWriteFailed("model repository is not configured")
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := updateInvocationCompletion(ctx, tx, invocation); err != nil {
		return err
	}
	if err := insertModelOutbox(ctx, tx, invocation); err != nil {
		return err
	}
	if invocation.Status == types.InvocationStatusFailed {
		if err := upsertProviderFailure(ctx, tx, invocation); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func (repository *Repository) GetModelInvocation(
	ctx context.Context,
	tenantID types.TenantID,
	invocationID string,
) (types.ModelInvocation, error) {
	if repository.pool == nil {
		return types.ModelInvocation{}, types.NewDBReadFailed("model repository is not configured")
	}
	row := repository.pool.QueryRow(ctx, selectInvocationSQL()+`
WHERE tenant_id = $1 AND invocation_id = $2
`, string(tenantID), invocationID)
	invocation, err := scanInvocation(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return types.ModelInvocation{}, types.NewNotFound("model invocation not found")
		}
		return types.ModelInvocation{}, types.NewDBReadFailed(err.Error())
	}
	return invocation, nil
}

func findInvocationByIdempotency(
	ctx context.Context,
	tx pgx.Tx,
	tenantID types.TenantID,
	callerService string,
	idempotencyKey string,
) (types.ModelInvocation, bool, error) {
	row := tx.QueryRow(ctx, selectInvocationSQL()+`
WHERE tenant_id = $1 AND caller_service = $2 AND idempotency_key = $3
LIMIT 1
`, string(tenantID), callerService, idempotencyKey)
	invocation, err := scanInvocation(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return types.ModelInvocation{}, false, nil
		}
		return types.ModelInvocation{}, false, types.NewDBReadFailed(err.Error())
	}
	return invocation, true, nil
}

func insertInvocation(ctx context.Context, tx pgx.Tx, invocation types.ModelInvocation) error {
	_, err := tx.Exec(ctx, `
INSERT INTO model_invocations (
    tenant_id, invocation_id, idempotency_key, command_hash, caller_service,
    caller_use_case, request_type, data_class, provider_id, model_id,
    route_version, prompt_hash, status, timeout_ms, max_output_tokens,
    prompt_schema_version, correlation_id, causation_id, trace_id, created_at
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8, $9, $10,
    $11, $12, $13, $14, $15,
    $16, $17, $18, $19, $20
)
`, string(invocation.TenantID), invocation.InvocationID, invocation.IdempotencyKey, invocation.CommandHash,
		invocation.CallerService, invocation.CallerUseCase, invocation.RequestType, invocation.DataClass,
		invocation.ProviderID, invocation.ModelID, invocation.RouteVersion, invocation.PromptHash,
		invocation.Status, invocation.Timeout.Milliseconds(), invocation.MaxOutputTokens,
		invocation.PromptSchemaVersion, invocation.CorrelationID, invocation.CausationID,
		invocation.TraceID, invocation.CreatedAt)
	return err
}

func updateInvocationCompletion(ctx context.Context, tx pgx.Tx, invocation types.ModelInvocation) error {
	tag, err := tx.Exec(ctx, `
UPDATE model_invocations
SET output_hash = $3,
    output_schema_version = $4,
    input_tokens = $5,
    output_tokens = $6,
    total_tokens = $7,
    estimated_cost_microunits = $8,
    status = $9,
    failure_class = $10,
    provider_latency_ms = $11,
    completed_at = $12
WHERE tenant_id = $1
  AND invocation_id = $2
  AND status = 'PENDING'
`, string(invocation.TenantID), invocation.InvocationID, invocation.OutputHash,
		invocation.OutputSchemaVersion, invocation.TokenUsage.InputTokens,
		invocation.TokenUsage.OutputTokens, invocation.TokenUsage.TotalTokens,
		invocation.EstimatedCostMicrounits, invocation.Status, invocation.FailureClass,
		invocation.ProviderLatency.Milliseconds(), invocation.CompletedAt)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	if tag.RowsAffected() == 0 {
		return types.NewFailedPrecondition("model invocation is not pending")
	}
	return nil
}

func insertModelOutbox(ctx context.Context, tx pgx.Tx, invocation types.ModelInvocation) error {
	eventType := "model.invocation.completed.v1"
	if invocation.Status == types.InvocationStatusFailed {
		eventType = "model.invocation.failed.v1"
	}
	payload := map[string]any{
		"tenant_id":                 string(invocation.TenantID),
		"invocation_id":             invocation.InvocationID,
		"caller_service":            invocation.CallerService,
		"caller_use_case":           invocation.CallerUseCase,
		"request_type":              invocation.RequestType,
		"data_class":                invocation.DataClass,
		"provider_id":               invocation.ProviderID,
		"model_id":                  invocation.ModelID,
		"route_version":             invocation.RouteVersion,
		"prompt_hash":               invocation.PromptHash,
		"output_hash":               invocation.OutputHash,
		"input_tokens":              invocation.TokenUsage.InputTokens,
		"output_tokens":             invocation.TokenUsage.OutputTokens,
		"total_tokens":              invocation.TokenUsage.TotalTokens,
		"estimated_cost_microunits": invocation.EstimatedCostMicrounits,
		"status":                    invocation.Status,
		"failure_class":             invocation.FailureClass,
		"provider_latency_ms":       invocation.ProviderLatency.Milliseconds(),
		"correlation_id":            invocation.CorrelationID,
		"causation_id":              invocation.CausationID,
		"trace_id":                  invocation.TraceID,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	_, err = tx.Exec(ctx, `
INSERT INTO model_outbox (
    event_id, tenant_id, aggregate_type, aggregate_id, event_type,
    event_version, partition_key, payload_json, status, available_at, created_at, updated_at
) VALUES (
    $1, $2, 'model_invocation', $3, $4,
    1, $5, $6::jsonb, 'PENDING', now(), now(), now()
)
ON CONFLICT (event_id) DO NOTHING
`, "evt_"+invocation.InvocationID, string(invocation.TenantID), invocation.InvocationID,
		eventType, string(invocation.TenantID)+":"+invocation.InvocationID, string(encoded))
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func upsertProviderFailure(ctx context.Context, tx pgx.Tx, invocation types.ModelInvocation) error {
	_, err := tx.Exec(ctx, `
INSERT INTO model_provider_failures (
    tenant_id, provider_id, model_id, failure_class,
    failure_count, first_seen_at, last_seen_at
) VALUES ($1, $2, $3, $4, 1, now(), now())
ON CONFLICT (tenant_id, provider_id, model_id, failure_class)
DO UPDATE SET
    failure_count = model_provider_failures.failure_count + 1,
    last_seen_at = now()
`, string(invocation.TenantID), invocation.ProviderID, invocation.ModelID, invocation.FailureClass)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func selectInvocationSQL() string {
	return `
SELECT tenant_id, invocation_id, idempotency_key, command_hash, caller_service,
       caller_use_case, request_type, data_class, provider_id, model_id,
       route_version, prompt_hash, output_hash, output_schema_version,
       input_tokens, output_tokens, total_tokens, estimated_cost_microunits,
       status, failure_class, provider_latency_ms, timeout_ms,
       max_output_tokens, prompt_schema_version, correlation_id, causation_id,
       trace_id, created_at, completed_at
FROM model_invocations
`
}

type invocationScanner interface {
	Scan(dest ...any) error
}

func scanInvocation(row invocationScanner) (types.ModelInvocation, error) {
	var invocation types.ModelInvocation
	var completedAt *time.Time
	var providerLatencyMs int64
	var timeoutMs int64
	if err := row.Scan(
		&invocation.TenantID, &invocation.InvocationID, &invocation.IdempotencyKey, &invocation.CommandHash,
		&invocation.CallerService, &invocation.CallerUseCase, &invocation.RequestType, &invocation.DataClass,
		&invocation.ProviderID, &invocation.ModelID, &invocation.RouteVersion, &invocation.PromptHash,
		&invocation.OutputHash, &invocation.OutputSchemaVersion, &invocation.TokenUsage.InputTokens,
		&invocation.TokenUsage.OutputTokens, &invocation.TokenUsage.TotalTokens, &invocation.EstimatedCostMicrounits,
		&invocation.Status, &invocation.FailureClass, &providerLatencyMs,
		&timeoutMs, &invocation.MaxOutputTokens, &invocation.PromptSchemaVersion, &invocation.CorrelationID,
		&invocation.CausationID, &invocation.TraceID, &invocation.CreatedAt, &completedAt,
	); err != nil {
		return types.ModelInvocation{}, err
	}
	if completedAt != nil {
		invocation.CompletedAt = *completedAt
	}
	invocation.ProviderLatency = time.Duration(providerLatencyMs) * time.Millisecond
	invocation.Timeout = time.Duration(timeoutMs) * time.Millisecond
	return invocation, nil
}
