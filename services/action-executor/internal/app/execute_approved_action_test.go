package app

import (
	"context"
	"errors"
	"testing"

	"github.com/qsyy0921/IM/services/action-executor/internal/types"
)

func TestExecuteApprovedActionRecordsAllowedExecutionWithoutRunningExternalTool(t *testing.T) {
	audit := &fakeAuditRepository{}
	usecase := NewExecuteApprovedActionUseCase(
		fakeSkillCatalog{skill: activeSkill()},
		fakeToolPolicy{decision: types.ToolPolicyDecision{
			Allowed:           true,
			RequiresApproval:  true,
			PermissionVersion: 9,
			Classification:    "TOOL_ALLOWED",
			Reason:            "approved",
			DecisionSource:    "TOOL_RULE",
		}},
		fakeApproval{},
		audit,
	)
	result, err := usecase.Execute(context.Background(), validCommand())
	if err != nil {
		t.Fatalf("execute action: %v", err)
	}
	if result.Status != types.ExecutionStatusRecorded || !result.Allowed || result.Executed {
		t.Fatalf("unexpected result: %+v", result)
	}
	if result.ResultID == "" || result.ResultStatus != types.ResultStatusNotExecuted || result.ResultRef == "" {
		t.Fatalf("expected not-executed result projection fields, got %+v", result)
	}
	if len(audit.rows) != 1 {
		t.Fatalf("expected one audit row, got %d", len(audit.rows))
	}
	if len(audit.results) != 1 {
		t.Fatalf("expected one result projection, got %d", len(audit.results))
	}
	row := audit.rows[0]
	if row.InputSHA256 == "" || row.Executed || row.Status != types.ExecutionStatusRecorded {
		t.Fatalf("unexpected audit row: %+v", row)
	}
	projection := audit.results[0]
	if projection.ExecutionID != result.ExecutionID || projection.ResultID != result.ResultID || projection.Status != types.ResultStatusNotExecuted {
		t.Fatalf("unexpected result projection: %+v", projection)
	}
}

func TestExecuteApprovedActionRunsSupportedToolExecutor(t *testing.T) {
	audit := &fakeAuditRepository{}
	executor := &fakeToolExecutor{result: types.ToolExecutionResult{
		Executed:   true,
		OutputJSON: `{"adapter":"local-safe-echo","status":"ok"}`,
	}}
	usecase := NewExecuteApprovedActionUseCaseWithToolExecutor(
		fakeSkillCatalog{skill: activeSkill()},
		fakeToolPolicy{decision: types.ToolPolicyDecision{
			Allowed:           true,
			RequiresApproval:  true,
			PermissionVersion: 9,
			Classification:    "TOOL_ALLOWED",
			Reason:            "approved",
			DecisionSource:    "TOOL_RULE",
		}},
		fakeApproval{},
		audit,
		executor,
	)
	result, err := usecase.Execute(context.Background(), validCommand())
	if err != nil {
		t.Fatalf("execute action: %v", err)
	}
	if !result.Executed || result.ResultStatus != types.ResultStatusSucceeded || result.OutputJSON == "{}" {
		t.Fatalf("expected executed successful result, got %+v", result)
	}
	if executor.command.ToolName != validCommand().ToolName || executor.command.InputSHA256 == "" {
		t.Fatalf("unexpected tool executor command: %+v", executor.command)
	}
	if len(audit.rows) != 1 || !audit.rows[0].Executed || audit.rows[0].OutputSHA256 == "" {
		t.Fatalf("expected executed audit row with output hash, got %+v", audit.rows)
	}
	if len(audit.results) != 1 || !audit.results[0].Executed || audit.results[0].Status != types.ResultStatusSucceeded {
		t.Fatalf("expected successful result projection, got %+v", audit.results)
	}
}

func TestExecuteApprovedActionLeavesUnsupportedToolUnexecuted(t *testing.T) {
	audit := &fakeAuditRepository{}
	usecase := NewExecuteApprovedActionUseCaseWithToolExecutor(
		fakeSkillCatalog{skill: activeSkill()},
		fakeToolPolicy{decision: types.ToolPolicyDecision{
			Allowed:           true,
			RequiresApproval:  true,
			PermissionVersion: 9,
			Classification:    "TOOL_ALLOWED",
			Reason:            "approved",
			DecisionSource:    "TOOL_RULE",
		}},
		fakeApproval{},
		audit,
		&fakeToolExecutor{err: types.ErrToolExecutionUnsupported},
	)
	result, err := usecase.Execute(context.Background(), validCommand())
	if err != nil {
		t.Fatalf("execute action: %v", err)
	}
	if result.Executed || result.ResultStatus != types.ResultStatusNotExecuted {
		t.Fatalf("expected unsupported tool to remain not executed, got %+v", result)
	}
	if len(audit.rows) != 1 || audit.rows[0].Executed || audit.rows[0].OutputSHA256 != "" {
		t.Fatalf("expected not-executed audit row, got %+v", audit.rows)
	}
}

func TestExecuteApprovedActionRequiresApprovalAndPreparedAudit(t *testing.T) {
	command := validCommand()
	command.ApprovalID = ""
	usecase := NewExecuteApprovedActionUseCase(fakeSkillCatalog{}, fakeToolPolicy{}, fakeApproval{}, &fakeAuditRepository{})
	if _, err := usecase.Execute(context.Background(), command); !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid argument for missing approval: %v", err)
	}
	command = validCommand()
	command.PreparedAuditID = ""
	if _, err := usecase.Execute(context.Background(), command); !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid argument for missing prepared audit: %v", err)
	}
}

func TestExecuteApprovedActionRecordsPolicyDeny(t *testing.T) {
	audit := &fakeAuditRepository{}
	usecase := NewExecuteApprovedActionUseCase(
		fakeSkillCatalog{skill: activeSkill()},
		fakeToolPolicy{decision: types.ToolPolicyDecision{
			Allowed:          false,
			RequiresApproval: true,
			Classification:   "TOOL_DENIED",
			Reason:           "denied",
			DecisionSource:   "TOOL_RULE",
		}},
		fakeApproval{},
		audit,
	)
	result, err := usecase.Execute(context.Background(), validCommand())
	if err != nil {
		t.Fatalf("execute action: %v", err)
	}
	if result.Status != types.ExecutionStatusBlocked || result.Allowed {
		t.Fatalf("unexpected result: %+v", result)
	}
	if result.ResultStatus != types.ResultStatusBlocked {
		t.Fatalf("expected blocked result projection status, got %+v", result)
	}
	if len(audit.rows) != 1 || audit.rows[0].Status != types.ExecutionStatusBlocked {
		t.Fatalf("expected blocked audit row, got %+v", audit.rows)
	}
	if len(audit.results) != 1 || audit.results[0].Status != types.ResultStatusBlocked {
		t.Fatalf("expected blocked result projection, got %+v", audit.results)
	}
}

func TestExecuteApprovedActionRejectsUnapprovedProposalBeforeAudit(t *testing.T) {
	audit := &fakeAuditRepository{}
	usecase := NewExecuteApprovedActionUseCase(
		fakeSkillCatalog{skill: activeSkill()},
		fakeToolPolicy{},
		fakeApproval{err: types.ErrProposalNotApproved},
		audit,
	)
	_, err := usecase.Execute(context.Background(), validCommand())
	if !errors.Is(err, types.ErrProposalNotApproved) {
		t.Fatalf("expected proposal not approved: %v", err)
	}
	if len(audit.rows) != 0 {
		t.Fatalf("expected no audit rows for unapproved proposal, got %+v", audit.rows)
	}
}

func TestExecuteApprovedActionRequiresProposalApprovalPort(t *testing.T) {
	usecase := NewExecuteApprovedActionUseCase(fakeSkillCatalog{}, fakeToolPolicy{}, nil, &fakeAuditRepository{})
	_, err := usecase.Execute(context.Background(), validCommand())
	if !errors.Is(err, types.ErrProposalApprovalUnavailable) {
		t.Fatalf("expected proposal approval unavailable: %v", err)
	}
}

func validCommand() types.ExecuteApprovedActionCommand {
	return types.ExecuteApprovedActionCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-1",
			UserID:   "user-1",
			DeviceID: "device-1",
		},
		ProposalID:      "proposal-1",
		ApprovalID:      "approval-1",
		PreparedAuditID: "mcp-audit-1",
		SkillID:         "skill-1",
		ToolName:        "conversation.reply.send",
		Action:          types.ToolActionExecute,
		ResourceType:    "conversation",
		ResourceID:      "conv-1",
		RiskLevel:       "LOW",
		Intent:          "send approved reply",
		InputJSON:       `{"body":"hello"}`,
		IdempotencyKey:  "idem-1",
	}
}

func activeSkill() types.SkillDefinition {
	return types.SkillDefinition{
		TenantID:         "tenant-1",
		SkillID:          "skill-1",
		Status:           types.SkillStatusActive,
		ToolName:         "conversation.reply.send",
		AllowedActions:   []string{types.ToolActionExecute},
		RiskLevel:        "LOW",
		RequiresApproval: true,
	}
}

type fakeSkillCatalog struct {
	skill types.SkillDefinition
	err   error
}

func (catalog fakeSkillCatalog) GetSkill(context.Context, types.AuthContext, string) (types.SkillDefinition, error) {
	if catalog.err != nil {
		return types.SkillDefinition{}, catalog.err
	}
	return catalog.skill, nil
}

type fakeToolPolicy struct {
	decision types.ToolPolicyDecision
	err      error
}

func (policy fakeToolPolicy) CheckToolAction(context.Context, types.CheckToolActionCommand) (types.ToolPolicyDecision, error) {
	if policy.err != nil {
		return types.ToolPolicyDecision{}, policy.err
	}
	return policy.decision, nil
}

type fakeApproval struct {
	err error
}

func (approval fakeApproval) VerifyApprovedProposal(
	_ context.Context,
	command types.VerifyApprovedProposalCommand,
) (types.ApprovedProposal, error) {
	if approval.err != nil {
		return types.ApprovedProposal{}, approval.err
	}
	return types.ApprovedProposal{
		ProposalID:      command.ProposalID,
		ApprovalID:      command.ApprovalID,
		SkillID:         command.SkillID,
		PreparedAuditID: command.PreparedAuditID,
		ToolName:        command.ToolName,
		ResourceType:    command.ResourceType,
		ResourceID:      command.ResourceID,
		RiskLevel:       "LOW",
	}, nil
}

type fakeAuditRepository struct {
	rows    []types.ExecutionAudit
	results []types.ToolResultProjection
	err     error
}

func (repository *fakeAuditRepository) RecordExecution(
	_ context.Context,
	audit types.ExecutionAudit,
	projection types.ToolResultProjection,
) error {
	if repository.err != nil {
		return repository.err
	}
	repository.rows = append(repository.rows, audit)
	repository.results = append(repository.results, projection)
	return nil
}

type fakeToolExecutor struct {
	command types.ToolExecutionCommand
	result  types.ToolExecutionResult
	err     error
}

func (executor *fakeToolExecutor) ExecuteTool(
	_ context.Context,
	command types.ToolExecutionCommand,
) (types.ToolExecutionResult, error) {
	executor.command = command
	if executor.err != nil {
		return types.ToolExecutionResult{}, executor.err
	}
	return executor.result, nil
}
