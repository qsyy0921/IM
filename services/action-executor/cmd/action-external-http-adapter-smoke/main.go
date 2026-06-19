package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/qsyy0921/IM/services/action-executor/internal/app"
	"github.com/qsyy0921/IM/services/action-executor/internal/infrastructure/tool"
	"github.com/qsyy0921/IM/services/action-executor/internal/types"
)

const (
	smokeToolName = "provider.safe.echo"
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
	ID                    string `json:"id"`
	Passed                bool   `json:"passed"`
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
		fmt.Fprintf(os.Stderr, "action external HTTP adapter smoke failed: %v\n", err)
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
	flagSet := flag.NewFlagSet("action-external-http-adapter-smoke", flag.ContinueOnError)
	flagSet.StringVar(&cfg.outputPath, "output", "", "optional summary output path")
	if err := flagSet.Parse(args); err != nil {
		return smokeConfig{}, err
	}
	cfg.outputPath = strings.TrimSpace(cfg.outputPath)
	return cfg, nil
}

func runSmoke(ctx context.Context) (smokeSummary, error) {
	cases := []smokeCase{}

	success, err := runProviderCase(ctx, providerBehavior{
		id:         "external-http-success",
		statusCode: http.StatusOK,
		body:       `{"schema_version":1,"status":"ok","result_ref":"provider-result-1"}`,
	})
	if err != nil {
		return smokeSummary{}, err
	}
	if !success.Executed || success.ExecutionStatus != types.ExecutionStatusRecorded ||
		success.ResultStatus != types.ResultStatusSucceeded ||
		!success.OutputSHA256Present || success.RawInputSent || success.ProviderBodyPersisted {
		return smokeSummary{}, fmt.Errorf("unexpected success case result: %+v", success)
	}
	cases = append(cases, success)

	unavailable, err := runProviderCase(ctx, providerBehavior{
		id:         "external-http-provider-unavailable",
		statusCode: http.StatusBadGateway,
		body:       `{"provider_error":"raw provider body must not persist"}`,
	})
	if err != nil {
		return smokeSummary{}, err
	}
	if unavailable.ExecutionStatus != types.ExecutionStatusFailed ||
		unavailable.ResultStatus != types.ResultStatusFailed ||
		unavailable.Classification != "TOOL_PROVIDER_UNAVAILABLE" ||
		unavailable.Executed || unavailable.OutputSHA256Present ||
		unavailable.ProviderBodyPersisted {
		return smokeSummary{}, fmt.Errorf("unexpected provider failure result: %+v", unavailable)
	}
	cases = append(cases, unavailable)

	unsafe, err := runProviderCase(ctx, providerBehavior{
		id:         "external-http-unsafe-output",
		statusCode: http.StatusOK,
		body:       `{"schema_version":1,"status":"ok","token":"redacted-provider-token"}`,
	})
	if err != nil {
		return smokeSummary{}, err
	}
	if unsafe.ExecutionStatus != types.ExecutionStatusFailed ||
		unsafe.ResultStatus != types.ResultStatusFailed ||
		unsafe.Classification != "TOOL_OUTPUT_UNSAFE" ||
		unsafe.Executed || unsafe.OutputSHA256Present ||
		unsafe.ProviderBodyPersisted {
		return smokeSummary{}, fmt.Errorf("unexpected unsafe output result: %+v", unsafe)
	}
	cases = append(cases, unsafe)

	return smokeSummary{
		SchemaVersion: 1,
		Adapter:       "action-external-http-provider",
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339Nano),
		Scope:         "local low-sensitive external HTTP tool adapter smoke; no external network, no database, no business write",
		CaseCount:     len(cases),
		Cases:         cases,
		Verified: []string{
			"allowlisted LOW-risk external HTTP provider tool can execute and only record output hash",
			"provider failure is classified with stable public fields and no raw provider body",
			"unsafe provider output fails closed and is not persisted or hashed",
			"raw input_json is never sent to the provider fixture",
		},
	}, nil
}

type providerBehavior struct {
	id         string
	statusCode int
	body       string
}

func runProviderCase(ctx context.Context, behavior providerBehavior) (smokeCase, error) {
	rawInputSent := false
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			http.Error(writer, "bad request", http.StatusBadRequest)
			return
		}
		encoded, _ := json.Marshal(payload)
		rawInputSent = strings.Contains(string(encoded), "raw-input-value") || payload["input_json"] != nil
		if payload["tool_name"] != smokeToolName || payload["input_sha256"] == "" {
			http.Error(writer, "bad request", http.StatusBadRequest)
			return
		}
		writer.WriteHeader(behavior.statusCode)
		_, _ = writer.Write([]byte(behavior.body))
	}))
	defer server.Close()

	executor, err := tool.NewExternalHTTPExecutor(tool.ExternalHTTPExecutorOptions{
		Endpoint:     server.URL,
		AllowedTools: []string{smokeToolName},
		Timeout:      time.Second,
	})
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
	result, err := usecase.Execute(ctx, validCommand())
	if err != nil {
		return smokeCase{}, err
	}
	if len(audit.rows) != 1 || len(audit.projections) != 1 {
		return smokeCase{}, errors.New("expected one audit row and one result projection")
	}
	row := audit.rows[0]
	projection := audit.projections[0]
	return smokeCase{
		ID:                    behavior.id,
		Passed:                true,
		ExecutionStatus:       result.Status,
		ResultStatus:          result.ResultStatus,
		Classification:        result.Classification,
		Executed:              result.Executed,
		OutputSHA256Present:   row.OutputSHA256 != "" || projection.OutputSHA256 != "",
		RawInputSent:          rawInputSent,
		ProviderBodyPersisted: strings.Contains(result.OutputJSON, "provider_error") || strings.Contains(result.OutputJSON, "redacted-provider-token"),
	}, nil
}

func validCommand() types.ExecuteApprovedActionCommand {
	return types.ExecuteApprovedActionCommand{
		AuthContext: types.AuthContext{
			TenantID: "tenant-action-smoke",
			UserID:   "agent-user",
			DeviceID: "agent-device",
		},
		ProposalID:      "proposal-action-smoke",
		ApprovalID:      "approval-action-smoke",
		PreparedAuditID: "mcp-audit-action-smoke",
		SkillID:         smokeToolName,
		ToolName:        smokeToolName,
		Action:          types.ToolActionExecute,
		ResourceType:    "diagnostic",
		ResourceID:      "diagnostic-action-smoke",
		RiskLevel:       "LOW",
		Intent:          "run external http adapter smoke",
		InputJSON:       `{"payload":"raw-input-value"}`,
		IdempotencyKey:  "action-external-http-smoke",
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
