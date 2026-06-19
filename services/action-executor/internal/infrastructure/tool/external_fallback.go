package tool

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/qsyy0921/IM/services/action-executor/internal/types"
)

const (
	ExternalMCPFallbackDisabled            = "disabled"
	ExternalMCPFallbackProviderUnavailable = "provider-unavailable"
	ExternalMCPFallbackTimeout             = "timeout"
	ExternalMCPFallbackRateLimited         = "rate-limited"
	ExternalMCPFallbackPermissionDenied    = "permission-denied"
	ExternalMCPFallbackFailed              = "failed"
)

type ExecutorChain struct {
	executors []Executor
}

type Executor interface {
	ExecuteTool(context.Context, types.ToolExecutionCommand) (types.ToolExecutionResult, error)
}

func NewExecutorChain(executors ...Executor) ExecutorChain {
	return ExecutorChain{executors: executors}
}

func (chain ExecutorChain) ExecuteTool(
	ctx context.Context,
	command types.ToolExecutionCommand,
) (types.ToolExecutionResult, error) {
	for _, executor := range chain.executors {
		if executor == nil {
			continue
		}
		result, err := executor.ExecuteTool(ctx, command)
		if err == nil || !errors.Is(err, types.ErrToolExecutionUnsupported) {
			return result, err
		}
	}
	return types.ToolExecutionResult{}, types.ErrToolExecutionUnsupported
}

type ExternalMCPFallbackExecutor struct {
	mode string
}

func NewExternalMCPFallbackExecutor(mode string) (ExternalMCPFallbackExecutor, error) {
	mode = normalizeExternalMCPFallbackMode(mode)
	switch mode {
	case ExternalMCPFallbackDisabled,
		ExternalMCPFallbackProviderUnavailable,
		ExternalMCPFallbackTimeout,
		ExternalMCPFallbackRateLimited,
		ExternalMCPFallbackPermissionDenied,
		ExternalMCPFallbackFailed:
		return ExternalMCPFallbackExecutor{mode: mode}, nil
	default:
		return ExternalMCPFallbackExecutor{}, fmt.Errorf("unsupported external MCP fallback mode %q", mode)
	}
}

func (executor ExternalMCPFallbackExecutor) ExecuteTool(
	_ context.Context,
	command types.ToolExecutionCommand,
) (types.ToolExecutionResult, error) {
	if strings.TrimSpace(command.ToolName) == "" || strings.TrimSpace(command.ToolName) == types.LocalSafeEchoToolName {
		return types.ToolExecutionResult{}, types.ErrToolExecutionUnsupported
	}
	switch executor.mode {
	case ExternalMCPFallbackProviderUnavailable:
		return types.ToolExecutionResult{}, types.ErrToolProviderUnavailable
	case ExternalMCPFallbackTimeout:
		return types.ToolExecutionResult{}, types.ErrToolExecutionTimeout
	case ExternalMCPFallbackRateLimited:
		return types.ToolExecutionResult{}, types.ErrToolProviderRateLimited
	case ExternalMCPFallbackPermissionDenied:
		return types.ToolExecutionResult{}, types.ErrToolProviderPermissionDenied
	case ExternalMCPFallbackFailed:
		return types.ToolExecutionResult{}, types.ErrToolExecutionFailed
	default:
		return types.ToolExecutionResult{}, types.ErrToolExecutionUnsupported
	}
}

func normalizeExternalMCPFallbackMode(mode string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		return ExternalMCPFallbackDisabled
	}
	mode = strings.ReplaceAll(mode, "_", "-")
	return mode
}
