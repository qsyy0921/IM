package tool

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/qsyy0921/IM/services/action-executor/internal/types"
)

type LocalSafeExecutor struct{}

func NewLocalSafeExecutor() LocalSafeExecutor {
	return LocalSafeExecutor{}
}

func (executor LocalSafeExecutor) ExecuteTool(
	_ context.Context,
	command types.ToolExecutionCommand,
) (types.ToolExecutionResult, error) {
	if strings.TrimSpace(command.ToolName) != types.LocalSafeEchoToolName {
		return types.ToolExecutionResult{}, types.ErrToolExecutionUnsupported
	}
	if strings.ToUpper(strings.TrimSpace(command.RiskLevel)) != "LOW" {
		return types.ToolExecutionResult{}, types.ErrToolExecutionUnsupported
	}
	if !types.ToolActionAllowed(command.Skill.AllowedActions, types.ToolActionExecute) {
		return types.ToolExecutionResult{}, types.ErrToolExecutionUnsupported
	}
	payload := map[string]any{
		"schema_version": 1,
		"adapter":        "local-safe-echo",
		"tool_name":      command.ToolName,
		"resource_type":  command.ResourceType,
		"resource_id":    command.ResourceID,
		"input_sha256":   command.InputSHA256,
		"status":         "ok",
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return types.ToolExecutionResult{}, types.ErrToolExecutionFailed
	}
	return types.ToolExecutionResult{
		Executed:   true,
		OutputJSON: string(encoded),
	}, nil
}
