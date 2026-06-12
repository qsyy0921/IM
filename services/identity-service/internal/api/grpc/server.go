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

type RegisterUserExecutor interface {
	Execute(context.Context, types.RegisterUserCommand) (types.RegisterUserResult, error)
}

type LoginExecutor interface {
	Execute(context.Context, types.LoginCommand) (types.LoginResult, error)
}

type RefreshGatewayTokenExecutor interface {
	Execute(context.Context, types.RefreshGatewayTokenCommand) (types.RefreshGatewayTokenResult, error)
}

type RequestVerificationChallengeExecutor interface {
	Execute(context.Context, types.RequestVerificationChallengeCommand) (types.RequestVerificationChallengeResult, error)
}

type ConfirmVerificationChallengeExecutor interface {
	Execute(context.Context, types.ConfirmVerificationChallengeCommand) (types.ConfirmVerificationChallengeResult, error)
}

type RequestPasswordResetExecutor interface {
	Execute(context.Context, types.RequestPasswordResetCommand) (types.RequestPasswordResetResult, error)
}

type ConfirmPasswordResetExecutor interface {
	Execute(context.Context, types.ConfirmPasswordResetCommand) (types.ConfirmPasswordResetResult, error)
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
	registerUser         RegisterUserExecutor
	login                LoginExecutor
	refreshGatewayToken  RefreshGatewayTokenExecutor
	requestVerification  RequestVerificationChallengeExecutor
	confirmVerification  ConfirmVerificationChallengeExecutor
	requestPasswordReset RequestPasswordResetExecutor
	confirmPasswordReset ConfirmPasswordResetExecutor
	issueGatewayToken    IssueGatewayTokenExecutor
	revokeDevice         RevokeDeviceExecutor
	revokeSession        RevokeSessionExecutor
	getDeviceState       GetDeviceStateExecutor
}

func NewServer(
	registerUser RegisterUserExecutor,
	login LoginExecutor,
	refreshGatewayToken RefreshGatewayTokenExecutor,
	requestVerification RequestVerificationChallengeExecutor,
	confirmVerification ConfirmVerificationChallengeExecutor,
	requestPasswordReset RequestPasswordResetExecutor,
	confirmPasswordReset ConfirmPasswordResetExecutor,
	issueGatewayToken IssueGatewayTokenExecutor,
	revokeDevice RevokeDeviceExecutor,
	revokeSession RevokeSessionExecutor,
	getDeviceState GetDeviceStateExecutor,
) *Server {
	return &Server{
		registerUser:         registerUser,
		login:                login,
		refreshGatewayToken:  refreshGatewayToken,
		requestVerification:  requestVerification,
		confirmVerification:  confirmVerification,
		requestPasswordReset: requestPasswordReset,
		confirmPasswordReset: confirmPasswordReset,
		issueGatewayToken:    issueGatewayToken,
		revokeDevice:         revokeDevice,
		revokeSession:        revokeSession,
		getDeviceState:       getDeviceState,
	}
}

func Register(registrar grpcgo.ServiceRegistrar, server *Server) {
	identityv1.RegisterIdentityServiceServer(registrar, server)
}

func (s *Server) RegisterUser(ctx context.Context, request *identityv1.RegisterUserRequest) (*identityv1.RegisterUserResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	if s.registerUser == nil {
		return nil, status.Error(codes.Unimplemented, "register user is not configured")
	}
	result, err := s.registerUser.Execute(ctx, types.RegisterUserCommand{
		TenantID:  types.TenantID(request.GetTenantId()),
		UserID:    types.UserID(request.GetUserId()),
		Password:  request.GetPassword(),
		TraceID:   request.GetTraceId(),
		RequestID: request.GetRequestId(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &identityv1.RegisterUserResponse{
		TenantId:        string(result.TenantID),
		UserId:          string(result.UserID),
		Status:          userStatusToProto(result.Status),
		CreatedAtUnixMs: result.CreatedAtUnixMS,
	}, nil
}

func (s *Server) Login(ctx context.Context, request *identityv1.LoginRequest) (*identityv1.LoginResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	if s.login == nil {
		return nil, status.Error(codes.Unimplemented, "login is not configured")
	}
	result, err := s.login.Execute(ctx, types.LoginCommand{
		TenantID:          types.TenantID(request.GetTenantId()),
		UserID:            types.UserID(request.GetUserId()),
		Password:          request.GetPassword(),
		DeviceID:          types.DeviceID(request.GetDeviceId()),
		Audience:          request.GetAudience(),
		GatewayTTLSeconds: request.GetGatewayTtlSeconds(),
		RefreshTTLSeconds: request.GetRefreshTtlSeconds(),
		TraceID:           request.GetTraceId(),
		RequestID:         request.GetRequestId(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &identityv1.LoginResponse{
		TenantId:               string(result.TenantID),
		UserId:                 string(result.UserID),
		DeviceId:               string(result.DeviceID),
		SessionId:              string(result.SessionID),
		Audience:               result.Audience,
		TokenType:              result.TokenType,
		GatewayToken:           result.GatewayToken,
		RefreshToken:           result.RefreshToken,
		GatewayExpiresAtUnixMs: result.GatewayExpiresAtUnixMS,
		RefreshExpiresAtUnixMs: result.RefreshExpiresAtUnixMS,
		IssuedAtUnixMs:         result.IssuedAtUnixMS,
	}, nil
}

func (s *Server) RefreshGatewayToken(ctx context.Context, request *identityv1.RefreshGatewayTokenRequest) (*identityv1.RefreshGatewayTokenResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	if s.refreshGatewayToken == nil {
		return nil, status.Error(codes.Unimplemented, "refresh gateway token is not configured")
	}
	result, err := s.refreshGatewayToken.Execute(ctx, types.RefreshGatewayTokenCommand{
		TenantID:          types.TenantID(request.GetTenantId()),
		UserID:            types.UserID(request.GetUserId()),
		DeviceID:          types.DeviceID(request.GetDeviceId()),
		RefreshToken:      request.GetRefreshToken(),
		Audience:          request.GetAudience(),
		GatewayTTLSeconds: request.GetGatewayTtlSeconds(),
		RefreshTTLSeconds: request.GetRefreshTtlSeconds(),
		TraceID:           request.GetTraceId(),
		RequestID:         request.GetRequestId(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &identityv1.RefreshGatewayTokenResponse{
		TenantId:               string(result.TenantID),
		UserId:                 string(result.UserID),
		DeviceId:               string(result.DeviceID),
		SessionId:              string(result.SessionID),
		Audience:               result.Audience,
		TokenType:              result.TokenType,
		GatewayToken:           result.GatewayToken,
		RefreshToken:           result.RefreshToken,
		GatewayExpiresAtUnixMs: result.GatewayExpiresAtUnixMS,
		RefreshExpiresAtUnixMs: result.RefreshExpiresAtUnixMS,
		IssuedAtUnixMs:         result.IssuedAtUnixMS,
	}, nil
}

func (s *Server) RequestVerificationChallenge(ctx context.Context, request *identityv1.RequestVerificationChallengeRequest) (*identityv1.RequestVerificationChallengeResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	if s.requestVerification == nil {
		return nil, status.Error(codes.Unimplemented, "request verification challenge is not configured")
	}
	result, err := s.requestVerification.Execute(ctx, types.RequestVerificationChallengeCommand{
		TenantID:    types.TenantID(request.GetTenantId()),
		UserID:      types.UserID(request.GetUserId()),
		Channel:     verificationChannelFromProto(request.GetChannel()),
		Destination: request.GetDestination(),
		TTLSeconds:  request.GetTtlSeconds(),
		Password:    request.GetPassword(),
		TraceID:     request.GetTraceId(),
		RequestID:   request.GetRequestId(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &identityv1.RequestVerificationChallengeResponse{
		TenantId:          string(result.TenantID),
		UserId:            string(result.UserID),
		ChallengeId:       string(result.ChallengeID),
		Channel:           verificationChannelToProto(result.Channel),
		Destination:       result.Destination,
		ExpiresAtUnixMs:   result.ExpiresAtUnixMS,
		DevChallengeToken: result.DevChallengeToken,
	}, nil
}

func (s *Server) ConfirmVerificationChallenge(ctx context.Context, request *identityv1.ConfirmVerificationChallengeRequest) (*identityv1.ConfirmVerificationChallengeResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	if s.confirmVerification == nil {
		return nil, status.Error(codes.Unimplemented, "confirm verification challenge is not configured")
	}
	result, err := s.confirmVerification.Execute(ctx, types.ConfirmVerificationChallengeCommand{
		TenantID:       types.TenantID(request.GetTenantId()),
		UserID:         types.UserID(request.GetUserId()),
		ChallengeID:    types.ChallengeID(request.GetChallengeId()),
		ChallengeToken: request.GetChallengeToken(),
		TraceID:        request.GetTraceId(),
		RequestID:      request.GetRequestId(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &identityv1.ConfirmVerificationChallengeResponse{
		TenantId:         string(result.TenantID),
		UserId:           string(result.UserID),
		Channel:          verificationChannelToProto(result.Channel),
		Destination:      result.Destination,
		VerifiedAtUnixMs: result.VerifiedAtUnixMS,
	}, nil
}

func (s *Server) RequestPasswordReset(ctx context.Context, request *identityv1.RequestPasswordResetRequest) (*identityv1.RequestPasswordResetResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	if s.requestPasswordReset == nil {
		return nil, status.Error(codes.Unimplemented, "request password reset is not configured")
	}
	result, err := s.requestPasswordReset.Execute(ctx, types.RequestPasswordResetCommand{
		TenantID:    types.TenantID(request.GetTenantId()),
		UserID:      types.UserID(request.GetUserId()),
		Channel:     verificationChannelFromProto(request.GetChannel()),
		Destination: request.GetDestination(),
		TTLSeconds:  request.GetTtlSeconds(),
		TraceID:     request.GetTraceId(),
		RequestID:   request.GetRequestId(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &identityv1.RequestPasswordResetResponse{
		TenantId:          string(result.TenantID),
		UserId:            string(result.UserID),
		ChallengeId:       string(result.ChallengeID),
		Channel:           verificationChannelToProto(result.Channel),
		Destination:       result.Destination,
		ExpiresAtUnixMs:   result.ExpiresAtUnixMS,
		DevChallengeToken: result.DevChallengeToken,
	}, nil
}

func (s *Server) ConfirmPasswordReset(ctx context.Context, request *identityv1.ConfirmPasswordResetRequest) (*identityv1.ConfirmPasswordResetResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	if s.confirmPasswordReset == nil {
		return nil, status.Error(codes.Unimplemented, "confirm password reset is not configured")
	}
	result, err := s.confirmPasswordReset.Execute(ctx, types.ConfirmPasswordResetCommand{
		TenantID:       types.TenantID(request.GetTenantId()),
		UserID:         types.UserID(request.GetUserId()),
		ChallengeID:    types.ChallengeID(request.GetChallengeId()),
		ChallengeToken: request.GetChallengeToken(),
		NewPassword:    request.GetNewPassword(),
		TraceID:        request.GetTraceId(),
		RequestID:      request.GetRequestId(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &identityv1.ConfirmPasswordResetResponse{
		TenantId:      string(result.TenantID),
		UserId:        string(result.UserID),
		ResetAtUnixMs: result.ResetAtUnixMS,
	}, nil
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

func userStatusToProto(status types.UserStatus) identityv1.UserStatus {
	switch status {
	case types.UserStatusActive:
		return identityv1.UserStatus_USER_STATUS_ACTIVE
	default:
		return identityv1.UserStatus_USER_STATUS_UNSPECIFIED
	}
}

func verificationChannelFromProto(channel identityv1.VerificationChannel) types.VerificationChannel {
	switch channel {
	case identityv1.VerificationChannel_VERIFICATION_CHANNEL_EMAIL:
		return types.VerificationChannelEmail
	case identityv1.VerificationChannel_VERIFICATION_CHANNEL_PHONE:
		return types.VerificationChannelPhone
	default:
		return ""
	}
}

func verificationChannelToProto(channel types.VerificationChannel) identityv1.VerificationChannel {
	switch channel {
	case types.VerificationChannelEmail:
		return identityv1.VerificationChannel_VERIFICATION_CHANNEL_EMAIL
	case types.VerificationChannelPhone:
		return identityv1.VerificationChannel_VERIFICATION_CHANNEL_PHONE
	default:
		return identityv1.VerificationChannel_VERIFICATION_CHANNEL_UNSPECIFIED
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
	case errors.Is(err, types.ErrUserAlreadyExists):
		return status.Error(codes.AlreadyExists, "user already exists")
	case errors.Is(err, types.ErrInvalidCredentials):
		return status.Error(codes.Unauthenticated, "invalid credentials")
	case errors.Is(err, types.ErrAccountLocked):
		return status.Error(codes.ResourceExhausted, "account temporarily locked")
	case errors.Is(err, types.ErrInvalidRefreshToken):
		return status.Error(codes.Unauthenticated, "invalid refresh token")
	case errors.Is(err, types.ErrRefreshTokenReuseDetected):
		return status.Error(codes.PermissionDenied, "refresh token rejected")
	case errors.Is(err, types.ErrInvalidChallenge):
		return status.Error(codes.Unauthenticated, "invalid challenge")
	case errors.Is(err, types.ErrChallengeExpired):
		return status.Error(codes.Unauthenticated, "challenge expired")
	case errors.Is(err, types.ErrChallengeRateLimited):
		return status.Error(codes.ResourceExhausted, "challenge rate limited")
	default:
		return status.Error(codes.Internal, "internal error")
	}
}
