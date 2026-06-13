package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	gatewayv1 "github.com/qsyy0921/IM/api/proto/nexusim/gateway/v1"
	identityv1 "github.com/qsyy0921/IM/api/proto/nexusim/identity/v1"
	"github.com/qsyy0921/IM/loadtest/internal/grpctls"
	"google.golang.org/grpc"
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
	ChallengeDeliveryOutbox    outboxStats          `json:"challenge_delivery_outbox"`
	ChallengeDeliveryOutboxRow deliveryOutboxRow    `json:"challenge_delivery_outbox_row"`
	ChallengeRow               challengeRow         `json:"challenge_row"`
	LatenciesMS                map[string]float64   `json:"latencies_ms"`
}

type identityChallengeClient interface {
	RegisterUser(context.Context, *identityv1.RegisterUserRequest, ...grpc.CallOption) (*identityv1.RegisterUserResponse, error)
	Login(context.Context, *identityv1.LoginRequest, ...grpc.CallOption) (*identityv1.LoginResponse, error)
	RefreshGatewayToken(context.Context, *identityv1.RefreshGatewayTokenRequest, ...grpc.CallOption) (*identityv1.RefreshGatewayTokenResponse, error)
	RequestVerificationChallenge(context.Context, *identityv1.RequestVerificationChallengeRequest, ...grpc.CallOption) (*identityv1.RequestVerificationChallengeResponse, error)
	ConfirmVerificationChallenge(context.Context, *identityv1.ConfirmVerificationChallengeRequest, ...grpc.CallOption) (*identityv1.ConfirmVerificationChallengeResponse, error)
	RequestPasswordReset(context.Context, *identityv1.RequestPasswordResetRequest, ...grpc.CallOption) (*identityv1.RequestPasswordResetResponse, error)
	ConfirmPasswordReset(context.Context, *identityv1.ConfirmPasswordResetRequest, ...grpc.CallOption) (*identityv1.ConfirmPasswordResetResponse, error)
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
