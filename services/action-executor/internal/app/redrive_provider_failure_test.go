package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/qsyy0921/IM/services/action-executor/internal/types"
)

func TestRedriveProviderFailureExecutesFreshApprovedActionWithRedriveMetadata(t *testing.T) {
	audit := &fakeAuditRepository{}
	executor := &fakeToolExecutor{result: types.ToolExecutionResult{
		Executed:   true,
		OutputJSON: `{"adapter":"local-safe-echo","status":"ok"}`,
	}}
	executeUseCase := NewExecuteApprovedActionUseCaseWithToolExecutor(
		fakeSkillCatalog{skill: activeSkill()},
		allowingPolicy(),
		fakeApproval{},
		audit,
		executor,
	)
	usecase := NewRedriveProviderFailureUseCase(
		fakeProviderFailureRedriveRepository{row: redrivableProviderFailure()},
		executeUseCase,
	)

	result, err := usecase.Execute(context.Background(), redriveCommand())
	if err != nil {
		t.Fatalf("redrive provider failure: %v", err)
	}
	if result.ProviderFailureID != "provider-failure-source-1" ||
		result.SourceExecutionID != "exec-source-1" ||
		!result.RedriveResult.Executed ||
		result.RedriveResult.ResultStatus != types.ResultStatusSucceeded {
		t.Fatalf("unexpected redrive result: %+v", result)
	}
	if executor.command.InputSHA256 == "" || executor.command.InputJSON == "" {
		t.Fatalf("expected normal execute path with new input: %+v", executor.command)
	}
	if len(audit.rows) != 1 {
		t.Fatalf("expected one audit row, got %+v", audit.rows)
	}
	row := audit.rows[0]
	if row.RedriveProviderFailureID != "provider-failure-source-1" ||
		row.RedriveReasonSHA256 != redriveReasonHash() ||
		row.ProposalID != "proposal-redrive-1" ||
		row.ApprovalID != "approval-redrive-1" ||
		row.PreparedAuditID != "mcp-audit-redrive-1" {
		t.Fatalf("redrive metadata was not persisted into audit: %+v", row)
	}
}

func TestRedriveProviderFailureRejectsNonDLQSourceBeforeExecution(t *testing.T) {
	row := redrivableProviderFailure()
	row.Status = types.ProviderFailureStatusRetryPending
	audit := &fakeAuditRepository{}
	executor := &fakeToolExecutor{}
	usecase := NewRedriveProviderFailureUseCase(
		fakeProviderFailureRedriveRepository{row: row},
		NewExecuteApprovedActionUseCaseWithToolExecutor(
			fakeSkillCatalog{skill: activeSkill()},
			allowingPolicy(),
			fakeApproval{},
			audit,
			executor,
		),
	)
	_, err := usecase.Execute(context.Background(), redriveCommand())
	if !errors.Is(err, types.ErrProviderFailureNotRedrivable) {
		t.Fatalf("expected not redrivable, got %v", err)
	}
	if executor.called || len(audit.rows) != 0 {
		t.Fatalf("redrive should fail before execution, called=%v audit=%+v", executor.called, audit.rows)
	}
}

func TestRedriveProviderFailureRejectsTargetMismatchAndStaleApproval(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*types.RedriveProviderFailureCommand)
	}{
		{
			name: "tool mismatch",
			mutate: func(command *types.RedriveProviderFailureCommand) {
				command.ToolName = "conversation.profile.update"
			},
		},
		{
			name: "resource mismatch",
			mutate: func(command *types.RedriveProviderFailureCommand) {
				command.ResourceID = "conv-other"
			},
		},
		{
			name: "stale proposal",
			mutate: func(command *types.RedriveProviderFailureCommand) {
				command.ProposalID = "proposal-source-1"
			},
		},
		{
			name: "stale approval",
			mutate: func(command *types.RedriveProviderFailureCommand) {
				command.ApprovalID = "approval-source-1"
			},
		},
		{
			name: "stale prepared audit",
			mutate: func(command *types.RedriveProviderFailureCommand) {
				command.PreparedAuditID = "mcp-audit-source-1"
			},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			command := redriveCommand()
			testCase.mutate(&command)
			executor := &fakeToolExecutor{}
			usecase := NewRedriveProviderFailureUseCase(
				fakeProviderFailureRedriveRepository{row: redrivableProviderFailure()},
				NewExecuteApprovedActionUseCaseWithToolExecutor(
					fakeSkillCatalog{skill: activeSkill()},
					allowingPolicy(),
					fakeApproval{},
					&fakeAuditRepository{},
					executor,
				),
			)
			_, err := usecase.Execute(context.Background(), command)
			if !errors.Is(err, types.ErrProviderFailureNotRedrivable) {
				t.Fatalf("expected not redrivable, got %v", err)
			}
			if executor.called {
				t.Fatalf("tool executor should not be called for invalid redrive source")
			}
		})
	}
}

func TestRedriveProviderFailureRequiresReasonHash(t *testing.T) {
	command := redriveCommand()
	command.ReasonSHA256 = strings.Repeat("x", 64)
	usecase := NewRedriveProviderFailureUseCase(
		fakeProviderFailureRedriveRepository{row: redrivableProviderFailure()},
		NewExecuteApprovedActionUseCase(fakeSkillCatalog{}, fakeToolPolicy{}, fakeApproval{}, &fakeAuditRepository{}),
	)
	if _, err := usecase.Execute(context.Background(), command); !errors.Is(err, types.ErrInvalidArgument) {
		t.Fatalf("expected invalid argument for malformed reason hash, got %v", err)
	}
}

type fakeProviderFailureRedriveRepository struct {
	row types.ProviderFailureAuditRow
	err error
}

func (repository fakeProviderFailureRedriveRepository) GetProviderFailureForRedrive(
	context.Context,
	types.TenantID,
	string,
) (types.ProviderFailureAuditRow, error) {
	if repository.err != nil {
		return types.ProviderFailureAuditRow{}, repository.err
	}
	return repository.row, nil
}

func redriveCommand() types.RedriveProviderFailureCommand {
	return types.RedriveProviderFailureCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-1",
			UserID:   "user-1",
			DeviceID: "device-1",
		},
		ProviderFailureID: "provider-failure-source-1",
		ReasonSHA256:      redriveReasonHash(),
		ProposalID:        "proposal-redrive-1",
		ApprovalID:        "approval-redrive-1",
		PreparedAuditID:   "mcp-audit-redrive-1",
		SkillID:           "skill-1",
		ToolName:          "conversation.reply.send",
		Action:            types.ToolActionExecute,
		ResourceType:      "conversation",
		ResourceID:        "conv-1",
		RiskLevel:         "LOW",
		Intent:            "operator approved provider redrive",
		InputJSON:         `{"body":"redrive message"}`,
		IdempotencyKey:    "idem-redrive-1",
	}
}

func redrivableProviderFailure() types.ProviderFailureAuditRow {
	now := time.Date(2026, 6, 25, 13, 0, 0, 0, time.UTC)
	return types.ProviderFailureAuditRow{
		TenantID:          "tenant-1",
		ProviderFailureID: "provider-failure-source-1",
		ExecutionID:       "exec-source-1",
		ResultID:          "result-source-1",
		ProposalID:        "proposal-source-1",
		ApprovalID:        "approval-source-1",
		PreparedAuditID:   "mcp-audit-source-1",
		UserID:            "user-1",
		SkillID:           "skill-1",
		ToolName:          "conversation.reply.send",
		ResourceType:      "conversation",
		ResourceID:        "conv-1",
		Classification:    "TOOL_PROVIDER_UNAVAILABLE",
		Status:            types.ProviderFailureStatusDLQ,
		Retryable:         false,
		RetryCount:        3,
		DeadLetteredAt:    &now,
		FailureRef:        "action-executor://executions/exec-source-1/provider-failures/provider-failure-source-1",
		CreatedAt:         now,
	}
}

func redriveReasonHash() string {
	return "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
}
