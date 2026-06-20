package app

import (
	"context"
	"errors"
	"strings"
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

func TestExecuteApprovedActionClassifiesExternalToolExecutionFailures(t *testing.T) {
	cases := []struct {
		name           string
		err            error
		classification string
		reason         string
	}{
		{
			name:           "timeout",
			err:            types.ErrToolExecutionTimeout,
			classification: "TOOL_EXECUTION_TIMEOUT",
			reason:         "tool execution timeout",
		},
		{
			name:           "provider unavailable",
			err:            types.ErrToolProviderUnavailable,
			classification: "TOOL_PROVIDER_UNAVAILABLE",
			reason:         "tool provider unavailable",
		},
		{
			name:           "provider rate limited",
			err:            types.ErrToolProviderRateLimited,
			classification: "TOOL_PROVIDER_RATE_LIMITED",
			reason:         "tool provider rate limited",
		},
		{
			name:           "provider permission denied",
			err:            types.ErrToolProviderPermissionDenied,
			classification: "TOOL_PROVIDER_PERMISSION_DENIED",
			reason:         "tool provider permission denied",
		},
		{
			name:           "generic failure",
			err:            errors.New("provider returned private body: token=abc"),
			classification: "TOOL_EXECUTION_FAILED",
			reason:         "tool execution failed",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			audit := &fakeAuditRepository{}
			usecase := NewExecuteApprovedActionUseCaseWithToolExecutor(
				fakeSkillCatalog{skill: activeSkill()},
				allowingPolicy(),
				fakeApproval{},
				audit,
				&fakeToolExecutor{err: testCase.err},
			)
			result, err := usecase.Execute(context.Background(), validCommand())
			if err != nil {
				t.Fatalf("execute action: %v", err)
			}
			if result.Status != types.ExecutionStatusFailed || result.ResultStatus != types.ResultStatusFailed || result.Executed {
				t.Fatalf("expected failed unexecuted result, got %+v", result)
			}
			if result.Classification != testCase.classification || result.Reason != testCase.reason {
				t.Fatalf("unexpected public failure fields: %+v", result)
			}
			if strings.Contains(result.Reason, "token=abc") || strings.Contains(result.Classification, "token=abc") {
				t.Fatalf("raw provider error leaked into public fields: %+v", result)
			}
			if len(audit.rows) != 1 || audit.rows[0].OutputSHA256 != "" || audit.rows[0].Executed {
				t.Fatalf("expected failed audit without output hash, got %+v", audit.rows)
			}
			if len(audit.results) != 1 || audit.results[0].Status != types.ResultStatusFailed || audit.results[0].OutputSHA256 != "" {
				t.Fatalf("expected failed result projection without output hash, got %+v", audit.results)
			}
		})
	}
}

func TestExecuteApprovedActionRejectsUnsafeToolOutputs(t *testing.T) {
	cases := []struct {
		name       string
		outputJSON string
	}{
		{name: "malformed json", outputJSON: `{"status":`},
		{name: "array output", outputJSON: `[{"status":"ok"}]`},
		{name: "secret key", outputJSON: `{"status":"ok","api_key":"sk-secret"}`},
		{name: "token string", outputJSON: `{"status":"ok","message":"Bearer token=abc"}`},
		{name: "private key", outputJSON: `{"status":"ok","pem":"-----BEGIN RSA PRIVATE KEY-----abc"}`},
		{name: "email pii", outputJSON: `{"status":"ok","contact":"ops@example.com"}`},
		{name: "oversized", outputJSON: `{"status":"ok","blob":"` + strings.Repeat("a", 20*1024) + `"}`},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			audit := &fakeAuditRepository{}
			usecase := NewExecuteApprovedActionUseCaseWithToolExecutor(
				fakeSkillCatalog{skill: activeSkill()},
				allowingPolicy(),
				fakeApproval{},
				audit,
				&fakeToolExecutor{result: types.ToolExecutionResult{
					Executed:   true,
					OutputJSON: testCase.outputJSON,
				}},
			)
			result, err := usecase.Execute(context.Background(), validCommand())
			if err != nil {
				t.Fatalf("execute action: %v", err)
			}
			if result.Status != types.ExecutionStatusFailed || result.ResultStatus != types.ResultStatusFailed {
				t.Fatalf("expected failed unsafe output result, got %+v", result)
			}
			if result.Executed || result.OutputJSON != "{}" || result.Classification != "TOOL_OUTPUT_UNSAFE" {
				t.Fatalf("unsafe output should be suppressed, got %+v", result)
			}
			if strings.Contains(result.OutputJSON, "sk-secret") || strings.Contains(result.OutputJSON, "ops@example.com") {
				t.Fatalf("unsafe output leaked into response: %+v", result)
			}
			if len(audit.rows) != 1 || audit.rows[0].OutputSHA256 != "" || audit.rows[0].Executed {
				t.Fatalf("unsafe output should not be hashed as executed: %+v", audit.rows)
			}
			if len(audit.results) != 1 || audit.results[0].OutputSHA256 != "" || audit.results[0].Executed {
				t.Fatalf("unsafe result projection should not store output hash: %+v", audit.results)
			}
		})
	}
}

func TestExecuteApprovedActionBlocksRateLimitedActionBeforeToolExecution(t *testing.T) {
	audit := &fakeAuditRepository{}
	executor := &fakeToolExecutor{result: types.ToolExecutionResult{
		Executed:   true,
		OutputJSON: `{"status":"should-not-run"}`,
	}}
	usecase := NewExecuteApprovedActionUseCaseWithToolExecutorAndRateLimiter(
		fakeSkillCatalog{skill: activeSkill()},
		allowingPolicy(),
		fakeApproval{},
		audit,
		executor,
		fakeRateLimiter{decision: types.ActionRateLimitDecision{
			Allowed:        false,
			Classification: "ACTION_RATE_LIMITED",
			Reason:         "action rate limited",
			DecisionSource: "action-executor-rate-limit",
		}},
	)
	result, err := usecase.Execute(context.Background(), validCommand())
	if err != nil {
		t.Fatalf("execute action: %v", err)
	}
	if result.Status != types.ExecutionStatusBlocked ||
		result.ResultStatus != types.ResultStatusBlocked ||
		result.Allowed ||
		result.Executed ||
		result.Classification != "ACTION_RATE_LIMITED" {
		t.Fatalf("expected rate-limited blocked result, got %+v", result)
	}
	if executor.called {
		t.Fatalf("tool executor should not be called when rate-limited")
	}
	if len(audit.rows) != 1 || audit.rows[0].Executed || audit.rows[0].OutputSHA256 != "" {
		t.Fatalf("expected blocked audit without output hash, got %+v", audit.rows)
	}
	if len(audit.results) != 1 || audit.results[0].Status != types.ResultStatusBlocked || audit.results[0].OutputSHA256 != "" {
		t.Fatalf("expected blocked projection without output hash, got %+v", audit.results)
	}
}

func TestExecuteApprovedActionFailsClosedWhenRateLimiterUnavailable(t *testing.T) {
	audit := &fakeAuditRepository{}
	executor := &fakeToolExecutor{result: types.ToolExecutionResult{
		Executed:   true,
		OutputJSON: `{"status":"should-not-run"}`,
	}}
	usecase := NewExecuteApprovedActionUseCaseWithToolExecutorAndRateLimiter(
		fakeSkillCatalog{skill: activeSkill()},
		allowingPolicy(),
		fakeApproval{},
		audit,
		executor,
		fakeRateLimiter{err: errors.New("limiter store unavailable: private details")},
	)
	result, err := usecase.Execute(context.Background(), validCommand())
	if err != nil {
		t.Fatalf("execute action: %v", err)
	}
	if result.Status != types.ExecutionStatusFailed ||
		result.ResultStatus != types.ResultStatusFailed ||
		result.Allowed ||
		result.Executed ||
		result.Classification != "ACTION_RATE_LIMIT_UNAVAILABLE" ||
		result.Reason != "action rate limit unavailable" {
		t.Fatalf("expected fail-closed limiter result, got %+v", result)
	}
	if executor.called {
		t.Fatalf("tool executor should not be called when limiter fails")
	}
	if strings.Contains(result.Reason, "private details") || strings.Contains(result.Classification, "private details") {
		t.Fatalf("limiter internal error leaked into public fields: %+v", result)
	}
	if len(audit.rows) != 1 || audit.rows[0].Status != types.ExecutionStatusFailed || audit.rows[0].OutputSHA256 != "" {
		t.Fatalf("expected failed audit without output hash, got %+v", audit.rows)
	}
	if len(audit.results) != 1 || audit.results[0].Status != types.ResultStatusFailed || audit.results[0].OutputSHA256 != "" {
		t.Fatalf("expected failed projection without output hash, got %+v", audit.results)
	}
}

func TestExecuteApprovedActionBlocksRepairOrDLQActionBeforeToolExecution(t *testing.T) {
	audit := &fakeAuditRepository{}
	executor := &fakeToolExecutor{result: types.ToolExecutionResult{
		Executed:   true,
		OutputJSON: `{"status":"should-not-run"}`,
	}}
	usecase := NewExecuteApprovedActionUseCaseWithToolExecutorAndRateLimiter(
		fakeSkillCatalog{skill: activeRepairSkill()},
		allowingPolicy(),
		fakeApproval{},
		audit,
		executor,
		fakeRateLimiter{decision: types.ActionRateLimitDecision{Allowed: true}},
	)
	result, err := usecase.Execute(context.Background(), repairCommand())
	if err != nil {
		t.Fatalf("execute action: %v", err)
	}
	if result.Status != types.ExecutionStatusBlocked ||
		result.ResultStatus != types.ResultStatusBlocked ||
		result.Allowed ||
		result.Executed ||
		result.Classification != "ACTION_REPAIR_REQUIRES_OPERATOR" ||
		result.Reason != "repair action requires operator workflow" {
		t.Fatalf("expected repair guard blocked result, got %+v", result)
	}
	if executor.called {
		t.Fatalf("tool executor should not be called for repair/DLQ actions")
	}
	if len(audit.rows) != 1 || audit.rows[0].Executed || audit.rows[0].OutputSHA256 != "" {
		t.Fatalf("expected repair guard audit without output hash, got %+v", audit.rows)
	}
	if len(audit.results) != 1 || audit.results[0].Status != types.ResultStatusBlocked || audit.results[0].OutputSHA256 != "" {
		t.Fatalf("expected blocked projection without output hash, got %+v", audit.results)
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

func repairCommand() types.ExecuteApprovedActionCommand {
	command := validCommand()
	command.ProposalID = "proposal-repair-1"
	command.ApprovalID = "approval-repair-1"
	command.PreparedAuditID = "mcp-audit-repair-1"
	command.SkillID = "skill-repair-1"
	command.ToolName = "delivery.outbox.repair"
	command.ResourceType = "delivery_outbox_dlq"
	command.ResourceID = "repair-target-present"
	command.RiskLevel = "HIGH"
	command.Intent = "operator-approved repair"
	command.InputJSON = `{"repair_id":"fixture"}`
	command.IdempotencyKey = "idem-repair-1"
	return command
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

func activeRepairSkill() types.SkillDefinition {
	skill := activeSkill()
	skill.SkillID = "skill-repair-1"
	skill.ToolName = "delivery.outbox.repair"
	skill.RiskLevel = "HIGH"
	return skill
}

func allowingPolicy() fakeToolPolicy {
	return fakeToolPolicy{decision: types.ToolPolicyDecision{
		Allowed:           true,
		RequiresApproval:  true,
		PermissionVersion: 9,
		Classification:    "TOOL_ALLOWED",
		Reason:            "approved",
		DecisionSource:    "TOOL_RULE",
	}}
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
	called  bool
}

func (executor *fakeToolExecutor) ExecuteTool(
	_ context.Context,
	command types.ToolExecutionCommand,
) (types.ToolExecutionResult, error) {
	executor.called = true
	executor.command = command
	if executor.err != nil {
		return types.ToolExecutionResult{}, executor.err
	}
	return executor.result, nil
}

type fakeRateLimiter struct {
	decision types.ActionRateLimitDecision
	err      error
}

func (limiter fakeRateLimiter) CheckActionRateLimit(
	context.Context,
	types.ActionRateLimitCommand,
) (types.ActionRateLimitDecision, error) {
	if limiter.err != nil {
		return types.ActionRateLimitDecision{}, limiter.err
	}
	return limiter.decision, nil
}
