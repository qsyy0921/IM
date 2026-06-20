package grpc

import (
	"context"
	"errors"
	"time"

	modelv1 "github.com/qsyy0921/IM/api/proto/nexusim/model/v1"
	"github.com/qsyy0921/IM/services/model-gateway/internal/types"
	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type InvokeTextGenerationExecutor interface {
	Execute(context.Context, types.TextGenerationCommand) (types.TextGenerationResult, error)
}

type InvokeEmbeddingExecutor interface {
	Execute(context.Context, types.EmbeddingCommand) (types.EmbeddingResult, error)
}

type GetModelInvocationExecutor interface {
	Execute(context.Context, types.GetModelInvocationCommand) (types.ModelInvocation, error)
}

type Server struct {
	modelv1.UnimplementedModelGatewayServiceServer
	invokeTextGeneration InvokeTextGenerationExecutor
	invokeEmbedding      InvokeEmbeddingExecutor
	getModelInvocation   GetModelInvocationExecutor
}

func NewServer(
	invokeTextGeneration InvokeTextGenerationExecutor,
	invokeEmbedding InvokeEmbeddingExecutor,
	getModelInvocation GetModelInvocationExecutor,
) *Server {
	return &Server{
		invokeTextGeneration: invokeTextGeneration,
		invokeEmbedding:      invokeEmbedding,
		getModelInvocation:   getModelInvocation,
	}
}

func Register(registrar grpcgo.ServiceRegistrar, server *Server) {
	modelv1.RegisterModelGatewayServiceServer(registrar, server)
}

func (server *Server) InvokeTextGeneration(
	ctx context.Context,
	request *modelv1.InvokeTextGenerationRequest,
) (*modelv1.InvokeTextGenerationResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	auth, ok := authFromProto(ctx, request.GetAuthContext())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "auth_context is required")
	}
	parts := make([]types.PromptPart, 0, len(request.GetPromptParts()))
	for _, part := range request.GetPromptParts() {
		parts = append(parts, types.PromptPart{
			Role:        part.GetRole(),
			Content:     part.GetContent(),
			ContentHash: part.GetContentHash(),
		})
	}
	result, err := server.invokeTextGeneration.Execute(ctx, types.TextGenerationCommand{
		AuthContext:         auth,
		CallerService:       request.GetCallerService(),
		CallerUseCase:       request.GetCallerUseCase(),
		RequestID:           request.GetRequestId(),
		IdempotencyKey:      request.GetIdempotencyKey(),
		ModelClass:          request.GetModelClass(),
		PreferredModel:      request.GetPreferredModel(),
		RoutePolicy:         request.GetRoutePolicy(),
		DataClass:           request.GetDataClass(),
		SafetyPolicy:        request.GetSafetyPolicy(),
		PromptParts:         parts,
		PromptHash:          request.GetPromptHash(),
		PromptSchemaVersion: int(request.GetPromptSchemaVersion()),
		EvidencePackRef:     request.GetEvidencePackRef(),
		CitationRequired:    request.GetCitationRequired(),
		MaxOutputTokens:     int(request.GetMaxOutputTokens()),
		Temperature:         request.GetTemperature(),
		Timeout:             durationFromMillis(request.GetTimeoutMs()),
		CorrelationID:       request.GetCorrelationId(),
		CausationID:         request.GetCausationId(),
		TraceID:             request.GetTraceId(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	invocation := result.Invocation
	return &modelv1.InvokeTextGenerationResponse{
		InvocationId:            invocation.InvocationID,
		ProviderId:              invocation.ProviderID,
		ModelId:                 invocation.ModelID,
		OutputText:              result.OutputText,
		OutputHash:              invocation.OutputHash,
		OutputSchemaVersion:     int32(invocation.OutputSchemaVersion),
		TokenUsage:              tokenUsageToProto(invocation.TokenUsage),
		EstimatedCostMicrounits: invocation.EstimatedCostMicrounits,
		FailureClass:            invocation.FailureClass,
		FallbackUsed:            invocation.FallbackUsed,
		ProviderLatencyMs:       invocation.ProviderLatency.Milliseconds(),
		Replayed:                result.Replayed,
		OutputReturned:          result.OutputReturned,
	}, nil
}

func (server *Server) InvokeEmbedding(
	ctx context.Context,
	request *modelv1.InvokeEmbeddingRequest,
) (*modelv1.InvokeEmbeddingResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	auth, ok := authFromProto(ctx, request.GetAuthContext())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "auth_context is required")
	}
	result, err := server.invokeEmbedding.Execute(ctx, types.EmbeddingCommand{
		AuthContext:        auth,
		CallerService:      request.GetCallerService(),
		CallerUseCase:      request.GetCallerUseCase(),
		RequestID:          request.GetRequestId(),
		IdempotencyKey:     request.GetIdempotencyKey(),
		ModelClass:         request.GetModelClass(),
		PreferredModel:     request.GetPreferredModel(),
		RoutePolicy:        request.GetRoutePolicy(),
		DataClass:          request.GetDataClass(),
		InputText:          request.GetInputText(),
		InputHash:          request.GetInputHash(),
		InputSchemaVersion: int(request.GetInputSchemaVersion()),
		Dimensions:         int(request.GetDimensions()),
		Timeout:            durationFromMillis(request.GetTimeoutMs()),
		CorrelationID:      request.GetCorrelationId(),
		CausationID:        request.GetCausationId(),
		TraceID:            request.GetTraceId(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	invocation := result.Invocation
	return &modelv1.InvokeEmbeddingResponse{
		InvocationId:            invocation.InvocationID,
		ProviderId:              invocation.ProviderID,
		ModelId:                 invocation.ModelID,
		EmbeddingValues:         result.EmbeddingValues,
		EmbeddingHash:           invocation.OutputHash,
		Dimensions:              int32(len(result.EmbeddingValues)),
		TokenUsage:              tokenUsageToProto(invocation.TokenUsage),
		EstimatedCostMicrounits: invocation.EstimatedCostMicrounits,
		FailureClass:            invocation.FailureClass,
		FallbackUsed:            invocation.FallbackUsed,
		ProviderLatencyMs:       invocation.ProviderLatency.Milliseconds(),
		Replayed:                result.Replayed,
		EmbeddingReturned:       result.EmbeddingReturned,
	}, nil
}

func (server *Server) GetModelInvocation(
	ctx context.Context,
	request *modelv1.GetModelInvocationRequest,
) (*modelv1.GetModelInvocationResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	auth, ok := authFromProto(ctx, request.GetAuthContext())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "auth_context is required")
	}
	invocation, err := server.getModelInvocation.Execute(ctx, types.GetModelInvocationCommand{
		AuthContext:  auth,
		InvocationID: request.GetInvocationId(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &modelv1.GetModelInvocationResponse{Invocation: invocationToProto(invocation)}, nil
}

func authFromProto(ctx context.Context, auth *modelv1.AuthContext) (types.AuthContext, bool) {
	if verified, ok := verifiedAuthFromContext(ctx); ok {
		return verified, true
	}
	if auth == nil {
		return types.AuthContext{}, false
	}
	return types.AuthContext{
		TenantID:    types.TenantID(auth.GetTenantId()),
		UserID:      auth.GetUserId(),
		ServiceName: auth.GetServiceName(),
		InstanceRef: auth.GetInstanceRef(),
		TraceID:     auth.GetTraceId(),
		RequestID:   auth.GetRequestId(),
	}, true
}

func invocationToProto(invocation types.ModelInvocation) *modelv1.ModelInvocation {
	return &modelv1.ModelInvocation{
		TenantId:                string(invocation.TenantID),
		InvocationId:            invocation.InvocationID,
		IdempotencyKey:          invocation.IdempotencyKey,
		CallerService:           invocation.CallerService,
		CallerUseCase:           invocation.CallerUseCase,
		RequestType:             invocation.RequestType,
		DataClass:               invocation.DataClass,
		ProviderId:              invocation.ProviderID,
		ModelId:                 invocation.ModelID,
		RouteVersion:            invocation.RouteVersion,
		PromptHash:              invocation.PromptHash,
		OutputHash:              invocation.OutputHash,
		TokenUsage:              tokenUsageToProto(invocation.TokenUsage),
		EstimatedCostMicrounits: invocation.EstimatedCostMicrounits,
		Status:                  invocation.Status,
		FailureClass:            invocation.FailureClass,
		FallbackUsed:            invocation.FallbackUsed,
		ProviderLatencyMs:       invocation.ProviderLatency.Milliseconds(),
		CorrelationId:           invocation.CorrelationID,
		CausationId:             invocation.CausationID,
		TraceId:                 invocation.TraceID,
		CreatedAtUnixMs:         timeToUnixMillis(invocation.CreatedAt),
		CompletedAtUnixMs:       timeToUnixMillis(invocation.CompletedAt),
	}
}

func tokenUsageToProto(usage types.TokenUsage) *modelv1.TokenUsage {
	return &modelv1.TokenUsage{
		InputTokens:  int32(usage.InputTokens),
		OutputTokens: int32(usage.OutputTokens),
		TotalTokens:  int32(usage.TotalTokens),
	}
}

func durationFromMillis(value int64) time.Duration {
	if value <= 0 {
		return 0
	}
	return time.Duration(value) * time.Millisecond
}

func timeToUnixMillis(value time.Time) int64 {
	if value.IsZero() {
		return 0
	}
	return value.UTC().UnixMilli()
}

func grpcError(err error) error {
	switch {
	case errors.Is(err, types.ErrInvalidArgument):
		return status.Error(codes.InvalidArgument, "invalid argument")
	case errors.Is(err, types.ErrPermissionDenied):
		return status.Error(codes.PermissionDenied, "permission denied")
	case errors.Is(err, types.ErrNotFound):
		return status.Error(codes.NotFound, "model invocation not found")
	case errors.Is(err, types.ErrFailedPrecondition):
		return status.Error(codes.FailedPrecondition, "model precondition failed")
	case errors.Is(err, types.ErrResourceExhausted):
		return status.Error(codes.ResourceExhausted, "model resource exhausted")
	case errors.Is(err, types.ErrDeadlineExceeded):
		return status.Error(codes.DeadlineExceeded, "model provider timeout")
	case errors.Is(err, types.ErrUnavailable), errors.Is(err, types.ErrDBReadFailed), errors.Is(err, types.ErrDBWriteFailed):
		return status.Error(codes.Unavailable, "model temporarily unavailable")
	default:
		return status.Error(codes.Internal, "model internal error")
	}
}
