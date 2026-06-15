package monitoring

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/delivery-service/internal/types"
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

func TestHandlerMetricsIncludesTimelineProjectionWorkerSnapshot(t *testing.T) {
	handler := NewHandler(nil).WithTimelineProjectionWorkerStats(func() types.ProjectionWorkerSnapshot {
		return types.ProjectionWorkerSnapshot{
			TotalErrors:        2,
			ConsecutiveErrors:  1,
			LastErrorAtMS:      100,
			LastSuccessAtMS:    90,
			LastCommitAtMS:     90,
			LastErrorBackoffMS: 1000,
		}
	})
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/debug/metrics", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}

	var body Snapshot
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode metrics response: %v", err)
	}
	if body.TimelineProjectionWorker == nil || body.TimelineProjectionWorker.TotalErrors != 2 {
		t.Fatalf("expected timeline worker metrics, got %+v", body.TimelineProjectionWorker)
	}
}

func TestHandlerMetricsIncludesOutboxRelaySnapshot(t *testing.T) {
	handler := NewHandler(nil).WithOutboxRelayStats(func() types.OutboxRelayWorkerSnapshot {
		return types.OutboxRelayWorkerSnapshot{
			TotalErrors:        3,
			ConsecutiveErrors:  1,
			LastErrorAtMS:      100,
			LastSuccessAtMS:    90,
			LastPublishedAtMS:  80,
			LastErrorBackoffMS: 1000,
		}
	})
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/debug/metrics", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}

	var body Snapshot
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode metrics response: %v", err)
	}
	if body.OutboxRelay == nil || body.OutboxRelay.TotalErrors != 3 {
		t.Fatalf("expected outbox relay metrics, got %+v", body.OutboxRelay)
	}
}

func TestHandlerMetricsIncludesTraceSnapshot(t *testing.T) {
	handler := NewHandler(nil).WithTraceStats(func() TraceSnapshot {
		return TraceSnapshot{Enabled: true, ServiceName: serviceName, Exporter: "stdout", SamplingRatio: 1}
	})
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/debug/metrics", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}

	var body Snapshot
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode metrics response: %v", err)
	}
	if body.Trace == nil || !body.Trace.Enabled || body.Trace.ServiceName != serviceName {
		t.Fatalf("expected trace metrics, got %+v", body.Trace)
	}
}

func TestHandlerPrometheusMetrics(t *testing.T) {
	grpcMetrics := NewGRPCMetrics()
	grpcMetrics.record("/nexusim.delivery.v1.DeliveryService/PullInbox", "OK", 12)
	grpcMetrics.record("/nexusim.delivery.v1.DeliveryService/AckDelivery", "PermissionDenied", 34)
	handler := NewHandler(nil, grpcMetrics).
		WithTimelineProjectionWorkerStats(func() types.ProjectionWorkerSnapshot {
			return types.ProjectionWorkerSnapshot{
				TotalErrors:        2,
				ConsecutiveErrors:  1,
				LastErrorAtMS:      100,
				LastSuccessAtMS:    90,
				LastCommitAtMS:     80,
				LastErrorBackoffMS: 1000,
			}
		}).
		WithOutboxRelayStats(func() types.OutboxRelayWorkerSnapshot {
			return types.OutboxRelayWorkerSnapshot{
				TotalErrors:        3,
				ConsecutiveErrors:  1,
				LastErrorAtMS:      200,
				LastSuccessAtMS:    190,
				LastPublishedAtMS:  180,
				LastErrorBackoffMS: 2000,
			}
		}).
		WithTraceStats(func() TraceSnapshot {
			return TraceSnapshot{
				Enabled:         true,
				ServiceName:     serviceName,
				Exporter:        "otlp-grpc",
				OTLPEndpointSet: true,
				OTLPInsecure:    true,
				SamplingRatio:   0.5,
			}
		})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}
	if contentType := response.Header().Get("Content-Type"); !strings.Contains(contentType, "text/plain") {
		t.Fatalf("expected prometheus content type, got %q", contentType)
	}
	body := response.Body.String()
	assertContains(t, body, "nexusim_delivery_build_info{service=\"delivery-service\"} 1")
	assertContains(t, body, "nexusim_delivery_grpc_requests_total{code=\"OK\",method=\"/nexusim.delivery.v1.DeliveryService/PullInbox\"} 1")
	assertContains(t, body, "nexusim_delivery_grpc_method_errors_total{method=\"/nexusim.delivery.v1.DeliveryService/AckDelivery\"} 1")
	assertContains(t, body, "nexusim_delivery_timeline_worker_errors_total 2")
	assertContains(t, body, "nexusim_delivery_outbox_relay_errors_total 3")
	assertContains(t, body, "nexusim_delivery_otel_traces_enabled{exporter=\"otlp-grpc\"} 1")
	assertContains(t, body, "nexusim_delivery_otel_traces_sampling_ratio{exporter=\"otlp-grpc\"} 0.5")
	for _, forbidden := range []string{
		"tenant_id",
		"user_id",
		"device_id",
		"session_id",
		"request_id",
		"trace_id",
		"conversation_id",
		"message_id",
		"event_id",
		"secret-token",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("prometheus metrics leaked forbidden field %q in:\n%s", forbidden, body)
		}
	}
}

func TestRenderPrometheusIncludesDeliveryAggregates(t *testing.T) {
	body := renderPrometheus(Snapshot{
		Service: serviceName,
		Delivery: &DeliverySnapshot{
			UserInboxTotal:                 10,
			UserInboxDistinctUsers:         3,
			UserInboxDistinctConversations: 2,
			MembershipProjectionTotal:      4,
			MembershipProjectionActive:     2,
			MembershipProjectionInactive:   2,
			DeviceDeliveryCursors:          5,
			KafkaCheckpoints:               6,
			KafkaConsumerGroups:            2,
		},
		DeliveryOutbox: &DeliveryOutboxSnapshot{
			Total:            7,
			Pending:          3,
			PendingReady:     1,
			PendingScheduled: 2,
			Published:        3,
			DLQ:              1,
			MaxPendingRetry:  4,
		},
		ProjectionFailures: &ProjectionFailureSnapshot{
			Total:                2,
			DecodeFailed:         1,
			InvalidArgument:      0,
			ProjectionDependency: 1,
			DBReadFailed:         0,
			DBWriteFailed:        0,
			Unknown:              0,
			MaxFailureCount:      3,
			ResolvedTotal:        8,
		},
	})

	assertContains(t, body, "nexusim_delivery_read_model{state=\"user_inbox_total\"} 10")
	assertContains(t, body, "nexusim_delivery_membership_projection{state=\"active\"} 2")
	assertContains(t, body, "nexusim_delivery_kafka_checkpoints{state=\"consumer_groups\"} 2")
	assertContains(t, body, "nexusim_delivery_outbox{state=\"dlq\"} 1")
	assertContains(t, body, "nexusim_delivery_outbox{state=\"max_pending_retry\"} 4")
	assertContains(t, body, "nexusim_delivery_projection_failures{state=\"unresolved_total\"} 2")
	assertContains(t, body, "nexusim_delivery_projection_failures_by_class{class=\"projection_dependency\"} 1")
	assertContains(t, body, "nexusim_delivery_metrics_query_error 0")
	assertContains(t, body, "nexusim_delivery_outbox_metrics_query_error 0")
	assertContains(t, body, "nexusim_delivery_projection_failure_metrics_query_error 0")
}

func TestRenderPrometheusIncludesQueryErrorsAndEscapesLabels(t *testing.T) {
	body := renderPrometheus(Snapshot{
		Service:                 serviceName,
		DeliveryError:           "delivery metrics query failed",
		DeliveryOutboxError:     "delivery outbox metrics query failed",
		ProjectionFailuresError: "delivery projection failure metrics query failed",
		Trace:                   &TraceSnapshot{Enabled: true, Exporter: "otlp\"bad\\label\nnext", SamplingRatio: 1},
		GeneratedAtMS:           1,
	})

	assertContains(t, body, "nexusim_delivery_metrics_query_error 1")
	assertContains(t, body, "nexusim_delivery_outbox_metrics_query_error 1")
	assertContains(t, body, "nexusim_delivery_projection_failure_metrics_query_error 1")
	assertContains(t, body, "exporter=\"otlp\\\"bad\\\\label\\nnext\"")
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
	if body.ProjectionFailures == nil {
		t.Fatalf("expected projection failure snapshot")
	}
	if body.ProjectionFailures.Total != 2 ||
		body.ProjectionFailures.DecodeFailed != 1 ||
		body.ProjectionFailures.ProjectionDependency != 1 ||
		body.ProjectionFailures.MaxFailureCount != 3 ||
		body.ProjectionFailures.ResolvedTotal != 1 {
		t.Fatalf("unexpected projection failure snapshot: %+v", *body.ProjectionFailures)
	}
}

func assertContains(t *testing.T, body string, want string) {
	t.Helper()
	if !strings.Contains(body, want) {
		t.Fatalf("expected body to contain %q in:\n%s", want, body)
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
	migrationDir := filepath.Join(root, "migrations", "postgres", "delivery")
	entries, err := os.ReadDir(migrationDir)
	if err != nil {
		t.Fatalf("read migration dir: %v", err)
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".sql" {
			continue
		}
		files = append(files, entry.Name())
	}
	slices.Sort(files)
	for _, file := range files {
		sqlBytes, err := os.ReadFile(filepath.Join(migrationDir, file))
		if err != nil {
			t.Fatalf("read migration %s: %v", file, err)
		}
		if _, err := pool.Exec(ctx, string(sqlBytes)); err != nil {
			t.Fatalf("apply migration %s: %v", file, err)
		}
	}
}

func resetDeliveryTables(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(ctx, `
TRUNCATE
    delivery_projection_failures,
    delivery_projection_checkpoint_repair_audit,
    delivery_outbox_repair_audit,
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
	if _, err := pool.Exec(ctx, `
INSERT INTO delivery_projection_failures (
    consumer_group, topic, partition_id, offset_value, event_id, event_type, tenant_id, conversation_id, aggregate_version, trace_id, failure_class, last_error, failure_count, resolved_at, resolved_checkpoint_offset
) VALUES
    ('group-1', 'conversation.timeline.events', 0, 41, '', '', '', '', 0, '', 'decode_failed', 'proto: cannot parse invalid wire-format data', 1, NULL, NULL),
    ('group-1', 'conversation.timeline.events', 0, 42, 'event-2', 'message.revoked.v1', 'tenant-a', 'conv-1', 4, 'trace-2', 'projection_dependency', 'message revoke has no projected original message', 3, NULL, NULL),
    ('group-1', 'conversation.timeline.events', 0, 43, 'event-3', 'message.edited.v1', 'tenant-a', 'conv-1', 5, 'trace-3', 'db_write_failed', 'write timeout', 2, now(), 44)
`); err != nil {
		t.Fatalf("seed delivery_projection_failures: %v", err)
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
