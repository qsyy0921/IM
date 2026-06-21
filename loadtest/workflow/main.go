package main

import (
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
	mode           string
	target         string
	requestTimeout time.Duration
	tls            grpctls.Config
	tenantID       string
	userID         string
	instanceRef    string
	traceID        string
	requestID      string
	workflowID     string
	status         string
	pageSize       int32
}

type commandResult struct {
	Mode         string                       `json:"mode"`
	Target       string                       `json:"target"`
	TenantID     string                       `json:"tenant_id"`
	WorkflowID   string                       `json:"workflow_id"`
	Status       string                       `json:"status,omitempty"`
	Instructions []compensationInstructionRef `json:"instructions"`
	CheckedAt    time.Time                    `json:"checked_at"`
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
	flags.StringVar(&cfg.mode, "mode", "list-compensation-instructions", "mode: list-compensation-instructions")
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
	flags.StringVar(&cfg.workflowID, "workflow-id", "", "workflow id")
	flags.StringVar(&cfg.status, "status", "", "optional instruction status filter")
	var pageSize int
	flags.IntVar(&pageSize, "page-size", 50, "list page size")
	_ = flags.Parse(args)
	cfg.mode = strings.ToLower(strings.TrimSpace(cfg.mode))
	cfg.status = strings.ToUpper(strings.TrimSpace(cfg.status))
	cfg.workflowID = strings.TrimSpace(cfg.workflowID)
	cfg.pageSize = int32(pageSize)
	if cfg.requestID == "" {
		cfg.requestID = "workflow-operator-" + cfg.mode
	}
	if cfg.traceID == "" {
		cfg.traceID = cfg.requestID
	}
	return cfg
}

func run(ctx context.Context, cfg config, out io.Writer) error {
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

func execute(ctx context.Context, cfg config, client workflowv1.WorkflowServiceClient) (commandResult, error) {
	result := commandResult{
		Mode:       cfg.mode,
		Target:     cfg.target,
		TenantID:   cfg.tenantID,
		WorkflowID: cfg.workflowID,
		Status:     cfg.status,
		CheckedAt:  time.Now().UTC(),
	}
	switch cfg.mode {
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
	if strings.TrimSpace(cfg.workflowID) == "" {
		return errors.New("workflow-id is required")
	}
	if cfg.mode != "list-compensation-instructions" {
		return fmt.Errorf("unsupported mode %q", cfg.mode)
	}
	if cfg.pageSize <= 0 {
		return errors.New("page-size must be greater than zero")
	}
	return nil
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

func envOr(key string, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
