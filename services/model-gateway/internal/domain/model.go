package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"github.com/qsyy0921/IM/services/model-gateway/internal/types"
)

type PreparedTextGeneration struct {
	Command      types.TextGenerationCommand
	CommandHash  string
	InvocationID string
	ProviderID   string
	ModelID      string
	RouteVersion string
	StartedAt    time.Time
}

type ProviderTextRequest struct {
	ProviderID      string
	ModelID         string
	PromptParts     []types.PromptPart
	PromptHash      string
	MaxOutputTokens int
	Temperature     float64
	Timeout         time.Duration
}

type ProviderTextResult struct {
	OutputText              string
	OutputHash              string
	OutputSchemaVersion     int
	TokenUsage              types.TokenUsage
	EstimatedCostMicrounits int64
	Latency                 time.Duration
	FallbackUsed            bool
}

func PrepareTextGeneration(
	command types.TextGenerationCommand,
	invocationID string,
	now time.Time,
) (PreparedTextGeneration, error) {
	if command.Timeout == 0 {
		command.Timeout = types.DefaultTimeout
	}
	if command.MaxOutputTokens == 0 {
		command.MaxOutputTokens = 512
	}
	normalized := command.Normalized()
	if err := normalized.Validate(); err != nil {
		return PreparedTextGeneration{}, err
	}
	if normalized.ModelClass != types.DefaultModelClass {
		return PreparedTextGeneration{}, types.NewFailedPrecondition("model_class is not allowlisted")
	}
	if normalized.RoutePolicy != types.DefaultRoutePolicy {
		return PreparedTextGeneration{}, types.NewFailedPrecondition("route_policy is not allowlisted")
	}
	if normalized.PreferredModel != types.DefaultModelID {
		return PreparedTextGeneration{}, types.NewFailedPrecondition("preferred_model is not allowlisted")
	}
	hash, err := commandHash(normalized)
	if err != nil {
		return PreparedTextGeneration{}, err
	}
	return PreparedTextGeneration{
		Command:      normalized,
		CommandHash:  hash,
		InvocationID: strings.TrimSpace(invocationID),
		ProviderID:   types.DefaultProviderID,
		ModelID:      types.DefaultModelID,
		RouteVersion: types.DefaultRouteVersion,
		StartedAt:    now.UTC(),
	}, nil
}

func ProviderRequest(prepared PreparedTextGeneration) ProviderTextRequest {
	command := prepared.Command
	return ProviderTextRequest{
		ProviderID:      prepared.ProviderID,
		ModelID:         prepared.ModelID,
		PromptParts:     command.PromptParts,
		PromptHash:      command.PromptHash,
		MaxOutputTokens: command.MaxOutputTokens,
		Temperature:     command.Temperature,
		Timeout:         command.Timeout,
	}
}

func InvocationFromStart(prepared PreparedTextGeneration) types.ModelInvocation {
	command := prepared.Command
	return types.ModelInvocation{
		TenantID:            command.AuthContext.TenantID,
		InvocationID:        prepared.InvocationID,
		IdempotencyKey:      command.IdempotencyKey,
		CommandHash:         prepared.CommandHash,
		CallerService:       command.CallerService,
		CallerUseCase:       command.CallerUseCase,
		RequestType:         types.RequestTypeTextGeneration,
		DataClass:           command.DataClass,
		ProviderID:          prepared.ProviderID,
		ModelID:             prepared.ModelID,
		RouteVersion:        prepared.RouteVersion,
		PromptHash:          command.PromptHash,
		Status:              types.InvocationStatusPending,
		Timeout:             command.Timeout,
		MaxOutputTokens:     command.MaxOutputTokens,
		PromptSchemaVersion: command.PromptSchemaVersion,
		CorrelationID:       command.CorrelationID,
		CausationID:         command.CausationID,
		TraceID:             command.TraceID,
		CreatedAt:           prepared.StartedAt,
	}
}

func InvocationFromSuccess(
	started types.ModelInvocation,
	result ProviderTextResult,
	completedAt time.Time,
) types.ModelInvocation {
	started.OutputHash = result.OutputHash
	started.OutputSchemaVersion = result.OutputSchemaVersion
	started.TokenUsage = result.TokenUsage
	started.EstimatedCostMicrounits = result.EstimatedCostMicrounits
	started.Status = types.InvocationStatusSucceeded
	started.FailureClass = types.FailureClassNone
	started.FallbackUsed = result.FallbackUsed
	started.ProviderLatency = result.Latency
	started.CompletedAt = completedAt.UTC()
	return started
}

func InvocationFromFailure(
	started types.ModelInvocation,
	failureClass string,
	latency time.Duration,
	completedAt time.Time,
) types.ModelInvocation {
	started.Status = types.InvocationStatusFailed
	started.FailureClass = strings.TrimSpace(failureClass)
	if started.FailureClass == "" {
		started.FailureClass = types.FailureClassProviderFailed
	}
	started.ProviderLatency = latency
	started.CompletedAt = completedAt.UTC()
	return started
}

func OutputHash(output string) string {
	return HashRef(output)
}

func HashRef(value string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func commandHash(command types.TextGenerationCommand) (string, error) {
	payload := map[string]any{
		"tenant_id":             string(command.AuthContext.TenantID),
		"caller_service":        command.CallerService,
		"caller_use_case":       command.CallerUseCase,
		"idempotency_key":       command.IdempotencyKey,
		"model_class":           command.ModelClass,
		"preferred_model":       command.PreferredModel,
		"route_policy":          command.RoutePolicy,
		"data_class":            command.DataClass,
		"safety_policy":         command.SafetyPolicy,
		"prompt_hash":           command.PromptHash,
		"prompt_schema_version": command.PromptSchemaVersion,
		"evidence_pack_ref":     command.EvidencePackRef,
		"citation_required":     command.CitationRequired,
		"max_output_tokens":     command.MaxOutputTokens,
		"temperature":           command.Temperature,
		"timeout_ms":            command.Timeout.Milliseconds(),
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", types.NewInvalidArgument("model command hash payload invalid")
	}
	return HashRef(string(encoded)), nil
}
