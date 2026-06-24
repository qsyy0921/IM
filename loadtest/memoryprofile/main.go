package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	memoryv1 "github.com/qsyy0921/IM/api/proto/nexusim/memory/v1"
	workflowv1 "github.com/qsyy0921/IM/api/proto/nexusim/workflow/v1"
	"github.com/qsyy0921/IM/loadtest/internal/grpctls"
	"google.golang.org/grpc"
)

const defaultResultRoot = `H:\NexusIM\loadtest-results`
const batchManifestSchemaVersion = "nexusim.memory.profile_repair_batch.v1"
const maxBatchManifestBytes = 256 * 1024

const (
	approvalWorkflowType           = "REPAIR_APPROVAL"
	approvalWorkflowStatusApproved = "APPROVED"
	approvalTargetService          = "memory-service"
	approvalTargetOperationBatch   = "RECOMPUTE_PROFILE_AGGREGATE_BATCH"
	approvalRequesterService       = "memory-profile-operator"
	approvalDefaultPolicyRef       = "memory.profile.repair.batch.approval.v1"
	approvalDefaultTimeoutRef      = "memory.profile.repair.batch.timeout.v1"
	approvalDefaultCompensationRef = "memory.profile.repair.batch.compensation.v1"
)

type config struct {
	memoryTarget       string
	memoryTLS          grpctls.Config
	workflowTarget     string
	workflowTLS        grpctls.Config
	tenantID           string
	userID             string
	deviceID           string
	subjectUserID      string
	aggregateType      string
	aggregateKey       string
	minSupportCount    int
	batchFile          string
	requestApproval    bool
	approvalWorkflowID string
	approvalRiskLevel  string
	approvalPolicyRef  string
	approvalReasonRef  string
	approvalEvidence   []string
	approvalRequester  string
	idempotencyKey     string
	requestTimeout     time.Duration
	resultRoot         string
	runName            string
	execute            bool
}

type summary struct {
	SchemaVersion              int             `json:"schema_version"`
	Mode                       string          `json:"mode"`
	Executed                   bool            `json:"executed"`
	Commit                     string          `json:"commit"`
	CommitFull                 string          `json:"commit_full"`
	GitDirty                   bool            `json:"git_dirty"`
	GitStatusShort             string          `json:"git_status_short,omitempty"`
	ResultDir                  string          `json:"result_dir"`
	MemoryTarget               string          `json:"memory_target"`
	MemoryTLSEnabled           bool            `json:"memory_tls_enabled"`
	WorkflowTarget             string          `json:"workflow_target,omitempty"`
	WorkflowTLSEnabled         bool            `json:"workflow_tls_enabled,omitempty"`
	TenantID                   string          `json:"tenant_id"`
	UserID                     string          `json:"user_id"`
	DeviceID                   string          `json:"device_id"`
	SubjectUserID              string          `json:"subject_user_id"`
	AggregateType              string          `json:"aggregate_type"`
	AggregateKeyHash           string          `json:"aggregate_key_sha256"`
	MinSupportCount            int             `json:"min_support_count"`
	BatchMode                  bool            `json:"batch_mode"`
	BatchFile                  string          `json:"batch_file,omitempty"`
	BatchTargetCount           int             `json:"batch_target_count,omitempty"`
	BatchPayloadRefHash        string          `json:"batch_payload_ref_hash,omitempty"`
	BatchTargetRefHash         string          `json:"batch_target_ref_hash,omitempty"`
	ApprovalRequired           bool            `json:"approval_required"`
	ApprovalRequested          bool            `json:"approval_requested,omitempty"`
	ApprovalVerified           bool            `json:"approval_verified,omitempty"`
	ApprovalWorkflowID         string          `json:"approval_workflow_id,omitempty"`
	ApprovalWorkflowType       string          `json:"approval_workflow_type,omitempty"`
	ApprovalWorkflowStatus     string          `json:"approval_workflow_status,omitempty"`
	ApprovalWorkflowPayloadRef string          `json:"approval_workflow_payload_ref_hash,omitempty"`
	ApprovalWorkflowTargetRef  string          `json:"approval_workflow_target_ref_hash,omitempty"`
	Targets                    []targetSummary `json:"targets,omitempty"`
	StartedAt                  time.Time       `json:"started_at"`
	FinishedAt                 time.Time       `json:"finished_at"`
	Success                    bool            `json:"success"`
	Error                      string          `json:"error,omitempty"`
	Active                     bool            `json:"active,omitempty"`
	SupportCount               int32           `json:"support_count,omitempty"`
	ProfileID                  string          `json:"profile_id,omitempty"`
	ProfileStatus              string          `json:"profile_status,omitempty"`
	ProfileReviewState         string          `json:"profile_review_state,omitempty"`
	SupportingMemoryCount      int             `json:"supporting_memory_count,omitempty"`
	SupportingMemoryIDHashes   []string        `json:"supporting_memory_id_hashes,omitempty"`
	SummaryTextSHA256          string          `json:"summary_text_sha256,omitempty"`
	SummaryTextLength          int             `json:"summary_text_length,omitempty"`
	ProfileUpdatedAtUnixMillis int64           `json:"profile_updated_at_unix_ms,omitempty"`
}

type batchManifest struct {
	SchemaVersion string         `json:"schema_version"`
	Targets       []repairTarget `json:"targets"`
}

type repairTarget struct {
	SubjectUserID   string `json:"subject_user_id"`
	AggregateType   string `json:"aggregate_type"`
	AggregateKey    string `json:"aggregate_key"`
	MinSupportCount int    `json:"min_support_count,omitempty"`
}

type targetSummary struct {
	SubjectUserID              string   `json:"subject_user_id"`
	AggregateType              string   `json:"aggregate_type"`
	AggregateKeyHash           string   `json:"aggregate_key_sha256"`
	MinSupportCount            int      `json:"min_support_count"`
	Success                    bool     `json:"success"`
	Error                      string   `json:"error,omitempty"`
	Active                     bool     `json:"active,omitempty"`
	SupportCount               int32    `json:"support_count,omitempty"`
	ProfileID                  string   `json:"profile_id,omitempty"`
	ProfileStatus              string   `json:"profile_status,omitempty"`
	ProfileReviewState         string   `json:"profile_review_state,omitempty"`
	SupportingMemoryCount      int      `json:"supporting_memory_count,omitempty"`
	SupportingMemoryIDHashes   []string `json:"supporting_memory_id_hashes,omitempty"`
	SummaryTextSHA256          string   `json:"summary_text_sha256,omitempty"`
	SummaryTextLength          int      `json:"summary_text_length,omitempty"`
	ProfileUpdatedAtUnixMillis int64    `json:"profile_updated_at_unix_ms,omitempty"`
}

type repairPlan struct {
	Targets         []repairTarget
	TargetSummaries []targetSummary
	PayloadRefHash  string
	TargetRefHash   string
}

type approvalCheck struct {
	WorkflowID        string
	WorkflowType      string
	Status            string
	PayloadRefHash    string
	TargetRefHash     string
	ApprovalVerified  bool
	ApprovalRequested bool
}

func main() {
	cfg, err := parseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if err := run(cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func parseArgs(args []string) (config, error) {
	var cfg config
	flags := flag.NewFlagSet("memory-profile-operator", flag.ContinueOnError)
	flags.StringVar(&cfg.memoryTarget, "memory-target", envOrDefault("NEXUSIM_MEMORY_GRPC_ADDR", "127.0.0.1:10580"), "memory-service gRPC target")
	registerTLSFlags(flags, "memory-tls", "NEXUSIM_MEMORY_TLS", "memory-service", &cfg.memoryTLS)
	flags.StringVar(&cfg.workflowTarget, "workflow-target", envOrDefault("NEXUSIM_WORKFLOW_GRPC_ADDR", "127.0.0.1:10750"), "workflow-service gRPC target for batch approval")
	registerTLSFlags(flags, "workflow-tls", "NEXUSIM_WORKFLOW_TLS", "workflow-service", &cfg.workflowTLS)
	flags.StringVar(&cfg.tenantID, "tenant-id", envOrDefault("NEXUSIM_TENANT_ID", "nexusim-local"), "tenant id")
	flags.StringVar(&cfg.userID, "user-id", "memory-profile-operator", "auth user id")
	flags.StringVar(&cfg.deviceID, "device-id", "memory-profile-operator-device", "auth device id")
	flags.StringVar(&cfg.subjectUserID, "subject-user-id", "", "profile subject user id; defaults to --user-id")
	flags.StringVar(&cfg.aggregateType, "aggregate-type", "SKILL", "profile aggregate type")
	flags.StringVar(&cfg.aggregateKey, "aggregate-key", "", "profile aggregate key")
	flags.IntVar(&cfg.minSupportCount, "min-support-count", 2, "minimum visible supporting PROFILE_SIGNAL memories")
	flags.StringVar(&cfg.batchFile, "batch-file", "", "optional profile repair batch manifest JSON")
	flags.BoolVar(&cfg.requestApproval, "request-approval", false, "create a REPAIR_APPROVAL workflow for the batch plan; does not execute repair")
	flags.StringVar(&cfg.approvalWorkflowID, "approval-workflow-id", "", "approved REPAIR_APPROVAL workflow id required for batch execute")
	flags.StringVar(&cfg.approvalRiskLevel, "approval-risk-level", "HIGH", "workflow risk level for request-approval")
	flags.StringVar(&cfg.approvalPolicyRef, "approval-policy-ref", approvalDefaultPolicyRef, "low-sensitive approval policy ref")
	flags.StringVar(&cfg.approvalReasonRef, "approval-reason-ref", "reason:memory-profile-repair-batch", "low-sensitive approval reason ref")
	var approvalEvidence string
	flags.StringVar(&approvalEvidence, "approval-evidence-refs", "evidence:memory-profile-repair-batch", "comma-separated low-sensitive approval evidence refs")
	flags.StringVar(&cfg.approvalRequester, "approval-requester-ref", "operator:memory-profile", "low-sensitive requester ref")
	flags.StringVar(&cfg.idempotencyKey, "idempotency-key", "", "optional idempotency key for request-approval")
	flags.DurationVar(&cfg.requestTimeout, "request-timeout", 5*time.Second, "request timeout")
	flags.StringVar(&cfg.resultRoot, "result-root", defaultResultRoot, "external result root for low-sensitive output")
	flags.StringVar(&cfg.runName, "run-name", "", "run name under result-root")
	flags.BoolVar(&cfg.execute, "execute", false, "execute recompute through memory-service; default is plan-only")
	if err := flags.Parse(args); err != nil {
		return config{}, err
	}
	if strings.TrimSpace(cfg.subjectUserID) == "" {
		cfg.subjectUserID = cfg.userID
	}
	if strings.TrimSpace(cfg.runName) == "" {
		cfg.runName = "memory-profile-operator-" + time.Now().UTC().Format("20060102-150405")
	}
	cfg.approvalWorkflowID = strings.TrimSpace(cfg.approvalWorkflowID)
	cfg.batchFile = strings.TrimSpace(cfg.batchFile)
	cfg.approvalRiskLevel = strings.ToUpper(strings.TrimSpace(cfg.approvalRiskLevel))
	cfg.approvalPolicyRef = strings.TrimSpace(cfg.approvalPolicyRef)
	cfg.approvalReasonRef = strings.TrimSpace(cfg.approvalReasonRef)
	cfg.approvalRequester = strings.TrimSpace(cfg.approvalRequester)
	cfg.idempotencyKey = strings.TrimSpace(cfg.idempotencyKey)
	cfg.approvalEvidence = splitCSV(approvalEvidence)
	return cfg, validateConfig(cfg)
}

func registerTLSFlags(flags *flag.FlagSet, prefix string, envPrefix string, serviceName string, config *grpctls.Config) {
	flags.StringVar(&config.CAFile, prefix+"-ca-file", os.Getenv(envPrefix+"_CA_FILE"), "CA PEM for "+serviceName+" gRPC TLS")
	flags.StringVar(&config.ServerName, prefix+"-server-name", os.Getenv(envPrefix+"_SERVER_NAME"), "override server name for "+serviceName+" gRPC TLS")
	flags.StringVar(&config.ClientCertFile, prefix+"-client-cert-file", os.Getenv(envPrefix+"_CLIENT_CERT_FILE"), "client certificate PEM for "+serviceName+" mTLS")
	flags.StringVar(&config.ClientKeyFile, prefix+"-client-key-file", os.Getenv(envPrefix+"_CLIENT_KEY_FILE"), "client private key PEM for "+serviceName+" mTLS")
}

func validateConfig(cfg config) error {
	if strings.TrimSpace(cfg.memoryTarget) == "" {
		return errors.New("--memory-target is required")
	}
	if strings.TrimSpace(cfg.workflowTarget) == "" && (cfg.requestApproval || cfg.approvalWorkflowID != "") {
		return errors.New("--workflow-target is required when approval is requested or verified")
	}
	if strings.TrimSpace(cfg.tenantID) == "" {
		return errors.New("--tenant-id is required")
	}
	if strings.TrimSpace(cfg.userID) == "" {
		return errors.New("--user-id is required")
	}
	if strings.TrimSpace(cfg.deviceID) == "" {
		return errors.New("--device-id is required")
	}
	if strings.TrimSpace(cfg.subjectUserID) == "" {
		return errors.New("--subject-user-id is required")
	}
	if cfg.subjectUserID != cfg.userID {
		return errors.New("--subject-user-id must match --user-id until a policy-controlled operator path exists")
	}
	if cfg.batchFile == "" {
		if strings.TrimSpace(cfg.aggregateType) == "" {
			return errors.New("--aggregate-type is required")
		}
		if strings.TrimSpace(cfg.aggregateKey) == "" {
			return errors.New("--aggregate-key is required")
		}
	}
	if cfg.minSupportCount <= 0 || cfg.minSupportCount > 20 {
		return errors.New("--min-support-count must be between 1 and 20")
	}
	if cfg.requestApproval && cfg.execute {
		return errors.New("--request-approval cannot be combined with --execute")
	}
	if cfg.requestApproval && cfg.batchFile == "" {
		return errors.New("--request-approval requires --batch-file")
	}
	if cfg.requestApproval && cfg.approvalWorkflowID != "" {
		return errors.New("--request-approval creates a new workflow and cannot use --approval-workflow-id")
	}
	if cfg.execute && cfg.batchFile != "" && cfg.approvalWorkflowID == "" {
		return errors.New("--execute with --batch-file requires --approval-workflow-id")
	}
	if cfg.requestApproval || cfg.approvalWorkflowID != "" {
		if err := validateLowSensitiveRef("--approval-policy-ref", cfg.approvalPolicyRef); err != nil {
			return err
		}
		if err := validateLowSensitiveRef("--approval-reason-ref", cfg.approvalReasonRef); err != nil {
			return err
		}
		if err := validateLowSensitiveRef("--approval-requester-ref", cfg.approvalRequester); err != nil {
			return err
		}
		if err := validateLowSensitiveRefs("--approval-evidence-refs", cfg.approvalEvidence); err != nil {
			return err
		}
	}
	if cfg.requestTimeout <= 0 {
		return errors.New("--request-timeout must be positive")
	}
	if strings.TrimSpace(cfg.resultRoot) == "" {
		return errors.New("--result-root is required")
	}
	return nil
}

func run(cfg config) error {
	resultDir := filepath.Join(cfg.resultRoot, cfg.runName)
	if err := validateExternalResultDir(resultDir); err != nil {
		return err
	}
	if err := os.MkdirAll(resultDir, 0o755); err != nil {
		return fmt.Errorf("create result dir: %w", err)
	}

	result := newSummary(cfg, resultDir)
	defer func() {
		result.FinishedAt = time.Now().UTC()
		_ = writeSummary(resultDir, result)
	}()

	plan, err := buildRepairPlan(cfg)
	if err != nil {
		result.Error = "build repair plan: " + err.Error()
		return err
	}
	applyPlan(&result, cfg, plan)

	if cfg.requestApproval {
		dialOption, err := grpctls.DialOption(cfg.workflowTLS, "workflow-tls")
		if err != nil {
			result.Error = "configure workflow TLS: " + err.Error()
			return err
		}
		conn, err := grpc.NewClient("passthrough:///"+cfg.workflowTarget, dialOption)
		if err != nil {
			result.Error = "dial workflow-service: " + err.Error()
			return fmt.Errorf("dial workflow-service: %w", err)
		}
		defer conn.Close()
		requestCtx, cancel := context.WithTimeout(context.Background(), cfg.requestTimeout)
		defer cancel()
		check, err := requestRepairApproval(requestCtx, cfg, plan, workflowv1.NewWorkflowServiceClient(conn))
		applyApproval(&result, check)
		if err != nil {
			result.Error = "request repair approval: " + err.Error()
			return fmt.Errorf("request repair approval: %w", err)
		}
		result.Success = true
		return nil
	}

	if !cfg.execute {
		result.Success = true
		return nil
	}

	var workflowClient workflowv1.WorkflowServiceClient
	var workflowConn *grpc.ClientConn
	if cfg.approvalWorkflowID != "" {
		dialOption, err := grpctls.DialOption(cfg.workflowTLS, "workflow-tls")
		if err != nil {
			result.Error = "configure workflow TLS: " + err.Error()
			return err
		}
		workflowConn, err = grpc.NewClient("passthrough:///"+cfg.workflowTarget, dialOption)
		if err != nil {
			result.Error = "dial workflow-service: " + err.Error()
			return fmt.Errorf("dial workflow-service: %w", err)
		}
		defer workflowConn.Close()
		workflowClient = workflowv1.NewWorkflowServiceClient(workflowConn)
		requestCtx, cancel := context.WithTimeout(context.Background(), cfg.requestTimeout)
		check, err := verifyRepairApproval(requestCtx, cfg, plan, workflowClient)
		cancel()
		applyApproval(&result, check)
		if err != nil {
			result.Error = "verify repair approval: " + err.Error()
			return fmt.Errorf("verify repair approval: %w", err)
		}
	}

	dialOption, err := grpctls.DialOption(cfg.memoryTLS, "memory-tls")
	if err != nil {
		result.Error = "configure memory TLS: " + err.Error()
		return err
	}
	conn, err := grpc.NewClient("passthrough:///"+cfg.memoryTarget, dialOption)
	if err != nil {
		result.Error = "dial memory-service: " + err.Error()
		return fmt.Errorf("dial memory-service: %w", err)
	}
	defer conn.Close()

	requestCtx, cancel := context.WithTimeout(context.Background(), cfg.requestTimeout)
	defer cancel()
	if err := executeTargets(requestCtx, cfg, plan, memoryv1.NewMemoryServiceClient(conn), &result); err != nil {
		return err
	}
	result.Success = true
	return nil
}

func execute(ctx context.Context, cfg config, client memoryv1.MemoryServiceClient) (summary, error) {
	return executeWithClients(ctx, cfg, client, nil)
}

func executeWithClients(
	ctx context.Context,
	cfg config,
	memoryClient memoryv1.MemoryServiceClient,
	workflowClient workflowv1.WorkflowServiceClient,
) (summary, error) {
	result := newSummary(cfg, "")
	if cfg.execute && cfg.batchFile != "" && cfg.approvalWorkflowID == "" {
		err := errors.New("--execute with --batch-file requires --approval-workflow-id")
		result.Error = err.Error()
		return result, err
	}
	plan, err := buildRepairPlan(cfg)
	if err != nil {
		result.Error = "build repair plan: " + err.Error()
		return result, err
	}
	applyPlan(&result, cfg, plan)
	if cfg.requestApproval {
		check, err := requestRepairApproval(ctx, cfg, plan, workflowClient)
		applyApproval(&result, check)
		if err != nil {
			result.Error = "request repair approval: " + err.Error()
			return result, err
		}
		result.Success = true
		return result, nil
	}
	if !cfg.execute {
		result.Success = true
		return result, nil
	}
	if cfg.approvalWorkflowID != "" {
		check, err := verifyRepairApproval(ctx, cfg, plan, workflowClient)
		applyApproval(&result, check)
		if err != nil {
			result.Error = "verify repair approval: " + err.Error()
			return result, err
		}
	}
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	if err := executeTargets(requestCtx, cfg, plan, memoryClient, &result); err != nil {
		return result, err
	}
	result.Success = true
	return result, nil
}

func executeTargets(
	ctx context.Context,
	cfg config,
	plan repairPlan,
	client memoryv1.MemoryServiceClient,
	result *summary,
) error {
	for index, target := range plan.Targets {
		response, err := client.RecomputeProfileAggregate(ctx, recomputeRequest(cfg, target))
		if err != nil {
			targetResult := targetSummaryFromTarget(target)
			targetResult.Error = "recompute profile aggregate: " + err.Error()
			result.Targets[index] = targetResult
			result.Error = targetResult.Error
			return fmt.Errorf("recompute profile aggregate target %d: %w", index, err)
		}
		targetResult := targetSummaryFromTarget(target)
		applyTargetResponse(&targetResult, response)
		targetResult.Success = true
		result.Targets[index] = targetResult
		if !result.BatchMode {
			applyResponse(result, response)
		}
	}
	return nil
}

func recomputeRequest(cfg config, target repairTarget) *memoryv1.RecomputeProfileAggregateRequest {
	return &memoryv1.RecomputeProfileAggregateRequest{
		AuthContext: &memoryv1.AuthContext{
			TenantId:  cfg.tenantID,
			UserId:    cfg.userID,
			DeviceId:  cfg.deviceID,
			SessionId: "memory-profile-operator",
			TraceId:   "memory-profile-operator",
			RequestId: "memory-profile-operator",
		},
		SubjectUserId:   target.SubjectUserID,
		AggregateType:   target.AggregateType,
		AggregateKey:    target.AggregateKey,
		MinSupportCount: int32(target.MinSupportCount),
	}
}

func newSummary(cfg config, resultDir string) summary {
	gitStatus := gitOutput("status", "--short")
	return summary{
		SchemaVersion:      1,
		Mode:               "recompute-profile",
		Executed:           cfg.execute,
		Commit:             gitOutput("rev-parse", "--short", "HEAD"),
		CommitFull:         gitOutput("rev-parse", "HEAD"),
		GitDirty:           strings.TrimSpace(gitStatus) != "",
		GitStatusShort:     gitStatus,
		ResultDir:          resultDir,
		MemoryTarget:       cfg.memoryTarget,
		MemoryTLSEnabled:   cfg.memoryTLS.Enabled(),
		WorkflowTarget:     cfg.workflowTarget,
		WorkflowTLSEnabled: cfg.workflowTLS.Enabled(),
		TenantID:           cfg.tenantID,
		UserID:             cfg.userID,
		DeviceID:           cfg.deviceID,
		SubjectUserID:      cfg.subjectUserID,
		AggregateType:      cfg.aggregateType,
		AggregateKeyHash:   sha256Hex(cfg.aggregateKey),
		MinSupportCount:    cfg.minSupportCount,
		StartedAt:          time.Now().UTC(),
	}
}

func buildRepairPlan(cfg config) (repairPlan, error) {
	targets, err := loadRepairTargets(cfg)
	if err != nil {
		return repairPlan{}, err
	}
	normalized := make([]targetSummary, 0, len(targets))
	for _, target := range targets {
		normalized = append(normalized, targetSummaryFromTarget(target))
	}
	payloadHash, targetHash, err := hashRepairPlan(cfg, normalized)
	if err != nil {
		return repairPlan{}, err
	}
	return repairPlan{
		Targets:         targets,
		TargetSummaries: normalized,
		PayloadRefHash:  "sha256:" + payloadHash,
		TargetRefHash:   "sha256:" + targetHash,
	}, nil
}

func loadRepairTargets(cfg config) ([]repairTarget, error) {
	if cfg.batchFile == "" {
		target := repairTarget{
			SubjectUserID:   strings.TrimSpace(cfg.subjectUserID),
			AggregateType:   strings.TrimSpace(cfg.aggregateType),
			AggregateKey:    strings.TrimSpace(cfg.aggregateKey),
			MinSupportCount: cfg.minSupportCount,
		}
		if err := validateRepairTarget(cfg, target); err != nil {
			return nil, err
		}
		return []repairTarget{target}, nil
	}
	info, err := os.Stat(cfg.batchFile)
	if err != nil {
		return nil, fmt.Errorf("read batch file: %w", err)
	}
	if info.Size() > maxBatchManifestBytes {
		return nil, errors.New("batch file is too large")
	}
	raw, err := os.ReadFile(cfg.batchFile)
	if err != nil {
		return nil, fmt.Errorf("read batch file: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var manifest batchManifest
	if err := decoder.Decode(&manifest); err != nil {
		return nil, fmt.Errorf("decode batch file: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, errors.New("batch file must contain a single JSON object")
	}
	if strings.TrimSpace(manifest.SchemaVersion) != batchManifestSchemaVersion {
		return nil, fmt.Errorf("unsupported batch schema_version %q", manifest.SchemaVersion)
	}
	if len(manifest.Targets) == 0 {
		return nil, errors.New("batch targets are required")
	}
	if len(manifest.Targets) > 100 {
		return nil, errors.New("batch targets must not exceed 100")
	}
	targets := make([]repairTarget, 0, len(manifest.Targets))
	seen := map[string]bool{}
	for index, target := range manifest.Targets {
		target = normalizeRepairTarget(cfg, target)
		if err := validateRepairTarget(cfg, target); err != nil {
			return nil, fmt.Errorf("batch target %d: %w", index, err)
		}
		key := target.SubjectUserID + "\x00" + target.AggregateType + "\x00" + target.AggregateKey
		if seen[key] {
			return nil, fmt.Errorf("batch target %d duplicates an earlier target", index)
		}
		seen[key] = true
		targets = append(targets, target)
	}
	return targets, nil
}

func normalizeRepairTarget(cfg config, target repairTarget) repairTarget {
	target.SubjectUserID = strings.TrimSpace(target.SubjectUserID)
	target.AggregateType = strings.TrimSpace(target.AggregateType)
	target.AggregateKey = strings.TrimSpace(target.AggregateKey)
	if target.MinSupportCount == 0 {
		target.MinSupportCount = cfg.minSupportCount
	}
	return target
}

func validateRepairTarget(cfg config, target repairTarget) error {
	if target.SubjectUserID == "" {
		return errors.New("subject_user_id is required")
	}
	if target.SubjectUserID != cfg.userID {
		return errors.New("subject_user_id must match --user-id until a policy-controlled operator path exists")
	}
	if target.AggregateType == "" {
		return errors.New("aggregate_type is required")
	}
	if target.AggregateKey == "" {
		return errors.New("aggregate_key is required")
	}
	if target.MinSupportCount <= 0 || target.MinSupportCount > 20 {
		return errors.New("min_support_count must be between 1 and 20")
	}
	return nil
}

func hashRepairPlan(cfg config, targets []targetSummary) (string, string, error) {
	orderedTargets := append([]targetSummary(nil), targets...)
	sort.Slice(orderedTargets, func(i, j int) bool {
		left := orderedTargets[i]
		right := orderedTargets[j]
		if left.SubjectUserID != right.SubjectUserID {
			return left.SubjectUserID < right.SubjectUserID
		}
		if left.AggregateType != right.AggregateType {
			return left.AggregateType < right.AggregateType
		}
		return left.AggregateKeyHash < right.AggregateKeyHash
	})
	payload := struct {
		SchemaVersion string          `json:"schema_version"`
		Targets       []targetSummary `json:"targets"`
	}{
		SchemaVersion: batchManifestSchemaVersion,
		Targets:       orderedTargets,
	}
	payloadHash, err := jsonSHA256(payload)
	if err != nil {
		return "", "", err
	}
	target := struct {
		TenantID string          `json:"tenant_id"`
		UserID   string          `json:"user_id"`
		Targets  []targetSummary `json:"targets"`
	}{
		TenantID: cfg.tenantID,
		UserID:   cfg.userID,
		Targets:  orderedTargets,
	}
	targetHash, err := jsonSHA256(target)
	if err != nil {
		return "", "", err
	}
	return payloadHash, targetHash, nil
}

func jsonSHA256(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return sha256Hex(string(encoded)), nil
}

func applyPlan(result *summary, cfg config, plan repairPlan) {
	result.BatchMode = cfg.batchFile != ""
	result.BatchFile = cfg.batchFile
	result.BatchTargetCount = len(plan.Targets)
	result.BatchPayloadRefHash = plan.PayloadRefHash
	result.BatchTargetRefHash = plan.TargetRefHash
	result.ApprovalRequired = cfg.batchFile != "" && cfg.execute
	result.ApprovalWorkflowID = cfg.approvalWorkflowID
	result.Targets = append([]targetSummary(nil), plan.TargetSummaries...)
}

func requestRepairApproval(
	ctx context.Context,
	cfg config,
	plan repairPlan,
	client workflowv1.WorkflowServiceClient,
) (approvalCheck, error) {
	if client == nil {
		return approvalCheck{}, errors.New("workflow client is required")
	}
	idempotencyKey := cfg.idempotencyKey
	if idempotencyKey == "" {
		idempotencyKey = "memory-profile-repair-batch:" + strings.TrimPrefix(plan.PayloadRefHash, "sha256:")
	}
	response, err := client.CreateWorkflow(ctx, &workflowv1.CreateWorkflowRequest{
		AuthContext:           workflowAuthContext(cfg),
		RequesterRef:          cfg.approvalRequester,
		RequesterService:      approvalRequesterService,
		WorkflowType:          approvalWorkflowType,
		RiskLevel:             cfg.approvalRiskLevel,
		TargetRefHash:         plan.TargetRefHash,
		TargetService:         approvalTargetService,
		TargetOperation:       approvalTargetOperationBatch,
		ApprovalPolicyRef:     cfg.approvalPolicyRef,
		TimeoutPolicyRef:      approvalDefaultTimeoutRef,
		CompensationPolicyRef: approvalDefaultCompensationRef,
		PayloadSchemaVersion:  batchManifestSchemaVersion,
		PayloadRefHash:        plan.PayloadRefHash,
		ReasonRef:             cfg.approvalReasonRef,
		EvidenceRefs:          append([]string(nil), cfg.approvalEvidence...),
		IdempotencyKey:        idempotencyKey,
		CorrelationId:         "memory-profile-repair-batch",
		CausationId:           "memory-profile-repair-batch",
		TraceId:               "memory-profile-repair-batch",
	})
	if err != nil {
		return approvalCheck{}, err
	}
	workflow := response.GetWorkflow()
	if workflow == nil || strings.TrimSpace(workflow.GetWorkflowId()) == "" {
		return approvalCheck{}, errors.New("workflow-service returned empty approval workflow")
	}
	if workflow.GetWorkflowType() != approvalWorkflowType ||
		workflow.GetTargetService() != approvalTargetService ||
		workflow.GetTargetOperation() != approvalTargetOperationBatch ||
		workflow.GetPayloadSchemaVersion() != batchManifestSchemaVersion ||
		workflow.GetPayloadRefHash() != plan.PayloadRefHash ||
		workflow.GetTargetRefHash() != plan.TargetRefHash {
		return approvalCheck{
			WorkflowID:     workflow.GetWorkflowId(),
			WorkflowType:   workflow.GetWorkflowType(),
			Status:         workflow.GetStatus(),
			PayloadRefHash: workflow.GetPayloadRefHash(),
			TargetRefHash:  workflow.GetTargetRefHash(),
		}, errors.New("workflow-service returned mismatched approval workflow")
	}
	return approvalCheck{
		WorkflowID:        workflow.GetWorkflowId(),
		WorkflowType:      workflow.GetWorkflowType(),
		Status:            workflow.GetStatus(),
		PayloadRefHash:    workflow.GetPayloadRefHash(),
		TargetRefHash:     workflow.GetTargetRefHash(),
		ApprovalRequested: true,
	}, nil
}

func verifyRepairApproval(
	ctx context.Context,
	cfg config,
	plan repairPlan,
	client workflowv1.WorkflowServiceClient,
) (approvalCheck, error) {
	if client == nil {
		return approvalCheck{}, errors.New("workflow client is required")
	}
	response, err := client.GetWorkflow(ctx, &workflowv1.GetWorkflowRequest{
		AuthContext: workflowAuthContext(cfg),
		WorkflowId:  cfg.approvalWorkflowID,
	})
	if err != nil {
		return approvalCheck{}, err
	}
	workflow := response.GetWorkflow()
	if workflow == nil {
		return approvalCheck{}, errors.New("approval workflow not found")
	}
	check := approvalCheck{
		WorkflowID:     workflow.GetWorkflowId(),
		WorkflowType:   workflow.GetWorkflowType(),
		Status:         workflow.GetStatus(),
		PayloadRefHash: workflow.GetPayloadRefHash(),
		TargetRefHash:  workflow.GetTargetRefHash(),
	}
	if workflow.GetTenantId() != cfg.tenantID {
		return check, errors.New("approval workflow tenant mismatch")
	}
	if workflow.GetWorkflowType() != approvalWorkflowType {
		return check, errors.New("approval workflow type must be REPAIR_APPROVAL")
	}
	if workflow.GetStatus() != approvalWorkflowStatusApproved {
		return check, errors.New("approval workflow must be APPROVED")
	}
	if workflow.GetTargetService() != approvalTargetService {
		return check, errors.New("approval workflow target service mismatch")
	}
	if workflow.GetTargetOperation() != approvalTargetOperationBatch {
		return check, errors.New("approval workflow target operation mismatch")
	}
	if workflow.GetPayloadSchemaVersion() != batchManifestSchemaVersion {
		return check, errors.New("approval workflow payload schema mismatch")
	}
	if workflow.GetPayloadRefHash() != plan.PayloadRefHash {
		return check, errors.New("approval workflow payload hash mismatch")
	}
	if workflow.GetTargetRefHash() != plan.TargetRefHash {
		return check, errors.New("approval workflow target hash mismatch")
	}
	check.ApprovalVerified = true
	return check, nil
}

func applyApproval(result *summary, check approvalCheck) {
	result.ApprovalWorkflowID = check.WorkflowID
	result.ApprovalWorkflowType = check.WorkflowType
	result.ApprovalWorkflowStatus = check.Status
	result.ApprovalWorkflowPayloadRef = check.PayloadRefHash
	result.ApprovalWorkflowTargetRef = check.TargetRefHash
	result.ApprovalVerified = check.ApprovalVerified
	result.ApprovalRequested = check.ApprovalRequested
}

func workflowAuthContext(cfg config) *workflowv1.AuthContext {
	return &workflowv1.AuthContext{
		TenantId:    cfg.tenantID,
		UserId:      cfg.userID,
		ServiceName: approvalRequesterService,
		InstanceRef: "memory-profile-operator",
		TraceId:     "memory-profile-operator",
		RequestId:   "memory-profile-operator",
	}
}

func applyResponse(result *summary, response *memoryv1.RecomputeProfileAggregateResponse) {
	result.Active = response.GetActive()
	result.SupportCount = response.GetSupportCount()
	item := response.GetItem()
	if item == nil {
		return
	}
	result.ProfileID = item.GetProfileId()
	result.ProfileStatus = item.GetStatus().String()
	result.ProfileReviewState = item.GetReviewState().String()
	result.SupportingMemoryCount = len(item.GetSupportingMemoryEventIds())
	for _, id := range item.GetSupportingMemoryEventIds() {
		result.SupportingMemoryIDHashes = append(result.SupportingMemoryIDHashes, sha256Hex(id))
	}
	result.SummaryTextSHA256 = sha256Hex(item.GetSummaryText())
	result.SummaryTextLength = len(item.GetSummaryText())
	result.ProfileUpdatedAtUnixMillis = item.GetUpdatedAtUnixMs()
}

func targetSummaryFromTarget(target repairTarget) targetSummary {
	return targetSummary{
		SubjectUserID:    target.SubjectUserID,
		AggregateType:    target.AggregateType,
		AggregateKeyHash: sha256Hex(target.AggregateKey),
		MinSupportCount:  target.MinSupportCount,
	}
}

func applyTargetResponse(result *targetSummary, response *memoryv1.RecomputeProfileAggregateResponse) {
	result.Active = response.GetActive()
	result.SupportCount = response.GetSupportCount()
	item := response.GetItem()
	if item == nil {
		return
	}
	result.ProfileID = item.GetProfileId()
	result.ProfileStatus = item.GetStatus().String()
	result.ProfileReviewState = item.GetReviewState().String()
	result.SupportingMemoryCount = len(item.GetSupportingMemoryEventIds())
	for _, id := range item.GetSupportingMemoryEventIds() {
		result.SupportingMemoryIDHashes = append(result.SupportingMemoryIDHashes, sha256Hex(id))
	}
	result.SummaryTextSHA256 = sha256Hex(item.GetSummaryText())
	result.SummaryTextLength = len(item.GetSummaryText())
	result.ProfileUpdatedAtUnixMillis = item.GetUpdatedAtUnixMs()
}

func validateLowSensitiveRefs(field string, values []string) error {
	for _, value := range values {
		if err := validateLowSensitiveRef(field, value); err != nil {
			return err
		}
	}
	return nil
}

func validateLowSensitiveRef(field string, value string) error {
	if looksSensitiveRef(value) {
		return fmt.Errorf("%s must be a low-sensitive ref or hash", field)
	}
	return nil
}

func looksSensitiveRef(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	for _, marker := range []string{"secret", "token", "api_key", "apikey", "password", "private://", "raw:", "dsn=", "postgres://"} {
		if strings.Contains(value, marker) {
			return true
		}
	}
	return false
}

func validateExternalResultDir(path string) error {
	clean := filepath.Clean(path)
	repoRoot := gitOutput("rev-parse", "--show-toplevel")
	if repoRoot != "" && strings.HasPrefix(strings.ToLower(clean), strings.ToLower(filepath.Clean(repoRoot))) {
		return fmt.Errorf("result dir must be outside repository: %s", clean)
	}
	return nil
}

func writeSummary(resultDir string, result summary) error {
	path := filepath.Join(resultDir, "memory-profile-operator-summary.json")
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, encoded, 0o644)
}

func gitOutput(args ...string) string {
	cmd := exec.Command("git", args...)
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func sha256Hex(value string) string {
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:])
}

func envOrDefault(name string, defaultValue string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return defaultValue
	}
	return value
}

func splitCSV(value string) []string {
	seen := map[string]bool{}
	var result []string
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(item)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		result = append(result, item)
	}
	return result
}
