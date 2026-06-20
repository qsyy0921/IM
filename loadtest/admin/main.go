package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	adminv1 "github.com/qsyy0921/IM/api/proto/nexusim/admin/v1"
	"github.com/qsyy0921/IM/loadtest/internal/grpctls"
	"google.golang.org/grpc"
)

type config struct {
	mode               string
	target             string
	requestTimeout     time.Duration
	tls                grpctls.Config
	tenantID           string
	userID             string
	instanceRef        string
	traceID            string
	requestID          string
	operationID        string
	operatorRef        string
	operatorRole       string
	targetRefHash      string
	riskLevel          string
	payloadSchema      string
	operationPayload   string
	payloadFile        string
	payloadLoadError   error
	approverRef        string
	approverRole       string
	decision           string
	approvalPolicyRef  string
	reasonRef          string
	evidenceRefs       []string
	idempotencyKey     string
	status             string
	operationType      string
	pageSize           int32
	controlPlaneTarget string
	pgDSN              string
	resultRoot         string
	runName            string
	applyMigration     bool
	cleanup            bool
	pollTimeout        time.Duration
	pollInterval       time.Duration
}

type commandResult struct {
	Mode       string             `json:"mode"`
	Target     string             `json:"target"`
	TenantID   string             `json:"tenant_id"`
	Operation  *operationSummary  `json:"operation,omitempty"`
	Approval   *approvalSummary   `json:"approval,omitempty"`
	Operations []operationSummary `json:"operations,omitempty"`
	Replayed   bool               `json:"replayed,omitempty"`
	CheckedAt  time.Time          `json:"checked_at"`
}

type operationSummary struct {
	OperationID          string   `json:"operation_id"`
	OperationType        string   `json:"operation_type"`
	TargetRefHash        string   `json:"target_ref_hash"`
	RiskLevel            string   `json:"risk_level"`
	PayloadSchemaVersion string   `json:"payload_schema_version"`
	PayloadHash          string   `json:"payload_hash"`
	ReasonRef            string   `json:"reason_ref,omitempty"`
	EvidenceRefs         []string `json:"evidence_refs,omitempty"`
	Status               string   `json:"status"`
	RequestedBy          string   `json:"requested_by,omitempty"`
	ApprovedBy           string   `json:"approved_by,omitempty"`
	CorrelationID        string   `json:"correlation_id,omitempty"`
	CausationID          string   `json:"causation_id,omitempty"`
	TraceID              string   `json:"trace_id,omitempty"`
	RequestedAtUnixMs    int64    `json:"requested_at_unix_ms,omitempty"`
	ApprovedAtUnixMs     int64    `json:"approved_at_unix_ms,omitempty"`
	UpdatedAtUnixMs      int64    `json:"updated_at_unix_ms,omitempty"`
}

type approvalSummary struct {
	ApprovalID        string   `json:"approval_id"`
	OperationID       string   `json:"operation_id"`
	ApproverRef       string   `json:"approver_ref"`
	Decision          string   `json:"decision"`
	ApprovalPolicyRef string   `json:"approval_policy_ref,omitempty"`
	ReasonRef         string   `json:"reason_ref,omitempty"`
	EvidenceRefs      []string `json:"evidence_refs,omitempty"`
	CreatedAtUnixMs   int64    `json:"created_at_unix_ms,omitempty"`
}

func main() {
	cfg := parseFlags(os.Args[1:])
	if err := run(context.Background(), cfg, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func parseFlags(args []string) config {
	cfg := config{}
	flags := flag.NewFlagSet("admin-operator", flag.ExitOnError)
	flags.StringVar(&cfg.mode, "mode", "approve", "mode: create, approve, reject, get, list, or config-publish-smoke")
	flags.StringVar(&cfg.target, "target", envOr("NEXUSIM_ADMIN_GRPC_ADDR", "127.0.0.1:10770"), "admin-service gRPC target")
	flags.DurationVar(&cfg.requestTimeout, "request-timeout", 5*time.Second, "request timeout")
	flags.StringVar(&cfg.tls.CAFile, "admin-tls-ca-file", os.Getenv("NEXUSIM_ADMIN_TLS_CA_FILE"), "CA PEM for admin-service gRPC TLS")
	flags.StringVar(&cfg.tls.ServerName, "admin-tls-server-name", os.Getenv("NEXUSIM_ADMIN_TLS_SERVER_NAME"), "server name for admin-service gRPC TLS")
	flags.StringVar(&cfg.tls.ClientCertFile, "admin-tls-client-cert-file", os.Getenv("NEXUSIM_ADMIN_TLS_CLIENT_CERT_FILE"), "client certificate PEM for admin-service mTLS")
	flags.StringVar(&cfg.tls.ClientKeyFile, "admin-tls-client-key-file", os.Getenv("NEXUSIM_ADMIN_TLS_CLIENT_KEY_FILE"), "client private key PEM for admin-service mTLS")
	flags.StringVar(&cfg.tenantID, "tenant-id", envOr("NEXUSIM_TENANT_ID", "tenant-admin-operator"), "tenant id")
	flags.StringVar(&cfg.userID, "user-id", "operator-cli", "auth user id")
	flags.StringVar(&cfg.instanceRef, "instance-ref", "admin-operator-cli", "operator client instance ref")
	flags.StringVar(&cfg.traceID, "trace-id", "", "trace id")
	flags.StringVar(&cfg.requestID, "request-id", "", "request id")
	flags.StringVar(&cfg.operationID, "operation-id", "", "admin operation id")
	flags.StringVar(&cfg.operatorRef, "operator-ref", "operator:cli", "operator ref for create")
	flags.StringVar(&cfg.operatorRole, "operator-role", "ADMIN", "operator role for create")
	flags.StringVar(&cfg.targetRefHash, "target-ref-hash", "sha256:operator-cli-target", "low-sensitive target ref hash")
	flags.StringVar(&cfg.riskLevel, "risk-level", "MEDIUM", "operation risk level")
	flags.StringVar(&cfg.payloadSchema, "payload-schema-version", "", "operation payload schema version")
	flags.StringVar(&cfg.operationPayload, "operation-payload-json", "", "operation payload JSON; not printed in output")
	flags.StringVar(&cfg.payloadFile, "operation-payload-file", "", "path to operation payload JSON file; not printed in output")
	flags.StringVar(&cfg.approverRef, "approver-ref", "operator:cli", "approver ref")
	flags.StringVar(&cfg.approverRole, "approver-role", "ADMIN", "approver role")
	flags.StringVar(&cfg.decision, "decision", "APPROVE", "approval decision")
	flags.StringVar(&cfg.approvalPolicyRef, "approval-policy-ref", "admin.operator.cli.v1", "approval policy ref")
	flags.StringVar(&cfg.reasonRef, "reason-ref", "reason:operator-cli", "low-sensitive reason ref")
	var evidenceRefs string
	flags.StringVar(&evidenceRefs, "evidence-refs", "evidence:operator-cli", "comma-separated low-sensitive evidence refs")
	flags.StringVar(&cfg.idempotencyKey, "idempotency-key", "", "approval idempotency key")
	flags.StringVar(&cfg.status, "status", "", "list filter status")
	flags.StringVar(&cfg.operationType, "operation-type", "", "list filter operation type")
	var pageSize int
	flags.IntVar(&pageSize, "page-size", 20, "list page size")
	flags.StringVar(&cfg.controlPlaneTarget, "control-plane-target", envOr("NEXUSIM_CONTROL_PLANE_GRPC_ADDR", "127.0.0.1:10710"), "control-plane-service gRPC target for config-publish-smoke")
	flags.StringVar(&cfg.pgDSN, "pg-dsn", envOr("NEXUSIM_PG_DSN", "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable"), "PostgreSQL DSN for config-publish-smoke")
	flags.StringVar(&cfg.resultRoot, "result-root", defaultResultRoot, "external result root for config-publish-smoke")
	flags.StringVar(&cfg.runName, "run-name", "", "run name for config-publish-smoke")
	flags.BoolVar(&cfg.applyMigration, "apply-migration", true, "apply admin/control-plane migrations before config-publish-smoke")
	flags.BoolVar(&cfg.cleanup, "cleanup", true, "cleanup smoke tenant before config-publish-smoke")
	flags.DurationVar(&cfg.pollTimeout, "poll-timeout", 15*time.Second, "operation polling timeout for config-publish-smoke")
	flags.DurationVar(&cfg.pollInterval, "poll-interval", 300*time.Millisecond, "operation polling interval for config-publish-smoke")
	_ = flags.Parse(args)
	cfg.pageSize = int32(pageSize)
	cfg.mode = strings.ToLower(strings.TrimSpace(cfg.mode))
	cfg.decision = strings.ToUpper(strings.TrimSpace(cfg.decision))
	cfg.riskLevel = strings.ToUpper(strings.TrimSpace(cfg.riskLevel))
	cfg.evidenceRefs = splitCSV(evidenceRefs)
	cfg.operationPayload = strings.TrimSpace(cfg.operationPayload)
	if cfg.operationPayload == "" && strings.TrimSpace(cfg.payloadFile) != "" {
		if content, err := os.ReadFile(strings.TrimSpace(cfg.payloadFile)); err == nil {
			cfg.operationPayload = strings.TrimSpace(string(content))
		} else {
			cfg.payloadLoadError = err
		}
	}
	if cfg.requestID == "" {
		cfg.requestID = "admin-operator-" + cfg.mode
	}
	if cfg.traceID == "" {
		cfg.traceID = cfg.requestID
	}
	if cfg.idempotencyKey == "" && cfg.mode == "create" {
		cfg.idempotencyKey = "create:" + cfg.operationType + ":" + cfg.targetRefHash + ":" + cfg.operatorRef
	}
	if cfg.idempotencyKey == "" && (cfg.mode == "approve" || cfg.mode == "reject") {
		cfg.idempotencyKey = cfg.mode + ":" + cfg.operationID + ":" + cfg.approverRef
	}
	if cfg.runName == "" && cfg.mode == "config-publish-smoke" {
		cfg.runName = "admin-config-publish-smoke-" + time.Now().UTC().Format("20060102-150405")
	}
	return cfg
}

func run(ctx context.Context, cfg config, out *os.File) error {
	if err := cfg.validate(); err != nil {
		return err
	}
	if cfg.mode == "config-publish-smoke" {
		return runConfigPublishSmoke(ctx, cfg, out)
	}
	dialOption, err := grpctls.DialOption(cfg.tls, "admin-tls")
	if err != nil {
		return err
	}
	conn, err := grpc.NewClient("passthrough:///"+cfg.target, dialOption)
	if err != nil {
		return fmt.Errorf("dial admin-service: %w", err)
	}
	defer conn.Close()
	client := adminv1.NewAdminServiceClient(conn)
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

func execute(ctx context.Context, cfg config, client adminv1.AdminServiceClient) (commandResult, error) {
	result := commandResult{
		Mode:      cfg.mode,
		Target:    cfg.target,
		TenantID:  cfg.tenantID,
		CheckedAt: time.Now().UTC(),
	}
	switch cfg.mode {
	case "create":
		response, err := client.CreateAdminOperation(ctx, &adminv1.CreateAdminOperationRequest{
			AuthContext:          authContext(cfg),
			OperatorRef:          cfg.operatorRef,
			OperatorRole:         cfg.operatorRole,
			OperationType:        cfg.operationType,
			TargetRefHash:        cfg.targetRefHash,
			RiskLevel:            cfg.riskLevel,
			PayloadSchemaVersion: cfg.payloadSchema,
			OperationPayloadJson: cfg.operationPayload,
			ReasonRef:            cfg.reasonRef,
			EvidenceRefs:         append([]string(nil), cfg.evidenceRefs...),
			IdempotencyKey:       cfg.idempotencyKey,
			CorrelationId:        cfg.requestID,
			CausationId:          cfg.operationType,
			TraceId:              cfg.traceID,
		})
		if err != nil {
			return commandResult{}, fmt.Errorf("create admin operation: %w", err)
		}
		result.Operation = summarizeOperation(response.GetOperation())
		result.Replayed = response.GetReplayed()
	case "approve", "reject":
		decision := cfg.decision
		if cfg.mode == "reject" {
			decision = "REJECT"
		}
		response, err := client.ApproveAdminOperation(ctx, &adminv1.ApproveAdminOperationRequest{
			AuthContext:       authContext(cfg),
			OperationId:       cfg.operationID,
			ApproverRef:       cfg.approverRef,
			ApproverRole:      cfg.approverRole,
			Decision:          decision,
			ApprovalPolicyRef: cfg.approvalPolicyRef,
			ReasonRef:         cfg.reasonRef,
			EvidenceRefs:      append([]string(nil), cfg.evidenceRefs...),
			IdempotencyKey:    cfg.idempotencyKey,
			CorrelationId:     cfg.requestID,
			CausationId:       cfg.operationID,
			TraceId:           cfg.traceID,
		})
		if err != nil {
			return commandResult{}, fmt.Errorf("approve admin operation: %w", err)
		}
		result.Operation = summarizeOperation(response.GetOperation())
		result.Approval = summarizeApproval(response.GetApproval())
		result.Replayed = response.GetReplayed()
	case "get":
		response, err := client.GetAdminOperation(ctx, &adminv1.GetAdminOperationRequest{
			AuthContext: authContext(cfg),
			OperationId: cfg.operationID,
		})
		if err != nil {
			return commandResult{}, fmt.Errorf("get admin operation: %w", err)
		}
		result.Operation = summarizeOperation(response.GetOperation())
		if approvals := response.GetApprovals(); len(approvals) > 0 {
			result.Approval = summarizeApproval(approvals[len(approvals)-1])
		}
	case "list":
		response, err := client.ListAdminOperations(ctx, &adminv1.ListAdminOperationsRequest{
			AuthContext:   authContext(cfg),
			Status:        cfg.status,
			OperationType: cfg.operationType,
			PageSize:      cfg.pageSize,
		})
		if err != nil {
			return commandResult{}, fmt.Errorf("list admin operations: %w", err)
		}
		for _, operation := range response.GetOperations() {
			if summary := summarizeOperation(operation); summary != nil {
				result.Operations = append(result.Operations, *summary)
			}
		}
	default:
		return commandResult{}, fmt.Errorf("unsupported mode %q", cfg.mode)
	}
	return result, nil
}

func (cfg config) validate() error {
	if strings.TrimSpace(cfg.target) == "" {
		return errors.New("--target is required")
	}
	if strings.TrimSpace(cfg.tenantID) == "" {
		return errors.New("--tenant-id is required")
	}
	if cfg.requestTimeout <= 0 {
		return errors.New("--request-timeout must be positive")
	}
	switch cfg.mode {
	case "create":
		if cfg.payloadLoadError != nil {
			return fmt.Errorf("read --operation-payload-file: %w", cfg.payloadLoadError)
		}
		if strings.TrimSpace(cfg.operatorRef) == "" {
			return errors.New("--operator-ref is required")
		}
		if strings.TrimSpace(cfg.operatorRole) == "" {
			return errors.New("--operator-role is required")
		}
		if strings.TrimSpace(cfg.operationType) == "" {
			return errors.New("--operation-type is required")
		}
		if strings.TrimSpace(cfg.targetRefHash) == "" {
			return errors.New("--target-ref-hash is required")
		}
		if strings.TrimSpace(cfg.riskLevel) == "" {
			return errors.New("--risk-level is required")
		}
		if strings.TrimSpace(cfg.payloadSchema) == "" {
			return errors.New("--payload-schema-version is required")
		}
		if strings.TrimSpace(cfg.operationPayload) == "" {
			return errors.New("--operation-payload-json or --operation-payload-file is required")
		}
		if !json.Valid([]byte(cfg.operationPayload)) {
			return errors.New("--operation-payload-json must be valid JSON")
		}
		if strings.TrimSpace(cfg.idempotencyKey) == "" {
			return errors.New("--idempotency-key is required")
		}
	case "approve", "reject":
		if strings.TrimSpace(cfg.operationID) == "" {
			return errors.New("--operation-id is required")
		}
		if strings.TrimSpace(cfg.approverRef) == "" {
			return errors.New("--approver-ref is required")
		}
		if strings.TrimSpace(cfg.idempotencyKey) == "" {
			return errors.New("--idempotency-key is required")
		}
	case "get":
		if strings.TrimSpace(cfg.operationID) == "" {
			return errors.New("--operation-id is required")
		}
	case "list":
		if cfg.pageSize <= 0 {
			return errors.New("--page-size must be positive")
		}
	case "config-publish-smoke":
		if strings.TrimSpace(cfg.controlPlaneTarget) == "" {
			return errors.New("--control-plane-target is required")
		}
		if strings.TrimSpace(cfg.pgDSN) == "" {
			return errors.New("--pg-dsn is required")
		}
		if strings.TrimSpace(cfg.resultRoot) == "" {
			return errors.New("--result-root is required")
		}
		if strings.TrimSpace(cfg.runName) == "" {
			return errors.New("--run-name is required")
		}
		if cfg.pollTimeout <= 0 {
			return errors.New("--poll-timeout must be positive")
		}
		if cfg.pollInterval <= 0 {
			return errors.New("--poll-interval must be positive")
		}
	default:
		return fmt.Errorf("--mode must be create, approve, reject, get, list, or config-publish-smoke")
	}
	return nil
}

func authContext(cfg config) *adminv1.AuthContext {
	return &adminv1.AuthContext{
		TenantId:    cfg.tenantID,
		UserId:      cfg.userID,
		ServiceName: "admin-operator-cli",
		InstanceRef: cfg.instanceRef,
		TraceId:     cfg.traceID,
		RequestId:   cfg.requestID,
	}
}

func summarizeOperation(operation *adminv1.AdminOperation) *operationSummary {
	if operation == nil {
		return nil
	}
	return &operationSummary{
		OperationID:          operation.GetOperationId(),
		OperationType:        operation.GetOperationType(),
		TargetRefHash:        operation.GetTargetRefHash(),
		RiskLevel:            operation.GetRiskLevel(),
		PayloadSchemaVersion: operation.GetPayloadSchemaVersion(),
		PayloadHash:          operation.GetPayloadHash(),
		ReasonRef:            operation.GetReasonRef(),
		EvidenceRefs:         append([]string(nil), operation.GetEvidenceRefs()...),
		Status:               operation.GetStatus(),
		RequestedBy:          operation.GetRequestedBy(),
		ApprovedBy:           operation.GetApprovedBy(),
		CorrelationID:        operation.GetCorrelationId(),
		CausationID:          operation.GetCausationId(),
		TraceID:              operation.GetTraceId(),
		RequestedAtUnixMs:    operation.GetRequestedAtUnixMs(),
		ApprovedAtUnixMs:     operation.GetApprovedAtUnixMs(),
		UpdatedAtUnixMs:      operation.GetUpdatedAtUnixMs(),
	}
}

func summarizeApproval(approval *adminv1.AdminApproval) *approvalSummary {
	if approval == nil {
		return nil
	}
	return &approvalSummary{
		ApprovalID:        approval.GetApprovalId(),
		OperationID:       approval.GetOperationId(),
		ApproverRef:       approval.GetApproverRef(),
		Decision:          approval.GetDecision(),
		ApprovalPolicyRef: approval.GetApprovalPolicyRef(),
		ReasonRef:         approval.GetReasonRef(),
		EvidenceRefs:      append([]string(nil), approval.GetEvidenceRefs()...),
		CreatedAtUnixMs:   approval.GetCreatedAtUnixMs(),
	}
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	items := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if _, ok := seen[part]; ok {
			continue
		}
		seen[part] = struct{}{}
		items = append(items, part)
	}
	return items
}

func envOr(key string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}
