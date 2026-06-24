package rpc

import (
	"context"
	"errors"
	"testing"
	"time"

	vectorv1 "github.com/qsyy0921/IM/api/proto/nexusim/vector/v1"
	"github.com/qsyy0921/IM/services/retrieval-gateway/internal/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestVectorClientSearchVectorsMapsLowSensitiveQuery(t *testing.T) {
	fake := &fakeVectorIndexServiceClient{
		response: &vectorv1.SearchVectorsResponse{
			Results: []*vectorv1.VectorSearchResult{{
				VectorItemRef:     "vitem-1",
				SourceRefHash:     "sha256:source",
				SourceService:     "memory-service",
				CollectionType:    types.VectorCollectionMemoryEvent,
				Score:             0.91,
				VisibilityVersion: 7,
				TombstoneStatus:   "NONE",
			}},
		},
	}
	result, err := NewVectorClient(fake, time.Second).SearchVectors(context.Background(), types.VectorQuery{
		AuthContext: types.AuthContext{
			TenantID:  "tenant-1",
			UserID:    "user-1",
			DeviceID:  "device-1",
			TraceID:   "trace-1",
			RequestID: "request-1",
		},
		RequesterRef:       "requester-hash",
		RetrievalRequestID: "pack-id",
		CollectionTypes:    []string{types.VectorCollectionMemoryEvent},
		QueryEmbeddingRef:  "embedding-ref",
		TopK:               5,
		MinScore:           0.25,
		VisibilityScope:    "tenant:tenant-1:user:user-1",
		PolicyVersion:      "policy-v1",
		At:                 time.Unix(2, 3000000).UTC(),
	})
	if err != nil {
		t.Fatalf("SearchVectors returned error: %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("unexpected vector results: %+v", result)
	}
	item := result.Items[0]
	if item.VectorItemRef != "vitem-1" ||
		item.SourceRefHash != "sha256:source" ||
		item.SourceService != "memory-service" ||
		item.CollectionType != types.VectorCollectionMemoryEvent ||
		item.Score != 0.91 ||
		item.VisibilityVersion != 7 ||
		item.TombstoneStatus != "NONE" {
		t.Fatalf("vector result not mapped: %+v", item)
	}
	request := fake.searchRequest
	if request == nil {
		t.Fatal("expected SearchVectors request")
	}
	if request.GetAuthContext().GetTenantId() != "tenant-1" ||
		request.GetAuthContext().GetUserId() != "user-1" ||
		request.GetAuthContext().GetServiceName() != retrievalGatewayServiceName ||
		request.GetAuthContext().GetTraceId() != "trace-1" ||
		request.GetAuthContext().GetRequestId() != "request-1" {
		t.Fatalf("auth context not mapped: %+v", request.GetAuthContext())
	}
	if request.GetRequesterRef() != "requester-hash" ||
		request.GetRetrievalRequestId() != "pack-id" ||
		request.GetQueryEmbeddingRef() != "embedding-ref" ||
		request.GetTopK() != 5 ||
		request.GetMinScore() != 0.25 ||
		request.GetVisibilityScope() != "tenant:tenant-1:user:user-1" ||
		request.GetPolicyVersion() != "policy-v1" ||
		request.GetAtUnixMs() != 2003 {
		t.Fatalf("vector query not mapped: %+v", request)
	}
	if len(request.GetCollectionTypes()) != 1 || request.GetCollectionTypes()[0] != types.VectorCollectionMemoryEvent {
		t.Fatalf("collection types not mapped: %+v", request.GetCollectionTypes())
	}
}

func TestVectorClientSearchVectorsMapsUnavailable(t *testing.T) {
	fake := &fakeVectorIndexServiceClient{err: status.Error(codes.Unavailable, "down")}
	_, err := NewVectorClient(fake, time.Second).SearchVectors(context.Background(), types.VectorQuery{})
	if !errors.Is(err, types.ErrVectorUnavailable) {
		t.Fatalf("expected vector unavailable, got %v", err)
	}
}

type fakeVectorIndexServiceClient struct {
	searchRequest *vectorv1.SearchVectorsRequest
	response      *vectorv1.SearchVectorsResponse
	err           error
}

func (client *fakeVectorIndexServiceClient) SearchVectors(
	_ context.Context,
	request *vectorv1.SearchVectorsRequest,
	_ ...grpc.CallOption,
) (*vectorv1.SearchVectorsResponse, error) {
	client.searchRequest = request
	return client.response, client.err
}

func (client *fakeVectorIndexServiceClient) UpsertVectorItem(
	context.Context,
	*vectorv1.UpsertVectorItemRequest,
	...grpc.CallOption,
) (*vectorv1.UpsertVectorItemResponse, error) {
	return nil, status.Error(codes.Unimplemented, "unused")
}

func (client *fakeVectorIndexServiceClient) TombstoneVectorItem(
	context.Context,
	*vectorv1.TombstoneVectorItemRequest,
	...grpc.CallOption,
) (*vectorv1.TombstoneVectorItemResponse, error) {
	return nil, status.Error(codes.Unimplemented, "unused")
}

func (client *fakeVectorIndexServiceClient) RequestVectorRebuild(
	context.Context,
	*vectorv1.RequestVectorRebuildRequest,
	...grpc.CallOption,
) (*vectorv1.RequestVectorRebuildResponse, error) {
	return nil, status.Error(codes.Unimplemented, "unused")
}

func (client *fakeVectorIndexServiceClient) GetVectorIndexJob(
	context.Context,
	*vectorv1.GetVectorIndexJobRequest,
	...grpc.CallOption,
) (*vectorv1.GetVectorIndexJobResponse, error) {
	return nil, status.Error(codes.Unimplemented, "unused")
}
