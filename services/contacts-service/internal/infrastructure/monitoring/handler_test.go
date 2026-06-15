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
	"github.com/qsyy0921/IM/services/contacts-service/internal/types"
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
	if body.PGPool != nil || body.Contacts != nil || body.Outbox != nil {
		t.Fatalf("nil pool should not include pg/contacts/outbox metrics: %+v", body)
	}
}

func TestHandlerMetricsIncludesGRPCSnapshot(t *testing.T) {
	grpcMetrics := NewGRPCMetrics()
	grpcMetrics.record("/nexusim.contacts.v1.ContactsService/ListContacts", "OK", 12)
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

func TestHandlerMetricsIncludesOutboxRelaySnapshot(t *testing.T) {
	handler := NewHandler(nil).WithOutboxRelayStats(func() types.OutboxRelayWorkerSnapshot {
		return types.OutboxRelayWorkerSnapshot{
			TotalErrors:        2,
			ConsecutiveErrors:  1,
			LastErrorAtMS:      100,
			LastSuccessAtMS:    90,
			LastPublishedAtMS:  90,
			LastErrorBackoffMS: 1000,
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
	if body.OutboxRelay == nil || body.OutboxRelay.TotalErrors != 2 {
		t.Fatalf("expected outbox relay snapshot, got %+v", body.OutboxRelay)
	}
}

func TestHandlerMetricsIncludesTraceSnapshot(t *testing.T) {
	handler := NewHandler(nil).WithTraceStats(func() TraceSnapshot {
		return TraceSnapshot{Enabled: true, ServiceName: serviceName, Exporter: "stdout", SamplingRatio: 1}
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
	if body.Trace == nil || !body.Trace.Enabled || body.Trace.ServiceName != serviceName || body.Trace.Exporter != "stdout" {
		t.Fatalf("expected trace snapshot, got %+v", body.Trace)
	}
}

func TestHandlerPrometheusMetricsWithoutPool(t *testing.T) {
	grpcMetrics := NewGRPCMetrics()
	grpcMetrics.record("/nexusim.contacts.v1.ContactsService/ListContacts", "OK", 12)
	grpcMetrics.record("/nexusim.contacts.v1.ContactsService/ListContacts", "PermissionDenied", 20)
	handler := NewHandler(nil, grpcMetrics).
		WithOutboxRelayStats(func() types.OutboxRelayWorkerSnapshot {
			return types.OutboxRelayWorkerSnapshot{
				TotalErrors:        2,
				ConsecutiveErrors:  1,
				LastErrorAtMS:      100,
				LastSuccessAtMS:    90,
				LastPublishedAtMS:  80,
				LastErrorBackoffMS: 1000,
			}
		}).
		WithTraceStats(func() TraceSnapshot {
			return TraceSnapshot{Enabled: true, ServiceName: serviceName, Exporter: "otlp-grpc", OTLPEndpointSet: true, OTLPInsecure: true, SamplingRatio: 0.25}
		})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", response.Code)
	}
	if contentType := response.Header().Get("Content-Type"); !strings.Contains(contentType, "text/plain") {
		t.Fatalf("expected prometheus text content type, got %q", contentType)
	}
	body := response.Body.String()
	for _, expected := range []string{
		`nexusim_contacts_build_info{service="contacts-service"} 1`,
		`nexusim_contacts_grpc_method_requests_total{method="/nexusim.contacts.v1.ContactsService/ListContacts"} 2`,
		`nexusim_contacts_grpc_requests_total{code="PermissionDenied",method="/nexusim.contacts.v1.ContactsService/ListContacts"} 1`,
		`nexusim_contacts_grpc_method_errors_total{method="/nexusim.contacts.v1.ContactsService/ListContacts"} 1`,
		`nexusim_contacts_outbox_relay_errors_total 2`,
		`nexusim_contacts_outbox_relay_consecutive_errors 1`,
		`nexusim_contacts_otel_traces_enabled{exporter="otlp-grpc"} 1`,
		`nexusim_contacts_otel_traces_sampling_ratio{exporter="otlp-grpc"} 0.25`,
	} {
		assertContains(t, body, expected)
	}
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
		"remark",
		"command_hash",
		"secret-token",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("prometheus metrics should not contain sensitive or high-cardinality field %q: %s", forbidden, body)
		}
	}
}

func TestRenderPrometheusIncludesContactsAggregates(t *testing.T) {
	oldestPending := int64(1200)
	oldestDLQ := int64(3400)
	body := renderPrometheus(Snapshot{
		Contacts: &ContactsSnapshot{
			Requests: &ContactRequestSnapshot{
				Total:    7,
				Pending:  2,
				Accepted: 1,
				Declined: 1,
				Canceled: 1,
				Expired:  2,
				ByStatus: []GroupCountSnapshot{
					{Value: "ACCEPTED", Total: 1},
					{Value: "PENDING", Total: 2},
				},
			},
			Edges: &ContactEdgeSnapshot{
				Total:      5,
				Active:     2,
				Deleted:    1,
				Blocked:    1,
				WithRemark: 1,
				ByStatus: []GroupCountSnapshot{
					{Value: "ACTIVE", Total: 2},
					{Value: "BLOCKED", Total: 1},
				},
			},
			CommandIdempotencyTotal: 9,
		},
		Outbox: &OutboxSnapshot{
			Total:              11,
			Pending:            3,
			Published:          7,
			DLQ:                1,
			ReadyPending:       2,
			OldestPendingAgeMS: &oldestPending,
			OldestDLQAgeMS:     &oldestDLQ,
		},
	})

	for _, expected := range []string{
		`nexusim_contacts_requests{state="total"} 7`,
		`nexusim_contacts_requests{state="pending"} 2`,
		`nexusim_contacts_request_status{status="ACCEPTED"} 1`,
		`nexusim_contacts_edges{state="active"} 2`,
		`nexusim_contacts_edges{state="with_remark"} 1`,
		`nexusim_contacts_edge_status{status="BLOCKED"} 1`,
		`nexusim_contacts_command_idempotency_total 9`,
		`nexusim_contacts_outbox{state="dlq"} 1`,
		`nexusim_contacts_outbox{state="ready_pending"} 2`,
		`nexusim_contacts_outbox_age_milliseconds{state="oldest_pending"} 1200`,
		`nexusim_contacts_outbox_age_milliseconds{state="oldest_dlq"} 3400`,
	} {
		assertContains(t, body, expected)
	}
}

func TestRenderPrometheusIncludesQueryErrorsAndEscapesLabels(t *testing.T) {
	body := renderPrometheus(Snapshot{
		ContactsError: "contacts metrics query failed",
		OutboxError:   "contacts outbox metrics query failed",
		Contacts: &ContactsSnapshot{
			Requests: &ContactRequestSnapshot{
				ByStatus: []GroupCountSnapshot{{Value: "BAD\"STATUS\\LINE\nNEXT", Total: 1}},
			},
		},
		Trace: &TraceSnapshot{Enabled: true, Exporter: "custom\"exporter"},
	})

	for _, expected := range []string{
		`nexusim_contacts_metrics_query_error 1`,
		`nexusim_contacts_outbox_metrics_query_error 1`,
		`nexusim_contacts_request_status{status="BAD\"STATUS\\LINE\nNEXT"} 1`,
		`nexusim_contacts_otel_traces_enabled{exporter="custom\"exporter"} 1`,
	} {
		assertContains(t, body, expected)
	}
}

func TestQueryContactsSnapshotIntegration(t *testing.T) {
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

	resetContactsMonitoringTables(t, ctx, pool)
	if _, err := pool.Exec(ctx, `
INSERT INTO contact_requests (
    request_id, tenant_id, sender_user_id, receiver_user_id, status, idempotency_key, command_hash, message
) VALUES
    ('request-1', 'tenant-metrics', 'alice', 'bob', 'PENDING', 'idem-1', 'hash-1', 'hello'),
    ('request-2', 'tenant-metrics', 'carol', 'dave', 'ACCEPTED', 'idem-2', 'hash-2', 'hi'),
    ('request-3', 'tenant-metrics', 'erin', 'frank', 'DECLINED', 'idem-3', 'hash-3', ''),
    ('request-4', 'tenant-metrics', 'gina', 'henry', 'CANCELED', 'idem-4', 'hash-4', ''),
    ('request-5', 'tenant-metrics', 'ivy', 'jack', 'EXPIRED', 'idem-5', 'hash-5', '');
INSERT INTO contact_edges (
    tenant_id, owner_user_id, contact_user_id, status, remark, source_request_id, version
) VALUES
    ('tenant-metrics', 'alice', 'bob', 'ACTIVE', 'friend', 'request-2', 1),
    ('tenant-metrics', 'bob', 'alice', 'ACTIVE', '', 'request-2', 1),
    ('tenant-metrics', 'carol', 'dave', 'DELETED', '', 'request-3', 2),
    ('tenant-metrics', 'erin', 'frank', 'BLOCKED', '', 'request-4', 3);
INSERT INTO contact_command_idempotency (
    tenant_id, user_id, idempotency_key, command_type, command_hash, result_id, result_json
) VALUES
    ('tenant-metrics', 'alice', 'idem-send', 'SEND_CONTACT_REQUEST', 'hash-send', 'request-1', '{}'::jsonb),
    ('tenant-metrics', 'bob', 'idem-respond', 'RESPOND_CONTACT_REQUEST', 'hash-respond', 'request-2', '{}'::jsonb);
`); err != nil {
		t.Fatalf("seed contacts monitoring tables: %v", err)
	}

	snapshot, err := queryContactsSnapshot(ctx, pool)
	if err != nil {
		t.Fatalf("query contacts snapshot: %v", err)
	}
	if snapshot.Requests == nil ||
		snapshot.Requests.Total != 5 ||
		snapshot.Requests.Pending != 1 ||
		snapshot.Requests.Accepted != 1 ||
		snapshot.Requests.Declined != 1 ||
		snapshot.Requests.Canceled != 1 ||
		snapshot.Requests.Expired != 1 {
		t.Fatalf("unexpected contact request snapshot: %+v", snapshot.Requests)
	}
	if snapshot.Edges == nil ||
		snapshot.Edges.Total != 4 ||
		snapshot.Edges.Active != 2 ||
		snapshot.Edges.Deleted != 1 ||
		snapshot.Edges.Blocked != 1 ||
		snapshot.Edges.WithRemark != 1 {
		t.Fatalf("unexpected contact edge snapshot: %+v", snapshot.Edges)
	}
	if snapshot.CommandIdempotencyTotal != 2 {
		t.Fatalf("unexpected command idempotency snapshot: %+v", snapshot)
	}
}

func resetContactsMonitoringTables(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(ctx, `
TRUNCATE TABLE
    contacts_outbox,
    contact_command_idempotency,
    contact_edges,
    contact_requests
RESTART IDENTITY
`); err != nil {
		t.Fatalf("truncate contacts monitoring tables: %v", err)
	}
}

func assertContains(t *testing.T, haystack string, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Fatalf("expected metrics to contain %q, got:\n%s", needle, haystack)
	}
}
