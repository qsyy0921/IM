package postgres

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
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

func (r *Repository) CreateVerificationChallenge(
	ctx context.Context,
	command types.RequestVerificationChallengeCommand,
	challengeType types.ChallengeType,
	challenge types.ChallengeRecord,
	delivery types.ChallengeDeliveryRecord,
	issuedAt time.Time,
	expiresAt time.Time,
) (types.RequestVerificationChallengeResult, error) {
	if r.pool == nil {
		return types.RequestVerificationChallengeResult{}, types.NewDBWriteFailed("identity repository is not configured")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return types.RequestVerificationChallengeResult{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lockExistingUser(ctx, tx, command.TenantID, command.UserID); err != nil {
		return types.RequestVerificationChallengeResult{}, err
	}
	if err := upsertChallengeDestination(ctx, tx, command.TenantID, command.UserID, command.Channel, command.Destination, issuedAt); err != nil {
		return types.RequestVerificationChallengeResult{}, err
	}
	if err := ensureChallengeCreationAllowed(ctx, tx, command.TenantID, command.UserID, challengeType, command.Channel, command.Destination, issuedAt, r.challengeRequestMaxPerWindow, r.challengeRequestWindow); err != nil {
		return types.RequestVerificationChallengeResult{}, err
	}
	if err := insertIdentityChallenge(ctx, tx, command.TenantID, command.UserID, challenge.ChallengeID, challengeType, command.Channel, command.Destination, challenge.TokenHash, issuedAt, expiresAt, command.TraceID, command.RequestID); err != nil {
		return types.RequestVerificationChallengeResult{}, err
	}
	if err := insertChallengeDeliveryOutbox(ctx, tx, command.TenantID, command.UserID, challenge.ChallengeID, challengeType, command.Channel, command.Destination, delivery, issuedAt, expiresAt, command.TraceID, command.RequestID); err != nil {
		return types.RequestVerificationChallengeResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return types.RequestVerificationChallengeResult{}, types.NewDBWriteFailed(err.Error())
	}
	return types.RequestVerificationChallengeResult{
		TenantID:        command.TenantID,
		UserID:          command.UserID,
		ChallengeID:     challenge.ChallengeID,
		Channel:         command.Channel,
		Destination:     command.Destination,
		ExpiresAtUnixMS: expiresAt.UnixMilli(),
	}, nil
}

func (r *Repository) ConfirmVerificationChallenge(
	ctx context.Context,
	command types.ConfirmVerificationChallengeCommand,
	tokenHash string,
	confirmedAt time.Time,
) (types.ConfirmVerificationChallengeResult, error) {
	if r.pool == nil {
		return types.ConfirmVerificationChallengeResult{}, types.NewDBWriteFailed("identity repository is not configured")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return types.ConfirmVerificationChallengeResult{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() { _ = tx.Rollback(ctx) }()
	challenge, err := lockIdentityChallenge(ctx, tx, command.TenantID, command.UserID, command.ChallengeID)
	if err != nil {
		return types.ConfirmVerificationChallengeResult{}, err
	}
	if challenge.Type != types.ChallengeTypeEmailVerification && challenge.Type != types.ChallengeTypePhoneVerification {
		return types.ConfirmVerificationChallengeResult{}, types.NewInvalidChallenge("challenge type mismatch")
	}
	if err := verifyChallengeToken(ctx, tx, challenge, tokenHash, confirmedAt); err != nil {
		return types.ConfirmVerificationChallengeResult{}, err
	}
	if err := markChallengeConsumed(ctx, tx, challenge, confirmedAt); err != nil {
		return types.ConfirmVerificationChallengeResult{}, err
	}
	if err := markDestinationVerified(ctx, tx, challenge, confirmedAt); err != nil {
		return types.ConfirmVerificationChallengeResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return types.ConfirmVerificationChallengeResult{}, types.NewDBWriteFailed(err.Error())
	}
	return types.ConfirmVerificationChallengeResult{
		TenantID:         challenge.TenantID,
		UserID:           challenge.UserID,
		Channel:          challenge.Channel,
		Destination:      challenge.Destination,
		VerifiedAtUnixMS: confirmedAt.UnixMilli(),
	}, nil
}

func (r *Repository) ExpireChallenge(
	ctx context.Context,
	tenantID types.TenantID,
	userID types.UserID,
	challengeID types.ChallengeID,
	expiredAt time.Time,
) error {
	if r.pool == nil {
		return types.NewDBWriteFailed("identity repository is not configured")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	defer func() { _ = tx.Rollback(ctx) }()
	challenge, err := lockIdentityChallenge(ctx, tx, tenantID, userID, challengeID)
	if err != nil {
		return err
	}
	if challenge.Status == "ACTIVE" {
		if err := expireChallenge(ctx, tx, challenge, expiredAt); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func (r *Repository) RecordChallengeDeliverySuccess(
	ctx context.Context,
	tenantID types.TenantID,
	userID types.UserID,
	challengeID types.ChallengeID,
	deliveredAt time.Time,
) error {
	if r.pool == nil {
		return types.NewDBWriteFailed("identity repository is not configured")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := lockIdentityChallenge(ctx, tx, tenantID, userID, challengeID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
UPDATE identity_challenges
SET delivery_status = 'DELIVERED',
    delivery_attempt_count = delivery_attempt_count + 1,
    delivered_at = $4,
    delivery_failed_at = NULL,
    delivery_last_error = '',
    delivery_failure_class = '',
    updated_at = $4
WHERE tenant_id = $1
  AND user_id = $2
  AND challenge_id = $3
`, tenantID, userID, challengeID, deliveredAt); err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	if err := tx.Commit(ctx); err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func (r *Repository) RecordChallengeDeliveryFailure(
	ctx context.Context,
	tenantID types.TenantID,
	userID types.UserID,
	challengeID types.ChallengeID,
	lastError string,
	failedAt time.Time,
) error {
	if r.pool == nil {
		return types.NewDBWriteFailed("identity repository is not configured")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := lockIdentityChallenge(ctx, tx, tenantID, userID, challengeID); err != nil {
		return err
	}
	lastError = sanitizeChallengeDeliveryError(lastError)
	failureClass := types.ClassifyChallengeDeliveryFailureMessage(lastError, true)
	if _, err := tx.Exec(ctx, `
UPDATE identity_challenges
SET status = CASE WHEN status = 'ACTIVE' THEN 'EXPIRED' ELSE status END,
    delivery_status = 'FAILED',
    delivery_attempt_count = delivery_attempt_count + 1,
    delivered_at = NULL,
    delivery_failed_at = $5,
    delivery_last_error = $4,
    delivery_failure_class = $6,
    updated_at = $5
WHERE tenant_id = $1
  AND user_id = $2
  AND challenge_id = $3
`, tenantID, userID, challengeID, lastError, failedAt, failureClass); err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	if err := tx.Commit(ctx); err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func (r *Repository) RecordPasswordResetRequest(
	ctx context.Context,
	tenantID types.TenantID,
	userID types.UserID,
	channel types.VerificationChannel,
	targetKey string,
	requestedAt time.Time,
) error {
	if r.pool == nil {
		return types.NewDBWriteFailed("identity repository is not configured")
	}
	targetKey = strings.TrimSpace(targetKey)
	if targetKey == "" || r.challengeRequestMaxPerWindow <= 0 || r.challengeRequestWindow <= 0 {
		return nil
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	defer func() { _ = tx.Rollback(ctx) }()
	err = recordChallengeRequestLimit(
		ctx,
		tx,
		tenantID,
		userID,
		types.ChallengeTypePasswordReset,
		channel,
		targetKey,
		requestedAt,
		r.challengeRequestMaxPerWindow,
		r.challengeRequestWindow,
		r.challengeRequestLockDuration,
	)
	if err != nil && !errors.Is(err, types.ErrChallengeRateLimited) {
		return err
	}
	if commitErr := tx.Commit(ctx); commitErr != nil {
		return types.NewDBWriteFailed(commitErr.Error())
	}
	return err
}

func (r *Repository) CleanupChallengeRequestLimits(ctx context.Context, cutoff time.Time, limit int) (int64, error) {
	if r.pool == nil {
		return 0, types.NewDBWriteFailed("identity repository is not configured")
	}
	if limit <= 0 {
		return 0, nil
	}
	rows, err := r.pool.Query(ctx, `
WITH doomed AS (
    SELECT tenant_id, user_id, challenge_type, channel, target_key
    FROM identity_challenge_request_limits
    WHERE last_request_at < $1
      AND (locked_until IS NULL OR locked_until < $1)
    ORDER BY last_request_at ASC
    LIMIT $2
    FOR UPDATE SKIP LOCKED
)
DELETE FROM identity_challenge_request_limits target
USING doomed
WHERE target.tenant_id = doomed.tenant_id
  AND target.user_id = doomed.user_id
  AND target.challenge_type = doomed.challenge_type
  AND target.channel = doomed.channel
  AND target.target_key = doomed.target_key
RETURNING 1
`, cutoff, limit)
	if err != nil {
		return 0, types.NewDBWriteFailed(err.Error())
	}
	defer rows.Close()
	var deleted int64
	for rows.Next() {
		deleted++
	}
	if err := rows.Err(); err != nil {
		return 0, types.NewDBWriteFailed(err.Error())
	}
	return deleted, nil
}

func (r *Repository) CreatePasswordResetChallenge(
	ctx context.Context,
	command types.RequestPasswordResetCommand,
	challenge types.ChallengeRecord,
	delivery types.ChallengeDeliveryRecord,
	issuedAt time.Time,
	expiresAt time.Time,
) (types.RequestPasswordResetResult, error) {
	if r.pool == nil {
		return types.RequestPasswordResetResult{}, types.NewDBWriteFailed("identity repository is not configured")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return types.RequestPasswordResetResult{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lockVerifiedDestination(ctx, tx, command.TenantID, command.UserID, command.Channel, command.Destination); err != nil {
		return types.RequestPasswordResetResult{}, err
	}
	if err := ensureChallengeCreationAllowed(ctx, tx, command.TenantID, command.UserID, types.ChallengeTypePasswordReset, command.Channel, command.Destination, issuedAt, r.challengeRequestMaxPerWindow, r.challengeRequestWindow); err != nil {
		return types.RequestPasswordResetResult{}, err
	}
	if err := insertIdentityChallenge(ctx, tx, command.TenantID, command.UserID, challenge.ChallengeID, types.ChallengeTypePasswordReset, command.Channel, command.Destination, challenge.TokenHash, issuedAt, expiresAt, command.TraceID, command.RequestID); err != nil {
		return types.RequestPasswordResetResult{}, err
	}
	if err := insertChallengeDeliveryOutbox(ctx, tx, command.TenantID, command.UserID, challenge.ChallengeID, types.ChallengeTypePasswordReset, command.Channel, command.Destination, delivery, issuedAt, expiresAt, command.TraceID, command.RequestID); err != nil {
		return types.RequestPasswordResetResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return types.RequestPasswordResetResult{}, types.NewDBWriteFailed(err.Error())
	}
	return types.RequestPasswordResetResult{
		TenantID:        command.TenantID,
		UserID:          command.UserID,
		ChallengeID:     challenge.ChallengeID,
		Channel:         command.Channel,
		Destination:     command.Destination,
		ExpiresAtUnixMS: expiresAt.UnixMilli(),
	}, nil
}

func (r *Repository) ConfirmPasswordReset(
	ctx context.Context,
	command types.ConfirmPasswordResetCommand,
	tokenHash string,
	passwordHash string,
	resetAt time.Time,
) (types.ConfirmPasswordResetResult, error) {
	if r.pool == nil {
		return types.ConfirmPasswordResetResult{}, types.NewDBWriteFailed("identity repository is not configured")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return types.ConfirmPasswordResetResult{}, types.NewDBWriteFailed(err.Error())
	}
	defer func() { _ = tx.Rollback(ctx) }()
	challenge, err := lockIdentityChallenge(ctx, tx, command.TenantID, command.UserID, command.ChallengeID)
	if err != nil {
		return types.ConfirmPasswordResetResult{}, err
	}
	if challenge.Type != types.ChallengeTypePasswordReset {
		return types.ConfirmPasswordResetResult{}, types.NewInvalidChallenge("challenge type mismatch")
	}
	if err := verifyChallengeToken(ctx, tx, challenge, tokenHash, resetAt); err != nil {
		return types.ConfirmPasswordResetResult{}, err
	}
	if err := markChallengeConsumed(ctx, tx, challenge, resetAt); err != nil {
		return types.ConfirmPasswordResetResult{}, err
	}
	if err := updatePasswordAfterReset(ctx, tx, command.TenantID, command.UserID, passwordHash, resetAt); err != nil {
		return types.ConfirmPasswordResetResult{}, err
	}
	if err := r.revokeUserSessionsAfterPasswordReset(ctx, tx, command.TenantID, command.UserID, resetAt, command.TraceID, command.RequestID); err != nil {
		return types.ConfirmPasswordResetResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return types.ConfirmPasswordResetResult{}, types.NewDBWriteFailed(err.Error())
	}
	return types.ConfirmPasswordResetResult{
		TenantID:      command.TenantID,
		UserID:        command.UserID,
		ResetAtUnixMS: resetAt.UnixMilli(),
	}, nil
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

type refreshTokenRow struct {
	TenantID  types.TenantID
	UserID    types.UserID
	DeviceID  types.DeviceID
	SessionID types.SessionID
	TokenID   types.RefreshTokenID
	TokenHash string
	Status    string
	ExpiresAt time.Time
}

type sessionMFAProof struct {
	Verified   bool
	VerifiedAt time.Time
	Method     string
	FactorID   types.MFAFactorID
}

type identityChallengeRow struct {
	TenantID     types.TenantID
	UserID       types.UserID
	ChallengeID  types.ChallengeID
	Type         types.ChallengeType
	Status       string
	Channel      types.VerificationChannel
	Destination  string
	TokenHash    string
	ExpiresAt    time.Time
	AttemptCount int
	MaxAttempts  int
}

func issueCommandFromLogin(command types.LoginCommand, sessionID types.SessionID) types.IssueGatewayTokenCommand {
	return types.IssueGatewayTokenCommand{
		TenantID:  command.TenantID,
		UserID:    command.UserID,
		DeviceID:  command.DeviceID,
		SessionID: sessionID,
		Audience:  command.Audience,
		TraceID:   command.TraceID,
		RequestID: command.RequestID,
	}
}

func issueCommandFromRefresh(command types.RefreshGatewayTokenCommand, sessionID types.SessionID) types.IssueGatewayTokenCommand {
	return types.IssueGatewayTokenCommand{
		TenantID:  command.TenantID,
		UserID:    command.UserID,
		DeviceID:  command.DeviceID,
		SessionID: sessionID,
		Audience:  command.Audience,
		TraceID:   command.TraceID,
		RequestID: command.RequestID,
	}
}

func insertRefreshToken(
	ctx context.Context,
	tx pgx.Tx,
	tenantID types.TenantID,
	userID types.UserID,
	deviceID types.DeviceID,
	sessionID types.SessionID,
	refreshToken types.RefreshTokenRecord,
	issuedAt time.Time,
	expiresAt time.Time,
	traceID string,
	requestID string,
) error {
	if refreshToken.TokenID == "" || refreshToken.TokenHash == "" {
		return types.NewTokenSigningFailed("refresh token is incomplete")
	}
	_, err := tx.Exec(ctx, `
INSERT INTO identity_refresh_tokens (
    tenant_id,
    user_id,
    device_id,
    session_id,
    token_id,
    token_hash,
    status,
    issued_at,
    expires_at,
    trace_id,
    request_id
) VALUES ($1, $2, $3, $4, $5, $6, 'ACTIVE', $7, $8, $9, $10)
`, tenantID, userID, deviceID, sessionID, refreshToken.TokenID, refreshToken.TokenHash, issuedAt, expiresAt, traceID, requestID)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func lockRefreshToken(ctx context.Context, tx pgx.Tx, command types.RefreshGatewayTokenCommand, tokenID types.RefreshTokenID) (refreshTokenRow, error) {
	var row refreshTokenRow
	err := tx.QueryRow(ctx, `
SELECT tenant_id, user_id, device_id, session_id, token_id, token_hash, status, expires_at
FROM identity_refresh_tokens
WHERE tenant_id = $1
  AND user_id = $2
  AND device_id = $3
  AND token_id = $4
FOR UPDATE
`, command.TenantID, command.UserID, command.DeviceID, tokenID).Scan(
		&row.TenantID,
		&row.UserID,
		&row.DeviceID,
		&row.SessionID,
		&row.TokenID,
		&row.TokenHash,
		&row.Status,
		&row.ExpiresAt,
	)
	if err == pgx.ErrNoRows {
		return refreshTokenRow{}, types.NewInvalidRefreshToken("invalid refresh token")
	}
	if err != nil {
		return refreshTokenRow{}, types.NewDBReadFailed(err.Error())
	}
	return row, nil
}

func markRefreshTokenUsed(ctx context.Context, tx pgx.Tx, row refreshTokenRow, replacedBy types.RefreshTokenID, usedAt time.Time) error {
	_, err := tx.Exec(ctx, `
UPDATE identity_refresh_tokens
SET status = 'USED',
    used_at = $6,
    replaced_by_token_id = $7,
    updated_at = $6
WHERE tenant_id = $1
  AND user_id = $2
  AND device_id = $3
  AND session_id = $4
  AND token_id = $5
`, row.TenantID, row.UserID, row.DeviceID, row.SessionID, row.TokenID, usedAt, replacedBy)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func revokeRefreshToken(ctx context.Context, tx pgx.Tx, row refreshTokenRow, revokedAt time.Time) error {
	_, err := tx.Exec(ctx, `
UPDATE identity_refresh_tokens
SET status = 'REVOKED',
    revoked_at = $6,
    updated_at = $6
WHERE tenant_id = $1
  AND user_id = $2
  AND device_id = $3
  AND session_id = $4
  AND token_id = $5
`, row.TenantID, row.UserID, row.DeviceID, row.SessionID, row.TokenID, revokedAt)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func (r *Repository) revokeSessionAfterRefreshReuse(ctx context.Context, tx pgx.Tx, row refreshTokenRow, revokedAt time.Time) (*sessionRow, error) {
	_, err := tx.Exec(ctx, `
UPDATE identity_refresh_tokens
SET status = 'REVOKED',
    revoked_at = $5,
    updated_at = $5
WHERE tenant_id = $1
  AND user_id = $2
  AND device_id = $3
  AND session_id = $4
  AND status = 'ACTIVE'
`, row.TenantID, row.UserID, row.DeviceID, row.SessionID, revokedAt)
	if err != nil {
		return nil, types.NewDBWriteFailed(err.Error())
	}

	var revoked sessionRow
	err = tx.QueryRow(ctx, `
UPDATE identity_sessions
SET status = 'REVOKED',
    revoked_at = $5,
    revoked_by = 'identity-service',
    revoke_reason = 'refresh token reuse detected'
WHERE tenant_id = $1
  AND user_id = $2
  AND device_id = $3
  AND session_id = $4
  AND status = 'ACTIVE'
RETURNING tenant_id, user_id, device_id, session_id, status, revoked_at
`, row.TenantID, row.UserID, row.DeviceID, row.SessionID, revokedAt).Scan(
		&revoked.TenantID,
		&revoked.UserID,
		&revoked.DeviceID,
		&revoked.SessionID,
		&revoked.Status,
		&revoked.RevokedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, types.NewDBWriteFailed(err.Error())
	}
	return &revoked, nil
}

func (r *Repository) revokeUserSessionsAfterPasswordReset(
	ctx context.Context,
	tx pgx.Tx,
	tenantID types.TenantID,
	userID types.UserID,
	revokedAt time.Time,
	traceID string,
	requestID string,
) error {
	_, err := tx.Exec(ctx, `
UPDATE identity_refresh_tokens
SET status = 'REVOKED',
    revoked_at = $3,
    updated_at = $3
WHERE tenant_id = $1
  AND user_id = $2
  AND status = 'ACTIVE'
`, tenantID, userID, revokedAt)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}

	rows, err := tx.Query(ctx, `
UPDATE identity_sessions
SET status = 'REVOKED',
    revoked_at = $3,
    revoked_by = 'identity-service',
    revoke_reason = 'password reset'
WHERE tenant_id = $1
  AND user_id = $2
  AND status = 'ACTIVE'
RETURNING tenant_id, user_id, device_id, session_id, status, revoked_at
`, tenantID, userID, revokedAt)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	defer rows.Close()

	var revokedSessions []sessionRow
	for rows.Next() {
		var row sessionRow
		if err := rows.Scan(
			&row.TenantID,
			&row.UserID,
			&row.DeviceID,
			&row.SessionID,
			&row.Status,
			&row.RevokedAt,
		); err != nil {
			return types.NewDBWriteFailed(err.Error())
		}
		revokedSessions = append(revokedSessions, row)
	}
	if err := rows.Err(); err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	rows.Close()

	command := types.RevokeSessionCommand{
		AdminContext: types.AdminContext{
			TenantID:       tenantID,
			OperatorUserID: "identity-service",
			TraceID:        traceID,
			RequestID:      requestID,
		},
		UserID: userID,
		Reason: "password reset",
	}
	for _, row := range revokedSessions {
		command.DeviceID = row.DeviceID
		command.SessionID = row.SessionID
		if err := r.insertSessionRevokedOutbox(ctx, tx, row, command, revokedAt); err != nil {
			return err
		}
	}
	return nil
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

type identityOutboxPayload struct {
	TenantID  string `json:"tenant_id"`
	UserID    string `json:"user_id"`
	DeviceID  string `json:"device_id"`
	SessionID string `json:"session_id,omitempty"`
	Status    string `json:"status"`
	RevokedBy string `json:"revoked_by"`
	Reason    string `json:"reason"`
	RevokedAt string `json:"revoked_at"`
}

func (r *Repository) insertDeviceRevokedOutbox(ctx context.Context, tx pgx.Tx, row deviceRow, command types.RevokeDeviceCommand, revokedAt time.Time) error {
	payload := identityOutboxPayload{
		TenantID:  string(row.TenantID),
		UserID:    string(row.UserID),
		DeviceID:  string(row.DeviceID),
		Status:    string(row.Status),
		RevokedBy: string(command.AdminContext.OperatorUserID),
		Reason:    command.Reason,
		RevokedAt: revokedAt.Format(time.RFC3339Nano),
	}
	return r.insertOutboxEvent(ctx, tx, outboxEventInput{
		TenantID:         row.TenantID,
		AggregateType:    "identity_device",
		AggregateID:      identityDeviceAggregateID(row.UserID, row.DeviceID),
		AggregateVersion: revokedAt.UnixMilli(),
		EventType:        types.IdentityEventDeviceRevoked,
		PartitionKey:     identityPartitionKey(row.TenantID, row.UserID, row.DeviceID),
		TraceID:          command.AdminContext.TraceID,
		CorrelationID:    command.AdminContext.RequestID,
		Payload:          payload,
	})
}

func (r *Repository) insertSessionRevokedOutbox(ctx context.Context, tx pgx.Tx, row sessionRow, command types.RevokeSessionCommand, revokedAt time.Time) error {
	payload := identityOutboxPayload{
		TenantID:  string(row.TenantID),
		UserID:    string(row.UserID),
		DeviceID:  string(row.DeviceID),
		SessionID: string(row.SessionID),
		Status:    string(row.Status),
		RevokedBy: string(command.AdminContext.OperatorUserID),
		Reason:    command.Reason,
		RevokedAt: revokedAt.Format(time.RFC3339Nano),
	}
	return r.insertOutboxEvent(ctx, tx, outboxEventInput{
		TenantID:         row.TenantID,
		AggregateType:    "identity_session",
		AggregateID:      identitySessionAggregateID(row.UserID, row.DeviceID, row.SessionID),
		AggregateVersion: revokedAt.UnixMilli(),
		EventType:        types.IdentityEventSessionRevoked,
		PartitionKey:     identityPartitionKey(row.TenantID, row.UserID, row.DeviceID),
		TraceID:          command.AdminContext.TraceID,
		CorrelationID:    command.AdminContext.RequestID,
		Payload:          payload,
	})
}

type outboxEventInput struct {
	TenantID         types.TenantID
	AggregateType    string
	AggregateID      string
	AggregateVersion int64
	EventType        string
	PartitionKey     string
	TraceID          string
	CorrelationID    string
	Payload          identityOutboxPayload
}

func (r *Repository) insertOutboxEvent(ctx context.Context, tx pgx.Tx, input outboxEventInput) error {
	eventID, err := r.eventID()
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	payloadJSON, err := json.Marshal(input.Payload)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	_, err = tx.Exec(ctx, `
INSERT INTO identity_outbox (
    event_id,
    tenant_id,
    aggregate_type,
    aggregate_id,
    aggregate_version,
    event_type,
    event_version,
    mapping_version,
    partition_key,
    producer,
    correlation_id,
    trace_id,
    payload_json
) VALUES ($1, $2, $3, $4, $5, $6, 'v1', 1, $7, 'identity-service', $8, $9, $10::jsonb)
`, eventID, input.TenantID, input.AggregateType, input.AggregateID, input.AggregateVersion, input.EventType, input.PartitionKey, input.CorrelationID, input.TraceID, string(payloadJSON))
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func identityPartitionKey(tenantID types.TenantID, userID types.UserID, deviceID types.DeviceID) string {
	return fmt.Sprintf("%s:%s:%s", tenantID, userID, deviceID)
}

func identityDeviceAggregateID(userID types.UserID, deviceID types.DeviceID) string {
	return fmt.Sprintf("%s:%s", userID, deviceID)
}

func identitySessionAggregateID(userID types.UserID, deviceID types.DeviceID, sessionID types.SessionID) string {
	return fmt.Sprintf("%s:%s:%s", userID, deviceID, sessionID)
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

func upsertChallengeDestination(ctx context.Context, tx pgx.Tx, tenantID types.TenantID, userID types.UserID, channel types.VerificationChannel, destination string, now time.Time) error {
	var err error
	switch channel {
	case types.VerificationChannelEmail:
		_, err = tx.Exec(ctx, `
UPDATE identity_users
SET email = $3,
    email_verified_at = CASE WHEN email = $3 THEN email_verified_at ELSE NULL END,
    updated_at = $4
WHERE tenant_id = $1
  AND user_id = $2
`, tenantID, userID, destination, now)
	case types.VerificationChannelPhone:
		_, err = tx.Exec(ctx, `
UPDATE identity_users
SET phone = $3,
    phone_verified_at = CASE WHEN phone = $3 THEN phone_verified_at ELSE NULL END,
    updated_at = $4
WHERE tenant_id = $1
  AND user_id = $2
`, tenantID, userID, destination, now)
	default:
		return types.NewInvalidArgument("verification channel is invalid")
	}
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func insertIdentityChallenge(
	ctx context.Context,
	tx pgx.Tx,
	tenantID types.TenantID,
	userID types.UserID,
	challengeID types.ChallengeID,
	challengeType types.ChallengeType,
	channel types.VerificationChannel,
	destination string,
	tokenHash string,
	issuedAt time.Time,
	expiresAt time.Time,
	traceID string,
	requestID string,
) error {
	_, err := tx.Exec(ctx, `
INSERT INTO identity_challenges (
    tenant_id,
    user_id,
    challenge_id,
    challenge_type,
    status,
    channel,
    destination,
    token_hash,
    issued_at,
    expires_at,
    trace_id,
    request_id,
    created_at,
    updated_at
) VALUES ($1, $2, $3, $4, 'ACTIVE', $5, $6, $7, $8, $9, $10, $11, $8, $8)
`, tenantID, userID, challengeID, challengeType, channel, destination, tokenHash, issuedAt, expiresAt, traceID, requestID)
	if isUniqueViolation(err) {
		return types.NewInvalidChallenge("challenge already exists")
	}
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func insertChallengeDeliveryOutbox(
	ctx context.Context,
	tx pgx.Tx,
	tenantID types.TenantID,
	userID types.UserID,
	challengeID types.ChallengeID,
	challengeType types.ChallengeType,
	channel types.VerificationChannel,
	destination string,
	delivery types.ChallengeDeliveryRecord,
	availableAt time.Time,
	expiresAt time.Time,
	traceID string,
	requestID string,
) error {
	if delivery.EncryptedToken.Ciphertext == "" && delivery.EncryptedToken.Nonce == "" && delivery.EncryptedToken.KeyVersion == "" {
		return nil
	}
	if delivery.EncryptedToken.Ciphertext == "" || delivery.EncryptedToken.Nonce == "" || delivery.EncryptedToken.KeyVersion == "" {
		return types.NewChallengeDeliveryFailed("challenge delivery encrypted token is incomplete")
	}
	_, err := tx.Exec(ctx, `
INSERT INTO identity_challenge_delivery_outbox (
    tenant_id,
    user_id,
    challenge_id,
    challenge_type,
    channel,
    destination,
    token_ciphertext,
    token_nonce,
    token_key_version,
    expires_at,
    trace_id,
    request_id,
    available_at,
    created_at,
    updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $13, $13)
`, tenantID, userID, challengeID, challengeType, channel, destination, delivery.EncryptedToken.Ciphertext, delivery.EncryptedToken.Nonce, delivery.EncryptedToken.KeyVersion, expiresAt, traceID, requestID, availableAt)
	if isUniqueViolation(err) {
		return types.NewInvalidChallenge("challenge delivery already exists")
	}
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func ensureChallengeCreationAllowed(
	ctx context.Context,
	tx pgx.Tx,
	tenantID types.TenantID,
	userID types.UserID,
	challengeType types.ChallengeType,
	channel types.VerificationChannel,
	destination string,
	now time.Time,
	maxPerWindow int,
	window time.Duration,
) error {
	var activeCount int
	err := tx.QueryRow(ctx, `
SELECT count(*)
FROM identity_challenges
WHERE tenant_id = $1
  AND user_id = $2
  AND challenge_type = $3
  AND channel = $4
  AND destination = $5
  AND status = 'ACTIVE'
  AND expires_at > $6
`, tenantID, userID, challengeType, channel, destination, now).Scan(&activeCount)
	if err != nil {
		return types.NewDBReadFailed(err.Error())
	}
	if activeCount >= maxActiveChallengesPerTarget {
		return types.NewChallengeRateLimited("too many active challenges")
	}
	if maxPerWindow > 0 && window > 0 {
		var recentCount int
		windowStart := now.Add(-window)
		err = tx.QueryRow(ctx, `
SELECT count(*)
FROM identity_challenges
WHERE tenant_id = $1
  AND user_id = $2
  AND challenge_type = $3
  AND channel = $4
  AND destination = $5
  AND issued_at >= $6
`, tenantID, userID, challengeType, channel, destination, windowStart).Scan(&recentCount)
		if err != nil {
			return types.NewDBReadFailed(err.Error())
		}
		if recentCount >= maxPerWindow {
			return types.NewChallengeRateLimited("too many recent challenges")
		}
	}
	return nil
}

func recordChallengeRequestLimit(
	ctx context.Context,
	tx pgx.Tx,
	tenantID types.TenantID,
	userID types.UserID,
	challengeType types.ChallengeType,
	channel types.VerificationChannel,
	targetKey string,
	now time.Time,
	maxPerWindow int,
	window time.Duration,
	lockDuration time.Duration,
) error {
	if err := lockChallengeRequestLimit(ctx, tx, tenantID, userID, challengeType, channel, targetKey); err != nil {
		return err
	}
	var requestCount int
	var windowStart time.Time
	var lockedUntil *time.Time
	err := tx.QueryRow(ctx, `
SELECT request_count, window_start, locked_until
FROM identity_challenge_request_limits
WHERE tenant_id = $1
  AND user_id = $2
  AND challenge_type = $3
  AND channel = $4
  AND target_key = $5
FOR UPDATE
`, tenantID, userID, challengeType, channel, targetKey).Scan(&requestCount, &windowStart, &lockedUntil)
	if err == pgx.ErrNoRows {
		_, err = tx.Exec(ctx, `
INSERT INTO identity_challenge_request_limits (
    tenant_id,
    user_id,
    challenge_type,
    channel,
    target_key,
    request_count,
    window_start,
    last_request_at,
    created_at,
    updated_at
) VALUES ($1, $2, $3, $4, $5, 1, $6, $6, $6, $6)
`, tenantID, userID, challengeType, channel, targetKey, now)
		if err != nil {
			return types.NewDBWriteFailed(err.Error())
		}
		return nil
	}
	if err != nil {
		return types.NewDBReadFailed(err.Error())
	}
	if lockedUntil != nil && lockedUntil.After(now) {
		if err := updateChallengeRequestLimitLastSeen(ctx, tx, tenantID, userID, challengeType, channel, targetKey, now); err != nil {
			return err
		}
		return types.NewChallengeRateLimited("challenge request temporarily limited")
	}
	windowStartThreshold := now.Add(-window)
	if windowStart.Before(windowStartThreshold) {
		_, err = tx.Exec(ctx, `
UPDATE identity_challenge_request_limits
SET request_count = 1,
    window_start = $6,
    last_request_at = $6,
    locked_until = NULL,
    updated_at = $6
WHERE tenant_id = $1
  AND user_id = $2
  AND challenge_type = $3
  AND channel = $4
  AND target_key = $5
`, tenantID, userID, challengeType, channel, targetKey, now)
		if err != nil {
			return types.NewDBWriteFailed(err.Error())
		}
		return nil
	}
	nextCount := requestCount + 1
	var nextLockedUntil any
	rateLimited := nextCount > maxPerWindow
	if rateLimited && lockDuration > 0 {
		nextLockedUntil = now.Add(lockDuration)
	}
	_, err = tx.Exec(ctx, `
UPDATE identity_challenge_request_limits
SET request_count = $6,
    last_request_at = $7,
    locked_until = $8,
    updated_at = $7
WHERE tenant_id = $1
  AND user_id = $2
  AND challenge_type = $3
  AND channel = $4
  AND target_key = $5
`, tenantID, userID, challengeType, channel, targetKey, nextCount, now, nextLockedUntil)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	if rateLimited {
		return types.NewChallengeRateLimited("challenge request temporarily limited")
	}
	return nil
}

func lockChallengeRequestLimit(
	ctx context.Context,
	tx pgx.Tx,
	tenantID types.TenantID,
	userID types.UserID,
	challengeType types.ChallengeType,
	channel types.VerificationChannel,
	targetKey string,
) error {
	key := fmt.Sprintf("%s\x1f%s\x1f%s\x1f%s\x1f%s\x1fidentity_challenge_request_limit", tenantID, userID, challengeType, channel, targetKey)
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, key)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func updateChallengeRequestLimitLastSeen(
	ctx context.Context,
	tx pgx.Tx,
	tenantID types.TenantID,
	userID types.UserID,
	challengeType types.ChallengeType,
	channel types.VerificationChannel,
	targetKey string,
	now time.Time,
) error {
	_, err := tx.Exec(ctx, `
UPDATE identity_challenge_request_limits
SET last_request_at = $6,
    updated_at = $6
WHERE tenant_id = $1
  AND user_id = $2
  AND challenge_type = $3
  AND channel = $4
  AND target_key = $5
`, tenantID, userID, challengeType, channel, targetKey, now)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
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

func lockIdentityChallenge(ctx context.Context, tx pgx.Tx, tenantID types.TenantID, userID types.UserID, challengeID types.ChallengeID) (identityChallengeRow, error) {
	var row identityChallengeRow
	err := tx.QueryRow(ctx, `
SELECT
    tenant_id,
    user_id,
    challenge_id,
    challenge_type,
    status,
    channel,
    destination,
    token_hash,
    expires_at,
    attempt_count,
    max_attempts
FROM identity_challenges
WHERE tenant_id = $1
  AND user_id = $2
  AND challenge_id = $3
FOR UPDATE
`, tenantID, userID, challengeID).Scan(
		&row.TenantID,
		&row.UserID,
		&row.ChallengeID,
		&row.Type,
		&row.Status,
		&row.Channel,
		&row.Destination,
		&row.TokenHash,
		&row.ExpiresAt,
		&row.AttemptCount,
		&row.MaxAttempts,
	)
	if err == pgx.ErrNoRows {
		return identityChallengeRow{}, types.NewInvalidChallenge("invalid challenge")
	}
	if err != nil {
		return identityChallengeRow{}, types.NewDBReadFailed(err.Error())
	}
	return row, nil
}

func verifyChallengeToken(ctx context.Context, tx pgx.Tx, challenge identityChallengeRow, tokenHash string, now time.Time) error {
	if challenge.Status != "ACTIVE" {
		return types.NewInvalidChallenge("challenge is not active")
	}
	if !now.Before(challenge.ExpiresAt) {
		if err := expireChallenge(ctx, tx, challenge, now); err != nil {
			return err
		}
		return types.NewChallengeExpired("challenge expired")
	}
	if subtle.ConstantTimeCompare([]byte(challenge.TokenHash), []byte(tokenHash)) != 1 {
		if err := recordChallengeAttempt(ctx, tx, challenge, now); err != nil {
			return err
		}
		return types.NewInvalidChallenge("invalid challenge")
	}
	return nil
}

func recordChallengeAttempt(ctx context.Context, tx pgx.Tx, challenge identityChallengeRow, now time.Time) error {
	nextAttempts := challenge.AttemptCount + 1
	status := "ACTIVE"
	if nextAttempts >= challenge.MaxAttempts {
		status = "EXPIRED"
	}
	_, err := tx.Exec(ctx, `
UPDATE identity_challenges
SET attempt_count = $4,
    status = $5,
    updated_at = $6
WHERE tenant_id = $1
  AND user_id = $2
  AND challenge_id = $3
`, challenge.TenantID, challenge.UserID, challenge.ChallengeID, nextAttempts, status, now)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func expireChallenge(ctx context.Context, tx pgx.Tx, challenge identityChallengeRow, now time.Time) error {
	_, err := tx.Exec(ctx, `
UPDATE identity_challenges
SET status = 'EXPIRED',
    updated_at = $4
WHERE tenant_id = $1
  AND user_id = $2
  AND challenge_id = $3
`, challenge.TenantID, challenge.UserID, challenge.ChallengeID, now)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func markChallengeConsumed(ctx context.Context, tx pgx.Tx, challenge identityChallengeRow, now time.Time) error {
	_, err := tx.Exec(ctx, `
UPDATE identity_challenges
SET status = 'CONSUMED',
    consumed_at = $4,
    updated_at = $4
WHERE tenant_id = $1
  AND user_id = $2
  AND challenge_id = $3
`, challenge.TenantID, challenge.UserID, challenge.ChallengeID, now)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func sanitizeChallengeDeliveryError(lastError string) string {
	lastError = strings.TrimSpace(lastError)
	if lastError == "" {
		return "challenge delivery unavailable"
	}
	if len(lastError) > 256 {
		return lastError[:256]
	}
	return lastError
}

func markDestinationVerified(ctx context.Context, tx pgx.Tx, challenge identityChallengeRow, now time.Time) error {
	var err error
	switch challenge.Channel {
	case types.VerificationChannelEmail:
		_, err = tx.Exec(ctx, `
UPDATE identity_users
SET email = $3,
    email_verified_at = $4,
    updated_at = $4
WHERE tenant_id = $1
  AND user_id = $2
`, challenge.TenantID, challenge.UserID, challenge.Destination, now)
	case types.VerificationChannelPhone:
		_, err = tx.Exec(ctx, `
UPDATE identity_users
SET phone = $3,
    phone_verified_at = $4,
    updated_at = $4
WHERE tenant_id = $1
  AND user_id = $2
`, challenge.TenantID, challenge.UserID, challenge.Destination, now)
	default:
		return types.NewInvalidChallenge("challenge channel is invalid")
	}
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func lockVerifiedDestination(ctx context.Context, tx pgx.Tx, tenantID types.TenantID, userID types.UserID, channel types.VerificationChannel, destination string) error {
	var matched bool
	var err error
	switch channel {
	case types.VerificationChannelEmail:
		err = tx.QueryRow(ctx, `
SELECT email = $3 AND email_verified_at IS NOT NULL
FROM identity_users
WHERE tenant_id = $1
  AND user_id = $2
  AND status = 'ACTIVE'
FOR UPDATE
`, tenantID, userID, destination).Scan(&matched)
	case types.VerificationChannelPhone:
		err = tx.QueryRow(ctx, `
SELECT phone = $3 AND phone_verified_at IS NOT NULL
FROM identity_users
WHERE tenant_id = $1
  AND user_id = $2
  AND status = 'ACTIVE'
FOR UPDATE
`, tenantID, userID, destination).Scan(&matched)
	default:
		return types.NewInvalidArgument("verification channel is invalid")
	}
	if err == pgx.ErrNoRows {
		return types.NewInvalidCredentials("invalid credentials")
	}
	if err != nil {
		return types.NewDBReadFailed(err.Error())
	}
	if !matched {
		return types.NewInvalidCredentials("invalid credentials")
	}
	return nil
}

func updatePasswordAfterReset(ctx context.Context, tx pgx.Tx, tenantID types.TenantID, userID types.UserID, passwordHash string, now time.Time) error {
	tag, err := tx.Exec(ctx, `
UPDATE identity_users
SET password_hash = $3,
    password_updated_at = $4,
    failed_login_count = 0,
    failed_login_last_at = NULL,
    locked_until = NULL,
    updated_at = $4
WHERE tenant_id = $1
  AND user_id = $2
  AND status = 'ACTIVE'
`, tenantID, userID, passwordHash, now)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	if tag.RowsAffected() == 0 {
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

func ensureActiveDevice(ctx context.Context, tx pgx.Tx, tenantID types.TenantID, userID types.UserID, deviceID types.DeviceID) (deviceRow, error) {
	var row deviceRow
	err := tx.QueryRow(ctx, `
INSERT INTO identity_devices (tenant_id, user_id, device_id, status, created_at, updated_at)
VALUES ($1, $2, $3, 'ACTIVE', now(), now())
ON CONFLICT (tenant_id, user_id, device_id) DO UPDATE
SET updated_at = now()
RETURNING tenant_id, user_id, device_id, status, created_at, updated_at, COALESCE(revoked_at, 'epoch'::timestamptz)
`, tenantID, userID, deviceID).Scan(
		&row.TenantID,
		&row.UserID,
		&row.DeviceID,
		&row.Status,
		&row.CreatedAt,
		&row.UpdatedAt,
		&row.RevokedAt,
	)
	if err != nil {
		return deviceRow{}, types.NewDBWriteFailed(err.Error())
	}
	return row, nil
}

func getDeviceForUpdate(ctx context.Context, tx pgx.Tx, tenantID types.TenantID, userID types.UserID, deviceID types.DeviceID) (deviceRow, error) {
	var row deviceRow
	err := tx.QueryRow(ctx, `
SELECT tenant_id, user_id, device_id, status, created_at, updated_at, COALESCE(revoked_at, 'epoch'::timestamptz)
FROM identity_devices
WHERE tenant_id = $1
  AND user_id = $2
  AND device_id = $3
FOR UPDATE
`, tenantID, userID, deviceID).Scan(
		&row.TenantID,
		&row.UserID,
		&row.DeviceID,
		&row.Status,
		&row.CreatedAt,
		&row.UpdatedAt,
		&row.RevokedAt,
	)
	if err == pgx.ErrNoRows {
		return deviceRow{}, types.NewDeviceNotFound("device not found")
	}
	if err != nil {
		return deviceRow{}, types.NewDBReadFailed(err.Error())
	}
	return row, nil
}

func sessionMFAProofFromLogin(command types.LoginCommand, verifiedAt time.Time) sessionMFAProof {
	if command.VerifiedMFAFactorID != "" {
		return sessionMFAProof{
			Verified:   true,
			VerifiedAt: verifiedAt,
			Method:     "TOTP",
			FactorID:   command.VerifiedMFAFactorID,
		}
	}
	if command.UsedMFARecoveryCode.CodeID != "" || command.UsedMFARecoveryCode.CodeHash != "" {
		return sessionMFAProof{
			Verified:   true,
			VerifiedAt: verifiedAt,
			Method:     "RECOVERY_CODE",
		}
	}
	return sessionMFAProof{}
}

func sessionMFAProofFromRefresh(command types.RefreshGatewayTokenCommand, verifiedAt time.Time) sessionMFAProof {
	if command.VerifiedMFAFactorID != "" {
		return sessionMFAProof{
			Verified:   true,
			VerifiedAt: verifiedAt,
			Method:     "TOTP",
			FactorID:   command.VerifiedMFAFactorID,
		}
	}
	if command.UsedMFARecoveryCode.CodeID != "" || command.UsedMFARecoveryCode.CodeHash != "" {
		return sessionMFAProof{
			Verified:   true,
			VerifiedAt: verifiedAt,
			Method:     "RECOVERY_CODE",
		}
	}
	return sessionMFAProof{}
}

func mergeRefreshMFAProof(existing sessionMFAProof, submitted sessionMFAProof) sessionMFAProof {
	if !submitted.Verified {
		return existing
	}
	if submitted.Method == "TOTP" {
		return submitted
	}
	if !existing.Verified {
		return submitted
	}
	return existing
}

func upsertSession(ctx context.Context, tx pgx.Tx, command types.IssueGatewayTokenCommand, sessionID types.SessionID, issuedAt time.Time, expiresAt time.Time, proof sessionMFAProof) error {
	var mfaVerifiedAt any
	if proof.Verified {
		mfaVerifiedAt = proof.VerifiedAt
	}
	_, err := tx.Exec(ctx, `
INSERT INTO identity_sessions (
    tenant_id,
    user_id,
    device_id,
    session_id,
    status,
    audience,
    issued_at,
    expires_at,
    mfa_verified_at,
    mfa_method,
    mfa_factor_id,
    trace_id,
    request_id
) VALUES ($1, $2, $3, $4, 'ACTIVE', $5, $6, $7, $8, $9, $10, $11, $12)
ON CONFLICT (tenant_id, user_id, device_id, session_id) DO UPDATE
SET status = 'ACTIVE',
    audience = EXCLUDED.audience,
    issued_at = EXCLUDED.issued_at,
    expires_at = EXCLUDED.expires_at,
    mfa_verified_at = EXCLUDED.mfa_verified_at,
    mfa_method = EXCLUDED.mfa_method,
    mfa_factor_id = EXCLUDED.mfa_factor_id,
    revoked_at = NULL,
    revoked_by = '',
    revoke_reason = '',
    trace_id = EXCLUDED.trace_id,
    request_id = EXCLUDED.request_id
`, command.TenantID, command.UserID, command.DeviceID, sessionID, command.Audience, issuedAt, expiresAt, mfaVerifiedAt, proof.Method, proof.FactorID, command.TraceID, command.RequestID)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
	}
	return nil
}

func ensureSessionCanIssue(ctx context.Context, tx pgx.Tx, tenantID types.TenantID, userID types.UserID, deviceID types.DeviceID, sessionID types.SessionID) error {
	var status types.SessionStatus
	err := tx.QueryRow(ctx, `
SELECT status
FROM identity_sessions
WHERE tenant_id = $1
  AND user_id = $2
  AND device_id = $3
  AND session_id = $4
FOR UPDATE
`, tenantID, userID, deviceID, sessionID).Scan(&status)
	if err == pgx.ErrNoRows {
		return nil
	}
	if err != nil {
		return types.NewDBReadFailed(err.Error())
	}
	if status == types.SessionStatusRevoked {
		return types.NewSessionRevoked("session is revoked")
	}
	return nil
}

func lockRefreshSession(ctx context.Context, tx pgx.Tx, tenantID types.TenantID, userID types.UserID, deviceID types.DeviceID, sessionID types.SessionID) (sessionMFAProof, error) {
	var status types.SessionStatus
	var verifiedAt time.Time
	var method string
	var factorID types.MFAFactorID
	err := tx.QueryRow(ctx, `
SELECT status, COALESCE(mfa_verified_at, 'epoch'::timestamptz), mfa_method, mfa_factor_id
FROM identity_sessions
WHERE tenant_id = $1
  AND user_id = $2
  AND device_id = $3
  AND session_id = $4
FOR UPDATE
`, tenantID, userID, deviceID, sessionID).Scan(&status, &verifiedAt, &method, &factorID)
	if err == pgx.ErrNoRows {
		return sessionMFAProof{}, types.NewSessionNotFound("session not found")
	}
	if err != nil {
		return sessionMFAProof{}, types.NewDBReadFailed(err.Error())
	}
	if status == types.SessionStatusRevoked {
		return sessionMFAProof{}, types.NewSessionRevoked("session is revoked")
	}
	if method == "" {
		return sessionMFAProof{}, nil
	}
	return sessionMFAProof{
		Verified:   true,
		VerifiedAt: verifiedAt,
		Method:     method,
		FactorID:   factorID,
	}, nil
}

func hasActiveTOTPFactor(ctx context.Context, tx pgx.Tx, tenantID types.TenantID, userID types.UserID) (bool, error) {
	var exists bool
	err := tx.QueryRow(ctx, `
SELECT EXISTS (
    SELECT 1
    FROM identity_mfa_factors
    WHERE tenant_id = $1
      AND user_id = $2
      AND factor_type = 'TOTP'
      AND status = 'ACTIVE'
)
`, tenantID, userID).Scan(&exists)
	if err != nil {
		return false, types.NewDBReadFailed(err.Error())
	}
	return exists, nil
}

func updateDeviceRevoked(ctx context.Context, tx pgx.Tx, command types.RevokeDeviceCommand, revokedAt time.Time) (deviceRow, error) {
	var row deviceRow
	err := tx.QueryRow(ctx, `
UPDATE identity_devices
SET status = 'REVOKED',
    updated_at = $4,
    revoked_at = $4,
    revoked_by = $5,
    revoke_reason = $6
WHERE tenant_id = $1
  AND user_id = $2
  AND device_id = $3
RETURNING tenant_id, user_id, device_id, status, created_at, updated_at, revoked_at
`, command.AdminContext.TenantID, command.UserID, command.DeviceID, revokedAt, command.AdminContext.OperatorUserID, command.Reason).Scan(
		&row.TenantID,
		&row.UserID,
		&row.DeviceID,
		&row.Status,
		&row.CreatedAt,
		&row.UpdatedAt,
		&row.RevokedAt,
	)
	if err == pgx.ErrNoRows {
		return deviceRow{}, types.NewDeviceNotFound("device not found")
	}
	if err != nil {
		return deviceRow{}, types.NewDBWriteFailed(err.Error())
	}
	return row, nil
}

func revokeDeviceSessions(ctx context.Context, tx pgx.Tx, command types.RevokeDeviceCommand, revokedAt time.Time) error {
	_, err := tx.Exec(ctx, `
UPDATE identity_sessions
SET status = 'REVOKED',
    revoked_at = $4,
    revoked_by = $5,
    revoke_reason = $6
WHERE tenant_id = $1
  AND user_id = $2
  AND device_id = $3
  AND status = 'ACTIVE'
`, command.AdminContext.TenantID, command.UserID, command.DeviceID, revokedAt, command.AdminContext.OperatorUserID, command.Reason)
	if err != nil {
		return types.NewDBWriteFailed(err.Error())
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
