package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/qsyy0921/IM/services/identity-service/internal/types"
)

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
