package grpc

import (
	"context"
	"errors"

	identityv1 "github.com/qsyy0921/IM/api/proto/nexusim/identity/v1"
	"github.com/qsyy0921/IM/services/identity-service/internal/types"
	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type IssueGatewayTokenExecutor interface {
	Execute(context.Context, types.IssueGatewayTokenCommand) (types.IssueGatewayTokenResult, error)
}

type RevokeDeviceExecutor interface {
	Execute(context.Context, types.RevokeDeviceCommand) (types.RevokeDeviceResult, error)
}

type RevokeSessionExecutor interface {
	Execute(context.Context, types.RevokeSessionCommand) (types.RevokeSessionResult, error)
}

type GetDeviceStateExecutor interface {
	Execute(context.Context, types.GetDeviceStateCommand) (types.GetDeviceStateResult, error)
}

type Server struct {
	identityv1.UnimplementedIdentityServiceServer
	issueGatewayToken IssueGatewayTokenExecutor
	revokeDevice      RevokeDeviceExecutor
	revokeSession     RevokeSessionExecutor
	getDeviceState    GetDeviceStateExecutor
}

func NewServer(
	issueGatewayToken IssueGatewayTokenExecutor,
	revokeDevice RevokeDeviceExecutor,
	revokeSession RevokeSessionExecutor,
	getDeviceState GetDeviceStateExecutor,
) *Server {
	return &Server{
		issueGatewayToken: issueGatewayToken,
		revokeDevice:      revokeDevice,
		revokeSession:     revokeSession,
		getDeviceState:    getDeviceState,
	}
}

func Register(registrar grpcgo.ServiceRegistrar, server *Server) {
	identityv1.RegisterIdentityServiceServer(registrar, server)
}

func (s *Server) IssueGatewayToken(ctx context.Context, request *identityv1.IssueGatewayTokenRequest) (*identityv1.IssueGatewayTokenResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	if s.issueGatewayToken == nil {
		return nil, status.Error(codes.Unimplemented, "issue gateway token is not configured")
	}
	result, err := s.issueGatewayToken.Execute(ctx, types.IssueGatewayTokenCommand{
		TenantID:   types.TenantID(request.GetTenantId()),
		UserID:     types.UserID(request.GetUserId()),
		DeviceID:   types.DeviceID(request.GetDeviceId()),
		SessionID:  types.SessionID(request.GetSessionId()),
		Audience:   request.GetAudience(),
		TTLSeconds: request.GetTtlSeconds(),
		TraceID:    request.GetTraceId(),
		RequestID:  request.GetRequestId(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &identityv1.IssueGatewayTokenResponse{
		TenantId:        string(result.TenantID),
		UserId:          string(result.UserID),
		DeviceId:        string(result.DeviceID),
		SessionId:       string(result.SessionID),
		Audience:        result.Audience,
		GatewayToken:    result.GatewayToken,
		ExpiresAtUnixMs: result.ExpiresAtUnixMS,
		IssuedAtUnixMs:  result.IssuedAtUnixMS,
	}, nil
}

func (s *Server) RevokeDevice(ctx context.Context, request *identityv1.RevokeDeviceRequest) (*identityv1.RevokeDeviceResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	if s.revokeDevice == nil {
		return nil, status.Error(codes.Unimplemented, "revoke device is not configured")
	}
	result, err := s.revokeDevice.Execute(ctx, types.RevokeDeviceCommand{
		AdminContext: adminFromProto(ctx, request.GetAdminContext()),
		UserID:       types.UserID(request.GetUserId()),
		DeviceID:     types.DeviceID(request.GetDeviceId()),
		Reason:       request.GetReason(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &identityv1.RevokeDeviceResponse{
		TenantId:        string(result.TenantID),
		UserId:          string(result.UserID),
		DeviceId:        string(result.DeviceID),
		Status:          deviceStatusToProto(result.Status),
		RevokedAtUnixMs: result.RevokedAtUnixMS,
	}, nil
}

func (s *Server) RevokeSession(ctx context.Context, request *identityv1.RevokeSessionRequest) (*identityv1.RevokeSessionResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	if s.revokeSession == nil {
		return nil, status.Error(codes.Unimplemented, "revoke session is not configured")
	}
	result, err := s.revokeSession.Execute(ctx, types.RevokeSessionCommand{
		AdminContext: adminFromProto(ctx, request.GetAdminContext()),
		UserID:       types.UserID(request.GetUserId()),
		DeviceID:     types.DeviceID(request.GetDeviceId()),
		SessionID:    types.SessionID(request.GetSessionId()),
		Reason:       request.GetReason(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &identityv1.RevokeSessionResponse{
		TenantId:        string(result.TenantID),
		UserId:          string(result.UserID),
		DeviceId:        string(result.DeviceID),
		SessionId:       string(result.SessionID),
		Status:          sessionStatusToProto(result.Status),
		RevokedAtUnixMs: result.RevokedAtUnixMS,
	}, nil
}

func (s *Server) GetDeviceState(ctx context.Context, request *identityv1.GetDeviceStateRequest) (*identityv1.GetDeviceStateResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	if s.getDeviceState == nil {
		return nil, status.Error(codes.Unimplemented, "get device state is not configured")
	}
	result, err := s.getDeviceState.Execute(ctx, types.GetDeviceStateCommand{
		AdminContext: adminFromProto(ctx, request.GetAdminContext()),
		UserID:       types.UserID(request.GetUserId()),
		DeviceID:     types.DeviceID(request.GetDeviceId()),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &identityv1.GetDeviceStateResponse{
		TenantId:        string(result.TenantID),
		UserId:          string(result.UserID),
		DeviceId:        string(result.DeviceID),
		Status:          deviceStatusToProto(result.Status),
		CreatedAtUnixMs: result.CreatedAtUnixMS,
		UpdatedAtUnixMs: result.UpdatedAtUnixMS,
		RevokedAtUnixMs: result.RevokedAtUnixMS,
	}, nil
}

func adminFromProto(ctx context.Context, auth *identityv1.AdminContext) types.AdminContext {
	if verified, ok := verifiedAdminFromContext(ctx); ok {
		if auth != nil {
			if verified.TraceID == "" {
				verified.TraceID = auth.GetTraceId()
			}
			if verified.RequestID == "" {
				verified.RequestID = auth.GetRequestId()
			}
		}
		return verified
	}
	if auth == nil {
		return types.AdminContext{}
	}
	return types.AdminContext{
		TenantID:       types.TenantID(auth.GetTenantId()),
		OperatorUserID: types.UserID(auth.GetOperatorUserId()),
		TraceID:        auth.GetTraceId(),
		RequestID:      auth.GetRequestId(),
	}
}

func deviceStatusToProto(status types.DeviceStatus) identityv1.DeviceStatus {
	switch status {
	case types.DeviceStatusActive:
		return identityv1.DeviceStatus_DEVICE_STATUS_ACTIVE
	case types.DeviceStatusRevoked:
		return identityv1.DeviceStatus_DEVICE_STATUS_REVOKED
	default:
		return identityv1.DeviceStatus_DEVICE_STATUS_UNSPECIFIED
	}
}

func sessionStatusToProto(status types.SessionStatus) identityv1.SessionStatus {
	switch status {
	case types.SessionStatusActive:
		return identityv1.SessionStatus_SESSION_STATUS_ACTIVE
	case types.SessionStatusRevoked:
		return identityv1.SessionStatus_SESSION_STATUS_REVOKED
	default:
		return identityv1.SessionStatus_SESSION_STATUS_UNSPECIFIED
	}
}

func grpcError(err error) error {
	switch {
	case errors.Is(err, types.ErrInvalidArgument):
		return status.Error(codes.InvalidArgument, "invalid identity request")
	case errors.Is(err, types.ErrPermissionDenied):
		return status.Error(codes.PermissionDenied, "permission denied")
	case errors.Is(err, types.ErrDeviceRevoked):
		return status.Error(codes.PermissionDenied, "device revoked")
	case errors.Is(err, types.ErrSessionRevoked):
		return status.Error(codes.PermissionDenied, "session revoked")
	case errors.Is(err, types.ErrDeviceNotFound):
		return status.Error(codes.NotFound, "device not found")
	case errors.Is(err, types.ErrSessionNotFound):
		return status.Error(codes.NotFound, "session not found")
	case errors.Is(err, types.ErrDBReadFailed), errors.Is(err, types.ErrDBWriteFailed):
		return status.Error(codes.Unavailable, "identity storage unavailable")
	case errors.Is(err, types.ErrTokenSigningFailed):
		return status.Error(codes.Internal, "token signing failed")
	default:
		return status.Error(codes.Internal, "internal error")
	}
}
