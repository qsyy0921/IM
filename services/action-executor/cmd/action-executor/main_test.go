package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/action-executor/internal/types"
)

func TestValidateActionExecutorMode(t *testing.T) {
	for _, mode := range []string{"noop", "grpc", "provider-failure-worker", "provider-failure-audit", "provider-failure-redrive-plan", "provider-replay-operator-ui"} {
		if err := validateActionExecutorMode(mode); err != nil {
			t.Fatalf("mode %s should be valid: %v", mode, err)
		}
	}
	if err := validateActionExecutorMode("worker"); err == nil {
		t.Fatal("expected invalid mode")
	}
}

func TestValidateActionExecutorDebugListenerConfigAllowsEmptyOrPrivateAddress(t *testing.T) {
	for _, addr := range []string{"", "127.0.0.1:11925", "localhost:11925", "172.30.80.28:11925"} {
		if err := validateActionExecutorDebugListenerConfig(addr, false); err != nil {
			t.Fatalf("addr %q should be allowed: %v", addr, err)
		}
	}
}

func TestValidateActionExecutorDebugListenerConfigRejectsPublicAddressByDefault(t *testing.T) {
	if err := validateActionExecutorDebugListenerConfig("8.8.8.8:11925", false); err == nil {
		t.Fatal("expected public debug address to be rejected")
	}
}

func TestValidateActionExecutorDebugListenerConfigAllowsExplicitPublicOptIn(t *testing.T) {
	if err := validateActionExecutorDebugListenerConfig("8.8.8.8:11925", true); err != nil {
		t.Fatalf("explicit public opt-in should be allowed: %v", err)
	}
}

func TestNewToolExecutorFromEnvDefaultsToUnsupportedExternalTool(t *testing.T) {
	t.Setenv("NEXUSIM_ACTION_EXECUTOR_EXTERNAL_MCP_FAILURE_MODE", "")
	executor, closeExecutor, err := newToolExecutorFromEnv(500 * time.Millisecond)
	if err != nil {
		t.Fatalf("new executor: %v", err)
	}
	defer closeExecutor()
	_, err = executor.ExecuteTool(context.Background(), types.ToolExecutionCommand{ToolName: "external.tool"})
	if !errors.Is(err, types.ErrToolExecutionUnsupported) {
		t.Fatalf("expected external tool to remain unsupported by default, got %v", err)
	}
}

func TestNewToolExecutorFromEnvCanEnableExternalMCPFailureMode(t *testing.T) {
	t.Setenv("NEXUSIM_ACTION_EXECUTOR_EXTERNAL_MCP_FAILURE_MODE", "timeout")
	executor, closeExecutor, err := newToolExecutorFromEnv(500 * time.Millisecond)
	if err != nil {
		t.Fatalf("new executor: %v", err)
	}
	defer closeExecutor()
	_, err = executor.ExecuteTool(context.Background(), types.ToolExecutionCommand{ToolName: "external.tool"})
	if !errors.Is(err, types.ErrToolExecutionTimeout) {
		t.Fatalf("expected timeout failure, got %v", err)
	}
}

func TestNewToolExecutorFromEnvCanEnableExternalHTTPAdapter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload["tool_name"] != "provider.safe.echo" || payload["input_sha256"] != "abc123" {
			t.Fatalf("unexpected request payload: %+v", payload)
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"schema_version": 1,
			"status":         "ok",
			"adapter":        "external-http",
		})
	}))
	defer server.Close()
	t.Setenv("NEXUSIM_ACTION_EXECUTOR_EXTERNAL_MCP_ADAPTER_MODE", "http")
	t.Setenv("NEXUSIM_ACTION_EXECUTOR_EXTERNAL_MCP_HTTP_ENDPOINT", server.URL)
	t.Setenv("NEXUSIM_ACTION_EXECUTOR_EXTERNAL_MCP_ALLOWED_TOOLS", "provider.safe.echo")

	executor, closeExecutor, err := newToolExecutorFromEnv(500 * time.Millisecond)
	if err != nil {
		t.Fatalf("new executor: %v", err)
	}
	defer closeExecutor()
	result, err := executor.ExecuteTool(context.Background(), types.ToolExecutionCommand{
		ToolName:    "provider.safe.echo",
		RiskLevel:   "LOW",
		InputSHA256: "abc123",
	})
	if err != nil {
		t.Fatalf("execute external HTTP adapter: %v", err)
	}
	if !result.Executed || result.OutputJSON == "" {
		t.Fatalf("expected executed external result: %+v", result)
	}
}

func TestNewToolExecutorFromEnvRequiresExternalHTTPAllowlist(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()
	t.Setenv("NEXUSIM_ACTION_EXECUTOR_EXTERNAL_MCP_ADAPTER_MODE", "http")
	t.Setenv("NEXUSIM_ACTION_EXECUTOR_EXTERNAL_MCP_HTTP_ENDPOINT", server.URL)

	if _, _, err := newToolExecutorFromEnv(500 * time.Millisecond); err == nil {
		t.Fatal("expected missing external HTTP allowlist to fail closed")
	}
}

func TestNewToolExecutorFromEnvRejectsUnknownExternalMCPAdapterMode(t *testing.T) {
	t.Setenv("NEXUSIM_ACTION_EXECUTOR_EXTERNAL_MCP_ADAPTER_MODE", "live")
	if _, _, err := newToolExecutorFromEnv(500 * time.Millisecond); err == nil {
		t.Fatal("expected unknown external adapter mode to fail closed")
	}
}

func TestNewToolExecutorFromEnvRejectsUnknownExternalMCPFailureMode(t *testing.T) {
	t.Setenv("NEXUSIM_ACTION_EXECUTOR_EXTERNAL_MCP_FAILURE_MODE", "live")
	if _, _, err := newToolExecutorFromEnv(500 * time.Millisecond); err == nil {
		t.Fatal("expected unknown failure mode to fail closed")
	}
}

func TestDebugMetricsIncludesProviderFailureSnapshotLowSensitive(t *testing.T) {
	handler := newDebugHandler(fakeProviderFailureMetricsStore{
		snapshot: types.ProviderFailureMetricsSnapshot{
			Total:        3,
			RetryPending: 2,
			DLQ:          1,
			Retryable:    2,
			DueRetry:     1,
			ByClass: []types.ProviderFailureMetricCount{
				{Status: types.ProviderFailureStatusDLQ, Classification: "TOOL_OUTPUT_UNSAFE", Count: 1},
				{Status: types.ProviderFailureStatusRetryPending, Classification: "TOOL_PROVIDER_UNAVAILABLE", Count: 2},
			},
		},
	})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	for _, want := range []string{
		`nexusim_action_executor_provider_failure_metrics_error 0`,
		`nexusim_action_executor_provider_failures_total{status="ALL"} 3`,
		`nexusim_action_executor_provider_failures_total{status="RETRY_PENDING"} 2`,
		`nexusim_action_executor_provider_failures_total{status="DLQ"} 1`,
		`nexusim_action_executor_provider_failures_retry_due_total 1`,
		`classification="TOOL_OUTPUT_UNSAFE"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics body missing %q: %s", want, body)
		}
	}
	for _, forbidden := range []string{"user-", "provider raw", "secret"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("metrics leaked %q: %s", forbidden, body)
		}
	}
}

func TestDebugMetricsFailClosedWhenProviderFailureMetricsUnavailable(t *testing.T) {
	handler := newDebugHandler(fakeProviderFailureMetricsStore{
		err: errors.New("postgres password=secret unavailable"),
	})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/debug/metrics", nil))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("unexpected status: %d body=%s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	if !strings.Contains(body, "nexusim_action_executor_provider_failure_metrics_error 1") ||
		!strings.Contains(body, "provider failure metrics unavailable") {
		t.Fatalf("unexpected failure metrics body: %s", body)
	}
	if strings.Contains(body, "password=secret") {
		t.Fatalf("metrics failure leaked internal error: %s", body)
	}
}

type fakeProviderFailureMetricsStore struct {
	snapshot types.ProviderFailureMetricsSnapshot
	err      error
}

func (store fakeProviderFailureMetricsStore) ProviderFailureMetrics(context.Context) (types.ProviderFailureMetricsSnapshot, error) {
	if store.err != nil {
		return types.ProviderFailureMetricsSnapshot{}, store.err
	}
	return store.snapshot, nil
}
