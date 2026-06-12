package postgres

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/identity-service/internal/types"
)

func TestRepositoryIssueGatewaySessionIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetIdentityTables(t, ctx, pool)
	repository := NewRepository(
		pool,
		WithSessionIDGenerator(func() (string, error) { return "session-1", nil }),
		WithEventIDGenerator(func() (string, error) { return "event-device-revoked-1", nil }),
	)

	issuedAt := time.Unix(1_800_000_000, 0).UTC()
	expiresAt := issuedAt.Add(15 * time.Minute)
	result, err := repository.IssueGatewaySession(ctx, types.IssueGatewayTokenCommand{
		TenantID:  "tenant-identity",
		UserID:    "user-1",
		DeviceID:  "device-1",
		Audience:  "push-gateway",
		TraceID:   "trace-1",
		RequestID: "request-1",
	}, issuedAt, expiresAt)
	if err != nil {
		t.Fatalf("issue session: %v", err)
	}
	if result.SessionID != "session-1" || result.ExpiresAtUnixMS != expiresAt.UnixMilli() {
		t.Fatalf("unexpected result: %+v", result)
	}
	assertDeviceStatus(t, ctx, pool, "ACTIVE")
	assertSessionStatus(t, ctx, pool, "session-1", "ACTIVE")
}

func TestRepositoryRevokeDeviceRejectsFutureIssueIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetIdentityTables(t, ctx, pool)
	repository := NewRepository(pool, WithSessionIDGenerator(func() (string, error) { return "session-1", nil }))
	issuedAt := time.Unix(1_800_000_000, 0).UTC()
	expiresAt := issuedAt.Add(15 * time.Minute)
	if _, err := repository.IssueGatewaySession(ctx, issueCommand(""), issuedAt, expiresAt); err != nil {
		t.Fatalf("issue before revoke: %v", err)
	}
	if _, err := repository.RevokeDevice(ctx, types.RevokeDeviceCommand{
		AdminContext: types.AdminContext{TenantID: "tenant-identity", OperatorUserID: "admin-1"},
		UserID:       "user-1",
		DeviceID:     "device-1",
		Reason:       "lost device",
	}, issuedAt.Add(time.Minute)); err != nil {
		t.Fatalf("revoke device: %v", err)
	}
	assertDeviceStatus(t, ctx, pool, "REVOKED")
	assertSessionStatus(t, ctx, pool, "session-1", "REVOKED")
	assertOutboxEvent(t, ctx, pool, "identity.device.revoked.v1", "identity_device", "event-device-revoked-1")
	_, err := repository.IssueGatewaySession(ctx, issueCommand("session-2"), issuedAt.Add(2*time.Minute), expiresAt.Add(2*time.Minute))
	if !errors.Is(err, types.ErrDeviceRevoked) {
		t.Fatalf("expected device revoked, got %v", err)
	}
}

func TestRepositoryRevokeSessionRejectsSameSessionIDIntegration(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	resetIdentityTables(t, ctx, pool)
	repository := NewRepository(pool, WithEventIDGenerator(func() (string, error) { return "event-session-revoked-1", nil }))
	issuedAt := time.Unix(1_800_000_000, 0).UTC()
	expiresAt := issuedAt.Add(15 * time.Minute)
	if _, err := repository.IssueGatewaySession(ctx, issueCommand("session-explicit"), issuedAt, expiresAt); err != nil {
		t.Fatalf("issue before revoke: %v", err)
	}
	if _, err := repository.RevokeSession(ctx, types.RevokeSessionCommand{
		AdminContext: types.AdminContext{TenantID: "tenant-identity", OperatorUserID: "admin-1"},
		UserID:       "user-1",
		DeviceID:     "device-1",
		SessionID:    "session-explicit",
		Reason:       "manual revoke",
	}, issuedAt.Add(time.Minute)); err != nil {
		t.Fatalf("revoke session: %v", err)
	}
	_, err := repository.IssueGatewaySession(ctx, issueCommand("session-explicit"), issuedAt.Add(2*time.Minute), expiresAt.Add(2*time.Minute))
	if !errors.Is(err, types.ErrSessionRevoked) {
		t.Fatalf("expected session revoked, got %v", err)
	}
	assertOutboxEvent(t, ctx, pool, "identity.session.revoked.v1", "identity_session", "event-session-revoked-1")
}

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
    identity_outbox,
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
