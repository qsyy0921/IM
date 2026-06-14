package monitoring

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
		return ratelimit.Snapshot{Enabled: true, RatePerSecond: 10, Burst: 20, TotalLimited: 3}
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
