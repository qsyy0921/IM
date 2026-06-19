package tool

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

func TestExternalHTTPExecutorPostsLowSensitiveRequest(t *testing.T) {
	var received externalToolRequest
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			t.Fatalf("unexpected method %s", request.Method)
		}
		if request.Header.Get("Authorization") != "Bearer provider-token" {
			t.Fatal("authorization header missing")
		}
		if err := json.NewDecoder(request.Body).Decode(&received); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"schema_version": 1,
			"adapter":        "external-http",
			"status":         "ok",
			"result_ref":     "provider://result/01",
		})
	}))
	defer server.Close()

	executor, err := NewExternalHTTPExecutor(ExternalHTTPExecutorOptions{
		Endpoint:     server.URL,
		BearerToken:  "provider-token",
		AllowedTools: []string{"provider.safe.echo"},
		Timeout:      time.Second,
	})
	if err != nil {
		t.Fatalf("new executor: %v", err)
	}
	result, err := executor.ExecuteTool(context.Background(), types.ToolExecutionCommand{
		Skill:        types.SkillDefinition{AllowedActions: []string{types.ToolActionExecute}},
		ToolName:     "provider.safe.echo",
		Action:       types.ToolActionExecute,
		ResourceType: "conversation",
		ResourceID:   "conv-1",
		RiskLevel:    "LOW",
		Intent:       "run safe external provider",
		InputSHA256:  "abc123",
	})
	if err != nil {
		t.Fatalf("execute external tool: %v", err)
	}
	if !result.Executed || result.OutputJSON == "" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if received.ToolName != "provider.safe.echo" ||
		received.Action != types.ToolActionExecute ||
		received.InputSHA256 != "abc123" ||
		!received.IntentPresent {
		t.Fatalf("unexpected request: %+v", received)
	}
}

func TestExternalHTTPExecutorDoesNotSendRawInputJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if _, ok := payload["input_json"]; ok {
			t.Fatalf("external adapter must not send raw input_json: %+v", payload)
		}
		if payload["input_sha256"] != "input-hash-only" {
			t.Fatalf("expected input hash only, got %+v", payload)
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"schema_version": 1,
			"status":         "ok",
		})
	}))
	defer server.Close()

	executor, err := NewExternalHTTPExecutor(ExternalHTTPExecutorOptions{
		Endpoint:     server.URL,
		AllowedTools: []string{"provider.safe.echo"},
	})
	if err != nil {
		t.Fatalf("new executor: %v", err)
	}
	if _, err := executor.ExecuteTool(context.Background(), types.ToolExecutionCommand{
		ToolName:    "provider.safe.echo",
		RiskLevel:   "LOW",
		InputSHA256: "input-hash-only",
	}); err != nil {
		t.Fatalf("execute external tool: %v", err)
	}
}

func TestExternalHTTPExecutorFailsClosedWhenToolNotAllowlistedOrHighRisk(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("server should not be called")
	}))
	defer server.Close()
	executor, err := NewExternalHTTPExecutor(ExternalHTTPExecutorOptions{
		Endpoint:     server.URL,
		AllowedTools: []string{"provider.safe.echo"},
	})
	if err != nil {
		t.Fatalf("new executor: %v", err)
	}
	for _, command := range []types.ToolExecutionCommand{
		{ToolName: "provider.other", RiskLevel: "LOW"},
		{ToolName: "provider.safe.echo", RiskLevel: "MEDIUM"},
		{ToolName: types.LocalSafeEchoToolName, RiskLevel: "LOW"},
	} {
		_, err := executor.ExecuteTool(context.Background(), command)
		if !errors.Is(err, types.ErrToolExecutionUnsupported) {
			t.Fatalf("expected unsupported for %+v, got %v", command, err)
		}
	}
}

func TestExternalHTTPExecutorClassifiesProviderStatus(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		expected   error
	}{
		{name: "permission", statusCode: http.StatusForbidden, expected: types.ErrToolProviderPermissionDenied},
		{name: "rate limited", statusCode: http.StatusTooManyRequests, expected: types.ErrToolProviderRateLimited},
		{name: "timeout", statusCode: http.StatusGatewayTimeout, expected: types.ErrToolExecutionTimeout},
		{name: "unavailable", statusCode: http.StatusBadGateway, expected: types.ErrToolProviderUnavailable},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(testCase.statusCode)
				_, _ = writer.Write([]byte(`{"provider_error":"raw secret body should not leak"}`))
			}))
			defer server.Close()
			executor, err := NewExternalHTTPExecutor(ExternalHTTPExecutorOptions{
				Endpoint:     server.URL,
				AllowedTools: []string{"provider.safe.echo"},
			})
			if err != nil {
				t.Fatalf("new executor: %v", err)
			}
			_, err = executor.ExecuteTool(context.Background(), types.ToolExecutionCommand{
				ToolName:  "provider.safe.echo",
				RiskLevel: "LOW",
			})
			if !errors.Is(err, testCase.expected) {
				t.Fatalf("expected %v, got %v", testCase.expected, err)
			}
		})
	}
}

func TestExternalHTTPExecutorRejectsPublicPlainHTTPAndMissingAllowlist(t *testing.T) {
	if _, err := NewExternalHTTPExecutor(ExternalHTTPExecutorOptions{
		Endpoint:     "http://example.com/tool",
		AllowedTools: []string{"provider.safe.echo"},
	}); err == nil {
		t.Fatal("expected public plain HTTP endpoint rejection")
	}
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()
	if _, err := NewExternalHTTPExecutor(ExternalHTTPExecutorOptions{Endpoint: server.URL}); err == nil {
		t.Fatal("expected missing allowed tools rejection")
	}
}

func TestExternalHTTPExecutorRejectsOversizedOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{"status":"` + string(make([]byte, 32)) + `"}`))
	}))
	defer server.Close()
	executor, err := NewExternalHTTPExecutor(ExternalHTTPExecutorOptions{
		Endpoint:         server.URL,
		AllowedTools:     []string{"provider.safe.echo"},
		MaxResponseBytes: 8,
	})
	if err != nil {
		t.Fatalf("new executor: %v", err)
	}
	_, err = executor.ExecuteTool(context.Background(), types.ToolExecutionCommand{
		ToolName:  "provider.safe.echo",
		RiskLevel: "LOW",
	})
	if !errors.Is(err, types.ErrToolOutputUnsafe) {
		t.Fatalf("expected unsafe output, got %v", err)
	}
}
