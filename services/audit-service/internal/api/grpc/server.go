package grpc

import (
	"context"
	"errors"
	"time"

	auditv1 "github.com/qsyy0921/IM/api/proto/nexusim/audit/v1"
	"github.com/qsyy0921/IM/services/audit-service/internal/types"
	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type AppendAuditRecordExecutor interface {
	Execute(context.Context, types.AppendAuditRecordCommand) (types.AuditRecord, error)
}

type QueryAuditRecordsExecutor interface {
	Execute(context.Context, types.QueryAuditRecordsCommand) (types.QueryAuditRecordsResult, error)
}

type VerifyAuditProofExecutor interface {
	Execute(context.Context, types.VerifyAuditProofCommand) (types.AuditProofVerification, error)
}

type Server struct {
	auditv1.UnimplementedAuditServiceServer
	appendAuditRecord AppendAuditRecordExecutor
	queryAuditRecords QueryAuditRecordsExecutor
	verifyAuditProof  VerifyAuditProofExecutor
}

func NewServer(
	appendAuditRecord AppendAuditRecordExecutor,
	queryAuditRecords QueryAuditRecordsExecutor,
	verifyAuditProof VerifyAuditProofExecutor,
) *Server {
	return &Server{
		appendAuditRecord: appendAuditRecord,
		queryAuditRecords: queryAuditRecords,
		verifyAuditProof:  verifyAuditProof,
	}
}

func Register(registrar grpcgo.ServiceRegistrar, server *Server) {
	auditv1.RegisterAuditServiceServer(registrar, server)
}

func (server *Server) AppendAuditRecord(
	ctx context.Context,
	request *auditv1.AppendAuditRecordRequest,
) (*auditv1.AppendAuditRecordResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	auth, ok := authFromProto(ctx, request.GetAuthContext())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "auth_context is required")
	}
	record, err := server.appendAuditRecord.Execute(ctx, types.AppendAuditRecordCommand{
		AuthContext:    auth,
		AuditStream:    request.GetAuditStream(),
		SourceService:  request.GetSourceService(),
		SourceEventID:  request.GetSourceEventId(),
		RecordType:     request.GetRecordType(),
		ActorRef:       request.GetActorRef(),
		SubjectRef:     request.GetSubjectRef(),
		ResourceRef:    request.GetResourceRef(),
		Action:         request.GetAction(),
		Outcome:        request.GetOutcome(),
		ReasonCode:     request.GetReasonCode(),
		RiskLevel:      request.GetRiskLevel(),
		OccurredAt:     unixMillisToTime(request.GetOccurredAtUnixMs()),
		AttributesJSON: request.GetAttributesJson(),
		IdempotencyKey: request.GetIdempotencyKey(),
		CorrelationID:  request.GetCorrelationId(),
		CausationID:    request.GetCausationId(),
		TraceID:        request.GetTraceId(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &auditv1.AppendAuditRecordResponse{Record: recordToProto(record)}, nil
}

func (server *Server) QueryAuditRecords(
	ctx context.Context,
	request *auditv1.QueryAuditRecordsRequest,
) (*auditv1.QueryAuditRecordsResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	auth, ok := authFromProto(ctx, request.GetAuthContext())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "auth_context is required")
	}
	result, err := server.queryAuditRecords.Execute(ctx, types.QueryAuditRecordsCommand{
		AuthContext:   auth,
		AuditStream:   request.GetAuditStream(),
		RecordType:    request.GetRecordType(),
		SourceService: request.GetSourceService(),
		AfterAuditID:  request.GetAfterAuditId(),
		Limit:         int(request.GetLimit()),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	records := make([]*auditv1.AuditRecord, 0, len(result.Records))
	for _, record := range result.Records {
		records = append(records, recordToProto(record))
	}
	return &auditv1.QueryAuditRecordsResponse{
		Records:    records,
		NextCursor: result.NextCursor,
	}, nil
}

func (server *Server) VerifyAuditProof(
	ctx context.Context,
	request *auditv1.VerifyAuditProofRequest,
) (*auditv1.VerifyAuditProofResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	auth, ok := authFromProto(ctx, request.GetAuthContext())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "auth_context is required")
	}
	result, err := server.verifyAuditProof.Execute(ctx, types.VerifyAuditProofCommand{
		AuthContext: auth,
		AuditID:     request.GetAuditId(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &auditv1.VerifyAuditProofResponse{
		AuditId:            result.AuditID,
		Valid:              result.Valid,
		FailureReason:      result.FailureReason,
		RecordHash:         result.RecordHash,
		PreviousRecordHash: result.PreviousRecordHash,
	}, nil
}

func authFromProto(ctx context.Context, auth *auditv1.AuthContext) (types.AuthContext, bool) {
	if verified, ok := verifiedAuthFromContext(ctx); ok {
		if auth != nil {
			if verified.TraceID == "" {
				verified.TraceID = auth.GetTraceId()
			}
			if verified.RequestID == "" {
				verified.RequestID = auth.GetRequestId()
			}
		}
		return verified, true
	}
	if auth == nil {
		return types.AuthContext{}, false
	}
	return types.AuthContext{
		TenantID:  types.TenantID(auth.GetTenantId()),
		UserID:    types.UserID(auth.GetUserId()),
		DeviceID:  auth.GetDeviceId(),
		SessionID: auth.GetSessionId(),
		TraceID:   auth.GetTraceId(),
		RequestID: auth.GetRequestId(),
	}, true
}

func recordToProto(record types.AuditRecord) *auditv1.AuditRecord {
	return &auditv1.AuditRecord{
		TenantId:           string(record.TenantID),
		AuditId:            record.AuditID,
		AuditStream:        record.AuditStream,
		SourceService:      record.SourceService,
		SourceEventId:      record.SourceEventID,
		RecordType:         record.RecordType,
		ActorRef:           record.ActorRef,
		SubjectRef:         record.SubjectRef,
		ResourceRef:        record.ResourceRef,
		Action:             record.Action,
		Outcome:            record.Outcome,
		ReasonCode:         record.ReasonCode,
		RiskLevel:          record.RiskLevel,
		OccurredAtUnixMs:   timeToUnixMillis(record.OccurredAt),
		IngestedAtUnixMs:   timeToUnixMillis(record.IngestedAt),
		AttributesJson:     record.AttributesJSON,
		CanonicalJsonHash:  record.CanonicalJSONHash,
		PreviousRecordHash: record.PreviousRecordHash,
		RecordHash:         record.RecordHash,
		IdempotencyKey:     record.IdempotencyKey,
	}
}

func unixMillisToTime(value int64) time.Time {
	if value <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(value).UTC()
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
	case errors.Is(err, types.ErrAlreadyExists):
		return status.Error(codes.AlreadyExists, "audit record already exists")
	case errors.Is(err, types.ErrNotFound):
		return status.Error(codes.NotFound, "audit record not found")
	case errors.Is(err, types.ErrFailedPrecondition):
		return status.Error(codes.FailedPrecondition, "audit precondition failed")
	case errors.Is(err, types.ErrDBReadFailed):
		return status.Error(codes.Unavailable, "audit read failed")
	case errors.Is(err, types.ErrDBWriteFailed):
		return status.Error(codes.Unavailable, "audit write failed")
	default:
		return status.Error(codes.Internal, "audit internal error")
	}
}
