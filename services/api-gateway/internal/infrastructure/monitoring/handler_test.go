package monitoring

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gatewayauth "github.com/qsyy0921/IM/internal/gatewayauth"
	"github.com/qsyy0921/IM/services/api-gateway/internal/infrastructure/ratelimit"
)

func TestHandlerHealthReadyAndMetrics(t *testing.T) {
	metrics := NewGRPCMetrics()
	metrics.record("/nexusim.api/Test", "OK", 7)
	handler := NewHandler(metrics).WithAuthJWKStats(func() gatewayauth.JWKStats {
		return gatewayauth.JWKStats{RemoteURLConfigured: true, CachedKeyCount: 2, RefreshFailures: 1}
	}).WithRateLimitStats(func() ratelimit.Snapshot {
		return ratelimit.Snapshot{Enabled: true, RatePerSecond: 10, Burst: 20, TenantPlanSource: "none", TotalLimited: 3}
	}).WithRuntimeStats(func() RuntimeSnapshot {
		return RuntimeSnapshot{RegisterLegacyDescriptors: false}
	}).WithTraceStats(func() TraceSnapshot {
		return TraceSnapshot{Enabled: true, ServiceName: serviceName, Exporter: "stdout", SamplingRatio: 1}
	})

	for _, path := range []string{"/healthz", "/readyz"} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s status = %d", path, recorder.Code)
		}
	}

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/debug/metrics", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("metrics status = %d", recorder.Code)
	}
	var snapshot Snapshot
	if err := json.Unmarshal(recorder.Body.Bytes(), &snapshot); err != nil {
		t.Fatalf("decode metrics: %v", err)
	}
	if snapshot.Service != serviceName || snapshot.GRPC == nil || snapshot.GRPC.TotalRequests != 1 {
		t.Fatalf("unexpected metrics snapshot: %+v", snapshot)
	}
	if snapshot.GRPC.FacadeRequests != 0 || snapshot.GRPC.OtherRequests != 1 {
		t.Fatalf("unexpected grpc exposure counters: %+v", snapshot.GRPC)
	}
	if snapshot.AuthJWKs == nil || !snapshot.AuthJWKs.RemoteURLConfigured || snapshot.AuthJWKs.CachedKeyCount != 2 {
		t.Fatalf("unexpected jwk stats: %+v", snapshot.AuthJWKs)
	}
	if snapshot.RateLimit == nil || !snapshot.RateLimit.Enabled || snapshot.RateLimit.TotalLimited != 3 {
		t.Fatalf("unexpected rate limit stats: %+v", snapshot.RateLimit)
	}
	if snapshot.Runtime == nil || snapshot.Runtime.RegisterLegacyDescriptors {
		t.Fatalf("unexpected runtime stats: %+v", snapshot.Runtime)
	}
	if snapshot.Trace == nil || !snapshot.Trace.Enabled || snapshot.Trace.Exporter != "stdout" {
		t.Fatalf("unexpected trace stats: %+v", snapshot.Trace)
	}
}

func TestHandlerPrometheusMetrics(t *testing.T) {
	metrics := NewGRPCMetrics()
	metrics.record("/nexusim.gateway.v1.GatewayService/SendMessage", "OK", 7)
	metrics.record("/nexusim.gateway.v1.GatewayService/SendMessage", "Unavailable", 11)
	metrics.record("/nexusim.api/Test\"Method\nLine", "OK", 3)
	handler := NewHandler(metrics).WithAuthJWKStats(func() gatewayauth.JWKStats {
		return gatewayauth.JWKStats{RemoteURLConfigured: true, CachedKeyCount: 2, RefreshFailures: 1}
	}).WithRateLimitStats(func() ratelimit.Snapshot {
		return ratelimit.Snapshot{
			Enabled:          true,
			Backend:          "redis",
			KeyScope:         "tenant",
			TenantPlans:      1,
			TenantPlanSource: "file",
			TenantReloads:    2,
			TenantErrors:     3,
			RedisErrors:      4,
			IdentityErrors:   5,
			TotalAccepted:    6,
			TotalLimited:     7,
		}
	}).WithRuntimeStats(func() RuntimeSnapshot {
		return RuntimeSnapshot{RegisterLegacyDescriptors: true}
	}).WithTraceStats(func() TraceSnapshot {
		return TraceSnapshot{Enabled: true, Exporter: "otlp-grpc", OTLPEndpointSet: true, SamplingRatio: 0.5}
	})

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("metrics status = %d", recorder.Code)
	}
	if got := recorder.Header().Get("Content-Type"); got != "text/plain; version=0.0.4; charset=utf-8" {
		t.Fatalf("unexpected content type %q", got)
	}
	body := recorder.Body.String()
	assertContains(t, body, "# TYPE nexusim_api_gateway_grpc_requests_total counter")
	assertContains(t, body, `nexusim_api_gateway_grpc_requests_total{code="OK",method="/nexusim.gateway.v1.GatewayService/SendMessage"} 1`)
	assertContains(t, body, `nexusim_api_gateway_grpc_requests_total{code="Unavailable",method="/nexusim.gateway.v1.GatewayService/SendMessage"} 1`)
	assertContains(t, body, `nexusim_api_gateway_grpc_errors_total{method="/nexusim.gateway.v1.GatewayService/SendMessage"} 1`)
	assertContains(t, body, `nexusim_api_gateway_grpc_requests_total{code="OK",method="/nexusim.api/Test\"Method\nLine"} 1`)
	assertContains(t, body, `nexusim_api_gateway_grpc_exposure_requests_total{exposure="facade"} 2`)
	assertContains(t, body, `nexusim_api_gateway_rate_limit_enabled{backend="redis",key_scope="tenant",tenant_plan_source="file"} 1`)
	assertContains(t, body, `nexusim_api_gateway_auth_jwks_refresh_failures_total 1`)
	assertContains(t, body, `nexusim_api_gateway_legacy_descriptors_registered 1`)
	assertContains(t, body, `nexusim_api_gateway_otel_traces_enabled{exporter="otlp-grpc"} 1`)
	for _, leaked := range []string{"tenant_a", "user_a", "gateway-token", "request_id", "trace_id"} {
		if strings.Contains(body, leaked) {
			t.Fatalf("prometheus metrics leaked %q in body:\n%s", leaked, body)
		}
	}
}

func assertContains(t *testing.T, body string, want string) {
	t.Helper()
	if !strings.Contains(body, want) {
		t.Fatalf("metrics body missing %q:\n%s", want, body)
	}
}
