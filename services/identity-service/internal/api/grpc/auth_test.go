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
			t.Fatalf("expected trace/request recovery from body, got %+v", admin)
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
	server := &Server{registerUser: executor}
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
	server := &Server{registerUser: &fakeRegisterUserExecutor{err: types.NewUserAlreadyExists("duplicate")}}
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
	server := &Server{login: &fakeLoginExecutor{err: types.NewAccountLocked("locked")}}
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

func TestServerLoginMapsMFAFields(t *testing.T) {
	executor := &fakeLoginExecutor{
		result: types.LoginResult{
			TenantID:               "tenant-1",
			UserID:                 "user-1",
			DeviceID:               "device-1",
			SessionID:              "session-1",
			Audience:               "push-gateway",
			TokenType:              "Bearer",
			GatewayToken:           "gateway-token",
			RefreshToken:           "refresh-token",
			GatewayExpiresAtUnixMS: 1_800_000_900_000,
			RefreshExpiresAtUnixMS: 1_802_592_000_000,
			IssuedAtUnixMS:         1_800_000_000_000,
		},
	}
	server := &Server{login: executor}
	_, err := server.Login(context.Background(), &identityv1.LoginRequest{
		TenantId:    "tenant-1",
		UserId:      "user-1",
		Password:    "correct horse battery staple",
		DeviceId:    "device-1",
		MfaFactorId: "mfa-1",
		MfaCode:     "123456",
	})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if executor.command.MFAFactorID != "mfa-1" || executor.command.MFACode != "123456" {
		t.Fatalf("expected mfa fields to map to command, got %+v", executor.command)
	}
}

func TestServerLoginMapsMFARecoveryCode(t *testing.T) {
	executor := &fakeLoginExecutor{
		result: types.LoginResult{
			TenantID:               "tenant-1",
			UserID:                 "user-1",
			DeviceID:               "device-1",
			SessionID:              "session-1",
			Audience:               "push-gateway",
			TokenType:              "Bearer",
			GatewayToken:           "gateway-token",
			RefreshToken:           "refresh-token",
			GatewayExpiresAtUnixMS: 1_800_000_900_000,
			RefreshExpiresAtUnixMS: 1_802_592_000_000,
			IssuedAtUnixMS:         1_800_000_000_000,
		},
	}
	server := &Server{login: executor}
	_, err := server.Login(context.Background(), &identityv1.LoginRequest{
		TenantId:        "tenant-1",
		UserId:          "user-1",
		Password:        "correct horse battery staple",
		DeviceId:        "device-1",
		MfaRecoveryCode: "aaaa-bbbb-cccc-dddd",
	})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if executor.command.MFARecoveryCode != "aaaa-bbbb-cccc-dddd" {
		t.Fatalf("expected recovery code to map to command, got %+v", executor.command)
	}
}

func TestServerRefreshGatewayTokenMapsMFAFields(t *testing.T) {
	executor := &fakeRefreshGatewayTokenExecutor{
		result: types.RefreshGatewayTokenResult{
			TenantID:               "tenant-1",
			UserID:                 "user-1",
			DeviceID:               "device-1",
			SessionID:              "session-1",
			Audience:               "push-gateway",
			TokenType:              "Bearer",
			GatewayToken:           "gateway-token",
			RefreshToken:           "refresh-token-next",
			GatewayExpiresAtUnixMS: 1_800_000_900_000,
			RefreshExpiresAtUnixMS: 1_802_592_000_000,
			IssuedAtUnixMS:         1_800_000_000_000,
		},
	}
	server := &Server{refreshGatewayToken: executor}
	_, err := server.RefreshGatewayToken(context.Background(), &identityv1.RefreshGatewayTokenRequest{
		TenantId:          "tenant-1",
		UserId:            "user-1",
		DeviceId:          "device-1",
		RefreshToken:      "refresh-token-old",
		MfaFactorId:       "mfa-1",
		MfaCode:           "123456",
		MfaRecoveryCode:   "",
		TraceId:           "trace-1",
		RequestId:         "request-1",
		GatewayTtlSeconds: 900,
		RefreshTtlSeconds: 2_592_000,
	})
	if err != nil {
		t.Fatalf("refresh gateway token: %v", err)
	}
	if executor.command.MFAFactorID != "mfa-1" || executor.command.MFACode != "123456" || executor.command.MFARecoveryCode != "" {
		t.Fatalf("expected refresh mfa fields to map to command, got %+v", executor.command)
	}
	if executor.command.TraceID != "trace-1" || executor.command.RequestID != "request-1" {
		t.Fatalf("expected trace/request to map to command, got %+v", executor.command)
	}
}

func TestServerLoginMapsMFARequiredToFailedPrecondition(t *testing.T) {
	server := &Server{login: &fakeLoginExecutor{err: types.NewMFARequired("mfa required")}}
	_, err := server.Login(context.Background(), &identityv1.LoginRequest{
		TenantId: "tenant-1",
		UserId:   "user-1",
		Password: "correct horse battery staple",
		DeviceId: "device-1",
	})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("expected mfa required to map to failed precondition, got %v (%v)", status.Code(err), err)
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
	server := &Server{requestPasswordReset: executor}
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

func TestServerBeginMFAEnrollmentMapsRequestAndResponse(t *testing.T) {
	executor := &fakeBeginMFAEnrollmentExecutor{
		result: types.BeginMFAEnrollmentResult{
			TenantID:        "tenant-1",
			UserID:          "user-1",
			FactorID:        "mfa-1",
			FactorType:      types.MFAFactorTypeTOTP,
			Status:          types.MFAFactorStatusPending,
			Secret:          "TOTPSECRET",
			OTPAuthURI:      "otpauth://totp/NexusIM:user-1?secret=TOTPSECRET",
			CreatedAtUnixMS: 1_800_000_000_000,
		},
	}
	server := &Server{beginMFAEnrollment: executor}
	response, err := server.BeginMFAEnrollment(context.Background(), &identityv1.BeginMFAEnrollmentRequest{
		TenantId:    "tenant-1",
		UserId:      "user-1",
		FactorType:  identityv1.MFAFactorType_MFA_FACTOR_TYPE_TOTP,
		Password:    "correct horse battery staple",
		DisplayName: "Authenticator",
		Issuer:      "NexusIM",
		TraceId:     "trace-1",
		RequestId:   "request-1",
	})
	if err != nil {
		t.Fatalf("begin mfa enrollment: %v", err)
	}
	if executor.command.FactorType != types.MFAFactorTypeTOTP || executor.command.Password == "" || executor.command.DisplayName != "Authenticator" {
		t.Fatalf("unexpected command: %+v", executor.command)
	}
	if response.GetFactorType() != identityv1.MFAFactorType_MFA_FACTOR_TYPE_TOTP ||
		response.GetStatus() != identityv1.MFAFactorStatus_MFA_FACTOR_STATUS_PENDING ||
		response.GetSecret() != "TOTPSECRET" ||
		response.GetOtpauthUri() == "" {
		t.Fatalf("unexpected response: %+v", response)
	}
}

func TestServerConfirmMFAEnrollmentMapsInvalidMFAToUnauthenticated(t *testing.T) {
	server := &Server{confirmMFAEnrollment: &fakeConfirmMFAEnrollmentExecutor{err: types.NewInvalidMFA("bad code")}}
	_, err := server.ConfirmMFAEnrollment(context.Background(), &identityv1.ConfirmMFAEnrollmentRequest{
		TenantId: "tenant-1",
		UserId:   "user-1",
		FactorId: "mfa-1",
		Code:     "123456",
	})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("expected invalid mfa to map to unauthenticated, got %v (%v)", status.Code(err), err)
	}
}

func TestServerConfirmMFAEnrollmentReturnsRecoveryCodes(t *testing.T) {
	executor := &fakeConfirmMFAEnrollmentExecutor{
		result: types.ConfirmMFAEnrollmentResult{
			TenantID:         "tenant-1",
			UserID:           "user-1",
			FactorID:         "mfa-1",
			Status:           types.MFAFactorStatusActive,
			VerifiedAtUnixMS: 1_800_000_000_000,
			RecoveryCodes:    []string{"aaaa-bbbb-cccc-dddd", "eeee-ffff-gggg-hhhh"},
		},
	}
	server := &Server{confirmMFAEnrollment: executor}
	response, err := server.ConfirmMFAEnrollment(context.Background(), &identityv1.ConfirmMFAEnrollmentRequest{
		TenantId: "tenant-1",
		UserId:   "user-1",
		FactorId: "mfa-1",
		Code:     "123456",
	})
	if err != nil {
		t.Fatalf("confirm mfa enrollment: %v", err)
	}
	if len(response.GetRecoveryCodes()) != 2 || response.GetRecoveryCodes()[0] != "aaaa-bbbb-cccc-dddd" {
		t.Fatalf("expected recovery codes to be returned, got %+v", response.GetRecoveryCodes())
	}
}

func TestServerDisableMFAFactorMapsResponse(t *testing.T) {
	executor := &fakeDisableMFAFactorExecutor{
		result: types.DisableMFAFactorResult{
			TenantID:         "tenant-1",
			UserID:           "user-1",
			FactorID:         "mfa-1",
			Status:           types.MFAFactorStatusDisabled,
			DisabledAtUnixMS: 1_800_000_001_000,
		},
	}
	server := &Server{disableMFAFactor: executor}
	response, err := server.DisableMFAFactor(context.Background(), &identityv1.DisableMFAFactorRequest{
		TenantId: "tenant-1",
		UserId:   "user-1",
		FactorId: "mfa-1",
		Password: "correct horse battery staple",
	})
	if err != nil {
		t.Fatalf("disable mfa factor: %v", err)
	}
	if executor.command.FactorID != "mfa-1" || response.GetStatus() != identityv1.MFAFactorStatus_MFA_FACTOR_STATUS_DISABLED {
		t.Fatalf("unexpected disable mfa mapping: command=%+v response=%+v", executor.command, response)
	}
}

func TestServerRegenerateMFARecoveryCodesMapsRequestAndResponse(t *testing.T) {
	executor := &fakeRegenerateMFARecoveryCodesExecutor{
		result: types.RegenerateMFARecoveryCodesResult{
			TenantID:          "tenant-1",
			UserID:            "user-1",
			FactorID:          "mfa-1",
			RecoveryCodes:     []string{"aaaa-bbbb-cccc-dddd", "eeee-ffff-gggg-hhhh"},
			GeneratedAtUnixMS: 1_800_000_002_000,
		},
	}
	server := &Server{regenerateMFARecoveryCodes: executor}
	response, err := server.RegenerateMFARecoveryCodes(context.Background(), &identityv1.RegenerateMFARecoveryCodesRequest{
		TenantId:  "tenant-1",
		UserId:    "user-1",
		FactorId:  "mfa-1",
		Password:  "correct horse battery staple",
		Code:      "123456",
		TraceId:   "trace-1",
		RequestId: "request-1",
	})
	if err != nil {
		t.Fatalf("regenerate recovery codes: %v", err)
	}
	if executor.command.FactorID != "mfa-1" || executor.command.Code != "123456" || executor.command.Password == "" {
		t.Fatalf("unexpected command: %+v", executor.command)
	}
	if response.GetFactorId() != "mfa-1" || len(response.GetRecoveryCodes()) != 2 || response.GetGeneratedAtUnixMs() != 1_800_000_002_000 {
		t.Fatalf("unexpected response: %+v", response)
	}
}

func TestServerRevokeMFARecoveryCodesMapsRequestAndResponse(t *testing.T) {
	executor := &fakeRevokeMFARecoveryCodesExecutor{
		result: types.RevokeMFARecoveryCodesResult{
			TenantID:        "tenant-1",
			UserID:          "user-1",
			RevokedCount:    2,
			RevokedAtUnixMS: 1_800_000_003_000,
		},
	}
	server := &Server{revokeMFARecoveryCodes: executor}
	response, err := server.RevokeMFARecoveryCodes(context.Background(), &identityv1.RevokeMFARecoveryCodesRequest{
		TenantId:  "tenant-1",
		UserId:    "user-1",
		Password:  "correct horse battery staple",
		TraceId:   "trace-1",
		RequestId: "request-1",
	})
	if err != nil {
		t.Fatalf("revoke recovery codes: %v", err)
	}
	if executor.command.TenantID != "tenant-1" || executor.command.Password == "" {
		t.Fatalf("unexpected command: %+v", executor.command)
	}
	if response.GetRevokedCount() != 2 || response.GetRevokedAtUnixMs() != 1_800_000_003_000 {
		t.Fatalf("unexpected response: %+v", response)
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
	if code := status.Code(grpcError(types.NewChallengeDeliveryFailed("webhook failed"))); code != codes.Unavailable {
		t.Fatalf("expected challenge delivery failure to map to unavailable, got %v", code)
	}
	if code := status.Code(grpcError(types.NewMFALocked("locked"))); code != codes.ResourceExhausted {
		t.Fatalf("expected mfa locked to map to resource exhausted, got %v", code)
	}
	err := grpcError(types.NewMFAUnavailable("mfa secret encryption key is required"))
	if code := status.Code(err); code != codes.Unavailable {
		t.Fatalf("expected mfa unavailable to map to unavailable, got %v", code)
	}
	if msg := status.Convert(err).Message(); msg != "mfa temporarily unavailable" {
		t.Fatalf("expected stable mfa unavailable message, got %q", msg)
	}
}

type fakeRegisterUserExecutor struct {
	command types.RegisterUserCommand
	result  types.RegisterUserResult
	err     error
}

type fakeLoginExecutor struct {
	command types.LoginCommand
	result  types.LoginResult
	err     error
}

type fakeRefreshGatewayTokenExecutor struct {
	command types.RefreshGatewayTokenCommand
	result  types.RefreshGatewayTokenResult
	err     error
}

type fakeRequestPasswordResetExecutor struct {
	command types.RequestPasswordResetCommand
	result  types.RequestPasswordResetResult
	err     error
}

type fakeBeginMFAEnrollmentExecutor struct {
	command types.BeginMFAEnrollmentCommand
	result  types.BeginMFAEnrollmentResult
	err     error
}

type fakeConfirmMFAEnrollmentExecutor struct {
	command types.ConfirmMFAEnrollmentCommand
	result  types.ConfirmMFAEnrollmentResult
	err     error
}

type fakeDisableMFAFactorExecutor struct {
	command types.DisableMFAFactorCommand
	result  types.DisableMFAFactorResult
	err     error
}

type fakeRegenerateMFARecoveryCodesExecutor struct {
	command types.RegenerateMFARecoveryCodesCommand
	result  types.RegenerateMFARecoveryCodesResult
	err     error
}

type fakeRevokeMFARecoveryCodesExecutor struct {
	command types.RevokeMFARecoveryCodesCommand
	result  types.RevokeMFARecoveryCodesResult
	err     error
}

func (executor *fakeLoginExecutor) Execute(_ context.Context, command types.LoginCommand) (types.LoginResult, error) {
	executor.command = command
	if executor.err != nil {
		return types.LoginResult{}, executor.err
	}
	return executor.result, nil
}

func (executor *fakeRefreshGatewayTokenExecutor) Execute(_ context.Context, command types.RefreshGatewayTokenCommand) (types.RefreshGatewayTokenResult, error) {
	executor.command = command
	if executor.err != nil {
		return types.RefreshGatewayTokenResult{}, executor.err
	}
	return executor.result, nil
}

func (executor *fakeRequestPasswordResetExecutor) Execute(_ context.Context, command types.RequestPasswordResetCommand) (types.RequestPasswordResetResult, error) {
	executor.command = command
	if executor.err != nil {
		return types.RequestPasswordResetResult{}, executor.err
	}
	return executor.result, nil
}

func (executor *fakeBeginMFAEnrollmentExecutor) Execute(_ context.Context, command types.BeginMFAEnrollmentCommand) (types.BeginMFAEnrollmentResult, error) {
	executor.command = command
	if executor.err != nil {
		return types.BeginMFAEnrollmentResult{}, executor.err
	}
	return executor.result, nil
}

func (executor *fakeConfirmMFAEnrollmentExecutor) Execute(_ context.Context, command types.ConfirmMFAEnrollmentCommand) (types.ConfirmMFAEnrollmentResult, error) {
	executor.command = command
	if executor.err != nil {
		return types.ConfirmMFAEnrollmentResult{}, executor.err
	}
	return executor.result, nil
}

func (executor *fakeDisableMFAFactorExecutor) Execute(_ context.Context, command types.DisableMFAFactorCommand) (types.DisableMFAFactorResult, error) {
	executor.command = command
	if executor.err != nil {
		return types.DisableMFAFactorResult{}, executor.err
	}
	return executor.result, nil
}

func (executor *fakeRegenerateMFARecoveryCodesExecutor) Execute(_ context.Context, command types.RegenerateMFARecoveryCodesCommand) (types.RegenerateMFARecoveryCodesResult, error) {
	executor.command = command
	if executor.err != nil {
		return types.RegenerateMFARecoveryCodesResult{}, executor.err
	}
	return executor.result, nil
}

func (executor *fakeRevokeMFARecoveryCodesExecutor) Execute(_ context.Context, command types.RevokeMFARecoveryCodesCommand) (types.RevokeMFARecoveryCodesResult, error) {
	executor.command = command
	if executor.err != nil {
		return types.RevokeMFARecoveryCodesResult{}, executor.err
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
