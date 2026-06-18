package main

import (
	"context"
	"time"

	identityv1 "github.com/qsyy0921/IM/api/proto/nexusim/identity/v1"
	"google.golang.org/grpc"
)

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
	CapacityMode               bool                 `json:"capacity_mode,omitempty"`
	VUs                        int                  `json:"vus,omitempty"`
	ConfiguredDurationSeconds  float64              `json:"configured_duration_seconds,omitempty"`
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
	capacityOperationCount     int
	capacityTokenIssueCount    int
	capacityExpectedErrorCount int
	capacityLatencySamples     []float64
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
	CapacityMode                     bool    `json:"capacity_mode,omitempty"`
	VUs                              int     `json:"vus,omitempty"`
	ConfiguredDurationSeconds        float64 `json:"configured_duration_seconds,omitempty"`
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
