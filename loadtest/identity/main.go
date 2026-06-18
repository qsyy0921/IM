package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	gatewayv1 "github.com/qsyy0921/IM/api/proto/nexusim/gateway/v1"
	identityv1 "github.com/qsyy0921/IM/api/proto/nexusim/identity/v1"
	"github.com/qsyy0921/IM/loadtest/internal/grpctls"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const defaultIdentityTenantID = "tenant-identity-smoke"

func main() {
	cfg := parseConfig()
	var err error
	switch cfg.mode {
	case "client":
		err = runClient(cfg)
	case "webhook":
		err = runWebhook(cfg)
	default:
		err = fmt.Errorf("unsupported mode %q", cfg.mode)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runClient(cfg config) error {
	started := time.Now().UTC()
	if cfg.duration > 0 && cfg.tenantID == defaultIdentityTenantID {
		cfg.tenantID = cfg.tenantID + "-" + started.Format("20060102150405")
	}
	result := summary{
		Commit:        gitOutput("rev-parse", "--short", "HEAD"),
		CommitFull:    gitOutput("rev-parse", "HEAD"),
		GitDirty:      gitDirty(),
		Target:        cfg.target,
		GatewayFacade: cfg.gatewayFacade,
		TLSEnabled:    cfg.tls.Enabled(),
		ResultDir:     cfg.resultDir,
		TenantID:      cfg.tenantID,
		UserID:        cfg.userID,
		Destination:   cfg.destination,
		StartedAt:     started,
		CapacityMode:  cfg.duration > 0,
		VUs:           cfg.vus,
		LatenciesMS:   map[string]float64{},
	}
	if cfg.duration > 0 {
		result.ConfiguredDurationSeconds = cfg.duration.Seconds()
	}
	if status := gitOutput("status", "--short"); strings.TrimSpace(status) != "" {
		result.GitStatusShort = status
	}
	if err := os.MkdirAll(cfg.resultDir, 0o755); err != nil {
		return fmt.Errorf("create result dir: %w", err)
	}
	runErr := runClientScenario(cfg, &result)
	return finish(cfg, &result, runErr)
}

func runClientScenario(cfg config, result *summary) error {
	ctx := context.Background()
	var pool *pgxpool.Pool
	if cfg.pgDSN != "" {
		var err error
		pool, err = pgxpool.New(ctx, cfg.pgDSN)
		if err != nil {
			return fmt.Errorf("connect postgres: %w", err)
		}
		defer pool.Close()
		if cfg.cleanup {
			if err := cleanupTenant(ctx, pool, cfg); err != nil {
				return err
			}
		}
	}

	dialOption, err := grpctls.DialOption(cfg.tls, "identity-tls")
	if err != nil {
		return fmt.Errorf("configure identity smoke target TLS: %w", err)
	}
	conn, err := grpc.NewClient(cfg.target, dialOption)
	if err != nil {
		return fmt.Errorf("connect identity smoke target: %w", err)
	}
	defer conn.Close()
	var client identityChallengeClient = identityv1.NewIdentityServiceClient(conn)
	if cfg.gatewayFacade {
		client = gatewayv1.NewGatewayServiceClient(conn)
	}
	if cfg.duration > 0 {
		return runCapacityScenario(ctx, cfg, client, pool, result)
	}

	registerStarted := time.Now()
	registerCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	register, err := client.RegisterUser(registerCtx, &identityv1.RegisterUserRequest{
		TenantId:  cfg.tenantID,
		UserId:    cfg.userID,
		Password:  cfg.password,
		TraceId:   "identity-challenge-delivery-outbox-smoke",
		RequestId: "identity-smoke-register",
	})
	cancel()
	result.LatenciesMS["register_user"] = elapsedMS(registerStarted)
	if err != nil {
		return fmt.Errorf("register user: %w", err)
	}
	result.RegisterUser = registerSummary{
		Status:          register.GetStatus().String(),
		CreatedAtUnixMS: register.GetCreatedAtUnixMs(),
	}

	requestStarted := time.Now()
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	challenge, err := client.RequestVerificationChallenge(requestCtx, &identityv1.RequestVerificationChallengeRequest{
		TenantId:    cfg.tenantID,
		UserId:      cfg.userID,
		Channel:     identityv1.VerificationChannel_VERIFICATION_CHANNEL_EMAIL,
		Destination: cfg.destination,
		TtlSeconds:  300,
		TraceId:     "identity-challenge-delivery-outbox-smoke",
		RequestId:   "identity-smoke-request-challenge",
		Password:    cfg.password,
	})
	cancel()
	result.LatenciesMS["request_verification_challenge"] = elapsedMS(requestStarted)
	if err != nil {
		return fmt.Errorf("request verification challenge: %w", err)
	}
	result.RequestChallenge = challengeSummary{
		ChallengeID:          challenge.GetChallengeId(),
		Channel:              challenge.GetChannel().String(),
		Destination:          challenge.GetDestination(),
		ExpiresAtUnixMS:      challenge.GetExpiresAtUnixMs(),
		DevChallengeTokenSet: challenge.GetDevChallengeToken() != "",
	}
	if challenge.GetDevChallengeToken() != "" {
		return errors.New("identity-service returned dev challenge token; smoke must prove worker webhook delivery")
	}

	notification, err := waitWebhookNotification(cfg, challenge.GetChallengeId())
	if err != nil {
		return err
	}
	result.Webhook = webhookSummary{
		Received:        true,
		ChallengeID:     notification.ChallengeID,
		ChallengeType:   notification.ChallengeType,
		Channel:         notification.Channel,
		Destination:     notification.Destination,
		TokenSet:        notification.Token != "",
		RequestID:       notification.RequestID,
		AuthorizationOK: cfg.webhookBearerToken == "" || notification.Authorization == "Bearer "+cfg.webhookBearerToken,
	}
	if notification.Token == "" {
		return errors.New("webhook notification did not include token")
	}
	if notification.TenantID != cfg.tenantID || notification.UserID != cfg.userID {
		return fmt.Errorf("webhook notification identity mismatch: %+v", notification)
	}
	if notification.ChallengeID != challenge.GetChallengeId() {
		return fmt.Errorf("webhook challenge id mismatch: webhook=%s response=%s", notification.ChallengeID, challenge.GetChallengeId())
	}

	confirmStarted := time.Now()
	confirmCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	confirm, err := client.ConfirmVerificationChallenge(confirmCtx, &identityv1.ConfirmVerificationChallengeRequest{
		TenantId:       cfg.tenantID,
		UserId:         cfg.userID,
		ChallengeId:    challenge.GetChallengeId(),
		ChallengeToken: notification.Token,
		TraceId:        "identity-challenge-delivery-outbox-smoke",
		RequestId:      "identity-smoke-confirm-challenge",
	})
	cancel()
	result.LatenciesMS["confirm_verification_challenge"] = elapsedMS(confirmStarted)
	if err != nil {
		return fmt.Errorf("confirm verification challenge: %w", err)
	}
	result.ConfirmChallenge = confirmSummary{
		Channel:          confirm.GetChannel().String(),
		Destination:      confirm.GetDestination(),
		VerifiedAtUnixMS: confirm.GetVerifiedAtUnixMs(),
	}

	loginStarted := time.Now()
	loginCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	login, err := client.Login(loginCtx, &identityv1.LoginRequest{
		TenantId:          cfg.tenantID,
		UserId:            cfg.userID,
		Password:          cfg.password,
		DeviceId:          cfg.deviceID,
		Audience:          cfg.audience,
		GatewayTtlSeconds: 900,
		RefreshTtlSeconds: 3600,
		TraceId:           "identity-challenge-delivery-outbox-smoke",
		RequestId:         "identity-smoke-login",
	})
	cancel()
	result.LatenciesMS["login"] = elapsedMS(loginStarted)
	if err != nil {
		return fmt.Errorf("login: %w", err)
	}
	result.Login = tokenSummary{
		Audience:               login.GetAudience(),
		TokenType:              login.GetTokenType(),
		SessionIDSet:           login.GetSessionId() != "",
		GatewayTokenSet:        login.GetGatewayToken() != "",
		RefreshTokenSet:        login.GetRefreshToken() != "",
		GatewayExpiresAtUnixMS: login.GetGatewayExpiresAtUnixMs(),
		RefreshExpiresAtUnixMS: login.GetRefreshExpiresAtUnixMs(),
		IssuedAtUnixMS:         login.GetIssuedAtUnixMs(),
	}
	if login.GetGatewayToken() == "" || login.GetRefreshToken() == "" || login.GetSessionId() == "" {
		return errors.New("login did not return gateway token, refresh token, and session id")
	}

	refreshStarted := time.Now()
	refreshCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	refresh, err := client.RefreshGatewayToken(refreshCtx, &identityv1.RefreshGatewayTokenRequest{
		TenantId:          cfg.tenantID,
		UserId:            cfg.userID,
		DeviceId:          cfg.deviceID,
		RefreshToken:      login.GetRefreshToken(),
		Audience:          cfg.audience,
		GatewayTtlSeconds: 900,
		RefreshTtlSeconds: 3600,
		TraceId:           "identity-challenge-delivery-outbox-smoke",
		RequestId:         "identity-smoke-refresh",
	})
	cancel()
	result.LatenciesMS["refresh_gateway_token"] = elapsedMS(refreshStarted)
	if err != nil {
		return fmt.Errorf("refresh gateway token: %w", err)
	}
	result.Refresh = tokenSummary{
		Audience:               refresh.GetAudience(),
		TokenType:              refresh.GetTokenType(),
		SessionIDSet:           refresh.GetSessionId() != "",
		GatewayTokenSet:        refresh.GetGatewayToken() != "",
		RefreshTokenSet:        refresh.GetRefreshToken() != "",
		RefreshTokenRotated:    refresh.GetRefreshToken() != "" && refresh.GetRefreshToken() != login.GetRefreshToken(),
		GatewayExpiresAtUnixMS: refresh.GetGatewayExpiresAtUnixMs(),
		RefreshExpiresAtUnixMS: refresh.GetRefreshExpiresAtUnixMs(),
		IssuedAtUnixMS:         refresh.GetIssuedAtUnixMs(),
	}
	if refresh.GetGatewayToken() == "" || refresh.GetRefreshToken() == "" || refresh.GetSessionId() == "" {
		return errors.New("refresh did not return gateway token, refresh token, and session id")
	}
	if refresh.GetRefreshToken() == login.GetRefreshToken() {
		return errors.New("refresh did not rotate refresh token")
	}

	passwordResetStarted := time.Now()
	passwordResetCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	passwordReset, err := client.RequestPasswordReset(passwordResetCtx, &identityv1.RequestPasswordResetRequest{
		TenantId:    cfg.tenantID,
		UserId:      cfg.userID,
		Channel:     identityv1.VerificationChannel_VERIFICATION_CHANNEL_EMAIL,
		Destination: cfg.destination,
		TtlSeconds:  300,
		TraceId:     "identity-challenge-delivery-outbox-smoke",
		RequestId:   "identity-smoke-request-password-reset",
	})
	cancel()
	result.LatenciesMS["request_password_reset"] = elapsedMS(passwordResetStarted)
	if err != nil {
		return fmt.Errorf("request password reset: %w", err)
	}
	result.RequestPasswordReset = challengeSummary{
		ChallengeID:          passwordReset.GetChallengeId(),
		Channel:              passwordReset.GetChannel().String(),
		Destination:          passwordReset.GetDestination(),
		ExpiresAtUnixMS:      passwordReset.GetExpiresAtUnixMs(),
		DevChallengeTokenSet: passwordReset.GetDevChallengeToken() != "",
	}
	if passwordReset.GetDevChallengeToken() != "" {
		return errors.New("identity-service returned dev password reset token; smoke must prove worker webhook delivery")
	}

	resetNotification, err := waitWebhookNotification(cfg, passwordReset.GetChallengeId())
	if err != nil {
		return err
	}
	result.PasswordResetWebhook = webhookSummary{
		Received:        true,
		ChallengeID:     resetNotification.ChallengeID,
		ChallengeType:   resetNotification.ChallengeType,
		Channel:         resetNotification.Channel,
		Destination:     resetNotification.Destination,
		TokenSet:        resetNotification.Token != "",
		RequestID:       resetNotification.RequestID,
		AuthorizationOK: cfg.webhookBearerToken == "" || resetNotification.Authorization == "Bearer "+cfg.webhookBearerToken,
	}
	if resetNotification.Token == "" {
		return errors.New("password reset webhook notification did not include token")
	}
	if resetNotification.TenantID != cfg.tenantID || resetNotification.UserID != cfg.userID {
		return fmt.Errorf("password reset webhook notification identity mismatch: %+v", resetNotification)
	}
	if resetNotification.ChallengeID != passwordReset.GetChallengeId() {
		return fmt.Errorf("password reset challenge id mismatch: webhook=%s response=%s", resetNotification.ChallengeID, passwordReset.GetChallengeId())
	}

	confirmResetStarted := time.Now()
	confirmResetCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	confirmReset, err := client.ConfirmPasswordReset(confirmResetCtx, &identityv1.ConfirmPasswordResetRequest{
		TenantId:       cfg.tenantID,
		UserId:         cfg.userID,
		ChallengeId:    passwordReset.GetChallengeId(),
		ChallengeToken: resetNotification.Token,
		NewPassword:    cfg.newPassword,
		TraceId:        "identity-challenge-delivery-outbox-smoke",
		RequestId:      "identity-smoke-confirm-password-reset",
	})
	cancel()
	result.LatenciesMS["confirm_password_reset"] = elapsedMS(confirmResetStarted)
	if err != nil {
		return fmt.Errorf("confirm password reset: %w", err)
	}
	result.ConfirmPasswordReset = passwordResetSummary{
		ResetAtUnixMS: confirmReset.GetResetAtUnixMs(),
	}
	if confirmReset.GetResetAtUnixMs() == 0 {
		return errors.New("confirm password reset did not return reset timestamp")
	}

	postResetLoginStarted := time.Now()
	postResetLoginCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	postResetLogin, err := client.Login(postResetLoginCtx, &identityv1.LoginRequest{
		TenantId:          cfg.tenantID,
		UserId:            cfg.userID,
		Password:          cfg.newPassword,
		DeviceId:          cfg.deviceID,
		Audience:          cfg.audience,
		GatewayTtlSeconds: 900,
		RefreshTtlSeconds: 3600,
		TraceId:           "identity-challenge-delivery-outbox-smoke",
		RequestId:         "identity-smoke-post-reset-login",
	})
	cancel()
	result.LatenciesMS["post_reset_login"] = elapsedMS(postResetLoginStarted)
	if err != nil {
		return fmt.Errorf("post-reset login: %w", err)
	}
	result.PostResetLogin = tokenSummary{
		Audience:               postResetLogin.GetAudience(),
		TokenType:              postResetLogin.GetTokenType(),
		SessionIDSet:           postResetLogin.GetSessionId() != "",
		GatewayTokenSet:        postResetLogin.GetGatewayToken() != "",
		RefreshTokenSet:        postResetLogin.GetRefreshToken() != "",
		GatewayExpiresAtUnixMS: postResetLogin.GetGatewayExpiresAtUnixMs(),
		RefreshExpiresAtUnixMS: postResetLogin.GetRefreshExpiresAtUnixMs(),
		IssuedAtUnixMS:         postResetLogin.GetIssuedAtUnixMs(),
	}
	if postResetLogin.GetGatewayToken() == "" || postResetLogin.GetRefreshToken() == "" || postResetLogin.GetSessionId() == "" {
		return errors.New("post-reset login did not return gateway token, refresh token, and session id")
	}

	if err := runMFAScenario(ctx, cfg, client, postResetLogin.GetRefreshToken(), result); err != nil {
		return err
	}

	if pool != nil {
		if err := fillPostgresStats(ctx, pool, cfg, challenge.GetChallengeId(), result); err != nil {
			return err
		}
		if result.ChallengeDeliveryOutbox.Pending != 0 || result.ChallengeDeliveryOutbox.DLQ != 0 {
			return fmt.Errorf("challenge delivery outbox did not drain: %+v", result.ChallengeDeliveryOutbox)
		}
		expectedDelivered := int64(1)
		if result.RequestPasswordReset.ChallengeID != "" {
			expectedDelivered = 2
		}
		if result.ChallengeDeliveryOutbox.Delivered < expectedDelivered {
			return fmt.Errorf("challenge delivery outbox was not delivered: %+v", result.ChallengeDeliveryOutbox)
		}
		if result.ChallengeRow.Status != "CONSUMED" || result.ChallengeRow.DeliveryStatus != "DELIVERED" {
			return fmt.Errorf("unexpected challenge row: %+v", result.ChallengeRow)
		}
	}

	return nil
}

func runMFAScenario(ctx context.Context, cfg config, client identityChallengeClient, preMFARefreshToken string, result *summary) error {
	beginStarted := time.Now()
	beginCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	begin, err := client.BeginMFAEnrollment(beginCtx, &identityv1.BeginMFAEnrollmentRequest{
		TenantId:    cfg.tenantID,
		UserId:      cfg.userID,
		FactorType:  identityv1.MFAFactorType_MFA_FACTOR_TYPE_TOTP,
		Password:    cfg.newPassword,
		DisplayName: "identity facade smoke",
		Issuer:      "NexusIM",
		TraceId:     "identity-challenge-delivery-outbox-smoke",
		RequestId:   "identity-smoke-begin-mfa",
	})
	cancel()
	result.LatenciesMS["begin_mfa_enrollment"] = elapsedMS(beginStarted)
	if err != nil {
		return fmt.Errorf("begin mfa enrollment: %w", err)
	}
	result.BeginMFAEnrollment = mfaBeginSummary{
		FactorIDSet:     begin.GetFactorId() != "",
		FactorType:      begin.GetFactorType().String(),
		Status:          begin.GetStatus().String(),
		SecretSet:       begin.GetSecret() != "",
		OTPAuthURISet:   begin.GetOtpauthUri() != "",
		CreatedAtUnixMS: begin.GetCreatedAtUnixMs(),
	}
	if begin.GetFactorId() == "" || begin.GetSecret() == "" || begin.GetStatus() != identityv1.MFAFactorStatus_MFA_FACTOR_STATUS_PENDING {
		return fmt.Errorf("unexpected begin mfa response: factor_id_set=%t secret_set=%t status=%s", begin.GetFactorId() != "", begin.GetSecret() != "", begin.GetStatus())
	}

	code := generateTOTPCode(begin.GetSecret(), time.Now().UTC())
	if code == "" {
		return errors.New("failed to generate totp code for mfa enrollment")
	}
	confirmStarted := time.Now()
	confirmCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	confirm, err := client.ConfirmMFAEnrollment(confirmCtx, &identityv1.ConfirmMFAEnrollmentRequest{
		TenantId:  cfg.tenantID,
		UserId:    cfg.userID,
		FactorId:  begin.GetFactorId(),
		Code:      code,
		TraceId:   "identity-challenge-delivery-outbox-smoke",
		RequestId: "identity-smoke-confirm-mfa",
	})
	cancel()
	result.LatenciesMS["confirm_mfa_enrollment"] = elapsedMS(confirmStarted)
	if err != nil {
		return fmt.Errorf("confirm mfa enrollment: %w", err)
	}
	result.ConfirmMFAEnrollment = mfaConfirmSummary{
		FactorIDSet:       confirm.GetFactorId() != "",
		Status:            confirm.GetStatus().String(),
		VerifiedAtUnixMS:  confirm.GetVerifiedAtUnixMs(),
		RecoveryCodeCount: len(confirm.GetRecoveryCodes()),
	}
	if confirm.GetStatus() != identityv1.MFAFactorStatus_MFA_FACTOR_STATUS_ACTIVE || confirm.GetVerifiedAtUnixMs() == 0 || len(confirm.GetRecoveryCodes()) == 0 {
		return fmt.Errorf("unexpected confirm mfa response: status=%s verified_at=%d recovery_count=%d", confirm.GetStatus(), confirm.GetVerifiedAtUnixMs(), len(confirm.GetRecoveryCodes()))
	}
	if strings.TrimSpace(preMFARefreshToken) == "" {
		return errors.New("pre-MFA refresh token is required for refresh step-up smoke")
	}

	refreshWithoutMFAStarted := time.Now()
	refreshWithoutMFACtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	_, err = client.RefreshGatewayToken(refreshWithoutMFACtx, &identityv1.RefreshGatewayTokenRequest{
		TenantId:          cfg.tenantID,
		UserId:            cfg.userID,
		DeviceId:          cfg.deviceID,
		RefreshToken:      preMFARefreshToken,
		Audience:          cfg.audience,
		GatewayTtlSeconds: 900,
		RefreshTtlSeconds: 3600,
		TraceId:           "identity-challenge-delivery-outbox-smoke",
		RequestId:         "identity-smoke-refresh-without-mfa",
	})
	cancel()
	result.LatenciesMS["refresh_without_mfa"] = elapsedMS(refreshWithoutMFAStarted)
	if err == nil {
		return errors.New("refresh without mfa unexpectedly succeeded after factor activation")
	}
	result.RefreshWithoutMFA = expectedErrorSummary{Occurred: true, Code: status.Code(err).String()}
	if status.Code(err) != codes.FailedPrecondition {
		return fmt.Errorf("refresh without mfa returned %s, want %s: %w", status.Code(err), codes.FailedPrecondition, err)
	}

	refreshWithMFACode := generateTOTPCode(begin.GetSecret(), time.Now().UTC())
	if refreshWithMFACode == "" {
		return errors.New("failed to generate totp code for refresh step-up")
	}
	refreshWithMFAStarted := time.Now()
	refreshWithMFACtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	refreshWithMFA, err := client.RefreshGatewayToken(refreshWithMFACtx, &identityv1.RefreshGatewayTokenRequest{
		TenantId:          cfg.tenantID,
		UserId:            cfg.userID,
		DeviceId:          cfg.deviceID,
		RefreshToken:      preMFARefreshToken,
		Audience:          cfg.audience,
		GatewayTtlSeconds: 900,
		RefreshTtlSeconds: 3600,
		TraceId:           "identity-challenge-delivery-outbox-smoke",
		RequestId:         "identity-smoke-refresh-with-mfa",
		MfaFactorId:       begin.GetFactorId(),
		MfaCode:           refreshWithMFACode,
	})
	cancel()
	result.LatenciesMS["refresh_with_mfa"] = elapsedMS(refreshWithMFAStarted)
	if err != nil {
		return fmt.Errorf("refresh with mfa: %w", err)
	}
	result.RefreshWithMFA = tokenSummary{
		Audience:               refreshWithMFA.GetAudience(),
		TokenType:              refreshWithMFA.GetTokenType(),
		SessionIDSet:           refreshWithMFA.GetSessionId() != "",
		GatewayTokenSet:        refreshWithMFA.GetGatewayToken() != "",
		RefreshTokenSet:        refreshWithMFA.GetRefreshToken() != "",
		RefreshTokenRotated:    refreshWithMFA.GetRefreshToken() != "" && refreshWithMFA.GetRefreshToken() != preMFARefreshToken,
		GatewayExpiresAtUnixMS: refreshWithMFA.GetGatewayExpiresAtUnixMs(),
		RefreshExpiresAtUnixMS: refreshWithMFA.GetRefreshExpiresAtUnixMs(),
		IssuedAtUnixMS:         refreshWithMFA.GetIssuedAtUnixMs(),
	}
	if refreshWithMFA.GetGatewayToken() == "" || refreshWithMFA.GetRefreshToken() == "" || refreshWithMFA.GetSessionId() == "" {
		return errors.New("refresh with mfa did not return gateway token, refresh token, and session id")
	}
	if refreshWithMFA.GetRefreshToken() == preMFARefreshToken {
		return errors.New("refresh with mfa did not rotate refresh token")
	}

	loginWithoutMFAStarted := time.Now()
	loginWithoutMFACtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	_, err = client.Login(loginWithoutMFACtx, &identityv1.LoginRequest{
		TenantId:          cfg.tenantID,
		UserId:            cfg.userID,
		Password:          cfg.newPassword,
		DeviceId:          cfg.deviceID,
		Audience:          cfg.audience,
		GatewayTtlSeconds: 900,
		RefreshTtlSeconds: 3600,
		TraceId:           "identity-challenge-delivery-outbox-smoke",
		RequestId:         "identity-smoke-login-without-mfa",
	})
	cancel()
	result.LatenciesMS["login_without_mfa"] = elapsedMS(loginWithoutMFAStarted)
	if err == nil {
		return errors.New("login without mfa unexpectedly succeeded after factor activation")
	}
	result.LoginWithoutMFA = expectedErrorSummary{Occurred: true, Code: status.Code(err).String()}
	if status.Code(err) != codes.FailedPrecondition {
		return fmt.Errorf("login without mfa returned %s, want %s: %w", status.Code(err), codes.FailedPrecondition, err)
	}

	mfaLoginCode := generateTOTPCode(begin.GetSecret(), time.Now().UTC())
	if mfaLoginCode == "" {
		return errors.New("failed to generate totp code for mfa login")
	}
	mfaLoginStarted := time.Now()
	mfaLoginCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	mfaLogin, err := client.Login(mfaLoginCtx, &identityv1.LoginRequest{
		TenantId:          cfg.tenantID,
		UserId:            cfg.userID,
		Password:          cfg.newPassword,
		DeviceId:          cfg.deviceID,
		Audience:          cfg.audience,
		GatewayTtlSeconds: 900,
		RefreshTtlSeconds: 3600,
		TraceId:           "identity-challenge-delivery-outbox-smoke",
		RequestId:         "identity-smoke-mfa-login",
		MfaFactorId:       begin.GetFactorId(),
		MfaCode:           mfaLoginCode,
	})
	cancel()
	result.LatenciesMS["mfa_login"] = elapsedMS(mfaLoginStarted)
	if err != nil {
		return fmt.Errorf("mfa login: %w", err)
	}
	result.MFALogin = tokenSummary{
		Audience:               mfaLogin.GetAudience(),
		TokenType:              mfaLogin.GetTokenType(),
		SessionIDSet:           mfaLogin.GetSessionId() != "",
		GatewayTokenSet:        mfaLogin.GetGatewayToken() != "",
		RefreshTokenSet:        mfaLogin.GetRefreshToken() != "",
		GatewayExpiresAtUnixMS: mfaLogin.GetGatewayExpiresAtUnixMs(),
		RefreshExpiresAtUnixMS: mfaLogin.GetRefreshExpiresAtUnixMs(),
		IssuedAtUnixMS:         mfaLogin.GetIssuedAtUnixMs(),
	}
	if mfaLogin.GetGatewayToken() == "" || mfaLogin.GetRefreshToken() == "" || mfaLogin.GetSessionId() == "" {
		return errors.New("mfa login did not return gateway token, refresh token, and session id")
	}

	regenerateCode := generateTOTPCode(begin.GetSecret(), time.Now().UTC())
	if regenerateCode == "" {
		return errors.New("failed to generate totp code for recovery code regeneration")
	}
	regenerateStarted := time.Now()
	regenerateCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	regenerate, err := client.RegenerateMFARecoveryCodes(regenerateCtx, &identityv1.RegenerateMFARecoveryCodesRequest{
		TenantId:  cfg.tenantID,
		UserId:    cfg.userID,
		FactorId:  begin.GetFactorId(),
		Password:  cfg.newPassword,
		Code:      regenerateCode,
		TraceId:   "identity-challenge-delivery-outbox-smoke",
		RequestId: "identity-smoke-regenerate-mfa-recovery",
	})
	cancel()
	result.LatenciesMS["regenerate_mfa_recovery_codes"] = elapsedMS(regenerateStarted)
	if err != nil {
		return fmt.Errorf("regenerate mfa recovery codes: %w", err)
	}
	result.RegenerateMFARecoveryCodes = mfaRegenerateSummary{
		FactorIDSet:       regenerate.GetFactorId() != "",
		RecoveryCodeCount: len(regenerate.GetRecoveryCodes()),
		GeneratedAtUnixMS: regenerate.GetGeneratedAtUnixMs(),
	}
	if regenerate.GetFactorId() != begin.GetFactorId() || regenerate.GetGeneratedAtUnixMs() == 0 || len(regenerate.GetRecoveryCodes()) == 0 {
		return fmt.Errorf("unexpected regenerate recovery response: factor_id_match=%t generated_at=%d recovery_count=%d", regenerate.GetFactorId() == begin.GetFactorId(), regenerate.GetGeneratedAtUnixMs(), len(regenerate.GetRecoveryCodes()))
	}

	revokeStarted := time.Now()
	revokeCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	revoke, err := client.RevokeMFARecoveryCodes(revokeCtx, &identityv1.RevokeMFARecoveryCodesRequest{
		TenantId:  cfg.tenantID,
		UserId:    cfg.userID,
		Password:  cfg.newPassword,
		TraceId:   "identity-challenge-delivery-outbox-smoke",
		RequestId: "identity-smoke-revoke-mfa-recovery",
	})
	cancel()
	result.LatenciesMS["revoke_mfa_recovery_codes"] = elapsedMS(revokeStarted)
	if err != nil {
		return fmt.Errorf("revoke mfa recovery codes: %w", err)
	}
	result.RevokeMFARecoveryCodes = mfaRevokeSummary{
		RevokedCount:    revoke.GetRevokedCount(),
		RevokedAtUnixMS: revoke.GetRevokedAtUnixMs(),
	}
	if revoke.GetRevokedCount() <= 0 || revoke.GetRevokedAtUnixMs() == 0 {
		return fmt.Errorf("unexpected revoke recovery response: revoked=%d revoked_at=%d", revoke.GetRevokedCount(), revoke.GetRevokedAtUnixMs())
	}

	disableStarted := time.Now()
	disableCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	disable, err := client.DisableMFAFactor(disableCtx, &identityv1.DisableMFAFactorRequest{
		TenantId:  cfg.tenantID,
		UserId:    cfg.userID,
		FactorId:  begin.GetFactorId(),
		Password:  cfg.newPassword,
		TraceId:   "identity-challenge-delivery-outbox-smoke",
		RequestId: "identity-smoke-disable-mfa",
	})
	cancel()
	result.LatenciesMS["disable_mfa_factor"] = elapsedMS(disableStarted)
	if err != nil {
		return fmt.Errorf("disable mfa factor: %w", err)
	}
	result.DisableMFAFactor = mfaDisableSummary{
		FactorIDSet:      disable.GetFactorId() != "",
		Status:           disable.GetStatus().String(),
		DisabledAtUnixMS: disable.GetDisabledAtUnixMs(),
	}
	if disable.GetFactorId() != begin.GetFactorId() || disable.GetStatus() != identityv1.MFAFactorStatus_MFA_FACTOR_STATUS_DISABLED || disable.GetDisabledAtUnixMs() == 0 {
		return fmt.Errorf("unexpected disable mfa response: factor_id_match=%t status=%s disabled_at=%d", disable.GetFactorId() == begin.GetFactorId(), disable.GetStatus(), disable.GetDisabledAtUnixMs())
	}
	return nil
}

func cleanupTenant(ctx context.Context, pool *pgxpool.Pool, cfg config) error {
	statements := []string{
		`DELETE FROM identity_challenge_delivery_outbox WHERE tenant_id = $1`,
		`DELETE FROM identity_mfa_recovery_codes WHERE tenant_id = $1`,
		`DELETE FROM identity_mfa_factors WHERE tenant_id = $1`,
		`DELETE FROM identity_challenges WHERE tenant_id = $1`,
		`DELETE FROM identity_outbox WHERE tenant_id = $1`,
		`DELETE FROM identity_refresh_tokens WHERE tenant_id = $1`,
		`DELETE FROM identity_sessions WHERE tenant_id = $1`,
		`DELETE FROM identity_devices WHERE tenant_id = $1`,
		`DELETE FROM identity_users WHERE tenant_id = $1`,
	}
	for _, statement := range statements {
		if _, err := pool.Exec(ctx, statement, cfg.tenantID); err != nil {
			return fmt.Errorf("cleanup tenant: %w", err)
		}
	}
	return nil
}

func fillPostgresStats(ctx context.Context, pool *pgxpool.Pool, cfg config, challengeID string, result *summary) error {
	rows, err := pool.Query(ctx, `
SELECT status, COUNT(*)
FROM identity_challenge_delivery_outbox
WHERE tenant_id = $1 AND user_id = $2
GROUP BY status
`, cfg.tenantID, cfg.userID)
	if err != nil {
		return fmt.Errorf("query challenge delivery outbox stats: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var count int64
		if err := rows.Scan(&status, &count); err != nil {
			return fmt.Errorf("scan challenge delivery outbox stats: %w", err)
		}
		result.ChallengeDeliveryOutbox.Total += count
		switch status {
		case "PENDING":
			result.ChallengeDeliveryOutbox.Pending = count
		case "DELIVERED":
			result.ChallengeDeliveryOutbox.Delivered = count
		case "DLQ":
			result.ChallengeDeliveryOutbox.DLQ = count
		case "CANCELED":
			result.ChallengeDeliveryOutbox.Canceled = count
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate challenge delivery outbox stats: %w", err)
	}

	var deliveredAt, deadLetteredAt pgtype.Timestamptz
	if err := pool.QueryRow(ctx, `
SELECT status, retry_count, COALESCE(last_error, ''), delivered_at, dead_lettered_at
FROM identity_challenge_delivery_outbox
WHERE tenant_id = $1 AND user_id = $2 AND challenge_id = $3
`, cfg.tenantID, cfg.userID, challengeID).Scan(
		&result.ChallengeDeliveryOutboxRow.Status,
		&result.ChallengeDeliveryOutboxRow.RetryCount,
		&result.ChallengeDeliveryOutboxRow.LastError,
		&deliveredAt,
		&deadLetteredAt,
	); err != nil {
		return fmt.Errorf("query challenge delivery outbox row: %w", err)
	}
	result.ChallengeDeliveryOutboxRow.Delivered = deliveredAt.Valid
	result.ChallengeDeliveryOutboxRow.DLQ = deadLetteredAt.Valid

	if err := pool.QueryRow(ctx, `
SELECT status, delivery_status, delivery_attempt_count, delivery_last_error
FROM identity_challenges
WHERE tenant_id = $1 AND user_id = $2 AND challenge_id = $3
`, cfg.tenantID, cfg.userID, challengeID).Scan(
		&result.ChallengeRow.Status,
		&result.ChallengeRow.DeliveryStatus,
		&result.ChallengeRow.DeliveryAttemptCount,
		&result.ChallengeRow.DeliveryLastError,
	); err != nil {
		return fmt.Errorf("query challenge row: %w", err)
	}

	return nil
}
