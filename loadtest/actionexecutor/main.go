package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	actionexecutorv1 "github.com/qsyy0921/IM/api/proto/nexusim/actionexecutor/v1"
	auditv1 "github.com/qsyy0921/IM/api/proto/nexusim/audit/v1"
	policyv1 "github.com/qsyy0921/IM/api/proto/nexusim/policy/v1"
	"github.com/qsyy0921/IM/loadtest/internal/grpctls"
	"google.golang.org/grpc"
)

const (
	defaultActionExecutorTarget = "127.0.0.1:10660"
	defaultAuditTarget          = "127.0.0.1:10700"
	maxInputJSONBytes           = 64 * 1024
	maxReasonBytes              = 4096
	maxResourceIDBytes          = 2048
)

type config struct {
	mode            string
	target          string
	auditTarget     string
	requestTimeout  time.Duration
	tls             grpctls.Config
	auditTLS        grpctls.Config
	manifestPath    string
	auditManifest   string
	resourceIDPath  string
	inputJSONPath   string
	reasonPath      string
	operatorUserID  string
	operatorDevice  string
	operatorSession string
	traceID         string
	requestID       string
	execute         bool
}

type redriveInvocationManifest struct {
	SchemaVersion       string `json:"schema_version"`
	ManifestID          string `json:"manifest_id"`
	Entrypoint          string `json:"entrypoint"`
	RPCFullMethod       string `json:"rpc_full_method"`
	ExecutesRedrive     bool   `json:"executes_redrive"`
	MutatesFailure      bool   `json:"mutates_provider_failure"`
	SourceDLQImmutable  bool   `json:"source_dlq_immutable"`
	DirectExecution     bool   `json:"direct_execution_allowed"`
	RequiresExecution   bool   `json:"requires_operator_execution"`
	ProviderFailureID   string `json:"provider_failure_id"`
	ReplayCandidateID   string `json:"replay_candidate_id"`
	AdminOperationID    string `json:"admin_operation_id"`
	WorkflowID          string `json:"workflow_id"`
	WorkflowStepID      string `json:"workflow_step_id"`
	ProposalID          string `json:"proposal_id"`
	ApprovalID          string `json:"approval_id"`
	PreparedAuditID     string `json:"prepared_audit_id"`
	SkillID             string `json:"skill_id"`
	ToolName            string `json:"tool_name"`
	Action              string `json:"action"`
	ResourceType        string `json:"resource_type"`
	ResourceIDHash      string `json:"resource_id_hash"`
	NewInputSHA256      string `json:"new_input_sha256"`
	ReasonSHA256        string `json:"reason_sha256"`
	AuthContextContract struct {
		TenantID string `json:"tenant_id"`
		TraceID  string `json:"trace_id"`
	} `json:"auth_context_contract"`
	RedriveRequestContract struct {
		ProviderFailureID string `json:"provider_failure_id"`
		ReasonSHA256      string `json:"reason_sha256"`
		ProposalID        string `json:"proposal_id"`
		ApprovalID        string `json:"approval_id"`
		PreparedAuditID   string `json:"prepared_audit_id"`
		SkillID           string `json:"skill_id"`
		ToolName          string `json:"tool_name"`
		Action            string `json:"action"`
		ResourceType      string `json:"resource_type"`
		ResourceIDHash    string `json:"resource_id_hash"`
		RiskLevel         string `json:"risk_level"`
		Intent            string `json:"intent"`
		NewInputSHA256    string `json:"new_input_sha256"`
		IdempotencyKey    string `json:"idempotency_key_hint"`
	} `json:"redrive_request_contract"`
	RequiredChecks    []string `json:"required_checks"`
	ForbiddenContents []string `json:"forbidden_contents"`
}

type preparedRedrive struct {
	Manifest       redriveInvocationManifest
	Request        *actionexecutorv1.RedriveProviderFailureRequest
	InputSHA256    string
	ReasonSHA256   string
	ResourceIDHash string
	Verified       []string
}

type commandResult struct {
	Mode              string          `json:"mode"`
	Target            string          `json:"target,omitempty"`
	ManifestID        string          `json:"manifest_id"`
	ReplayCandidateID string          `json:"replay_candidate_id"`
	ProviderFailureID string          `json:"provider_failure_id"`
	AdminOperationID  string          `json:"admin_operation_id"`
	WorkflowID        string          `json:"workflow_id"`
	Request           requestSummary  `json:"request"`
	Response          *redriveSummary `json:"response,omitempty"`
	ExecutedRedrive   bool            `json:"executed_redrive"`
	Verified          []string        `json:"verified"`
	CheckedAt         time.Time       `json:"checked_at"`
}

type requestSummary struct {
	TenantID        string `json:"tenant_id"`
	UserID          string `json:"user_id"`
	DeviceID        string `json:"device_id"`
	ProposalID      string `json:"proposal_id"`
	ApprovalID      string `json:"approval_id"`
	PreparedAuditID string `json:"prepared_audit_id"`
	SkillID         string `json:"skill_id"`
	ToolName        string `json:"tool_name"`
	ResourceType    string `json:"resource_type"`
	ResourceIDHash  string `json:"resource_id_hash"`
	InputSHA256     string `json:"input_sha256"`
	ReasonSHA256    string `json:"reason_sha256"`
	IdempotencyKey  string `json:"idempotency_key"`
}

type redriveSummary struct {
	ProviderFailureID  string `json:"provider_failure_id"`
	SourceExecutionID  string `json:"source_execution_id"`
	SourceResultID     string `json:"source_result_id"`
	RedriveExecutionID string `json:"redrive_execution_id"`
	RedriveResultID    string `json:"redrive_result_id"`
	ProposalID         string `json:"proposal_id"`
	ApprovalID         string `json:"approval_id"`
	PreparedAuditID    string `json:"prepared_audit_id"`
	SkillID            string `json:"skill_id"`
	ToolName           string `json:"tool_name"`
	ResourceType       string `json:"resource_type"`
	ResourceIDHash     string `json:"resource_id_hash"`
	Status             string `json:"status"`
	ResultStatus       string `json:"result_status"`
	Executed           bool   `json:"executed"`
	Classification     string `json:"classification,omitempty"`
	Reason             string `json:"reason,omitempty"`
	ResultRef          string `json:"result_ref,omitempty"`
}

type actionExecutorClient interface {
	RedriveProviderFailure(context.Context, *actionexecutorv1.RedriveProviderFailureRequest, ...grpc.CallOption) (*actionexecutorv1.RedriveProviderFailureResponse, error)
}

type auditClient interface {
	AppendAuditRecord(context.Context, *auditv1.AppendAuditRecordRequest, ...grpc.CallOption) (*auditv1.AppendAuditRecordResponse, error)
}

type operatorClients struct {
	actionExecutor actionExecutorClient
	audit          auditClient
}

func main() {
	cfg, err := parseFlags(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if err := run(context.Background(), cfg, os.Stdout, operatorClients{}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func parseFlags(args []string) (config, error) {
	cfg := config{}
	flags := flag.NewFlagSet("actionexecutor-operator", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&cfg.mode, "mode", "provider-replay-redrive", "mode: provider-replay-redrive, external-audit-append")
	flags.StringVar(&cfg.target, "target", envOr("NEXUSIM_ACTION_EXECUTOR_GRPC_ADDR", defaultActionExecutorTarget), "action-executor gRPC target")
	flags.StringVar(&cfg.auditTarget, "audit-target", envOr("NEXUSIM_AUDIT_GRPC_ADDR", defaultAuditTarget), "audit-service gRPC target")
	flags.DurationVar(&cfg.requestTimeout, "request-timeout", 5*time.Second, "request timeout")
	flags.StringVar(&cfg.tls.CAFile, "action-executor-tls-ca-file", os.Getenv("NEXUSIM_ACTION_EXECUTOR_TLS_CA_FILE"), "CA PEM for action-executor gRPC TLS")
	flags.StringVar(&cfg.tls.ServerName, "action-executor-tls-server-name", os.Getenv("NEXUSIM_ACTION_EXECUTOR_TLS_SERVER_NAME"), "server name for action-executor gRPC TLS")
	flags.StringVar(&cfg.tls.ClientCertFile, "action-executor-tls-client-cert-file", os.Getenv("NEXUSIM_ACTION_EXECUTOR_TLS_CLIENT_CERT_FILE"), "client certificate PEM for action-executor mTLS")
	flags.StringVar(&cfg.tls.ClientKeyFile, "action-executor-tls-client-key-file", os.Getenv("NEXUSIM_ACTION_EXECUTOR_TLS_CLIENT_KEY_FILE"), "client private key PEM for action-executor mTLS")
	flags.StringVar(&cfg.auditTLS.CAFile, "audit-tls-ca-file", os.Getenv("NEXUSIM_AUDIT_TLS_CA_FILE"), "CA PEM for audit-service gRPC TLS")
	flags.StringVar(&cfg.auditTLS.ServerName, "audit-tls-server-name", os.Getenv("NEXUSIM_AUDIT_TLS_SERVER_NAME"), "server name for audit-service gRPC TLS")
	flags.StringVar(&cfg.auditTLS.ClientCertFile, "audit-tls-client-cert-file", os.Getenv("NEXUSIM_AUDIT_TLS_CLIENT_CERT_FILE"), "client certificate PEM for audit-service mTLS")
	flags.StringVar(&cfg.auditTLS.ClientKeyFile, "audit-tls-client-key-file", os.Getenv("NEXUSIM_AUDIT_TLS_CLIENT_KEY_FILE"), "client private key PEM for audit-service mTLS")
	flags.StringVar(&cfg.manifestPath, "manifest", os.Getenv("NEXUSIM_ACTION_EXECUTOR_REDRIVE_MANIFEST"), "provider replay redrive invocation manifest")
	flags.StringVar(&cfg.auditManifest, "audit-manifest", os.Getenv("NEXUSIM_ACTION_EXECUTOR_EXTERNAL_AUDIT_MANIFEST"), "action-executor external audit append manifest")
	flags.StringVar(&cfg.resourceIDPath, "resource-id-file", os.Getenv("NEXUSIM_ACTION_EXECUTOR_REDRIVE_RESOURCE_ID_FILE"), "external raw resource id file")
	flags.StringVar(&cfg.inputJSONPath, "input-json-file", os.Getenv("NEXUSIM_ACTION_EXECUTOR_REDRIVE_INPUT_JSON_FILE"), "external new input JSON file")
	flags.StringVar(&cfg.reasonPath, "reason-file", os.Getenv("NEXUSIM_ACTION_EXECUTOR_REDRIVE_REASON_FILE"), "external redrive reason file")
	flags.StringVar(&cfg.operatorUserID, "operator-user-id", os.Getenv("NEXUSIM_OPERATOR_USER_ID"), "operator user id for auth context")
	flags.StringVar(&cfg.operatorDevice, "operator-device-id", os.Getenv("NEXUSIM_OPERATOR_DEVICE_ID"), "operator device id for auth context")
	flags.StringVar(&cfg.operatorSession, "operator-session-id", os.Getenv("NEXUSIM_OPERATOR_SESSION_ID"), "operator session id for auth context")
	flags.StringVar(&cfg.traceID, "trace-id", "", "trace id")
	flags.StringVar(&cfg.requestID, "request-id", "", "request id")
	flags.BoolVar(&cfg.execute, "execute", false, "actually call action-executor RedriveProviderFailure after preflight")
	if err := flags.Parse(args); err != nil {
		return config{}, err
	}
	cfg.mode = strings.ToLower(strings.TrimSpace(cfg.mode))
	cfg.target = strings.TrimSpace(cfg.target)
	cfg.auditTarget = strings.TrimSpace(cfg.auditTarget)
	cfg.manifestPath = strings.TrimSpace(cfg.manifestPath)
	cfg.auditManifest = strings.TrimSpace(cfg.auditManifest)
	cfg.resourceIDPath = strings.TrimSpace(cfg.resourceIDPath)
	cfg.inputJSONPath = strings.TrimSpace(cfg.inputJSONPath)
	cfg.reasonPath = strings.TrimSpace(cfg.reasonPath)
	cfg.operatorUserID = strings.TrimSpace(cfg.operatorUserID)
	cfg.operatorDevice = strings.TrimSpace(cfg.operatorDevice)
	cfg.operatorSession = strings.TrimSpace(cfg.operatorSession)
	cfg.traceID = strings.TrimSpace(cfg.traceID)
	cfg.requestID = strings.TrimSpace(cfg.requestID)
	if cfg.requestID == "" {
		cfg.requestID = "actionexecutor-provider-replay-redrive"
	}
	if cfg.traceID == "" {
		cfg.traceID = cfg.requestID
	}
	return cfg, nil
}

func run(ctx context.Context, cfg config, out io.Writer, clients operatorClients) error {
	switch cfg.mode {
	case "provider-replay-redrive":
		return runProviderReplayRedrive(ctx, cfg, out, clients.actionExecutor)
	case "external-audit-append":
		return runExternalAuditAppend(ctx, cfg, out, clients.audit)
	default:
		return fmt.Errorf("unsupported mode: %s", cfg.mode)
	}
}

func runProviderReplayRedrive(ctx context.Context, cfg config, out io.Writer, client actionExecutorClient) error {
	prepared, err := prepareRedrive(cfg)
	if err != nil {
		return err
	}
	result := commandResult{
		Mode:              cfg.mode,
		Target:            cfg.target,
		ManifestID:        prepared.Manifest.ManifestID,
		ReplayCandidateID: prepared.Manifest.ReplayCandidateID,
		ProviderFailureID: prepared.Manifest.ProviderFailureID,
		AdminOperationID:  prepared.Manifest.AdminOperationID,
		WorkflowID:        prepared.Manifest.WorkflowID,
		Request:           summarizeRequest(prepared),
		ExecutedRedrive:   false,
		Verified:          prepared.Verified,
		CheckedAt:         time.Now().UTC(),
	}
	if cfg.execute {
		if cfg.target == "" {
			return errors.New("--target is required when --execute is set")
		}
		if client == nil {
			dialOption, err := grpctls.DialOption(cfg.tls, "action-executor-tls")
			if err != nil {
				return err
			}
			conn, err := grpc.NewClient("passthrough:///"+cfg.target, dialOption)
			if err != nil {
				return fmt.Errorf("dial action-executor: %w", err)
			}
			defer conn.Close()
			client = actionexecutorv1.NewActionExecutorServiceClient(conn)
		}
		requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
		defer cancel()
		response, err := client.RedriveProviderFailure(requestCtx, prepared.Request)
		if err != nil {
			return fmt.Errorf("redrive provider failure: %w", err)
		}
		result.ExecutedRedrive = true
		result.Response = summarizeRedriveResponse(response)
	}
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

func prepareRedrive(cfg config) (preparedRedrive, error) {
	if cfg.manifestPath == "" {
		return preparedRedrive{}, errors.New("--manifest is required")
	}
	if cfg.resourceIDPath == "" {
		return preparedRedrive{}, errors.New("--resource-id-file is required")
	}
	if cfg.inputJSONPath == "" {
		return preparedRedrive{}, errors.New("--input-json-file is required")
	}
	if cfg.reasonPath == "" {
		return preparedRedrive{}, errors.New("--reason-file is required")
	}
	if cfg.operatorUserID == "" {
		return preparedRedrive{}, errors.New("--operator-user-id is required")
	}
	if cfg.operatorDevice == "" {
		return preparedRedrive{}, errors.New("--operator-device-id is required")
	}
	for field, path := range map[string]string{
		"manifest":         cfg.manifestPath,
		"resource-id-file": cfg.resourceIDPath,
		"input-json-file":  cfg.inputJSONPath,
		"reason-file":      cfg.reasonPath,
	} {
		if err := requireExternalFile(path, field); err != nil {
			return preparedRedrive{}, err
		}
	}
	manifest, rawManifest, err := readManifest(cfg.manifestPath)
	if err != nil {
		return preparedRedrive{}, err
	}
	if err := validateManifest(manifest, rawManifest); err != nil {
		return preparedRedrive{}, err
	}
	inputBytes, err := readBoundedFile(cfg.inputJSONPath, maxInputJSONBytes, "input-json-file")
	if err != nil {
		return preparedRedrive{}, err
	}
	if !json.Valid(inputBytes) {
		return preparedRedrive{}, errors.New("input-json-file must contain valid JSON")
	}
	reasonBytes, err := readBoundedFile(cfg.reasonPath, maxReasonBytes, "reason-file")
	if err != nil {
		return preparedRedrive{}, err
	}
	if strings.TrimSpace(string(reasonBytes)) == "" {
		return preparedRedrive{}, errors.New("reason-file is empty")
	}
	resourceBytes, err := readBoundedFile(cfg.resourceIDPath, maxResourceIDBytes, "resource-id-file")
	if err != nil {
		return preparedRedrive{}, err
	}
	resourceID := strings.TrimSpace(string(resourceBytes))
	if resourceID == "" {
		return preparedRedrive{}, errors.New("resource-id-file is empty")
	}
	inputSHA := sha256Ref(inputBytes)
	if inputSHA != strings.ToLower(strings.TrimSpace(manifest.NewInputSHA256)) {
		return preparedRedrive{}, errors.New("input-json-file sha256 does not match manifest new_input_sha256")
	}
	reasonSHA := sha256Ref(reasonBytes)
	if reasonSHA != strings.ToLower(strings.TrimSpace(manifest.ReasonSHA256)) {
		return preparedRedrive{}, errors.New("reason-file sha256 does not match manifest reason_sha256")
	}
	resourceHash := sha256Hex([]byte(resourceID))
	if resourceHash != normalizeHashRef(manifest.ResourceIDHash) {
		return preparedRedrive{}, errors.New("resource-id-file sha256 does not match manifest resource_id_hash")
	}
	reasonHex := strings.TrimPrefix(reasonSHA, "sha256:")
	request := &actionexecutorv1.RedriveProviderFailureRequest{
		AuthContext: &actionexecutorv1.AuthContext{
			TenantId:  manifest.AuthContextContract.TenantID,
			UserId:    cfg.operatorUserID,
			DeviceId:  cfg.operatorDevice,
			SessionId: cfg.operatorSession,
			TraceId:   firstNonEmpty(cfg.traceID, manifest.AuthContextContract.TraceID),
			RequestId: cfg.requestID,
		},
		ProviderFailureId: manifest.ProviderFailureID,
		ReasonSha256:      reasonHex,
		ProposalId:        manifest.ProposalID,
		ApprovalId:        manifest.ApprovalID,
		PreparedAuditId:   manifest.PreparedAuditID,
		SkillId:           manifest.SkillID,
		ToolName:          manifest.ToolName,
		Action:            policyv1.ToolAction_TOOL_ACTION_EXECUTE,
		ResourceType:      manifest.ResourceType,
		ResourceId:        resourceID,
		RiskLevel:         manifest.RedriveRequestContract.RiskLevel,
		Intent:            firstNonEmpty(manifest.RedriveRequestContract.Intent, "provider failure redrive after approved repair workflow"),
		InputJson:         string(inputBytes),
		IdempotencyKey:    manifest.RedriveRequestContract.IdempotencyKey,
	}
	return preparedRedrive{
		Manifest:       manifest,
		Request:        request,
		InputSHA256:    inputSHA,
		ReasonSHA256:   reasonSHA,
		ResourceIDHash: manifest.ResourceIDHash,
		Verified: []string{
			"manifest_contract_valid",
			"operator_raw_resource_id_hash_matches",
			"operator_new_input_sha256_matches",
			"operator_reason_sha256_matches",
			"request_targets_action_executor_redrive_provider_failure",
		},
	}, nil
}

func readManifest(path string) (redriveInvocationManifest, string, error) {
	data, err := readBoundedFile(path, 128*1024, "manifest")
	if err != nil {
		return redriveInvocationManifest{}, "", err
	}
	var manifest redriveInvocationManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return redriveInvocationManifest{}, "", fmt.Errorf("manifest must be valid JSON: %w", err)
	}
	return manifest, string(data), nil
}

func validateManifest(manifest redriveInvocationManifest, raw string) error {
	required := map[string]string{
		"schema_version":         manifest.SchemaVersion,
		"manifest_id":            manifest.ManifestID,
		"provider_failure_id":    manifest.ProviderFailureID,
		"replay_candidate_id":    manifest.ReplayCandidateID,
		"admin_operation_id":     manifest.AdminOperationID,
		"workflow_id":            manifest.WorkflowID,
		"workflow_step_id":       manifest.WorkflowStepID,
		"proposal_id":            manifest.ProposalID,
		"approval_id":            manifest.ApprovalID,
		"prepared_audit_id":      manifest.PreparedAuditID,
		"skill_id":               manifest.SkillID,
		"tool_name":              manifest.ToolName,
		"resource_type":          manifest.ResourceType,
		"resource_id_hash":       manifest.ResourceIDHash,
		"new_input_sha256":       manifest.NewInputSHA256,
		"reason_sha256":          manifest.ReasonSHA256,
		"auth_context.tenant_id": manifest.AuthContextContract.TenantID,
	}
	for field, value := range required {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("manifest %s is required", field)
		}
	}
	if manifest.SchemaVersion != "nexusim.action_executor.provider_replay_redrive_invocation.v1" {
		return errors.New("unsupported manifest schema_version")
	}
	if manifest.Entrypoint != "RedriveProviderFailure" ||
		manifest.RPCFullMethod != "/nexusim.actionexecutor.v1.ActionExecutorService/RedriveProviderFailure" {
		return errors.New("manifest must target RedriveProviderFailure")
	}
	if manifest.ExecutesRedrive || manifest.MutatesFailure || manifest.DirectExecution {
		return errors.New("manifest must not claim it already executes or mutates provider failure state")
	}
	if !manifest.SourceDLQImmutable || !manifest.RequiresExecution {
		return errors.New("manifest must require operator execution while preserving source DLQ immutability")
	}
	if strings.ToUpper(strings.TrimSpace(manifest.Action)) != "EXECUTE" ||
		strings.ToUpper(strings.TrimSpace(manifest.RedriveRequestContract.Action)) != "EXECUTE" {
		return errors.New("redrive request action must be EXECUTE")
	}
	if manifest.RedriveRequestContract.ProviderFailureID != manifest.ProviderFailureID ||
		manifest.RedriveRequestContract.ProposalID != manifest.ProposalID ||
		manifest.RedriveRequestContract.ApprovalID != manifest.ApprovalID ||
		manifest.RedriveRequestContract.PreparedAuditID != manifest.PreparedAuditID ||
		manifest.RedriveRequestContract.SkillID != manifest.SkillID ||
		manifest.RedriveRequestContract.ToolName != manifest.ToolName ||
		manifest.RedriveRequestContract.ResourceType != manifest.ResourceType ||
		normalizeHashRef(manifest.RedriveRequestContract.ResourceIDHash) != normalizeHashRef(manifest.ResourceIDHash) ||
		strings.ToLower(strings.TrimSpace(manifest.RedriveRequestContract.NewInputSHA256)) != strings.ToLower(strings.TrimSpace(manifest.NewInputSHA256)) {
		return errors.New("redrive request contract does not match manifest top-level fields")
	}
	if !isSHA256Ref(manifest.NewInputSHA256) || !isSHA256Ref(manifest.ReasonSHA256) {
		return errors.New("new_input_sha256 and reason_sha256 must be sha256:<hex>")
	}
	if !isSHA256HexOrRef(manifest.ResourceIDHash) {
		return errors.New("resource_id_hash must be sha256 hex or sha256:<hex>")
	}
	for _, check := range []string{
		"admin_operation_approved",
		"workflow_approval_recorded",
		"fresh_agent_proposal",
		"fresh_agent_approval",
		"fresh_prepared_audit",
		"new_input_sha256_matches_external_file",
		"reason_sha256_matches_external_file",
		"resource_id_hash_matches_operator_supplied_resource",
		"action_executor_redrive_provider_failure_only",
	} {
		if !contains(manifest.RequiredChecks, check) {
			return fmt.Errorf("manifest missing required check: %s", check)
		}
	}
	if containsSensitiveManifestText(raw) {
		return errors.New("manifest contains sensitive-looking content")
	}
	return nil
}

func summarizeRequest(prepared preparedRedrive) requestSummary {
	request := prepared.Request
	return requestSummary{
		TenantID:        request.GetAuthContext().GetTenantId(),
		UserID:          request.GetAuthContext().GetUserId(),
		DeviceID:        request.GetAuthContext().GetDeviceId(),
		ProposalID:      request.GetProposalId(),
		ApprovalID:      request.GetApprovalId(),
		PreparedAuditID: request.GetPreparedAuditId(),
		SkillID:         request.GetSkillId(),
		ToolName:        request.GetToolName(),
		ResourceType:    request.GetResourceType(),
		ResourceIDHash:  prepared.ResourceIDHash,
		InputSHA256:     prepared.InputSHA256,
		ReasonSHA256:    prepared.ReasonSHA256,
		IdempotencyKey:  request.GetIdempotencyKey(),
	}
}

func summarizeRedriveResponse(response *actionexecutorv1.RedriveProviderFailureResponse) *redriveSummary {
	if response == nil {
		return nil
	}
	return &redriveSummary{
		ProviderFailureID:  response.GetProviderFailureId(),
		SourceExecutionID:  response.GetSourceExecutionId(),
		SourceResultID:     response.GetSourceResultId(),
		RedriveExecutionID: response.GetRedriveExecutionId(),
		RedriveResultID:    response.GetRedriveResultId(),
		ProposalID:         response.GetProposalId(),
		ApprovalID:         response.GetApprovalId(),
		PreparedAuditID:    response.GetPreparedAuditId(),
		SkillID:            response.GetSkillId(),
		ToolName:           response.GetToolName(),
		ResourceType:       response.GetResourceType(),
		ResourceIDHash:     "sha256:" + sha256Hex([]byte(strings.TrimSpace(response.GetResourceId()))),
		Status:             response.GetStatus().String(),
		ResultStatus:       response.GetResultStatus(),
		Executed:           response.GetExecuted(),
		Classification:     response.GetClassification(),
		Reason:             response.GetReason(),
		ResultRef:          response.GetResultRef(),
	}
}

func requireExternalFile(path string, field string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("--%s is required", field)
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", field, err)
	}
	if info.IsDir() {
		return fmt.Errorf("%s must point to a file", field)
	}
	inside, err := pathInsideRepo(path)
	if err != nil {
		return err
	}
	if inside {
		return fmt.Errorf("%s must be outside the repository", field)
	}
	return nil
}

func readBoundedFile(path string, maxBytes int64, label string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", label, err)
	}
	if info.Size() > maxBytes {
		return nil, fmt.Errorf("%s exceeds %d bytes", label, maxBytes)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", label, err)
	}
	return data, nil
}

func pathInsideRepo(path string) (bool, error) {
	root, err := findRepoRoot()
	if err != nil {
		return false, err
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false, err
	}
	rel, err := filepath.Rel(root, absPath)
	if err != nil {
		return false, nil
	}
	return rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".." && !filepath.IsAbs(rel)), nil
}

func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("could not locate repository root")
		}
		dir = parent
	}
}

func sha256Ref(data []byte) string {
	return "sha256:" + sha256Hex(data)
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func isSHA256Ref(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	if !strings.HasPrefix(value, "sha256:") {
		return false
	}
	return isSHA256Hex(strings.TrimPrefix(value, "sha256:"))
}

func isSHA256HexOrRef(value string) bool {
	return isSHA256Ref(value) || isSHA256Hex(value)
}

func isSHA256Hex(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) != 64 {
		return false
	}
	for _, r := range value {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') && (r < 'A' || r > 'F') {
			return false
		}
	}
	return true
}

func normalizeHashRef(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.TrimPrefix(value, "sha256:")
	return value
}

func contains(values []string, expected string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == expected {
			return true
		}
	}
	return false
}

func containsSensitiveManifestText(text string) bool {
	patterns := []string{
		`(?i)bearer\s+[a-z0-9._-]{12,}`,
		`(?i)sk-[a-z0-9_-]{12,}`,
		`(?i)password\s*[:=]`,
		`(?i)api[_-]?key\s*[:=]`,
		`(?i)provider_body\s*[:=]`,
		`(?i)message_body\s*[:=]`,
		`(?i)postgres://`,
		`(?i)mysql://`,
		`(?i)mongodb://`,
		`(?i)-----BEGIN\s+(RSA\s+)?PRIVATE KEY-----`,
	}
	for _, pattern := range patterns {
		if regexp.MustCompile(pattern).MatchString(text) {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func envOr(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}
