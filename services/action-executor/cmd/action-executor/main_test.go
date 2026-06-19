package main

import (
	"context"
	"errors"
	"testing"

	"github.com/qsyy0921/IM/services/action-executor/internal/types"
)

func TestValidateActionExecutorMode(t *testing.T) {
	for _, mode := range []string{"noop", "grpc"} {
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
	t.Setenv("NEXUSIM_ACTION_EXECUTOR_EXTERNAL_MCP_FALLBACK_MODE", "")
	executor, err := newToolExecutorFromEnv()
	if err != nil {
		t.Fatalf("new executor: %v", err)
	}
	_, err = executor.ExecuteTool(context.Background(), types.ToolExecutionCommand{ToolName: "external.tool"})
	if !errors.Is(err, types.ErrToolExecutionUnsupported) {
		t.Fatalf("expected external tool to remain unsupported by default, got %v", err)
	}
}

func TestNewToolExecutorFromEnvCanEnableExternalMCPFailureFallback(t *testing.T) {
	t.Setenv("NEXUSIM_ACTION_EXECUTOR_EXTERNAL_MCP_FALLBACK_MODE", "timeout")
	executor, err := newToolExecutorFromEnv()
	if err != nil {
		t.Fatalf("new executor: %v", err)
	}
	_, err = executor.ExecuteTool(context.Background(), types.ToolExecutionCommand{ToolName: "external.tool"})
	if !errors.Is(err, types.ErrToolExecutionTimeout) {
		t.Fatalf("expected timeout fallback, got %v", err)
	}
}

func TestNewToolExecutorFromEnvRejectsUnknownExternalMCPFallbackMode(t *testing.T) {
	t.Setenv("NEXUSIM_ACTION_EXECUTOR_EXTERNAL_MCP_FALLBACK_MODE", "live")
	if _, err := newToolExecutorFromEnv(); err == nil {
		t.Fatal("expected unknown fallback mode to fail closed")
	}
}
