package postgres

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/model-gateway/internal/domain"
	"github.com/qsyy0921/IM/services/model-gateway/internal/types"
)

func TestRepositoryTextInvocationIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openModelTestPool(t)
	resetModelTables(t, ctx, pool)
	repository := NewRepository(pool)
	prepared := prepareTextInvocation(t, "idem-1", "sha256:prompt-1", "minv_1")

	started, replayed, err := repository.StartTextInvocation(ctx, prepared)
	if err != nil {
		t.Fatalf("start invocation: %v", err)
	}
	if replayed || started.Status != types.InvocationStatusPending {
		t.Fatalf("unexpected started invocation: replayed=%v %+v", replayed, started)
	}
	completed := domain.InvocationFromSuccess(started, domain.ProviderTextResult{
		OutputText:              "do not persist this raw output",
		OutputHash:              domain.OutputHash("do not persist this raw output"),
		OutputSchemaVersion:     1,
		TokenUsage:              types.TokenUsage{InputTokens: 4, OutputTokens: 5, TotalTokens: 9},
		EstimatedCostMicrounits: 90,
		Latency:                 15 * time.Millisecond,
	}, time.Now().UTC())
	if err := repository.CompleteTextInvocation(ctx, completed); err != nil {
		t.Fatalf("complete invocation: %v", err)
	}

	replay, replayed, err := repository.StartTextInvocation(ctx, prepared)
	if err != nil {
		t.Fatalf("replay start invocation: %v", err)
	}
	if !replayed || replay.InvocationID != started.InvocationID || replay.Status != types.InvocationStatusSucceeded {
		t.Fatalf("unexpected replay: replayed=%v %+v", replayed, replay)
	}
	conflict := prepareTextInvocation(t, "idem-1", "sha256:other-prompt", "minv_2")
	if _, _, err := repository.StartTextInvocation(ctx, conflict); !errors.Is(err, types.ErrFailedPrecondition) {
		t.Fatalf("expected conflict, got %v", err)
	}

	loaded, err := repository.GetModelInvocation(ctx, "tenant-model-test", started.InvocationID)
	if err != nil {
		t.Fatalf("get invocation: %v", err)
	}
	if loaded.PromptHash != "sha256:prompt-1" || loaded.OutputHash == "" || loaded.Status != types.InvocationStatusSucceeded {
		t.Fatalf("unexpected loaded invocation: %+v", loaded)
	}
	assertModelOutboxLowSensitive(t, ctx, pool)
}

func TestRepositoryEmbeddingInvocationIntegration(t *testing.T) {
	ctx := context.Background()
	pool := openModelTestPool(t)
	resetModelTables(t, ctx, pool)
	repository := NewRepository(pool)
	prepared := prepareEmbeddingInvocation(t, "embed-idem-1", "sha256:embedding-input", "minv_embedding_1")

	started, replayed, err := repository.StartEmbeddingInvocation(ctx, prepared)
	if err != nil {
		t.Fatalf("start embedding invocation: %v", err)
	}
	if replayed || started.Status != types.InvocationStatusPending || started.RequestType != types.RequestTypeEmbedding {
		t.Fatalf("unexpected started embedding invocation: replayed=%v %+v", replayed, started)
	}
	completed := domain.InvocationFromEmbeddingSuccess(started, domain.ProviderEmbeddingResult{
		EmbeddingValues:         []float32{0.11, 0.22, 0.33, 0.44},
		EmbeddingHash:           "sha256:embedding-output",
		OutputSchemaVersion:     1,
		TokenUsage:              types.TokenUsage{InputTokens: 4, OutputTokens: 0, TotalTokens: 4},
		EstimatedCostMicrounits: 20,
		Latency:                 7 * time.Millisecond,
	}, time.Now().UTC())
	if err := repository.CompleteEmbeddingInvocation(ctx, completed); err != nil {
		t.Fatalf("complete embedding invocation: %v", err)
	}

	replay, replayed, err := repository.StartEmbeddingInvocation(ctx, prepared)
	if err != nil {
		t.Fatalf("replay start embedding invocation: %v", err)
	}
	if !replayed || replay.InvocationID != started.InvocationID || replay.OutputHash != "sha256:embedding-output" {
		t.Fatalf("unexpected embedding replay: replayed=%v %+v", replayed, replay)
	}
	loaded, err := repository.GetModelInvocation(ctx, "tenant-model-test", started.InvocationID)
	if err != nil {
		t.Fatalf("get embedding invocation: %v", err)
	}
	if loaded.PromptHash != "sha256:embedding-input" || loaded.RequestType != types.RequestTypeEmbedding {
		t.Fatalf("unexpected loaded embedding invocation: %+v", loaded)
	}
	assertModelOutboxLowSensitive(t, ctx, pool)
}

func prepareTextInvocation(
	t *testing.T,
	idempotencyKey string,
	promptHash string,
	invocationID string,
) domain.PreparedTextGeneration {
	t.Helper()
	prepared, err := domain.PrepareTextGeneration(types.TextGenerationCommand{
		AuthContext: types.AuthContext{
			TenantID:    "tenant-model-test",
			ServiceName: "rag-service",
		},
		CallerService:       "rag-service",
		CallerUseCase:       "answer-question",
		IdempotencyKey:      idempotencyKey,
		ModelClass:          types.DefaultModelClass,
		PreferredModel:      types.DefaultModelID,
		RoutePolicy:         types.DefaultRoutePolicy,
		DataClass:           types.DataClassBusinessInternal,
		SafetyPolicy:        types.DefaultSafetyPolicy,
		PromptParts:         []types.PromptPart{{Role: "USER", Content: "source backed answer only"}},
		PromptHash:          promptHash,
		PromptSchemaVersion: 1,
		MaxOutputTokens:     128,
		Temperature:         0,
		Timeout:             time.Second,
		CorrelationID:       "corr-model-test",
		TraceID:             "trace-model-test",
	}, invocationID, time.Now().UTC())
	if err != nil {
		t.Fatalf("prepare text invocation: %v", err)
	}
	return prepared
}

func prepareEmbeddingInvocation(
	t *testing.T,
	idempotencyKey string,
	inputHash string,
	invocationID string,
) domain.PreparedEmbedding {
	t.Helper()
	prepared, err := domain.PrepareEmbedding(types.EmbeddingCommand{
		AuthContext: types.AuthContext{
			TenantID:    "tenant-model-test",
			ServiceName: "vector-index-service",
		},
		CallerService:      "vector-index-service",
		CallerUseCase:      "embed-vector-item",
		IdempotencyKey:     idempotencyKey,
		ModelClass:         types.DefaultEmbeddingClass,
		PreferredModel:     types.DefaultEmbeddingModelID,
		RoutePolicy:        types.DefaultRoutePolicy,
		DataClass:          types.DataClassBusinessInternal,
		InputText:          "do not persist this raw embedding input",
		InputHash:          inputHash,
		InputSchemaVersion: 1,
		Dimensions:         4,
		Timeout:            time.Second,
		CorrelationID:      "corr-model-test",
		TraceID:            "trace-model-test",
	}, invocationID, time.Now().UTC())
	if err != nil {
		t.Fatalf("prepare embedding invocation: %v", err)
	}
	return prepared
}

func openModelTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("NEXUSIM_PG_DSN")
	if dsn == "" {
		t.Skip("NEXUSIM_PG_DSN is required for model-gateway postgres integration tests")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("open pg pool: %v", err)
	}
	t.Cleanup(pool.Close)
	applyModelMigration(t, context.Background(), pool)
	return pool
}

func applyModelMigration(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	path := filepath.Join("..", "..", "..", "..", "..", "migrations", "postgres", "model-gateway", "000001_model_gateway_core.sql")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read model migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(content)); err != nil {
		t.Fatalf("apply model migration: %v", err)
	}
}

func resetModelTables(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(ctx, `
TRUNCATE
    model_outbox,
    model_provider_failures,
    model_provider_route_snapshots,
    model_budget_windows,
    model_invocations
CASCADE
`)
	if err != nil {
		t.Fatalf("reset model tables: %v", err)
	}
}

func assertModelOutboxLowSensitive(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	rows, err := pool.Query(ctx, `SELECT aggregate_id, partition_key, payload_json::text FROM model_outbox WHERE tenant_id = 'tenant-model-test'`)
	if err != nil {
		t.Fatalf("query model outbox: %v", err)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		count++
		var aggregateID string
		var partitionKey string
		var payload string
		if err := rows.Scan(&aggregateID, &partitionKey, &payload); err != nil {
			t.Fatalf("scan model outbox: %v", err)
		}
		for _, forbidden := range []string{"source backed answer only", "do not persist this raw output", "do not persist this raw embedding input", "0.11", "0.22", "0.33", "0.44", "api_key", "private_key", "provider_body", "password"} {
			if strings.Contains(payload, forbidden) || strings.Contains(aggregateID, forbidden) || strings.Contains(partitionKey, forbidden) {
				t.Fatalf("model outbox leaked forbidden value %q: aggregate=%s partition=%s payload=%s", forbidden, aggregateID, partitionKey, payload)
			}
		}
		if !strings.Contains(payload, "sha256:") {
			t.Fatalf("model outbox payload missing hash refs: %s", payload)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("model outbox rows: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one model outbox row, got %d", count)
	}
}
