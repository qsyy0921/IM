package postgres

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/identity-service/internal/types"
)

type Repository struct {
	pool                         *pgxpool.Pool
	sessionID                    func() (string, error)
	eventID                      func() (string, error)
	factorID                     func() (string, error)
	challengeRequestMaxPerWindow int
	challengeRequestWindow       time.Duration
	challengeRequestLockDuration time.Duration
}

type SessionMFAProofAuditStats struct {
	InvalidTotal         int64
	UnknownMethod        int64
	EmptyMethodWithProof int64
	TOTPMissingProof     int64
	RecoveryInvalidProof int64
}

const maxActiveChallengesPerTarget = 3
const maxEnabledMFAFactorsPerUser = 5
const DefaultChallengeRequestMaxPerWindow = 5
const DefaultChallengeRequestWindow = 15 * time.Minute
const DefaultChallengeRequestLockDuration = 15 * time.Minute

type RepositoryOption func(*Repository)

func NewRepository(pool *pgxpool.Pool, opts ...RepositoryOption) *Repository {
	repository := &Repository{
		pool: pool,
		sessionID: func() (string, error) {
			return newID("sess")
		},
		eventID: func() (string, error) {
			return newID("evt")
		},
		factorID: func() (string, error) {
			return newID("mfa")
		},
		challengeRequestMaxPerWindow: DefaultChallengeRequestMaxPerWindow,
		challengeRequestWindow:       DefaultChallengeRequestWindow,
		challengeRequestLockDuration: DefaultChallengeRequestLockDuration,
	}
	for _, opt := range opts {
		opt(repository)
	}
	return repository
}

func WithEventIDGenerator(generator func() (string, error)) RepositoryOption {
	return func(repository *Repository) {
		if generator != nil {
			repository.eventID = generator
		}
	}
}

func WithSessionIDGenerator(generator func() (string, error)) RepositoryOption {
	return func(repository *Repository) {
		if generator != nil {
			repository.sessionID = generator
		}
	}
}

func WithMFAFactorIDGenerator(generator func() (string, error)) RepositoryOption {
	return func(repository *Repository) {
		if generator != nil {
			repository.factorID = generator
		}
	}
}

func WithChallengeRequestLimit(maxPerWindow int, window time.Duration) RepositoryOption {
	return func(repository *Repository) {
		repository.challengeRequestMaxPerWindow = maxPerWindow
		repository.challengeRequestWindow = window
	}
}

func WithChallengeRequestLockDuration(duration time.Duration) RepositoryOption {
	return func(repository *Repository) {
		repository.challengeRequestLockDuration = duration
	}
}

func (r *Repository) RegisterUser(ctx context.Context, command types.RegisterUserCommand, passwordHash string, createdAt time.Time) (types.RegisterUserResult, error) {
	if r.pool == nil {
		return types.RegisterUserResult{}, types.NewDBWriteFailed("identity repository is not configured")
	}
	var row struct {
		TenantID  types.TenantID
		UserID    types.UserID
		Status    types.UserStatus
		CreatedAt time.Time
	}
	err := r.pool.QueryRow(ctx, `
INSERT INTO identity_users (
    tenant_id,
    user_id,
    status,
    password_hash,
    password_updated_at,
    created_at,
    updated_at
) VALUES ($1, $2, 'ACTIVE', $3, $4, $4, $4)
RETURNING tenant_id, user_id, status, created_at
`, command.TenantID, command.UserID, passwordHash, createdAt).Scan(
		&row.TenantID,
		&row.UserID,
		&row.Status,
		&row.CreatedAt,
	)
	if isUniqueViolation(err) {
		return types.RegisterUserResult{}, types.NewUserAlreadyExists("user already exists")
	}
	if err != nil {
		return types.RegisterUserResult{}, types.NewDBWriteFailed(err.Error())
	}
	return types.RegisterUserResult{
		TenantID:        row.TenantID,
		UserID:          row.UserID,
		Status:          row.Status,
		CreatedAtUnixMS: row.CreatedAt.UnixMilli(),
	}, nil
}

func (r *Repository) AuditSessionMFAProofConstraints(ctx context.Context) (SessionMFAProofAuditStats, error) {
	if r.pool == nil {
		return SessionMFAProofAuditStats{}, types.NewDBReadFailed("identity repository is not configured")
	}
	var stats SessionMFAProofAuditStats
	err := r.pool.QueryRow(ctx, `
SELECT
    COUNT(*) FILTER (
        WHERE mfa_method NOT IN ('', 'TOTP', 'RECOVERY_CODE')
    ) AS unknown_method,
    COUNT(*) FILTER (
        WHERE mfa_method = ''
          AND (mfa_verified_at IS NOT NULL OR mfa_factor_id <> '')
    ) AS empty_method_with_proof,
    COUNT(*) FILTER (
        WHERE mfa_method = 'TOTP'
          AND (mfa_verified_at IS NULL OR mfa_factor_id = '')
    ) AS totp_missing_proof,
    COUNT(*) FILTER (
        WHERE mfa_method = 'RECOVERY_CODE'
          AND (mfa_verified_at IS NULL OR mfa_factor_id <> '')
    ) AS recovery_invalid_proof
FROM identity_sessions
`).Scan(
		&stats.UnknownMethod,
		&stats.EmptyMethodWithProof,
		&stats.TOTPMissingProof,
		&stats.RecoveryInvalidProof,
	)
	if err != nil {
		return SessionMFAProofAuditStats{}, types.NewDBReadFailed(err.Error())
	}
	stats.InvalidTotal = stats.UnknownMethod + stats.EmptyMethodWithProof + stats.TOTPMissingProof + stats.RecoveryInvalidProof
	return stats, nil
}

func (r *Repository) GetUserCredential(ctx context.Context, tenantID types.TenantID, userID types.UserID) (types.UserCredential, error) {
	if r.pool == nil {
		return types.UserCredential{}, types.NewDBReadFailed("identity repository is not configured")
	}
	var credential types.UserCredential
	err := r.pool.QueryRow(ctx, `
SELECT
    tenant_id,
    user_id,
    status,
    password_hash,
    failed_login_count,
    COALESCE(locked_until, 'epoch'::timestamptz),
    mfa_recovery_failed_count,
    COALESCE(mfa_recovery_locked_until, 'epoch'::timestamptz)
FROM identity_users
WHERE tenant_id = $1
  AND user_id = $2
`, tenantID, userID).Scan(
		&credential.TenantID,
		&credential.UserID,
		&credential.Status,
		&credential.PasswordHash,
		&credential.FailedLoginCount,
		&credential.LockedUntil,
		&credential.MFARecoveryFailedCount,
		&credential.MFARecoveryLockedUntil,
	)
	if err == pgx.ErrNoRows {
		return types.UserCredential{}, types.NewInvalidCredentials("invalid credentials")
	}
	if err != nil {
		return types.UserCredential{}, types.NewDBReadFailed(err.Error())
	}
	if credential.PasswordHash == "" {
		return types.UserCredential{}, types.NewInvalidCredentials("invalid credentials")
	}
	return credential, nil
}

func (r *Repository) RecordLoginFailure(
	ctx context.Context,
	tenantID types.TenantID,
	userID types.UserID,
	failedAt time.Time,
	lockUntil time.Time,
	maxFailedAttempts int,
	failureWindowStart time.Time,
) error {
	if r.pool == nil {
		return types.NewDBWriteFailed("identity repository is not configured")
	}
	if maxFailedAttempts <= 0 {
		return types.NewInvalidArgument("max failed attempts must be positive")
	}
	var failedCount int
	var lockedUntil time.Time
	err := r.pool.QueryRow(ctx, `
WITH next_failure AS (
    SELECT
        tenant_id,
        user_id,
        CASE
            WHEN failed_login_last_at IS NULL
              OR failed_login_last_at < $6
              OR (failed_login_count >= $5 AND COALESCE(locked_until, 'epoch'::timestamptz) <= $3)
                THEN 1
            ELSE failed_login_count + 1
        END AS next_failed_login_count
    FROM identity_users
    WHERE tenant_id = $1
      AND user_id = $2
    FOR UPDATE
)
UPDATE identity_users
SET failed_login_count = next_failure.next_failed_login_count,
    failed_login_last_at = $3,
    locked_until = CASE
        WHEN next_failure.next_failed_login_count >= $5 THEN $4::timestamptz
        ELSE NULL
    END,
    updated_at = $3
FROM next_failure
WHERE identity_users.tenant_id = next_failure.tenant_id
  AND identity_users.user_id = next_failure.user_id
RETURNING failed_login_count, COALESCE(locked_until, 'epoch'::timestamptz)
`, tenantID, userID, failedAt, lockUntil, maxFailedAttempts, failureWindowStart).Scan(&failedCount, &lockedUntil)
	if err == pgx.ErrNoRows {
		return types.NewInvalidCredentials("invalid credentials")
	}
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	if lockedUntil.After(failedAt) {
		return types.NewAccountLocked("account temporarily locked")
	}
	return nil
}

func (r *Repository) RecordMFARecoveryLoginFailure(
	ctx context.Context,
	tenantID types.TenantID,
	userID types.UserID,
	failedAt time.Time,
	lockUntil time.Time,
	maxFailedAttempts int,
	failureWindowStart time.Time,
) error {
	if r.pool == nil {
		return types.NewDBWriteFailed("identity repository is not configured")
	}
	if maxFailedAttempts <= 0 {
		return types.NewInvalidArgument("max failed attempts must be positive")
	}
	var failedCount int
	var lockedUntil time.Time
	err := r.pool.QueryRow(ctx, `
WITH next_failure AS (
    SELECT
        tenant_id,
        user_id,
        CASE
            WHEN mfa_recovery_failed_last_at IS NULL
              OR mfa_recovery_failed_last_at < $6
              OR (mfa_recovery_failed_count >= $5 AND COALESCE(mfa_recovery_locked_until, 'epoch'::timestamptz) <= $3)
                THEN 1
            ELSE mfa_recovery_failed_count + 1
        END AS next_failed_login_count
    FROM identity_users
    WHERE tenant_id = $1
      AND user_id = $2
    FOR UPDATE
)
UPDATE identity_users
SET mfa_recovery_failed_count = next_failure.next_failed_login_count,
    mfa_recovery_failed_last_at = $3,
    mfa_recovery_locked_until = CASE
        WHEN next_failure.next_failed_login_count >= $5 THEN $4::timestamptz
        ELSE NULL
    END,
    updated_at = $3
FROM next_failure
WHERE identity_users.tenant_id = next_failure.tenant_id
  AND identity_users.user_id = next_failure.user_id
RETURNING mfa_recovery_failed_count, COALESCE(mfa_recovery_locked_until, 'epoch'::timestamptz)
`, tenantID, userID, failedAt, lockUntil, maxFailedAttempts, failureWindowStart).Scan(&failedCount, &lockedUntil)
	if err == pgx.ErrNoRows {
		return types.NewInvalidCredentials("invalid credentials")
	}
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	if lockedUntil.After(failedAt) {
		return types.NewMFALocked("mfa temporarily locked")
	}
	return nil
}

func (r *Repository) LoginGatewaySession(
	ctx context.Context,
	command types.LoginCommand,
	refreshToken types.RefreshTokenRecord,
	issuedAt time.Time,
	gatewayExpiresAt time.Time,
	refreshExpiresAt time.Time,
) (types.LoginResult, error) {
	if r.pool == nil {
		return types.LoginResult{}, types.NewDBWriteFailed("identity repository is not configured")
	}
	sessionID, err := r.newSessionID("")
	if err != nil {
		return types.LoginResult{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return types.LoginResult{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := lockDevice(ctx, tx, command.TenantID, command.UserID, command.DeviceID); err != nil {
		return types.LoginResult{}, err
	}
	if err := ensureUserCanLogin(ctx, tx, command.TenantID, command.UserID, issuedAt); err != nil {
		return types.LoginResult{}, err
	}
	device, err := ensureActiveDevice(ctx, tx, command.TenantID, command.UserID, command.DeviceID)
	if err != nil {
		return types.LoginResult{}, err
	}
	if device.Status == types.DeviceStatusRevoked {
		return types.LoginResult{}, types.NewDeviceRevoked("device is revoked")
	}
	if err := ensureSessionCanIssue(ctx, tx, command.TenantID, command.UserID, command.DeviceID, sessionID); err != nil {
		return types.LoginResult{}, err
	}
	if command.UsedMFARecoveryCode.CodeID != "" || command.UsedMFARecoveryCode.CodeHash != "" {
		if err := ensureMFARecoveryLoginNotLocked(ctx, tx, command.TenantID, command.UserID, issuedAt); err != nil {
			return types.LoginResult{}, err
		}
	}
	if command.VerifiedMFAFactorID != "" {
		if err := ensureMFALoginNotLocked(ctx, tx, command.TenantID, command.UserID, command.VerifiedMFAFactorID, issuedAt); err != nil {
			return types.LoginResult{}, err
		}
	}
	if err := upsertSession(ctx, tx, issueCommandFromLogin(command, sessionID), sessionID, issuedAt, gatewayExpiresAt, sessionMFAProofFromLogin(command, issuedAt)); err != nil {
		return types.LoginResult{}, err
	}
	if err := insertRefreshToken(ctx, tx, command.TenantID, command.UserID, command.DeviceID, sessionID, refreshToken, issuedAt, refreshExpiresAt, command.TraceID, command.RequestID); err != nil {
		return types.LoginResult{}, err
	}
	if command.UsedMFARecoveryCode.CodeID != "" || command.UsedMFARecoveryCode.CodeHash != "" {
		if err := consumeMFARecoveryCode(ctx, tx, command.TenantID, command.UserID, command.UsedMFARecoveryCode, issuedAt); err != nil {
			return types.LoginResult{}, err
		}
		if err := clearMFARecoveryLoginFailures(ctx, tx, command.TenantID, command.UserID, issuedAt); err != nil {
			return types.LoginResult{}, err
		}
	}
	if err := clearLoginFailures(ctx, tx, command.TenantID, command.UserID, issuedAt); err != nil {
		return types.LoginResult{}, err
	}
	if command.VerifiedMFAFactorID != "" {
		if err := clearMFALoginFailures(ctx, tx, command.TenantID, command.UserID, command.VerifiedMFAFactorID, issuedAt); err != nil {
			return types.LoginResult{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return types.LoginResult{}, types.NewDBWriteFailed(err.Error())
	}
	return types.LoginResult{
		TenantID:               command.TenantID,
		UserID:                 command.UserID,
		DeviceID:               command.DeviceID,
		SessionID:              sessionID,
		Audience:               command.Audience,
		IssuedAtUnixMS:         issuedAt.UnixMilli(),
		GatewayExpiresAtUnixMS: gatewayExpiresAt.UnixMilli(),
		RefreshExpiresAtUnixMS: refreshExpiresAt.UnixMilli(),
	}, nil
}

func (r *Repository) RefreshGatewaySession(
	ctx context.Context,
	command types.RefreshGatewayTokenCommand,
	presentedTokenID types.RefreshTokenID,
	presentedTokenHash string,
	nextRefreshToken types.RefreshTokenRecord,
	issuedAt time.Time,
	gatewayExpiresAt time.Time,
	refreshExpiresAt time.Time,
) (types.RefreshGatewayTokenResult, error) {
	if r.pool == nil {
		return types.RefreshGatewayTokenResult{}, types.NewDBWriteFailed("identity repository is not configured")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return types.RefreshGatewayTokenResult{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() { _ = tx.Rollback(ctx) }()

	row, err := lockRefreshToken(ctx, tx, command, presentedTokenID)
	if err != nil {
		return types.RefreshGatewayTokenResult{}, err
	}
	if row.TokenHash != presentedTokenHash {
		return types.RefreshGatewayTokenResult{}, types.NewInvalidRefreshToken("invalid refresh token")
	}
	if row.Status != "ACTIVE" {
		if err := r.handleRefreshTokenReuse(ctx, tx, row, command.TraceID, command.RequestID, issuedAt); err != nil {
			return types.RefreshGatewayTokenResult{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return types.RefreshGatewayTokenResult{}, types.NewDBWriteFailed(err.Error())
		}
		return types.RefreshGatewayTokenResult{}, types.NewRefreshTokenReuseDetected("refresh token was already used")
	}
	if !issuedAt.Before(row.ExpiresAt) {
		if err := revokeRefreshToken(ctx, tx, row, issuedAt); err != nil {
			return types.RefreshGatewayTokenResult{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return types.RefreshGatewayTokenResult{}, types.NewDBWriteFailed(err.Error())
		}
		return types.RefreshGatewayTokenResult{}, types.NewInvalidRefreshToken("refresh token expired")
	}
	if err := lockDevice(ctx, tx, command.TenantID, command.UserID, command.DeviceID); err != nil {
		return types.RefreshGatewayTokenResult{}, err
	}
	device, err := getDeviceForUpdate(ctx, tx, command.TenantID, command.UserID, command.DeviceID)
	if err != nil {
		return types.RefreshGatewayTokenResult{}, err
	}
	if device.Status == types.DeviceStatusRevoked {
		return types.RefreshGatewayTokenResult{}, types.NewDeviceRevoked("device is revoked")
	}
	proof, err := lockRefreshSession(ctx, tx, command.TenantID, command.UserID, command.DeviceID, row.SessionID)
	if err != nil {
		return types.RefreshGatewayTokenResult{}, err
	}
	submittedProof := sessionMFAProofFromRefresh(command, issuedAt)
	proof = mergeRefreshMFAProof(proof, submittedProof)
	requiresMFA, err := hasActiveTOTPFactor(ctx, tx, command.TenantID, command.UserID)
	if err != nil {
		return types.RefreshGatewayTokenResult{}, err
	}
	if requiresMFA && !proof.Verified {
		return types.RefreshGatewayTokenResult{}, types.NewMFARequired("mfa required")
	}
	if command.UsedMFARecoveryCode.CodeID != "" || command.UsedMFARecoveryCode.CodeHash != "" {
		if err := ensureMFARecoveryLoginNotLocked(ctx, tx, command.TenantID, command.UserID, issuedAt); err != nil {
			return types.RefreshGatewayTokenResult{}, err
		}
		if err := consumeMFARecoveryCode(ctx, tx, command.TenantID, command.UserID, command.UsedMFARecoveryCode, issuedAt); err != nil {
			return types.RefreshGatewayTokenResult{}, err
		}
		if err := clearMFARecoveryLoginFailures(ctx, tx, command.TenantID, command.UserID, issuedAt); err != nil {
			return types.RefreshGatewayTokenResult{}, err
		}
	}
	if command.VerifiedMFAFactorID != "" {
		if err := ensureMFALoginNotLocked(ctx, tx, command.TenantID, command.UserID, command.VerifiedMFAFactorID, issuedAt); err != nil {
			return types.RefreshGatewayTokenResult{}, err
		}
		if err := clearMFALoginFailures(ctx, tx, command.TenantID, command.UserID, command.VerifiedMFAFactorID, issuedAt); err != nil {
			return types.RefreshGatewayTokenResult{}, err
		}
	}
	if err := markRefreshTokenUsed(ctx, tx, row, nextRefreshToken.TokenID, issuedAt); err != nil {
		return types.RefreshGatewayTokenResult{}, err
	}
	if err := upsertSession(ctx, tx, issueCommandFromRefresh(command, row.SessionID), row.SessionID, issuedAt, gatewayExpiresAt, proof); err != nil {
		return types.RefreshGatewayTokenResult{}, err
	}
	if err := insertRefreshToken(ctx, tx, command.TenantID, command.UserID, command.DeviceID, row.SessionID, nextRefreshToken, issuedAt, refreshExpiresAt, command.TraceID, command.RequestID); err != nil {
		return types.RefreshGatewayTokenResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return types.RefreshGatewayTokenResult{}, types.NewDBWriteFailed(err.Error())
	}
	return types.RefreshGatewayTokenResult{
		TenantID:               command.TenantID,
		UserID:                 command.UserID,
		DeviceID:               command.DeviceID,
		SessionID:              row.SessionID,
		Audience:               command.Audience,
		IssuedAtUnixMS:         issuedAt.UnixMilli(),
		GatewayExpiresAtUnixMS: gatewayExpiresAt.UnixMilli(),
		RefreshExpiresAtUnixMS: refreshExpiresAt.UnixMilli(),
	}, nil
}

func (r *Repository) ValidateRefreshGatewaySession(
	ctx context.Context,
	command types.RefreshGatewayTokenCommand,
	presentedTokenID types.RefreshTokenID,
	presentedTokenHash string,
	now time.Time,
) error {
	if r.pool == nil {
		return types.NewDBReadFailed("identity repository is not configured")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return types.NewDBReadFailed(err.Error())
	}
	defer func() { _ = tx.Rollback(ctx) }()

	row, err := lockRefreshToken(ctx, tx, command, presentedTokenID)
	if err != nil {
		return err
	}
	if row.TokenHash != presentedTokenHash {
		return types.NewInvalidRefreshToken("invalid refresh token")
	}
	if row.Status != "ACTIVE" {
		if err := r.handleRefreshTokenReuse(ctx, tx, row, command.TraceID, command.RequestID, now); err != nil {
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return types.NewDBWriteFailed(err.Error())
		}
		return types.NewRefreshTokenReuseDetected("refresh token was already used")
	}
	if !now.Before(row.ExpiresAt) {
		if err := revokeRefreshToken(ctx, tx, row, now); err != nil {
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return types.NewDBWriteFailed(err.Error())
		}
		return types.NewInvalidRefreshToken("refresh token expired")
	}
	if err := lockDevice(ctx, tx, command.TenantID, command.UserID, command.DeviceID); err != nil {
		return err
	}
	device, err := getDeviceForUpdate(ctx, tx, command.TenantID, command.UserID, command.DeviceID)
	if err != nil {
		return err
	}
	if device.Status == types.DeviceStatusRevoked {
		return types.NewDeviceRevoked("device is revoked")
	}
	if _, err := lockRefreshSession(ctx, tx, command.TenantID, command.UserID, command.DeviceID, row.SessionID); err != nil {
		return err
	}
	return nil
}

func (r *Repository) handleRefreshTokenReuse(ctx context.Context, tx pgx.Tx, row refreshTokenRow, traceID string, requestID string, reusedAt time.Time) error {
	revoked, err := r.revokeSessionAfterRefreshReuse(ctx, tx, row, reusedAt)
	if err != nil {
		return err
	}
	if revoked == nil {
		return nil
	}
	return r.insertSessionRevokedOutbox(ctx, tx, *revoked, types.RevokeSessionCommand{
		AdminContext: types.AdminContext{
			TenantID:       row.TenantID,
			OperatorUserID: "identity-service",
			TraceID:        traceID,
			RequestID:      requestID,
		},
		UserID:    row.UserID,
		DeviceID:  row.DeviceID,
		SessionID: row.SessionID,
		Reason:    "refresh token reuse detected",
	}, reusedAt)
}

func (r *Repository) CreateMFAFactor(
	ctx context.Context,
	command types.BeginMFAEnrollmentCommand,
	secret types.EncryptedMFASecret,
	createdAt time.Time,
) (types.BeginMFAEnrollmentResult, error) {
	if r.pool == nil {
		return types.BeginMFAEnrollmentResult{}, types.NewDBWriteFailed("identity repository is not configured")
	}
	factorID, err := r.factorID()
	if err != nil {
		return types.BeginMFAEnrollmentResult{}, types.NewDBWriteFailed(err.Error())
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return types.BeginMFAEnrollmentResult{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lockExistingUser(ctx, tx, command.TenantID, command.UserID); err != nil {
		return types.BeginMFAEnrollmentResult{}, err
	}
	if err := ensureMFAFactorCreationAllowed(ctx, tx, command.TenantID, command.UserID); err != nil {
		return types.BeginMFAEnrollmentResult{}, err
	}
	_, err = tx.Exec(ctx, `
INSERT INTO identity_mfa_factors (
    tenant_id,
    user_id,
    factor_id,
    factor_type,
    status,
    display_name,
    secret_ciphertext,
    secret_nonce,
    secret_key_version,
    created_at,
    trace_id,
    request_id,
    updated_at
) VALUES ($1, $2, $3, 'TOTP', 'PENDING', $4, $5, $6, $7, $8, $9, $10, $8)
`, command.TenantID, command.UserID, factorID, strings.TrimSpace(command.DisplayName), secret.Ciphertext, secret.Nonce, secret.KeyVersion, createdAt, command.TraceID, command.RequestID)
	if err != nil {
		return types.BeginMFAEnrollmentResult{}, types.NewDBWriteFailed(err.Error())
	}
	if err := tx.Commit(ctx); err != nil {
		return types.BeginMFAEnrollmentResult{}, types.NewDBWriteFailed(err.Error())
	}
	return types.BeginMFAEnrollmentResult{
		TenantID:        command.TenantID,
		UserID:          command.UserID,
		FactorID:        types.MFAFactorID(factorID),
		FactorType:      types.MFAFactorTypeTOTP,
		Status:          types.MFAFactorStatusPending,
		CreatedAtUnixMS: createdAt.UnixMilli(),
	}, nil
}

func (r *Repository) GetMFAFactorSecret(ctx context.Context, tenantID types.TenantID, userID types.UserID, factorID types.MFAFactorID) (types.MFAFactorSecret, error) {
	if r.pool == nil {
		return types.MFAFactorSecret{}, types.NewDBReadFailed("identity repository is not configured")
	}
	var row types.MFAFactorSecret
	err := r.pool.QueryRow(ctx, `
SELECT
    tenant_id,
    user_id,
    factor_id,
    factor_type,
    status,
    secret_ciphertext,
    secret_nonce,
    secret_key_version
FROM identity_mfa_factors
WHERE tenant_id = $1
  AND user_id = $2
  AND factor_id = $3
`, tenantID, userID, factorID).Scan(
		&row.TenantID,
		&row.UserID,
		&row.FactorID,
		&row.Type,
		&row.Status,
		&row.Secret.Ciphertext,
		&row.Secret.Nonce,
		&row.Secret.KeyVersion,
	)
	if err == pgx.ErrNoRows {
		return types.MFAFactorSecret{}, types.NewMFAFactorNotFound("mfa factor not found")
	}
	if err != nil {
		return types.MFAFactorSecret{}, types.NewDBReadFailed(err.Error())
	}
	return row, nil
}

func (r *Repository) ListActiveMFAFactorSecrets(ctx context.Context, tenantID types.TenantID, userID types.UserID) ([]types.MFAFactorSecret, error) {
	if r.pool == nil {
		return nil, types.NewDBReadFailed("identity repository is not configured")
	}
	rows, err := r.pool.Query(ctx, `
SELECT
    tenant_id,
    user_id,
    factor_id,
    factor_type,
    status,
    secret_ciphertext,
    secret_nonce,
    secret_key_version,
    login_failed_count,
    COALESCE(login_locked_until, 'epoch'::timestamptz)
FROM identity_mfa_factors
WHERE tenant_id = $1
  AND user_id = $2
  AND factor_type = 'TOTP'
  AND status = 'ACTIVE'
ORDER BY verified_at ASC NULLS LAST, created_at ASC, factor_id ASC
`, tenantID, userID)
	if err != nil {
		return nil, types.NewDBReadFailed(err.Error())
	}
	defer rows.Close()
	factors := make([]types.MFAFactorSecret, 0)
	for rows.Next() {
		var row types.MFAFactorSecret
		if err := rows.Scan(
			&row.TenantID,
			&row.UserID,
			&row.FactorID,
			&row.Type,
			&row.Status,
			&row.Secret.Ciphertext,
			&row.Secret.Nonce,
			&row.Secret.KeyVersion,
			&row.LoginFailedCount,
			&row.LoginLockedUntil,
		); err != nil {
			return nil, types.NewDBReadFailed(err.Error())
		}
		factors = append(factors, row)
	}
	if err := rows.Err(); err != nil {
		return nil, types.NewDBReadFailed(err.Error())
	}
	return factors, nil
}

func (r *Repository) RecordMFALoginFailure(
	ctx context.Context,
	tenantID types.TenantID,
	userID types.UserID,
	factorID types.MFAFactorID,
	failedAt time.Time,
	lockUntil time.Time,
	maxFailedAttempts int,
	failureWindowStart time.Time,
) error {
	if r.pool == nil {
		return types.NewDBWriteFailed("identity repository is not configured")
	}
	if maxFailedAttempts <= 0 {
		return types.NewInvalidArgument("max failed attempts must be positive")
	}
	var failedCount int
	var lockedUntil time.Time
	err := r.pool.QueryRow(ctx, `
WITH next_failure AS (
    SELECT
        tenant_id,
        user_id,
        factor_id,
        CASE
            WHEN login_failed_last_at IS NULL
              OR login_failed_last_at < $7
              OR (login_failed_count >= $6 AND COALESCE(login_locked_until, 'epoch'::timestamptz) <= $4)
                THEN 1
            ELSE login_failed_count + 1
        END AS next_failed_login_count
    FROM identity_mfa_factors
    WHERE tenant_id = $1
      AND user_id = $2
      AND factor_id = $3
      AND factor_type = 'TOTP'
      AND status = 'ACTIVE'
    FOR UPDATE
)
UPDATE identity_mfa_factors
SET login_failed_count = next_failure.next_failed_login_count,
    login_failed_last_at = $4,
    login_locked_until = CASE
        WHEN next_failure.next_failed_login_count >= $6 THEN $5::timestamptz
        ELSE NULL
    END,
    updated_at = $4
FROM next_failure
WHERE identity_mfa_factors.tenant_id = next_failure.tenant_id
  AND identity_mfa_factors.user_id = next_failure.user_id
  AND identity_mfa_factors.factor_id = next_failure.factor_id
RETURNING login_failed_count, COALESCE(login_locked_until, 'epoch'::timestamptz)
`, tenantID, userID, factorID, failedAt, lockUntil, maxFailedAttempts, failureWindowStart).Scan(&failedCount, &lockedUntil)
	if err == pgx.ErrNoRows {
		return types.NewMFAFactorNotFound("mfa factor not found")
	}
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	if lockedUntil.After(failedAt) {
		return types.NewMFALocked("mfa temporarily locked")
	}
	return nil
}

func (r *Repository) ConfirmMFAFactor(ctx context.Context, command types.ConfirmMFAEnrollmentCommand, recoveryCodes []types.MFARecoveryCodeRecord, verifiedAt time.Time) (types.ConfirmMFAEnrollmentResult, error) {
	if r.pool == nil {
		return types.ConfirmMFAEnrollmentResult{}, types.NewDBWriteFailed("identity repository is not configured")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return types.ConfirmMFAEnrollmentResult{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var status types.MFAFactorStatus
	err = tx.QueryRow(ctx, `
UPDATE identity_mfa_factors
SET status = 'ACTIVE',
    verified_at = $4,
    updated_at = $4
WHERE tenant_id = $1
  AND user_id = $2
  AND factor_id = $3
  AND factor_type = 'TOTP'
  AND status = 'PENDING'
RETURNING status
`, command.TenantID, command.UserID, command.FactorID, verifiedAt).Scan(&status)
	if err == pgx.ErrNoRows {
		return types.ConfirmMFAEnrollmentResult{}, types.NewMFAFactorNotFound("mfa factor not found")
	}
	if err != nil {
		return types.ConfirmMFAEnrollmentResult{}, types.NewDBWriteFailed(err.Error())
	}
	if err := replaceMFARecoveryCodes(ctx, tx, command.TenantID, command.UserID, command.TraceID, command.RequestID, recoveryCodes, verifiedAt); err != nil {
		return types.ConfirmMFAEnrollmentResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return types.ConfirmMFAEnrollmentResult{}, types.NewDBWriteFailed(err.Error())
	}
	return types.ConfirmMFAEnrollmentResult{
		TenantID:         command.TenantID,
		UserID:           command.UserID,
		FactorID:         command.FactorID,
		Status:           status,
		VerifiedAtUnixMS: verifiedAt.UnixMilli(),
	}, nil
}

func (r *Repository) FindActiveMFARecoveryCode(ctx context.Context, tenantID types.TenantID, userID types.UserID, codeHash string) (types.MFARecoveryCodeRecord, error) {
	if r.pool == nil {
		return types.MFARecoveryCodeRecord{}, types.NewDBReadFailed("identity repository is not configured")
	}
	var record types.MFARecoveryCodeRecord
	err := r.pool.QueryRow(ctx, `
SELECT code_id, code_hash
FROM identity_mfa_recovery_codes
WHERE tenant_id = $1
  AND user_id = $2
  AND code_hash = $3
  AND status = 'ACTIVE'
`, tenantID, userID, codeHash).Scan(&record.CodeID, &record.CodeHash)
	if err == pgx.ErrNoRows {
		return types.MFARecoveryCodeRecord{}, types.NewInvalidMFA("invalid recovery code")
	}
	if err != nil {
		return types.MFARecoveryCodeRecord{}, types.NewDBReadFailed(err.Error())
	}
	return record, nil
}

func (r *Repository) DisableMFAFactor(ctx context.Context, command types.DisableMFAFactorCommand, disabledAt time.Time) (types.DisableMFAFactorResult, error) {
	if r.pool == nil {
		return types.DisableMFAFactorResult{}, types.NewDBWriteFailed("identity repository is not configured")
	}
	var status types.MFAFactorStatus
	err := r.pool.QueryRow(ctx, `
UPDATE identity_mfa_factors
SET status = 'DISABLED',
    disabled_at = $4,
    updated_at = $4
WHERE tenant_id = $1
  AND user_id = $2
  AND factor_id = $3
  AND factor_type = 'TOTP'
  AND status IN ('PENDING', 'ACTIVE')
RETURNING status
`, command.TenantID, command.UserID, command.FactorID, disabledAt).Scan(&status)
	if err == pgx.ErrNoRows {
		return types.DisableMFAFactorResult{}, types.NewMFAFactorNotFound("mfa factor not found")
	}
	if err != nil {
		return types.DisableMFAFactorResult{}, types.NewDBWriteFailed(err.Error())
	}
	return types.DisableMFAFactorResult{
		TenantID:         command.TenantID,
		UserID:           command.UserID,
		FactorID:         command.FactorID,
		Status:           status,
		DisabledAtUnixMS: disabledAt.UnixMilli(),
	}, nil
}

func (r *Repository) ReplaceMFARecoveryCodes(ctx context.Context, command types.RegenerateMFARecoveryCodesCommand, recoveryCodes []types.MFARecoveryCodeRecord, generatedAt time.Time) (types.RegenerateMFARecoveryCodesResult, error) {
	if r.pool == nil {
		return types.RegenerateMFARecoveryCodesResult{}, types.NewDBWriteFailed("identity repository is not configured")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return types.RegenerateMFARecoveryCodesResult{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lockActiveTOTPFactor(ctx, tx, command.TenantID, command.UserID, command.FactorID); err != nil {
		return types.RegenerateMFARecoveryCodesResult{}, err
	}
	if err := replaceMFARecoveryCodes(ctx, tx, command.TenantID, command.UserID, command.TraceID, command.RequestID, recoveryCodes, generatedAt); err != nil {
		return types.RegenerateMFARecoveryCodesResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return types.RegenerateMFARecoveryCodesResult{}, types.NewDBWriteFailed(err.Error())
	}
	return types.RegenerateMFARecoveryCodesResult{
		TenantID:          command.TenantID,
		UserID:            command.UserID,
		FactorID:          command.FactorID,
		GeneratedAtUnixMS: generatedAt.UnixMilli(),
	}, nil
}

func (r *Repository) RevokeMFARecoveryCodes(ctx context.Context, command types.RevokeMFARecoveryCodesCommand, revokedAt time.Time) (types.RevokeMFARecoveryCodesResult, error) {
	if r.pool == nil {
		return types.RevokeMFARecoveryCodesResult{}, types.NewDBWriteFailed("identity repository is not configured")
	}
	tag, err := r.pool.Exec(ctx, `
UPDATE identity_mfa_recovery_codes
SET status = 'DISABLED',
    disabled_at = $3,
    updated_at = $3,
    trace_id = $4,
    request_id = $5
WHERE tenant_id = $1
  AND user_id = $2
  AND status = 'ACTIVE'
`, command.TenantID, command.UserID, revokedAt, command.TraceID, command.RequestID)
	if err != nil {
		return types.RevokeMFARecoveryCodesResult{}, types.NewDBWriteFailed(err.Error())
	}
	return types.RevokeMFARecoveryCodesResult{
		TenantID:        command.TenantID,
		UserID:          command.UserID,
		RevokedCount:    int(tag.RowsAffected()),
		RevokedAtUnixMS: revokedAt.UnixMilli(),
	}, nil
}

func (r *Repository) IssueGatewaySession(
	ctx context.Context,
	command types.IssueGatewayTokenCommand,
	issuedAt time.Time,
	expiresAt time.Time,
) (types.IssueGatewayTokenResult, error) {
	if r.pool == nil {
		return types.IssueGatewayTokenResult{}, types.NewDBWriteFailed("identity repository is not configured")
	}
	sessionID, err := r.newSessionID(command.SessionID)
	if err != nil {
		return types.IssueGatewayTokenResult{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return types.IssueGatewayTokenResult{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := lockDevice(ctx, tx, command.TenantID, command.UserID, command.DeviceID); err != nil {
		return types.IssueGatewayTokenResult{}, err
	}
	if err := ensureUser(ctx, tx, command.TenantID, command.UserID); err != nil {
		return types.IssueGatewayTokenResult{}, err
	}
	device, err := ensureActiveDevice(ctx, tx, command.TenantID, command.UserID, command.DeviceID)
	if err != nil {
		return types.IssueGatewayTokenResult{}, err
	}
	if device.Status == types.DeviceStatusRevoked {
		return types.IssueGatewayTokenResult{}, types.NewDeviceRevoked("device is revoked")
	}
	if err := ensureSessionCanIssue(ctx, tx, command.TenantID, command.UserID, command.DeviceID, sessionID); err != nil {
		return types.IssueGatewayTokenResult{}, err
	}
	if err := upsertSession(ctx, tx, command, sessionID, issuedAt, expiresAt, sessionMFAProof{}); err != nil {
		return types.IssueGatewayTokenResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return types.IssueGatewayTokenResult{}, types.NewDBWriteFailed(err.Error())
	}
	return types.IssueGatewayTokenResult{
		TenantID:        command.TenantID,
		UserID:          command.UserID,
		DeviceID:        command.DeviceID,
		SessionID:       sessionID,
		Audience:        command.Audience,
		IssuedAtUnixMS:  issuedAt.UnixMilli(),
		ExpiresAtUnixMS: expiresAt.UnixMilli(),
	}, nil
}

func (r *Repository) newSessionID(input types.SessionID) (types.SessionID, error) {
	if input != "" {
		return input, nil
	}
	generated, err := r.sessionID()
	if err != nil {
		return "", types.NewDBWriteFailed(err.Error())
	}
	return types.SessionID(generated), nil
}

func (r *Repository) RevokeDevice(ctx context.Context, command types.RevokeDeviceCommand, revokedAt time.Time) (types.RevokeDeviceResult, error) {
	if r.pool == nil {
		return types.RevokeDeviceResult{}, types.NewDBWriteFailed("identity repository is not configured")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return types.RevokeDeviceResult{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() { _ = tx.Rollback(ctx) }()
	row, err := updateDeviceRevoked(ctx, tx, command, revokedAt)
	if err != nil {
		return types.RevokeDeviceResult{}, err
	}
	if err := revokeDeviceSessions(ctx, tx, command, revokedAt); err != nil {
		return types.RevokeDeviceResult{}, err
	}
	if err := r.insertDeviceRevokedOutbox(ctx, tx, row, command, revokedAt); err != nil {
		return types.RevokeDeviceResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return types.RevokeDeviceResult{}, types.NewDBWriteFailed(err.Error())
	}
	return types.RevokeDeviceResult{
		TenantID:        row.TenantID,
		UserID:          row.UserID,
		DeviceID:        row.DeviceID,
		Status:          row.Status,
		RevokedAtUnixMS: row.RevokedAt.UnixMilli(),
	}, nil
}

func (r *Repository) RevokeSession(ctx context.Context, command types.RevokeSessionCommand, revokedAt time.Time) (types.RevokeSessionResult, error) {
	if r.pool == nil {
		return types.RevokeSessionResult{}, types.NewDBWriteFailed("identity repository is not configured")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return types.RevokeSessionResult{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var row sessionRow
	err = tx.QueryRow(ctx, `
UPDATE identity_sessions
SET status = 'REVOKED',
    revoked_at = $5,
    revoked_by = $6,
    revoke_reason = $7
WHERE tenant_id = $1
  AND user_id = $2
  AND device_id = $3
  AND session_id = $4
RETURNING tenant_id, user_id, device_id, session_id, status, revoked_at
`, command.AdminContext.TenantID, command.UserID, command.DeviceID, command.SessionID, revokedAt, command.AdminContext.OperatorUserID, command.Reason).Scan(
		&row.TenantID,
		&row.UserID,
		&row.DeviceID,
		&row.SessionID,
		&row.Status,
		&row.RevokedAt,
	)
	if err == pgx.ErrNoRows {
		return types.RevokeSessionResult{}, types.NewSessionNotFound("session not found")
	}
	if err != nil {
		return types.RevokeSessionResult{}, types.NewDBWriteFailed(err.Error())
	}
	if err := r.insertSessionRevokedOutbox(ctx, tx, row, command, revokedAt); err != nil {
		return types.RevokeSessionResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return types.RevokeSessionResult{}, types.NewDBWriteFailed(err.Error())
	}
	return types.RevokeSessionResult{
		TenantID:        row.TenantID,
		UserID:          row.UserID,
		DeviceID:        row.DeviceID,
		SessionID:       row.SessionID,
		Status:          row.Status,
		RevokedAtUnixMS: row.RevokedAt.UnixMilli(),
	}, nil
}

func (r *Repository) GetDeviceState(ctx context.Context, command types.GetDeviceStateCommand) (types.GetDeviceStateResult, error) {
	if r.pool == nil {
		return types.GetDeviceStateResult{}, types.NewDBReadFailed("identity repository is not configured")
	}
	var row deviceRow
	var revokedAt *time.Time
	err := r.pool.QueryRow(ctx, `
SELECT tenant_id, user_id, device_id, status, created_at, updated_at, revoked_at
FROM identity_devices
WHERE tenant_id = $1
  AND user_id = $2
  AND device_id = $3
`, command.AdminContext.TenantID, command.UserID, command.DeviceID).Scan(
		&row.TenantID,
		&row.UserID,
		&row.DeviceID,
		&row.Status,
		&row.CreatedAt,
		&row.UpdatedAt,
		&revokedAt,
	)
	if err == pgx.ErrNoRows {
		return types.GetDeviceStateResult{}, types.NewDeviceNotFound("device not found")
	}
	if err != nil {
		return types.GetDeviceStateResult{}, types.NewDBReadFailed(err.Error())
	}
	revokedAtUnixMS := int64(0)
	if revokedAt != nil {
		revokedAtUnixMS = revokedAt.UnixMilli()
	}
	return types.GetDeviceStateResult{
		TenantID:        row.TenantID,
		UserID:          row.UserID,
		DeviceID:        row.DeviceID,
		Status:          row.Status,
		CreatedAtUnixMS: row.CreatedAt.UnixMilli(),
		UpdatedAtUnixMS: row.UpdatedAt.UnixMilli(),
		RevokedAtUnixMS: revokedAtUnixMS,
	}, nil
}

type deviceRow struct {
	TenantID  types.TenantID
	UserID    types.UserID
	DeviceID  types.DeviceID
	Status    types.DeviceStatus
	CreatedAt time.Time
	UpdatedAt time.Time
	RevokedAt time.Time
}

type sessionRow struct {
	TenantID  types.TenantID
	UserID    types.UserID
	DeviceID  types.DeviceID
	SessionID types.SessionID
	Status    types.SessionStatus
	RevokedAt time.Time
}

func ensureUser(ctx context.Context, tx pgx.Tx, tenantID types.TenantID, userID types.UserID) error {
	_, err := tx.Exec(ctx, `
INSERT INTO identity_users (tenant_id, user_id, status, created_at, updated_at)
VALUES ($1, $2, 'ACTIVE', now(), now())
ON CONFLICT (tenant_id, user_id) DO UPDATE
SET updated_at = now()
`, tenantID, userID)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func lockExistingUser(ctx context.Context, tx pgx.Tx, tenantID types.TenantID, userID types.UserID) error {
	var status string
	err := tx.QueryRow(ctx, `
SELECT status
FROM identity_users
WHERE tenant_id = $1
  AND user_id = $2
FOR UPDATE
`, tenantID, userID).Scan(&status)
	if err == pgx.ErrNoRows {
		return types.NewInvalidCredentials("invalid credentials")
	}
	if err != nil {
		return types.NewDBReadFailed(err.Error())
	}
	if status != "ACTIVE" {
		return types.NewInvalidCredentials("invalid credentials")
	}
	return nil
}

func ensureMFAFactorCreationAllowed(ctx context.Context, tx pgx.Tx, tenantID types.TenantID, userID types.UserID) error {
	var factorCount int
	err := tx.QueryRow(ctx, `
SELECT count(*)
FROM identity_mfa_factors
WHERE tenant_id = $1
  AND user_id = $2
  AND status IN ('PENDING', 'ACTIVE')
`, tenantID, userID).Scan(&factorCount)
	if err != nil {
		return types.NewDBReadFailed(err.Error())
	}
	if factorCount >= maxEnabledMFAFactorsPerUser {
		return types.NewChallengeRateLimited("too many mfa factors")
	}
	return nil
}

func ensureUserCanLogin(ctx context.Context, tx pgx.Tx, tenantID types.TenantID, userID types.UserID, now time.Time) error {
	var status string
	var lockedUntil time.Time
	err := tx.QueryRow(ctx, `
SELECT status, COALESCE(locked_until, 'epoch'::timestamptz)
FROM identity_users
WHERE tenant_id = $1
  AND user_id = $2
FOR UPDATE
`, tenantID, userID).Scan(&status, &lockedUntil)
	if err == pgx.ErrNoRows {
		return types.NewInvalidCredentials("invalid credentials")
	}
	if err != nil {
		return types.NewDBReadFailed(err.Error())
	}
	if status != "ACTIVE" {
		return types.NewInvalidCredentials("invalid credentials")
	}
	if lockedUntil.After(now) {
		return types.NewAccountLocked("account temporarily locked")
	}
	return nil
}

func clearLoginFailures(ctx context.Context, tx pgx.Tx, tenantID types.TenantID, userID types.UserID, now time.Time) error {
	tag, err := tx.Exec(ctx, `
UPDATE identity_users
SET failed_login_count = 0,
    failed_login_last_at = NULL,
    locked_until = NULL,
    updated_at = $3
WHERE tenant_id = $1
  AND user_id = $2
`, tenantID, userID, now)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	if tag.RowsAffected() == 0 {
		return types.NewInvalidCredentials("invalid credentials")
	}
	return nil
}

func clearMFARecoveryLoginFailures(ctx context.Context, tx pgx.Tx, tenantID types.TenantID, userID types.UserID, now time.Time) error {
	tag, err := tx.Exec(ctx, `
UPDATE identity_users
SET mfa_recovery_failed_count = 0,
    mfa_recovery_failed_last_at = NULL,
    mfa_recovery_locked_until = NULL,
    updated_at = $3
WHERE tenant_id = $1
  AND user_id = $2
`, tenantID, userID, now)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	if tag.RowsAffected() == 0 {
		return types.NewInvalidCredentials("invalid credentials")
	}
	return nil
}

func ensureMFARecoveryLoginNotLocked(ctx context.Context, tx pgx.Tx, tenantID types.TenantID, userID types.UserID, now time.Time) error {
	var lockedUntil time.Time
	err := tx.QueryRow(ctx, `
SELECT COALESCE(mfa_recovery_locked_until, 'epoch'::timestamptz)
FROM identity_users
WHERE tenant_id = $1
  AND user_id = $2
FOR UPDATE
`, tenantID, userID).Scan(&lockedUntil)
	if err == pgx.ErrNoRows {
		return types.NewInvalidCredentials("invalid credentials")
	}
	if err != nil {
		return types.NewDBReadFailed(err.Error())
	}
	if lockedUntil.After(now) {
		return types.NewMFALocked("mfa temporarily locked")
	}
	return nil
}

func ensureMFALoginNotLocked(ctx context.Context, tx pgx.Tx, tenantID types.TenantID, userID types.UserID, factorID types.MFAFactorID, now time.Time) error {
	var lockedUntil time.Time
	err := tx.QueryRow(ctx, `
SELECT COALESCE(login_locked_until, 'epoch'::timestamptz)
FROM identity_mfa_factors
WHERE tenant_id = $1
  AND user_id = $2
  AND factor_id = $3
  AND factor_type = 'TOTP'
  AND status = 'ACTIVE'
FOR UPDATE
`, tenantID, userID, factorID).Scan(&lockedUntil)
	if err == pgx.ErrNoRows {
		return types.NewMFAFactorNotFound("mfa factor not found")
	}
	if err != nil {
		return types.NewDBReadFailed(err.Error())
	}
	if lockedUntil.After(now) {
		return types.NewMFALocked("mfa temporarily locked")
	}
	return nil
}

func clearMFALoginFailures(ctx context.Context, tx pgx.Tx, tenantID types.TenantID, userID types.UserID, factorID types.MFAFactorID, now time.Time) error {
	tag, err := tx.Exec(ctx, `
UPDATE identity_mfa_factors
SET login_failed_count = 0,
    login_failed_last_at = NULL,
    login_locked_until = NULL,
    last_used_at = $4,
    updated_at = $4
WHERE tenant_id = $1
  AND user_id = $2
  AND factor_id = $3
  AND factor_type = 'TOTP'
  AND status = 'ACTIVE'
`, tenantID, userID, factorID, now)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	if tag.RowsAffected() == 0 {
		return types.NewMFAFactorNotFound("mfa factor not found")
	}
	return nil
}

func lockActiveTOTPFactor(ctx context.Context, tx pgx.Tx, tenantID types.TenantID, userID types.UserID, factorID types.MFAFactorID) error {
	var lockedFactorID string
	err := tx.QueryRow(ctx, `
SELECT factor_id
FROM identity_mfa_factors
WHERE tenant_id = $1
  AND user_id = $2
  AND factor_id = $3
  AND factor_type = 'TOTP'
  AND status = 'ACTIVE'
FOR UPDATE
`, tenantID, userID, factorID).Scan(&lockedFactorID)
	if err == pgx.ErrNoRows {
		return types.NewMFAFactorNotFound("mfa factor not found")
	}
	if err != nil {
		return types.NewDBReadFailed(err.Error())
	}
	return nil
}

func replaceMFARecoveryCodes(ctx context.Context, tx pgx.Tx, tenantID types.TenantID, userID types.UserID, traceID string, requestID string, recoveryCodes []types.MFARecoveryCodeRecord, now time.Time) error {
	if len(recoveryCodes) == 0 {
		return types.NewMFAUnavailable("mfa recovery codes are not configured")
	}
	if _, err := tx.Exec(ctx, `
UPDATE identity_mfa_recovery_codes
SET status = 'DISABLED',
    disabled_at = $3,
    updated_at = $3
WHERE tenant_id = $1
  AND user_id = $2
  AND status = 'ACTIVE'
`, tenantID, userID, now); err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	for _, code := range recoveryCodes {
		if strings.TrimSpace(code.CodeID) == "" || strings.TrimSpace(code.CodeHash) == "" {
			return types.NewMFAUnavailable("mfa recovery code record is invalid")
		}
		if _, err := tx.Exec(ctx, `
INSERT INTO identity_mfa_recovery_codes (
    tenant_id,
    user_id,
    code_id,
    code_hash,
    status,
    created_at,
    trace_id,
    request_id,
    updated_at
) VALUES ($1, $2, $3, $4, 'ACTIVE', $5, $6, $7, $5)
`, tenantID, userID, code.CodeID, code.CodeHash, now, traceID, requestID); err != nil {
			return types.NewDBWriteFailed(err.Error())
		}
	}
	return nil
}

func consumeMFARecoveryCode(ctx context.Context, tx pgx.Tx, tenantID types.TenantID, userID types.UserID, record types.MFARecoveryCodeRecord, now time.Time) error {
	tag, err := tx.Exec(ctx, `
UPDATE identity_mfa_recovery_codes
SET status = 'USED',
    used_at = $5,
    updated_at = $5
WHERE tenant_id = $1
  AND user_id = $2
  AND code_id = $3
  AND code_hash = $4
  AND status = 'ACTIVE'
`, tenantID, userID, record.CodeID, record.CodeHash, now)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	if tag.RowsAffected() == 0 {
		return types.NewInvalidMFA("invalid recovery code")
	}
	return nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func lockDevice(ctx context.Context, tx pgx.Tx, tenantID types.TenantID, userID types.UserID, deviceID types.DeviceID) error {
	key := fmt.Sprintf("%s\x1f%s\x1f%s\x1fidentity_device", tenantID, userID, deviceID)
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, key)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func newID(prefix string) (string, error) {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		prefix = "id"
	}
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	return prefix + "_" + hex.EncodeToString(bytes[:]), nil
}
