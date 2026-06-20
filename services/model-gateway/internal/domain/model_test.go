package domain

import (
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/model-gateway/internal/types"
)

func TestPrepareTextGenerationNormalizesAndRejectsExternalRoute(t *testing.T) {
	command := types.TextGenerationCommand{
		AuthContext:         types.AuthContext{TenantID: "tenant-1", ServiceName: "rag-service"},
		CallerService:       "rag-service",
		CallerUseCase:       "answer-question",
		IdempotencyKey:      "idem-1",
		ModelClass:          "TEXT_GENERATION",
		PreferredModel:      types.DefaultModelID,
		RoutePolicy:         types.DefaultRoutePolicy,
		DataClass:           "business_internal",
		PromptParts:         []types.PromptPart{{Role: "user", Content: "hello"}},
		PromptHash:          "sha256:prompt",
		PromptSchemaVersion: 1,
		MaxOutputTokens:     128,
		Timeout:             time.Second,
	}
	prepared, err := PrepareTextGeneration(command, "minv_1", time.Unix(1, 0))
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if prepared.Command.DataClass != types.DataClassBusinessInternal {
		t.Fatalf("data class = %s", prepared.Command.DataClass)
	}
	command.RoutePolicy = "internet"
	if _, err := PrepareTextGeneration(command, "minv_2", time.Unix(1, 0)); err == nil {
		t.Fatal("expected non-allowlisted route policy to fail")
	}
}

func TestPrepareEmbeddingUsesInputHashAndRejectsTextModel(t *testing.T) {
	command := types.EmbeddingCommand{
		AuthContext:        types.AuthContext{TenantID: "tenant-1", ServiceName: "vector-index-service"},
		CallerService:      "vector-index-service",
		CallerUseCase:      "embed-vector-item",
		IdempotencyKey:     "idem-embed-1",
		ModelClass:         "embedding",
		PreferredModel:     types.DefaultEmbeddingModelID,
		RoutePolicy:        types.DefaultRoutePolicy,
		DataClass:          "business_internal",
		InputText:          "source chunk text",
		InputHash:          "sha256:input",
		InputSchemaVersion: 1,
		Dimensions:         8,
		Timeout:            time.Second,
	}
	prepared, err := PrepareEmbedding(command, "minv_embed", time.Unix(1, 0))
	if err != nil {
		t.Fatalf("prepare embedding: %v", err)
	}
	invocation := InvocationFromEmbeddingStart(prepared)
	if invocation.RequestType != types.RequestTypeEmbedding || invocation.PromptHash != "sha256:input" {
		t.Fatalf("unexpected embedding invocation: %+v", invocation)
	}
	command.PreferredModel = types.DefaultModelID
	if _, err := PrepareEmbedding(command, "minv_embed_2", time.Unix(1, 0)); err == nil {
		t.Fatal("expected text model to be rejected for embedding")
	}
}
