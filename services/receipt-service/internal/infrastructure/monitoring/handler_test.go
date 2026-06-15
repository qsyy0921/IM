package monitoring

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/qsyy0921/IM/services/receipt-service/internal/types"
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
	if body.PGPool != nil || body.Receipt != nil || body.Outbox != nil {
		t.Fatalf("nil pool should not include pg/receipt/outbox metrics: %+v", body)
	}
}

func TestHandlerMetricsIncludesGRPCSnapshot(t *testing.T) {
	grpcMetrics := NewGRPCMetrics()
	grpcMetrics.record("/nexusim.receipt.v1.ReceiptService/GetReceiptState", "OK", 12)
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

func TestHandlerMetricsIncludesDeliveryProjectionWorkerSnapshot(t *testing.T) {
	handler := NewHandler(nil).WithDeliveryProjectionWorkerStats(func() types.ProjectionWorkerSnapshot {
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
		t.Fatalf("expected status 200, got %d", response.Code)
	}
	var body Snapshot
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode metrics response: %v", err)
	}
	if body.DeliveryProjectionWorker == nil || body.DeliveryProjectionWorker.TotalErrors != 2 {
		t.Fatalf("expected delivery worker metrics, got %+v", body.DeliveryProjectionWorker)
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
		t.Fatalf("expected status 200, got %d", response.Code)
	}
	var body Snapshot
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
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
		t.Fatalf("expected status 200, got %d", response.Code)
	}
	var body Snapshot
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode metrics response: %v", err)
	}
	if body.Trace == nil || !body.Trace.Enabled || body.Trace.ServiceName != serviceName {
		t.Fatalf("expected trace metrics, got %+v", body.Trace)
	}
}

func TestHandlerPrometheusMetricsWithoutPool(t *testing.T) {
	grpcMetrics := NewGRPCMetrics()
	grpcMetrics.record("/nexusim.receipt.v1.ReceiptService/GetReceiptState", "OK", 12)
	handler := NewHandler(nil, grpcMetrics).
		WithDeliveryProjectionWorkerStats(func() types.ProjectionWorkerSnapshot {
			return types.ProjectionWorkerSnapshot{
				TotalErrors:        2,
				ConsecutiveErrors:  1,
				LastErrorAtMS:      100,
				LastSuccessAtMS:    90,
				LastCommitAtMS:     90,
				LastErrorBackoffMS: 1000,
			}
		}).
		WithOutboxRelayStats(func() types.OutboxRelayWorkerSnapshot {
			return types.OutboxRelayWorkerSnapshot{
				TotalErrors:        3,
				ConsecutiveErrors:  1,
				LastErrorAtMS:      110,
				LastSuccessAtMS:    95,
				LastPublishedAtMS:  80,
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
		t.Fatalf("expected status 200, got %d", response.Code)
	}
	if contentType := response.Header().Get("Content-Type"); !strings.Contains(contentType, "text/plain") {
		t.Fatalf("unexpected content type %q", contentType)
	}
	body := response.Body.String()
	assertContains(t, body, `nexusim_receipt_build_info{service="receipt-service"} 1`)
	assertContains(t, body, `nexusim_receipt_grpc_method_requests_total{method="/nexusim.receipt.v1.ReceiptService/GetReceiptState"} 1`)
	assertContains(t, body, `nexusim_receipt_grpc_requests_total{code="OK",method="/nexusim.receipt.v1.ReceiptService/GetReceiptState"} 1`)
	assertContains(t, body, `nexusim_receipt_delivery_projection_worker_errors_total 2`)
	assertContains(t, body, `nexusim_receipt_delivery_projection_worker_consecutive_errors 1`)
	assertContains(t, body, `nexusim_receipt_outbox_relay_errors_total 3`)
	assertContains(t, body, `nexusim_receipt_outbox_relay_last_published_unix_milliseconds 80`)
	assertContains(t, body, `nexusim_receipt_otel_traces_enabled{exporter="otlp-grpc"} 1`)
	assertContains(t, body, `nexusim_receipt_otel_traces_sampling_ratio{exporter="otlp-grpc"} 0.5`)
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
			t.Fatalf("prometheus metrics leaked forbidden text %q:\n%s", forbidden, body)
		}
	}
}

func TestRenderPrometheusIncludesReceiptAggregates(t *testing.T) {
	oldestPending := int64(1234)
	oldestDLQ := int64(5678)
	body := renderPrometheus(Snapshot{
		Service: serviceName,
		Receipt: &ReceiptSnapshot{
			InboxProjectionTotal:        10,
			DeviceReceivedCursors:       11,
			UserReceivedCursors:         12,
			UserReadCursors:             13,
			MessageReceiptStates:        14,
			ConversationSummaries:       15,
			ArchivedConversationCount:   2,
			PinnedConversationCount:     3,
			MutedConversationCount:      4,
			UnreadConversationCount:     5,
			KafkaCheckpoints:            6,
			KafkaConsumerGroups:         7,
			ConversationListCheckpoints: 8,
		},
		Outbox: &OutboxSnapshot{
			Total:              20,
			Pending:            1,
			Published:          18,
			DLQ:                1,
			ReadyPending:       1,
			OldestPendingAgeMS: &oldestPending,
			OldestDLQAgeMS:     &oldestDLQ,
		},
	})
	assertContains(t, body, `nexusim_receipt_metrics_query_error 0`)
	assertContains(t, body, `nexusim_receipt_projection{state="inbox_projection_total"} 10`)
	assertContains(t, body, `nexusim_receipt_projection{state="message_receipt_states"} 14`)
	assertContains(t, body, `nexusim_receipt_conversation_summary{state="unread"} 5`)
	assertContains(t, body, `nexusim_receipt_kafka_checkpoints{state="conversation_list_checkpoints"} 8`)
	assertContains(t, body, `nexusim_receipt_outbox{state="dlq"} 1`)
	assertContains(t, body, `nexusim_receipt_outbox_age_milliseconds{state="oldest_pending"} 1234`)
	assertContains(t, body, `nexusim_receipt_outbox_age_milliseconds{state="oldest_dlq"} 5678`)
}

func TestRenderPrometheusIncludesQueryErrorsAndEscapesLabels(t *testing.T) {
	body := renderPrometheus(Snapshot{
		Service:      serviceName,
		ReceiptError: "receipt metrics query failed",
		OutboxError:  "receipt outbox metrics query failed",
		Trace: &TraceSnapshot{
			Enabled:       true,
			Exporter:      "otlp-\ngrpc\"quoted",
			SamplingRatio: 0.75,
		},
	})
	assertContains(t, body, `nexusim_receipt_metrics_query_error 1`)
	assertContains(t, body, `nexusim_receipt_outbox_metrics_query_error 1`)
	assertContains(t, body, `nexusim_receipt_otel_traces_enabled{exporter="otlp-\ngrpc\"quoted"} 1`)
}

func assertContains(t *testing.T, text string, expected string) {
	t.Helper()
	if !strings.Contains(text, expected) {
		t.Fatalf("expected %q in:\n%s", expected, text)
	}
}
