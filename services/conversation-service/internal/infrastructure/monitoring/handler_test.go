package monitoring

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
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

func TestHandlerMetricsIncludesTraceSnapshot(t *testing.T) {
	handler := NewHandler(nil).WithTraceStats(func() TraceSnapshot {
		return TraceSnapshot{
			Enabled:       true,
			ServiceName:   "conversation-service",
			Exporter:      "stdout",
			SamplingRatio: 1,
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
	if body.Trace == nil || !body.Trace.Enabled || body.Trace.ServiceName != "conversation-service" {
		t.Fatalf("unexpected trace snapshot: %+v", body.Trace)
	}
}

func TestHandlerPrometheusMetrics(t *testing.T) {
	grpcMetrics := NewGRPCMetrics()
	grpcMetrics.record("/nexusim.conversation.v1.ConversationService/GetSendContext", "OK", 11)
	grpcMetrics.record("/nexusim.conversation.v1.ConversationService/CreateMemberChange", "PermissionDenied", 23)
	handler := NewHandler(nil, grpcMetrics).
		WithMemberChangeWorkerStats(func() types.MemberChangeWorkerSnapshot {
			return types.MemberChangeWorkerSnapshot{
				TotalErrors:        2,
				ConsecutiveErrors:  1,
				LastErrorAtMS:      123,
				LastSuccessAtMS:    456,
				LastAdvancedAtMS:   789,
				LastAdvancedCount:  3,
				LastErrorBackoffMS: 500,
				LastPollIntervalMS: 1000,
			}
		}).
		WithTraceStats(func() TraceSnapshot {
			return TraceSnapshot{
				Enabled:         true,
				ServiceName:     "conversation-service",
				Exporter:        "otlp-grpc",
				OTLPEndpointSet: true,
				OTLPInsecure:    true,
				SamplingRatio:   0.25,
			}
		})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", response.Code)
	}
	if contentType := response.Header().Get("Content-Type"); !strings.Contains(contentType, "text/plain") {
		t.Fatalf("expected prometheus content type, got %q", contentType)
	}
	body := response.Body.String()
	assertContains(t, body, "nexusim_conversation_build_info{service=\"conversation-service\"} 1")
	assertContains(t, body, "nexusim_conversation_grpc_requests_total{code=\"OK\",method=\"/nexusim.conversation.v1.ConversationService/GetSendContext\"} 1")
	assertContains(t, body, "nexusim_conversation_grpc_method_errors_total{method=\"/nexusim.conversation.v1.ConversationService/CreateMemberChange\"} 1")
	assertContains(t, body, "nexusim_conversation_member_change_worker_errors_total 2")
	assertContains(t, body, "nexusim_conversation_member_change_worker_consecutive_errors 1")
	assertContains(t, body, "nexusim_conversation_otel_traces_enabled{exporter=\"otlp-grpc\"} 1")
	assertContains(t, body, "nexusim_conversation_otel_traces_sampling_ratio{exporter=\"otlp-grpc\"} 0.25")
	for _, forbidden := range []string{
		"tenant_id",
		"user_id",
		"device_id",
		"session_id",
		"request_id",
		"trace_id",
		"conversation_id",
		"message_id",
		"target_user_id",
		"secret-token",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("prometheus metrics leaked forbidden field %q in:\n%s", forbidden, body)
		}
	}
}

func TestRenderPrometheusIncludesConversationAggregates(t *testing.T) {
	body := renderPrometheus(Snapshot{
		Service: serviceName,
		Conversation: &ConversationSnapshot{
			Conversations: &ConversationStoreSnapshot{
				Total:    3,
				Active:   1,
				Archived: 1,
				Deleted:  1,
				ByType: []GroupCountSnapshot{
					{Value: "GROUP", Total: 2},
					{Value: "DIRECT", Total: 1},
				},
				ByStatus: []GroupCountSnapshot{
					{Value: "ACTIVE", Total: 1},
					{Value: "ARCHIVED", Total: 1},
					{Value: "DELETED", Total: 1},
				},
			},
			Members: &ConversationMemberSnapshot{
				Total:  4,
				Active: 2,
				Left:   1,
				Banned: 1,
				ByRole: []GroupCountSnapshot{
					{Value: "OWNER", Total: 1},
					{Value: "ADMIN", Total: 1},
					{Value: "MEMBER", Total: 2},
				},
				ByStatus: []GroupCountSnapshot{
					{Value: "ACTIVE", Total: 2},
					{Value: "LEFT", Total: 1},
					{Value: "BANNED", Total: 1},
				},
				ByRoleStatus: []RoleStatusCount{
					{Role: "OWNER", Status: "ACTIVE", Total: 1},
					{Role: "MEMBER", Status: "LEFT", Total: 1},
				},
			},
			MemberChanges: &MemberChangeSagaSnapshot{
				Total:             3,
				OutboxEnqueued:    1,
				Done:              1,
				FailedCompensated: 1,
				ByStatus: []GroupCountSnapshot{
					{Value: "OUTBOX_ENQUEUED", Total: 1},
					{Value: "DONE", Total: 1},
					{Value: "FAILED_COMPENSATED", Total: 1},
				},
			},
		},
	})

	assertContains(t, body, "nexusim_conversation_conversations{state=\"active\"} 1")
	assertContains(t, body, "nexusim_conversation_conversations_by_type{type=\"GROUP\"} 2")
	assertContains(t, body, "nexusim_conversation_members{state=\"banned\"} 1")
	assertContains(t, body, "nexusim_conversation_members_by_role_status{role=\"MEMBER\",status=\"LEFT\"} 1")
	assertContains(t, body, "nexusim_conversation_member_changes{state=\"outbox_enqueued\"} 1")
	assertContains(t, body, "nexusim_conversation_member_changes_by_status{status=\"FAILED_COMPENSATED\"} 1")
	assertContains(t, body, "nexusim_conversation_metrics_query_error 0")
}

func TestRenderPrometheusEscapesLabelsAndQueryError(t *testing.T) {
	body := renderPrometheus(Snapshot{
		Service:           serviceName,
		ConversationError: "conversation metrics query failed",
		Conversation: &ConversationSnapshot{
			Conversations: &ConversationStoreSnapshot{
				ByType: []GroupCountSnapshot{{Value: "GROUP\"bad\\label\nnext", Total: 1}},
			},
		},
	})

	assertContains(t, body, "nexusim_conversation_metrics_query_error 1")
	assertContains(t, body, "type=\"GROUP\\\"bad\\\\label\\nnext\"")
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

func assertContains(t *testing.T, body string, want string) {
	t.Helper()
	if !strings.Contains(body, want) {
		t.Fatalf("expected body to contain %q in:\n%s", want, body)
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
