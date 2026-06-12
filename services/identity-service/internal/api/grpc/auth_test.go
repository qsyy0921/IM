package grpc

import (
	"context"
	"errors"
	"testing"

	identityv1 "github.com/qsyy0921/IM/api/proto/nexusim/identity/v1"
	"github.com/qsyy0921/IM/services/identity-service/internal/types"
	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestAdminFromProtoPrefersVerifiedMetadata(t *testing.T) {
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		metadataTenantID, "trusted-tenant",
		metadataUserID, "trusted-admin",
	))
	interceptor := VerifiedAdminUnaryInterceptor(true)
	_, err := interceptor(ctx, nil, &grpcgo.UnaryServerInfo{FullMethod: "/nexusim.identity.v1.IdentityService/RevokeDevice"}, func(ctx context.Context, req any) (any, error) {
		admin := adminFromProto(ctx, &identityv1.AdminContext{
			TenantId:       "spoofed-tenant",
			OperatorUserId: "spoofed-admin",
			TraceId:        "body-trace",
			RequestId:      "body-request",
		})
		if admin.TenantID != types.TenantID("trusted-tenant") || admin.OperatorUserID != types.UserID("trusted-admin") {
			t.Fatalf("verified metadata should override body admin context: %+v", admin)
		}
		if admin.TraceID != "body-trace" || admin.RequestID != "body-request" {
			t.Fatalf("expected trace/request fallback from body, got %+v", admin)
		}
		return nil, nil
	})
	if err != nil {
		t.Fatalf("interceptor returned error: %v", err)
	}
}

func TestVerifiedAdminUnaryInterceptorRequiresAdminMetadataForAdminMethods(t *testing.T) {
	interceptor := VerifiedAdminUnaryInterceptor(true)
	_, err := interceptor(context.Background(), nil, &grpcgo.UnaryServerInfo{FullMethod: "/nexusim.identity.v1.IdentityService/RevokeSession"}, func(ctx context.Context, req any) (any, error) {
		t.Fatal("handler should not be called without verified admin metadata")
		return nil, nil
	})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("expected unauthenticated, got %v (%v)", status.Code(err), err)
	}
}

func TestVerifiedAdminUnaryInterceptorDoesNotRequireMetadataForPublicTokenMethods(t *testing.T) {
	interceptor := VerifiedAdminUnaryInterceptor(true)
	for _, method := range []string{
		"/nexusim.identity.v1.IdentityService/RegisterUser",
		"/nexusim.identity.v1.IdentityService/Login",
		"/nexusim.identity.v1.IdentityService/RefreshGatewayToken",
		"/nexusim.identity.v1.IdentityService/IssueGatewayToken",
	} {
		t.Run(method, func(t *testing.T) {
			called := false
			_, err := interceptor(context.Background(), nil, &grpcgo.UnaryServerInfo{FullMethod: method}, func(ctx context.Context, req any) (any, error) {
				called = true
				return nil, nil
			})
			if err != nil {
				t.Fatalf("%s should not require admin metadata: %v", method, err)
			}
			if !called {
				t.Fatal("handler was not called")
			}
		})
	}
}

func TestServerRegisterUserMapsRequestAndResponse(t *testing.T) {
	executor := &fakeRegisterUserExecutor{
		result: types.RegisterUserResult{
			TenantID:        "tenant-1",
			UserID:          "user-1",
			Status:          types.UserStatusActive,
			CreatedAtUnixMS: 1_800_000_000_000,
		},
	}
	server := NewServer(executor, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	response, err := server.RegisterUser(context.Background(), &identityv1.RegisterUserRequest{
		TenantId:  "tenant-1",
		UserId:    "user-1",
		Password:  "correct horse battery staple",
		TraceId:   "trace-1",
		RequestId: "request-1",
	})
	if err != nil {
		t.Fatalf("register user: %v", err)
	}
	if executor.command.TenantID != "tenant-1" || executor.command.UserID != "user-1" || executor.command.Password == "" {
		t.Fatalf("unexpected command: %+v", executor.command)
	}
	if response.GetStatus() != identityv1.UserStatus_USER_STATUS_ACTIVE || response.GetCreatedAtUnixMs() != 1_800_000_000_000 {
		t.Fatalf("unexpected response: %+v", response)
	}
}

func TestServerRegisterUserMapsDuplicateToAlreadyExists(t *testing.T) {
	server := NewServer(&fakeRegisterUserExecutor{err: types.NewUserAlreadyExists("duplicate")}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	_, err := server.RegisterUser(context.Background(), &identityv1.RegisterUserRequest{
		TenantId: "tenant-1",
		UserId:   "user-1",
		Password: "correct horse battery staple",
	})
	if status.Code(err) != codes.AlreadyExists {
		t.Fatalf("expected already exists, got %v (%v)", status.Code(err), err)
	}
}

func TestServerLoginMapsAccountLockedToResourceExhausted(t *testing.T) {
	server := NewServer(nil, &fakeLoginExecutor{err: types.NewAccountLocked("locked")}, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	_, err := server.Login(context.Background(), &identityv1.LoginRequest{
		TenantId: "tenant-1",
		UserId:   "user-1",
		Password: "correct horse battery staple",
		DeviceId: "device-1",
	})
	if status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("expected resource exhausted, got %v (%v)", status.Code(err), err)
	}
}

func TestServerRequestPasswordResetReturnsAcceptedShape(t *testing.T) {
	executor := &fakeRequestPasswordResetExecutor{
		result: types.RequestPasswordResetResult{
			TenantID:          "tenant-1",
			UserID:            "user-1",
			ChallengeID:       "challenge-neutral",
			Channel:           types.VerificationChannelEmail,
			Destination:       "user1@example.com",
			ExpiresAtUnixMS:   1_800_000_600_000,
			DevChallengeToken: "",
		},
	}
	server := NewServer(nil, nil, nil, nil, nil, executor, nil, nil, nil, nil, nil)
	response, err := server.RequestPasswordReset(context.Background(), &identityv1.RequestPasswordResetRequest{
		TenantId:    "tenant-1",
		UserId:      "user-1",
		Channel:     identityv1.VerificationChannel_VERIFICATION_CHANNEL_EMAIL,
		Destination: "user1@example.com",
		TtlSeconds:  600,
	})
	if err != nil {
		t.Fatalf("request password reset should return OK accepted shape: %v", err)
	}
	if executor.command.TenantID != "tenant-1" || executor.command.Channel != types.VerificationChannelEmail {
		t.Fatalf("unexpected command: %+v", executor.command)
	}
	if response.GetChallengeId() != "challenge-neutral" || response.GetDevChallengeToken() != "" {
		t.Fatalf("unexpected password reset response: %+v", response)
	}
}

func TestGRPCErrorMapsCredentialErrors(t *testing.T) {
	if code := status.Code(grpcError(types.NewUserAlreadyExists("duplicate"))); code != codes.AlreadyExists {
		t.Fatalf("expected user already exists to map to already exists, got %v", code)
	}
	if code := status.Code(grpcError(types.NewInvalidCredentials("bad password"))); code != codes.Unauthenticated {
		t.Fatalf("expected invalid credentials to map to unauthenticated, got %v", code)
	}
	if code := status.Code(grpcError(types.NewAccountLocked("too many attempts"))); code != codes.ResourceExhausted {
		t.Fatalf("expected account locked to map to resource exhausted, got %v", code)
	}
	if code := status.Code(grpcError(types.NewInvalidRefreshToken("bad refresh"))); code != codes.Unauthenticated {
		t.Fatalf("expected invalid refresh token to map to unauthenticated, got %v", code)
	}
	if code := status.Code(grpcError(types.NewRefreshTokenReuseDetected("reuse"))); code != codes.PermissionDenied {
		t.Fatalf("expected refresh token reuse to map to permission denied, got %v", code)
	}
}

type fakeRegisterUserExecutor struct {
	command types.RegisterUserCommand
	result  types.RegisterUserResult
	err     error
}

type fakeLoginExecutor struct {
	err error
}

type fakeRequestPasswordResetExecutor struct {
	command types.RequestPasswordResetCommand
	result  types.RequestPasswordResetResult
	err     error
}

func (executor *fakeLoginExecutor) Execute(context.Context, types.LoginCommand) (types.LoginResult, error) {
	return types.LoginResult{}, executor.err
}

func (executor *fakeRequestPasswordResetExecutor) Execute(_ context.Context, command types.RequestPasswordResetCommand) (types.RequestPasswordResetResult, error) {
	executor.command = command
	if executor.err != nil {
		return types.RequestPasswordResetResult{}, executor.err
	}
	return executor.result, nil
}

func (executor *fakeRegisterUserExecutor) Execute(_ context.Context, command types.RegisterUserCommand) (types.RegisterUserResult, error) {
	executor.command = command
	if executor.err != nil {
		return types.RegisterUserResult{}, executor.err
	}
	if executor.result.TenantID == "" {
		return types.RegisterUserResult{}, errors.New("fake result is not configured")
	}
	return executor.result, nil
}
