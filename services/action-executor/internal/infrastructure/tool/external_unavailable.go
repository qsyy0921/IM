package tool

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/qsyy0921/IM/services/action-executor/internal/types"
)

const (
	ExternalMCPFailureDisabled            = "disabled"
	ExternalMCPFailureProviderUnavailable = "provider-unavailable"
	ExternalMCPFailureTimeout             = "timeout"
	ExternalMCPFailureRateLimited         = "rate-limited"
	ExternalMCPFailurePermissionDenied    = "permission-denied"
	ExternalMCPFailureFailed              = "failed"
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

type ExternalMCPFailureExecutor struct {
	mode string
}

func NewExternalMCPFailureExecutor(mode string) (ExternalMCPFailureExecutor, error) {
	mode = normalizeExternalMCPFailureMode(mode)
	switch mode {
	case ExternalMCPFailureDisabled,
		ExternalMCPFailureProviderUnavailable,
		ExternalMCPFailureTimeout,
		ExternalMCPFailureRateLimited,
		ExternalMCPFailurePermissionDenied,
		ExternalMCPFailureFailed:
		return ExternalMCPFailureExecutor{mode: mode}, nil
	default:
		return ExternalMCPFailureExecutor{}, fmt.Errorf("unsupported external MCP failure mode %q", mode)
	}
}

func (executor ExternalMCPFailureExecutor) ExecuteTool(
	_ context.Context,
	command types.ToolExecutionCommand,
) (types.ToolExecutionResult, error) {
	if strings.TrimSpace(command.ToolName) == "" || strings.TrimSpace(command.ToolName) == types.LocalSafeEchoToolName {
		return types.ToolExecutionResult{}, types.ErrToolExecutionUnsupported
	}
	switch executor.mode {
	case ExternalMCPFailureProviderUnavailable:
		return types.ToolExecutionResult{}, types.ErrToolProviderUnavailable
	case ExternalMCPFailureTimeout:
		return types.ToolExecutionResult{}, types.ErrToolExecutionTimeout
	case ExternalMCPFailureRateLimited:
		return types.ToolExecutionResult{}, types.ErrToolProviderRateLimited
	case ExternalMCPFailurePermissionDenied:
		return types.ToolExecutionResult{}, types.ErrToolProviderPermissionDenied
	case ExternalMCPFailureFailed:
		return types.ToolExecutionResult{}, types.ErrToolExecutionFailed
	default:
		return types.ToolExecutionResult{}, types.ErrToolExecutionUnsupported
	}
}

func normalizeExternalMCPFailureMode(mode string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		return ExternalMCPFailureDisabled
	}
	mode = strings.ReplaceAll(mode, "_", "-")
	return mode
}
