package rpc

import (
	"context"
	"errors"
	"strings"
	"time"

	vectorv1 "github.com/qsyy0921/IM/api/proto/nexusim/vector/v1"
	"github.com/qsyy0921/IM/services/retrieval-gateway/internal/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const retrievalGatewayServiceName = "retrieval-gateway"

type VectorClient struct {
	client  vectorv1.VectorIndexServiceClient
	timeout time.Duration
}

func NewVectorClient(client vectorv1.VectorIndexServiceClient, timeout time.Duration) VectorClient {
	if timeout <= 0 {
		timeout = 500 * time.Millisecond
	}
	return VectorClient{client: client, timeout: timeout}
}

func DialVectorClient(_ context.Context, addr string, timeout time.Duration) (VectorClient, func() error, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return VectorClient{}, nil, errors.New("vector-index-service address is required")
	}
	conn, err := grpc.NewClient(
		"passthrough:///"+addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return VectorClient{}, nil, err
	}
	return NewVectorClient(vectorv1.NewVectorIndexServiceClient(conn), timeout), conn.Close, nil
}

func (client VectorClient) SearchVectors(ctx context.Context, query types.VectorQuery) (types.VectorResult, error) {
	callCtx, cancel := context.WithTimeout(ctx, client.timeout)
	defer cancel()
	callCtx = outgoingMetadataContext(callCtx, query.AuthContext)
	response, err := client.client.SearchVectors(callCtx, &vectorv1.SearchVectorsRequest{
		AuthContext: &vectorv1.AuthContext{
			TenantId:    string(query.AuthContext.TenantID),
			UserId:      string(query.AuthContext.UserID),
			ServiceName: retrievalGatewayServiceName,
			InstanceRef: retrievalGatewayServiceName,
			TraceId:     query.AuthContext.TraceID,
			RequestId:   query.AuthContext.RequestID,
		},
		RequesterRef:       query.RequesterRef,
		RetrievalRequestId: query.RetrievalRequestID,
		CollectionTypes:    query.CollectionTypes,
		QueryEmbeddingRef:  query.QueryEmbeddingRef,
		TopK:               int32(query.TopK),
		MinScore:           query.MinScore,
		VisibilityScope:    query.VisibilityScope,
		PolicyVersion:      query.PolicyVersion,
		AtUnixMs:           timeToUnixMillis(query.At),
	})
	if err != nil {
		return types.VectorResult{}, mapVectorError(err)
	}
	items := make([]types.VectorItemEvidence, 0, len(response.GetResults()))
	for _, result := range response.GetResults() {
		items = append(items, types.VectorItemEvidence{
			VectorItemRef:     result.GetVectorItemRef(),
			SourceRefHash:     result.GetSourceRefHash(),
			SourceService:     result.GetSourceService(),
			CollectionType:    result.GetCollectionType(),
			Score:             result.GetScore(),
			VisibilityVersion: result.GetVisibilityVersion(),
			TombstoneStatus:   result.GetTombstoneStatus(),
		})
	}
	return types.VectorResult{Items: items}, nil
}

func timeToUnixMillis(value time.Time) int64 {
	if value.IsZero() {
		return 0
	}
	return value.UTC().UnixMilli()
}
