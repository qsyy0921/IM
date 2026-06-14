package monitoring

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
