package rpc

import (
	"context"
	"reflect"
	"testing"
	"time"

	modelv1 "github.com/qsyy0921/IM/api/proto/nexusim/model/v1"
	"github.com/qsyy0921/IM/services/vector-index-service/internal/types"
	"google.golang.org/grpc"
)

func TestModelGatewayClientEmbedPreservesEmbeddingValues(t *testing.T) {
	client := NewModelGatewayClient(fakeModelGatewayClient{
		embedding: &modelv1.InvokeEmbeddingResponse{
			InvocationId:      "minv_embed_1",
			ProviderId:        "mock",
			ModelId:           "deterministic-embedding-v1",
			EmbeddingValues:   []float32{0.25, -0.5, 0.75},
			EmbeddingHash:     "sha256:embedding",
			Dimensions:        3,
			EmbeddingReturned: true,
		},
	}, time.Second)

	result, err := client.Embed(context.Background(), types.VectorEmbeddingTask{
		AuthContext: types.AuthContext{
			TenantID:    "tenant-vector",
			ServiceName: types.AllowedCallerVectorIndex,
			RequestID:   "req-vector",
		},
		InputText:          "chunk text",
		InputHash:          "sha256:input",
		InputSchemaVersion: 1,
		EmbeddingModelRef:  "deterministic-embedding-v1",
		Dimension:          3,
		DataClass:          "BUSINESS_INTERNAL",
		IdempotencyKey:     "idem-vector",
	})
	if err != nil {
		t.Fatalf("embed: %v", err)
	}
	if !reflect.DeepEqual(result.EmbeddingValues, []float32{0.25, -0.5, 0.75}) {
		t.Fatalf("embedding values were not preserved: %+v", result.EmbeddingValues)
	}
	if result.EmbeddingVectorHash != "sha256:embedding" || result.Dimension != 3 {
		t.Fatalf("unexpected embedding metadata: %+v", result)
	}
}

func TestModelGatewayClientEmbedRejectsReturnedDimensionMismatch(t *testing.T) {
	client := NewModelGatewayClient(fakeModelGatewayClient{
		embedding: &modelv1.InvokeEmbeddingResponse{
			InvocationId:      "minv_embed_1",
			ProviderId:        "mock",
			ModelId:           "deterministic-embedding-v1",
			EmbeddingValues:   []float32{0.25},
			EmbeddingHash:     "sha256:embedding",
			Dimensions:        3,
			EmbeddingReturned: true,
		},
	}, time.Second)

	_, err := client.Embed(context.Background(), types.VectorEmbeddingTask{
		AuthContext: types.AuthContext{
			TenantID:    "tenant-vector",
			ServiceName: types.AllowedCallerVectorIndex,
			RequestID:   "req-vector",
		},
		InputText:          "chunk text",
		InputHash:          "sha256:input",
		InputSchemaVersion: 1,
		EmbeddingModelRef:  "deterministic-embedding-v1",
		Dimension:          3,
		DataClass:          "BUSINESS_INTERNAL",
		IdempotencyKey:     "idem-vector",
	})
	if err == nil {
		t.Fatal("expected incomplete embedding response error")
	}
}

type fakeModelGatewayClient struct {
	modelv1.ModelGatewayServiceClient
	embedding *modelv1.InvokeEmbeddingResponse
	err       error
}

func (client fakeModelGatewayClient) InvokeEmbedding(
	_ context.Context,
	_ *modelv1.InvokeEmbeddingRequest,
	_ ...grpc.CallOption,
) (*modelv1.InvokeEmbeddingResponse, error) {
	return client.embedding, client.err
}
