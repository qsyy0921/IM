package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	modelv1 "github.com/qsyy0921/IM/api/proto/nexusim/model/v1"
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
	Commit                  string        `json:"commit"`
	CommitFull              string        `json:"commit_full"`
	GitDirty                bool          `json:"git_dirty"`
	ResultDir               string        `json:"result_dir"`
	Target                  string        `json:"target"`
	TenantID                string        `json:"tenant_id"`
	StartedAt               time.Time     `json:"started_at"`
	FinishedAt              time.Time     `json:"finished_at"`
	Success                 bool          `json:"success"`
	Error                   string        `json:"error,omitempty"`
	InvocationID            string        `json:"invocation_id"`
	ProviderID              string        `json:"provider_id"`
	ModelID                 string        `json:"model_id"`
	OutputReturned          bool          `json:"output_returned"`
	OutputHash              string        `json:"output_hash"`
	ReplaySameInvocation    bool          `json:"replay_same_invocation"`
	ReplayOutputReturned    bool          `json:"replay_output_returned"`
	GetStatus               string        `json:"get_status"`
	InputTokens             int32         `json:"input_tokens"`
	OutputTokens            int32         `json:"output_tokens"`
	EstimatedCostMicrounits int64         `json:"estimated_cost_microunits"`
	Outbox                  outboxSummary `json:"outbox"`
	DBPayloadLowSensitive   bool          `json:"db_payload_low_sensitive"`
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
	flag.StringVar(&cfg.target, "target", envOr("NEXUSIM_MODEL_GATEWAY_GRPC_ADDR", "127.0.0.1:10730"), "model-gateway gRPC target")
	flag.StringVar(&cfg.pgDSN, "pg-dsn", envOr("NEXUSIM_PG_DSN", "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable"), "PostgreSQL DSN")
	flag.StringVar(&cfg.resultRoot, "result-root", defaultResultRoot, "external result root")
	flag.StringVar(&cfg.runName, "run-name", "", "run name")
	flag.DurationVar(&cfg.requestTimeout, "request-timeout", 5*time.Second, "request timeout")
	flag.StringVar(&cfg.tenantID, "tenant-id", "", "tenant id")
	flag.BoolVar(&cfg.applyMigration, "apply-migration", true, "apply model-gateway migration before running")
	flag.BoolVar(&cfg.cleanup, "cleanup", true, "cleanup smoke tenant before running")
	flag.Parse()
	if cfg.runName == "" {
		cfg.runName = "model-gateway-grpc-smoke-" + time.Now().UTC().Format("20060102-150405")
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
		return fmt.Errorf("dial model-gateway: %w", err)
	}
	defer conn.Close()
	client := modelv1.NewModelGatewayServiceClient(conn)
	if err := runSmoke(ctx, cfg, pool, client, result); err != nil {
		return err
	}
	result.Success = true
	return nil
}

func runSmoke(ctx context.Context, cfg config, pool *pgxpool.Pool, client modelv1.ModelGatewayServiceClient, result *summary) error {
	rawPrompt := "model gateway smoke prompt should not persist"
	promptHash := hashRef(rawPrompt)
	request := &modelv1.InvokeTextGenerationRequest{
		AuthContext: &modelv1.AuthContext{
			TenantId:    cfg.tenantID,
			ServiceName: "rag-service",
			TraceId:     "trace-model-smoke",
			RequestId:   "request-model-smoke",
		},
		CallerService:       "rag-service",
		CallerUseCase:       "answer-question",
		RequestId:           "request-model-smoke",
		IdempotencyKey:      "model-gateway-smoke-idem-1",
		ModelClass:          "TEXT_GENERATION",
		PreferredModel:      "deterministic-text-v1",
		RoutePolicy:         "LOCAL_MOCK",
		DataClass:           "BUSINESS_INTERNAL",
		SafetyPolicy:        "DEFAULT",
		PromptParts:         []*modelv1.PromptPart{{Role: "USER", Content: rawPrompt, ContentHash: promptHash}},
		PromptHash:          promptHash,
		PromptSchemaVersion: 1,
		EvidencePackRef:     "epack:model-smoke",
		CitationRequired:    true,
		MaxOutputTokens:     128,
		Temperature:         0,
		TimeoutMs:           int64((5 * time.Second).Milliseconds()),
		CorrelationId:       "corr-model-smoke",
		TraceId:             "trace-model-smoke",
	}
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	first, err := client.InvokeTextGeneration(requestCtx, request)
	cancel()
	if err != nil {
		return fmt.Errorf("invoke text generation: %w", err)
	}
	if first.GetInvocationId() == "" || !first.GetOutputReturned() || first.GetOutputText() == "" {
		return fmt.Errorf("unexpected first response: %+v", first)
	}
	result.InvocationID = first.GetInvocationId()
	result.ProviderID = first.GetProviderId()
	result.ModelID = first.GetModelId()
	result.OutputReturned = first.GetOutputReturned()
	result.OutputHash = first.GetOutputHash()
	result.InputTokens = first.GetTokenUsage().GetInputTokens()
	result.OutputTokens = first.GetTokenUsage().GetOutputTokens()
	result.EstimatedCostMicrounits = first.GetEstimatedCostMicrounits()

	requestCtx, cancel = context.WithTimeout(ctx, cfg.requestTimeout)
	replay, err := client.InvokeTextGeneration(requestCtx, request)
	cancel()
	if err != nil {
		return fmt.Errorf("replay text generation: %w", err)
	}
	result.ReplaySameInvocation = replay.GetInvocationId() == first.GetInvocationId() && replay.GetReplayed()
	result.ReplayOutputReturned = replay.GetOutputReturned()
	if !result.ReplaySameInvocation || result.ReplayOutputReturned {
		return fmt.Errorf("unexpected replay response: %+v", replay)
	}

	requestCtx, cancel = context.WithTimeout(ctx, cfg.requestTimeout)
	loaded, err := client.GetModelInvocation(requestCtx, &modelv1.GetModelInvocationRequest{
		AuthContext:  request.GetAuthContext(),
		InvocationId: first.GetInvocationId(),
	})
	cancel()
	if err != nil {
		return fmt.Errorf("get model invocation: %w", err)
	}
	result.GetStatus = loaded.GetInvocation().GetStatus()
	if result.GetStatus != "SUCCEEDED" || loaded.GetInvocation().GetPromptHash() != promptHash {
		return fmt.Errorf("unexpected loaded invocation: %+v", loaded.GetInvocation())
	}
	outbox, err := readOutboxSummary(ctx, pool, cfg.tenantID)
	if err != nil {
		return err
	}
	result.Outbox = outbox
	lowSensitive, err := checkDBPayloadLowSensitive(ctx, pool, cfg.tenantID, rawPrompt, first.GetOutputText())
	if err != nil {
		return err
	}
	result.DBPayloadLowSensitive = lowSensitive
	if !lowSensitive {
		return errors.New("model-gateway DB payload leaked raw prompt or output")
	}
	return nil
}

func applyMigration(ctx context.Context, pool *pgxpool.Pool) error {
	content, err := os.ReadFile(filepath.Join("migrations", "postgres", "model-gateway", "000001_model_gateway_core.sql"))
	if err != nil {
		return fmt.Errorf("read model migration: %w", err)
	}
	if _, err := pool.Exec(ctx, string(content)); err != nil {
		return fmt.Errorf("apply model migration: %w", err)
	}
	return nil
}

func cleanupTenant(ctx context.Context, pool *pgxpool.Pool, tenantID string) error {
	for _, statement := range []string{
		`DELETE FROM model_outbox WHERE tenant_id = $1`,
		`DELETE FROM model_provider_failures WHERE tenant_id = $1`,
		`DELETE FROM model_provider_route_snapshots WHERE tenant_id = $1`,
		`DELETE FROM model_budget_windows WHERE tenant_id = $1`,
		`DELETE FROM model_invocations WHERE tenant_id = $1`,
	} {
		if _, err := pool.Exec(ctx, statement, tenantID); err != nil {
			return fmt.Errorf("cleanup model tenant: %w", err)
		}
	}
	return nil
}

func readOutboxSummary(ctx context.Context, pool *pgxpool.Pool, tenantID string) (outboxSummary, error) {
	var summary outboxSummary
	err := pool.QueryRow(ctx, `
SELECT
    count(*),
    count(*) FILTER (WHERE status = 'PENDING'),
    count(*) FILTER (WHERE status = 'PUBLISHED'),
    count(*) FILTER (WHERE status = 'DLQ')
FROM model_outbox
WHERE tenant_id = $1
`, tenantID).Scan(&summary.Total, &summary.Pending, &summary.Published, &summary.DLQ)
	if err != nil {
		return outboxSummary{}, fmt.Errorf("read model outbox summary: %w", err)
	}
	return summary, nil
}

func checkDBPayloadLowSensitive(ctx context.Context, pool *pgxpool.Pool, tenantID string, rawPrompt string, rawOutput string) (bool, error) {
	rows, err := pool.Query(ctx, `
SELECT row_to_json(mi)::text
FROM model_invocations mi
WHERE tenant_id = $1
UNION ALL
SELECT payload_json::text
FROM model_outbox
WHERE tenant_id = $1
`, tenantID)
	if err != nil {
		return false, fmt.Errorf("read model payload text: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return false, fmt.Errorf("scan model payload text: %w", err)
		}
		for _, forbidden := range []string{rawPrompt, rawOutput, "secret", "password", "api_key", "private_key"} {
			if forbidden != "" && strings.Contains(payload, forbidden) {
				return false, nil
			}
		}
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("iterate model payload text: %w", err)
	}
	return true, nil
}

func writeSummary(cfg config, result summary) error {
	resultDir := filepath.Join(cfg.resultRoot, cfg.runName)
	if err := os.MkdirAll(resultDir, 0o755); err != nil {
		return err
	}
	result.ResultDir = resultDir
	content, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(resultDir, "summary.json"), append(content, '\n'), 0o644)
}

func validateExternalResultRoot(root string) error {
	if strings.TrimSpace(root) == "" {
		return errors.New("result root is required")
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	absRepo, err := filepath.Abs(".")
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(absRepo, absRoot)
	if err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
		return fmt.Errorf("result root must be outside repository: %s", absRoot)
	}
	return nil
}

func hashRef(value string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func sanitizeRunName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			builder.WriteRune(r)
		} else {
			builder.WriteByte('-')
		}
	}
	return strings.Trim(builder.String(), "-")
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
