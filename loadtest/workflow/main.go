package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	workflowv1 "github.com/qsyy0921/IM/api/proto/nexusim/workflow/v1"
	"github.com/qsyy0921/IM/loadtest/internal/grpctls"
	"google.golang.org/grpc"
)

type config struct {
	mode                         string
	target                       string
	requestTimeout               time.Duration
	tls                          grpctls.Config
	tenantID                     string
	userID                       string
	instanceRef                  string
	traceID                      string
	requestID                    string
	correlationID                string
	causationID                  string
	requesterRef                 string
	requesterService             string
	workflowID                   string
	workflowType                 string
	expectedWorkflowType         string
	expectedStatus               string
	stepID                       string
	decisionManifestPath         string
	deciderRef                   string
	decision                     string
	decisionPolicy               string
	reasonRef                    string
	evidenceRefs                 []string
	idempotencyKey               string
	riskLevel                    string
	status                       string
	targetService                string
	expectedTargetService        string
	targetOperation              string
	expectedTargetOperation      string
	expectedTargetRefHash        string
	targetRefHash                string
	expectedPayloadSchemaVersion string
	expectedPayloadRefHash       string
	payloadSchemaVersion         string
	payloadRefHash               string
	approvalPolicyRef            string
	expectedApprovalPolicyRef    string
	timeoutPolicyRef             string
	compensationPolicyRef        string
	pageSize                     int32
}

type commandResult struct {
	Mode               string                       `json:"mode"`
	Target             string                       `json:"target"`
	TenantID           string                       `json:"tenant_id"`
	WorkflowID         string                       `json:"workflow_id"`
	WorkflowType       string                       `json:"workflow_type,omitempty"`
	Status             string                       `json:"status,omitempty"`
	TargetService      string                       `json:"target_service,omitempty"`
	TargetOperation    string                       `json:"target_operation,omitempty"`
	ApprovalPolicyRef  string                       `json:"approval_policy_ref,omitempty"`
	Workflow           *workflowRef                 `json:"workflow,omitempty"`
	Workflows          []workflowRef                `json:"workflows,omitempty"`
	OperatorQueues     []operatorQueueRef           `json:"operator_queues,omitempty"`
	DecisionManifest   *decisionManifestTemplate    `json:"decision_manifest_template,omitempty"`
	Decision           *decisionRef                 `json:"decision,omitempty"`
	Decisions          []decisionRef                `json:"decisions,omitempty"`
	Instructions       []compensationInstructionRef `json:"instructions,omitempty"`
	CompensationReview *compensationReviewBundle    `json:"compensation_review,omitempty"`
	Replayed           bool                         `json:"replayed,omitempty"`
	CheckedAt          time.Time                    `json:"checked_at"`
}

type operatorQueueRef struct {
	QueueID           string        `json:"queue_id"`
	WorkflowType      string        `json:"workflow_type"`
	Status            string        `json:"status"`
	TargetService     string        `json:"target_service,omitempty"`
	TargetOperation   string        `json:"target_operation,omitempty"`
	ApprovalPolicyRef string        `json:"approval_policy_ref,omitempty"`
	WorkflowCount     int           `json:"workflow_count"`
	Workflows         []workflowRef `json:"workflows,omitempty"`
}

type workflowRef struct {
	WorkflowID            string   `json:"workflow_id"`
	WorkflowType          string   `json:"workflow_type"`
	RiskLevel             string   `json:"risk_level"`
	RequesterRef          string   `json:"requester_ref,omitempty"`
	RequesterService      string   `json:"requester_service,omitempty"`
	TargetService         string   `json:"target_service,omitempty"`
	TargetOperation       string   `json:"target_operation,omitempty"`
	TargetRefHash         string   `json:"target_ref_hash,omitempty"`
	PayloadSchemaVersion  string   `json:"payload_schema_version,omitempty"`
	PayloadRefHash        string   `json:"payload_ref_hash,omitempty"`
	ApprovalPolicyRef     string   `json:"approval_policy_ref,omitempty"`
	TimeoutPolicyRef      string   `json:"timeout_policy_ref,omitempty"`
	CompensationPolicyRef string   `json:"compensation_policy_ref,omitempty"`
	ReasonRef             string   `json:"reason_ref,omitempty"`
	EvidenceRefs          []string `json:"evidence_refs,omitempty"`
	Status                string   `json:"status"`
	CurrentStepID         string   `json:"current_step_id,omitempty"`
	CorrelationID         string   `json:"correlation_id,omitempty"`
	CausationID           string   `json:"causation_id,omitempty"`
	TraceID               string   `json:"trace_id,omitempty"`
	CreatedAtUnixMs       int64    `json:"created_at_unix_ms,omitempty"`
	UpdatedAtUnixMs       int64    `json:"updated_at_unix_ms,omitempty"`
	CompletedAtUnixMs     int64    `json:"completed_at_unix_ms,omitempty"`
}

type decisionRef struct {
	DecisionID        string   `json:"decision_id"`
	WorkflowID        string   `json:"workflow_id"`
	StepID            string   `json:"step_id"`
	DeciderRef        string   `json:"decider_ref"`
	DecisionType      string   `json:"decision_type"`
	DecisionPolicyRef string   `json:"decision_policy_ref,omitempty"`
	ReasonRef         string   `json:"reason_ref,omitempty"`
	EvidenceRefs      []string `json:"evidence_refs,omitempty"`
	CreatedAtUnixMs   int64    `json:"created_at_unix_ms,omitempty"`
}

type compensationInstructionRef struct {
	InstructionID   string `json:"instruction_id"`
	WorkflowID      string `json:"workflow_id"`
	PayloadRefHash  string `json:"payload_ref_hash"`
	TargetService   string `json:"target_service"`
	TargetOperation string `json:"target_operation"`
	InstructionType string `json:"instruction_type"`
	Environment     string `json:"environment,omitempty"`
	ConfigKind      string `json:"config_kind,omitempty"`
	BundleKey       string `json:"bundle_key,omitempty"`
	TargetVersion   string `json:"target_version,omitempty"`
	OperatorRef     string `json:"operator_ref,omitempty"`
	ReasonRef       string `json:"reason_ref,omitempty"`
	Status          string `json:"status"`
	CreatedAtUnixMs int64  `json:"created_at_unix_ms,omitempty"`
	UpdatedAtUnixMs int64  `json:"updated_at_unix_ms,omitempty"`
}

type compensationReviewBundle struct {
	SchemaVersion      string                       `json:"schema_version"`
	Workflow           workflowRef                  `json:"workflow"`
	InstructionStatus  string                       `json:"instruction_status"`
	InstructionCount   int                          `json:"instruction_count"`
	Instructions       []compensationInstructionRef `json:"instructions"`
	ReviewChecks       []string                     `json:"review_checks"`
	ApprovalBoundary   []string                     `json:"approval_boundary"`
	ExecutionBoundary  []string                     `json:"execution_boundary"`
	NoDirectExecution  bool                         `json:"no_direct_execution"`
	NoDecisionRecorded bool                         `json:"no_decision_recorded"`
}

type decisionManifest struct {
	SchemaVersion                string   `json:"schema_version"`
	WorkflowID                   string   `json:"workflow_id"`
	StepID                       string   `json:"step_id"`
	ExpectedWorkflowType         string   `json:"expected_workflow_type"`
	ExpectedStatus               string   `json:"expected_status"`
	ExpectedTargetService        string   `json:"expected_target_service"`
	ExpectedTargetOperation      string   `json:"expected_target_operation"`
	ExpectedTargetRefHash        string   `json:"expected_target_ref_hash"`
	ExpectedPayloadSchemaVersion string   `json:"expected_payload_schema_version"`
	ExpectedPayloadRefHash       string   `json:"expected_payload_ref_hash"`
	ExpectedApprovalPolicyRef    string   `json:"expected_approval_policy_ref"`
	Decision                     string   `json:"decision"`
	DeciderRef                   string   `json:"decider_ref"`
	DecisionPolicyRef            string   `json:"decision_policy_ref"`
	ReasonRef                    string   `json:"reason_ref"`
	EvidenceRefs                 []string `json:"evidence_refs"`
	IdempotencyKey               string   `json:"idempotency_key"`
	CorrelationID                string   `json:"correlation_id"`
	CausationID                  string   `json:"causation_id"`
	TraceID                      string   `json:"trace_id"`
}

type decisionManifestTemplate struct {
	SchemaVersion                string   `json:"schema_version"`
	WorkflowID                   string   `json:"workflow_id"`
	StepID                       string   `json:"step_id"`
	ExpectedWorkflowType         string   `json:"expected_workflow_type"`
	ExpectedStatus               string   `json:"expected_status"`
	ExpectedTargetService        string   `json:"expected_target_service"`
	ExpectedTargetOperation      string   `json:"expected_target_operation"`
	ExpectedTargetRefHash        string   `json:"expected_target_ref_hash"`
	ExpectedPayloadSchemaVersion string   `json:"expected_payload_schema_version"`
	ExpectedPayloadRefHash       string   `json:"expected_payload_ref_hash"`
	ExpectedApprovalPolicyRef    string   `json:"expected_approval_policy_ref"`
	Decision                     string   `json:"decision,omitempty"`
	DeciderRef                   string   `json:"decider_ref,omitempty"`
	DecisionPolicyRef            string   `json:"decision_policy_ref,omitempty"`
	ReasonRef                    string   `json:"reason_ref,omitempty"`
	EvidenceRefs                 []string `json:"evidence_refs,omitempty"`
	IdempotencyKey               string   `json:"idempotency_key,omitempty"`
	CorrelationID                string   `json:"correlation_id,omitempty"`
	CausationID                  string   `json:"causation_id,omitempty"`
	TraceID                      string   `json:"trace_id,omitempty"`
}

const (
	decisionManifestSchemaVersion         = "nexusim.workflow.external_decision_manifest.v1"
	compensationReviewBundleSchemaVersion = "nexusim.workflow.compensation_review_bundle.v1"
	maxDecisionManifestBytes              = 64 * 1024
)

func main() {
	cfg := parseFlags(os.Args[1:])
	if err := run(context.Background(), cfg, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func parseFlags(args []string) config {
	cfg := config{}
	flags := flag.NewFlagSet("workflow-operator", flag.ExitOnError)
	flags.StringVar(&cfg.mode, "mode", "get", "mode: compensation-review-bundle, external-callback-wait, get, record-decision, list-workflows, provider-replay-queue, operator-queues, list-compensation-instructions")
	flags.StringVar(&cfg.target, "target", envOr("NEXUSIM_WORKFLOW_GRPC_ADDR", "127.0.0.1:10750"), "workflow-service gRPC target")
	flags.DurationVar(&cfg.requestTimeout, "request-timeout", 5*time.Second, "request timeout")
	flags.StringVar(&cfg.tls.CAFile, "workflow-tls-ca-file", os.Getenv("NEXUSIM_WORKFLOW_TLS_CA_FILE"), "CA PEM for workflow-service gRPC TLS")
	flags.StringVar(&cfg.tls.ServerName, "workflow-tls-server-name", os.Getenv("NEXUSIM_WORKFLOW_TLS_SERVER_NAME"), "server name for workflow-service gRPC TLS")
	flags.StringVar(&cfg.tls.ClientCertFile, "workflow-tls-client-cert-file", os.Getenv("NEXUSIM_WORKFLOW_TLS_CLIENT_CERT_FILE"), "client certificate PEM for workflow-service mTLS")
	flags.StringVar(&cfg.tls.ClientKeyFile, "workflow-tls-client-key-file", os.Getenv("NEXUSIM_WORKFLOW_TLS_CLIENT_KEY_FILE"), "client private key PEM for workflow-service mTLS")
	flags.StringVar(&cfg.tenantID, "tenant-id", envOr("NEXUSIM_TENANT_ID", "tenant-workflow-operator"), "tenant id")
	flags.StringVar(&cfg.userID, "user-id", "workflow-operator-cli", "auth user id")
	flags.StringVar(&cfg.instanceRef, "instance-ref", "workflow-operator-cli", "operator client instance ref")
	flags.StringVar(&cfg.traceID, "trace-id", "", "trace id")
	flags.StringVar(&cfg.requestID, "request-id", "", "request id")
	flags.StringVar(&cfg.correlationID, "correlation-id", "", "optional correlation id for record-decision")
	flags.StringVar(&cfg.causationID, "causation-id", "", "optional causation id for record-decision")
	flags.StringVar(&cfg.requesterRef, "requester-ref", "operator:workflow-cli", "low-sensitive requester ref for create workflow modes")
	flags.StringVar(&cfg.requesterService, "requester-service", "workflow-operator", "low-sensitive requester service for create workflow modes")
	flags.StringVar(&cfg.workflowID, "workflow-id", "", "workflow id")
	flags.StringVar(&cfg.workflowType, "workflow-type", "", "workflow type filter for list-workflows")
	flags.StringVar(&cfg.expectedWorkflowType, "expected-workflow-type", "", "expected workflow type for binding checks")
	flags.StringVar(&cfg.expectedStatus, "expected-workflow-status", "", "expected workflow status for binding checks")
	flags.StringVar(&cfg.stepID, "step-id", "", "workflow step id for record-decision")
	flags.StringVar(&cfg.decisionManifestPath, "decision-manifest", "", "optional low-sensitive decision manifest JSON for record-decision")
	flags.StringVar(&cfg.deciderRef, "decider-ref", "operator:workflow-cli", "low-sensitive decider ref for record-decision")
	flags.StringVar(&cfg.decision, "decision", "APPROVE", "decision type for record-decision")
	flags.StringVar(&cfg.decisionPolicy, "decision-policy-ref", "workflow.operator.cli.v1", "low-sensitive decision policy ref")
	flags.StringVar(&cfg.reasonRef, "reason-ref", "reason:workflow-cli", "low-sensitive reason ref")
	var evidenceRefs string
	flags.StringVar(&evidenceRefs, "evidence-refs", "evidence:workflow-cli", "comma-separated low-sensitive evidence refs")
	flags.StringVar(&cfg.idempotencyKey, "idempotency-key", "", "idempotency key for record-decision")
	flags.StringVar(&cfg.riskLevel, "risk-level", "", "risk level for create workflow modes")
	flags.StringVar(&cfg.status, "status", "", "optional workflow or instruction status filter")
	flags.StringVar(&cfg.targetService, "target-service", "", "target service filter for list-workflows")
	flags.StringVar(&cfg.targetOperation, "target-operation", "", "target operation filter for list-workflows")
	flags.StringVar(&cfg.targetRefHash, "target-ref-hash", "", "low-sensitive target ref hash for create workflow modes")
	flags.StringVar(&cfg.payloadSchemaVersion, "payload-schema-version", "", "payload schema version for create workflow modes")
	flags.StringVar(&cfg.payloadRefHash, "payload-ref-hash", "", "low-sensitive payload ref hash for create workflow modes")
	flags.StringVar(&cfg.approvalPolicyRef, "approval-policy-ref", "", "approval policy ref filter for list-workflows")
	flags.StringVar(&cfg.timeoutPolicyRef, "timeout-policy-ref", "", "optional timeout policy ref for create workflow modes")
	flags.StringVar(&cfg.compensationPolicyRef, "compensation-policy-ref", "", "optional compensation policy ref for create workflow modes")
	var pageSize int
	flags.IntVar(&pageSize, "page-size", 50, "list page size")
	_ = flags.Parse(args)
	cfg.mode = strings.ToLower(strings.TrimSpace(cfg.mode))
	cfg.status = strings.ToUpper(strings.TrimSpace(cfg.status))
	cfg.requestID = strings.TrimSpace(cfg.requestID)
	cfg.traceID = strings.TrimSpace(cfg.traceID)
	cfg.correlationID = strings.TrimSpace(cfg.correlationID)
	cfg.causationID = strings.TrimSpace(cfg.causationID)
	cfg.requesterRef = strings.TrimSpace(cfg.requesterRef)
	cfg.requesterService = strings.TrimSpace(cfg.requesterService)
	cfg.decisionManifestPath = strings.TrimSpace(cfg.decisionManifestPath)
	cfg.workflowID = strings.TrimSpace(cfg.workflowID)
	cfg.workflowType = strings.ToUpper(strings.TrimSpace(cfg.workflowType))
	cfg.expectedWorkflowType = strings.ToUpper(strings.TrimSpace(cfg.expectedWorkflowType))
	cfg.expectedStatus = strings.ToUpper(strings.TrimSpace(cfg.expectedStatus))
	cfg.stepID = strings.TrimSpace(cfg.stepID)
	cfg.deciderRef = strings.TrimSpace(cfg.deciderRef)
	cfg.decision = strings.ToUpper(strings.TrimSpace(cfg.decision))
	cfg.decisionPolicy = strings.TrimSpace(cfg.decisionPolicy)
	cfg.reasonRef = strings.TrimSpace(cfg.reasonRef)
	cfg.evidenceRefs = splitCSV(evidenceRefs)
	cfg.idempotencyKey = strings.TrimSpace(cfg.idempotencyKey)
	cfg.riskLevel = strings.ToUpper(strings.TrimSpace(cfg.riskLevel))
	cfg.targetService = strings.TrimSpace(cfg.targetService)
	cfg.targetOperation = strings.TrimSpace(cfg.targetOperation)
	cfg.targetRefHash = strings.TrimSpace(cfg.targetRefHash)
	cfg.payloadSchemaVersion = strings.TrimSpace(cfg.payloadSchemaVersion)
	cfg.payloadRefHash = strings.TrimSpace(cfg.payloadRefHash)
	cfg.approvalPolicyRef = strings.TrimSpace(cfg.approvalPolicyRef)
	cfg.timeoutPolicyRef = strings.TrimSpace(cfg.timeoutPolicyRef)
	cfg.compensationPolicyRef = strings.TrimSpace(cfg.compensationPolicyRef)
	cfg.pageSize = int32(pageSize)
	return fillDerivedDefaults(cfg)
}

func fillDerivedDefaults(cfg config) config {
	if cfg.requestID == "" {
		cfg.requestID = "workflow-operator-" + cfg.mode
	}
	if cfg.traceID == "" {
		cfg.traceID = cfg.requestID
	}
	if cfg.correlationID == "" {
		cfg.correlationID = cfg.requestID
	}
	if cfg.causationID == "" && cfg.mode == "record-decision" {
		cfg.causationID = cfg.workflowID
	}
	if cfg.idempotencyKey == "" && cfg.mode == "record-decision" {
		cfg.idempotencyKey = "decision:" + cfg.workflowID + ":" + cfg.stepID + ":" + cfg.decision + ":" + cfg.deciderRef
	}
	if cfg.mode == "external-callback-wait" && cfg.causationID == "" {
		cfg.causationID = cfg.requestID
	}
	if cfg.mode == "provider-replay-queue" {
		if cfg.workflowType == "" {
			cfg.workflowType = "REPAIR_APPROVAL"
		}
		if cfg.status == "" {
			cfg.status = "WAITING_DECISION"
		}
		if cfg.targetService == "" {
			cfg.targetService = "action-executor"
		}
		if cfg.targetOperation == "" {
			cfg.targetOperation = "PROVIDER_REPLAY_REQUEST"
		}
		if cfg.approvalPolicyRef == "" {
			cfg.approvalPolicyRef = "admin.workflow.provider_replay.v1"
		}
	}
	if cfg.mode == "compensation-review-bundle" {
		if cfg.status == "" {
			cfg.status = "ACTIVE"
		}
		if cfg.expectedWorkflowType == "" {
			cfg.expectedWorkflowType = "COMPENSATION_REQUEST"
		}
		if cfg.expectedStatus == "" {
			cfg.expectedStatus = "COMPENSATION_PENDING"
		}
	}
	return cfg
}

func run(ctx context.Context, cfg config, out io.Writer) error {
	prepared, err := prepareConfig(cfg)
	if err != nil {
		return err
	}
	cfg = prepared
	if err := cfg.validate(); err != nil {
		return err
	}
	dialOption, err := grpctls.DialOption(cfg.tls, "workflow-tls")
	if err != nil {
		return err
	}
	conn, err := grpc.NewClient("passthrough:///"+cfg.target, dialOption)
	if err != nil {
		return fmt.Errorf("dial workflow-service: %w", err)
	}
	defer conn.Close()
	client := workflowv1.NewWorkflowServiceClient(conn)
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	result, err := execute(requestCtx, cfg, client)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

func prepareConfig(cfg config) (config, error) {
	if strings.TrimSpace(cfg.decisionManifestPath) == "" {
		return cfg, nil
	}
	if cfg.mode != "record-decision" {
		return cfg, errors.New("decision-manifest is only supported for record-decision")
	}
	manifest, err := readDecisionManifest(cfg.decisionManifestPath)
	if err != nil {
		return cfg, err
	}
	cfg.workflowID = strings.TrimSpace(manifest.WorkflowID)
	cfg.stepID = strings.TrimSpace(manifest.StepID)
	cfg.expectedWorkflowType = strings.ToUpper(strings.TrimSpace(manifest.ExpectedWorkflowType))
	cfg.expectedStatus = strings.ToUpper(strings.TrimSpace(manifest.ExpectedStatus))
	cfg.expectedTargetService = strings.TrimSpace(manifest.ExpectedTargetService)
	cfg.expectedTargetOperation = strings.TrimSpace(manifest.ExpectedTargetOperation)
	cfg.expectedTargetRefHash = strings.TrimSpace(manifest.ExpectedTargetRefHash)
	cfg.expectedPayloadSchemaVersion = strings.TrimSpace(manifest.ExpectedPayloadSchemaVersion)
	cfg.expectedPayloadRefHash = strings.TrimSpace(manifest.ExpectedPayloadRefHash)
	cfg.expectedApprovalPolicyRef = strings.TrimSpace(manifest.ExpectedApprovalPolicyRef)
	cfg.deciderRef = strings.TrimSpace(manifest.DeciderRef)
	cfg.decision = strings.ToUpper(strings.TrimSpace(manifest.Decision))
	cfg.decisionPolicy = strings.TrimSpace(manifest.DecisionPolicyRef)
	cfg.reasonRef = strings.TrimSpace(manifest.ReasonRef)
	cfg.evidenceRefs = normalizeRefs(manifest.EvidenceRefs)
	cfg.idempotencyKey = strings.TrimSpace(manifest.IdempotencyKey)
	if strings.TrimSpace(manifest.CorrelationID) != "" {
		cfg.correlationID = strings.TrimSpace(manifest.CorrelationID)
	}
	if strings.TrimSpace(manifest.CausationID) != "" {
		cfg.causationID = strings.TrimSpace(manifest.CausationID)
	}
	if strings.TrimSpace(manifest.TraceID) != "" {
		cfg.traceID = strings.TrimSpace(manifest.TraceID)
	}
	return fillDerivedDefaults(cfg), nil
}

func readDecisionManifest(path string) (decisionManifest, error) {
	path = strings.TrimSpace(path)
	info, err := os.Stat(path)
	if err != nil {
		return decisionManifest{}, fmt.Errorf("read decision manifest: %w", err)
	}
	if info.Size() > maxDecisionManifestBytes {
		return decisionManifest{}, errors.New("decision manifest is too large")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return decisionManifest{}, fmt.Errorf("read decision manifest: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var manifest decisionManifest
	if err := decoder.Decode(&manifest); err != nil {
		return decisionManifest{}, fmt.Errorf("decode decision manifest: %w", err)
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return decisionManifest{}, errors.New("decision manifest must contain a single JSON object")
	}
	if strings.TrimSpace(manifest.SchemaVersion) != decisionManifestSchemaVersion {
		return decisionManifest{}, fmt.Errorf("unsupported decision manifest schema_version %q", manifest.SchemaVersion)
	}
	return manifest, nil
}

func execute(ctx context.Context, cfg config, client workflowv1.WorkflowServiceClient) (commandResult, error) {
	result := commandResult{
		Mode:              cfg.mode,
		Target:            cfg.target,
		TenantID:          cfg.tenantID,
		WorkflowID:        cfg.workflowID,
		WorkflowType:      cfg.workflowType,
		Status:            cfg.status,
		TargetService:     cfg.targetService,
		TargetOperation:   cfg.targetOperation,
		ApprovalPolicyRef: cfg.approvalPolicyRef,
		CheckedAt:         time.Now().UTC(),
	}
	switch cfg.mode {
	case "external-callback-wait":
		response, err := client.CreateWorkflow(ctx, &workflowv1.CreateWorkflowRequest{
			AuthContext:           authContext(cfg),
			RequesterRef:          cfg.requesterRef,
			RequesterService:      cfg.requesterService,
			WorkflowType:          cfg.workflowType,
			RiskLevel:             cfg.riskLevel,
			TargetRefHash:         cfg.targetRefHash,
			TargetService:         cfg.targetService,
			TargetOperation:       cfg.targetOperation,
			ApprovalPolicyRef:     cfg.approvalPolicyRef,
			TimeoutPolicyRef:      cfg.timeoutPolicyRef,
			CompensationPolicyRef: cfg.compensationPolicyRef,
			PayloadSchemaVersion:  cfg.payloadSchemaVersion,
			PayloadRefHash:        cfg.payloadRefHash,
			ReasonRef:             cfg.reasonRef,
			EvidenceRefs:          append([]string(nil), cfg.evidenceRefs...),
			IdempotencyKey:        cfg.idempotencyKey,
			CorrelationId:         cfg.correlationID,
			CausationId:           cfg.causationID,
			TraceId:               cfg.traceID,
		})
		if err != nil {
			return commandResult{}, fmt.Errorf("create external callback wait workflow: %w", err)
		}
		result.Workflow = summarizeWorkflow(response.GetWorkflow())
		result.Replayed = response.GetReplayed()
		if result.Workflow != nil {
			result.WorkflowID = result.Workflow.WorkflowID
			result.WorkflowType = result.Workflow.WorkflowType
			result.Status = result.Workflow.Status
			result.TargetService = result.Workflow.TargetService
			result.TargetOperation = result.Workflow.TargetOperation
			result.ApprovalPolicyRef = result.Workflow.ApprovalPolicyRef
			result.DecisionManifest = buildDecisionManifestTemplate(*result.Workflow, cfg)
		}
	case "get":
		response, err := client.GetWorkflow(ctx, &workflowv1.GetWorkflowRequest{
			AuthContext: authContext(cfg),
			WorkflowId:  cfg.workflowID,
		})
		if err != nil {
			return commandResult{}, fmt.Errorf("get workflow: %w", err)
		}
		result.Workflow = summarizeWorkflow(response.GetWorkflow())
		for _, decision := range response.GetDecisions() {
			result.Decisions = append(result.Decisions, summarizeDecision(decision))
		}
	case "list-workflows", "provider-replay-queue":
		response, err := client.ListWorkflows(ctx, &workflowv1.ListWorkflowsRequest{
			AuthContext:       authContext(cfg),
			WorkflowType:      cfg.workflowType,
			Status:            cfg.status,
			TargetService:     cfg.targetService,
			TargetOperation:   cfg.targetOperation,
			ApprovalPolicyRef: cfg.approvalPolicyRef,
			PageSize:          cfg.pageSize,
		})
		if err != nil {
			return commandResult{}, fmt.Errorf("list workflows: %w", err)
		}
		for _, workflow := range response.GetWorkflows() {
			if summarized := summarizeWorkflow(workflow); summarized != nil {
				result.Workflows = append(result.Workflows, *summarized)
			}
		}
	case "operator-queues":
		for _, queue := range defaultOperatorQueues() {
			response, err := client.ListWorkflows(ctx, &workflowv1.ListWorkflowsRequest{
				AuthContext:       authContext(cfg),
				WorkflowType:      queue.WorkflowType,
				Status:            queue.Status,
				TargetService:     queue.TargetService,
				TargetOperation:   queue.TargetOperation,
				ApprovalPolicyRef: queue.ApprovalPolicyRef,
				PageSize:          cfg.pageSize,
			})
			if err != nil {
				return commandResult{}, fmt.Errorf("list operator queue %s workflows: %w", queue.QueueID, err)
			}
			queue.Workflows = nil
			for _, workflow := range response.GetWorkflows() {
				if summarized := summarizeWorkflow(workflow); summarized != nil {
					queue.Workflows = append(queue.Workflows, *summarized)
				}
			}
			queue.WorkflowCount = len(queue.Workflows)
			result.OperatorQueues = append(result.OperatorQueues, queue)
		}
	case "record-decision":
		if cfg.decisionManifestPath != "" {
			response, err := client.GetWorkflow(ctx, &workflowv1.GetWorkflowRequest{
				AuthContext: authContext(cfg),
				WorkflowId:  cfg.workflowID,
			})
			if err != nil {
				return commandResult{}, fmt.Errorf("get workflow for external decision binding: %w", err)
			}
			if err := verifyExternalDecisionBinding(cfg, response.GetWorkflow()); err != nil {
				return commandResult{}, err
			}
		}
		response, err := client.RecordWorkflowDecision(ctx, &workflowv1.RecordWorkflowDecisionRequest{
			AuthContext:       authContext(cfg),
			WorkflowId:        cfg.workflowID,
			StepId:            cfg.stepID,
			DecisionType:      cfg.decision,
			DeciderRef:        cfg.deciderRef,
			DecisionPolicyRef: cfg.decisionPolicy,
			ReasonRef:         cfg.reasonRef,
			EvidenceRefs:      append([]string(nil), cfg.evidenceRefs...),
			IdempotencyKey:    cfg.idempotencyKey,
			CorrelationId:     cfg.correlationID,
			CausationId:       cfg.causationID,
			TraceId:           cfg.traceID,
		})
		if err != nil {
			return commandResult{}, fmt.Errorf("record workflow decision: %w", err)
		}
		result.Workflow = summarizeWorkflow(response.GetWorkflow())
		decision := summarizeDecision(response.GetDecision())
		result.Decision = &decision
		result.Replayed = response.GetReplayed()
	case "list-compensation-instructions":
		response, err := client.ListWorkflowCompensationInstructions(ctx, &workflowv1.ListWorkflowCompensationInstructionsRequest{
			AuthContext: authContext(cfg),
			WorkflowId:  cfg.workflowID,
			Status:      cfg.status,
			PageSize:    cfg.pageSize,
		})
		if err != nil {
			return commandResult{}, fmt.Errorf("list workflow compensation instructions: %w", err)
		}
		for _, instruction := range response.GetInstructions() {
			result.Instructions = append(result.Instructions, summarizeInstruction(instruction))
		}
	case "compensation-review-bundle":
		workflowResponse, err := client.GetWorkflow(ctx, &workflowv1.GetWorkflowRequest{
			AuthContext: authContext(cfg),
			WorkflowId:  cfg.workflowID,
		})
		if err != nil {
			return commandResult{}, fmt.Errorf("get workflow for compensation review bundle: %w", err)
		}
		if err := verifyCompensationReviewWorkflowBinding(cfg, workflowResponse.GetWorkflow()); err != nil {
			return commandResult{}, err
		}
		workflow := summarizeWorkflow(workflowResponse.GetWorkflow())
		if workflow == nil {
			return commandResult{}, errors.New("compensation review workflow not found")
		}
		instructionsResponse, err := client.ListWorkflowCompensationInstructions(ctx, &workflowv1.ListWorkflowCompensationInstructionsRequest{
			AuthContext: authContext(cfg),
			WorkflowId:  cfg.workflowID,
			Status:      cfg.status,
			PageSize:    cfg.pageSize,
		})
		if err != nil {
			return commandResult{}, fmt.Errorf("list workflow compensation instructions for review bundle: %w", err)
		}
		for _, instruction := range instructionsResponse.GetInstructions() {
			summarized := summarizeInstruction(instruction)
			if err := verifyCompensationReviewInstructionBinding(*workflow, summarized); err != nil {
				return commandResult{}, err
			}
			result.Instructions = append(result.Instructions, summarized)
		}
		if len(result.Instructions) == 0 {
			return commandResult{}, errors.New("no matching compensation instructions found for review bundle")
		}
		result.Workflow = workflow
		result.WorkflowID = workflow.WorkflowID
		result.WorkflowType = workflow.WorkflowType
		result.Status = workflow.Status
		result.TargetService = workflow.TargetService
		result.TargetOperation = workflow.TargetOperation
		result.ApprovalPolicyRef = workflow.ApprovalPolicyRef
		result.CompensationReview = buildCompensationReviewBundle(*workflow, cfg.status, result.Instructions)
	default:
		return commandResult{}, fmt.Errorf("unsupported mode %q", cfg.mode)
	}
	return result, nil
}

func (cfg config) validate() error {
	if strings.TrimSpace(cfg.target) == "" {
		return errors.New("target is required")
	}
	if strings.TrimSpace(cfg.tenantID) == "" {
		return errors.New("tenant-id is required")
	}
	if strings.TrimSpace(cfg.userID) == "" {
		return errors.New("user-id is required")
	}
	if !isAllowedMode(cfg.mode) {
		return fmt.Errorf("unsupported mode %q", cfg.mode)
	}
	if requiresWorkflowID(cfg.mode) && strings.TrimSpace(cfg.workflowID) == "" {
		return errors.New("workflow-id is required")
	}
	if cfg.mode == "record-decision" {
		if strings.TrimSpace(cfg.stepID) == "" {
			return errors.New("step-id is required for record-decision")
		}
		if cfg.decisionManifestPath != "" {
			if err := validateExternalDecisionBindingConfig(cfg); err != nil {
				return err
			}
		}
		if strings.TrimSpace(cfg.deciderRef) == "" {
			return errors.New("decider-ref is required for record-decision")
		}
		if err := validateLowSensitiveRef("decider-ref", cfg.deciderRef); err != nil {
			return err
		}
		if err := validateLowSensitiveRef("decision-policy-ref", cfg.decisionPolicy); err != nil {
			return err
		}
		if err := validateLowSensitiveRef("reason-ref", cfg.reasonRef); err != nil {
			return err
		}
		if err := validateLowSensitiveRefs("evidence-refs", cfg.evidenceRefs); err != nil {
			return err
		}
		if !isAllowedDecision(cfg.decision) {
			return errors.New("decision must be APPROVE, REJECT, REQUEST_CHANGES, or CANCEL")
		}
		if strings.TrimSpace(cfg.idempotencyKey) == "" {
			return errors.New("idempotency-key is required for record-decision")
		}
	}
	if cfg.mode == "external-callback-wait" {
		if strings.TrimSpace(cfg.workflowType) == "" {
			return errors.New("workflow-type is required for external-callback-wait")
		}
		if strings.TrimSpace(cfg.riskLevel) == "" {
			return errors.New("risk-level is required for external-callback-wait")
		}
		if strings.TrimSpace(cfg.requesterRef) == "" || strings.TrimSpace(cfg.requesterService) == "" {
			return errors.New("requester-ref and requester-service are required for external-callback-wait")
		}
		if strings.TrimSpace(cfg.targetService) == "" ||
			strings.TrimSpace(cfg.targetOperation) == "" ||
			strings.TrimSpace(cfg.targetRefHash) == "" {
			return errors.New("target-service, target-operation and target-ref-hash are required for external-callback-wait")
		}
		if strings.TrimSpace(cfg.payloadSchemaVersion) == "" || strings.TrimSpace(cfg.payloadRefHash) == "" {
			return errors.New("payload-schema-version and payload-ref-hash are required for external-callback-wait")
		}
		if strings.TrimSpace(cfg.approvalPolicyRef) == "" {
			return errors.New("approval-policy-ref is required for external-callback-wait")
		}
		if strings.TrimSpace(cfg.idempotencyKey) == "" {
			return errors.New("idempotency-key is required for external-callback-wait")
		}
		for _, item := range []struct {
			name  string
			value string
		}{
			{"requester-ref", cfg.requesterRef},
			{"requester-service", cfg.requesterService},
			{"target-service", cfg.targetService},
			{"target-operation", cfg.targetOperation},
			{"target-ref-hash", cfg.targetRefHash},
			{"payload-schema-version", cfg.payloadSchemaVersion},
			{"payload-ref-hash", cfg.payloadRefHash},
			{"approval-policy-ref", cfg.approvalPolicyRef},
			{"timeout-policy-ref", cfg.timeoutPolicyRef},
			{"compensation-policy-ref", cfg.compensationPolicyRef},
			{"reason-ref", cfg.reasonRef},
		} {
			if err := validateLowSensitiveRef(item.name, item.value); err != nil {
				return err
			}
		}
		if err := validateLowSensitiveRefs("evidence-refs", cfg.evidenceRefs); err != nil {
			return err
		}
		if !isAllowedWorkflowType(cfg.workflowType) {
			return errors.New("workflow-type must be ACTION_APPROVAL, REPAIR_APPROVAL, ADMIN_OPERATION, or COMPENSATION_REQUEST")
		}
		if !isAllowedRiskLevel(cfg.riskLevel) {
			return errors.New("risk-level must be LOW, MEDIUM, HIGH, or CRITICAL")
		}
	}
	if isListMode(cfg.mode) && cfg.pageSize <= 0 {
		return errors.New("page-size must be greater than zero")
	}
	if cfg.mode == "list-workflows" || cfg.mode == "provider-replay-queue" {
		if err := validateLowSensitiveRef("target-service", cfg.targetService); err != nil {
			return err
		}
		if err := validateLowSensitiveRef("target-operation", cfg.targetOperation); err != nil {
			return err
		}
		if err := validateLowSensitiveRef("approval-policy-ref", cfg.approvalPolicyRef); err != nil {
			return err
		}
	}
	if cfg.mode == "compensation-review-bundle" {
		if cfg.expectedWorkflowType != "COMPENSATION_REQUEST" {
			return errors.New("expected-workflow-type must be COMPENSATION_REQUEST for compensation-review-bundle")
		}
		if cfg.expectedStatus != "COMPENSATION_PENDING" {
			return errors.New("expected-workflow-status must be COMPENSATION_PENDING for compensation-review-bundle")
		}
		if cfg.status != "ACTIVE" {
			return errors.New("status must be ACTIVE for compensation-review-bundle")
		}
	}
	return nil
}

func buildDecisionManifestTemplate(workflow workflowRef, cfg config) *decisionManifestTemplate {
	return &decisionManifestTemplate{
		SchemaVersion:                decisionManifestSchemaVersion,
		WorkflowID:                   workflow.WorkflowID,
		StepID:                       workflow.CurrentStepID,
		ExpectedWorkflowType:         workflow.WorkflowType,
		ExpectedStatus:               workflow.Status,
		ExpectedTargetService:        workflow.TargetService,
		ExpectedTargetOperation:      workflow.TargetOperation,
		ExpectedTargetRefHash:        workflow.TargetRefHash,
		ExpectedPayloadSchemaVersion: workflow.PayloadSchemaVersion,
		ExpectedPayloadRefHash:       workflow.PayloadRefHash,
		ExpectedApprovalPolicyRef:    workflow.ApprovalPolicyRef,
		DecisionPolicyRef:            cfg.decisionPolicy,
		EvidenceRefs:                 append([]string(nil), cfg.evidenceRefs...),
		CorrelationID:                workflow.CorrelationID,
		CausationID:                  workflow.WorkflowID,
		TraceID:                      workflow.TraceID,
	}
}

func defaultOperatorQueues() []operatorQueueRef {
	return []operatorQueueRef{
		{
			QueueID:      "action-approval",
			WorkflowType: "ACTION_APPROVAL",
			Status:       "WAITING_DECISION",
		},
		{
			QueueID:      "repair-approval",
			WorkflowType: "REPAIR_APPROVAL",
			Status:       "WAITING_DECISION",
		},
		{
			QueueID:           "provider-replay",
			WorkflowType:      "REPAIR_APPROVAL",
			Status:            "WAITING_DECISION",
			TargetService:     "action-executor",
			TargetOperation:   "PROVIDER_REPLAY_REQUEST",
			ApprovalPolicyRef: "admin.workflow.provider_replay.v1",
		},
		{
			QueueID:      "admin-operation",
			WorkflowType: "ADMIN_OPERATION",
			Status:       "WAITING_DECISION",
		},
		{
			QueueID:      "compensation-request",
			WorkflowType: "COMPENSATION_REQUEST",
			Status:       "WAITING_DECISION",
		},
		{
			QueueID:      "compensation-pending",
			WorkflowType: "COMPENSATION_REQUEST",
			Status:       "COMPENSATION_PENDING",
		},
	}
}

func validateExternalDecisionBindingConfig(cfg config) error {
	required := []struct {
		name  string
		value string
	}{
		{"expected_workflow_type", cfg.expectedWorkflowType},
		{"expected_status", cfg.expectedStatus},
		{"expected_target_service", cfg.expectedTargetService},
		{"expected_target_operation", cfg.expectedTargetOperation},
		{"expected_target_ref_hash", cfg.expectedTargetRefHash},
		{"expected_payload_schema_version", cfg.expectedPayloadSchemaVersion},
		{"expected_payload_ref_hash", cfg.expectedPayloadRefHash},
		{"expected_approval_policy_ref", cfg.expectedApprovalPolicyRef},
	}
	for _, item := range required {
		if strings.TrimSpace(item.value) == "" {
			return fmt.Errorf("%s is required in external decision manifest", item.name)
		}
		if err := validateLowSensitiveRef(item.name, item.value); err != nil {
			return err
		}
	}
	if cfg.expectedStatus != "WAITING_DECISION" {
		return errors.New("expected_status must be WAITING_DECISION for external decision manifest")
	}
	return nil
}

func verifyExternalDecisionBinding(cfg config, workflow *workflowv1.Workflow) error {
	if workflow == nil {
		return errors.New("external decision binding workflow not found")
	}
	checks := []struct {
		name string
		got  string
		want string
	}{
		{"workflow_id", workflow.GetWorkflowId(), cfg.workflowID},
		{"step_id", workflow.GetCurrentStepId(), cfg.stepID},
		{"workflow_type", workflow.GetWorkflowType(), cfg.expectedWorkflowType},
		{"status", workflow.GetStatus(), cfg.expectedStatus},
		{"target_service", workflow.GetTargetService(), cfg.expectedTargetService},
		{"target_operation", workflow.GetTargetOperation(), cfg.expectedTargetOperation},
		{"target_ref_hash", workflow.GetTargetRefHash(), cfg.expectedTargetRefHash},
		{"payload_schema_version", workflow.GetPayloadSchemaVersion(), cfg.expectedPayloadSchemaVersion},
		{"payload_ref_hash", workflow.GetPayloadRefHash(), cfg.expectedPayloadRefHash},
		{"approval_policy_ref", workflow.GetApprovalPolicyRef(), cfg.expectedApprovalPolicyRef},
	}
	for _, check := range checks {
		if strings.TrimSpace(check.got) != strings.TrimSpace(check.want) {
			return fmt.Errorf("external decision manifest %s binding mismatch", check.name)
		}
	}
	return nil
}

func verifyCompensationReviewWorkflowBinding(cfg config, workflow *workflowv1.Workflow) error {
	if workflow == nil {
		return errors.New("compensation review workflow not found")
	}
	checks := []struct {
		name string
		got  string
		want string
	}{
		{"workflow_id", workflow.GetWorkflowId(), cfg.workflowID},
		{"workflow_type", workflow.GetWorkflowType(), cfg.expectedWorkflowType},
		{"status", workflow.GetStatus(), cfg.expectedStatus},
	}
	for _, check := range checks {
		if strings.TrimSpace(check.got) != strings.TrimSpace(check.want) {
			return fmt.Errorf("compensation review %s binding mismatch", check.name)
		}
	}
	if strings.TrimSpace(workflow.GetPayloadRefHash()) == "" {
		return errors.New("compensation review workflow payload_ref_hash is required")
	}
	return nil
}

func verifyCompensationReviewInstructionBinding(workflow workflowRef, instruction compensationInstructionRef) error {
	checks := []struct {
		name string
		got  string
		want string
	}{
		{"workflow_id", instruction.WorkflowID, workflow.WorkflowID},
		{"payload_ref_hash", instruction.PayloadRefHash, workflow.PayloadRefHash},
		{"target_service", instruction.TargetService, workflow.TargetService},
		{"target_operation", instruction.TargetOperation, workflow.TargetOperation},
		{"status", instruction.Status, "ACTIVE"},
	}
	for _, check := range checks {
		if strings.TrimSpace(check.want) == "" {
			continue
		}
		if strings.TrimSpace(check.got) != strings.TrimSpace(check.want) {
			return fmt.Errorf("compensation review instruction %s binding mismatch", check.name)
		}
	}
	if strings.TrimSpace(instruction.InstructionID) == "" {
		return errors.New("compensation review instruction_id is required")
	}
	if strings.TrimSpace(instruction.InstructionType) == "" {
		return errors.New("compensation review instruction_type is required")
	}
	return nil
}

func buildCompensationReviewBundle(
	workflow workflowRef,
	instructionStatus string,
	instructions []compensationInstructionRef,
) *compensationReviewBundle {
	copiedInstructions := append([]compensationInstructionRef(nil), instructions...)
	return &compensationReviewBundle{
		SchemaVersion:     compensationReviewBundleSchemaVersion,
		Workflow:          workflow,
		InstructionStatus: instructionStatus,
		InstructionCount:  len(copiedInstructions),
		Instructions:      copiedInstructions,
		ReviewChecks: []string{
			"workflow_type_status_payload_binding_verified",
			"active_instruction_refs_bound_to_same_workflow",
			"instruction_payload_hash_matches_workflow_payload_hash",
			"instruction_target_matches_workflow_target",
			"operator_must_use_explicit_approval_or_repair_invocation",
		},
		ApprovalBoundary: []string{
			"review_bundle_is_read_only",
			"does_not_record_workflow_decision",
			"does_not_create_or_reuse_approval",
			"does_not_modify_compensation_instruction_status",
		},
		ExecutionBoundary: []string{
			"does_not_execute_compensation",
			"does_not_call_control_plane_or_action_executor",
			"workflow_compensation_executor_remains_final_compensation_execution_owner",
			"downstream_mutation_requires_public_service_api_and_audit",
		},
		NoDirectExecution:  true,
		NoDecisionRecorded: true,
	}
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
	if value == "" {
		return false
	}
	for _, marker := range []string{"secret", "token", "api_key", "apikey", "password", "private://", "raw:", "dsn=", "postgres://"} {
		if strings.Contains(value, marker) {
			return true
		}
	}
	return false
}

func authContext(cfg config) *workflowv1.AuthContext {
	return &workflowv1.AuthContext{
		TenantId:    cfg.tenantID,
		UserId:      cfg.userID,
		ServiceName: "workflow-operator",
		InstanceRef: cfg.instanceRef,
		TraceId:     cfg.traceID,
		RequestId:   cfg.requestID,
	}
}

func summarizeWorkflow(workflow *workflowv1.Workflow) *workflowRef {
	if workflow == nil {
		return nil
	}
	return &workflowRef{
		WorkflowID:            workflow.GetWorkflowId(),
		WorkflowType:          workflow.GetWorkflowType(),
		RiskLevel:             workflow.GetRiskLevel(),
		RequesterRef:          workflow.GetRequesterRef(),
		RequesterService:      workflow.GetRequesterService(),
		TargetService:         workflow.GetTargetService(),
		TargetOperation:       workflow.GetTargetOperation(),
		TargetRefHash:         workflow.GetTargetRefHash(),
		PayloadSchemaVersion:  workflow.GetPayloadSchemaVersion(),
		PayloadRefHash:        workflow.GetPayloadRefHash(),
		ApprovalPolicyRef:     workflow.GetApprovalPolicyRef(),
		TimeoutPolicyRef:      workflow.GetTimeoutPolicyRef(),
		CompensationPolicyRef: workflow.GetCompensationPolicyRef(),
		ReasonRef:             workflow.GetReasonRef(),
		EvidenceRefs:          append([]string(nil), workflow.GetEvidenceRefs()...),
		Status:                workflow.GetStatus(),
		CurrentStepID:         workflow.GetCurrentStepId(),
		CorrelationID:         workflow.GetCorrelationId(),
		CausationID:           workflow.GetCausationId(),
		TraceID:               workflow.GetTraceId(),
		CreatedAtUnixMs:       workflow.GetCreatedAtUnixMs(),
		UpdatedAtUnixMs:       workflow.GetUpdatedAtUnixMs(),
		CompletedAtUnixMs:     workflow.GetCompletedAtUnixMs(),
	}
}

func summarizeDecision(decision *workflowv1.WorkflowDecision) decisionRef {
	if decision == nil {
		return decisionRef{}
	}
	return decisionRef{
		DecisionID:        decision.GetDecisionId(),
		WorkflowID:        decision.GetWorkflowId(),
		StepID:            decision.GetStepId(),
		DeciderRef:        decision.GetDeciderRef(),
		DecisionType:      decision.GetDecisionType(),
		DecisionPolicyRef: decision.GetDecisionPolicyRef(),
		ReasonRef:         decision.GetReasonRef(),
		EvidenceRefs:      append([]string(nil), decision.GetEvidenceRefs()...),
		CreatedAtUnixMs:   decision.GetCreatedAtUnixMs(),
	}
}

func summarizeInstruction(instruction *workflowv1.WorkflowCompensationInstruction) compensationInstructionRef {
	if instruction == nil {
		return compensationInstructionRef{}
	}
	return compensationInstructionRef{
		InstructionID:   instruction.GetInstructionId(),
		WorkflowID:      instruction.GetWorkflowId(),
		PayloadRefHash:  instruction.GetPayloadRefHash(),
		TargetService:   instruction.GetTargetService(),
		TargetOperation: instruction.GetTargetOperation(),
		InstructionType: instruction.GetInstructionType(),
		Environment:     instruction.GetEnvironment(),
		ConfigKind:      instruction.GetConfigKind(),
		BundleKey:       instruction.GetBundleKey(),
		TargetVersion:   instruction.GetTargetVersion(),
		OperatorRef:     instruction.GetOperatorRef(),
		ReasonRef:       instruction.GetReasonRef(),
		Status:          instruction.GetStatus(),
		CreatedAtUnixMs: instruction.GetCreatedAtUnixMs(),
		UpdatedAtUnixMs: instruction.GetUpdatedAtUnixMs(),
	}
}

func isAllowedMode(value string) bool {
	return value == "external-callback-wait" ||
		value == "compensation-review-bundle" ||
		value == "get" ||
		value == "record-decision" ||
		value == "list-workflows" ||
		value == "provider-replay-queue" ||
		value == "operator-queues" ||
		value == "list-compensation-instructions"
}

func requiresWorkflowID(value string) bool {
	return value == "get" ||
		value == "record-decision" ||
		value == "compensation-review-bundle" ||
		value == "list-compensation-instructions"
}

func isListMode(value string) bool {
	return value == "list-workflows" ||
		value == "provider-replay-queue" ||
		value == "operator-queues" ||
		value == "compensation-review-bundle" ||
		value == "list-compensation-instructions"
}

func isAllowedDecision(value string) bool {
	switch value {
	case "APPROVE", "REJECT", "REQUEST_CHANGES", "CANCEL":
		return true
	default:
		return false
	}
}

func isAllowedWorkflowType(value string) bool {
	switch value {
	case "ACTION_APPROVAL", "REPAIR_APPROVAL", "ADMIN_OPERATION", "COMPENSATION_REQUEST":
		return true
	default:
		return false
	}
}

func isAllowedRiskLevel(value string) bool {
	switch value {
	case "LOW", "MEDIUM", "HIGH", "CRITICAL":
		return true
	default:
		return false
	}
}

func splitCSV(value string) []string {
	return normalizeRefs(strings.Split(value, ","))
}

func normalizeRefs(values []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, item := range values {
		item = strings.TrimSpace(item)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		result = append(result, item)
	}
	return result
}

func envOr(key string, defaultValue string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return defaultValue
}
