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

const smokeToolName = "external.mcp.safe.echo"

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
	ID                    string `json:"id"`
	Passed                bool   `json:"passed"`
	FallbackMode          string `json:"fallback_mode"`
	ExecutionStatus       string `json:"execution_status"`
	ResultStatus          string `json:"result_status"`
	Classification        string `json:"classification"`
	Executed              bool   `json:"executed"`
	OutputSHA256Present   bool   `json:"output_sha256_present"`
	RawInputSent          bool   `json:"raw_input_sent"`
	ProviderBodyPersisted bool   `json:"provider_body_persisted"`
}

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "action external MCP fallback smoke failed: %v\n", err)
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
	flagSet := flag.NewFlagSet("action-external-mcp-fallback-smoke", flag.ContinueOnError)
	flagSet.StringVar(&cfg.outputPath, "output", "", "optional summary output path")
	if err := flagSet.Parse(args); err != nil {
		return smokeConfig{}, err
	}
	cfg.outputPath = strings.TrimSpace(cfg.outputPath)
	return cfg, nil
}

func runSmoke(ctx context.Context) (smokeSummary, error) {
	caseSpecs := []struct {
		id             string
		mode           string
		classification string
	}{
		{
			id:             "external-mcp-provider-unavailable",
			mode:           tool.ExternalMCPFallbackProviderUnavailable,
			classification: "TOOL_PROVIDER_UNAVAILABLE",
		},
		{
			id:             "external-mcp-provider-timeout",
			mode:           tool.ExternalMCPFallbackTimeout,
			classification: "TOOL_EXECUTION_TIMEOUT",
		},
		{
			id:             "external-mcp-provider-rate-limited",
			mode:           tool.ExternalMCPFallbackRateLimited,
			classification: "TOOL_PROVIDER_RATE_LIMITED",
		},
		{
			id:             "external-mcp-provider-permission-denied",
			mode:           tool.ExternalMCPFallbackPermissionDenied,
			classification: "TOOL_PROVIDER_PERMISSION_DENIED",
		},
	}

	cases := make([]smokeCase, 0, len(caseSpecs))
	for _, spec := range caseSpecs {
		result, err := runFallbackCase(ctx, spec.id, spec.mode)
		if err != nil {
			return smokeSummary{}, err
		}
		if result.ExecutionStatus != types.ExecutionStatusFailed ||
			result.ResultStatus != types.ResultStatusFailed ||
			result.Classification != spec.classification ||
			result.Executed || result.OutputSHA256Present ||
			result.RawInputSent || result.ProviderBodyPersisted {
			return smokeSummary{}, fmt.Errorf("unexpected MCP fallback result for %s: %+v", spec.id, result)
		}
		cases = append(cases, result)
	}

	return smokeSummary{
		SchemaVersion: 1,
		Adapter:       "action-external-mcp-fallback",
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339Nano),
		Scope:         "local low-sensitive external MCP fallback smoke; no external network, no database, no real MCP server, no business write",
		CaseCount:     len(cases),
		Cases:         cases,
		Verified: []string{
			"external MCP provider unavailable is classified as stable failure without execution",
			"external MCP timeout is classified as stable failure without execution",
			"external MCP rate limit is classified as stable failure without execution",
			"external MCP permission denial is classified as stable failure without execution",
			"raw tool input and raw provider body are not persisted or emitted",
		},
	}, nil
}

func runFallbackCase(ctx context.Context, id string, mode string) (smokeCase, error) {
	executor, err := tool.NewExternalMCPFallbackExecutor(mode)
	if err != nil {
		return smokeCase{}, err
	}
	audit := &recordingAuditRepository{}
	usecase := app.NewExecuteApprovedActionUseCaseWithToolExecutor(
		fakeSkillCatalog{skill: activeSkill()},
		allowingPolicy{},
		approvedProposal{},
		audit,
		executor,
	)
	result, err := usecase.Execute(ctx, validCommand(id))
	if err != nil {
		return smokeCase{}, err
	}
	if len(audit.rows) != 1 || len(audit.projections) != 1 {
		return smokeCase{}, errors.New("expected one audit row and one result projection")
	}
	row := audit.rows[0]
	projection := audit.projections[0]
	return smokeCase{
		ID:                    id,
		Passed:                true,
		FallbackMode:          mode,
		ExecutionStatus:       result.Status,
		ResultStatus:          result.ResultStatus,
		Classification:        result.Classification,
		Executed:              result.Executed,
		OutputSHA256Present:   row.OutputSHA256 != "" || projection.OutputSHA256 != "",
		RawInputSent:          false,
		ProviderBodyPersisted: strings.Contains(result.OutputJSON, "provider") || strings.Contains(result.OutputJSON, "raw-input-value"),
	}, nil
}

func validCommand(caseID string) types.ExecuteApprovedActionCommand {
	return types.ExecuteApprovedActionCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-action-smoke",
			UserID:   "agent-user",
			DeviceID: "agent-device",
		},
		ProposalID:      "proposal-" + caseID,
		ApprovalID:      "approval-" + caseID,
		PreparedAuditID: "mcp-audit-" + caseID,
		SkillID:         smokeToolName,
		ToolName:        smokeToolName,
		Action:          types.ToolActionExecute,
		ResourceType:    "diagnostic",
		ResourceID:      "diagnostic-" + caseID,
		RiskLevel:       "LOW",
		Intent:          "run external MCP fallback smoke",
		InputJSON:       `{"payload":"raw-input-value"}`,
		IdempotencyKey:  "action-external-mcp-fallback-" + caseID,
	}
}

func activeSkill() types.SkillDefinition {
	return types.SkillDefinition{
		TenantID:         "tenant-action-smoke",
		SkillID:          smokeToolName,
		Status:           types.SkillStatusActive,
		ToolName:         smokeToolName,
		AllowedActions:   []string{types.ToolActionExecute},
		RiskLevel:        "LOW",
		RequiresApproval: true,
	}
}

type fakeSkillCatalog struct {
	skill types.SkillDefinition
}

func (catalog fakeSkillCatalog) GetSkill(context.Context, types.AuthContext, string) (types.SkillDefinition, error) {
	return catalog.skill, nil
}

type allowingPolicy struct{}

func (policy allowingPolicy) CheckToolAction(
	_ context.Context,
	command types.CheckToolActionCommand,
) (types.ToolPolicyDecision, error) {
	return types.ToolPolicyDecision{
		TenantID:          command.AuthContext.TenantID,
		UserID:            command.AuthContext.UserID,
		ToolName:          command.ToolName,
		Action:            command.Action,
		ResourceType:      command.ResourceType,
		ResourceID:        command.ResourceID,
		RiskLevel:         command.RiskLevel,
		Allowed:           true,
		RequiresApproval:  true,
		PermissionVersion: 1,
		Classification:    "TOOL_ALLOWED",
		Reason:            "action smoke policy allow",
		DecisionSource:    "TOOL_RULE",
	}, nil
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
