package postgres

import (
	"context"
	"crypto/subtle"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/qsyy0921/IM/services/identity-service/internal/types"
)

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
	failureClass := types.ClassifyChallengeDeliveryFailureMessage(lastError, true)
	switch failureClass {
	case types.ChallengeDeliveryFailureClassInactive:
		return "challenge no longer active before delivery"
	case types.ChallengeDeliveryFailureClassConfiguration:
		return "challenge delivery not configured"
	case types.ChallengeDeliveryFailureClassProviderNonSuccess:
		return "challenge delivery provider returned non-success status"
	case types.ChallengeDeliveryFailureClassTimeout:
		return "challenge delivery timeout"
	case types.ChallengeDeliveryFailureClassNetwork:
		return "challenge delivery network failed"
	case types.ChallengeDeliveryFailureClassSerialization:
		return "challenge delivery json serialization failed"
	case types.ChallengeDeliveryFailureClassTokenCrypto:
		return "challenge delivery token decrypt failed"
	case types.ChallengeDeliveryFailureClassCanceled:
		return "challenge delivery canceled"
	case types.ChallengeDeliveryFailureClassDeliveryFailed, types.ChallengeDeliveryFailureClassUnknown:
		return "challenge delivery unavailable"
	default:
		return "challenge delivery unavailable"
	}
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
