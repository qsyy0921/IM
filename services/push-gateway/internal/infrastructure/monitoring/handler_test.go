package monitoring

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	authinfra "github.com/qsyy0921/IM/services/push-gateway/internal/infrastructure/auth"
	"github.com/qsyy0921/IM/services/push-gateway/internal/infrastructure/memory"
	redisroute "github.com/qsyy0921/IM/services/push-gateway/internal/infrastructure/redisroute"
	"github.com/qsyy0921/IM/services/push-gateway/internal/types"
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
		WithRedisSubscriberWorkerStats(func() types.RedisSubscriberWorkerSnapshot {
			return types.RedisSubscriberWorkerSnapshot{
				TotalErrors:        4,
				ConsecutiveErrors:  1,
				LastErrorAtMS:      70,
				LastSuccessAtMS:    120,
				LastErrorBackoffMS: 250,
			}
		}).
		WithAuthJWKStats(func() *authinfra.JWKStats {
			return &authinfra.JWKStats{
				RemoteURLConfigured: true,
				CachedKeyCount:      2,
				RefreshFailures:     1,
				LastRefreshSuccess:  1000,
				LastRefreshFailure:  2000,
			}
		}).
		WithDeliveryConsumerStats(func() types.ConsumerWorkerSnapshot {
			return types.ConsumerWorkerSnapshot{
				TotalErrors:        2,
				ConsecutiveErrors:  1,
				LastErrorAtMS:      100,
				LastSuccessAtMS:    90,
				LastCommitAtMS:     90,
				LastErrorBackoffMS: 1000,
			}
		}).
		WithIdentityConsumerStats(func() types.ConsumerWorkerSnapshot {
			return types.ConsumerWorkerSnapshot{
				TotalErrors:        3,
				ConsecutiveErrors:  0,
				LastErrorAtMS:      80,
				LastSuccessAtMS:    110,
				LastCommitAtMS:     110,
				LastErrorBackoffMS: 500,
			}
		}).
		WithWebSocketWriterStats(func() types.WebSocketWriterSnapshot {
			return types.WebSocketWriterSnapshot{
				OutboundFrameDequeuedCount:      11,
				FrameWriteSuccessCount:          12,
				DeliveryNotifyWriteSuccessCount: 10,
				LastDeliveryNotifyWriteAtMS:     1200,
				FrameWriteDuration: types.WebSocketWriterDurationSnapshot{
					Count: 2,
					SumMS: 3.5,
					MaxMS: 2.5,
				},
			}
		}).
		WithTraceStats(func() TraceSnapshot {
			return TraceSnapshot{
				Enabled:       true,
				ServiceName:   "push-gateway-test",
				Exporter:      "stdout",
				SamplingRatio: 0.25,
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
	if snapshot.RedisSubscriberWorker == nil || snapshot.RedisSubscriberWorker.TotalErrors != 4 {
		t.Fatalf("unexpected redis subscriber worker stats: %+v", snapshot.RedisSubscriberWorker)
	}
	if snapshot.AuthJWKStats == nil || !snapshot.AuthJWKStats.RemoteURLConfigured || snapshot.AuthJWKStats.CachedKeyCount != 2 {
		t.Fatalf("unexpected auth jwk stats: %+v", snapshot.AuthJWKStats)
	}
	if snapshot.DeliveryConsumer == nil || snapshot.DeliveryConsumer.TotalErrors != 2 {
		t.Fatalf("unexpected delivery consumer stats: %+v", snapshot.DeliveryConsumer)
	}
	if snapshot.IdentityConsumer == nil || snapshot.IdentityConsumer.TotalErrors != 3 {
		t.Fatalf("unexpected identity consumer stats: %+v", snapshot.IdentityConsumer)
	}
	if snapshot.WebSocketWriter == nil ||
		snapshot.WebSocketWriter.OutboundFrameDequeuedCount != 11 ||
		snapshot.WebSocketWriter.DeliveryNotifyWriteSuccessCount != 10 ||
		snapshot.WebSocketWriter.FrameWriteDuration.Count != 2 {
		t.Fatalf("unexpected websocket writer stats: %+v", snapshot.WebSocketWriter)
	}
	if snapshot.Trace == nil ||
		!snapshot.Trace.Enabled ||
		snapshot.Trace.ServiceName != "push-gateway-test" ||
		snapshot.Trace.Exporter != "stdout" ||
		snapshot.Trace.SamplingRatio != 0.25 {
		t.Fatalf("unexpected trace stats: %+v", snapshot.Trace)
	}
}

func TestHandlerPrometheusMetrics(t *testing.T) {
	handler := NewHandler().
		WithMemoryMetrics(func() memory.Metrics {
			return memory.Metrics{
				ConnectedSessions:           2,
				SessionQueueFullCount:       1,
				ResumeBufferReplayCount:     3,
				ResumeBufferMissCount:       4,
				ResumeBufferStoredFrames:    5,
				ResumeBufferTokenCount:      6,
				ResumeBufferExpiredCount:    7,
				SlowSessionEvictedCount:     8,
				IdentitySessionEvictedCount: 9,
			}
		}).
		WithRedisRegistryMetrics(func() redisroute.Metrics {
			return redisroute.Metrics{
				RedisRouteRegisterErrorCount:       1,
				RedisRouteRenewErrorCount:          2,
				RedisRouteRenewSessionEvictedCount: 3,
				RedisRouteLookupErrorCount:         4,
				RedisRouteRemoteMatchedSessions:    5,
				RedisRouteRemotePublishCallCount:   6,
				RedisRouteRemotePublishErrorCount:  7,
				RedisRouteRemoteNoSubscriberCount:  8,
				RedisRouteRemoteEnqueuedSessions:   9,
				RedisRouteStaleRemovedCount:        10,
				RedisRouteCleanupErrorCount:        11,
				RedisResumeReplayCount:             12,
				RedisResumeMissCount:               13,
				RedisResumeAppendCount:             14,
				RedisResumeAppendErrorCount:        15,
				RedisResumePermissionDeniedCount:   16,
			}
		}).
		WithRedisSubscriberMetrics(func() redisroute.Metrics {
			return redisroute.Metrics{
				RedisRouteSubscriberMessageCount:   21,
				RedisRouteSubscriberMalformedCount: 22,
				RedisRouteSubscriberEnqueuedCount:  23,
				RedisRouteSubscriberEvictedCount:   24,
				RedisRouteSubscriberErrorCount:     25,
			}
		}).
		WithRedisSubscriberWorkerStats(func() types.RedisSubscriberWorkerSnapshot {
			return types.RedisSubscriberWorkerSnapshot{
				TotalErrors:        4,
				ConsecutiveErrors:  1,
				LastErrorAtMS:      70,
				LastSuccessAtMS:    120,
				LastErrorBackoffMS: 250,
			}
		}).
		WithAuthJWKStats(func() *authinfra.JWKStats {
			return &authinfra.JWKStats{
				RemoteURLConfigured: true,
				CachedKeyCount:      2,
				RefreshFailures:     1,
				LastRefreshSuccess:  1000,
				LastRefreshFailure:  2000,
			}
		}).
		WithDeliveryConsumerStats(func() types.ConsumerWorkerSnapshot {
			return types.ConsumerWorkerSnapshot{
				TotalErrors:        2,
				ConsecutiveErrors:  1,
				LastErrorAtMS:      100,
				LastSuccessAtMS:    90,
				LastCommitAtMS:     90,
				LastErrorBackoffMS: 1000,
			}
		}).
		WithIdentityConsumerStats(func() types.ConsumerWorkerSnapshot {
			return types.ConsumerWorkerSnapshot{
				TotalErrors:        3,
				ConsecutiveErrors:  0,
				LastErrorAtMS:      80,
				LastSuccessAtMS:    110,
				LastCommitAtMS:     110,
				LastErrorBackoffMS: 500,
			}
		}).
		WithWebSocketWriterStats(func() types.WebSocketWriterSnapshot {
			return types.WebSocketWriterSnapshot{
				OutboundFrameDequeuedCount:       31,
				FrameWriteAttemptCount:           32,
				FrameWriteSuccessCount:           30,
				FrameWriteErrorCount:             2,
				DeliveryNotifyWriteAttemptCount:  24,
				DeliveryNotifyWriteSuccessCount:  23,
				DeliveryNotifyWriteErrorCount:    1,
				ResumeHintWriteAttemptCount:      3,
				ResumeHintWriteSuccessCount:      2,
				ResumeHintWriteErrorCount:        1,
				LastWriteSuccessAtMS:             3000,
				LastWriteErrorAtMS:               3100,
				LastDeliveryNotifyWriteAtMS:      3200,
				LastDeliveryNotifyWriteErrorAtMS: 3300,
				FrameWriteDuration: types.WebSocketWriterDurationSnapshot{
					Count: 3,
					SumMS: 7.5,
					MaxMS: 4.5,
					Buckets: []types.WebSocketWriterDurationBucket{
						{LE: "1", Count: 1},
						{LE: "+Inf", Count: 3},
					},
				},
				DeliveryNotifyWriteDuration: types.WebSocketWriterDurationSnapshot{
					Count: 2,
					SumMS: 6,
					MaxMS: 4,
					Buckets: []types.WebSocketWriterDurationBucket{
						{LE: "1", Count: 0},
						{LE: "+Inf", Count: 2},
					},
				},
			}
		}).
		WithTraceStats(func() TraceSnapshot {
			return TraceSnapshot{
				Enabled:         true,
				Exporter:        "otlp-grpc",
				OTLPEndpointSet: true,
				OTLPInsecure:    true,
				SamplingRatio:   0.5,
			}
		})

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	if contentType := recorder.Header().Get("Content-Type"); !strings.Contains(contentType, "text/plain") {
		t.Fatalf("content type = %q", contentType)
	}
	body := recorder.Body.String()
	assertContains(t, body, `nexusim_push_gateway_build_info{service="push-gateway"} 1`)
	assertContains(t, body, `nexusim_push_gateway_sessions{state="connected"} 2`)
	assertContains(t, body, `nexusim_push_gateway_session_events_total{event="queue_full"} 1`)
	assertContains(t, body, `nexusim_push_gateway_session_events_total{event="slow_evicted"} 8`)
	assertContains(t, body, `nexusim_push_gateway_resume_buffer{state="stored_frames"} 5`)
	assertContains(t, body, `nexusim_push_gateway_resume_buffer_events_total{event="miss"} 4`)
	assertContains(t, body, `nexusim_push_gateway_redis_route_events_total{event="remote_publish_error",role="registry"} 7`)
	assertContains(t, body, `nexusim_push_gateway_redis_route_events_total{event="subscriber_malformed",role="subscriber"} 22`)
	assertContains(t, body, `nexusim_push_gateway_redis_resume_events_total{event="append_error",role="registry"} 15`)
	assertContains(t, body, `nexusim_push_gateway_redis_subscriber_worker_consecutive_errors 1`)
	assertContains(t, body, `nexusim_push_gateway_auth_jwks_cached_keys 2`)
	assertContains(t, body, `nexusim_push_gateway_auth_jwks_refresh_failures_total 1`)
	assertContains(t, body, `nexusim_push_gateway_consumer_worker_errors_total{consumer="delivery"} 2`)
	assertContains(t, body, `nexusim_push_gateway_consumer_worker_errors_total{consumer="identity"} 3`)
	assertContains(t, body, `nexusim_push_gateway_ws_writer_events_total{event="outbound_frame_dequeued"} 31`)
	assertContains(t, body, `nexusim_push_gateway_ws_writer_events_total{event="delivery_notify_write_success"} 23`)
	assertContains(t, body, `nexusim_push_gateway_ws_writer_events_total{event="resume_hint_write_error"} 1`)
	assertContains(t, body, `nexusim_push_gateway_ws_writer_last_event_unix_milliseconds{event="delivery_notify_write_error"} 3300`)
	assertContains(t, body, `nexusim_push_gateway_ws_writer_write_duration_milliseconds_bucket{le="1",operation="frame_write"} 1`)
	assertContains(t, body, `nexusim_push_gateway_ws_writer_write_duration_milliseconds_bucket{le="+Inf",operation="delivery_notify"} 2`)
	assertContains(t, body, `nexusim_push_gateway_ws_writer_write_duration_milliseconds_sum{operation="frame_write"} 7.5`)
	assertContains(t, body, `nexusim_push_gateway_ws_writer_write_duration_milliseconds_count{operation="delivery_notify"} 2`)
	assertContains(t, body, `nexusim_push_gateway_ws_writer_write_duration_max_milliseconds{operation="delivery_notify"} 4`)
	assertContains(t, body, `nexusim_push_gateway_otel_traces_enabled{exporter="otlp-grpc"} 1`)
	assertContains(t, body, `nexusim_push_gateway_otel_traces_sampling_ratio{exporter="otlp-grpc"} 0.5`)
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
	if strings.Count(body, "# TYPE nexusim_push_gateway_consumer_worker_errors_total counter") != 1 {
		t.Fatalf("consumer worker TYPE header should be emitted once:\n%s", body)
	}
	if strings.Count(body, "# TYPE nexusim_push_gateway_redis_route_events_total counter") != 1 {
		t.Fatalf("redis route TYPE header should be emitted once:\n%s", body)
	}
}

func TestRenderPrometheusEscapesTraceExporterLabel(t *testing.T) {
	body := renderPrometheus(Snapshot{
		Service: serviceName,
		Trace: &TraceSnapshot{
			Enabled:       true,
			Exporter:      "otlp-\ngrpc\"quoted",
			SamplingRatio: 0.75,
		},
	})
	assertContains(t, body, `nexusim_push_gateway_otel_traces_enabled{exporter="otlp-\ngrpc\"quoted"} 1`)
}

func TestHandlerNotFound(t *testing.T) {
	recorder := httptest.NewRecorder()
	NewHandler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/unknown", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d", recorder.Code)
	}
}

func assertContains(t *testing.T, text string, expected string) {
	t.Helper()
	if !strings.Contains(text, expected) {
		t.Fatalf("expected %q in:\n%s", expected, text)
	}
}
