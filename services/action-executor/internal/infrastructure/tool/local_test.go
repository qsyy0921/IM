package tool

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/qsyy0921/IM/services/action-executor/internal/types"
)

func TestLocalSafeExecutorExecutesOnlyLowRiskEcho(t *testing.T) {
	executor := NewLocalSafeExecutor()
	result, err := executor.ExecuteTool(context.Background(), types.ToolExecutionCommand{
		Skill: types.SkillDefinition{
			AllowedActions: []string{types.ToolActionExecute},
		},
		ToolName:     types.LocalSafeEchoToolName,
		Action:       types.ToolActionExecute,
		ResourceType: "diagnostic",
		ResourceID:   "res-1",
		RiskLevel:    "LOW",
		InputSHA256:  "abc123",
	})
	if err != nil {
		t.Fatalf("execute local echo: %v", err)
	}
	if !result.Executed {
		t.Fatalf("expected executed result: %+v", result)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(result.OutputJSON), &payload); err != nil {
		t.Fatalf("output must be json: %v", err)
	}
	if payload["input_sha256"] != "abc123" || payload["tool_name"] != types.LocalSafeEchoToolName {
		t.Fatalf("unexpected low-sensitive payload: %+v", payload)
	}
}

func TestLocalSafeExecutorRejectsUnsupportedOrHigherRiskTool(t *testing.T) {
	executor := NewLocalSafeExecutor()
	for _, command := range []types.ToolExecutionCommand{
		{ToolName: "conversation.note.create", RiskLevel: "LOW", Skill: types.SkillDefinition{AllowedActions: []string{types.ToolActionExecute}}},
		{ToolName: types.LocalSafeEchoToolName, RiskLevel: "HIGH", Skill: types.SkillDefinition{AllowedActions: []string{types.ToolActionExecute}}},
		{ToolName: types.LocalSafeEchoToolName, RiskLevel: "LOW", Skill: types.SkillDefinition{AllowedActions: []string{types.ToolActionCall}}},
	} {
		if _, err := executor.ExecuteTool(context.Background(), command); !errors.Is(err, types.ErrToolExecutionUnsupported) {
			t.Fatalf("expected unsupported for %+v, got %v", command, err)
		}
	}
}
