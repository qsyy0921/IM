package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/qsyy0921/IM/services/model-gateway/internal/domain"
	"github.com/qsyy0921/IM/services/model-gateway/internal/types"
)

type InvokeTextGenerationUseCase struct {
	repository Repository
	provider   TextProvider
	ids        InvocationIDGenerator
}

func NewInvokeTextGenerationUseCase(
	repository Repository,
	provider TextProvider,
	ids InvocationIDGenerator,
) *InvokeTextGenerationUseCase {
	return &InvokeTextGenerationUseCase{repository: repository, provider: provider, ids: ids}
}

func (useCase *InvokeTextGenerationUseCase) Execute(
	ctx context.Context,
	command types.TextGenerationCommand,
) (types.TextGenerationResult, error) {
	if useCase.repository == nil {
		return types.TextGenerationResult{}, types.NewUnavailable("model repository is not configured")
	}
	if useCase.provider == nil {
		return types.TextGenerationResult{}, types.NewUnavailable("model provider is not configured")
	}
	prepared, err := domain.PrepareTextGeneration(command, useCase.ids.NewInvocationID(), time.Now())
	if err != nil {
		return types.TextGenerationResult{}, err
	}
	started, replayed, err := useCase.repository.StartTextInvocation(ctx, prepared)
	if err != nil {
		return types.TextGenerationResult{}, err
	}
	if replayed {
		return types.TextGenerationResult{
			Invocation:     started,
			Replayed:       true,
			OutputReturned: false,
		}, nil
	}
	providerCtx, cancel := context.WithTimeout(ctx, prepared.Command.Timeout)
	defer cancel()
	providerStart := time.Now()
	providerResult, err := useCase.provider.GenerateText(providerCtx, domain.ProviderRequest(prepared))
	if err != nil {
		failureClass := types.FailureClassProviderFailed
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(providerCtx.Err(), context.DeadlineExceeded) {
			failureClass = types.FailureClassProviderTimeout
		}
		failed := domain.InvocationFromFailure(started, failureClass, time.Since(providerStart), time.Now())
		_ = useCase.repository.CompleteTextInvocation(ctx, failed)
		if failureClass == types.FailureClassProviderTimeout {
			return types.TextGenerationResult{}, types.NewDeadlineExceeded("model provider timeout")
		}
		return types.TextGenerationResult{}, types.NewUnavailable("model provider failed")
	}
	if providerResult.OutputHash == "" {
		providerResult.OutputHash = domain.OutputHash(providerResult.OutputText)
	}
	completed := domain.InvocationFromSuccess(started, providerResult, time.Now())
	if err := useCase.repository.CompleteTextInvocation(ctx, completed); err != nil {
		return types.TextGenerationResult{}, err
	}
	return types.TextGenerationResult{
		Invocation:     completed,
		OutputText:     providerResult.OutputText,
		OutputReturned: true,
	}, nil
}

type GetModelInvocationUseCase struct {
	repository Repository
}

func NewGetModelInvocationUseCase(repository Repository) *GetModelInvocationUseCase {
	return &GetModelInvocationUseCase{repository: repository}
}

func (useCase *GetModelInvocationUseCase) Execute(
	ctx context.Context,
	command types.GetModelInvocationCommand,
) (types.ModelInvocation, error) {
	if useCase.repository == nil {
		return types.ModelInvocation{}, types.NewUnavailable("model repository is not configured")
	}
	if err := command.Validate(); err != nil {
		return types.ModelInvocation{}, err
	}
	normalized := command.Normalized()
	return useCase.repository.GetModelInvocation(ctx, normalized.AuthContext.TenantID, normalized.InvocationID)
}

type RandomInvocationIDGenerator struct{}

func NewRandomInvocationIDGenerator() RandomInvocationIDGenerator {
	return RandomInvocationIDGenerator{}
}

func (RandomInvocationIDGenerator) NewInvocationID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "minv_fallback"
	}
	return "minv_" + hex.EncodeToString(value[:])
}
