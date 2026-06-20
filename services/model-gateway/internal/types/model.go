package types

import (
	"strings"
	"time"
)

const (
	RequestTypeTextGeneration = "TEXT_GENERATION"

	DataClassLowSensitive      = "LOW_SENSITIVE"
	DataClassBusinessInternal  = "BUSINESS_INTERNAL"
	DataClassUserContent       = "USER_CONTENT"
	DataClassSecuritySensitive = "SECURITY_SENSITIVE"

	InvocationStatusPending   = "PENDING"
	InvocationStatusSucceeded = "SUCCEEDED"
	InvocationStatusFailed    = "FAILED"

	FailureClassNone             = ""
	FailureClassProviderFailed   = "PROVIDER_FAILED"
	FailureClassProviderTimeout  = "PROVIDER_TIMEOUT"
	FailureClassBudgetExhausted  = "BUDGET_EXHAUSTED"
	FailureClassRouteUnavailable = "ROUTE_UNAVAILABLE"

	DefaultModelClass       = "TEXT_GENERATION"
	DefaultRoutePolicy      = "LOCAL_MOCK"
	DefaultSafetyPolicy     = "DEFAULT"
	DefaultRouteVersion     = "local-v1"
	DefaultProviderID       = "local-mock"
	DefaultModelID          = "deterministic-text-v1"
	DefaultTimeout          = 5 * time.Second
	MaxTimeout              = 30 * time.Second
	MaxTextGenerationTokens = 4096
)

type PromptPart struct {
	Role        string
	Content     string
	ContentHash string
}

type TokenUsage struct {
	InputTokens  int
	OutputTokens int
	TotalTokens  int
}

type TextGenerationCommand struct {
	AuthContext         AuthContext
	CallerService       string
	CallerUseCase       string
	RequestID           string
	IdempotencyKey      string
	ModelClass          string
	PreferredModel      string
	RoutePolicy         string
	DataClass           string
	SafetyPolicy        string
	PromptParts         []PromptPart
	PromptHash          string
	PromptSchemaVersion int
	EvidencePackRef     string
	CitationRequired    bool
	MaxOutputTokens     int
	Temperature         float64
	Timeout             time.Duration
	CorrelationID       string
	CausationID         string
	TraceID             string
}

func (command TextGenerationCommand) Validate() error {
	if err := command.AuthContext.ValidateService(); err != nil {
		return err
	}
	if strings.TrimSpace(command.CallerService) == "" {
		return NewInvalidArgument("caller_service is required")
	}
	if strings.TrimSpace(command.CallerUseCase) == "" {
		return NewInvalidArgument("caller_use_case is required")
	}
	if strings.TrimSpace(command.IdempotencyKey) == "" {
		return NewInvalidArgument("idempotency_key is required")
	}
	if !IsValidDataClass(command.DataClass) {
		return NewInvalidArgument("data_class is invalid")
	}
	if strings.TrimSpace(command.PromptHash) == "" {
		return NewInvalidArgument("prompt_hash is required")
	}
	if command.PromptSchemaVersion <= 0 {
		return NewInvalidArgument("prompt_schema_version is required")
	}
	if len(command.PromptParts) == 0 {
		return NewInvalidArgument("prompt_parts is required")
	}
	for _, part := range command.PromptParts {
		if strings.TrimSpace(part.Role) == "" {
			return NewInvalidArgument("prompt_part role is required")
		}
		if strings.TrimSpace(part.Content) == "" {
			return NewInvalidArgument("prompt_part content is required")
		}
	}
	if command.MaxOutputTokens <= 0 || command.MaxOutputTokens > MaxTextGenerationTokens {
		return NewInvalidArgument("max_output_tokens is invalid")
	}
	if command.Temperature < 0 || command.Temperature > 2 {
		return NewInvalidArgument("temperature is invalid")
	}
	if command.Timeout <= 0 || command.Timeout > MaxTimeout {
		return NewInvalidArgument("timeout_ms is invalid")
	}
	return nil
}

func (command TextGenerationCommand) Normalized() TextGenerationCommand {
	command.CallerService = strings.TrimSpace(command.CallerService)
	command.CallerUseCase = strings.TrimSpace(command.CallerUseCase)
	command.RequestID = strings.TrimSpace(command.RequestID)
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	command.ModelClass = strings.ToUpper(strings.TrimSpace(command.ModelClass))
	if command.ModelClass == "" {
		command.ModelClass = DefaultModelClass
	}
	command.PreferredModel = strings.TrimSpace(command.PreferredModel)
	if command.PreferredModel == "" {
		command.PreferredModel = DefaultModelID
	}
	command.RoutePolicy = strings.ToUpper(strings.TrimSpace(command.RoutePolicy))
	if command.RoutePolicy == "" {
		command.RoutePolicy = DefaultRoutePolicy
	}
	command.DataClass = strings.ToUpper(strings.TrimSpace(command.DataClass))
	command.SafetyPolicy = strings.ToUpper(strings.TrimSpace(command.SafetyPolicy))
	if command.SafetyPolicy == "" {
		command.SafetyPolicy = DefaultSafetyPolicy
	}
	command.PromptHash = strings.TrimSpace(command.PromptHash)
	command.EvidencePackRef = strings.TrimSpace(command.EvidencePackRef)
	command.CorrelationID = strings.TrimSpace(command.CorrelationID)
	command.CausationID = strings.TrimSpace(command.CausationID)
	command.TraceID = strings.TrimSpace(command.TraceID)
	if command.TraceID == "" {
		command.TraceID = strings.TrimSpace(command.AuthContext.TraceID)
	}
	for index := range command.PromptParts {
		command.PromptParts[index].Role = strings.ToUpper(strings.TrimSpace(command.PromptParts[index].Role))
		command.PromptParts[index].ContentHash = strings.TrimSpace(command.PromptParts[index].ContentHash)
	}
	return command
}

type GetModelInvocationCommand struct {
	AuthContext  AuthContext
	InvocationID string
}

func (command GetModelInvocationCommand) Validate() error {
	if err := command.AuthContext.ValidateService(); err != nil {
		return err
	}
	if strings.TrimSpace(command.InvocationID) == "" {
		return NewInvalidArgument("invocation_id is required")
	}
	return nil
}

func (command GetModelInvocationCommand) Normalized() GetModelInvocationCommand {
	command.InvocationID = strings.TrimSpace(command.InvocationID)
	return command
}

type ModelInvocation struct {
	TenantID                TenantID
	InvocationID            string
	IdempotencyKey          string
	CommandHash             string
	CallerService           string
	CallerUseCase           string
	RequestType             string
	DataClass               string
	ProviderID              string
	ModelID                 string
	RouteVersion            string
	PromptHash              string
	OutputHash              string
	OutputSchemaVersion     int
	TokenUsage              TokenUsage
	EstimatedCostMicrounits int64
	Status                  string
	FailureClass            string
	FallbackUsed            bool
	ProviderLatency         time.Duration
	Timeout                 time.Duration
	MaxOutputTokens         int
	PromptSchemaVersion     int
	CorrelationID           string
	CausationID             string
	TraceID                 string
	CreatedAt               time.Time
	CompletedAt             time.Time
}

type TextGenerationResult struct {
	Invocation     ModelInvocation
	OutputText     string
	OutputReturned bool
	Replayed       bool
}

func IsValidDataClass(value string) bool {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case DataClassLowSensitive, DataClassBusinessInternal, DataClassUserContent, DataClassSecuritySensitive:
		return true
	default:
		return false
	}
}
