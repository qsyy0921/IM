package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/qsyy0921/IM/services/action-executor/internal/app"
	"github.com/qsyy0921/IM/services/action-executor/internal/infrastructure/tool"
	"github.com/qsyy0921/IM/services/action-executor/internal/types"
)

type smokeConfig struct {
	outputPath string
}

type smokeSummary struct {
	SchemaVersion int         `json:"schema_version"`
	Adapter       string      `json:"adapter"`
	GeneratedAt   string      `json:"generated_at"`
	Scope         string      `json:"scope"`
	CaseCount     int         `json:"case_count"`
	Cases         []smokeCase `json:"cases"`
	Verified      []string    `json:"verified"`
}

type smokeCase struct {
	ID                  string `json:"id"`
	Passed              bool   `json:"passed"`
	ErrorClass          string `json:"error_class,omitempty"`
	ExecutionStatus     string `json:"execution_status"`
	ResultStatus        string `json:"result_status"`
	Classification      string `json:"classification"`
	Allowed             bool   `json:"allowed"`
	Executed            bool   `json:"executed"`
	AuditRecorded       bool   `json:"audit_recorded"`
	ProjectionRecorded  bool   `json:"projection_recorded"`
	OutputSHA256Present bool   `json:"output_sha256_present"`
	ExecutorCalled      bool   `json:"executor_called"`
}

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "action preflight safety smoke failed: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	cfg, err := parseConfig(args)
	if err != nil {
		return err
	}
	summary, err := runSmoke(ctx)
	if err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return err
	}
	if cfg.outputPath != "" {
		if err := os.MkdirAll(filepath.Dir(cfg.outputPath), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(cfg.outputPath, append(encoded, '\n'), 0o644); err != nil {
			return err
		}
	}
	fmt.Println(string(encoded))
	return nil
}

func parseConfig(args []string) (smokeConfig, error) {
	var cfg smokeConfig
	flagSet := flag.NewFlagSet("action-preflight-safety-smoke", flag.ContinueOnError)
	flagSet.StringVar(&cfg.outputPath, "output", "", "optional summary output path")
	if err := flagSet.Parse(args); err != nil {
		return smokeConfig{}, err
	}
	cfg.outputPath = strings.TrimSpace(cfg.outputPath)
	return cfg, nil
}

func runSmoke(ctx context.Context) (smokeSummary, error) {
	cases := make([]smokeCase, 0, 5)

	policyDenied, err := runCase(ctx, caseConfig{
		id:       "action-preflight-policy-denied",
		skill:    activeSkill(types.LocalSafeEchoToolName, "LOW"),
		policy:   deniedPolicy(),
		approval: approvedProposal{},
		executor: &recordingToolExecutor{
			result: types.ToolExecutionResult{
				Executed:   true,
				OutputJSON: `{"status":"should-not-run"}`,
			},
		},
		command: localEchoCommand("LOW"),
	})
	if err != nil {
		return smokeSummary{}, err
	}
	if !isBlocked(policyDenied, "TOOL_DENIED") || policyDenied.ExecutorCalled {
		return smokeSummary{}, fmt.Errorf("unexpected policy denied case: %+v", policyDenied)
	}
	cases = append(cases, policyDenied)

	disabled, err := runCase(ctx, caseConfig{
		id:       "action-preflight-disabled-skill-blocked",
		skill:    disabledSkill(types.LocalSafeEchoToolName, "LOW"),
		policy:   allowingPolicy(),
		approval: approvedProposal{},
		executor: &recordingToolExecutor{
			result: types.ToolExecutionResult{
				Executed:   true,
				OutputJSON: `{"status":"should-not-run"}`,
			},
		},
		command: localEchoCommand("LOW"),
	})
	if err != nil {
		return smokeSummary{}, err
	}
	if !isBlocked(disabled, "SKILL_DISABLED") || disabled.ExecutorCalled {
		return smokeSummary{}, fmt.Errorf("unexpected disabled skill case: %+v", disabled)
	}
	cases = append(cases, disabled)

	mismatch, err := runCase(ctx, caseConfig{
		id:       "action-preflight-tool-mismatch-blocked",
		skill:    activeSkill("different.tool", "LOW"),
		policy:   allowingPolicy(),
		approval: approvedProposal{},
		executor: &recordingToolExecutor{
			result: types.ToolExecutionResult{
				Executed:   true,
				OutputJSON: `{"status":"should-not-run"}`,
			},
		},
		command: localEchoCommand("LOW"),
	})
	if err != nil {
		return smokeSummary{}, err
	}
	if !isBlocked(mismatch, "TOOL_MISMATCH") || mismatch.ExecutorCalled {
		return smokeSummary{}, fmt.Errorf("unexpected tool mismatch case: %+v", mismatch)
	}
	cases = append(cases, mismatch)

	highRisk, err := runCase(ctx, caseConfig{
		id:       "action-preflight-elevated-local-tool-not-executed",
		skill:    activeSkill(types.LocalSafeEchoToolName, "HIGH"),
		policy:   allowingPolicy(),
		approval: approvedProposal{},
		executor: &recordingDelegatingToolExecutor{delegate: tool.NewLocalSafeExecutor()},
		command:  localEchoCommand("HIGH"),
	})
	if err != nil {
		return smokeSummary{}, err
	}
	if highRisk.ExecutionStatus != types.ExecutionStatusRecorded ||
		highRisk.ResultStatus != types.ResultStatusNotExecuted ||
		!highRisk.Allowed ||
		highRisk.Executed ||
		!highRisk.AuditRecorded ||
		!highRisk.ProjectionRecorded ||
		highRisk.OutputSHA256Present {
		return smokeSummary{}, fmt.Errorf("unexpected high risk local case: %+v", highRisk)
	}
	cases = append(cases, highRisk)

	unapproved, err := runCase(ctx, caseConfig{
		id:       "action-preflight-unapproved-proposal-no-audit",
		skill:    activeSkill(types.LocalSafeEchoToolName, "LOW"),
		policy:   allowingPolicy(),
		approval: rejectedProposal{err: types.ErrProposalNotApproved},
		executor: &recordingToolExecutor{
			result: types.ToolExecutionResult{
				Executed:   true,
				OutputJSON: `{"status":"should-not-run"}`,
			},
		},
		command: localEchoCommand("LOW"),
	})
	if err != nil {
		return smokeSummary{}, err
	}
	if unapproved.ErrorClass != "PROPOSAL_NOT_APPROVED" ||
		unapproved.AuditRecorded ||
		unapproved.ProjectionRecorded ||
		unapproved.ExecutorCalled ||
		unapproved.Executed {
		return smokeSummary{}, fmt.Errorf("unexpected unapproved proposal case: %+v", unapproved)
	}
	cases = append(cases, unapproved)

	return smokeSummary{
		SchemaVersion: 1,
		Adapter:       "action-preflight-safety",
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339Nano),
		Scope:         "first-stage action-executor preflight safety smoke; in-memory ports only, no external network, no database and no business write",
		CaseCount:     len(cases),
		Cases:         cases,
		Verified: []string{
			"policy denied actions are blocked before tool execution",
			"disabled skills and tool mismatches are blocked before policy/provider execution",
			"elevated risk local-safe tools remain not executed and hashless",
			"unapproved proposals fail before audit/result rows are recorded",
		},
	}, nil
}

type caseConfig struct {
	id       string
	skill    types.SkillDefinition
	policy   fakeToolPolicy
	approval proposalPort
	executor app.ToolExecutorPort
	command  types.ExecuteApprovedActionCommand
}

func runCase(ctx context.Context, cfg caseConfig) (smokeCase, error) {
	audit := &recordingAuditRepository{}
	usecase := app.NewExecuteApprovedActionUseCaseWithToolExecutor(
		fakeSkillCatalog{skill: cfg.skill},
		cfg.policy,
		cfg.approval,
		audit,
		cfg.executor,
	)
	result, err := usecase.Execute(ctx, cfg.command)
	if err != nil {
		return smokeCase{
			ID:                 cfg.id,
			Passed:             true,
			ErrorClass:         errorClass(err),
			AuditRecorded:      len(audit.rows) > 0,
			ProjectionRecorded: len(audit.projections) > 0,
			ExecutorCalled:     executorCalled(cfg.executor),
		}, nil
	}
	outputSHA256Present := false
	if len(audit.rows) > 0 {
		outputSHA256Present = strings.TrimSpace(audit.rows[0].OutputSHA256) != ""
	}
	if len(audit.projections) > 0 && strings.TrimSpace(audit.projections[0].OutputSHA256) != "" {
		outputSHA256Present = true
	}
	return smokeCase{
		ID:                  cfg.id,
		Passed:              true,
		ExecutionStatus:     result.Status,
		ResultStatus:        result.ResultStatus,
		Classification:      result.Classification,
		Allowed:             result.Allowed,
		Executed:            result.Executed,
		AuditRecorded:       len(audit.rows) == 1,
		ProjectionRecorded:  len(audit.projections) == 1,
		OutputSHA256Present: outputSHA256Present,
		ExecutorCalled:      executorCalled(cfg.executor),
	}, nil
}

func isBlocked(result smokeCase, classification string) bool {
	return result.ExecutionStatus == types.ExecutionStatusBlocked &&
		result.ResultStatus == types.ResultStatusBlocked &&
		result.Classification == classification &&
		!result.Allowed &&
		!result.Executed &&
		result.AuditRecorded &&
		result.ProjectionRecorded &&
		!result.OutputSHA256Present
}

func localEchoCommand(riskLevel string) types.ExecuteApprovedActionCommand {
	return types.ExecuteApprovedActionCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-action-preflight",
			UserID:   "agent-user",
			DeviceID: "agent-device",
		},
		ProposalID:      "proposal-action-preflight",
		ApprovalID:      "approval-action-preflight",
		PreparedAuditID: "mcp-audit-action-preflight",
		SkillID:         types.LocalSafeEchoToolName,
		ToolName:        types.LocalSafeEchoToolName,
		Action:          types.ToolActionExecute,
		ResourceType:    "diagnostic",
		ResourceID:      "diagnostic-action-preflight",
		RiskLevel:       riskLevel,
		Intent:          "run action preflight safety smoke",
		InputJSON:       `{"payload":"low-sensitive fixture"}`,
		IdempotencyKey:  "action-preflight-safety",
	}
}

func activeSkill(toolName string, riskLevel string) types.SkillDefinition {
	return types.SkillDefinition{
		TenantID:         "tenant-action-preflight",
		SkillID:          types.LocalSafeEchoToolName,
		Status:           types.SkillStatusActive,
		ToolName:         toolName,
		AllowedActions:   []string{types.ToolActionExecute},
		RiskLevel:        riskLevel,
		RequiresApproval: true,
	}
}

func disabledSkill(toolName string, riskLevel string) types.SkillDefinition {
	skill := activeSkill(toolName, riskLevel)
	skill.Status = types.SkillStatusDisabled
	return skill
}

type fakeSkillCatalog struct {
	skill types.SkillDefinition
}

func (catalog fakeSkillCatalog) GetSkill(context.Context, types.AuthContext, string) (types.SkillDefinition, error) {
	return catalog.skill, nil
}

type fakeToolPolicy struct {
	decision types.ToolPolicyDecision
}

func allowingPolicy() fakeToolPolicy {
	return fakeToolPolicy{decision: types.ToolPolicyDecision{
		Allowed:           true,
		RequiresApproval:  true,
		PermissionVersion: 1,
		Classification:    "TOOL_ALLOWED",
		Reason:            "approved",
		DecisionSource:    "TOOL_RULE",
	}}
}

func deniedPolicy() fakeToolPolicy {
	return fakeToolPolicy{decision: types.ToolPolicyDecision{
		Allowed:           false,
		RequiresApproval:  true,
		PermissionVersion: 1,
		Classification:    "TOOL_DENIED",
		Reason:            "denied",
		DecisionSource:    "TOOL_RULE",
	}}
}

func (policy fakeToolPolicy) CheckToolAction(
	_ context.Context,
	command types.CheckToolActionCommand,
) (types.ToolPolicyDecision, error) {
	decision := policy.decision
	decision.TenantID = command.AuthContext.TenantID
	decision.UserID = command.AuthContext.UserID
	decision.ToolName = command.ToolName
	decision.Action = command.Action
	decision.ResourceType = command.ResourceType
	decision.ResourceID = command.ResourceID
	decision.RiskLevel = command.RiskLevel
	return decision, nil
}

type proposalPort interface {
	VerifyApprovedProposal(context.Context, types.VerifyApprovedProposalCommand) (types.ApprovedProposal, error)
}

type approvedProposal struct{}

func (approval approvedProposal) VerifyApprovedProposal(
	_ context.Context,
	command types.VerifyApprovedProposalCommand,
) (types.ApprovedProposal, error) {
	return types.ApprovedProposal{
		ProposalID:      command.ProposalID,
		ApprovalID:      command.ApprovalID,
		Status:          "APPROVED",
		UserID:          command.AuthContext.UserID,
		SkillID:         command.SkillID,
		PreparedAuditID: command.PreparedAuditID,
		ToolName:        command.ToolName,
		ResourceType:    command.ResourceType,
		ResourceID:      command.ResourceID,
		RiskLevel:       "LOW",
		ApprovedAt:      time.Now().UTC(),
	}, nil
}

type rejectedProposal struct {
	err error
}

func (approval rejectedProposal) VerifyApprovedProposal(
	context.Context,
	types.VerifyApprovedProposalCommand,
) (types.ApprovedProposal, error) {
	return types.ApprovedProposal{}, approval.err
}

type recordingAuditRepository struct {
	rows        []types.ExecutionAudit
	projections []types.ToolResultProjection
}

func (repository *recordingAuditRepository) RecordExecution(
	_ context.Context,
	audit types.ExecutionAudit,
	projection types.ToolResultProjection,
) error {
	repository.rows = append(repository.rows, audit)
	repository.projections = append(repository.projections, projection)
	return nil
}

type recordingToolExecutor struct {
	called bool
	result types.ToolExecutionResult
	err    error
}

func (executor *recordingToolExecutor) ExecuteTool(
	context.Context,
	types.ToolExecutionCommand,
) (types.ToolExecutionResult, error) {
	executor.called = true
	if executor.err != nil {
		return types.ToolExecutionResult{}, executor.err
	}
	return executor.result, nil
}

func executorCalled(executor app.ToolExecutorPort) bool {
	switch typed := executor.(type) {
	case *recordingToolExecutor:
		return typed.called
	case *recordingDelegatingToolExecutor:
		return typed.called
	default:
		return false
	}
}

type recordingDelegatingToolExecutor struct {
	called   bool
	delegate app.ToolExecutorPort
}

func (executor *recordingDelegatingToolExecutor) ExecuteTool(
	ctx context.Context,
	command types.ToolExecutionCommand,
) (types.ToolExecutionResult, error) {
	executor.called = true
	return executor.delegate.ExecuteTool(ctx, command)
}

func errorClass(err error) string {
	switch {
	case errors.Is(err, types.ErrProposalNotApproved):
		return "PROPOSAL_NOT_APPROVED"
	case errors.Is(err, types.ErrProposalMismatch):
		return "PROPOSAL_MISMATCH"
	default:
		return "ERROR"
	}
}
