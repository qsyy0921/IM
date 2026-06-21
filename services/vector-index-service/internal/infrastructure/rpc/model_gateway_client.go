package rpc

import (
	"context"
	"errors"
	"strings"
	"time"

	modelv1 "github.com/qsyy0921/IM/api/proto/nexusim/model/v1"
	"github.com/qsyy0921/IM/services/vector-index-service/internal/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

type ModelGatewayClient struct {
	client  modelv1.ModelGatewayServiceClient
	timeout time.Duration
}

func NewModelGatewayClient(client modelv1.ModelGatewayServiceClient, timeout time.Duration) ModelGatewayClient {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return ModelGatewayClient{client: client, timeout: timeout}
}

func DialModelGatewayClient(
	_ context.Context,
	addr string,
	timeout time.Duration,
) (ModelGatewayClient, func() error, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return ModelGatewayClient{}, nil, errors.New("model-gateway address is required")
	}
	conn, err := grpc.NewClient(
		"passthrough:///"+addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return ModelGatewayClient{}, nil, err
	}
	return NewModelGatewayClient(modelv1.NewModelGatewayServiceClient(conn), timeout), conn.Close, nil
}

func (client ModelGatewayClient) Embed(
	ctx context.Context,
	task types.VectorEmbeddingTask,
) (types.VectorEmbeddingResult, error) {
	if client.client == nil {
		return types.VectorEmbeddingResult{}, types.NewUnavailable("model-gateway client is not configured")
	}
	timeout := task.Timeout
	if timeout <= 0 {
		timeout = client.timeout
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	response, err := client.client.InvokeEmbedding(callCtx, &modelv1.InvokeEmbeddingRequest{
		AuthContext: &modelv1.AuthContext{
			TenantId:    string(task.AuthContext.TenantID),
			ServiceName: types.AllowedCallerVectorIndex,
			InstanceRef: firstNonEmpty(task.AuthContext.InstanceRef, "embedding-worker"),
			TraceId:     firstNonEmpty(task.TraceID, task.AuthContext.TraceID),
			RequestId:   firstNonEmpty(task.AuthContext.RequestID, task.IdempotencyKey),
		},
		CallerService:      types.AllowedCallerVectorIndex,
		CallerUseCase:      "vector.embedding-worker",
		RequestId:          firstNonEmpty(task.AuthContext.RequestID, task.IdempotencyKey),
		IdempotencyKey:     "vector-embedding:" + task.IdempotencyKey,
		ModelClass:         "EMBEDDING",
		PreferredModel:     task.EmbeddingModelRef,
		RoutePolicy:        "LOCAL_MOCK",
		DataClass:          task.DataClass,
		InputText:          task.InputText,
		InputHash:          task.InputHash,
		InputSchemaVersion: int32(task.InputSchemaVersion),
		Dimensions:         int32(task.Dimension),
		TimeoutMs:          timeout.Milliseconds(),
		CorrelationId:      task.CorrelationID,
		CausationId:        task.CausationID,
		TraceId:            task.TraceID,
	})
	if err != nil {
		return types.VectorEmbeddingResult{}, mapModelGatewayError(err)
	}
	if response.GetEmbeddingHash() == "" || response.GetDimensions() <= 0 {
		return types.VectorEmbeddingResult{}, types.NewUnavailable("model-gateway embedding response is incomplete")
	}
	values := append([]float32(nil), response.GetEmbeddingValues()...)
	if response.GetEmbeddingReturned() && len(values) != int(response.GetDimensions()) {
		return types.VectorEmbeddingResult{}, types.NewUnavailable("model-gateway embedding response is incomplete")
	}
	return types.VectorEmbeddingResult{
		InvocationID:        response.GetInvocationId(),
		ProviderID:          response.GetProviderId(),
		ModelID:             response.GetModelId(),
		EmbeddingValues:     values,
		EmbeddingVectorHash: response.GetEmbeddingHash(),
		Dimension:           int(response.GetDimensions()),
		Replayed:            response.GetReplayed(),
		EmbeddingReturned:   response.GetEmbeddingReturned(),
	}, nil
}

func mapModelGatewayError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return types.NewUnavailable("model-gateway temporarily unavailable")
	}
	st, ok := status.FromError(err)
	if !ok {
		return types.NewUnavailable("model-gateway temporarily unavailable")
	}
	switch st.Code() {
	case codes.InvalidArgument:
		return types.NewInvalidArgument("model-gateway request invalid")
	case codes.PermissionDenied:
		return types.NewPermissionDenied("model-gateway permission denied")
	case codes.FailedPrecondition:
		return types.NewFailedPrecondition("model-gateway precondition failed")
	case codes.NotFound:
		return types.NewNotFound("model-gateway target not found")
	case codes.ResourceExhausted:
		return types.NewFailedPrecondition("model-gateway resource exhausted")
	case codes.Unavailable, codes.DeadlineExceeded:
		return types.NewUnavailable("model-gateway temporarily unavailable")
	default:
		return types.NewUnavailable("model-gateway temporarily unavailable")
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
