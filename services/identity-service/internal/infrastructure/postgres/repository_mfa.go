package postgres

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/qsyy0921/IM/services/identity-service/internal/types"
)

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
