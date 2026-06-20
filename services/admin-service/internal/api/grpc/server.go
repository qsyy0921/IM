package grpc

import (
	"context"
	"errors"
	"time"

	adminv1 "github.com/qsyy0921/IM/api/proto/nexusim/admin/v1"
	"github.com/qsyy0921/IM/services/admin-service/internal/app"
	"github.com/qsyy0921/IM/services/admin-service/internal/types"
	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type CreateAdminOperationExecutor interface {
	Execute(context.Context, types.CreateAdminOperationCommand) (app.CreateAdminOperationResult, error)
}

type ApproveAdminOperationExecutor interface {
	Execute(context.Context, types.ApproveAdminOperationCommand) (app.ApproveAdminOperationResult, error)
}

type GetAdminOperationExecutor interface {
	Execute(context.Context, types.GetAdminOperationCommand) (types.AdminOperation, []types.AdminApproval, error)
}

type ListAdminOperationsExecutor interface {
	Execute(context.Context, types.ListAdminOperationsCommand) ([]types.AdminOperation, error)
}

type Server struct {
	adminv1.UnimplementedAdminServiceServer
	create  CreateAdminOperationExecutor
	approve ApproveAdminOperationExecutor
	get     GetAdminOperationExecutor
	list    ListAdminOperationsExecutor
}

func NewServer(
	create CreateAdminOperationExecutor,
	approve ApproveAdminOperationExecutor,
	get GetAdminOperationExecutor,
	list ListAdminOperationsExecutor,
) *Server {
	return &Server{
		create:  create,
		approve: approve,
		get:     get,
		list:    list,
	}
}

func Register(registrar grpcgo.ServiceRegistrar, server *Server) {
	adminv1.RegisterAdminServiceServer(registrar, server)
}

func (server *Server) CreateAdminOperation(
	ctx context.Context,
	request *adminv1.CreateAdminOperationRequest,
) (*adminv1.CreateAdminOperationResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	auth, ok := authFromProto(ctx, request.GetAuthContext())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "auth_context is required")
	}
	result, err := server.create.Execute(ctx, types.CreateAdminOperationCommand{
		AuthContext:          auth,
		OperatorRef:          request.GetOperatorRef(),
		OperatorRole:         request.GetOperatorRole(),
		OperationType:        request.GetOperationType(),
		TargetRefHash:        request.GetTargetRefHash(),
		RiskLevel:            request.GetRiskLevel(),
		PayloadSchemaVersion: request.GetPayloadSchemaVersion(),
		OperationPayloadJSON: request.GetOperationPayloadJson(),
		ReasonRef:            request.GetReasonRef(),
		EvidenceRefs:         request.GetEvidenceRefs(),
		IdempotencyKey:       request.GetIdempotencyKey(),
		CorrelationID:        request.GetCorrelationId(),
		CausationID:          request.GetCausationId(),
		TraceID:              request.GetTraceId(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &adminv1.CreateAdminOperationResponse{
		Operation: operationToProto(result.Operation),
		Replayed:  result.Replayed,
	}, nil
}

func (server *Server) ApproveAdminOperation(
	ctx context.Context,
	request *adminv1.ApproveAdminOperationRequest,
) (*adminv1.ApproveAdminOperationResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	auth, ok := authFromProto(ctx, request.GetAuthContext())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "auth_context is required")
	}
	result, err := server.approve.Execute(ctx, types.ApproveAdminOperationCommand{
		AuthContext:       auth,
		OperationID:       request.GetOperationId(),
		ApproverRef:       request.GetApproverRef(),
		ApproverRole:      request.GetApproverRole(),
		Decision:          request.GetDecision(),
		ApprovalPolicyRef: request.GetApprovalPolicyRef(),
		ReasonRef:         request.GetReasonRef(),
		EvidenceRefs:      request.GetEvidenceRefs(),
		IdempotencyKey:    request.GetIdempotencyKey(),
		CorrelationID:     request.GetCorrelationId(),
		CausationID:       request.GetCausationId(),
		TraceID:           request.GetTraceId(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &adminv1.ApproveAdminOperationResponse{
		Operation: operationToProto(result.Operation),
		Approval:  approvalToProto(result.Approval),
		Replayed:  result.Replayed,
	}, nil
}

func (server *Server) GetAdminOperation(
	ctx context.Context,
	request *adminv1.GetAdminOperationRequest,
) (*adminv1.GetAdminOperationResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	auth, ok := authFromProto(ctx, request.GetAuthContext())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "auth_context is required")
	}
	operation, approvals, err := server.get.Execute(ctx, types.GetAdminOperationCommand{
		AuthContext: auth,
		OperationID: request.GetOperationId(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	response := &adminv1.GetAdminOperationResponse{Operation: operationToProto(operation)}
	for _, approval := range approvals {
		response.Approvals = append(response.Approvals, approvalToProto(approval))
	}
	return response, nil
}

func (server *Server) ListAdminOperations(
	ctx context.Context,
	request *adminv1.ListAdminOperationsRequest,
) (*adminv1.ListAdminOperationsResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	auth, ok := authFromProto(ctx, request.GetAuthContext())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "auth_context is required")
	}
	operations, err := server.list.Execute(ctx, types.ListAdminOperationsCommand{
		AuthContext:   auth,
		Status:        request.GetStatus(),
		OperationType: request.GetOperationType(),
		PageSize:      int(request.GetPageSize()),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	response := &adminv1.ListAdminOperationsResponse{}
	for _, operation := range operations {
		response.Operations = append(response.Operations, operationToProto(operation))
	}
	return response, nil
}

func authFromProto(ctx context.Context, auth *adminv1.AuthContext) (types.AuthContext, bool) {
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

func operationToProto(operation types.AdminOperation) *adminv1.AdminOperation {
	return &adminv1.AdminOperation{
		TenantId:             string(operation.TenantID),
		OperationId:          operation.OperationID,
		OperationType:        operation.OperationType,
		TargetRefHash:        operation.TargetRefHash,
		RiskLevel:            operation.RiskLevel,
		PayloadSchemaVersion: operation.PayloadSchemaVersion,
		PayloadHash:          operation.PayloadHash,
		ReasonRef:            operation.ReasonRef,
		EvidenceRefs:         operation.EvidenceRefs,
		Status:               operation.Status,
		RequestedBy:          operation.RequestedBy,
		ApprovedBy:           operation.ApprovedBy,
		CorrelationId:        operation.CorrelationID,
		CausationId:          operation.CausationID,
		TraceId:              operation.TraceID,
		RequestedAtUnixMs:    timeToUnixMillis(operation.RequestedAt),
		ApprovedAtUnixMs:     timeToUnixMillis(operation.ApprovedAt),
		UpdatedAtUnixMs:      timeToUnixMillis(operation.UpdatedAt),
	}
}

func approvalToProto(approval types.AdminApproval) *adminv1.AdminApproval {
	return &adminv1.AdminApproval{
		TenantId:          string(approval.TenantID),
		ApprovalId:        approval.ApprovalID,
		OperationId:       approval.OperationID,
		ApproverRef:       approval.ApproverRef,
		Decision:          approval.Decision,
		ApprovalPolicyRef: approval.ApprovalPolicyRef,
		ReasonRef:         approval.ReasonRef,
		EvidenceRefs:      approval.EvidenceRefs,
		CreatedAtUnixMs:   timeToUnixMillis(approval.CreatedAt),
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
	case errors.Is(err, types.ErrAlreadyExists):
		return status.Error(codes.AlreadyExists, "admin operation already exists")
	case errors.Is(err, types.ErrNotFound):
		return status.Error(codes.NotFound, "admin operation not found")
	case errors.Is(err, types.ErrFailedPrecondition):
		return status.Error(codes.FailedPrecondition, "admin operation precondition failed")
	case errors.Is(err, types.ErrUnavailable), errors.Is(err, types.ErrDBReadFailed), errors.Is(err, types.ErrDBWriteFailed):
		return status.Error(codes.Unavailable, "admin service temporarily unavailable")
	default:
		return status.Error(codes.Internal, "admin internal error")
	}
}
