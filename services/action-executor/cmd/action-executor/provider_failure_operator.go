package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	postgresinfra "github.com/qsyy0921/IM/services/action-executor/internal/infrastructure/postgres"
	"github.com/qsyy0921/IM/services/action-executor/internal/types"
)

const actionProviderFailureReasonMaxBytes = 16 * 1024

type providerFailureOperatorOutput struct {
	SchemaVersion            int                                `json:"schema_version"`
	GeneratedAt              string                             `json:"generated_at"`
	Kind                     string                             `json:"kind"`
	BatchID                  string                             `json:"batch_id,omitempty"`
	Filters                  map[string]string                  `json:"filters,omitempty"`
	Counts                   providerFailureOperatorCounts      `json:"counts"`
	CandidateCount           int                                `json:"candidate_count"`
	Rows                     []providerFailureOperatorOutputRow `json:"rows"`
	ExecutesTool             bool                               `json:"executes_tool"`
	MutatesProviderFailure   bool                               `json:"mutates_provider_failure"`
	RequiresOperatorApproval bool                               `json:"requires_operator_approval"`
	RequiresFreshApproval    bool                               `json:"requires_fresh_approval,omitempty"`
	RequiresPreparedAudit    bool                               `json:"requires_prepared_audit,omitempty"`
	RequiresNewInput         bool                               `json:"requires_new_input,omitempty"`
	RequiresReasonSHA256     bool                               `json:"requires_reason_sha256,omitempty"`
	DryRun                   bool                               `json:"dry_run"`
	ReasonPresent            bool                               `json:"reason_present,omitempty"`
	ReasonSHA256             string                             `json:"reason_sha256,omitempty"`
	PermissionGate           string                             `json:"permission_gate,omitempty"`
	AuditContract            string                             `json:"audit_contract,omitempty"`
	EvalGate                 string                             `json:"eval_gate,omitempty"`
	RedriveRequirements      []string                           `json:"redrive_requirements,omitempty"`
	OperatorNextStep         string                             `json:"operator_next_step,omitempty"`
	Note                     string                             `json:"note"`
}

type providerFailureOperatorCounts struct {
	Total        int `json:"total"`
	RetryPending int `json:"retry_pending"`
	DLQ          int `json:"dlq"`
}

type providerFailureOperatorOutputRow struct {
	ReplayCandidateID string `json:"replay_candidate_id,omitempty"`
	ReplayState       string `json:"replay_state,omitempty"`
	TenantID          string `json:"tenant_id"`
	ProviderFailureID string `json:"provider_failure_id"`
	ExecutionID       string `json:"execution_id"`
	ResultID          string `json:"result_id"`
	ProposalID        string `json:"proposal_id"`
	ApprovalID        string `json:"approval_id"`
	PreparedAuditID   string `json:"prepared_audit_id"`
	UserIDHash        string `json:"user_id_hash"`
	SkillID           string `json:"skill_id"`
	ToolName          string `json:"tool_name"`
	ResourceType      string `json:"resource_type"`
	ResourceIDHash    string `json:"resource_id_hash,omitempty"`
	Classification    string `json:"classification"`
	Status            string `json:"status"`
	Retryable         bool   `json:"retryable"`
	RetryCount        int    `json:"retry_count"`
	NextRetryAt       string `json:"next_retry_at,omitempty"`
	DeadLetteredAt    string `json:"dead_lettered_at,omitempty"`
	FailureRefHash    string `json:"failure_ref_hash,omitempty"`
	CreatedAt         string `json:"created_at"`
}

func runProviderFailureAudit(ctx context.Context) error {
	rows, options, err := loadProviderFailureRows(ctx, "NEXUSIM_ACTION_EXECUTOR_PROVIDER_FAILURE_AUDIT")
	if err != nil {
		return err
	}
	output := newProviderFailureOperatorOutput(
		"action-executor.provider-failure.audit",
		options,
		rows,
		false,
		"",
	)
	if outputPath := strings.TrimSpace(os.Getenv("NEXUSIM_ACTION_EXECUTOR_PROVIDER_FAILURE_AUDIT_OUTPUT")); outputPath != "" {
		return writeProviderFailureOperatorOutput(outputPath, output)
	}
	encoded, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(encoded))
	return nil
}

func runProviderFailureRedrivePlan(ctx context.Context) error {
	if !envBool("NEXUSIM_ACTION_EXECUTOR_PROVIDER_FAILURE_REDRIVE_DRY_RUN", false) {
		return errors.New("NEXUSIM_ACTION_EXECUTOR_PROVIDER_FAILURE_REDRIVE_DRY_RUN=true is required; this mode only writes a non-mutating redrive plan")
	}
	reasonSHA256, err := actionProviderFailureReasonSHA256FromEnv("NEXUSIM_ACTION_EXECUTOR_PROVIDER_FAILURE_REDRIVE_REASON_FILE")
	if err != nil {
		return err
	}
	options, err := providerFailureAuditOptionsFromEnv("NEXUSIM_ACTION_EXECUTOR_PROVIDER_FAILURE_REDRIVE")
	if err != nil {
		return err
	}
	if strings.TrimSpace(options.Status) == "" {
		options.Status = types.ProviderFailureStatusDLQ
	}
	rows, options, err := loadProviderFailureRowsWithOptions(ctx, options)
	if err != nil {
		return err
	}
	outputPath := strings.TrimSpace(os.Getenv("NEXUSIM_ACTION_EXECUTOR_PROVIDER_FAILURE_REDRIVE_PLAN_OUTPUT"))
	if outputPath == "" {
		return errors.New("NEXUSIM_ACTION_EXECUTOR_PROVIDER_FAILURE_REDRIVE_PLAN_OUTPUT is required")
	}
	output := newProviderFailureOperatorOutput(
		"action-executor.provider-failure.redrive-plan",
		options,
		rows,
		true,
		reasonSHA256,
	)
	output.OperatorNextStep = "Create or verify a fresh approved Agent proposal for each candidate, then rerun ExecuteApprovedAction through normal policy, preflight safety and audit; this plan does not replay provider output."
	return writeProviderFailureOperatorOutput(outputPath, output)
}

func runProviderFailureReplayOperatorUI(ctx context.Context) error {
	options, err := providerFailureAuditOptionsFromEnv("NEXUSIM_ACTION_EXECUTOR_PROVIDER_REPLAY_UI")
	if err != nil {
		return err
	}
	if strings.TrimSpace(options.Status) == "" {
		options.Status = types.ProviderFailureStatusDLQ
	}
	if strings.ToUpper(strings.TrimSpace(options.Status)) != types.ProviderFailureStatusDLQ {
		return errors.New("NEXUSIM_ACTION_EXECUTOR_PROVIDER_REPLAY_UI_STATUS must be DLQ; replay operator UI only reviews redrivable DLQ candidates")
	}
	rows, options, err := loadProviderFailureRowsWithOptions(ctx, options)
	if err != nil {
		return err
	}
	outputPath := strings.TrimSpace(os.Getenv("NEXUSIM_ACTION_EXECUTOR_PROVIDER_REPLAY_UI_OUTPUT"))
	if outputPath == "" {
		return errors.New("NEXUSIM_ACTION_EXECUTOR_PROVIDER_REPLAY_UI_OUTPUT is required")
	}
	output := newProviderFailureReplayOperatorUIOutput(options, rows)
	return writeProviderFailureOperatorOutput(outputPath, output)
}

func loadProviderFailureRows(ctx context.Context, prefix string) ([]types.ProviderFailureAuditRow, types.ProviderFailureAuditOptions, error) {
	options, err := providerFailureAuditOptionsFromEnv(prefix)
	if err != nil {
		return nil, options, err
	}
	return loadProviderFailureRowsWithOptions(ctx, options)
}

func loadProviderFailureRowsWithOptions(
	ctx context.Context,
	options types.ProviderFailureAuditOptions,
) ([]types.ProviderFailureAuditRow, types.ProviderFailureAuditOptions, error) {
	if err := validateProviderFailureAuditStatus(options.Status); err != nil {
		return nil, options, err
	}
	pool, err := openPGPool(ctx)
	if err != nil {
		return nil, options, err
	}
	defer pool.Close()
	rows, err := postgresinfra.NewRepository(pool).AuditProviderFailures(ctx, options)
	if err != nil {
		return nil, options, err
	}
	return rows, options, nil
}

func providerFailureAuditOptionsFromEnv(prefix string) (types.ProviderFailureAuditOptions, error) {
	limit, err := actionExecutorPositiveLimitFromEnv(prefix+"_LIMIT", 50)
	if err != nil {
		return types.ProviderFailureAuditOptions{}, err
	}
	return types.ProviderFailureAuditOptions{
		TenantID: strings.TrimSpace(os.Getenv(prefix + "_TENANT_ID")),
		Status:   strings.TrimSpace(os.Getenv(prefix + "_STATUS")),
		ToolName: strings.TrimSpace(os.Getenv(prefix + "_TOOL_NAME")),
		Limit:    limit,
	}, nil
}

func validateProviderFailureAuditStatus(status string) error {
	status = strings.TrimSpace(status)
	if status == "" {
		return nil
	}
	switch strings.ToUpper(status) {
	case types.ProviderFailureStatusRetryPending, types.ProviderFailureStatusDLQ:
		return nil
	default:
		return fmt.Errorf("unsupported provider failure status %q", status)
	}
}

func actionExecutorPositiveLimitFromEnv(name string, defaultValue int) (int, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return defaultValue, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	if value > 500 {
		return 0, fmt.Errorf("%s must be <= 500", name)
	}
	return value, nil
}

func newProviderFailureOperatorOutput(
	kind string,
	options types.ProviderFailureAuditOptions,
	rows []types.ProviderFailureAuditRow,
	redrivePlan bool,
	reasonSHA256 string,
) providerFailureOperatorOutput {
	output := providerFailureOperatorOutput{
		SchemaVersion:            1,
		GeneratedAt:              time.Now().UTC().Format(time.RFC3339Nano),
		Kind:                     kind,
		Filters:                  compactProviderFailureFilters(options),
		Rows:                     make([]providerFailureOperatorOutputRow, 0, len(rows)),
		CandidateCount:           len(rows),
		ExecutesTool:             false,
		MutatesProviderFailure:   false,
		RequiresOperatorApproval: redrivePlan,
		DryRun:                   redrivePlan,
		ReasonPresent:            redrivePlan,
		ReasonSHA256:             reasonSHA256,
		Note:                     "Low-sensitive operator artifact. It does not include raw tool input, provider output, reason text, or business payloads.",
	}
	if redrivePlan {
		output.BatchID = providerFailureBatchID(kind, rows, reasonSHA256)
		output.RedriveRequirements = []string{
			"fresh_agent_proposal",
			"fresh_approval",
			"fresh_prepared_audit",
			"matching_skill_tool_resource",
			"new_input_json",
			"reason_sha256",
		}
	}
	for _, row := range rows {
		switch row.Status {
		case types.ProviderFailureStatusRetryPending:
			output.Counts.RetryPending++
		case types.ProviderFailureStatusDLQ:
			output.Counts.DLQ++
		}
		output.Counts.Total++
		output.Rows = append(output.Rows, providerFailureOperatorOutputRow{
			TenantID:          row.TenantID,
			ProviderFailureID: row.ProviderFailureID,
			ExecutionID:       row.ExecutionID,
			ResultID:          row.ResultID,
			ProposalID:        row.ProposalID,
			ApprovalID:        row.ApprovalID,
			PreparedAuditID:   row.PreparedAuditID,
			UserIDHash:        providerFailureSHA256(row.UserID),
			SkillID:           row.SkillID,
			ToolName:          row.ToolName,
			ResourceType:      row.ResourceType,
			ResourceIDHash:    providerFailureSHA256(row.ResourceID),
			Classification:    row.Classification,
			Status:            row.Status,
			Retryable:         row.Retryable,
			RetryCount:        row.RetryCount,
			NextRetryAt:       optionalProviderFailureTime(row.NextRetryAt),
			DeadLetteredAt:    optionalProviderFailureTime(row.DeadLetteredAt),
			FailureRefHash:    providerFailureSHA256(row.FailureRef),
			CreatedAt:         row.CreatedAt.UTC().Format(time.RFC3339Nano),
		})
	}
	return output
}

func newProviderFailureReplayOperatorUIOutput(
	options types.ProviderFailureAuditOptions,
	rows []types.ProviderFailureAuditRow,
) providerFailureOperatorOutput {
	output := newProviderFailureOperatorOutput(
		"action-executor.provider-failure.replay-operator-ui",
		options,
		rows,
		false,
		"",
	)
	output.BatchID = providerFailureBatchID(output.Kind, rows, "")
	output.RequiresOperatorApproval = true
	output.RequiresFreshApproval = true
	output.RequiresPreparedAudit = true
	output.RequiresNewInput = true
	output.RequiresReasonSHA256 = true
	output.DryRun = true
	output.PermissionGate = "policy-service CheckToolAction and fresh agent-service approval are required before RedriveProviderFailure"
	output.AuditContract = "new prepared audit and action_executor_execution_audits redrive lineage are required"
	output.EvalGate = "action-provider-replay-operator-ui-first-path"
	output.RedriveRequirements = []string{
		"operator_review",
		"fresh_agent_proposal",
		"fresh_approval",
		"fresh_prepared_audit",
		"matching_skill_tool_resource",
		"new_input_json",
		"reason_sha256",
	}
	output.OperatorNextStep = "Use this low-sensitive view to create a fresh proposal and approval package; execute RedriveProviderFailure only after policy, approval and prepared audit are verified."
	for index := range output.Rows {
		row := &output.Rows[index]
		row.ReplayCandidateID = providerFailureReplayCandidateID(row.TenantID, row.ProviderFailureID)
		row.ReplayState = "AWAITING_FRESH_APPROVAL"
	}
	return output
}

func providerFailureReplayCandidateID(tenantID string, providerFailureID string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(tenantID) + ":" + strings.TrimSpace(providerFailureID)))
	return "provider-replay-candidate-" + hex.EncodeToString(sum[:])[:16]
}

func providerFailureBatchID(kind string, rows []types.ProviderFailureAuditRow, reasonSHA256 string) string {
	parts := make([]string, 0, len(rows)+2)
	parts = append(parts, strings.TrimSpace(kind), strings.TrimSpace(reasonSHA256))
	for _, row := range rows {
		parts = append(parts, row.TenantID+":"+row.ProviderFailureID)
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return "provider-failure-batch-" + hex.EncodeToString(sum[:])[:16]
}

func compactProviderFailureFilters(options types.ProviderFailureAuditOptions) map[string]string {
	filters := map[string]string{}
	if strings.TrimSpace(options.TenantID) != "" {
		filters["tenant_id"] = strings.TrimSpace(options.TenantID)
	}
	if strings.TrimSpace(options.Status) != "" {
		filters["status"] = strings.ToUpper(strings.TrimSpace(options.Status))
	}
	if strings.TrimSpace(options.ToolName) != "" {
		filters["tool_name"] = strings.TrimSpace(options.ToolName)
	}
	if options.Limit > 0 {
		filters["limit"] = fmt.Sprintf("%d", options.Limit)
	}
	if len(filters) == 0 {
		return nil
	}
	return filters
}

func writeProviderFailureOperatorOutput(path string, output providerFailureOperatorOutput) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}

func optionalProviderFailureTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func providerFailureSHA256(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func actionProviderFailureReasonSHA256FromEnv(reasonFileEnv string) (string, error) {
	path := strings.TrimSpace(os.Getenv(reasonFileEnv))
	if path == "" {
		return "", errors.New(reasonFileEnv + " is required")
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", reasonFileEnv, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("%s must point to a file", reasonFileEnv)
	}
	if info.Size() > actionProviderFailureReasonMaxBytes {
		return "", fmt.Errorf("%s exceeds %d bytes", reasonFileEnv, actionProviderFailureReasonMaxBytes)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", reasonFileEnv, err)
	}
	reason := strings.TrimSpace(string(data))
	if reason == "" {
		return "", errors.New(reasonFileEnv + " is empty")
	}
	return providerFailureSHA256(reason), nil
}
