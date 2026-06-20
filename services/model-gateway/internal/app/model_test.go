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

type fixedIDs string

func (ids fixedIDs) NewInvocationID() string {
	return string(ids)
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

func (repository *fakeRepository) GetModelInvocation(context.Context, types.TenantID, string) (types.ModelInvocation, error) {
	return repository.invocation, nil
}
