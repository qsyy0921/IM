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

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestHandlerHealthz(t *testing.T) {
	handler := NewHandler(nil, false, nil, nil)
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

func TestHandlerReadyzWithoutRequiredPool(t *testing.T) {
	handler := NewHandler(nil, false, nil, nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", response.Code)
	}
	var body healthResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode ready response: %v", err)
	}
	if body.Status != "ready" {
		t.Fatalf("unexpected ready response: %+v", body)
	}
}

func TestHandlerReadyzWithRequiredMissingPool(t *testing.T) {
	handler := NewHandler(nil, true, nil, nil)
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

func TestHandlerMetricsIncludesGRPCAndDecisionSnapshots(t *testing.T) {
	grpcMetrics := NewGRPCMetrics()
	grpcMetrics.record("/nexusim.policy.v1.PolicyService/CheckMessageAction", "OK", 12)
	decisionMetrics := NewDecisionMetrics()
	decisionMetrics.Record("SEND", true, false, 7)
	decisionMetrics.Record("DELETE", false, false, 5)
	handler := NewHandler(nil, false, grpcMetrics, decisionMetrics)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/debug/metrics", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", response.Code)
	}
	bodyRaw := response.Body.String()
	if strings.Contains(bodyRaw, "tenant-") ||
		strings.Contains(bodyRaw, "policy-message-user") ||
		strings.Contains(bodyRaw, "conversation") ||
		strings.Contains(bodyRaw, "message_id") {
		t.Fatalf("metrics should not expose high-cardinality identity fields: %s", bodyRaw)
	}
	var body Snapshot
	if err := json.Unmarshal([]byte(bodyRaw), &body); err != nil {
		t.Fatalf("decode metrics response: %v", err)
	}
	if body.GRPC == nil || body.GRPC.TotalRequests != 1 {
		t.Fatalf("expected grpc metrics, got %+v", body.GRPC)
	}
	if body.Decisions == nil || body.Decisions.Total != 2 || body.Decisions.Allowed != 1 || body.Decisions.Denied != 1 {
		t.Fatalf("expected decision metrics, got %+v", body.Decisions)
	}
}

func TestQueryRuleSnapshotIncludesAllPolicyRuleStoresIntegration(t *testing.T) {
	dsn := os.Getenv("NEXUSIM_PG_DSN")
	if dsn == "" {
		t.Skip("NEXUSIM_PG_DSN is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("open pg pool: %v", err)
	}
	defer pool.Close()
	applyPolicyMonitoringMigrations(t, ctx, pool)
	if _, err := pool.Exec(ctx, `
TRUNCATE
    policy_message_ownership_override_rules,
    policy_conversation_role_action_rules,
    policy_tenant_message_action_rules,
    policy_message_action_rules
`); err != nil {
		t.Fatalf("truncate policy rule tables: %v", err)
	}
	if _, err := pool.Exec(ctx, `
INSERT INTO policy_message_action_rules (
    tenant_id, user_id, conversation_id, action, allowed, permission_version, classification
) VALUES
    ('tenant-metrics', 'user-1', 'conv-1', 'SEND', true, 1, 'EXACT_ALLOW'),
    ('tenant-metrics', 'user-2', 'conv-1', 'DELETE', false, 1, 'EXACT_DENY');
INSERT INTO policy_tenant_message_action_rules (
    tenant_id, action, allowed, permission_version, classification
) VALUES
    ('tenant-metrics', 'SEND', true, 1, 'TENANT_ALLOW'),
    ('tenant-metrics', 'EDIT', false, 1, 'TENANT_DENY');
INSERT INTO policy_conversation_role_action_rules (
    tenant_id, action, min_role, classification
) VALUES
    ('tenant-metrics', 'SEND', 'MEMBER', 'ROLE_MEMBER'),
    ('tenant-metrics', 'DELETE', 'ADMIN', 'ROLE_ADMIN');
INSERT INTO policy_message_ownership_override_rules (
    tenant_id, action, min_role, classification
) VALUES
    ('tenant-metrics', 'EDIT', 'ADMIN', 'OWNERSHIP_ADMIN'),
    ('tenant-metrics', 'DELETE', 'OWNER', 'OWNERSHIP_OWNER');
`); err != nil {
		t.Fatalf("seed policy rule tables: %v", err)
	}

	snapshot, err := queryRuleSnapshot(ctx, pool)
	if err != nil {
		t.Fatalf("query rule snapshot: %v", err)
	}
	if snapshot.Total != 2 || snapshot.Allow != 1 || snapshot.Deny != 1 || snapshot.ExactMessageActions == nil {
		t.Fatalf("unexpected exact rule snapshot: %+v", snapshot)
	}
	if snapshot.TenantMessageActions == nil ||
		snapshot.TenantMessageActions.Total != 2 ||
		snapshot.TenantMessageActions.Allow != 1 ||
		snapshot.TenantMessageActions.Deny != 1 {
		t.Fatalf("unexpected tenant rule snapshot: %+v", snapshot.TenantMessageActions)
	}
	if snapshot.ConversationRoleActions == nil || snapshot.ConversationRoleActions.Total != 2 || len(snapshot.ConversationRoleActions.Actions) != 2 {
		t.Fatalf("unexpected role rule snapshot: %+v", snapshot.ConversationRoleActions)
	}
	if snapshot.OwnershipOverrides == nil || snapshot.OwnershipOverrides.Total != 2 || len(snapshot.OwnershipOverrides.Actions) != 2 {
		t.Fatalf("unexpected ownership override snapshot: %+v", snapshot.OwnershipOverrides)
	}
}

func applyPolicyMonitoringMigrations(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	for _, name := range []string{
		"000001_policy_core.sql",
		"000007_policy_tenant_message_action_rules.sql",
		"000008_policy_conversation_role_rules.sql",
		"000009_policy_message_ownership_override_rules.sql",
	} {
		path := filepath.Join("..", "..", "..", "..", "..", "migrations", "postgres", "policy", name)
		statement, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read migration %s: %v", name, err)
		}
		if _, err := pool.Exec(ctx, string(statement)); err != nil {
			t.Fatalf("apply migration %s: %v", name, err)
		}
	}
}
