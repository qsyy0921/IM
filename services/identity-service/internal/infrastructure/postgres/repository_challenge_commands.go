package postgres

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/qsyy0921/IM/services/identity-service/internal/types"
)

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

func (r *Repository) CleanupChallengeRequestLimits(ctx context.Context, cutoff time.Time, limit int, dryRun bool) (int64, error) {
	if r.pool == nil {
		return 0, types.NewDBWriteFailed("identity repository is not configured")
	}
	if limit <= 0 {
		return 0, nil
	}
	if dryRun {
		var deleted int64
		err := r.pool.QueryRow(ctx, `
WITH doomed AS (
    SELECT tenant_id, user_id, challenge_type, channel, target_key
    FROM identity_challenge_request_limits
    WHERE last_request_at < $1
      AND (locked_until IS NULL OR locked_until < $1)
    ORDER BY last_request_at ASC
    LIMIT $2
)
SELECT count(*) FROM doomed
`, cutoff, limit).Scan(&deleted)
		if err != nil {
			return 0, types.NewDBReadFailed(err.Error())
		}
		return deleted, nil
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
