package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/action-executor/internal/types"
)

func TestValidateActionExecutorMode(t *testing.T) {
	for _, mode := range []string{"noop", "grpc", "provider-failure-worker"} {
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
