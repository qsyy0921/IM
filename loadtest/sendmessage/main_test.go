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

func TestParseConfigUsesEnvironment(t *testing.T) {
	env := map[string]string{
		"NEXUSIM_TARGET":             "127.0.0.1:10495",
		"NEXUSIM_VUS":                "3",
		"NEXUSIM_DURATION":           "5s",
		"NEXUSIM_RESULT_DIR":         "loadtest/results/test",
		"NEXUSIM_CONVERSATION_COUNT": "2",
	}
	cfg, err := parseConfig(nil, func(name string) string { return env[name] })
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	if cfg.Target != "127.0.0.1:10495" ||
		cfg.VUs != 3 ||
		cfg.Duration != 5*time.Second ||
		cfg.ResultDir != "loadtest/results/test" ||
		cfg.ConversationCount != 2 {
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
