package postgres

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/identity-service/internal/types"
)

type Repository struct {
	pool      *pgxpool.Pool
	sessionID func() (string, error)
}

type RepositoryOption func(*Repository)

func NewRepository(pool *pgxpool.Pool, opts ...RepositoryOption) *Repository {
	repository := &Repository{
		pool: pool,
		sessionID: func() (string, error) {
			return newID("sess")
		},
	}
	for _, opt := range opts {
		opt(repository)
	}
	return repository
}

func WithSessionIDGenerator(generator func() (string, error)) RepositoryOption {
	return func(repository *Repository) {
		if generator != nil {
			repository.sessionID = generator
		}
	}
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
	sessionID := command.SessionID
	if sessionID == "" {
		generated, err := r.sessionID()
		if err != nil {
			return types.IssueGatewayTokenResult{}, types.NewDBWriteFailed(err.Error())
		}
		sessionID = types.SessionID(generated)
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
	if err := upsertSession(ctx, tx, command, sessionID, issuedAt, expiresAt); err != nil {
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
	var row sessionRow
	err := r.pool.QueryRow(ctx, `
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

func upsertSession(ctx context.Context, tx pgx.Tx, command types.IssueGatewayTokenCommand, sessionID types.SessionID, issuedAt time.Time, expiresAt time.Time) error {
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
    trace_id,
    request_id
) VALUES ($1, $2, $3, $4, 'ACTIVE', $5, $6, $7, $8, $9)
ON CONFLICT (tenant_id, user_id, device_id, session_id) DO UPDATE
SET status = 'ACTIVE',
    audience = EXCLUDED.audience,
    issued_at = EXCLUDED.issued_at,
    expires_at = EXCLUDED.expires_at,
    revoked_at = NULL,
    revoked_by = '',
    revoke_reason = '',
    trace_id = EXCLUDED.trace_id,
    request_id = EXCLUDED.request_id
`, command.TenantID, command.UserID, command.DeviceID, sessionID, command.Audience, issuedAt, expiresAt, command.TraceID, command.RequestID)
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
