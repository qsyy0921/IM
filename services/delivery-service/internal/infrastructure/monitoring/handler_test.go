package monitoring

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestHandlerHealthz(t *testing.T) {
	handler := NewHandler(nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}
}

func TestHandlerReadyzWithoutPool(t *testing.T) {
	handler := NewHandler(nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", response.Code)
	}
}

func TestHandlerMetricsIncludesGRPCSnapshot(t *testing.T) {
	grpcMetrics := NewGRPCMetrics()
	grpcMetrics.record("/nexusim.delivery.v1.DeliveryService/PullInbox", "OK", 12)
	handler := NewHandler(nil, grpcMetrics)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/debug/metrics", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}

	var body Snapshot
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode metrics response: %v", err)
	}
	if body.GRPC == nil || body.GRPC.TotalRequests != 1 || len(body.GRPC.Methods) != 1 {
		t.Fatalf("unexpected grpc metrics snapshot: %+v", body.GRPC)
	}
}

func TestHandlerMetricsIncludesDeliverySnapshotsIntegration(t *testing.T) {
	pool := openMonitoringTestPool(t)
	ctx := context.Background()
	resetDeliveryTables(t, ctx, pool)

	seedDeliveryMetricsRows(t, ctx, pool)

	handler := NewHandler(pool)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/debug/metrics", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}

	var body Snapshot
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode metrics response: %v", err)
	}
	if body.Delivery == nil {
		t.Fatalf("expected delivery snapshot")
	}
	if body.Delivery.UserInboxTotal != 2 ||
		body.Delivery.UserInboxDistinctUsers != 2 ||
		body.Delivery.MembershipProjectionTotal != 2 ||
		body.Delivery.MembershipProjectionActive != 1 ||
		body.Delivery.MembershipProjectionInactive != 1 ||
		body.Delivery.DeviceDeliveryCursors != 1 ||
		body.Delivery.KafkaCheckpoints != 2 ||
		body.Delivery.KafkaConsumerGroups != 2 {
		t.Fatalf("unexpected delivery snapshot: %+v", *body.Delivery)
	}
	if body.DeliveryOutbox == nil {
		t.Fatalf("expected delivery outbox snapshot")
	}
	if body.DeliveryOutbox.Total != 3 ||
		body.DeliveryOutbox.Pending != 2 ||
		body.DeliveryOutbox.PendingReady != 1 ||
		body.DeliveryOutbox.PendingScheduled != 1 ||
		body.DeliveryOutbox.Published != 0 ||
		body.DeliveryOutbox.DLQ != 1 ||
		body.DeliveryOutbox.MaxPendingRetry != 2 {
		t.Fatalf("unexpected delivery outbox snapshot: %+v", *body.DeliveryOutbox)
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
	applyDeliveryMigration(t, ctx, pool)
	return pool
}

func applyDeliveryMigration(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	root := findRepoRoot(t)
	migrationPath := filepath.Join(root, "migrations", "postgres", "delivery", "000001_delivery_core.sql")
	sqlBytes, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(sqlBytes)); err != nil {
		t.Fatalf("apply migration: %v", err)
	}
}

func resetDeliveryTables(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(ctx, `
TRUNCATE
    delivery_outbox,
    device_delivery_cursors,
    user_inbox,
    delivery_membership_projection,
    delivery_kafka_checkpoints
RESTART IDENTITY
`)
	if err != nil {
		t.Fatalf("reset delivery tables: %v", err)
	}
}

func seedDeliveryMetricsRows(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(ctx, `
INSERT INTO user_inbox (
    tenant_id, user_id, conversation_id, conversation_seq, event_id, event_type, message_id, sender_id, payload_json, fanout_mode, permission_version
) VALUES
    ('tenant-a', 'user-1', 'conv-1', 2, 'event-1', 'message.persisted.v1', 'msg-1', 'sender-1', '{}'::jsonb, 'ALL_MEMBERS', 1),
    ('tenant-a', 'user-2', 'conv-1', 2, 'event-2', 'message.persisted.v1', 'msg-1', 'sender-1', '{}'::jsonb, 'ALL_MEMBERS', 1)
`); err != nil {
		t.Fatalf("seed user_inbox: %v", err)
	}
	if _, err := pool.Exec(ctx, `
INSERT INTO delivery_membership_projection (
    tenant_id, conversation_id, user_id, role, status, join_seq, leave_seq, member_version, permission_version, updated_by_event_id
) VALUES
    ('tenant-a', 'conv-1', 'user-1', 'MEMBER', 'ACTIVE', 1, NULL, 1, 1, 'member-1'),
    ('tenant-a', 'conv-1', 'user-2', 'MEMBER', 'LEFT', 1, 3, 2, 2, 'member-2')
`); err != nil {
		t.Fatalf("seed delivery_membership_projection: %v", err)
	}
	if _, err := pool.Exec(ctx, `
INSERT INTO device_delivery_cursors (
    tenant_id, user_id, device_id, conversation_id, last_received_seq
) VALUES
    ('tenant-a', 'user-1', 'device-1', 'conv-1', 2)
`); err != nil {
		t.Fatalf("seed device_delivery_cursors: %v", err)
	}
	if _, err := pool.Exec(ctx, `
INSERT INTO delivery_kafka_checkpoints (
    consumer_group, topic, partition_id, offset_value
) VALUES
    ('group-1', 'conversation.timeline.events', 0, 12),
    ('group-2', 'conversation.timeline.events', 1, 34)
`); err != nil {
		t.Fatalf("seed delivery_kafka_checkpoints: %v", err)
	}
	if _, err := pool.Exec(ctx, `
INSERT INTO delivery_outbox (
    event_id, tenant_id, conversation_id, aggregate_version, event_type, event_version, partition_key, mapping_version, producer, payload_json, status, retry_count, available_at, next_retry_at
) VALUES
    ('outbox-1', 'tenant-a', 'conv-1', 2, 'delivery.inbox_item.created.v1', 'v1', 'tenant-a:conv-1', 1, 'delivery-service', '{}'::jsonb, 'PENDING', 2, now() - interval '1 minute', NULL),
    ('outbox-2', 'tenant-a', 'conv-1', 3, 'delivery.ack.recorded.v1', 'v1', 'tenant-a:conv-1', 1, 'delivery-service', '{}'::jsonb, 'PENDING', 1, now() - interval '1 minute', now() + interval '5 minutes'),
    ('outbox-3', 'tenant-a', 'conv-1', 4, 'delivery.inbox_item.created.v1', 'v1', 'tenant-a:conv-1', 1, 'delivery-service', '{}'::jsonb, 'DLQ', 4, now() - interval '1 minute', NULL)
`); err != nil {
		t.Fatalf("seed delivery_outbox: %v", err)
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
