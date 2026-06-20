package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	adminv1 "github.com/qsyy0921/IM/api/proto/nexusim/admin/v1"
	controlv1 "github.com/qsyy0921/IM/api/proto/nexusim/controlplane/v1"
	"github.com/qsyy0921/IM/loadtest/internal/grpctls"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const defaultResultRoot = `H:\NexusIM\loadtest-results`

type configPublishSmokeSummary struct {
	Commit             string    `json:"commit"`
	CommitFull         string    `json:"commit_full"`
	GitDirty           bool      `json:"git_dirty"`
	ResultDir          string    `json:"result_dir"`
	RunName            string    `json:"run_name"`
	AdminTarget        string    `json:"admin_target"`
	ControlPlaneTarget string    `json:"control_plane_target"`
	TenantID           string    `json:"tenant_id"`
	StartedAt          time.Time `json:"started_at"`
	FinishedAt         time.Time `json:"finished_at"`
	Success            bool      `json:"success"`
	Error              string    `json:"error,omitempty"`
	OperationID        string    `json:"operation_id,omitempty"`
	CreatedStatus      string    `json:"created_status,omitempty"`
	ApprovedStatus     string    `json:"approved_status,omitempty"`
	FinalStatus        string    `json:"final_status,omitempty"`
	SnapshotVersion    string    `json:"snapshot_version,omitempty"`
	SnapshotChecksum   string    `json:"snapshot_checksum,omitempty"`
	ReplayedCreate     bool      `json:"replayed_create,omitempty"`
	ReplayedApproval   bool      `json:"replayed_approval,omitempty"`
}

func runConfigPublishSmoke(ctx context.Context, cfg config, out *os.File) error {
	summary := configPublishSmokeSummary{
		Commit:             strings.TrimSpace(gitOutput("rev-parse", "--short", "HEAD")),
		CommitFull:         strings.TrimSpace(gitOutput("rev-parse", "HEAD")),
		GitDirty:           strings.TrimSpace(gitOutput("status", "--short", "--untracked-files=all")) != "",
		RunName:            cfg.runName,
		AdminTarget:        cfg.target,
		ControlPlaneTarget: cfg.controlPlaneTarget,
		TenantID:           cfg.tenantID,
		StartedAt:          time.Now().UTC(),
	}
	err := executeConfigPublishSmoke(ctx, cfg, &summary)
	summary.FinishedAt = time.Now().UTC()
	if err != nil {
		summary.Error = err.Error()
	} else {
		summary.Success = true
	}
	if writeErr := writeConfigPublishSmokeSummary(cfg, &summary, out); writeErr != nil {
		return writeErr
	}
	return err
}

func executeConfigPublishSmoke(ctx context.Context, cfg config, summary *configPublishSmokeSummary) error {
	if err := validateExternalResultRoot(cfg.resultRoot); err != nil {
		return err
	}
	pool, err := pgxpool.New(ctx, cfg.pgDSN)
	if err != nil {
		return fmt.Errorf("open pg pool: %w", err)
	}
	defer pool.Close()
	if cfg.applyMigration {
		if err := applySQLMigrations(ctx, pool, filepath.Join("migrations", "postgres", "control-plane")); err != nil {
			return err
		}
		if err := applySQLMigrations(ctx, pool, filepath.Join("migrations", "postgres", "admin")); err != nil {
			return err
		}
	}
	if cfg.cleanup {
		if err := cleanupConfigPublishSmokeTenant(ctx, pool, cfg.tenantID); err != nil {
			return err
		}
	}
	adminConn, err := dialAdmin(ctx, cfg)
	if err != nil {
		return err
	}
	defer adminConn.Close()
	controlConn, err := grpc.NewClient(
		"passthrough:///"+cfg.controlPlaneTarget,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return fmt.Errorf("dial control-plane-service: %w", err)
	}
	defer controlConn.Close()
	adminClient := adminv1.NewAdminServiceClient(adminConn)
	controlClient := controlv1.NewControlPlaneServiceClient(controlConn)
	return runConfigPublishWorkflow(ctx, cfg, adminClient, controlClient, summary)
}

func dialAdmin(ctx context.Context, cfg config) (*grpc.ClientConn, error) {
	_ = ctx
	dialOption, err := grpctls.DialOption(cfg.tls, "admin-tls")
	if err != nil {
		return nil, err
	}
	conn, err := grpc.NewClient("passthrough:///"+cfg.target, dialOption)
	if err != nil {
		return nil, fmt.Errorf("dial admin-service: %w", err)
	}
	return conn, nil
}

func runConfigPublishWorkflow(
	ctx context.Context,
	cfg config,
	adminClient adminv1.AdminServiceClient,
	controlClient controlv1.ControlPlaneServiceClient,
	summary *configPublishSmokeSummary,
) error {
	version := "quota-" + sanitizeRunName(cfg.runName)
	payload := configPublishOperationPayload(version)
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	create, err := adminClient.CreateAdminOperation(requestCtx, &adminv1.CreateAdminOperationRequest{
		AuthContext:          smokeAdminAuth(cfg, "create"),
		OperatorRef:          "operator:admin-smoke",
		OperatorRole:         "ADMIN",
		OperationType:        "CONFIG_PUBLISH",
		TargetRefHash:        "sha256:admin-config-publish-smoke-target",
		RiskLevel:            "MEDIUM",
		PayloadSchemaVersion: "admin.config_publish.v1",
		OperationPayloadJson: payload,
		ReasonRef:            "reason:admin-config-publish-smoke",
		EvidenceRefs:         []string{"evidence:admin-config-publish-smoke"},
		IdempotencyKey:       "admin-config-publish-smoke:create:" + version,
		CorrelationId:        "corr-admin-config-publish-smoke",
		CausationId:          "admin-config-publish-smoke",
		TraceId:              "trace-admin-config-publish-smoke",
	})
	cancel()
	if err != nil {
		return fmt.Errorf("create admin config publish operation: %w", err)
	}
	operation := create.GetOperation()
	if operation == nil {
		return errors.New("create admin operation returned empty operation")
	}
	summary.OperationID = operation.GetOperationId()
	summary.CreatedStatus = operation.GetStatus()
	summary.ReplayedCreate = create.GetReplayed()

	requestCtx, cancel = context.WithTimeout(ctx, cfg.requestTimeout)
	approve, err := adminClient.ApproveAdminOperation(requestCtx, &adminv1.ApproveAdminOperationRequest{
		AuthContext:       smokeAdminAuth(cfg, "approve"),
		OperationId:       summary.OperationID,
		ApproverRef:       "operator:approver-smoke",
		ApproverRole:      "ADMIN",
		Decision:          "APPROVE",
		ApprovalPolicyRef: "admin.config_publish.smoke.v1",
		ReasonRef:         "reason:admin-config-publish-smoke-approval",
		EvidenceRefs:      []string{"evidence:admin-config-publish-smoke-approval"},
		IdempotencyKey:    "admin-config-publish-smoke:approve:" + summary.OperationID,
		CorrelationId:     "corr-admin-config-publish-smoke",
		CausationId:       summary.OperationID,
		TraceId:           "trace-admin-config-publish-smoke",
	})
	cancel()
	if err != nil {
		return fmt.Errorf("approve admin config publish operation: %w", err)
	}
	approved := approve.GetOperation()
	if approved == nil {
		return errors.New("approve admin operation returned empty operation")
	}
	summary.ApprovedStatus = approved.GetStatus()
	summary.ReplayedApproval = approve.GetReplayed()

	final, err := waitForAdminOperationStatus(ctx, cfg, adminClient, summary.OperationID)
	if err != nil {
		return err
	}
	summary.FinalStatus = final.GetStatus()
	if summary.FinalStatus != "SUCCEEDED" {
		return fmt.Errorf("admin operation final status = %s", summary.FinalStatus)
	}
	requestCtx, cancel = context.WithTimeout(ctx, cfg.requestTimeout)
	snapshot, err := controlClient.GetConfigSnapshot(requestCtx, &controlv1.GetConfigSnapshotRequest{
		AuthContext: &controlv1.AuthContext{
			TenantId:    cfg.tenantID,
			ServiceName: "api-gateway",
			InstanceRef: "api-gateway-admin-smoke",
			TraceId:     "trace-admin-config-publish-smoke",
			RequestId:   "request-admin-config-publish-smoke-snapshot",
		},
		Environment:    "local",
		ServiceName:    "api-gateway",
		ConfigKind:     "API_GATEWAY_TENANT_QUOTA",
		BundleKey:      "api-gateway/default",
		InstanceRef:    "api-gateway-admin-smoke",
		ServiceVersion: "local",
	})
	cancel()
	if err != nil {
		return fmt.Errorf("get config snapshot after admin publish: %w", err)
	}
	got := snapshot.GetSnapshot()
	if got == nil {
		return errors.New("config snapshot response is empty")
	}
	summary.SnapshotVersion = got.GetVersion()
	summary.SnapshotChecksum = got.GetPayloadChecksum()
	if summary.SnapshotVersion != version {
		return fmt.Errorf("config snapshot version = %s, want %s", summary.SnapshotVersion, version)
	}
	if strings.TrimSpace(summary.SnapshotChecksum) == "" {
		return errors.New("config snapshot checksum is empty")
	}
	return nil
}

func waitForAdminOperationStatus(
	ctx context.Context,
	cfg config,
	client adminv1.AdminServiceClient,
	operationID string,
) (*adminv1.AdminOperation, error) {
	deadline := time.Now().Add(cfg.pollTimeout)
	for {
		requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
		response, err := client.GetAdminOperation(requestCtx, &adminv1.GetAdminOperationRequest{
			AuthContext: smokeAdminAuth(cfg, "poll"),
			OperationId: operationID,
		})
		cancel()
		if err != nil {
			return nil, fmt.Errorf("get admin operation while polling: %w", err)
		}
		operation := response.GetOperation()
		if operation == nil {
			return nil, errors.New("get admin operation returned empty operation")
		}
		switch operation.GetStatus() {
		case "SUCCEEDED", "FAILED", "CANCELED", "COMPENSATION_REQUESTED":
			return operation, nil
		}
		if time.Now().After(deadline) {
			return operation, fmt.Errorf("timed out waiting for operation %s, last status %s", operationID, operation.GetStatus())
		}
		timer := time.NewTimer(cfg.pollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func configPublishOperationPayload(version string) string {
	payloadJSON := `{"plans":{"tenant-free":{"requests_per_second":20,"burst":40}}}`
	payload := map[string]any{
		"environment":          "local",
		"config_kind":          "API_GATEWAY_TENANT_QUOTA",
		"bundle_key":           "api-gateway/default",
		"version":              version,
		"schema_version":       "quota-v1",
		"payload_json":         payloadJSON,
		"effective_at_unix_ms": time.Now().Add(-time.Minute).UTC().UnixMilli(),
	}
	encoded, _ := json.Marshal(payload)
	return string(encoded)
}

func smokeAdminAuth(cfg config, request string) *adminv1.AuthContext {
	return &adminv1.AuthContext{
		TenantId:    cfg.tenantID,
		UserId:      "admin-smoke",
		ServiceName: "loadtest-admin",
		InstanceRef: "admin-config-publish-smoke",
		TraceId:     "trace-admin-config-publish-smoke",
		RequestId:   "request-admin-config-publish-smoke-" + request,
	}
}

func applySQLMigrations(ctx context.Context, pool *pgxpool.Pool, dir string) error {
	files, err := filepath.Glob(filepath.Join(dir, "*.sql"))
	if err != nil {
		return fmt.Errorf("list migrations %s: %w", dir, err)
	}
	sort.Strings(files)
	for _, file := range files {
		sqlBytes, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", file, err)
		}
		if _, err := pool.Exec(ctx, string(sqlBytes)); err != nil {
			return fmt.Errorf("apply migration %s: %w", file, err)
		}
	}
	return nil
}

func cleanupConfigPublishSmokeTenant(ctx context.Context, pool *pgxpool.Pool, tenantID string) error {
	for _, query := range []string{
		`DELETE FROM admin_outbox WHERE tenant_id = $1`,
		`DELETE FROM admin_operation_results WHERE tenant_id = $1`,
		`DELETE FROM admin_approvals WHERE tenant_id = $1`,
		`DELETE FROM admin_operations WHERE tenant_id = $1`,
		`DELETE FROM control_outbox WHERE tenant_id = $1`,
		`DELETE FROM control_applied_acks WHERE tenant_id = $1`,
		`DELETE FROM control_rollout_rules WHERE tenant_id = $1`,
		`DELETE FROM control_config_versions WHERE tenant_id = $1`,
		`DELETE FROM control_config_bundles WHERE tenant_id = $1`,
	} {
		if _, err := pool.Exec(ctx, query, tenantID); err != nil {
			return fmt.Errorf("cleanup smoke tenant: %w", err)
		}
	}
	return nil
}

func writeConfigPublishSmokeSummary(cfg config, summary *configPublishSmokeSummary, out *os.File) error {
	resultDir := filepath.Join(cfg.resultRoot, sanitizeRunName(cfg.runName))
	if err := os.MkdirAll(resultDir, 0o755); err != nil {
		return err
	}
	summary.ResultDir = resultDir
	encoded, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(resultDir, "admin-config-publish-smoke-summary.json")
	if err := os.WriteFile(path, encoded, 0o644); err != nil {
		return err
	}
	if _, err := out.Write(encoded); err != nil {
		return err
	}
	if _, err := out.Write([]byte("\nsummary: " + path + "\n")); err != nil {
		return err
	}
	return nil
}

func validateExternalResultRoot(root string) error {
	fullRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	repoRoot, err := filepath.Abs(".")
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(repoRoot, fullRoot)
	if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && rel != "." {
		return errors.New("result root must be outside repository")
	}
	return nil
}

func sanitizeRunName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "admin-config-publish-smoke"
	}
	replacer := strings.NewReplacer("\\", "-", "/", "-", ":", "-", "*", "-", "?", "-", "\"", "-", "<", "-", ">", "-", "|", "-", " ", "-")
	return replacer.Replace(value)
}

func gitOutput(args ...string) string {
	cmd := exec.Command("git", args...)
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return string(output)
}
