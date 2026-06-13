package monitoring

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
