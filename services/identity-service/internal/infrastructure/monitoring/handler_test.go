package monitoring

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestHandlerHealthz(t *testing.T) {
	handler := NewHandler(nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", response.Code)
	}
	var body healthResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode health response: %v", err)
	}
	if body.Service != serviceName || body.Status != "ok" {
		t.Fatalf("unexpected health response: %+v", body)
	}
}

func TestHandlerReadyzWithoutPool(t *testing.T) {
	handler := NewHandler(nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d", response.Code)
	}
	var body healthResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode ready response: %v", err)
	}
	if body.Status != "unready" || body.Error == "" {
		t.Fatalf("unexpected ready response: %+v", body)
	}
}

func TestHandlerMetricsWithoutPool(t *testing.T) {
	handler := NewHandler(nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/debug/metrics", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", response.Code)
	}
	var body Snapshot
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode metrics response: %v", err)
	}
	if body.Service != serviceName || body.GeneratedAtMS == 0 {
		t.Fatalf("unexpected metrics response: %+v", body)
	}
	if body.PGPool != nil || body.Identity != nil {
		t.Fatalf("nil pool should not include pg/identity metrics: %+v", body)
	}
}

func TestHandlerMetricsIncludesGRPCSnapshot(t *testing.T) {
	grpcMetrics := NewGRPCMetrics()
	grpcMetrics.record("/nexusim.identity.v1.IdentityService/IssueGatewayToken", "OK", 12)
	handler := NewHandler(nil, grpcMetrics)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/debug/metrics", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", response.Code)
	}
	var body Snapshot
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode metrics response: %v", err)
	}
	if body.GRPC == nil || body.GRPC.TotalRequests != 1 || len(body.GRPC.Methods) != 1 {
		t.Fatalf("expected grpc metrics, got %+v", body.GRPC)
	}
}

func TestHandlerJWKS(t *testing.T) {
	handler := NewHandler(nil).WithJWKSet(map[string]any{
		"keys": []map[string]string{{
			"kty": "RSA",
			"kid": "kid-1",
			"alg": "RS256",
			"n":   "modulus",
			"e":   "AQAB",
		}},
	})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/.well-known/jwks.json", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", response.Code)
	}
	var body map[string]any
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode jwks: %v", err)
	}
	keys, ok := body["keys"].([]any)
	if !ok || len(keys) != 1 {
		t.Fatalf("unexpected jwks: %+v", body)
	}
}

func TestHandlerJWKSNotConfigured(t *testing.T) {
	handler := NewHandler(nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/.well-known/jwks.json", nil))

	if response.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", response.Code)
	}
}

func TestQueryIdentitySnapshotIncludesChallengeRequestLimiterIntegration(t *testing.T) {
	pool := openMonitoringTestPool(t)
	ctx := context.Background()
	resetMonitoringIdentityTables(t, ctx, pool)
	now := time.Now().UTC()
	lockedKey := strings.Repeat("a", 64)
	unlockedKey := strings.Repeat("b", 64)
	for _, row := range []struct {
		userID      string
		targetKey   string
		lockedUntil any
	}{
		{userID: "locked-user", targetKey: lockedKey, lockedUntil: now.Add(time.Hour)},
		{userID: "unlocked-user", targetKey: unlockedKey, lockedUntil: nil},
	} {
		if _, err := pool.Exec(ctx, `
INSERT INTO identity_challenge_request_limits (
    tenant_id,
    user_id,
    challenge_type,
    channel,
    target_key,
    request_count,
    window_start,
    last_request_at,
    locked_until,
    created_at,
    updated_at
) VALUES ($1, $2, 'PASSWORD_RESET', 'EMAIL', $3, 3, $4, $4, $5, $4, $4)
`, "tenant-identity", row.userID, row.targetKey, now.Add(-time.Minute), row.lockedUntil); err != nil {
			t.Fatalf("seed challenge request limiter row: %v", err)
		}
	}

	snapshot, err := queryIdentitySnapshot(ctx, pool)
	if err != nil {
		t.Fatalf("query identity snapshot: %v", err)
	}
	if snapshot.ChallengeRequestLimits != 2 || snapshot.ChallengeRequestLimitsLocked != 1 {
		t.Fatalf("unexpected challenge request limiter metrics: %+v", snapshot)
	}

	handler := NewHandler(pool)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/debug/metrics", nil))
	body := response.Body.String()
	if strings.Contains(body, lockedKey) || strings.Contains(body, unlockedKey) {
		t.Fatalf("identity metrics leaked challenge request target key: %s", body)
	}
}

func openMonitoringTestPool(t *testing.T) *pgxpool.Pool {
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
	applyMonitoringIdentityMigrations(t, ctx, pool)
	return pool
}

func applyMonitoringIdentityMigrations(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	root := findMonitoringRepoRoot(t)
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

func resetMonitoringIdentityTables(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
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

func findMonitoringRepoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(wd, "go.mod")); err == nil {
			return wd
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			t.Fatal("repository root with go.mod not found")
		}
		wd = parent
	}
}
