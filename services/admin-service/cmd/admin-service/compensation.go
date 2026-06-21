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

	"github.com/qsyy0921/IM/services/admin-service/internal/domain"
	postgresinfra "github.com/qsyy0921/IM/services/admin-service/internal/infrastructure/postgres"
	rpcinfra "github.com/qsyy0921/IM/services/admin-service/internal/infrastructure/rpc"
	"github.com/qsyy0921/IM/services/admin-service/internal/types"
)

const adminCompensationSummarySchemaVersion = 1

type adminCompensationSummary struct {
	SchemaVersion         int       `json:"schema_version"`
	Service               string    `json:"service"`
	Mode                  string    `json:"mode"`
	TenantID              string    `json:"tenant_id"`
	OperationID           string    `json:"operation_id"`
	DryRun                bool      `json:"dry_run"`
	Changed               bool      `json:"changed"`
	PreviousStatus        string    `json:"previous_status"`
	CurrentStatus         string    `json:"current_status"`
	RequestedByHash       string    `json:"requested_by_hash"`
	CompensationReasonRef string    `json:"compensation_reason_ref,omitempty"`
	ReasonFileSHA256      string    `json:"reason_file_sha256,omitempty"`
	WorkflowRef           string    `json:"workflow_ref,omitempty"`
	GeneratedAt           time.Time `json:"generated_at"`
}

func runCompensationRequest(ctx context.Context) error {
	tenantID := strings.TrimSpace(os.Getenv("NEXUSIM_ADMIN_COMPENSATION_TENANT_ID"))
	operationID := strings.TrimSpace(os.Getenv("NEXUSIM_ADMIN_COMPENSATION_OPERATION_ID"))
	requestedBy := strings.TrimSpace(envString("NEXUSIM_ADMIN_COMPENSATION_REQUESTED_BY", "operator:admin-compensation"))
	dryRun, err := envBoolDefault("NEXUSIM_ADMIN_COMPENSATION_DRY_RUN", true)
	if err != nil {
		return err
	}
	reasonRef := strings.TrimSpace(os.Getenv("NEXUSIM_ADMIN_COMPENSATION_REASON_REF"))
	reasonSHA, err := adminReasonFileSHA256(os.Getenv("NEXUSIM_ADMIN_COMPENSATION_REASON_FILE"))
	if err != nil {
		return err
	}
	if reasonRef == "" && reasonSHA != "" {
		reasonRef = "reason-sha256:" + reasonSHA
	}
	if tenantID == "" || operationID == "" || requestedBy == "" {
		return errors.New("NEXUSIM_ADMIN_COMPENSATION_TENANT_ID, NEXUSIM_ADMIN_COMPENSATION_OPERATION_ID and NEXUSIM_ADMIN_COMPENSATION_REQUESTED_BY are required")
	}
	if !dryRun && reasonRef == "" {
		return errors.New("NEXUSIM_ADMIN_COMPENSATION_REASON_REF or NEXUSIM_ADMIN_COMPENSATION_REASON_FILE is required when dry-run is false")
	}

	pool, err := openPGPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()
	repository := postgresinfra.NewRepository(pool)
	operation, changed, err := repository.RequestAdminOperationCompensation(ctx, types.RequestAdminOperationCompensationCommand{
		TenantID:              types.TenantID(tenantID),
		OperationID:           operationID,
		RequestedBy:           requestedBy,
		CompensationReasonRef: reasonRef,
		DryRun:                dryRun,
	})
	if err != nil {
		return err
	}
	workflowRef := ""
	if !dryRun {
		workflowRef, err = createCompensationWorkflowIfConfigured(ctx, operation, requestedBy, reasonRef)
		if err != nil {
			return err
		}
	}
	previousStatus := operation.Status
	if changed {
		previousStatus = types.OperationStatusFailed
	}
	summary := adminCompensationSummary{
		SchemaVersion:         adminCompensationSummarySchemaVersion,
		Service:               "admin-service",
		Mode:                  "compensation-request",
		TenantID:              tenantID,
		OperationID:           operationID,
		DryRun:                dryRun,
		Changed:               changed,
		PreviousStatus:        previousStatus,
		CurrentStatus:         operation.Status,
		RequestedByHash:       domain.HashText(requestedBy),
		CompensationReasonRef: reasonRef,
		ReasonFileSHA256:      reasonSHA,
		WorkflowRef:           workflowRef,
		GeneratedAt:           time.Now().UTC(),
	}
	return writeAdminCompensationSummary(summary, os.Getenv("NEXUSIM_ADMIN_COMPENSATION_OUTPUT"))
}

func createCompensationWorkflowIfConfigured(ctx context.Context, operation types.AdminOperation, requestedBy string, reasonRef string) (string, error) {
	workflowAddr := strings.TrimSpace(os.Getenv("NEXUSIM_WORKFLOW_GRPC_ADDR"))
	if workflowAddr == "" {
		return "", nil
	}
	executor, closeWorkflow, err := rpcinfra.DialWorkflowExecutor(
		ctx,
		workflowAddr,
		envDuration("NEXUSIM_ADMIN_COMPENSATION_WORKFLOW_RPC_TIMEOUT", envDuration("NEXUSIM_ADMIN_WORKFLOW_RPC_TIMEOUT", time.Second)),
	)
	if err != nil {
		return "", err
	}
	defer func() { _ = closeWorkflow() }()
	return executor.CreateCompensationWorkflow(ctx, operation, requestedBy, reasonRef)
}

func envBoolDefault(name string, fallback bool) (bool, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean: %w", name, err)
	}
	return value, nil
}

func adminReasonFileSHA256(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read admin compensation reason file: %w", err)
	}
	if len(content) > 64*1024 {
		return "", errors.New("admin compensation reason file is larger than 64 KiB")
	}
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:]), nil
}

func writeAdminCompensationSummary(summary adminCompensationSummary, outputPath string) error {
	encoded, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return err
	}
	outputPath = strings.TrimSpace(outputPath)
	if outputPath == "" {
		fmt.Println(string(encoded))
		return nil
	}
	if err := validateOutputOutsideRepo(outputPath); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(outputPath, encoded, 0o644)
}

func validateOutputOutsideRepo(path string) error {
	fullPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	repoRoot, err := filepath.Abs(".")
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(repoRoot, fullPath)
	if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && rel != "." {
		return errors.New("admin compensation output path must be outside repository")
	}
	return nil
}
