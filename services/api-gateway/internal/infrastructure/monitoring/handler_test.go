package monitoring

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	gatewayauth "github.com/qsyy0921/IM/internal/gatewayauth"
)

func TestHandlerHealthReadyAndMetrics(t *testing.T) {
	metrics := NewGRPCMetrics()
	metrics.record("/nexusim.api/Test", "OK", 7)
	handler := NewHandler(metrics).WithAuthJWKStats(func() gatewayauth.JWKStats {
		return gatewayauth.JWKStats{RemoteURLConfigured: true, CachedKeyCount: 2, RefreshFailures: 1}
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
	if snapshot.AuthJWKs == nil || !snapshot.AuthJWKs.RemoteURLConfigured || snapshot.AuthJWKs.CachedKeyCount != 2 {
		t.Fatalf("unexpected jwk stats: %+v", snapshot.AuthJWKs)
	}
}
