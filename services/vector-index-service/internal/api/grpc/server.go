package grpc

import (
	"context"
	"errors"
	"time"

	vectorv1 "github.com/qsyy0921/IM/api/proto/nexusim/vector/v1"
	"github.com/qsyy0921/IM/services/vector-index-service/internal/app"
	"github.com/qsyy0921/IM/services/vector-index-service/internal/types"
	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type UpsertVectorItemExecutor interface {
	Execute(context.Context, types.UpsertVectorItemCommand) (app.UpsertVectorItemResult, error)
}

type TombstoneVectorItemExecutor interface {
	Execute(context.Context, types.TombstoneVectorItemCommand) (app.TombstoneVectorItemResult, error)
}

type SearchVectorsExecutor interface {
	Execute(context.Context, types.SearchVectorsCommand) ([]types.VectorSearchResult, error)
}

type GetVectorIndexJobExecutor interface {
	Execute(context.Context, types.GetVectorIndexJobCommand) (types.VectorIndexJob, error)
}

type Server struct {
	vectorv1.UnimplementedVectorIndexServiceServer
	upsert    UpsertVectorItemExecutor
	tombstone TombstoneVectorItemExecutor
	search    SearchVectorsExecutor
	getJob    GetVectorIndexJobExecutor
}

func NewServer(
	upsert UpsertVectorItemExecutor,
	tombstone TombstoneVectorItemExecutor,
	search SearchVectorsExecutor,
	getJob GetVectorIndexJobExecutor,
) *Server {
	return &Server{
		upsert:    upsert,
		tombstone: tombstone,
		search:    search,
		getJob:    getJob,
	}
}

func Register(registrar grpcgo.ServiceRegistrar, server *Server) {
	vectorv1.RegisterVectorIndexServiceServer(registrar, server)
}

func (server *Server) UpsertVectorItem(
	ctx context.Context,
	request *vectorv1.UpsertVectorItemRequest,
) (*vectorv1.UpsertVectorItemResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	auth, ok := authFromProto(ctx, request.GetAuthContext())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "auth_context is required")
	}
	result, err := server.upsert.Execute(ctx, types.UpsertVectorItemCommand{
		AuthContext:         auth,
		SourceService:       request.GetSourceService(),
		CollectionType:      request.GetCollectionType(),
		SourceRefHash:       request.GetSourceRefHash(),
		SourceID:            request.GetSourceId(),
		SourceVersion:       request.GetSourceVersion(),
		SourceHash:          request.GetSourceHash(),
		ChunkHash:           request.GetChunkHash(),
		EmbeddingModelRef:   request.GetEmbeddingModelRef(),
		EmbeddingVectorHash: request.GetEmbeddingVectorHash(),
		Dimension:           int(request.GetDimension()),
		VisibilityScope:     request.GetVisibilityScope(),
		VisibilityVersion:   request.GetVisibilityVersion(),
		PolicyVersion:       request.GetPolicyVersion(),
		DataClass:           request.GetDataClass(),
		DeleteProofID:       request.GetDeleteProofId(),
		RetentionPolicyRef:  request.GetRetentionPolicyRef(),
		IdempotencyKey:      request.GetIdempotencyKey(),
		CorrelationID:       request.GetCorrelationId(),
		CausationID:         request.GetCausationId(),
		TraceID:             request.GetTraceId(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &vectorv1.UpsertVectorItemResponse{
		Item:     itemToProto(result.Item),
		Job:      jobToProto(result.Job),
		Replayed: result.Replayed,
	}, nil
}

func (server *Server) TombstoneVectorItem(
	ctx context.Context,
	request *vectorv1.TombstoneVectorItemRequest,
) (*vectorv1.TombstoneVectorItemResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	auth, ok := authFromProto(ctx, request.GetAuthContext())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "auth_context is required")
	}
	result, err := server.tombstone.Execute(ctx, types.TombstoneVectorItemCommand{
		AuthContext:    auth,
		VectorItemID:   request.GetVectorItemId(),
		DeleteProofID:  request.GetDeleteProofId(),
		ReasonClass:    request.GetReasonClass(),
		IdempotencyKey: request.GetIdempotencyKey(),
		CorrelationID:  request.GetCorrelationId(),
		CausationID:    request.GetCausationId(),
		TraceID:        request.GetTraceId(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &vectorv1.TombstoneVectorItemResponse{
		Item:        itemToProto(result.Item),
		Job:         jobToProto(result.Job),
		TombstoneId: result.TombstoneID,
		Replayed:    result.Replayed,
	}, nil
}

func (server *Server) SearchVectors(
	ctx context.Context,
	request *vectorv1.SearchVectorsRequest,
) (*vectorv1.SearchVectorsResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	auth, ok := authFromProto(ctx, request.GetAuthContext())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "auth_context is required")
	}
	var at time.Time
	if request.GetAtUnixMs() > 0 {
		at = time.UnixMilli(request.GetAtUnixMs()).UTC()
	}
	results, err := server.search.Execute(ctx, types.SearchVectorsCommand{
		AuthContext:        auth,
		RequesterRef:       request.GetRequesterRef(),
		RetrievalRequestID: request.GetRetrievalRequestId(),
		CollectionTypes:    request.GetCollectionTypes(),
		QueryEmbeddingRef:  request.GetQueryEmbeddingRef(),
		TopK:               int(request.GetTopK()),
		MinScore:           request.GetMinScore(),
		VisibilityScope:    request.GetVisibilityScope(),
		PolicyVersion:      request.GetPolicyVersion(),
		At:                 at,
	})
	if err != nil {
		return nil, grpcError(err)
	}
	response := &vectorv1.SearchVectorsResponse{}
	for _, result := range results {
		response.Results = append(response.Results, resultToProto(result))
	}
	return response, nil
}

func (server *Server) GetVectorIndexJob(
	ctx context.Context,
	request *vectorv1.GetVectorIndexJobRequest,
) (*vectorv1.GetVectorIndexJobResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	auth, ok := authFromProto(ctx, request.GetAuthContext())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "auth_context is required")
	}
	job, err := server.getJob.Execute(ctx, types.GetVectorIndexJobCommand{
		AuthContext: auth,
		JobID:       request.GetJobId(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &vectorv1.GetVectorIndexJobResponse{Job: jobToProto(job)}, nil
}

func authFromProto(ctx context.Context, auth *vectorv1.AuthContext) (types.AuthContext, bool) {
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

func itemToProto(item types.VectorItem) *vectorv1.VectorItem {
	return &vectorv1.VectorItem{
		TenantId:            string(item.TenantID),
		VectorItemId:        item.VectorItemID,
		CollectionId:        item.CollectionID,
		CollectionType:      item.CollectionType,
		SourceService:       item.SourceService,
		SourceRefHash:       item.SourceRefHash,
		SourceId:            item.SourceID,
		SourceVersion:       item.SourceVersion,
		SourceHash:          item.SourceHash,
		ChunkHash:           item.ChunkHash,
		EmbeddingModelRef:   item.EmbeddingModelRef,
		EmbeddingVectorHash: item.EmbeddingVectorHash,
		Dimension:           int32(item.Dimension),
		VisibilityScope:     item.VisibilityScope,
		VisibilityVersion:   item.VisibilityVersion,
		PolicyVersion:       item.PolicyVersion,
		DataClass:           item.DataClass,
		TombstoneStatus:     item.TombstoneStatus,
		DeleteProofId:       item.DeleteProofID,
		RetentionPolicyRef:  item.RetentionPolicyRef,
		Status:              item.Status,
		CreatedAtUnixMs:     timeToUnixMillis(item.CreatedAt),
		UpdatedAtUnixMs:     timeToUnixMillis(item.UpdatedAt),
	}
}

func jobToProto(job types.VectorIndexJob) *vectorv1.VectorIndexJob {
	return &vectorv1.VectorIndexJob{
		TenantId:          string(job.TenantID),
		JobId:             job.JobID,
		CollectionId:      job.CollectionID,
		VectorItemId:      job.VectorItemID,
		JobType:           job.JobType,
		Status:            job.Status,
		RetryCount:        int32(job.RetryCount),
		FailureClass:      job.FailureClass,
		PublicError:       job.PublicError,
		CreatedAtUnixMs:   timeToUnixMillis(job.CreatedAt),
		CompletedAtUnixMs: timeToUnixMillis(job.CompletedAt),
	}
}

func resultToProto(result types.VectorSearchResult) *vectorv1.VectorSearchResult {
	return &vectorv1.VectorSearchResult{
		VectorItemRef:     result.VectorItemRef,
		SourceRefHash:     result.SourceRefHash,
		SourceService:     result.SourceService,
		CollectionType:    result.CollectionType,
		Score:             result.Score,
		VisibilityVersion: result.VisibilityVersion,
		TombstoneStatus:   result.TombstoneStatus,
	}
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
		return status.Error(codes.NotFound, "vector resource not found")
	case errors.Is(err, types.ErrAlreadyExists):
		return status.Error(codes.AlreadyExists, "vector resource already exists")
	case errors.Is(err, types.ErrFailedPrecondition):
		return status.Error(codes.FailedPrecondition, "vector precondition failed")
	case errors.Is(err, types.ErrUnavailable), errors.Is(err, types.ErrDBReadFailed), errors.Is(err, types.ErrDBWriteFailed):
		return status.Error(codes.Unavailable, "vector index temporarily unavailable")
	default:
		return status.Error(codes.Internal, "vector index internal error")
	}
}
