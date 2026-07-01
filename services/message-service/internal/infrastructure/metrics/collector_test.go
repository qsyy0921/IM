package metrics

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	monitoringinfra "github.com/qsyy0921/IM/services/message-service/internal/infrastructure/monitoring"
	"github.com/qsyy0921/IM/services/message-service/internal/types"
)

func TestCollectorSnapshot(t *testing.T) {
	collector := NewCollector()
	collector.ObserveSendMessage(40 * time.Millisecond)
	collector.ObserveRepositoryAppend(35 * time.Millisecond)
	collector.ObserveRepositoryBegin(6 * time.Millisecond)
	collector.ObserveRepositoryPoolAcquire(5 * time.Millisecond)
	collector.ObserveRepositoryTxBegin(1 * time.Millisecond)
	collector.ObserveRepositoryIdempotencyLock(7 * time.Millisecond)
	collector.ObserveRepositoryFindExisting(8 * time.Millisecond)
	collector.ObserveRepositoryEnsureSeq(9 * time.Millisecond)
	collector.ObserveRepositoryAllocateSeq(10 * time.Millisecond)
	collector.ObserveRepositoryInsertMessage(11 * time.Millisecond)
	collector.ObserveRepositoryInsertTimeline(12 * time.Millisecond)
	collector.ObserveRepositoryInsertOutbox(13 * time.Millisecond)
	collector.ObserveRepositoryCommit(4 * time.Millisecond)
	collector.ObserveConversationSeqAlloc(10 * time.Millisecond)
	collector.ObserveConversationSeqAlloc(20 * time.Millisecond)
	collector.ObserveKafkaPublish(30 * time.Millisecond)
	collector.ObserveKafkaPublishCall(40*time.Millisecond, 4)
	collector.ObserveOutboxProcessReady(50 * time.Millisecond)
	collector.ObserveOutboxProcessReadyResult(60*time.Millisecond, 3)
	collector.ObserveOutboxProcessReadyResult(5*time.Millisecond, 0)
	collector.ObserveOutboxFetchReady(6 * time.Millisecond)
	collector.ObserveOutboxMarkPublished(7 * time.Millisecond)
	collector.ObserveOutboxCommit(8 * time.Millisecond)

	snapshot := collector.Snapshot()
	if snapshot.SendMessageLatencyMS.Count != 1 ||
		snapshot.SendMessageLatencyMS.AvgMS != 40 {
		t.Fatalf("unexpected send message snapshot: %+v", snapshot.SendMessageLatencyMS)
	}
	if snapshot.SendMessageRecentLatencyMS.Count != 1 ||
		snapshot.SendMessageRecentLatencyMS.AvgMS != 40 {
		t.Fatalf("unexpected recent send message snapshot: %+v", snapshot.SendMessageRecentLatencyMS)
	}
	if snapshot.RepositoryAppendLatencyMS.Count != 1 ||
		snapshot.RepositoryAppendLatencyMS.AvgMS != 35 {
		t.Fatalf("unexpected repository append snapshot: %+v", snapshot.RepositoryAppendLatencyMS)
	}
	if snapshot.RepositoryBeginLatencyMS.Count != 1 ||
		snapshot.RepositoryPoolAcquireLatencyMS.Count != 1 ||
		snapshot.RepositoryPoolAcquireLatencyMS.AvgMS != 5 ||
		snapshot.RepositoryPoolAcquireRecentLatencyMS.Count != 1 ||
		snapshot.RepositoryPoolAcquireRecentLatencyMS.AvgMS != 5 ||
		snapshot.RepositoryTxBeginLatencyMS.Count != 1 ||
		snapshot.RepositoryTxBeginLatencyMS.AvgMS != 1 ||
		snapshot.RepositoryIdempotencyLockLatencyMS.Count != 1 ||
		snapshot.RepositoryFindExistingLatencyMS.Count != 1 ||
		snapshot.RepositoryEnsureSeqLatencyMS.Count != 1 ||
		snapshot.RepositoryEnsureSeqRecentLatencyMS.Count != 1 ||
		snapshot.RepositoryAllocateSeqLatencyMS.Count != 1 ||
		snapshot.RepositoryAllocateSeqRecentLatencyMS.Count != 1 ||
		snapshot.RepositoryInsertMessageLatencyMS.Count != 1 ||
		snapshot.RepositoryInsertTimelineLatencyMS.Count != 1 ||
		snapshot.RepositoryInsertOutboxLatencyMS.Count != 1 {
		t.Fatalf("missing repository stage snapshots: %+v", snapshot)
	}
	if snapshot.RepositoryCommitLatencyMS.Count != 1 ||
		snapshot.RepositoryCommitLatencyMS.AvgMS != 4 {
		t.Fatalf("unexpected repository commit snapshot: %+v", snapshot.RepositoryCommitLatencyMS)
	}
	if snapshot.ConversationSeqAllocLatencyMS.Count != 2 ||
		snapshot.ConversationSeqAllocLatencyMS.AvgMS != 15 ||
		snapshot.ConversationSeqAllocLatencyMS.P95MS != 20 {
		t.Fatalf("unexpected seq alloc snapshot: %+v", snapshot.ConversationSeqAllocLatencyMS)
	}
	if snapshot.KafkaPublishLatencyMS.Count != 2 ||
		snapshot.KafkaPublishLatencyMS.AvgMS != 35 ||
		snapshot.KafkaPublishCallLatencyMS.Count != 2 ||
		snapshot.KafkaPublishRecordLatencyEstimateMS.Count != 2 ||
		snapshot.KafkaPublishRecordLatencyEstimateMS.AvgMS != 20 ||
		snapshot.KafkaPublishRecordsPerCall.Count != 2 ||
		snapshot.KafkaPublishRecordsPerCall.Avg != 2.5 ||
		snapshot.KafkaPublishRecordsPerCallRecent.Count != 2 ||
		snapshot.KafkaPublishRecordsPerCallRecent.Avg != 2.5 {
		t.Fatalf("unexpected kafka snapshot: %+v", snapshot.KafkaPublishLatencyMS)
	}
	if snapshot.OutboxProcessReadyLatencyMS.Count != 3 ||
		snapshot.OutboxProcessReadyActiveLatencyMS.Count != 1 ||
		snapshot.OutboxProcessReadyActiveLatencyMS.AvgMS != 60 ||
		snapshot.OutboxProcessReadyActiveRecentLatencyMS.Count != 1 ||
		snapshot.OutboxProcessReadyActiveRecentLatencyMS.AvgMS != 60 ||
		snapshot.OutboxProcessReadyIdleLatencyMS.Count != 1 ||
		snapshot.OutboxProcessReadyIdleLatencyMS.AvgMS != 5 ||
		snapshot.OutboxFetchedPerCall.Count != 2 ||
		snapshot.OutboxFetchedPerCall.Avg != 1.5 ||
		snapshot.OutboxFetchedPerCallRecent.Count != 2 ||
		snapshot.OutboxFetchedPerCallRecent.Avg != 1.5 ||
		snapshot.OutboxFetchReadyLatencyMS.Count != 1 ||
		snapshot.OutboxMarkPublishedLatencyMS.Count != 1 ||
		snapshot.OutboxCommitLatencyMS.Count != 1 {
		t.Fatalf("unexpected outbox snapshots: %+v", snapshot)
	}
}

func TestCollectorRecentSnapshotDropsOldSamples(t *testing.T) {
	collector := NewCollector()
	collector.ObserveSendMessage(90 * time.Millisecond)
	collector.ObserveRepositoryPoolAcquire(100 * time.Millisecond)
	for i := 0; i < recentSampleLimit; i++ {
		collector.ObserveSendMessage(2 * time.Millisecond)
		collector.ObserveRepositoryPoolAcquire(time.Millisecond)
	}

	snapshot := collector.Snapshot()
	if snapshot.SendMessageLatencyMS.MaxMS != 90 {
		t.Fatalf("unexpected cumulative send snapshot: %+v", snapshot.SendMessageLatencyMS)
	}
	if snapshot.SendMessageRecentLatencyMS.Count != int64(recentSampleLimit) ||
		snapshot.SendMessageRecentLatencyMS.MaxMS != 2 {
		t.Fatalf("unexpected recent send snapshot: %+v", snapshot.SendMessageRecentLatencyMS)
	}
	if snapshot.RepositoryPoolAcquireLatencyMS.Count != int64(recentSampleLimit+1) ||
		snapshot.RepositoryPoolAcquireLatencyMS.MaxMS != 100 {
		t.Fatalf("unexpected cumulative snapshot: %+v", snapshot.RepositoryPoolAcquireLatencyMS)
	}
	if snapshot.RepositoryPoolAcquireRecentLatencyMS.Count != int64(recentSampleLimit) ||
		snapshot.RepositoryPoolAcquireRecentLatencyMS.MaxMS != 1 {
		t.Fatalf("unexpected recent snapshot: %+v", snapshot.RepositoryPoolAcquireRecentLatencyMS)
	}
}

func TestCollectorServeHTTP(t *testing.T) {
	collector := NewCollector()
	collector.ObserveSendMessage(7 * time.Millisecond)
	collector.ObserveKafkaPublish(5 * time.Millisecond)

	request := httptest.NewRequest(http.MethodGet, "/debug/metrics", nil)
	response := httptest.NewRecorder()
	collector.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", response.Code)
	}
	var snapshot Snapshot
	if err := json.Unmarshal(response.Body.Bytes(), &snapshot); err != nil {
		t.Fatalf("decode metrics: %v", err)
	}
	if snapshot.SendMessageLatencyMS.Count != 1 ||
		snapshot.KafkaPublishLatencyMS.Count != 1 {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
}

func TestHandlerHealthz(t *testing.T) {
	handler := NewHandler(NewCollector(), nil)
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", response.Code)
	}
	var body healthResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode health response: %v", err)
	}
	if body.Service != "message-service" || body.Status != "ok" {
		t.Fatalf("unexpected health response: %+v", body)
	}
}

func TestHandlerReadyzWithoutPool(t *testing.T) {
	handler := NewHandler(NewCollector(), nil)
	request := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("unexpected status: %d", response.Code)
	}
	var body healthResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode ready response: %v", err)
	}
	if body.Status != "unready" || body.Error == "" {
		t.Fatalf("unexpected ready response: %+v", body)
	}
}

func TestHandlerMetricsIncludesOutboxRelaySnapshot(t *testing.T) {
	handler := NewHandler(NewCollector(), nil).WithOutboxRelayStats(func() types.OutboxRelayWorkerSnapshot {
		return types.OutboxRelayWorkerSnapshot{
			TotalErrors:        2,
			ConsecutiveErrors:  1,
			LastErrorAtMS:      100,
			LastSuccessAtMS:    90,
			LastPublishedAtMS:  90,
			LastErrorBackoffMS: 1000,
		}
	})
	request := httptest.NewRequest(http.MethodGet, "/debug/metrics", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", response.Code)
	}
	var snapshot Snapshot
	if err := json.Unmarshal(response.Body.Bytes(), &snapshot); err != nil {
		t.Fatalf("decode metrics: %v", err)
	}
	if snapshot.OutboxRelay == nil || snapshot.OutboxRelay.TotalErrors != 2 {
		t.Fatalf("unexpected outbox relay snapshot: %+v", snapshot.OutboxRelay)
	}
}

func TestHandlerMetricsIncludesTraceSnapshot(t *testing.T) {
	handler := NewHandler(NewCollector(), nil).WithTraceStats(func() monitoringinfra.TraceSnapshot {
		return monitoringinfra.TraceSnapshot{
			Enabled:       true,
			ServiceName:   "message-service",
			Exporter:      "stdout",
			SamplingRatio: 1,
		}
	})
	request := httptest.NewRequest(http.MethodGet, "/debug/metrics", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", response.Code)
	}
	var snapshot Snapshot
	if err := json.Unmarshal(response.Body.Bytes(), &snapshot); err != nil {
		t.Fatalf("decode metrics: %v", err)
	}
	if snapshot.Trace == nil || !snapshot.Trace.Enabled || snapshot.Trace.ServiceName != "message-service" {
		t.Fatalf("unexpected trace snapshot: %+v", snapshot.Trace)
	}
}

func TestHandlerPrometheusMetrics(t *testing.T) {
	collector := NewCollector()
	collector.ObserveSendMessage(40 * time.Millisecond)
	collector.ObserveRepositoryPoolAcquire(5 * time.Millisecond)
	collector.ObserveKafkaPublishCall(20*time.Millisecond, 4)
	collector.ObserveOutboxProcessReadyResult(10*time.Millisecond, 2)
	handler := NewHandler(collector, nil).
		WithOutboxRelayStats(func() types.OutboxRelayWorkerSnapshot {
			return types.OutboxRelayWorkerSnapshot{
				TotalErrors:        3,
				ConsecutiveErrors:  1,
				LastErrorAtMS:      100,
				LastSuccessAtMS:    200,
				LastPublishedAtMS:  300,
				LastErrorBackoffMS: 1500,
			}
		}).
		WithTraceStats(func() monitoringinfra.TraceSnapshot {
			return monitoringinfra.TraceSnapshot{Enabled: true, Exporter: "otlp-grpc", OTLPEndpointSet: true, SamplingRatio: 0.5}
		})

	request := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", response.Code)
	}
	if got := response.Header().Get("Content-Type"); got != "text/plain; version=0.0.4; charset=utf-8" {
		t.Fatalf("unexpected content type %q", got)
	}
	body := response.Body.String()
	assertContains(t, body, "# TYPE nexusim_message_latency_samples_total counter")
	assertContains(t, body, `nexusim_message_latency_samples_total{operation="send_message"} 1`)
	assertContains(t, body, `nexusim_message_latency_p95_milliseconds{operation="send_message"} 40`)
	assertContains(t, body, `nexusim_message_latency_p95_milliseconds{operation="send_message_recent"} 40`)
	assertContains(t, body, `nexusim_message_latency_p95_milliseconds{operation="repository_pool_acquire"} 5`)
	assertContains(t, body, `nexusim_message_latency_p95_milliseconds{operation="repository_pool_acquire_recent"} 5`)
	assertContains(t, body, `nexusim_message_value_avg{operation="kafka_publish_records_per_call"} 4`)
	assertContains(t, body, `nexusim_message_outbox_relay_errors_total 3`)
	assertContains(t, body, `nexusim_message_otel_traces_enabled{exporter="otlp-grpc"} 1`)
	for _, leaked := range []string{"secret-token", "tenant_id", "user_id", "device_id", "session_id", "request_id", "trace_id", "conversation_id", "message_id"} {
		if strings.Contains(body, leaked) {
			t.Fatalf("prometheus metrics leaked %q in body:\n%s", leaked, body)
		}
	}
}

func TestRenderPrometheusIncludesPoolAndTraceAggregates(t *testing.T) {
	body := renderPrometheus(Snapshot{
		SendMessageLatencyMS:           LatencySnapshot{Count: 2, AvgMS: 15, P50MS: 10, P95MS: 20, P99MS: 20, MaxMS: 20},
		RepositoryPoolAcquireLatencyMS: LatencySnapshot{Count: 1, AvgMS: 6, P50MS: 6, P95MS: 6, P99MS: 6, MaxMS: 6},
		PGPool:                         &PGPoolSnapshot{AcquireCount: 4, IdleConns: 2, TotalConns: 3, MaxConns: 8},
		Trace:                          &monitoringinfra.TraceSnapshot{Enabled: true, Exporter: "stdout", SamplingRatio: 1},
	})
	assertContains(t, body, `nexusim_message_latency_avg_milliseconds{operation="send_message"} 15`)
	assertContains(t, body, `nexusim_message_latency_p95_milliseconds{operation="repository_pool_acquire"} 6`)
	assertContains(t, body, `nexusim_message_pg_pool_conns{state="idle"} 2`)
	assertContains(t, body, `nexusim_message_pg_pool_conns{state="max"} 8`)
	assertContains(t, body, `nexusim_message_otel_traces_enabled{exporter="stdout"} 1`)
}

func TestPrometheusEscapesLabelValues(t *testing.T) {
	var builder strings.Builder
	writePrometheusSample(&builder, "nexusim_message_test", map[string]string{"operation": "send\"message\npath\\x"}, "1")
	assertContains(t, builder.String(), `operation="send\"message\npath\\x"`)
}

func assertContains(t *testing.T, body string, want string) {
	t.Helper()
	if !strings.Contains(body, want) {
		t.Fatalf("body missing %q:\n%s", want, body)
	}
}
