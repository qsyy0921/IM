package grpc

import (
	"context"
	"errors"
	"time"

	knowledgev1 "github.com/qsyy0921/IM/api/proto/nexusim/knowledge/v1"
	"github.com/qsyy0921/IM/services/knowledge-ingestion-service/internal/app"
	"github.com/qsyy0921/IM/services/knowledge-ingestion-service/internal/types"
	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type CreateKnowledgeSourceExecutor interface {
	Execute(context.Context, types.CreateKnowledgeSourceCommand) (app.CreateKnowledgeSourceResult, error)
}

type SubmitIngestionJobExecutor interface {
	Execute(context.Context, types.SubmitIngestionJobCommand) (app.SubmitIngestionJobResult, error)
}

type GetIngestionJobExecutor interface {
	Execute(context.Context, types.GetIngestionJobCommand) (types.KnowledgeIngestionJob, error)
}

type ListKnowledgeChunksExecutor interface {
	Execute(context.Context, types.ListKnowledgeChunksCommand) ([]types.KnowledgeChunk, string, error)
}

type Server struct {
	knowledgev1.UnimplementedKnowledgeIngestionServiceServer
	createSource CreateKnowledgeSourceExecutor
	submitJob    SubmitIngestionJobExecutor
	getJob       GetIngestionJobExecutor
	listChunks   ListKnowledgeChunksExecutor
}

func NewServer(
	createSource CreateKnowledgeSourceExecutor,
	submitJob SubmitIngestionJobExecutor,
	getJob GetIngestionJobExecutor,
	listChunks ListKnowledgeChunksExecutor,
) *Server {
	return &Server{
		createSource: createSource,
		submitJob:    submitJob,
		getJob:       getJob,
		listChunks:   listChunks,
	}
}

func Register(registrar grpcgo.ServiceRegistrar, server *Server) {
	knowledgev1.RegisterKnowledgeIngestionServiceServer(registrar, server)
}

func (server *Server) CreateKnowledgeSource(
	ctx context.Context,
	request *knowledgev1.CreateKnowledgeSourceRequest,
) (*knowledgev1.CreateKnowledgeSourceResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	auth, ok := authFromProto(ctx, request.GetAuthContext())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "auth_context is required")
	}
	result, err := server.createSource.Execute(ctx, types.CreateKnowledgeSourceCommand{
		AuthContext:        auth,
		SourceType:         request.GetSourceType(),
		SourceRef:          request.GetSourceRef(),
		SourceURIHash:      request.GetSourceUriHash(),
		MediaObjectRef:     request.GetMediaObjectRef(),
		OwnerRef:           request.GetOwnerRef(),
		VisibilityScope:    request.GetVisibilityScope(),
		DataClass:          request.GetDataClass(),
		ContentHash:        request.GetContentHash(),
		MimeType:           request.GetMimeType(),
		SizeBytes:          request.GetSizeBytes(),
		SourceVersion:      request.GetSourceVersion(),
		RetentionPolicyRef: request.GetRetentionPolicyRef(),
		IdempotencyKey:     request.GetIdempotencyKey(),
		CorrelationID:      request.GetCorrelationId(),
		CausationID:        request.GetCausationId(),
		TraceID:            request.GetTraceId(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &knowledgev1.CreateKnowledgeSourceResponse{
		Source:   sourceToProto(result.Source),
		Replayed: result.Replayed,
	}, nil
}

func (server *Server) SubmitIngestionJob(
	ctx context.Context,
	request *knowledgev1.SubmitIngestionJobRequest,
) (*knowledgev1.SubmitIngestionJobResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	auth, ok := authFromProto(ctx, request.GetAuthContext())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "auth_context is required")
	}
	chunks := make([]types.ChunkManifestItem, 0, len(request.GetChunks()))
	for _, chunk := range request.GetChunks() {
		chunks = append(chunks, types.ChunkManifestItem{
			ChunkHash:            chunk.GetChunkHash(),
			ChunkPreviewRedacted: chunk.GetChunkPreviewRedacted(),
			VisibilityScope:      chunk.GetVisibilityScope(),
			DataClass:            chunk.GetDataClass(),
			PolicyVersion:        chunk.GetPolicyVersion(),
			ChunkVersion:         chunk.GetChunkVersion(),
		})
	}
	result, err := server.submitJob.Execute(ctx, types.SubmitIngestionJobCommand{
		AuthContext:        auth,
		SourceID:           request.GetSourceId(),
		SourceVersion:      request.GetSourceVersion(),
		JobType:            request.GetJobType(),
		ParserProfile:      request.GetParserProfile(),
		ChunkProfile:       request.GetChunkProfile(),
		EmbeddingPolicyRef: request.GetEmbeddingPolicyRef(),
		VectorPolicyRef:    request.GetVectorPolicyRef(),
		RequestedBy:        request.GetRequestedBy(),
		IdempotencyKey:     request.GetIdempotencyKey(),
		DocumentHash:       request.GetDocumentHash(),
		MimeType:           request.GetMimeType(),
		SizeBytes:          request.GetSizeBytes(),
		PageCount:          int(request.GetPageCount()),
		Language:           request.GetLanguage(),
		Chunks:             chunks,
		CorrelationID:      request.GetCorrelationId(),
		CausationID:        request.GetCausationId(),
		TraceID:            request.GetTraceId(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &knowledgev1.SubmitIngestionJobResponse{
		Job:        jobToProto(result.Job),
		DocumentId: result.DocumentID,
		ChunkCount: int32(result.ChunkCount),
		Replayed:   result.Replayed,
	}, nil
}

func (server *Server) GetIngestionJob(
	ctx context.Context,
	request *knowledgev1.GetIngestionJobRequest,
) (*knowledgev1.GetIngestionJobResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	auth, ok := authFromProto(ctx, request.GetAuthContext())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "auth_context is required")
	}
	job, err := server.getJob.Execute(ctx, types.GetIngestionJobCommand{
		AuthContext: auth,
		JobID:       request.GetJobId(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &knowledgev1.GetIngestionJobResponse{Job: jobToProto(job)}, nil
}

func (server *Server) ListKnowledgeChunks(
	ctx context.Context,
	request *knowledgev1.ListKnowledgeChunksRequest,
) (*knowledgev1.ListKnowledgeChunksResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	auth, ok := authFromProto(ctx, request.GetAuthContext())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "auth_context is required")
	}
	chunks, nextToken, err := server.listChunks.Execute(ctx, types.ListKnowledgeChunksCommand{
		AuthContext: auth,
		SourceID:    request.GetSourceId(),
		DocumentID:  request.GetDocumentId(),
		PageSize:    int(request.GetPageSize()),
		PageToken:   request.GetPageToken(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	response := &knowledgev1.ListKnowledgeChunksResponse{NextPageToken: nextToken}
	for _, chunk := range chunks {
		response.Chunks = append(response.Chunks, chunkToProto(chunk))
	}
	return response, nil
}

func authFromProto(ctx context.Context, auth *knowledgev1.AuthContext) (types.AuthContext, bool) {
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

func sourceToProto(source types.KnowledgeSource) *knowledgev1.KnowledgeSource {
	return &knowledgev1.KnowledgeSource{
		TenantId:           string(source.TenantID),
		SourceId:           source.SourceID,
		SourceType:         source.SourceType,
		SourceRefHash:      source.SourceRefHash,
		MediaObjectRef:     source.MediaObjectRef,
		OwnerRef:           source.OwnerRef,
		VisibilityScope:    source.VisibilityScope,
		DataClass:          source.DataClass,
		ContentHash:        source.ContentHash,
		MimeType:           source.MimeType,
		SizeBytes:          source.SizeBytes,
		SourceVersion:      source.SourceVersion,
		RetentionPolicyRef: source.RetentionPolicyRef,
		Status:             source.Status,
		CreatedAtUnixMs:    timeToUnixMillis(source.CreatedAt),
		UpdatedAtUnixMs:    timeToUnixMillis(source.UpdatedAt),
	}
}

func jobToProto(job types.KnowledgeIngestionJob) *knowledgev1.KnowledgeIngestionJob {
	return &knowledgev1.KnowledgeIngestionJob{
		TenantId:           string(job.TenantID),
		JobId:              job.JobID,
		SourceId:           job.SourceID,
		SourceVersion:      job.SourceVersion,
		JobType:            job.JobType,
		ParserProfile:      job.ParserProfile,
		ChunkProfile:       job.ChunkProfile,
		EmbeddingPolicyRef: job.EmbeddingPolicyRef,
		VectorPolicyRef:    job.VectorPolicyRef,
		RequestedBy:        job.RequestedBy,
		Status:             job.Status,
		FailureClass:       job.FailureClass,
		PublicError:        job.PublicError,
		CreatedAtUnixMs:    timeToUnixMillis(job.CreatedAt),
		CompletedAtUnixMs:  timeToUnixMillis(job.CompletedAt),
	}
}

func chunkToProto(chunk types.KnowledgeChunk) *knowledgev1.KnowledgeChunk {
	return &knowledgev1.KnowledgeChunk{
		TenantId:             string(chunk.TenantID),
		ChunkId:              chunk.ChunkID,
		SourceId:             chunk.SourceID,
		DocumentId:           chunk.DocumentID,
		ChunkIndex:           int32(chunk.ChunkIndex),
		ChunkHash:            chunk.ChunkHash,
		ChunkPreviewRedacted: chunk.ChunkPreviewRedacted,
		SourceVersion:        chunk.SourceVersion,
		VisibilityScope:      chunk.VisibilityScope,
		DataClass:            chunk.DataClass,
		ChunkVersion:         chunk.ChunkVersion,
		Status:               chunk.Status,
		TombstoneStatus:      chunk.TombstoneStatus,
		PolicyVersion:        chunk.PolicyVersion,
		DeleteProofId:        chunk.DeleteProofID,
		CreatedAtUnixMs:      timeToUnixMillis(chunk.CreatedAt),
		UpdatedAtUnixMs:      timeToUnixMillis(chunk.UpdatedAt),
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
		return status.Error(codes.NotFound, "knowledge resource not found")
	case errors.Is(err, types.ErrFailedPrecondition):
		return status.Error(codes.FailedPrecondition, "knowledge precondition failed")
	case errors.Is(err, types.ErrUnavailable), errors.Is(err, types.ErrDBReadFailed), errors.Is(err, types.ErrDBWriteFailed):
		return status.Error(codes.Unavailable, "knowledge ingestion temporarily unavailable")
	default:
		return status.Error(codes.Internal, "knowledge ingestion internal error")
	}
}
