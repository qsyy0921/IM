package app

import (
	"context"
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/model-gateway/internal/domain"
	"github.com/qsyy0921/IM/services/model-gateway/internal/types"
)

func TestInvokeTextGenerationDoesNotReplayProvider(t *testing.T) {
	repository := &fakeRepository{}
	provider := &fakeProvider{}
	useCase := NewInvokeTextGenerationUseCase(repository, provider, fixedIDs("minv_test"))
	command := validCommand()

	first, err := useCase.Execute(context.Background(), command)
	if err != nil {
		t.Fatalf("first invoke: %v", err)
	}
	if first.Replayed || !first.OutputReturned || first.OutputText == "" {
		t.Fatalf("unexpected first result: %+v", first)
	}
	second, err := useCase.Execute(context.Background(), command)
	if err != nil {
		t.Fatalf("replay invoke: %v", err)
	}
	if !second.Replayed || second.OutputReturned || second.OutputText != "" {
		t.Fatalf("unexpected replay result: %+v", second)
	}
	if provider.calls != 1 {
		t.Fatalf("provider calls = %d, want 1", provider.calls)
	}
}

func TestInvokeTextGenerationRejectsUnallowlistedModel(t *testing.T) {
	useCase := NewInvokeTextGenerationUseCase(&fakeRepository{}, &fakeProvider{}, fixedIDs("minv_test"))
	command := validCommand()
	command.PreferredModel = "external-model"
	if _, err := useCase.Execute(context.Background(), command); err == nil {
		t.Fatal("expected unallowlisted preferred model to fail")
	}
}

func TestInvokeEmbeddingDoesNotReplayProvider(t *testing.T) {
	repository := &fakeRepository{}
	provider := &fakeEmbeddingProvider{}
	useCase := NewInvokeEmbeddingUseCase(repository, provider, fixedIDs("minv_embed"))
	command := validEmbeddingCommand()

	first, err := useCase.Execute(context.Background(), command)
	if err != nil {
		t.Fatalf("first embedding invoke: %v", err)
	}
	if first.Replayed || !first.EmbeddingReturned || len(first.EmbeddingValues) != command.Dimensions {
		t.Fatalf("unexpected first embedding result: %+v", first)
	}
	second, err := useCase.Execute(context.Background(), command)
	if err != nil {
		t.Fatalf("replay embedding invoke: %v", err)
	}
	if !second.Replayed || second.EmbeddingReturned || len(second.EmbeddingValues) != 0 {
		t.Fatalf("unexpected replay embedding result: %+v", second)
	}
	if provider.calls != 1 {
		t.Fatalf("provider calls = %d, want 1", provider.calls)
	}
}

func validCommand() types.TextGenerationCommand {
	return types.TextGenerationCommand{
		AuthContext: types.AuthContext{
			TenantID:    "tenant-1",
			ServiceName: "rag-service",
		},
		CallerService:       "rag-service",
		CallerUseCase:       "answer-question",
		IdempotencyKey:      "idem-1",
		ModelClass:          types.DefaultModelClass,
		PreferredModel:      types.DefaultModelID,
		RoutePolicy:         types.DefaultRoutePolicy,
		DataClass:           types.DataClassBusinessInternal,
		SafetyPolicy:        types.DefaultSafetyPolicy,
		PromptParts:         []types.PromptPart{{Role: "USER", Content: "Summarize source refs only."}},
		PromptHash:          "sha256:prompt",
		PromptSchemaVersion: 1,
		MaxOutputTokens:     128,
		Temperature:         0,
		Timeout:             time.Second,
	}
}

func validEmbeddingCommand() types.EmbeddingCommand {
	return types.EmbeddingCommand{
		AuthContext: types.AuthContext{
			TenantID:    "tenant-1",
			ServiceName: "vector-index-service",
		},
		CallerService:      "vector-index-service",
		CallerUseCase:      "embed-vector-item",
		IdempotencyKey:     "embed-idem-1",
		ModelClass:         types.DefaultEmbeddingClass,
		PreferredModel:     types.DefaultEmbeddingModelID,
		RoutePolicy:        types.DefaultRoutePolicy,
		DataClass:          types.DataClassBusinessInternal,
		InputText:          "source chunk text",
		InputHash:          "sha256:input",
		InputSchemaVersion: 1,
		Dimensions:         4,
		Timeout:            time.Second,
	}
}

type fixedIDs string

func (ids fixedIDs) NewInvocationID() (string, error) {
	return string(ids), nil
}

type fakeProvider struct {
	calls int
}

func (provider *fakeProvider) GenerateText(context.Context, domain.ProviderTextRequest) (domain.ProviderTextResult, error) {
	provider.calls++
	return domain.ProviderTextResult{
		OutputText:          "ok",
		OutputHash:          domain.OutputHash("ok"),
		OutputSchemaVersion: 1,
		TokenUsage:          types.TokenUsage{InputTokens: 2, OutputTokens: 1, TotalTokens: 3},
		Latency:             time.Millisecond,
	}, nil
}

type fakeEmbeddingProvider struct {
	calls int
}

func (provider *fakeEmbeddingProvider) Embed(context.Context, domain.ProviderEmbeddingRequest) (domain.ProviderEmbeddingResult, error) {
	provider.calls++
	return domain.ProviderEmbeddingResult{
		EmbeddingValues:         []float32{0.1, 0.2, 0.3, 0.4},
		EmbeddingHash:           "sha256:embedding",
		OutputSchemaVersion:     1,
		TokenUsage:              types.TokenUsage{InputTokens: 3, TotalTokens: 3},
		EstimatedCostMicrounits: 15,
		Latency:                 time.Millisecond,
	}, nil
}

type fakeRepository struct {
	invocation types.ModelInvocation
	started    bool
}

func (repository *fakeRepository) StartTextInvocation(
	_ context.Context,
	prepared domain.PreparedTextGeneration,
) (types.ModelInvocation, bool, error) {
	if repository.started {
		return repository.invocation, true, nil
	}
	repository.started = true
	repository.invocation = domain.InvocationFromStart(prepared)
	return repository.invocation, false, nil
}

func (repository *fakeRepository) CompleteTextInvocation(_ context.Context, invocation types.ModelInvocation) error {
	repository.invocation = invocation
	return nil
}

func (repository *fakeRepository) StartEmbeddingInvocation(
	_ context.Context,
	prepared domain.PreparedEmbedding,
) (types.ModelInvocation, bool, error) {
	if repository.started {
		return repository.invocation, true, nil
	}
	repository.started = true
	repository.invocation = domain.InvocationFromEmbeddingStart(prepared)
	return repository.invocation, false, nil
}

func (repository *fakeRepository) CompleteEmbeddingInvocation(_ context.Context, invocation types.ModelInvocation) error {
	repository.invocation = invocation
	return nil
}

func (repository *fakeRepository) GetModelInvocation(context.Context, types.TenantID, string) (types.ModelInvocation, error) {
	return repository.invocation, nil
}
