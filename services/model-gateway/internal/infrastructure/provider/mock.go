package provider

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/qsyy0921/IM/services/model-gateway/internal/domain"
	"github.com/qsyy0921/IM/services/model-gateway/internal/types"
)

type MockTextProvider struct{}

func NewMockTextProvider() MockTextProvider {
	return MockTextProvider{}
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
