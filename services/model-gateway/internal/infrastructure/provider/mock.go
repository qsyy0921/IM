package provider

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"strings"
	"time"

	"github.com/qsyy0921/IM/services/model-gateway/internal/domain"
	"github.com/qsyy0921/IM/services/model-gateway/internal/types"
)

type MockTextProvider struct{}

type MockEmbeddingProvider struct{}

func NewMockTextProvider() MockTextProvider {
	return MockTextProvider{}
}

func NewMockEmbeddingProvider() MockEmbeddingProvider {
	return MockEmbeddingProvider{}
}

func (MockTextProvider) GenerateText(
	ctx context.Context,
	request domain.ProviderTextRequest,
) (domain.ProviderTextResult, error) {
	started := time.Now()
	select {
	case <-ctx.Done():
		return domain.ProviderTextResult{}, ctx.Err()
	default:
	}
	suffix := request.PromptHash
	if len(suffix) > 19 {
		suffix = suffix[len(suffix)-16:]
	}
	output := fmt.Sprintf("deterministic model response for %s", suffix)
	inputTokens := estimateTokens(request.PromptParts)
	outputTokens := estimateTextTokens(output)
	return domain.ProviderTextResult{
		OutputText:              output,
		OutputHash:              domain.OutputHash(output),
		OutputSchemaVersion:     1,
		TokenUsage:              types.TokenUsage{InputTokens: inputTokens, OutputTokens: outputTokens, TotalTokens: inputTokens + outputTokens},
		EstimatedCostMicrounits: int64(inputTokens+outputTokens) * 10,
		Latency:                 time.Since(started),
		FallbackUsed:            false,
	}, nil
}

func (MockEmbeddingProvider) Embed(
	ctx context.Context,
	request domain.ProviderEmbeddingRequest,
) (domain.ProviderEmbeddingResult, error) {
	started := time.Now()
	select {
	case <-ctx.Done():
		return domain.ProviderEmbeddingResult{}, ctx.Err()
	default:
	}
	dimensions := request.Dimensions
	if dimensions <= 0 {
		dimensions = types.DefaultEmbeddingDims
	}
	values := make([]float32, dimensions)
	seed := request.InputHash + ":" + request.ModelID
	for index := range values {
		sum := sha256.Sum256([]byte(fmt.Sprintf("%s:%d", seed, index)))
		raw := binary.BigEndian.Uint32(sum[:4])
		values[index] = float32(raw%2000000)/1000000 - 1
	}
	inputTokens := estimateTextTokens(request.InputText)
	return domain.ProviderEmbeddingResult{
		EmbeddingValues:         values,
		EmbeddingHash:           domain.EmbeddingHash(values),
		OutputSchemaVersion:     1,
		TokenUsage:              types.TokenUsage{InputTokens: inputTokens, OutputTokens: 0, TotalTokens: inputTokens},
		EstimatedCostMicrounits: int64(inputTokens) * 5,
		Latency:                 time.Since(started),
		FallbackUsed:            false,
	}, nil
}

func estimateTokens(parts []types.PromptPart) int {
	total := 0
	for _, part := range parts {
		total += estimateTextTokens(part.Content)
	}
	if total == 0 {
		return 1
	}
	return total
}

func estimateTextTokens(value string) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	words := strings.Fields(value)
	if len(words) == 0 {
		return 1
	}
	return len(words)
}
