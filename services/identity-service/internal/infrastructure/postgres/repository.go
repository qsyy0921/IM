package postgres

import (
	"context"
	"crypto/rand"
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
	pool      *pgxpool.Pool
	sessionID func() (string, error)
	eventID   func() (string, error)
}

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
SELECT tenant_id, user_id, status, password_hash
FROM identity_users
WHERE tenant_id = $1
  AND user_id = $2
`, tenantID, userID).Scan(
		&credential.TenantID,
		&credential.UserID,
		&credential.Status,
		&credential.PasswordHash,
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
	if err := upsertSession(ctx, tx, issueCommandFromLogin(command, sessionID), sessionID, issuedAt, gatewayExpiresAt); err != nil {
		return types.LoginResult{}, err
	}
	if err := insertRefreshToken(ctx, tx, command.TenantID, command.UserID, command.DeviceID, sessionID, refreshToken, issuedAt, refreshExpiresAt, command.TraceID, command.RequestID); err != nil {
		return types.LoginResult{}, err
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
		revoked, err := r.revokeSessionAfterRefreshReuse(ctx, tx, row, issuedAt)
		if err != nil {
			return types.RefreshGatewayTokenResult{}, err
		}
		if revoked != nil {
			if err := r.insertSessionRevokedOutbox(ctx, tx, *revoked, types.RevokeSessionCommand{
				AdminContext: types.AdminContext{
					TenantID:       row.TenantID,
					OperatorUserID: "identity-service",
					TraceID:        command.TraceID,
					RequestID:      command.RequestID,
				},
				UserID:    row.UserID,
				DeviceID:  row.DeviceID,
				SessionID: row.SessionID,
				Reason:    "refresh token reuse detected",
			}, issuedAt); err != nil {
				return types.RefreshGatewayTokenResult{}, err
			}
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
	if err := ensureRefreshSessionActive(ctx, tx, command.TenantID, command.UserID, command.DeviceID, row.SessionID); err != nil {
		return types.RefreshGatewayTokenResult{}, err
	}
	if err := markRefreshTokenUsed(ctx, tx, row, nextRefreshToken.TokenID, issuedAt); err != nil {
		return types.RefreshGatewayTokenResult{}, err
	}
	if err := upsertSession(ctx, tx, issueCommandFromRefresh(command, row.SessionID), row.SessionID, issuedAt, gatewayExpiresAt); err != nil {
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

func ensureRefreshSessionActive(ctx context.Context, tx pgx.Tx, tenantID types.TenantID, userID types.UserID, deviceID types.DeviceID, sessionID types.SessionID) error {
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
		return types.NewSessionNotFound("session not found")
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
