package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	controlv1 "github.com/qsyy0921/IM/api/proto/nexusim/controlplane/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const defaultResultRoot = `H:\NexusIM\loadtest-results`

type config struct {
	target         string
	pgDSN          string
	resultRoot     string
	runName        string
	requestTimeout time.Duration
	tenantID       string
	applyMigration bool
	cleanup        bool
}

type summary struct {
	Commit            string        `json:"commit"`
	CommitFull        string        `json:"commit_full"`
	GitDirty          bool          `json:"git_dirty"`
	ResultDir         string        `json:"result_dir"`
	Target            string        `json:"target"`
	TenantID          string        `json:"tenant_id"`
	StartedAt         time.Time     `json:"started_at"`
	FinishedAt        time.Time     `json:"finished_at"`
	Success           bool          `json:"success"`
	Error             string        `json:"error,omitempty"`
	PublishedVersion  string        `json:"published_version"`
	ReplaySameVersion bool          `json:"replay_same_version"`
	SnapshotVersion   string        `json:"snapshot_version"`
	SnapshotChecksum  string        `json:"snapshot_checksum"`
	AckStatus         string        `json:"ack_status"`
	Outbox            outboxSummary `json:"outbox"`
	OutboxPayloadSafe bool          `json:"outbox_payload_safe"`
}

type outboxSummary struct {
	Total     int `json:"total"`
	Pending   int `json:"pending"`
	Published int `json:"published"`
	DLQ       int `json:"dlq"`
}

func main() {
	cfg := parseFlags()
	result := summary{
		Target:    cfg.target,
		TenantID:  cfg.tenantID,
		StartedAt: time.Now().UTC(),
	}
	result.Commit = strings.TrimSpace(gitOutput("rev-parse", "--short", "HEAD"))
	result.CommitFull = strings.TrimSpace(gitOutput("rev-parse", "HEAD"))
	result.GitDirty = strings.TrimSpace(gitOutput("status", "--short", "--untracked-files=all")) != ""

	err := run(context.Background(), cfg, &result)
	result.FinishedAt = time.Now().UTC()
	if err != nil {
		result.Error = err.Error()
	}
	if writeErr := writeSummary(cfg, result); writeErr != nil {
		fmt.Fprintf(os.Stderr, "write summary: %v\n", writeErr)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func parseFlags() config {
	cfg := config{}
	flag.StringVar(&cfg.target, "target", envOr("NEXUSIM_CONTROL_PLANE_GRPC_ADDR", "127.0.0.1:10710"), "control-plane gRPC target")
	flag.StringVar(&cfg.pgDSN, "pg-dsn", envOr("NEXUSIM_PG_DSN", "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable"), "PostgreSQL DSN")
	flag.StringVar(&cfg.resultRoot, "result-root", defaultResultRoot, "external result root")
	flag.StringVar(&cfg.runName, "run-name", "", "run name")
	flag.DurationVar(&cfg.requestTimeout, "request-timeout", 5*time.Second, "request timeout")
	flag.StringVar(&cfg.tenantID, "tenant-id", "", "tenant id")
	flag.BoolVar(&cfg.applyMigration, "apply-migration", true, "apply control-plane migration before running")
	flag.BoolVar(&cfg.cleanup, "cleanup", true, "cleanup smoke tenant before running")
	flag.Parse()
	if cfg.runName == "" {
		cfg.runName = "control-plane-grpc-smoke-" + time.Now().UTC().Format("20060102-150405")
	}
	if cfg.tenantID == "" {
		cfg.tenantID = "tenant-" + sanitizeRunName(cfg.runName)
	}
	return cfg
}

func run(ctx context.Context, cfg config, result *summary) error {
	if err := validateExternalResultRoot(cfg.resultRoot); err != nil {
		return err
	}
	pool, err := pgxpool.New(ctx, cfg.pgDSN)
	if err != nil {
		return fmt.Errorf("open pg pool: %w", err)
	}
	defer pool.Close()
	if cfg.applyMigration {
		if err := applyMigration(ctx, pool); err != nil {
			return err
		}
	}
	if cfg.cleanup {
		if err := cleanupTenant(ctx, pool, cfg.tenantID); err != nil {
			return err
		}
	}
	conn, err := grpc.NewClient("passthrough:///"+cfg.target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("dial control-plane-service: %w", err)
	}
	defer conn.Close()
	client := controlv1.NewControlPlaneServiceClient(conn)
	if err := runSmoke(ctx, cfg, pool, client, result); err != nil {
		return err
	}
	result.Success = true
	return nil
}

func runSmoke(ctx context.Context, cfg config, pool *pgxpool.Pool, client controlv1.ControlPlaneServiceClient, result *summary) error {
	auth := &controlv1.AuthContext{
		TenantId:  cfg.tenantID,
		UserId:    "operator-1",
		TraceId:   "trace-control-plane-smoke",
		RequestId: "request-control-publish",
	}
	publish := &controlv1.PublishConfigVersionRequest{
		AuthContext:   auth,
		Environment:   "local",
		ConfigKind:    "API_GATEWAY_TENANT_QUOTA",
		BundleKey:     "api-gateway/default",
		Version:       "quota-v1.smoke",
		SchemaVersion: "quota-v1",
		PayloadJson:   `{"plans":{"tenant-free":{"requests_per_second":20,"burst":40}}}`,
		EffectiveAtUnixMs: time.Now().
			Add(-time.Minute).
			UTC().
			UnixMilli(),
		OperatorRef:    "operator:smoke",
		ApprovalRef:    "approval:smoke",
		IdempotencyKey: "control-plane-smoke-idem-1",
		CorrelationId:  "corr-control-smoke",
		TraceId:        "trace-control-plane-smoke",
	}
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	first, err := client.PublishConfigVersion(requestCtx, publish)
	cancel()
	if err != nil {
		return fmt.Errorf("publish config version: %w", err)
	}
	result.PublishedVersion = first.GetVersion().GetVersion()
	result.SnapshotChecksum = first.GetSnapshot().GetPayloadChecksum()

	requestCtx, cancel = context.WithTimeout(ctx, cfg.requestTimeout)
	replay, err := client.PublishConfigVersion(requestCtx, publish)
	cancel()
	if err != nil {
		return fmt.Errorf("publish replay: %w", err)
	}
	result.ReplaySameVersion = replay.GetVersion().GetVersion() == first.GetVersion().GetVersion()
	if !result.ReplaySameVersion {
		return fmt.Errorf("replay returned %s, want %s", replay.GetVersion().GetVersion(), first.GetVersion().GetVersion())
	}

	requestCtx, cancel = context.WithTimeout(ctx, cfg.requestTimeout)
	snapshot, err := client.GetConfigSnapshot(requestCtx, &controlv1.GetConfigSnapshotRequest{
		AuthContext:    &controlv1.AuthContext{TenantId: cfg.tenantID, ServiceName: "api-gateway", InstanceRef: "api-gateway-1"},
		Environment:    "local",
		ServiceName:    "api-gateway",
		ConfigKind:     "API_GATEWAY_TENANT_QUOTA",
		BundleKey:      "api-gateway/default",
		InstanceRef:    "api-gateway-1",
		ServiceVersion: "local",
	})
	cancel()
	if err != nil {
		return fmt.Errorf("get config snapshot: %w", err)
	}
	result.SnapshotVersion = snapshot.GetSnapshot().GetVersion()
	if result.SnapshotVersion != result.PublishedVersion || snapshot.GetSnapshot().GetPayloadChecksum() != result.SnapshotChecksum {
		return fmt.Errorf("unexpected snapshot: %+v", snapshot.GetSnapshot())
	}

	requestCtx, cancel = context.WithTimeout(ctx, cfg.requestTimeout)
	ack, err := client.AckAppliedConfigVersion(requestCtx, &controlv1.AckAppliedConfigVersionRequest{
		AuthContext:    &controlv1.AuthContext{TenantId: cfg.tenantID, ServiceName: "api-gateway", InstanceRef: "api-gateway-1"},
		Environment:    "local",
		ServiceName:    "api-gateway",
		InstanceRef:    "api-gateway-1",
		ConfigKind:     "API_GATEWAY_TENANT_QUOTA",
		BundleKey:      "api-gateway/default",
		Version:        result.PublishedVersion,
		ServiceVersion: "local",
		Status:         "IN_SYNC",
		CorrelationId:  "corr-control-smoke",
		TraceId:        "trace-control-plane-smoke",
	})
	cancel()
	if err != nil {
		return fmt.Errorf("ack applied config version: %w", err)
	}
	result.AckStatus = ack.GetApplied().GetStatus()
	outbox, safe, err := readOutboxSummary(ctx, pool, cfg.tenantID)
	if err != nil {
		return err
	}
	result.Outbox = outbox
	result.OutboxPayloadSafe = safe
	if outbox.Total != 2 || outbox.DLQ != 0 || !safe {
		return fmt.Errorf("unexpected outbox summary safe=%v summary=%+v", safe, outbox)
	}
	return nil
}

func applyMigration(ctx context.Context, pool *pgxpool.Pool) error {
	files, err := filepath.Glob(filepath.Join("migrations", "postgres", "control-plane", "*.sql"))
	if err != nil {
		return fmt.Errorf("list control-plane migrations: %w", err)
	}
	sort.Strings(files)
	for _, file := range files {
		sqlBytes, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("read control-plane migration %s: %w", file, err)
		}
		if _, err := pool.Exec(ctx, string(sqlBytes)); err != nil {
			return fmt.Errorf("apply control-plane migration %s: %w", file, err)
		}
	}
	return nil
}

func cleanupTenant(ctx context.Context, pool *pgxpool.Pool, tenantID string) error {
	for _, query := range []string{
		`DELETE FROM control_outbox WHERE tenant_id = $1`,
		`DELETE FROM control_applied_acks WHERE tenant_id = $1`,
		`DELETE FROM control_config_rollbacks WHERE tenant_id = $1`,
		`DELETE FROM control_rollout_rules WHERE tenant_id = $1`,
		`DELETE FROM control_config_versions WHERE tenant_id = $1`,
		`DELETE FROM control_config_bundles WHERE tenant_id = $1`,
	} {
		if _, err := pool.Exec(ctx, query, tenantID); err != nil {
			return fmt.Errorf("cleanup tenant: %w", err)
		}
	}
	return nil
}

func readOutboxSummary(ctx context.Context, pool *pgxpool.Pool, tenantID string) (outboxSummary, bool, error) {
	rows, err := pool.Query(ctx, `SELECT status, payload_json::text FROM control_outbox WHERE tenant_id = $1`, tenantID)
	if err != nil {
		return outboxSummary{}, false, fmt.Errorf("read control outbox: %w", err)
	}
	defer rows.Close()
	var summary outboxSummary
	safe := true
	for rows.Next() {
		var status string
		var payload string
		if err := rows.Scan(&status, &payload); err != nil {
			return outboxSummary{}, false, fmt.Errorf("scan control outbox: %w", err)
		}
		summary.Total++
		if payloadLeaksSensitiveValue(payload) {
			safe = false
		}
		switch status {
		case "PENDING":
			summary.Pending++
		case "PUBLISHED":
			summary.Published++
		case "DLQ":
			summary.DLQ++
		}
	}
	return summary, safe, rows.Err()
}

func payloadLeaksSensitiveValue(payload string) bool {
	normalized := strings.ToLower(payload)
	for _, forbidden := range []string{
		"requests_per_second",
		"provider_token",
		"payload_json",
		"secret",
		"password",
		"private_key",
		"dsn",
	} {
		if strings.Contains(normalized, forbidden) {
			return true
		}
	}
	return false
}

func writeSummary(cfg config, result summary) error {
	resultDir := filepath.Join(cfg.resultRoot, sanitizeRunName(cfg.runName))
	if err := os.MkdirAll(resultDir, 0o755); err != nil {
		return err
	}
	result.ResultDir = resultDir
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(resultDir, "control-plane-grpc-summary.json")
	if err := os.WriteFile(path, encoded, 0o644); err != nil {
		return err
	}
	fmt.Println(string(encoded))
	fmt.Printf("summary: %s\n", path)
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
		return "control-plane-smoke"
	}
	replacer := strings.NewReplacer("\\", "-", "/", "-", ":", "-", "*", "-", "?", "-", "\"", "-", "<", "-", ">", "-", "|", "-", " ", "-")
	return replacer.Replace(value)
}

func envOr(name string, defaultValue string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return defaultValue
	}
	return value
}

func gitOutput(args ...string) string {
	cmd := exec.Command("git", args...)
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return string(output)
}
