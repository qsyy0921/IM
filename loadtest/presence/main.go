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
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	presencev1 "github.com/qsyy0921/IM/api/proto/nexusim/presence/v1"
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
	cleanup        bool
	applyMigration bool
}

type summary struct {
	Commit                     string        `json:"commit"`
	CommitFull                 string        `json:"commit_full"`
	GitDirty                   bool          `json:"git_dirty"`
	ResultDir                  string        `json:"result_dir"`
	Target                     string        `json:"target"`
	TenantID                   string        `json:"tenant_id"`
	StartedAt                  time.Time     `json:"started_at"`
	FinishedAt                 time.Time     `json:"finished_at"`
	Success                    bool          `json:"success"`
	Error                      string        `json:"error,omitempty"`
	OnlineVisibleState         string        `json:"online_visible_state"`
	ReplaySameState            bool          `json:"replay_same_state"`
	SelfDeviceCount            int32         `json:"self_device_count"`
	UnauthorizedVisibleState   string        `json:"unauthorized_visible_state"`
	InvisibleVisibleState      string        `json:"invisible_visible_state"`
	InvisibleActualState       string        `json:"invisible_actual_state"`
	TypingState                string        `json:"typing_state"`
	Outbox                     outboxSummary `json:"outbox"`
	OutboxPayloadLowSensitive  bool          `json:"outbox_payload_low_sensitive"`
	ReplayDidNotWriteOutboxRow bool          `json:"replay_did_not_write_outbox_row"`
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
	flag.StringVar(&cfg.target, "target", envOr("NEXUSIM_PRESENCE_GRPC_ADDR", "127.0.0.1:10720"), "presence-service gRPC target")
	flag.StringVar(&cfg.pgDSN, "pg-dsn", envOr("NEXUSIM_PG_DSN", "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable"), "PostgreSQL DSN")
	flag.StringVar(&cfg.resultRoot, "result-root", defaultResultRoot, "external result root")
	flag.StringVar(&cfg.runName, "run-name", "", "run name")
	flag.DurationVar(&cfg.requestTimeout, "request-timeout", 5*time.Second, "request timeout")
	flag.StringVar(&cfg.tenantID, "tenant-id", "", "tenant id")
	flag.BoolVar(&cfg.cleanup, "cleanup", true, "cleanup smoke tenant before running")
	flag.BoolVar(&cfg.applyMigration, "apply-migration", true, "apply presence migration before running")
	flag.Parse()
	if cfg.runName == "" {
		cfg.runName = "presence-grpc-smoke-" + time.Now().UTC().Format("20060102-150405")
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
		return fmt.Errorf("dial presence-service: %w", err)
	}
	defer conn.Close()
	client := presencev1.NewPresenceServiceClient(conn)
	if err := runSmoke(ctx, cfg, pool, client, result); err != nil {
		return err
	}
	result.Success = true
	return nil
}

func runSmoke(ctx context.Context, cfg config, pool *pgxpool.Pool, client presencev1.PresenceServiceClient, result *summary) error {
	auth := &presencev1.AuthContext{
		TenantId:  cfg.tenantID,
		UserId:    "user-1",
		TraceId:   "trace-presence-smoke",
		RequestId: "request-presence-online",
	}
	online := &presencev1.UpdatePresenceRequest{
		AuthContext:    auth,
		UserId:         "user-1",
		DeviceId:       "device-1",
		SessionId:      "session-1",
		PresenceState:  "ONLINE",
		ManualStatus:   "available",
		TtlMs:          int64((5 * time.Minute).Milliseconds()),
		Source:         "CLIENT",
		IdempotencyKey: "presence-smoke-online-1",
		CorrelationId:  "corr-presence-smoke",
		TraceId:        "trace-presence-smoke",
	}
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	first, err := client.UpdatePresence(requestCtx, online)
	cancel()
	if err != nil {
		return fmt.Errorf("update online presence: %w", err)
	}
	result.OnlineVisibleState = first.GetState().GetVisibleState()
	if result.OnlineVisibleState != "ONLINE" || first.GetState().GetDeviceCount() != 1 {
		return fmt.Errorf("unexpected online presence response: %+v", first.GetState())
	}

	outboxBeforeReplay, _, err := readOutboxSummary(ctx, pool, cfg.tenantID)
	if err != nil {
		return err
	}
	requestCtx, cancel = context.WithTimeout(ctx, cfg.requestTimeout)
	replay, err := client.UpdatePresence(requestCtx, online)
	cancel()
	if err != nil {
		return fmt.Errorf("replay online presence: %w", err)
	}
	result.ReplaySameState = replay.GetState().GetActualState() == first.GetState().GetActualState() &&
		replay.GetState().GetVisibleState() == first.GetState().GetVisibleState()
	outboxAfterReplay, _, err := readOutboxSummary(ctx, pool, cfg.tenantID)
	if err != nil {
		return err
	}
	result.ReplayDidNotWriteOutboxRow = outboxBeforeReplay.Total == outboxAfterReplay.Total
	if !result.ReplaySameState || !result.ReplayDidNotWriteOutboxRow {
		return fmt.Errorf("presence replay was not idempotent: same=%v before=%+v after=%+v",
			result.ReplaySameState, outboxBeforeReplay, outboxAfterReplay)
	}

	selfState, err := getPresence(ctx, cfg, client, auth, "user-1", []string{"user-1"}, true)
	if err != nil {
		return err
	}
	result.SelfDeviceCount = selfState.GetDeviceCount()
	if result.SelfDeviceCount != 1 || len(selfState.GetDeviceStates()) != 1 {
		return fmt.Errorf("self presence should include one device: %+v", selfState)
	}
	deniedState, err := getPresence(ctx, cfg, client,
		&presencev1.AuthContext{TenantId: cfg.tenantID, UserId: "user-2", TraceId: "trace-presence-denied"},
		"user-2", []string{"user-1"}, true)
	if err != nil {
		return err
	}
	result.UnauthorizedVisibleState = deniedState.GetVisibleState()
	if result.UnauthorizedVisibleState != "UNKNOWN" || deniedState.GetDeviceCount() != 0 || len(deniedState.GetDeviceStates()) != 0 {
		return fmt.Errorf("unauthorized presence should be masked: %+v", deniedState)
	}

	invisible := *online
	invisible.AuthContext = &presencev1.AuthContext{
		TenantId:  cfg.tenantID,
		UserId:    "user-1",
		TraceId:   "trace-presence-smoke",
		RequestId: "request-presence-invisible",
	}
	invisible.PresenceState = "INVISIBLE"
	invisible.IdempotencyKey = "presence-smoke-invisible-1"
	requestCtx, cancel = context.WithTimeout(ctx, cfg.requestTimeout)
	invisibleResponse, err := client.UpdatePresence(requestCtx, &invisible)
	cancel()
	if err != nil {
		return fmt.Errorf("update invisible presence: %w", err)
	}
	result.InvisibleVisibleState = invisibleResponse.GetState().GetVisibleState()
	result.InvisibleActualState = invisibleResponse.GetState().GetActualState()
	if result.InvisibleActualState != "INVISIBLE" || result.InvisibleVisibleState != "OFFLINE" {
		return fmt.Errorf("unexpected invisible presence response: %+v", invisibleResponse.GetState())
	}

	requestCtx, cancel = context.WithTimeout(ctx, cfg.requestTimeout)
	typing, err := client.UpdateTyping(requestCtx, &presencev1.UpdateTypingRequest{
		AuthContext:    auth,
		ConversationId: "conversation-1",
		UserId:         "user-1",
		DeviceId:       "device-1",
		TypingState:    "STARTED",
		TtlMs:          int64((15 * time.Second).Milliseconds()),
		CorrelationId:  "corr-presence-typing",
		TraceId:        "trace-presence-smoke",
	})
	cancel()
	if err != nil {
		return fmt.Errorf("update typing: %w", err)
	}
	result.TypingState = typing.GetTyping().GetTypingState()
	if result.TypingState != "STARTED" {
		return fmt.Errorf("unexpected typing response: %+v", typing.GetTyping())
	}

	outbox, safe, err := readOutboxSummary(ctx, pool, cfg.tenantID)
	if err != nil {
		return err
	}
	result.Outbox = outbox
	result.OutboxPayloadLowSensitive = safe
	if outbox.Total != 3 || outbox.DLQ != 0 || !safe {
		return fmt.Errorf("unexpected presence outbox summary safe=%v summary=%+v", safe, outbox)
	}
	return nil
}

func getPresence(
	ctx context.Context,
	cfg config,
	client presencev1.PresenceServiceClient,
	auth *presencev1.AuthContext,
	requester string,
	targets []string,
	includeDevices bool,
) (*presencev1.PresenceState, error) {
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	response, err := client.GetPresence(requestCtx, &presencev1.GetPresenceRequest{
		AuthContext:     auth,
		RequesterUserId: requester,
		TargetUserIds:   targets,
		IncludeDevices:  includeDevices,
	})
	if err != nil {
		return nil, fmt.Errorf("get presence: %w", err)
	}
	if len(response.GetStates()) != 1 {
		return nil, fmt.Errorf("unexpected presence state count: %d", len(response.GetStates()))
	}
	return response.GetStates()[0], nil
}

func applyMigration(ctx context.Context, pool *pgxpool.Pool) error {
	sqlBytes, err := os.ReadFile(filepath.Join("migrations", "postgres", "presence", "000001_presence_core.sql"))
	if err != nil {
		return fmt.Errorf("read presence migration: %w", err)
	}
	if _, err := pool.Exec(ctx, string(sqlBytes)); err != nil {
		return fmt.Errorf("apply presence migration: %w", err)
	}
	return nil
}

func cleanupTenant(ctx context.Context, pool *pgxpool.Pool, tenantID string) error {
	for _, query := range []string{
		`DELETE FROM presence_outbox WHERE tenant_id = $1`,
		`DELETE FROM presence_typing_indicators WHERE tenant_id = $1`,
		`DELETE FROM presence_subscriptions WHERE tenant_id = $1`,
		`DELETE FROM presence_sessions WHERE tenant_id = $1`,
		`DELETE FROM presence_user_states WHERE tenant_id = $1`,
	} {
		if _, err := pool.Exec(ctx, query, tenantID); err != nil {
			return fmt.Errorf("cleanup tenant: %w", err)
		}
	}
	return nil
}

func readOutboxSummary(ctx context.Context, pool *pgxpool.Pool, tenantID string) (outboxSummary, bool, error) {
	rows, err := pool.Query(ctx, `SELECT status, payload_json::text, aggregate_id, partition_key FROM presence_outbox WHERE tenant_id = $1`, tenantID)
	if err != nil {
		return outboxSummary{}, false, fmt.Errorf("read presence outbox: %w", err)
	}
	defer rows.Close()
	var summary outboxSummary
	safe := true
	for rows.Next() {
		var status string
		var payload string
		var aggregateID string
		var partitionKey string
		if err := rows.Scan(&status, &payload, &aggregateID, &partitionKey); err != nil {
			return outboxSummary{}, false, fmt.Errorf("scan presence outbox: %w", err)
		}
		summary.Total++
		if payloadLeaksSensitiveValue(payload) || payloadLeaksSensitiveValue(aggregateID) || payloadLeaksSensitiveValue(partitionKey) {
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

func payloadLeaksSensitiveValue(value string) bool {
	normalized := strings.ToLower(value)
	for _, forbidden := range []string{
		"user-1",
		"user-2",
		"device-1",
		"session-1",
		"conversation-1",
		"available",
		"manual_status",
		"token",
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
	path := filepath.Join(resultDir, "presence-grpc-summary.json")
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
		return "presence-smoke"
	}
	replacer := strings.NewReplacer("\\", "-", "/", "-", ":", "-", "*", "-", "?", "-", "\"", "-", "<", "-", ">", "-", "|", "-", " ", "-")
	return replacer.Replace(value)
}

func envOr(name string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
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
