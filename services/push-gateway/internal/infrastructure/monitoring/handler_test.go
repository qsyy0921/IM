package monitoring

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	authinfra "github.com/qsyy0921/IM/services/push-gateway/internal/infrastructure/auth"
	"github.com/qsyy0921/IM/services/push-gateway/internal/infrastructure/memory"
	redisroute "github.com/qsyy0921/IM/services/push-gateway/internal/infrastructure/redisroute"
)

func TestHandlerHealthReadyAndMetrics(t *testing.T) {
	handler := NewHandler().
		WithMemoryMetrics(func() memory.Metrics {
			return memory.Metrics{
				ConnectedSessions:           2,
				SessionQueueFullCount:       1,
				ResumeBufferReplayCount:     3,
				ResumeBufferStoredFrames:    4,
				ResumeBufferTokenCount:      2,
				ResumeBufferExpiredCount:    5,
				SlowSessionEvictedCount:     6,
				IdentitySessionEvictedCount: 7,
			}
		}).
		WithRedisRegistryMetrics(func() redisroute.Metrics {
			return redisroute.Metrics{
				RedisRouteRemotePublishCallCount: 2,
				RedisRouteRemoteEnqueuedSessions: 4,
				RedisResumeReplayCount:           3,
			}
		}).
		WithRedisSubscriberMetrics(func() redisroute.Metrics {
			return redisroute.Metrics{
				RedisRouteSubscriberMessageCount:  5,
				RedisRouteSubscriberEnqueuedCount: 4,
			}
		}).
		WithAuthJWKStats(func() *authinfra.JWKStats {
			return &authinfra.JWKStats{
				RemoteURLConfigured: true,
				CachedKeyCount:      2,
				RefreshFailures:     1,
			}
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
	if snapshot.Service != serviceName || snapshot.Memory == nil || snapshot.Memory.ConnectedSessions != 2 {
		t.Fatalf("unexpected metrics snapshot: %+v", snapshot)
	}
	if snapshot.RedisRegistryMetrics == nil || snapshot.RedisRegistryMetrics.RedisRouteRemotePublishCallCount != 2 {
		t.Fatalf("unexpected redis registry metrics: %+v", snapshot.RedisRegistryMetrics)
	}
	if snapshot.RedisSubscriberStats == nil || snapshot.RedisSubscriberStats.RedisRouteSubscriberMessageCount != 5 {
		t.Fatalf("unexpected redis subscriber metrics: %+v", snapshot.RedisSubscriberStats)
	}
	if snapshot.AuthJWKStats == nil || !snapshot.AuthJWKStats.RemoteURLConfigured || snapshot.AuthJWKStats.CachedKeyCount != 2 {
		t.Fatalf("unexpected auth jwk stats: %+v", snapshot.AuthJWKStats)
	}
}

func TestHandlerNotFound(t *testing.T) {
	recorder := httptest.NewRecorder()
	NewHandler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/unknown", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d", recorder.Code)
	}
}
