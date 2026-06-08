package main

import (
	"testing"
	"time"
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

func TestApplyLatency(t *testing.T) {
	var avg *float64
	var p95 *float64
	var p99 *float64
	applyLatency(&avg, &p95, &p99, latencySnapshot{Count: 2, AvgMS: 1.5, P95MS: 2.5, P99MS: 3.5})
	if avg == nil || *avg != 1.5 || p95 == nil || *p95 != 2.5 || p99 == nil || *p99 != 3.5 {
		t.Fatalf("unexpected latency values avg=%v p95=%v p99=%v", avg, p95, p99)
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
		cfg.RelayMetricsURL != "127.0.0.1:10499" {
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
