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

func TestExecutorChainFallsBackToExternalMCPFailureMode(t *testing.T) {
	external, err := NewExternalMCPFailureExecutor("provider_unavailable")
	if err != nil {
		t.Fatalf("new external failure executor: %v", err)
	}
	executor := NewExecutorChain(NewLocalSafeExecutor(), external)
	_, err = executor.ExecuteTool(context.Background(), types.ToolExecutionCommand{
		ToolName:     "conversation.reply.send",
		RiskLevel:    "LOW",
		Skill:        types.SkillDefinition{AllowedActions: []string{types.ToolActionExecute}},
		ResourceType: "conversation",
		ResourceID:   "conv-1",
	})
	if !errors.Is(err, types.ErrToolProviderUnavailable) {
		t.Fatalf("expected provider unavailable failure, got %v", err)
	}
}

func TestExecutorChainKeepsLocalSafeToolExecution(t *testing.T) {
	external, err := NewExternalMCPFailureExecutor("provider-unavailable")
	if err != nil {
		t.Fatalf("new external failure executor: %v", err)
	}
	executor := NewExecutorChain(NewLocalSafeExecutor(), external)
	result, err := executor.ExecuteTool(context.Background(), types.ToolExecutionCommand{
		ToolName:    types.LocalSafeEchoToolName,
		Action:      types.ToolActionExecute,
		RiskLevel:   "LOW",
		InputSHA256: "abc123",
		Skill:       types.SkillDefinition{AllowedActions: []string{types.ToolActionExecute}},
	})
	if err != nil {
		t.Fatalf("local safe tool should execute before external failure executor: %v", err)
	}
	if !result.Executed {
		t.Fatalf("expected local safe execution: %+v", result)
	}
}

func TestExternalMCPFailureModes(t *testing.T) {
	cases := []struct {
		mode string
		err  error
	}{
		{mode: "disabled", err: types.ErrToolExecutionUnsupported},
		{mode: "timeout", err: types.ErrToolExecutionTimeout},
		{mode: "rate-limited", err: types.ErrToolProviderRateLimited},
		{mode: "permission-denied", err: types.ErrToolProviderPermissionDenied},
		{mode: "failed", err: types.ErrToolExecutionFailed},
	}
	for _, testCase := range cases {
		t.Run(testCase.mode, func(t *testing.T) {
			executor, err := NewExternalMCPFailureExecutor(testCase.mode)
			if err != nil {
				t.Fatalf("new external failure executor: %v", err)
			}
			_, err = executor.ExecuteTool(context.Background(), types.ToolExecutionCommand{ToolName: "external.tool"})
			if !errors.Is(err, testCase.err) {
				t.Fatalf("expected %v, got %v", testCase.err, err)
			}
		})
	}
}

func TestExternalMCPFailureRejectsUnknownMode(t *testing.T) {
	if _, err := NewExternalMCPFailureExecutor("execute-for-real"); err == nil {
		t.Fatal("expected unknown external failure mode to fail closed")
	}
}
