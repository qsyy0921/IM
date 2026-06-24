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
	HandoffContract          *providerFailureHandoffContract    `json:"handoff_contract,omitempty"`
	AdminOperationRequests   []providerFailureAdminRequest      `json:"admin_operation_requests,omitempty"`
	WorkflowHandoffRequests  []providerFailureWorkflowRequest   `json:"workflow_handoff_requests,omitempty"`
	OperatorNextStep         string                             `json:"operator_next_step,omitempty"`
	Note                     string                             `json:"note"`
}

type providerFailureOperatorCounts struct {
	Total        int `json:"total"`
	RetryPending int `json:"retry_pending"`
	DLQ          int `json:"dlq"`
}

type providerFailureHandoffContract struct {
	AdminOperationType     string   `json:"admin_operation_type"`
	WorkflowType           string   `json:"workflow_type"`
	TargetService          string   `json:"target_service"`
	TargetOperation        string   `json:"target_operation"`
	RedriveEntrypoint      string   `json:"redrive_entrypoint"`
	ApprovalPolicyRef      string   `json:"approval_policy_ref"`
	PayloadSchemaVersion   string   `json:"payload_schema_version"`
	DirectExecutionAllowed bool     `json:"direct_execution_allowed"`
	SourceDLQImmutable     bool     `json:"source_dlq_immutable"`
	Requires               []string `json:"requires"`
}

type providerFailureAdminRequest struct {
	AuthTenantID           string         `json:"auth_tenant_id"`
	OperatorRef            string         `json:"operator_ref"`
	OperatorRole           string         `json:"operator_role"`
	OperationType          string         `json:"operation_type"`
	TargetRefHash          string         `json:"target_ref_hash"`
	RiskLevel              string         `json:"risk_level"`
	PayloadSchemaVersion   string         `json:"payload_schema_version"`
	OperationPayload       map[string]any `json:"operation_payload"`
	OperationPayloadHash   string         `json:"operation_payload_hash"`
	ReasonRef              string         `json:"reason_ref"`
	EvidenceRefs           []string       `json:"evidence_refs"`
	IdempotencyKey         string         `json:"idempotency_key"`
	CorrelationID          string         `json:"correlation_id,omitempty"`
	CausationID            string         `json:"causation_id,omitempty"`
	TraceID                string         `json:"trace_id,omitempty"`
	ExpectedWorkflowPolicy string         `json:"expected_workflow_policy"`
}

type providerFailureWorkflowRequest struct {
	WorkflowType         string   `json:"workflow_type"`
	RequesterService     string   `json:"requester_service"`
	TargetService        string   `json:"target_service"`
	TargetOperation      string   `json:"target_operation"`
	RiskLevel            string   `json:"risk_level"`
	TargetRefHash        string   `json:"target_ref_hash"`
	PayloadSchemaVersion string   `json:"payload_schema_version"`
	PayloadRefHash       string   `json:"payload_ref_hash"`
	ApprovalPolicyRef    string   `json:"approval_policy_ref"`
	ReasonRef            string   `json:"reason_ref"`
	EvidenceRefs         []string `json:"evidence_refs"`
	IdempotencyKey       string   `json:"idempotency_key"`
	CorrelationID        string   `json:"correlation_id,omitempty"`
	CausationID          string   `json:"causation_id,omitempty"`
	TraceID              string   `json:"trace_id,omitempty"`
}

type providerFailureReplayHandoffConfig struct {
	OperatorRef   string
	OperatorRole  string
	ReasonRef     string
	EvidenceRefs  []string
	CorrelationID string
	TraceID       string
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

func runProviderFailureReplayHandoff(ctx context.Context) error {
	options, err := providerFailureAuditOptionsFromEnv("NEXUSIM_ACTION_EXECUTOR_PROVIDER_REPLAY_HANDOFF")
	if err != nil {
		return err
	}
	if strings.TrimSpace(options.Status) == "" {
		options.Status = types.ProviderFailureStatusDLQ
	}
	if strings.ToUpper(strings.TrimSpace(options.Status)) != types.ProviderFailureStatusDLQ {
		return errors.New("NEXUSIM_ACTION_EXECUTOR_PROVIDER_REPLAY_HANDOFF_STATUS must be DLQ; replay handoff only creates requests for redrivable DLQ candidates")
	}
	config, err := providerFailureReplayHandoffConfigFromEnv()
	if err != nil {
		return err
	}
	rows, options, err := loadProviderFailureRowsWithOptions(ctx, options)
	if err != nil {
		return err
	}
	outputPath := strings.TrimSpace(os.Getenv("NEXUSIM_ACTION_EXECUTOR_PROVIDER_REPLAY_HANDOFF_OUTPUT"))
	if outputPath == "" {
		return errors.New("NEXUSIM_ACTION_EXECUTOR_PROVIDER_REPLAY_HANDOFF_OUTPUT is required")
	}
	output := newProviderFailureReplayHandoffOutput(options, rows, config)
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

func providerFailureReplayHandoffConfigFromEnv() (providerFailureReplayHandoffConfig, error) {
	config := providerFailureReplayHandoffConfig{
		OperatorRef:   strings.TrimSpace(os.Getenv("NEXUSIM_ACTION_EXECUTOR_PROVIDER_REPLAY_HANDOFF_OPERATOR_REF")),
		OperatorRole:  strings.ToUpper(strings.TrimSpace(os.Getenv("NEXUSIM_ACTION_EXECUTOR_PROVIDER_REPLAY_HANDOFF_OPERATOR_ROLE"))),
		ReasonRef:     strings.TrimSpace(os.Getenv("NEXUSIM_ACTION_EXECUTOR_PROVIDER_REPLAY_HANDOFF_REASON_REF")),
		EvidenceRefs:  splitCSV(os.Getenv("NEXUSIM_ACTION_EXECUTOR_PROVIDER_REPLAY_HANDOFF_EVIDENCE_REFS")),
		CorrelationID: strings.TrimSpace(os.Getenv("NEXUSIM_ACTION_EXECUTOR_PROVIDER_REPLAY_HANDOFF_CORRELATION_ID")),
		TraceID:       strings.TrimSpace(os.Getenv("NEXUSIM_ACTION_EXECUTOR_PROVIDER_REPLAY_HANDOFF_TRACE_ID")),
	}
	if config.OperatorRef == "" {
		return config, errors.New("NEXUSIM_ACTION_EXECUTOR_PROVIDER_REPLAY_HANDOFF_OPERATOR_REF is required")
	}
	if config.OperatorRole == "" {
		config.OperatorRole = "OPERATOR"
	}
	if config.ReasonRef == "" {
		return config, errors.New("NEXUSIM_ACTION_EXECUTOR_PROVIDER_REPLAY_HANDOFF_REASON_REF is required")
	}
	if len(config.EvidenceRefs) == 0 {
		return config, errors.New("NEXUSIM_ACTION_EXECUTOR_PROVIDER_REPLAY_HANDOFF_EVIDENCE_REFS is required")
	}
	if !providerFailureLowSensitiveRef(config.OperatorRef) ||
		!providerFailureLowSensitiveRef(config.ReasonRef) ||
		!providerFailureLowSensitiveRef(config.CorrelationID) ||
		!providerFailureLowSensitiveRef(config.TraceID) {
		return config, errors.New("provider replay handoff refs must be low-sensitive")
	}
	for _, ref := range config.EvidenceRefs {
		if !providerFailureLowSensitiveRef(ref) {
			return config, errors.New("provider replay handoff evidence refs must be low-sensitive")
		}
	}
	return config, nil
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

func newProviderFailureReplayHandoffOutput(
	options types.ProviderFailureAuditOptions,
	rows []types.ProviderFailureAuditRow,
	config providerFailureReplayHandoffConfig,
) providerFailureOperatorOutput {
	output := newProviderFailureOperatorOutput(
		"action-executor.provider-failure.replay-admin-workflow-handoff",
		options,
		rows,
		false,
		"",
	)
	output.BatchID = providerFailureBatchID(output.Kind, rows, config.ReasonRef)
	output.RequiresOperatorApproval = true
	output.RequiresFreshApproval = true
	output.RequiresPreparedAudit = true
	output.RequiresNewInput = true
	output.RequiresReasonSHA256 = true
	output.DryRun = true
	output.PermissionGate = "admin-service creates PROVIDER_REPLAY_REQUEST; workflow-service records REPAIR_APPROVAL; action-executor RedriveProviderFailure remains the only execution entrypoint"
	output.AuditContract = "admin operation, workflow approval, fresh agent approval, prepared audit and action-executor redrive lineage are all required"
	output.EvalGate = "action-provider-replay-admin-workflow-handoff"
	output.RedriveRequirements = providerReplayRequiredGates()
	output.HandoffContract = &providerFailureHandoffContract{
		AdminOperationType:     "PROVIDER_REPLAY_REQUEST",
		WorkflowType:           "REPAIR_APPROVAL",
		TargetService:          "action-executor",
		TargetOperation:        "PROVIDER_REPLAY_REQUEST",
		RedriveEntrypoint:      "RedriveProviderFailure",
		ApprovalPolicyRef:      "admin.workflow.provider_replay.v1",
		PayloadSchemaVersion:   "admin.provider_replay_request.v1",
		DirectExecutionAllowed: false,
		SourceDLQImmutable:     true,
		Requires:               providerReplayRequiredGates(),
	}
	output.OperatorNextStep = "Submit each admin_operation_request to admin-service; after workflow approval, create fresh Agent proposal / approval / prepared audit and call action-executor.RedriveProviderFailure with new input_json and reason_sha256."
	output.AdminOperationRequests = make([]providerFailureAdminRequest, 0, len(output.Rows))
	output.WorkflowHandoffRequests = make([]providerFailureWorkflowRequest, 0, len(output.Rows))
	for index := range output.Rows {
		row := &output.Rows[index]
		row.ReplayCandidateID = providerFailureReplayCandidateID(row.TenantID, row.ProviderFailureID)
		row.ReplayState = "AWAITING_ADMIN_WORKFLOW"
		adminRequest := providerFailureAdminHandoffRequest(*row, config)
		output.AdminOperationRequests = append(output.AdminOperationRequests, adminRequest)
		output.WorkflowHandoffRequests = append(output.WorkflowHandoffRequests, providerFailureWorkflowHandoffRequest(adminRequest))
	}
	return output
}

func providerFailureAdminHandoffRequest(
	row providerFailureOperatorOutputRow,
	config providerFailureReplayHandoffConfig,
) providerFailureAdminRequest {
	payload := map[string]any{
		"provider_failure_ref_hash": row.ProviderFailureRefHash(),
		"source_execution_ref_hash": "sha256:" + providerFailureSHA256(row.ExecutionID),
		"source_result_ref_hash":    "sha256:" + providerFailureSHA256(row.ResultID),
		"replay_candidate_id":       row.ReplayCandidateID,
		"redrive_entrypoint":        "RedriveProviderFailure",
		"requires_fresh_proposal":   true,
		"requires_fresh_approval":   true,
		"requires_prepared_audit":   true,
		"requires_new_input":        true,
		"requires_reason_sha256":    true,
		"source_dlq_immutable":      true,
		"direct_execution_allowed":  false,
	}
	payloadHash := providerFailurePayloadHash(payload)
	return providerFailureAdminRequest{
		AuthTenantID:           row.TenantID,
		OperatorRef:            strings.TrimSpace(config.OperatorRef),
		OperatorRole:           providerFailureFirstNonEmpty(strings.ToUpper(strings.TrimSpace(config.OperatorRole)), "OPERATOR"),
		OperationType:          "PROVIDER_REPLAY_REQUEST",
		TargetRefHash:          row.ProviderFailureRefHash(),
		RiskLevel:              "HIGH",
		PayloadSchemaVersion:   "admin.provider_replay_request.v1",
		OperationPayload:       payload,
		OperationPayloadHash:   payloadHash,
		ReasonRef:              strings.TrimSpace(config.ReasonRef),
		EvidenceRefs:           append([]string(nil), config.EvidenceRefs...),
		IdempotencyKey:         "provider-replay-admin:" + row.ReplayCandidateID,
		CorrelationID:          strings.TrimSpace(config.CorrelationID),
		CausationID:            row.ProviderFailureID,
		TraceID:                strings.TrimSpace(config.TraceID),
		ExpectedWorkflowPolicy: "admin.workflow.provider_replay.v1",
	}
}

func providerFailureWorkflowHandoffRequest(adminRequest providerFailureAdminRequest) providerFailureWorkflowRequest {
	return providerFailureWorkflowRequest{
		WorkflowType:         "REPAIR_APPROVAL",
		RequesterService:     "admin-service",
		TargetService:        "action-executor",
		TargetOperation:      "PROVIDER_REPLAY_REQUEST",
		RiskLevel:            adminRequest.RiskLevel,
		TargetRefHash:        adminRequest.TargetRefHash,
		PayloadSchemaVersion: adminRequest.PayloadSchemaVersion,
		PayloadRefHash:       adminRequest.OperationPayloadHash,
		ApprovalPolicyRef:    adminRequest.ExpectedWorkflowPolicy,
		ReasonRef:            adminRequest.ReasonRef,
		EvidenceRefs:         append([]string(nil), adminRequest.EvidenceRefs...),
		IdempotencyKey:       "admin-workflow:${operation_id}",
		CorrelationID:        adminRequest.CorrelationID,
		CausationID:          "${operation_id}",
		TraceID:              adminRequest.TraceID,
	}
}

func providerReplayRequiredGates() []string {
	return []string{
		"admin_operation_request",
		"workflow_repair_approval",
		"fresh_agent_proposal",
		"fresh_agent_approval",
		"fresh_prepared_audit",
		"matching_skill_tool_resource",
		"new_input_json",
		"reason_sha256",
		"action_executor_redrive_entrypoint",
	}
}

func (row providerFailureOperatorOutputRow) ProviderFailureRefHash() string {
	return "sha256:" + providerFailureSHA256(row.TenantID+":"+row.ProviderFailureID)
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

func providerFailurePayloadHash(payload map[string]any) string {
	encoded, _ := json.Marshal(payload)
	return "sha256:" + providerFailureSHA256(string(encoded))
}

func providerFailureFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func providerFailureLowSensitiveRef(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return true
	}
	for _, marker := range []string{"password", "token", "secret", "api_key", "apikey", "private://", "raw:", "dsn=", "postgres://", "http://", "https://", "message_body", "provider_body", "prompt"} {
		if strings.Contains(value, marker) {
			return false
		}
	}
	return len(value) <= 256
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
