package postgres

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/identity-service/internal/types"
)

func issueCommand(sessionID types.SessionID) types.IssueGatewayTokenCommand {
	return types.IssueGatewayTokenCommand{
		TenantID:  "tenant-identity",
		UserID:    "user-1",
		DeviceID:  "device-1",
		SessionID: sessionID,
		Audience:  "push-gateway",
		TraceID:   "trace-1",
		RequestID: "request-1",
	}
}

func seedUserCredential(t *testing.T, ctx context.Context, pool *pgxpool.Pool, passwordHash string) {
	t.Helper()
	_, err := pool.Exec(ctx, `
INSERT INTO identity_users (tenant_id, user_id, status, password_hash, password_updated_at, created_at, updated_at)
VALUES ('tenant-identity', 'user-1', 'ACTIVE', $1, now(), now(), now())
ON CONFLICT (tenant_id, user_id) DO UPDATE
SET status = 'ACTIVE',
    password_hash = EXCLUDED.password_hash,
    password_updated_at = EXCLUDED.password_updated_at,
    updated_at = EXCLUDED.updated_at
`, passwordHash)
	if err != nil {
		t.Fatalf("seed user credential: %v", err)
	}
}

func seedVerifiedEmail(t *testing.T, ctx context.Context, pool *pgxpool.Pool, email string, verifiedAt time.Time) {
	t.Helper()
	_, err := pool.Exec(ctx, `
UPDATE identity_users
SET email = $3,
    email_verified_at = $4,
    updated_at = $4
WHERE tenant_id = $1
  AND user_id = $2
`, "tenant-identity", "user-1", email, verifiedAt)
	if err != nil {
		t.Fatalf("seed verified email: %v", err)
	}
}

func assertDeviceStatus(t *testing.T, ctx context.Context, pool *pgxpool.Pool, want string) {
	t.Helper()
	var got string
	err := pool.QueryRow(ctx, `
SELECT status
FROM identity_devices
WHERE tenant_id = 'tenant-identity'
  AND user_id = 'user-1'
  AND device_id = 'device-1'
`).Scan(&got)
	if err != nil {
		t.Fatalf("read device status: %v", err)
	}
	if got != want {
		t.Fatalf("expected device status %s, got %s", want, got)
	}
}

func assertSessionStatus(t *testing.T, ctx context.Context, pool *pgxpool.Pool, sessionID string, want string) {
	t.Helper()
	var got string
	err := pool.QueryRow(ctx, `
SELECT status
FROM identity_sessions
WHERE tenant_id = 'tenant-identity'
  AND user_id = 'user-1'
  AND device_id = 'device-1'
  AND session_id = $1
`, sessionID).Scan(&got)
	if err != nil {
		t.Fatalf("read session status: %v", err)
	}
	if got != want {
		t.Fatalf("expected session status %s, got %s", want, got)
	}
}

func assertSessionMissing(t *testing.T, ctx context.Context, pool *pgxpool.Pool, sessionID string) {
	t.Helper()
	var count int
	err := pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM identity_sessions
WHERE tenant_id = 'tenant-identity'
  AND user_id = 'user-1'
  AND device_id = 'device-1'
  AND session_id = $1
`, sessionID).Scan(&count)
	if err != nil {
		t.Fatalf("read session count: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected session %s to be missing, got count %d", sessionID, count)
	}
}

func assertRefreshTokenStatus(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tokenID string, want string) {
	t.Helper()
	var got string
	err := pool.QueryRow(ctx, `
SELECT status
FROM identity_refresh_tokens
WHERE tenant_id = 'tenant-identity'
  AND user_id = 'user-1'
  AND device_id = 'device-1'
  AND token_id = $1
`, tokenID).Scan(&got)
	if err != nil {
		t.Fatalf("read refresh token status: %v", err)
	}
	if got != want {
		t.Fatalf("expected refresh token status %s, got %s", want, got)
	}
}

func assertRefreshTokenMissing(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tokenID string) {
	t.Helper()
	var count int
	err := pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM identity_refresh_tokens
WHERE token_id = $1
`, tokenID).Scan(&count)
	if err != nil {
		t.Fatalf("read refresh token count: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected refresh token %s to be missing, got count %d", tokenID, count)
	}
}

func assertSessionMFAProof(t *testing.T, ctx context.Context, pool *pgxpool.Pool, sessionID string, wantMethod string, wantFactorID types.MFAFactorID) {
	t.Helper()
	var verifiedAt time.Time
	var method string
	var factorID types.MFAFactorID
	err := pool.QueryRow(ctx, `
SELECT COALESCE(mfa_verified_at, 'epoch'::timestamptz), mfa_method, mfa_factor_id
FROM identity_sessions
WHERE tenant_id = 'tenant-identity'
  AND user_id = 'user-1'
  AND device_id = 'device-1'
  AND session_id = $1
`, sessionID).Scan(&verifiedAt, &method, &factorID)
	if err != nil {
		t.Fatalf("read session mfa proof: %v", err)
	}
	if method != wantMethod || factorID != wantFactorID {
		t.Fatalf("expected session mfa proof method=%q factor=%q, got method=%q factor=%q", wantMethod, wantFactorID, method, factorID)
	}
	emptyTime := time.Unix(0, 0).UTC()
	if wantMethod == "" && !verifiedAt.Equal(emptyTime) {
		t.Fatalf("expected empty mfa verified time, got %s", verifiedAt)
	}
	if wantMethod != "" && verifiedAt.Equal(emptyTime) {
		t.Fatal("expected mfa verified time to be set")
	}
}

func insertSessionProof(ctx context.Context, pool *pgxpool.Pool, sessionID string, method string, factorID string, verifiedAt any, now time.Time) error {
	_, err := pool.Exec(ctx, `
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
    mfa_factor_id
) VALUES ('tenant-identity', 'user-1', 'device-1', $1, 'ACTIVE', 'push-gateway', $2, $3, $4, $5, $6)
`, sessionID, now, now.Add(15*time.Minute), verifiedAt, method, factorID)
	return err
}

func assertLoginRisk(t *testing.T, ctx context.Context, pool *pgxpool.Pool, wantFailedCount int, wantLocked bool) {
	t.Helper()
	var failedCount int
	var lockedUntil *time.Time
	err := pool.QueryRow(ctx, `
SELECT failed_login_count, locked_until
FROM identity_users
WHERE tenant_id = 'tenant-identity'
  AND user_id = 'user-1'
`).Scan(&failedCount, &lockedUntil)
	if err != nil {
		t.Fatalf("read login risk: %v", err)
	}
	if failedCount != wantFailedCount {
		t.Fatalf("expected failed login count %d, got %d", wantFailedCount, failedCount)
	}
	if wantLocked && lockedUntil == nil {
		t.Fatal("expected account to be locked")
	}
	if !wantLocked && lockedUntil != nil {
		t.Fatalf("expected account to be unlocked, got locked_until=%s", lockedUntil)
	}
}

func assertMFARecoveryLoginRisk(t *testing.T, ctx context.Context, pool *pgxpool.Pool, wantFailedCount int, wantLocked bool) {
	t.Helper()
	var failedCount int
	var lockedUntil *time.Time
	err := pool.QueryRow(ctx, `
SELECT mfa_recovery_failed_count, mfa_recovery_locked_until
FROM identity_users
WHERE tenant_id = 'tenant-identity'
  AND user_id = 'user-1'
`).Scan(&failedCount, &lockedUntil)
	if err != nil {
		t.Fatalf("read mfa recovery login risk: %v", err)
	}
	if failedCount != wantFailedCount {
		t.Fatalf("expected mfa recovery failed count %d, got %d", wantFailedCount, failedCount)
	}
	if wantLocked && lockedUntil == nil {
		t.Fatal("expected mfa recovery login to be locked")
	}
	if !wantLocked && lockedUntil != nil {
		t.Fatalf("expected mfa recovery login to be unlocked, got mfa_recovery_locked_until=%s", lockedUntil)
	}
}

type challengeDeliveryState struct {
	Status               string
	DeliveryStatus       string
	DeliveryAttemptCount int
	DeliveredAt          *time.Time
	DeliveryFailedAt     *time.Time
	DeliveryLastError    string
	DeliveryFailureClass string
}

func readChallengeDeliveryState(t *testing.T, ctx context.Context, pool *pgxpool.Pool, challengeID string) challengeDeliveryState {
	t.Helper()
	var state challengeDeliveryState
	err := pool.QueryRow(ctx, `
SELECT
    status,
    delivery_status,
    delivery_attempt_count,
    delivered_at,
    delivery_failed_at,
    delivery_last_error,
    delivery_failure_class
FROM identity_challenges
WHERE tenant_id = 'tenant-identity'
  AND user_id = 'user-1'
  AND challenge_id = $1
`, challengeID).Scan(
		&state.Status,
		&state.DeliveryStatus,
		&state.DeliveryAttemptCount,
		&state.DeliveredAt,
		&state.DeliveryFailedAt,
		&state.DeliveryLastError,
		&state.DeliveryFailureClass,
	)
	if err != nil {
		t.Fatalf("read challenge delivery state: %v", err)
	}
	return state
}

func assertOutboxEvent(t *testing.T, ctx context.Context, pool *pgxpool.Pool, eventType string, aggregateType string, eventID string) {
	t.Helper()
	var gotEventID, gotEventType, gotAggregateType, gotStatus string
	var payloadTenantID, payloadUserID, payloadDeviceID string
	err := pool.QueryRow(ctx, `
SELECT
    event_id,
    event_type,
    aggregate_type,
    status,
    payload_json->>'tenant_id',
    payload_json->>'user_id',
    payload_json->>'device_id'
FROM identity_outbox
WHERE event_id = $1
`, eventID).Scan(
		&gotEventID,
		&gotEventType,
		&gotAggregateType,
		&gotStatus,
		&payloadTenantID,
		&payloadUserID,
		&payloadDeviceID,
	)
	if err != nil {
		t.Fatalf("read identity outbox event: %v", err)
	}
	if gotEventID != eventID || gotEventType != eventType || gotAggregateType != aggregateType || gotStatus != "PENDING" {
		t.Fatalf("unexpected outbox event: id=%s type=%s aggregate=%s status=%s", gotEventID, gotEventType, gotAggregateType, gotStatus)
	}
	if payloadTenantID != "tenant-identity" || payloadUserID != "user-1" || payloadDeviceID != "device-1" {
		t.Fatalf("unexpected outbox payload tenant=%s user=%s device=%s", payloadTenantID, payloadUserID, payloadDeviceID)
	}
}

func assertEmailVerified(t *testing.T, ctx context.Context, pool *pgxpool.Pool, email string, wantVerified bool) {
	t.Helper()
	var gotEmail string
	var verified bool
	err := pool.QueryRow(ctx, `
SELECT email, email_verified_at IS NOT NULL
FROM identity_users
WHERE tenant_id = 'tenant-identity'
  AND user_id = 'user-1'
`).Scan(&gotEmail, &verified)
	if err != nil {
		t.Fatalf("read identity user email state: %v", err)
	}
	if gotEmail != email || verified != wantVerified {
		t.Fatalf("unexpected email state: email=%s verified=%v", gotEmail, verified)
	}
}

func assertMFALoginRisk(t *testing.T, ctx context.Context, pool *pgxpool.Pool, factorID string, wantFailedCount int, wantLocked bool, wantLastUsed bool) {
	t.Helper()
	var failedCount int
	var lockedUntil *time.Time
	var lastUsedAt *time.Time
	err := pool.QueryRow(ctx, `
SELECT login_failed_count, login_locked_until, last_used_at
FROM identity_mfa_factors
WHERE tenant_id = 'tenant-identity'
  AND user_id = 'user-1'
  AND factor_id = $1
`, factorID).Scan(&failedCount, &lockedUntil, &lastUsedAt)
	if err != nil {
		t.Fatalf("read mfa login risk: %v", err)
	}
	if failedCount != wantFailedCount {
		t.Fatalf("expected mfa failed count %d, got %d", wantFailedCount, failedCount)
	}
	if wantLocked && lockedUntil == nil {
		t.Fatal("expected mfa factor to be locked")
	}
	if !wantLocked && lockedUntil != nil {
		t.Fatalf("expected mfa factor to be unlocked, got login_locked_until=%s", lockedUntil)
	}
	if wantLastUsed && lastUsedAt == nil {
		t.Fatal("expected mfa factor last_used_at to be set")
	}
	if !wantLastUsed && lastUsedAt != nil {
		t.Fatalf("expected mfa factor last_used_at to be empty, got %s", lastUsedAt)
	}
}

func readMFASecret(t *testing.T, ctx context.Context, pool *pgxpool.Pool, factorID string) types.MFAFactorSecret {
	t.Helper()
	var row types.MFAFactorSecret
	err := pool.QueryRow(ctx, `
SELECT tenant_id, user_id, factor_id, factor_type, status, secret_ciphertext, secret_nonce, secret_key_version
FROM identity_mfa_factors
WHERE tenant_id = 'tenant-identity'
  AND user_id = 'user-1'
  AND factor_id = $1
`, factorID).Scan(
		&row.TenantID,
		&row.UserID,
		&row.FactorID,
		&row.Type,
		&row.Status,
		&row.Secret.Ciphertext,
		&row.Secret.Nonce,
		&row.Secret.KeyVersion,
	)
	if err != nil {
		t.Fatalf("read mfa secret: %v", err)
	}
	return row
}

func openTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("NEXUSIM_PG_DSN")
	if dsn == "" {
		t.Skip("NEXUSIM_PG_DSN is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("open pg pool: %v", err)
	}
	t.Cleanup(pool.Close)
	applyIdentityMigration(t, ctx, pool)
	return pool
}

func applyIdentityMigration(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	root := findRepoRoot(t)
	migrationFiles, err := filepath.Glob(filepath.Join(root, "migrations", "postgres", "identity", "*.sql"))
	if err != nil {
		t.Fatalf("find migrations: %v", err)
	}
	for _, migrationPath := range migrationFiles {
		sqlBytes, err := os.ReadFile(migrationPath)
		if err != nil {
			t.Fatalf("read migration %s: %v", migrationPath, err)
		}
		if _, err := pool.Exec(ctx, string(sqlBytes)); err != nil {
			t.Fatalf("apply migration %s: %v", migrationPath, err)
		}
	}
}

func resetIdentityTables(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(ctx, `
TRUNCATE
    identity_challenge_delivery_repair_audit,
    identity_challenge_delivery_outbox,
    identity_challenge_request_limits,
    identity_mfa_recovery_codes,
    identity_mfa_factors,
    identity_challenges,
    identity_outbox,
    identity_refresh_tokens,
    identity_sessions,
    identity_devices,
    identity_users
RESTART IDENTITY
`)
	if err != nil {
		t.Fatalf("reset identity tables: %v", err)
	}
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(wd, "go.mod")); err == nil {
			return wd
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			t.Fatal("repo root not found")
		}
		wd = parent
	}
}
