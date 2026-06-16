package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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

type config struct {
	mode               string
	target             string
	gatewayFacade      bool
	tls                grpctls.Config
	resultDir          string
	pgDSN              string
	webhookListen      string
	webhookFile        string
	webhookBearerToken string
	requestTimeout     time.Duration
	waitTimeout        time.Duration
	pollInterval       time.Duration
	tenantID           string
	userID             string
	deviceID           string
	audience           string
	password           string
	newPassword        string
	destination        string
	cleanup            bool
}

type summary struct {
	Commit                     string               `json:"commit"`
	CommitFull                 string               `json:"commit_full"`
	GitDirty                   bool                 `json:"git_dirty"`
	GitStatusShort             string               `json:"git_status_short,omitempty"`
	Target                     string               `json:"target"`
	GatewayFacade              bool                 `json:"gateway_facade"`
	TLSEnabled                 bool                 `json:"tls_enabled"`
	ResultDir                  string               `json:"result_dir"`
	TenantID                   string               `json:"tenant_id"`
	UserID                     string               `json:"user_id"`
	Destination                string               `json:"destination"`
	StartedAt                  time.Time            `json:"started_at"`
	FinishedAt                 time.Time            `json:"finished_at"`
	Success                    bool                 `json:"success"`
	Error                      string               `json:"error,omitempty"`
	RegisterUser               registerSummary      `json:"register_user"`
	RequestChallenge           challengeSummary     `json:"request_verification_challenge"`
	Webhook                    webhookSummary       `json:"webhook"`
	ConfirmChallenge           confirmSummary       `json:"confirm_verification_challenge"`
	Login                      tokenSummary         `json:"login"`
	Refresh                    tokenSummary         `json:"refresh_gateway_token"`
	RequestPasswordReset       challengeSummary     `json:"request_password_reset"`
	PasswordResetWebhook       webhookSummary       `json:"password_reset_webhook"`
	ConfirmPasswordReset       passwordResetSummary `json:"confirm_password_reset"`
	PostResetLogin             tokenSummary         `json:"post_reset_login"`
	BeginMFAEnrollment         mfaBeginSummary      `json:"begin_mfa_enrollment"`
	ConfirmMFAEnrollment       mfaConfirmSummary    `json:"confirm_mfa_enrollment"`
	RefreshWithoutMFA          expectedErrorSummary `json:"refresh_without_mfa"`
	RefreshWithMFA             tokenSummary         `json:"refresh_with_mfa"`
	LoginWithoutMFA            expectedErrorSummary `json:"login_without_mfa"`
	MFALogin                   tokenSummary         `json:"mfa_login"`
	RegenerateMFARecoveryCodes mfaRegenerateSummary `json:"regenerate_mfa_recovery_codes"`
	RevokeMFARecoveryCodes     mfaRevokeSummary     `json:"revoke_mfa_recovery_codes"`
	DisableMFAFactor           mfaDisableSummary    `json:"disable_mfa_factor"`
	ChallengeDeliveryOutbox    outboxStats          `json:"challenge_delivery_outbox"`
	ChallengeDeliveryOutboxRow deliveryOutboxRow    `json:"challenge_delivery_outbox_row"`
	ChallengeRow               challengeRow         `json:"challenge_row"`
	LatenciesMS                map[string]float64   `json:"latencies_ms"`
	Capacity                   *capacitySummary     `json:"capacity_summary,omitempty"`
}

type identityChallengeClient interface {
	RegisterUser(context.Context, *identityv1.RegisterUserRequest, ...grpc.CallOption) (*identityv1.RegisterUserResponse, error)
	Login(context.Context, *identityv1.LoginRequest, ...grpc.CallOption) (*identityv1.LoginResponse, error)
	RefreshGatewayToken(context.Context, *identityv1.RefreshGatewayTokenRequest, ...grpc.CallOption) (*identityv1.RefreshGatewayTokenResponse, error)
	RequestVerificationChallenge(context.Context, *identityv1.RequestVerificationChallengeRequest, ...grpc.CallOption) (*identityv1.RequestVerificationChallengeResponse, error)
	ConfirmVerificationChallenge(context.Context, *identityv1.ConfirmVerificationChallengeRequest, ...grpc.CallOption) (*identityv1.ConfirmVerificationChallengeResponse, error)
	RequestPasswordReset(context.Context, *identityv1.RequestPasswordResetRequest, ...grpc.CallOption) (*identityv1.RequestPasswordResetResponse, error)
	ConfirmPasswordReset(context.Context, *identityv1.ConfirmPasswordResetRequest, ...grpc.CallOption) (*identityv1.ConfirmPasswordResetResponse, error)
	BeginMFAEnrollment(context.Context, *identityv1.BeginMFAEnrollmentRequest, ...grpc.CallOption) (*identityv1.BeginMFAEnrollmentResponse, error)
	ConfirmMFAEnrollment(context.Context, *identityv1.ConfirmMFAEnrollmentRequest, ...grpc.CallOption) (*identityv1.ConfirmMFAEnrollmentResponse, error)
	DisableMFAFactor(context.Context, *identityv1.DisableMFAFactorRequest, ...grpc.CallOption) (*identityv1.DisableMFAFactorResponse, error)
	RegenerateMFARecoveryCodes(context.Context, *identityv1.RegenerateMFARecoveryCodesRequest, ...grpc.CallOption) (*identityv1.RegenerateMFARecoveryCodesResponse, error)
	RevokeMFARecoveryCodes(context.Context, *identityv1.RevokeMFARecoveryCodesRequest, ...grpc.CallOption) (*identityv1.RevokeMFARecoveryCodesResponse, error)
}

type registerSummary struct {
	Status          string `json:"status"`
	CreatedAtUnixMS int64  `json:"created_at_unix_ms"`
}

type challengeSummary struct {
	ChallengeID          string `json:"challenge_id"`
	Channel              string `json:"channel"`
	Destination          string `json:"destination"`
	ExpiresAtUnixMS      int64  `json:"expires_at_unix_ms"`
	DevChallengeTokenSet bool   `json:"dev_challenge_token_set"`
}

type webhookSummary struct {
	Received        bool   `json:"received"`
	ChallengeID     string `json:"challenge_id"`
	ChallengeType   string `json:"challenge_type"`
	Channel         string `json:"channel"`
	Destination     string `json:"destination"`
	TokenSet        bool   `json:"token_set"`
	RequestID       string `json:"request_id,omitempty"`
	AuthorizationOK bool   `json:"authorization_ok"`
}

type confirmSummary struct {
	Channel          string `json:"channel"`
	Destination      string `json:"destination"`
	VerifiedAtUnixMS int64  `json:"verified_at_unix_ms"`
}

type tokenSummary struct {
	Audience               string `json:"audience"`
	TokenType              string `json:"token_type"`
	SessionIDSet           bool   `json:"session_id_set"`
	GatewayTokenSet        bool   `json:"gateway_token_set"`
	RefreshTokenSet        bool   `json:"refresh_token_set"`
	RefreshTokenRotated    bool   `json:"refresh_token_rotated,omitempty"`
	GatewayExpiresAtUnixMS int64  `json:"gateway_expires_at_unix_ms"`
	RefreshExpiresAtUnixMS int64  `json:"refresh_expires_at_unix_ms"`
	IssuedAtUnixMS         int64  `json:"issued_at_unix_ms"`
}

type passwordResetSummary struct {
	ResetAtUnixMS int64 `json:"reset_at_unix_ms"`
}

type mfaBeginSummary struct {
	FactorIDSet     bool   `json:"factor_id_set"`
	FactorType      string `json:"factor_type"`
	Status          string `json:"status"`
	SecretSet       bool   `json:"secret_set"`
	OTPAuthURISet   bool   `json:"otpauth_uri_set"`
	CreatedAtUnixMS int64  `json:"created_at_unix_ms"`
}

type mfaConfirmSummary struct {
	FactorIDSet       bool   `json:"factor_id_set"`
	Status            string `json:"status"`
	VerifiedAtUnixMS  int64  `json:"verified_at_unix_ms"`
	RecoveryCodeCount int    `json:"recovery_code_count"`
}

type mfaRegenerateSummary struct {
	FactorIDSet       bool  `json:"factor_id_set"`
	RecoveryCodeCount int   `json:"recovery_code_count"`
	GeneratedAtUnixMS int64 `json:"generated_at_unix_ms"`
}

type mfaRevokeSummary struct {
	RevokedCount    int32 `json:"revoked_count"`
	RevokedAtUnixMS int64 `json:"revoked_at_unix_ms"`
}

type mfaDisableSummary struct {
	FactorIDSet      bool   `json:"factor_id_set"`
	Status           string `json:"status"`
	DisabledAtUnixMS int64  `json:"disabled_at_unix_ms"`
}

type expectedErrorSummary struct {
	Occurred bool   `json:"occurred"`
	Code     string `json:"code,omitempty"`
}

type outboxStats struct {
	Total     int64 `json:"total"`
	Pending   int64 `json:"pending"`
	Delivered int64 `json:"delivered"`
	DLQ       int64 `json:"dlq"`
	Canceled  int64 `json:"canceled"`
}

type deliveryOutboxRow struct {
	Status     string `json:"status"`
	RetryCount int    `json:"retry_count"`
	LastError  string `json:"last_error,omitempty"`
	Delivered  bool   `json:"delivered"`
	DLQ        bool   `json:"dlq"`
}

type challengeRow struct {
	Status               string `json:"status"`
	DeliveryStatus       string `json:"delivery_status"`
	DeliveryAttemptCount int    `json:"delivery_attempt_count"`
	DeliveryLastError    string `json:"delivery_last_error,omitempty"`
}

type capacitySummary struct {
	DurationSeconds                  float64 `json:"duration_seconds"`
	OperationCount                   int     `json:"operation_count"`
	TokenIssueCount                  int     `json:"token_issue_count"`
	ExpectedErrorCount               int     `json:"expected_error_count"`
	ChallengeDeliveryOutboxTotal     int64   `json:"challenge_delivery_outbox_total"`
	ChallengeDeliveryOutboxPending   int64   `json:"challenge_delivery_outbox_pending"`
	ChallengeDeliveryOutboxDelivered int64   `json:"challenge_delivery_outbox_delivered"`
	ChallengeDeliveryOutboxDLQ       int64   `json:"challenge_delivery_outbox_dlq"`
	ChallengeDeliveryAttemptCount    int     `json:"challenge_delivery_attempt_count"`
	OperationsPerSecond              float64 `json:"operations_per_second"`
	LatencyP95MS                     float64 `json:"latency_p95_ms,omitempty"`
	LatencyP99MS                     float64 `json:"latency_p99_ms,omitempty"`
	MFARecoveryCodeCount             int     `json:"mfa_recovery_code_count,omitempty"`
}

type challengeNotification struct {
	TenantID        string `json:"tenant_id"`
	UserID          string `json:"user_id"`
	ChallengeID     string `json:"challenge_id"`
	ChallengeType   string `json:"challenge_type"`
	Channel         string `json:"channel"`
	Destination     string `json:"destination"`
	Token           string `json:"token"`
	ExpiresAtUnixMS int64  `json:"expires_at_unix_ms"`
	TraceID         string `json:"trace_id,omitempty"`
	RequestID       string `json:"request_id,omitempty"`
	Authorization   string `json:"authorization,omitempty"`
}

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

func parseConfig() config {
	var cfg config
	flag.StringVar(&cfg.mode, "mode", "client", "mode: client or webhook")
	flag.StringVar(&cfg.target, "target", "127.0.0.1:10600", "identity-service or api-gateway gRPC target")
	flag.BoolVar(&cfg.gatewayFacade, "gateway-facade", false, "call identity RPCs through nexusim.gateway.v1.GatewayService facade")
	flag.StringVar(&cfg.tls.CAFile, "identity-tls-ca-file", "", "CA PEM for target gRPC TLS")
	flag.StringVar(&cfg.tls.ServerName, "identity-tls-server-name", "", "server name for target gRPC TLS")
	flag.StringVar(&cfg.tls.ClientCertFile, "identity-tls-client-cert-file", "", "client certificate PEM for target mTLS")
	flag.StringVar(&cfg.tls.ClientKeyFile, "identity-tls-client-key-file", "", "client private key PEM for target mTLS")
	flag.StringVar(&cfg.resultDir, "result-dir", "H:\\NexusIM\\loadtest-results\\identity-challenge-delivery-outbox-smoke", "result directory")
	flag.StringVar(&cfg.pgDSN, "pg-dsn", "", "PostgreSQL DSN")
	flag.StringVar(&cfg.webhookListen, "webhook-listen", "127.0.0.1:0", "webhook listen address for webhook mode")
	flag.StringVar(&cfg.webhookFile, "webhook-file", "", "path where webhook mode writes the last received challenge")
	flag.StringVar(&cfg.webhookBearerToken, "webhook-bearer-token", "", "expected webhook bearer token")
	flag.DurationVar(&cfg.requestTimeout, "request-timeout", 5*time.Second, "per-request timeout")
	flag.DurationVar(&cfg.waitTimeout, "wait-timeout", 20*time.Second, "wait timeout")
	flag.DurationVar(&cfg.pollInterval, "poll-interval", 200*time.Millisecond, "poll interval")
	flag.StringVar(&cfg.tenantID, "tenant-id", "tenant-identity-smoke", "tenant id")
	flag.StringVar(&cfg.userID, "user-id", "identity-user", "user id")
	flag.StringVar(&cfg.deviceID, "device-id", "identity-device", "device id for Login and RefreshGatewayToken")
	flag.StringVar(&cfg.audience, "audience", "api-gateway", "gateway token audience for Login and RefreshGatewayToken")
	flag.StringVar(&cfg.password, "password", "IdentitySmokePassw0rd!", "user password")
	flag.StringVar(&cfg.newPassword, "new-password", "IdentitySmokeResetPassw0rd!", "new password used by ConfirmPasswordReset")
	flag.StringVar(&cfg.destination, "destination", "identity-user@example.com", "verification destination")
	flag.BoolVar(&cfg.cleanup, "cleanup", false, "delete identity rows for this tenant before running")
	flag.Parse()
	cfg.mode = strings.ToLower(strings.TrimSpace(cfg.mode))
	return cfg
}

func runWebhook(cfg config) error {
	if strings.TrimSpace(cfg.webhookFile) == "" {
		return errors.New("--webhook-file is required in webhook mode")
	}
	if err := os.MkdirAll(filepath.Dir(cfg.webhookFile), 0o755); err != nil {
		return fmt.Errorf("create webhook file directory: %w", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/challenge", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if cfg.webhookBearerToken != "" && r.Header.Get("Authorization") != "Bearer "+cfg.webhookBearerToken {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		defer r.Body.Close()
		var notification challengeNotification
		if err := json.NewDecoder(r.Body).Decode(&notification); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		notification.Authorization = r.Header.Get("Authorization")
		bytes, err := json.MarshalIndent(notification, "", "  ")
		if err != nil {
			http.Error(w, "encode failed", http.StatusInternalServerError)
			return
		}
		if err := os.WriteFile(cfg.webhookFile, append(bytes, '\n'), 0o644); err != nil {
			http.Error(w, "write failed", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	listener, err := net.Listen("tcp", cfg.webhookListen)
	if err != nil {
		return fmt.Errorf("listen webhook: %w", err)
	}
	fmt.Printf("webhook_listen=%s\n", listener.Addr().String())
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve webhook: %w", err)
	}
	return nil
}

func runClient(cfg config) error {
	started := time.Now().UTC()
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
		LatenciesMS:   map[string]float64{},
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

func waitWebhookNotification(cfg config, challengeID string) (challengeNotification, error) {
	if strings.TrimSpace(cfg.webhookFile) == "" {
		return challengeNotification{}, errors.New("--webhook-file is required in client mode")
	}
	deadline := time.Now().Add(cfg.waitTimeout)
	var lastErr error
	for time.Now().Before(deadline) {
		bytes, err := os.ReadFile(cfg.webhookFile)
		if err == nil && len(bytes) > 0 {
			var notification challengeNotification
			if err := json.Unmarshal(bytes, &notification); err != nil {
				lastErr = err
			} else if notification.ChallengeID == challengeID {
				return notification, nil
			}
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			lastErr = err
		}
		time.Sleep(cfg.pollInterval)
	}
	if lastErr != nil {
		return challengeNotification{}, fmt.Errorf("wait webhook notification: %w", lastErr)
	}
	return challengeNotification{}, fmt.Errorf("timed out waiting for webhook notification for challenge %s", challengeID)
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

func finish(cfg config, result *summary, runErr error) error {
	result.FinishedAt = time.Now().UTC()
	result.Success = runErr == nil
	if runErr != nil {
		result.Error = runErr.Error()
	}
	result.Capacity = buildCapacitySummary(*result)
	if err := os.MkdirAll(cfg.resultDir, 0o755); err != nil {
		return fmt.Errorf("create result dir: %w", err)
	}
	bytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("encode summary: %w", err)
	}
	path := filepath.Join(cfg.resultDir, "identity-summary.json")
	if err := os.WriteFile(path, append(bytes, '\n'), 0o644); err != nil {
		return fmt.Errorf("write summary: %w", err)
	}
	fmt.Printf("summary: %s\n", path)
	if runErr != nil {
		return runErr
	}
	return nil
}

func elapsedMS(start time.Time) float64 {
	return float64(time.Since(start).Microseconds()) / 1000.0
}

func buildCapacitySummary(s summary) *capacitySummary {
	duration := s.FinishedAt.Sub(s.StartedAt).Seconds()
	if duration <= 0 {
		return nil
	}
	operationCount := len(s.LatenciesMS)
	return &capacitySummary{
		DurationSeconds:                  duration,
		OperationCount:                   operationCount,
		TokenIssueCount:                  tokenIssueCount(s),
		ExpectedErrorCount:               expectedErrorCount(s),
		ChallengeDeliveryOutboxTotal:     s.ChallengeDeliveryOutbox.Total,
		ChallengeDeliveryOutboxPending:   s.ChallengeDeliveryOutbox.Pending,
		ChallengeDeliveryOutboxDelivered: s.ChallengeDeliveryOutbox.Delivered,
		ChallengeDeliveryOutboxDLQ:       s.ChallengeDeliveryOutbox.DLQ,
		ChallengeDeliveryAttemptCount:    s.ChallengeRow.DeliveryAttemptCount,
		OperationsPerSecond:              ratePerSecond(operationCount, duration),
		LatencyP95MS:                     latencyQuantile(s.LatenciesMS, 0.95),
		LatencyP99MS:                     latencyQuantile(s.LatenciesMS, 0.99),
		MFARecoveryCodeCount:             recoveryCodeCount(s),
	}
}

func tokenIssueCount(s summary) int {
	count := 0
	if s.Login.GatewayTokenSet {
		count++
	}
	if s.Refresh.GatewayTokenSet {
		count++
	}
	if s.PostResetLogin.GatewayTokenSet {
		count++
	}
	if s.RefreshWithMFA.GatewayTokenSet {
		count++
	}
	if s.MFALogin.GatewayTokenSet {
		count++
	}
	return count
}

func expectedErrorCount(s summary) int {
	count := 0
	if s.RefreshWithoutMFA.Occurred {
		count++
	}
	if s.LoginWithoutMFA.Occurred {
		count++
	}
	return count
}

func recoveryCodeCount(s summary) int {
	return s.ConfirmMFAEnrollment.RecoveryCodeCount + s.RegenerateMFARecoveryCodes.RecoveryCodeCount
}

func ratePerSecond(count int, durationSeconds float64) float64 {
	if count <= 0 || durationSeconds <= 0 {
		return 0
	}
	return float64(count) / durationSeconds
}

func latencyQuantile(values map[string]float64, quantile float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := make([]float64, 0, len(values))
	for _, value := range values {
		sorted = append(sorted, value)
	}
	sort.Float64s(sorted)
	index := int(math.Ceil(quantile*float64(len(sorted)))) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

func generateTOTPCode(secret string, now time.Time) string {
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(strings.TrimSpace(secret)))
	if err != nil {
		return ""
	}
	counter := uint64(now.Unix() / 30)
	var counterBytes [8]byte
	binary.BigEndian.PutUint64(counterBytes[:], counter)
	mac := hmac.New(sha1.New, key)
	_, _ = mac.Write(counterBytes[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	value := (int(sum[offset])&0x7f)<<24 |
		(int(sum[offset+1])&0xff)<<16 |
		(int(sum[offset+2])&0xff)<<8 |
		(int(sum[offset+3]) & 0xff)
	return fmt.Sprintf("%06d", value%1_000_000)
}

func gitOutput(args ...string) string {
	out, err := exec.Command("git", args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func gitDirty() bool {
	return strings.TrimSpace(gitOutput("status", "--short")) != ""
}
