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
