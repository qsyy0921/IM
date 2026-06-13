package monitoring

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/conversation-service/internal/types"
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

func TestHandlerMetricsIncludesGRPCSnapshot(t *testing.T) {
	grpcMetrics := NewGRPCMetrics()
	grpcMetrics.record("/nexusim.conversation.v1.ConversationService/GetSendContext", "OK", 11)
	handler := NewHandler(nil, grpcMetrics).WithMemberChangeWorkerStats(func() types.MemberChangeWorkerSnapshot {
		return types.MemberChangeWorkerSnapshot{
			TotalErrors:       2,
			ConsecutiveErrors: 1,
			LastErrorAtMS:     123,
		}
	})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/debug/metrics", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", response.Code)
	}
	var body Snapshot
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode metrics response: %v", err)
	}
	if body.GRPC == nil || body.GRPC.TotalRequests != 1 || body.GRPC.TotalErrors != 0 {
		t.Fatalf("unexpected grpc snapshot: %+v", body.GRPC)
	}
	if body.MemberChangeWorker == nil || body.MemberChangeWorker.TotalErrors != 2 || body.MemberChangeWorker.ConsecutiveErrors != 1 {
		t.Fatalf("unexpected worker snapshot: %+v", body.MemberChangeWorker)
	}
}

func TestQueryConversationSnapshotIntegration(t *testing.T) {
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

	resetConversationMonitoringTables(t, ctx, pool)
	_, err = pool.Exec(ctx, `
INSERT INTO conversations (
    tenant_id, conversation_id, conversation_type, status, conversation_mode,
    fanout_mode, fanout_policy_version, member_version, permission_version, current_seq_shard
) VALUES
    ('tenant-metrics', 'conv-1', 'GROUP', 'ACTIVE', 'LOCAL_ROW_LOCK', 'WRITE_FANOUT', 1, 1, 1, 'local'),
    ('tenant-metrics', 'conv-2', 'DIRECT', 'ARCHIVED', 'LOCAL_ROW_LOCK', 'WRITE_FANOUT', 1, 2, 2, 'local'),
    ('tenant-metrics', 'conv-3', 'GROUP', 'DELETED', 'LOCAL_ROW_LOCK', 'WRITE_FANOUT', 1, 3, 3, 'local');
INSERT INTO conversation_members (
    tenant_id, conversation_id, user_id, role, status, member_version, permission_version
) VALUES
    ('tenant-metrics', 'conv-1', 'owner-1', 'OWNER', 'ACTIVE', 1, 1),
    ('tenant-metrics', 'conv-1', 'admin-1', 'ADMIN', 'ACTIVE', 1, 1),
    ('tenant-metrics', 'conv-1', 'member-left', 'MEMBER', 'LEFT', 1, 1),
    ('tenant-metrics', 'conv-2', 'member-banned', 'MEMBER', 'BANNED', 1, 1);
INSERT INTO member_change_saga (
    change_id, tenant_id, conversation_id, user_id, change_type, boundary_seq, status,
    idempotency_key, expected_member_version, command_hash, operator_id, conflict_policy,
    timeline_event_id, outbox_event_id, metadata_json, created_at, updated_at
) VALUES
    ('change-1', 'tenant-metrics', 'conv-1', 'user-a', 'JOIN', 1, 'OUTBOX_ENQUEUED', 'idem-1', 1, 'hash-1', 'owner-1', 'REJECT', 'timeline-1', 'outbox-1', '{}'::jsonb, now(), now()),
    ('change-2', 'tenant-metrics', 'conv-1', 'user-b', 'LEAVE', 2, 'DONE', 'idem-2', 1, 'hash-2', 'owner-1', 'REJECT', 'timeline-2', 'outbox-2', '{}'::jsonb, now(), now()),
    ('change-3', 'tenant-metrics', 'conv-1', 'user-c', 'REMOVE', 3, 'FAILED_COMPENSATED', 'idem-3', 1, 'hash-3', 'owner-1', 'REJECT', 'timeline-3', 'outbox-3', '{}'::jsonb, now(), now());
`)
	if err != nil {
		t.Fatalf("seed conversation metrics tables: %v", err)
	}

	snapshot, err := queryConversationSnapshot(ctx, pool)
	if err != nil {
		t.Fatalf("query conversation snapshot: %v", err)
	}
	if snapshot.Conversations == nil ||
		snapshot.Conversations.Total != 3 ||
		snapshot.Conversations.Active != 1 ||
		snapshot.Conversations.Archived != 1 ||
		snapshot.Conversations.Deleted != 1 {
		t.Fatalf("unexpected conversation snapshot: %+v", snapshot.Conversations)
	}
	if snapshot.Members == nil ||
		snapshot.Members.Total != 4 ||
		snapshot.Members.Active != 2 ||
		snapshot.Members.Left != 1 ||
		snapshot.Members.Banned != 1 {
		t.Fatalf("unexpected member snapshot: %+v", snapshot.Members)
	}
	if snapshot.MemberChanges == nil ||
		snapshot.MemberChanges.Total != 3 ||
		snapshot.MemberChanges.OutboxEnqueued != 1 ||
		snapshot.MemberChanges.Done != 1 ||
		snapshot.MemberChanges.FailedCompensated != 1 {
		t.Fatalf("unexpected member change snapshot: %+v", snapshot.MemberChanges)
	}
}

func resetConversationMonitoringTables(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(ctx, `
TRUNCATE TABLE
    member_change_saga,
    conversation_members,
    conversations
CASCADE
`); err != nil {
		t.Fatalf("truncate conversation monitoring tables: %v", err)
	}
}
