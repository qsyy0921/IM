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
	}).WithHTTPBFFStats(func() HTTPBFFSnapshot {
		return HTTPBFFSnapshot{TotalRequests: 2, TotalErrors: 1, Routes: []HTTPBFFRouteSnapshot{{
			Route:        "messages.send",
			Method:       "POST",
			Count:        2,
			ErrorCount:   1,
			LatencyAvgMS: 5,
			LatencyMaxMS: 8,
			StatusCodes:  map[string]int64{"200": 1, "429": 1},
		}}}
	}).WithRateLimitStats(func() ratelimit.Snapshot {
		return ratelimit.Snapshot{Enabled: true, Backend: "redis", RedisMode: "single", RatePerSecond: 10, Burst: 20, TenantPlanSource: "none", TotalLimited: 3}
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
	if snapshot.HTTPBFF == nil || snapshot.HTTPBFF.TotalRequests != 2 || snapshot.HTTPBFF.TotalErrors != 1 {
		t.Fatalf("unexpected http bff stats: %+v", snapshot.HTTPBFF)
	}
	if snapshot.RateLimit == nil || !snapshot.RateLimit.Enabled || snapshot.RateLimit.TotalLimited != 3 {
		t.Fatalf("unexpected rate limit stats: %+v", snapshot.RateLimit)
	}
	if snapshot.RateLimit.RedisMode != "single" {
		t.Fatalf("expected redis mode in rate limit stats, got %+v", snapshot.RateLimit)
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
	metrics.record("/nexusim.message.v1.MessageService/SendMessage", "OK", 5)
	metrics.record("/nexusim.api/Test\"Method\nLine", "OK", 3)
	handler := NewHandler(metrics).WithAuthJWKStats(func() gatewayauth.JWKStats {
		return gatewayauth.JWKStats{RemoteURLConfigured: true, CachedKeyCount: 2, RefreshFailures: 1}
	}).WithHTTPBFFStats(func() HTTPBFFSnapshot {
		return HTTPBFFSnapshot{Routes: []HTTPBFFRouteSnapshot{{
			Route:        "conversation.messages",
			Method:       "GET",
			Count:        3,
			ErrorCount:   1,
			LatencyAvgMS: 4,
			LatencyMaxMS: 10,
			StatusCodes:  map[string]int64{"200": 2, "404": 1},
		}}}
	}).WithRateLimitStats(func() ratelimit.Snapshot {
		return ratelimit.Snapshot{
			Enabled:                    true,
			Backend:                    "redis",
			RedisMode:                  "cluster",
			KeyScope:                   "tenant",
			TenantPlans:                1,
			TenantPlanSource:           "file",
			TenantPlanGeneratedAt:      1_800_000_000_000,
			TenantPlanRequireChecksum:  true,
			TenantPlanRequireVersioned: true,
			TenantPlanMaxAgeMS:         3_600_000,
			TenantPlanAgeMS:            3_700_000,
			TenantPlanStale:            true,
			TenantPlanURLBearerSet:     true,
			TenantPlanURLRequireHTTPS:  true,
			TenantPlanURLTLSConfigured: true,
			TenantPlanURLClientCertSet: true,
			TenantReloads:              2,
			TenantErrors:               3,
			RedisErrors:                4,
			IdentityErrors:             5,
			TotalAccepted:              6,
			TotalLimited:               7,
		}
	}).WithRuntimeStats(func() RuntimeSnapshot {
		return RuntimeSnapshot{RegisterLegacyDescriptors: true, LegacyDescriptorsAllowedUntilMS: 1_800_000_000_000}
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
	rateLabels := `backend="redis",key_scope="tenant",redis_mode="cluster",tenant_plan_source="file"`
	assertContains(t, body, "# TYPE nexusim_api_gateway_grpc_requests_total counter")
	assertContains(t, body, `nexusim_api_gateway_grpc_requests_total{code="OK",method="/nexusim.gateway.v1.GatewayService/SendMessage"} 1`)
	assertContains(t, body, `nexusim_api_gateway_grpc_requests_total{code="Unavailable",method="/nexusim.gateway.v1.GatewayService/SendMessage"} 1`)
	assertContains(t, body, `nexusim_api_gateway_grpc_errors_total{method="/nexusim.gateway.v1.GatewayService/SendMessage"} 1`)
	assertContains(t, body, `nexusim_api_gateway_grpc_requests_total{code="OK",method="/nexusim.api/Test\"Method\nLine"} 1`)
	assertContains(t, body, `nexusim_api_gateway_grpc_exposure_requests_total{exposure="facade"} 2`)
	assertContains(t, body, `nexusim_api_gateway_grpc_exposure_requests_total{exposure="legacy_descriptor"} 1`)
	assertContains(t, body, `nexusim_api_gateway_grpc_legacy_descriptor_last_seen_unix_milliseconds`)
	assertContains(t, body, `nexusim_api_gateway_bff_http_requests_total{method="GET",route="conversation.messages",status_code="200"} 2`)
	assertContains(t, body, `nexusim_api_gateway_bff_http_requests_total{method="GET",route="conversation.messages",status_code="404"} 1`)
	assertContains(t, body, `nexusim_api_gateway_bff_http_errors_total{method="GET",route="conversation.messages"} 1`)
	assertContains(t, body, `nexusim_api_gateway_bff_http_latency_avg_milliseconds{method="GET",route="conversation.messages"} 4`)
	assertContains(t, body, `nexusim_api_gateway_rate_limit_enabled{`+rateLabels+`} 1`)
	assertContains(t, body, `nexusim_api_gateway_rate_limit_tenant_plan_require_checksum{`+rateLabels+`} 1`)
	assertContains(t, body, `nexusim_api_gateway_rate_limit_tenant_plan_require_versioned{`+rateLabels+`} 1`)
	assertContains(t, body, `nexusim_api_gateway_rate_limit_tenant_plan_url_bearer_token_configured{`+rateLabels+`} 1`)
	assertContains(t, body, `nexusim_api_gateway_rate_limit_tenant_plan_url_require_https{`+rateLabels+`} 1`)
	assertContains(t, body, `nexusim_api_gateway_rate_limit_tenant_plan_url_tls_configured{`+rateLabels+`} 1`)
	assertContains(t, body, `nexusim_api_gateway_rate_limit_tenant_plan_url_client_cert_configured{`+rateLabels+`} 1`)
	assertContains(t, body, `nexusim_api_gateway_rate_limit_tenant_plan_generated_at_unix_milliseconds{`+rateLabels+`} 1800000000000`)
	assertContains(t, body, `nexusim_api_gateway_rate_limit_tenant_plan_age_milliseconds{`+rateLabels+`} 3700000`)
	assertContains(t, body, `nexusim_api_gateway_rate_limit_tenant_plan_stale{`+rateLabels+`} 1`)
	assertContains(t, body, `nexusim_api_gateway_rate_limit_redis_errors_total{`+rateLabels+`} 4`)
	assertContains(t, body, `nexusim_api_gateway_rate_limit_identity_errors_total{`+rateLabels+`} 5`)
	assertContains(t, body, `nexusim_api_gateway_rate_limit_tenant_plan_reload_errors_total{`+rateLabels+`} 3`)
	assertContains(t, body, `nexusim_api_gateway_auth_jwks_refresh_failures_total 1`)
	assertContains(t, body, `nexusim_api_gateway_legacy_descriptors_registered 1`)
	assertContains(t, body, `nexusim_api_gateway_legacy_descriptors_allowed_until_unix_milliseconds 1800000000000`)
	assertContains(t, body, `nexusim_api_gateway_otel_traces_enabled{exporter="otlp-grpc"} 1`)
	for _, leaked := range []string{"tenant_a", "user_a", "gateway-token", "request_id", "trace_id", "quota-config-token", "client-key.pem", "ca.pem"} {
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
