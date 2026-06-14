package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/qsyy0921/IM/services/identity-service/internal/types"
)

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
