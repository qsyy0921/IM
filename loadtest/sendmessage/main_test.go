package main

import (
	"math"
	"testing"
	"time"

	messagev1 "github.com/qsyy0921/IM/api/proto/nexusim/message/v1"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
)

func TestNormalizeTarget(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{input: "127.0.0.1:10495", want: "127.0.0.1:10495"},
		{input: "http://192.168.0.141:10495", want: "192.168.0.141:10495"},
		{input: "grpc://localhost:10495", want: "localhost:10495"},
	}

	for _, tc := range cases {
		got, err := normalizeTarget(tc.input)
		if err != nil {
			t.Fatalf("normalize target %q: %v", tc.input, err)
		}
		if got != tc.want {
			t.Fatalf("normalize target %q: got %q want %q", tc.input, got, tc.want)
		}
	}
}

func TestNormalizeTargets(t *testing.T) {
	got, err := normalizeTargets("127.0.0.1:10495, http://127.0.0.1:10501,grpc://localhost:10502")
	if err != nil {
		t.Fatalf("normalize targets: %v", err)
	}
	want := []string{"127.0.0.1:10495", "127.0.0.1:10501", "localhost:10502"}
	if len(got) != len(want) {
		t.Fatalf("targets length got %d want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("target[%d] got %q want %q", i, got[i], want[i])
		}
	}
}

func TestNormalizeMetricsURL(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{input: "127.0.0.1:10498", want: "http://127.0.0.1:10498/debug/metrics"},
		{input: "http://127.0.0.1:10498", want: "http://127.0.0.1:10498/debug/metrics"},
		{input: "http://127.0.0.1:10498/debug/metrics", want: "http://127.0.0.1:10498/debug/metrics"},
	}
	for _, tc := range cases {
		got, err := normalizeMetricsURL(tc.input)
		if err != nil {
			t.Fatalf("normalize metrics URL %q: %v", tc.input, err)
		}
		if got != tc.want {
			t.Fatalf("normalize metrics URL %q: got %q want %q", tc.input, got, tc.want)
		}
	}
}

func TestNormalizeMetricsURLs(t *testing.T) {
	got, err := normalizeMetricsURLs("127.0.0.1:10498, http://127.0.0.1:10598/debug/metrics")
	if err != nil {
		t.Fatalf("normalize metrics URLs: %v", err)
	}
	want := []string{
		"http://127.0.0.1:10498/debug/metrics",
		"http://127.0.0.1:10598/debug/metrics",
	}
	if len(got) != len(want) {
		t.Fatalf("metrics URL length got %d want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("metrics URL[%d] got %q want %q", i, got[i], want[i])
		}
	}
}

func TestPercentile(t *testing.T) {
	values := []time.Duration{
		10 * time.Millisecond,
		20 * time.Millisecond,
		30 * time.Millisecond,
		40 * time.Millisecond,
		50 * time.Millisecond,
	}

	if got := percentile(values, 0.50); got != 30*time.Millisecond {
		t.Fatalf("p50 got %s", got)
	}
	if got := percentile(values, 0.95); got != 50*time.Millisecond {
		t.Fatalf("p95 got %s", got)
	}
}

func TestSummarizeLatencies(t *testing.T) {
	values := []time.Duration{
		10 * time.Millisecond,
		20 * time.Millisecond,
		30 * time.Millisecond,
		40 * time.Millisecond,
	}
	summary := summarizeLatencies(values, 100*time.Millisecond)
	if summary.AvgMS != 25 ||
		summary.P50MS != 20 ||
		summary.P95MS != 40 ||
		summary.P99MS != 40 {
		t.Fatalf("unexpected latency summary: %+v", summary)
	}
	if empty := summarizeLatencies(nil, 0); empty.AvgMS != 0 || empty.P99MS != 0 {
		t.Fatalf("unexpected empty summary: %+v", empty)
	}
}

func TestApplyLatency(t *testing.T) {
	var avg *float64
	var p95 *float64
	var p99 *float64
	applyLatency(&avg, &p95, &p99, latencySnapshot{Count: 2, AvgMS: 1.5, P95MS: 2.5, P99MS: 3.5})
	if avg == nil || *avg != 1.5 || p95 == nil || *p95 != 2.5 || p99 == nil || *p99 != 3.5 {
		t.Fatalf("unexpected latency values avg=%v p95=%v p99=%v", avg, p95, p99)
	}
}

func TestAggregateProcessLatencyMetrics(t *testing.T) {
	metrics := aggregateProcessLatencyMetrics([]processMetrics{
		{
			URL: "http://127.0.0.1:10498/debug/metrics",
			Snapshot: metricsSnapshot{
				RepositoryBeginLatencyMS:          latencySnapshot{Count: 2, AvgMS: 10, P95MS: 20, P99MS: 30},
				RepositoryPoolAcquireLatencyMS:    latencySnapshot{Count: 2, AvgMS: 8, P95MS: 18, P99MS: 28},
				OutboxProcessReadyLatencyMS:       latencySnapshot{Count: 2, AvgMS: 4, P95MS: 5, P99MS: 6},
				OutboxProcessReadyActiveLatencyMS: latencySnapshot{Count: 1, AvgMS: 7, P95MS: 7, P99MS: 7},
				OutboxProcessReadyIdleLatencyMS:   latencySnapshot{Count: 1, AvgMS: 1, P95MS: 1, P99MS: 1},
				KafkaPublishRecordsPerCall:        valueSnapshot{Count: 2, Avg: 10, P95: 12, P99: 12},
				OutboxFetchedPerCall:              valueSnapshot{Count: 2, Avg: 5, P95: 8, P99: 8},
			},
		},
		{
			URL: "http://127.0.0.1:10598/debug/metrics",
			Snapshot: metricsSnapshot{
				RepositoryBeginLatencyMS:          latencySnapshot{Count: 3, AvgMS: 20, P95MS: 25, P99MS: 40},
				RepositoryPoolAcquireLatencyMS:    latencySnapshot{Count: 3, AvgMS: 18, P95MS: 23, P99MS: 38},
				OutboxProcessReadyLatencyMS:       latencySnapshot{Count: 3, AvgMS: 8, P95MS: 9, P99MS: 10},
				OutboxProcessReadyActiveLatencyMS: latencySnapshot{Count: 2, AvgMS: 11, P95MS: 13, P99MS: 13},
				OutboxProcessReadyIdleLatencyMS:   latencySnapshot{Count: 1, AvgMS: 2, P95MS: 2, P99MS: 2},
				KafkaPublishRecordsPerCall:        valueSnapshot{Count: 3, Avg: 20, P95: 22, P99: 22},
				OutboxFetchedPerCall:              valueSnapshot{Count: 3, Avg: 15, P95: 20, P99: 20},
			},
		},
	})

	metric := metrics["repository_begin_latency_ms"]
	if metric.Count != 5 ||
		metric.AvgMS != 16 ||
		metric.P95MS != 25 ||
		metric.P99MS != 40 {
		t.Fatalf("unexpected aggregate metric: %+v", metric)
	}
	poolAcquire := metrics["repository_pool_acquire_latency_ms"]
	if poolAcquire.Count != 5 ||
		poolAcquire.AvgMS != 14 ||
		poolAcquire.P95MS != 23 ||
		poolAcquire.P99MS != 38 {
		t.Fatalf("unexpected aggregate pool acquire metric: %+v", poolAcquire)
	}
	outboxProcess := metrics["outbox_process_ready_latency_ms"]
	if outboxProcess.Count != 5 ||
		outboxProcess.AvgMS != 6.4 ||
		outboxProcess.P95MS != 9 ||
		outboxProcess.P99MS != 10 {
		t.Fatalf("unexpected aggregate outbox process metric: %+v", outboxProcess)
	}
	outboxActive := metrics["outbox_process_ready_active_latency_ms"]
	if outboxActive.Count != 3 ||
		math.Abs(outboxActive.AvgMS-(29.0/3.0)) > 0.000001 ||
		outboxActive.P95MS != 13 ||
		outboxActive.P99MS != 13 {
		t.Fatalf("unexpected aggregate active outbox process metric: %+v", outboxActive)
	}
	values := aggregateProcessValueMetrics([]processMetrics{
		{Snapshot: metricsSnapshot{
			KafkaPublishRecordsPerCall: valueSnapshot{Count: 2, Avg: 10, P95: 12, P99: 12},
			OutboxFetchedPerCall:       valueSnapshot{Count: 2, Avg: 5, P95: 8, P99: 8},
		}},
		{Snapshot: metricsSnapshot{
			KafkaPublishRecordsPerCall: valueSnapshot{Count: 3, Avg: 20, P95: 22, P99: 22},
			OutboxFetchedPerCall:       valueSnapshot{Count: 3, Avg: 15, P95: 20, P99: 20},
		}},
	})
	recordsPerCall := values["kafka_publish_records_per_call"]
	if recordsPerCall.Count != 5 ||
		recordsPerCall.Avg != 16 ||
		recordsPerCall.P95 != 22 ||
		recordsPerCall.P99 != 22 {
		t.Fatalf("unexpected aggregate records per call metric: %+v", recordsPerCall)
	}
	fetchedPerCall := values["outbox_fetched_per_call"]
	if fetchedPerCall.Count != 5 ||
		fetchedPerCall.Avg != 11 ||
		fetchedPerCall.P95 != 20 ||
		fetchedPerCall.P99 != 20 {
		t.Fatalf("unexpected aggregate fetched per call metric: %+v", fetchedPerCall)
	}
}

func TestAggregatePGPool(t *testing.T) {
	pool := aggregatePGPool([]processMetrics{
		{Snapshot: metricsSnapshot{PGPool: &pgPoolStats{AcquireCount: 2, AcquireDurationMS: 10, MaxConns: 16, TotalConns: 16}}},
		{Snapshot: metricsSnapshot{PGPool: &pgPoolStats{AcquireCount: 3, AcquireDurationMS: 30, MaxConns: 16, TotalConns: 12}}},
	})
	if pool == nil ||
		pool.AcquireCount != 5 ||
		pool.AcquireDurationMS != 40 ||
		pool.MaxConns != 32 ||
		pool.TotalConns != 28 {
		t.Fatalf("unexpected aggregate pg pool: %+v", pool)
	}
}

func TestMessageErrorDetailAndTopMessageErrors(t *testing.T) {
	st := status.New(codes.Unavailable, "service overloaded")
	withDetails, err := st.WithDetails(&messagev1.MessageError{
		Code:      messagev1.MessageErrorCode_MESSAGE_ERROR_CODE_SERVICE_OVERLOADED,
		Retryable: true,
	})
	if err != nil {
		t.Fatalf("attach detail: %v", err)
	}

	detail, ok := messageErrorDetail(withDetails.Err())
	if !ok {
		t.Fatalf("expected message error detail")
	}
	if detail.GetCode() != messagev1.MessageErrorCode_MESSAGE_ERROR_CODE_SERVICE_OVERLOADED ||
		!detail.GetRetryable() {
		t.Fatalf("unexpected detail: %+v", detail)
	}

	counts := topMessageErrors(map[messageErrorKey]int64{
		{Code: messagev1.MessageErrorCode_MESSAGE_ERROR_CODE_SERVICE_OVERLOADED, Retryable: true}: 3,
		{Code: messagev1.MessageErrorCode_MESSAGE_ERROR_CODE_DB_WRITE_FAILED, Retryable: true}:    1,
	}, 10)
	if len(counts) != 2 ||
		counts[0].Code != "MESSAGE_ERROR_CODE_SERVICE_OVERLOADED" ||
		!counts[0].Retryable ||
		counts[0].Count != 3 {
		t.Fatalf("unexpected counts: %+v", counts)
	}
}

func TestOverloadRetryDelayUsesRetryInfoAndJitter(t *testing.T) {
	st := status.New(codes.Unavailable, "service overloaded")
	withDetails, err := st.WithDetails(
		&messagev1.MessageError{
			Code:      messagev1.MessageErrorCode_MESSAGE_ERROR_CODE_SERVICE_OVERLOADED,
			Retryable: true,
		},
		&errdetails.RetryInfo{RetryDelay: durationpb.New(500 * time.Millisecond)},
	)
	if err != nil {
		t.Fatalf("attach detail: %v", err)
	}

	delay, ok := overloadRetryDelay(withDetails.Err(), 10*time.Millisecond, 7, 1)
	if !ok {
		t.Fatalf("expected overload retry delay")
	}
	if delay < 500*time.Millisecond || delay > 510*time.Millisecond {
		t.Fatalf("unexpected retry delay: %s", delay)
	}

	withoutRetryInfo, err := st.WithDetails(&messagev1.MessageError{
		Code:      messagev1.MessageErrorCode_MESSAGE_ERROR_CODE_SERVICE_OVERLOADED,
		Retryable: true,
	})
	if err != nil {
		t.Fatalf("attach detail without retry info: %v", err)
	}
	if _, ok := overloadRetryDelay(withoutRetryInfo.Err(), 0, 1, 0); ok {
		t.Fatalf("expected no retry without RetryInfo")
	}
}

func TestCommitInfoFromEnv(t *testing.T) {
	t.Setenv("NEXUSIM_COMMIT", "abc1234")
	t.Setenv("NEXUSIM_COMMIT_FULL", "abc1234full")
	t.Setenv("NEXUSIM_GIT_DIRTY", "true")
	t.Setenv("NEXUSIM_GIT_STATUS_SHORT", "M file.go")

	commit := commitInfoFromEnv()
	if commit.Short != "abc1234-dirty" ||
		commit.Full != "abc1234full" ||
		!commit.Dirty ||
		commit.StatusShort != "M file.go" {
		t.Fatalf("unexpected commit info: %+v", commit)
	}
}

func TestParseConfigUsesEnvironment(t *testing.T) {
	env := map[string]string{
		"NEXUSIM_TARGET":              "127.0.0.1:10495,127.0.0.1:10501",
		"NEXUSIM_VUS":                 "3",
		"NEXUSIM_DURATION":            "5s",
		"NEXUSIM_RESULT_DIR":          "loadtest/results/test",
		"NEXUSIM_CONVERSATION_COUNT":  "2",
		"NEXUSIM_SERVICE_METRICS_URL": "127.0.0.1:10498",
		"NEXUSIM_RELAY_METRICS_URL":   "127.0.0.1:10499",
		"NEXUSIM_RETRY_OVERLOADED":    "true",
		"NEXUSIM_MAX_RETRIES":         "2",
		"NEXUSIM_RETRY_JITTER":        "25ms",
	}
	cfg, err := parseConfig(nil, func(name string) string { return env[name] })
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	if cfg.Target != "127.0.0.1:10495,127.0.0.1:10501" ||
		cfg.VUs != 3 ||
		cfg.Duration != 5*time.Second ||
		cfg.ResultDir != "loadtest/results/test" ||
		cfg.ConversationCount != 2 ||
		cfg.ServiceMetricsURL != "127.0.0.1:10498" ||
		cfg.RelayMetricsURL != "127.0.0.1:10499" ||
		!cfg.RetryOverloaded ||
		cfg.MaxRetries != 2 ||
		cfg.RetryJitter != 25*time.Millisecond {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestBuildRequestUsesUniqueClientMessageID(t *testing.T) {
	cfg := config{
		TenantID:           "tenant-1",
		ConversationPrefix: "conv",
		ConversationCount:  2,
	}

	first := buildRequest(cfg, "run-1", 1, 1)
	second := buildRequest(cfg, "run-1", 1, 2)
	if first.GetClientMsgId() == second.GetClientMsgId() {
		t.Fatalf("client_msg_id should be unique")
	}
	if first.GetConversationId() == second.GetConversationId() {
		t.Fatalf("requests should spread across conversations")
	}
}
