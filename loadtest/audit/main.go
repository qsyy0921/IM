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
	auditv1 "github.com/qsyy0921/IM/api/proto/nexusim/audit/v1"
	"github.com/qsyy0921/IM/loadtest/internal/grpctls"
	"google.golang.org/grpc"
)

const defaultResultRoot = `H:\NexusIM\loadtest-results`

type config struct {
	auditTarget    string
	auditTLS       grpctls.Config
	pgDSN          string
	resultDir      string
	requestTimeout time.Duration
	tenantID       string
	userID         string
	deviceID       string
	cleanup        bool
	applyMigration bool
}

type summary struct {
	Commit            string        `json:"commit"`
	CommitFull        string        `json:"commit_full"`
	GitDirty          bool          `json:"git_dirty"`
	GitStatusShort    string        `json:"git_status_short,omitempty"`
	ResultDir         string        `json:"result_dir"`
	AuditTarget       string        `json:"audit_target"`
	AuditTLSEnabled   bool          `json:"audit_tls_enabled"`
	TenantID          string        `json:"tenant_id"`
	UserID            string        `json:"user_id"`
	StartedAt         time.Time     `json:"started_at"`
	FinishedAt        time.Time     `json:"finished_at"`
	Success           bool          `json:"success"`
	Error             string        `json:"error,omitempty"`
	FirstAuditID      string        `json:"first_audit_id"`
	SecondAuditID     string        `json:"second_audit_id"`
	ReplaySameAuditID bool          `json:"replay_same_audit_id"`
	QueryCount        int           `json:"query_count"`
	NextCursor        string        `json:"next_cursor,omitempty"`
	Proof             proofSummary  `json:"proof"`
	Outbox            outboxSummary `json:"outbox"`
	OutboxPayloadSafe bool          `json:"outbox_payload_safe"`
}

type proofSummary struct {
	Valid              bool   `json:"valid"`
	FailureReason      string `json:"failure_reason,omitempty"`
	RecordHash         string `json:"record_hash"`
	PreviousRecordHash string `json:"previous_record_hash"`
}

type outboxSummary struct {
	Total     int64 `json:"total"`
	Pending   int64 `json:"pending"`
	Published int64 `json:"published"`
	DLQ       int64 `json:"dlq"`
}

func main() {
	cfg := parseConfig()
	if err := run(cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func parseConfig() config {
	var cfg config
	var resultRoot string
	var runName string
	flag.StringVar(&cfg.auditTarget, "audit-target", envOr("NEXUSIM_AUDIT_GRPC_ADDR", "127.0.0.1:10700"), "audit-service gRPC target")
	registerTLSFlags("audit-tls", "NEXUSIM_AUDIT_TLS", "audit-service", &cfg.auditTLS)
	flag.StringVar(&cfg.pgDSN, "pg-dsn", envOr("NEXUSIM_PG_DSN", "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable"), "PostgreSQL DSN")
	flag.StringVar(&resultRoot, "result-root", defaultResultRoot, "external result root for raw smoke output")
	flag.StringVar(&runName, "run-name", "", "run name under result-root")
	flag.DurationVar(&cfg.requestTimeout, "request-timeout", 3*time.Second, "per request timeout")
	flag.StringVar(&cfg.tenantID, "tenant-id", "", "tenant id; defaults to tenant derived from run name")
	flag.StringVar(&cfg.userID, "user-id", "audit-user-1", "requesting user id")
	flag.StringVar(&cfg.deviceID, "device-id", "audit-device-1", "requesting device id")
	flag.BoolVar(&cfg.cleanup, "cleanup", true, "delete audit rows for the smoke tenant before running")
	flag.BoolVar(&cfg.applyMigration, "apply-migration", true, "apply audit migrations before running")
	flag.Parse()

	if runName == "" {
		runName = "audit-service-grpc-smoke-" + time.Now().Format("20060102-150405")
	}
	safeRunName := sanitizeRunName(runName)
	if cfg.tenantID == "" {
		cfg.tenantID = "tenant-" + safeRunName
	}
	if cfg.requestTimeout <= 0 {
		cfg.requestTimeout = 3 * time.Second
	}
	cfg.resultDir = filepath.Join(resultRoot, runName)
	return cfg
}

func registerTLSFlags(prefix string, envPrefix string, serviceName string, config *grpctls.Config) {
	flag.StringVar(&config.CAFile, prefix+"-ca-file", os.Getenv(envPrefix+"_CA_FILE"), "CA PEM for "+serviceName+" gRPC TLS")
	flag.StringVar(&config.ServerName, prefix+"-server-name", os.Getenv(envPrefix+"_SERVER_NAME"), "override server name for "+serviceName+" gRPC TLS")
	flag.StringVar(&config.ClientCertFile, prefix+"-client-cert-file", os.Getenv(envPrefix+"_CLIENT_CERT_FILE"), "client certificate PEM for "+serviceName+" mTLS")
	flag.StringVar(&config.ClientKeyFile, prefix+"-client-key-file", os.Getenv(envPrefix+"_CLIENT_KEY_FILE"), "client private key PEM for "+serviceName+" mTLS")
}

func run(cfg config) error {
	if err := validateConfig(cfg); err != nil {
		return err
	}
	if err := validateExternalResultDir(cfg.resultDir); err != nil {
		return err
	}
	if err := os.MkdirAll(cfg.resultDir, 0o755); err != nil {
		return fmt.Errorf("create result dir: %w", err)
	}

	result := summary{
		Commit:          gitOutput("rev-parse", "--short", "HEAD"),
		CommitFull:      gitOutput("rev-parse", "HEAD"),
		GitStatusShort:  gitOutput("status", "--short"),
		ResultDir:       cfg.resultDir,
		AuditTarget:     cfg.auditTarget,
		AuditTLSEnabled: cfg.auditTLS.Enabled(),
		TenantID:        cfg.tenantID,
		UserID:          cfg.userID,
		StartedAt:       time.Now().UTC(),
	}
	result.GitDirty = strings.TrimSpace(result.GitStatusShort) != ""
	defer func() {
		result.FinishedAt = time.Now().UTC()
		_ = writeSummary(cfg.resultDir, result)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), cfg.requestTimeout+10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, cfg.pgDSN)
	if err != nil {
		result.Error = "open postgres: " + err.Error()
		return fmt.Errorf("open postgres: %w", err)
	}
	defer pool.Close()
	if cfg.applyMigration {
		if err := applyAuditMigrations(ctx, pool); err != nil {
			result.Error = "apply audit migrations: " + err.Error()
			return err
		}
	}
	if cfg.cleanup {
		if err := cleanupTenant(ctx, pool, cfg.tenantID); err != nil {
			result.Error = "cleanup audit rows: " + err.Error()
			return err
		}
	}

	dialOption, err := grpctls.DialOption(cfg.auditTLS, "audit-tls")
	if err != nil {
		result.Error = "configure audit TLS: " + err.Error()
		return err
	}
	conn, err := grpc.NewClient("passthrough:///"+cfg.auditTarget, dialOption)
	if err != nil {
		result.Error = "dial audit-service: " + err.Error()
		return fmt.Errorf("dial audit-service: %w", err)
	}
	defer conn.Close()
	client := auditv1.NewAuditServiceClient(conn)

	if err := runSmoke(ctx, cfg, pool, client, &result); err != nil {
		result.Error = err.Error()
		return err
	}
	result.Success = true
	return nil
}

func runSmoke(ctx context.Context, cfg config, pool *pgxpool.Pool, client auditv1.AuditServiceClient, result *summary) error {
	auth := &auditv1.AuthContext{
		TenantId:  cfg.tenantID,
		UserId:    cfg.userID,
		DeviceId:  cfg.deviceID,
		SessionId: "audit-smoke-session",
		TraceId:   "trace-audit-grpc-smoke",
		RequestId: "request-audit-append-1",
	}
	firstRequest := buildAppendAuditRecordRequest(cfg, auth, appendSpec{
		SourceService:  "identity-service",
		SourceEventID:  "audit-smoke-event-1",
		RecordType:     "IDENTITY_AUTH",
		Action:         "LOGIN",
		Outcome:        "SUCCEEDED",
		ReasonCode:     "OK",
		RiskLevel:      "LOW",
		AttributesJSON: `{"session_key":"session-smoke-1"}`,
		IdempotencyKey: "audit-smoke-idem-1",
	})
	first, err := appendAuditRecordRequest(ctx, cfg, client, firstRequest)
	if err != nil {
		return err
	}
	result.FirstAuditID = first.GetAuditId()

	replay, err := appendAuditRecordRequest(ctx, cfg, client, firstRequest)
	if err != nil {
		return err
	}
	result.ReplaySameAuditID = replay.GetAuditId() == first.GetAuditId()
	if !result.ReplaySameAuditID {
		return fmt.Errorf("idempotent replay returned %s, want %s", replay.GetAuditId(), first.GetAuditId())
	}

	secondAuth := protoCloneAuth(auth)
	secondAuth.RequestId = "request-audit-append-2"
	second, err := appendAuditRecord(ctx, cfg, client, secondAuth, appendSpec{
		SourceService:  "agent-service",
		SourceEventID:  "audit-smoke-event-2",
		RecordType:     "AGENT_ACTION",
		Action:         "APPROVE",
		Outcome:        "SUCCEEDED",
		ReasonCode:     "APPROVED",
		RiskLevel:      "LOW",
		AttributesJSON: `{"proposal_id":"proposal-smoke-1","approval_id":"approval-smoke-1"}`,
		IdempotencyKey: "audit-smoke-idem-2",
	})
	if err != nil {
		return err
	}
	result.SecondAuditID = second.GetAuditId()

	queryAuth := protoCloneAuth(auth)
	queryAuth.RequestId = "request-audit-query"
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	query, err := client.QueryAuditRecords(requestCtx, &auditv1.QueryAuditRecordsRequest{
		AuthContext: queryAuth,
		AuditStream: "security",
		Limit:       10,
	})
	cancel()
	if err != nil {
		return fmt.Errorf("query audit records: %w", err)
	}
	result.QueryCount = len(query.GetRecords())
	result.NextCursor = query.GetNextCursor()
	if result.QueryCount != 2 {
		return fmt.Errorf("expected two audit records, got %d", result.QueryCount)
	}

	verifyAuth := protoCloneAuth(auth)
	verifyAuth.RequestId = "request-audit-proof"
	requestCtx, cancel = context.WithTimeout(ctx, cfg.requestTimeout)
	proof, err := client.VerifyAuditProof(requestCtx, &auditv1.VerifyAuditProofRequest{
		AuthContext: verifyAuth,
		AuditId:     second.GetAuditId(),
	})
	cancel()
	if err != nil {
		return fmt.Errorf("verify audit proof: %w", err)
	}
	result.Proof = proofSummary{
		Valid:              proof.GetValid(),
		FailureReason:      proof.GetFailureReason(),
		RecordHash:         proof.GetRecordHash(),
		PreviousRecordHash: proof.GetPreviousRecordHash(),
	}
	if !result.Proof.Valid || result.Proof.PreviousRecordHash != first.GetRecordHash() {
		return fmt.Errorf("unexpected proof result: %+v first_hash=%s", result.Proof, first.GetRecordHash())
	}

	outbox, safe, err := readOutboxSummary(ctx, pool, cfg.tenantID)
	if err != nil {
		return err
	}
	result.Outbox = outbox
	result.OutboxPayloadSafe = safe
	if !safe || outbox.Total != 2 || outbox.DLQ != 0 {
		return fmt.Errorf("unexpected audit outbox result safe=%v summary=%+v", safe, outbox)
	}
	return nil
}

type appendSpec struct {
	SourceService  string
	SourceEventID  string
	RecordType     string
	Action         string
	Outcome        string
	ReasonCode     string
	RiskLevel      string
	AttributesJSON string
	IdempotencyKey string
}

func appendAuditRecord(ctx context.Context, cfg config, client auditv1.AuditServiceClient, auth *auditv1.AuthContext, spec appendSpec) (*auditv1.AuditRecord, error) {
	return appendAuditRecordRequest(ctx, cfg, client, buildAppendAuditRecordRequest(cfg, auth, spec))
}

func buildAppendAuditRecordRequest(cfg config, auth *auditv1.AuthContext, spec appendSpec) *auditv1.AppendAuditRecordRequest {
	return &auditv1.AppendAuditRecordRequest{
		AuthContext:   auth,
		AuditStream:   "security",
		SourceService: spec.SourceService,
		SourceEventId: spec.SourceEventID,
		RecordType:    spec.RecordType,
		ActorRef:      "user:" + cfg.userID,
		ResourceRef:   "audit-smoke-resource",
		Action:        spec.Action,
		Outcome:       spec.Outcome,
		ReasonCode:    spec.ReasonCode,
		RiskLevel:     spec.RiskLevel,
		OccurredAtUnixMs: time.Now().
			UTC().
			Truncate(time.Millisecond).
			UnixMilli(),
		AttributesJson: spec.AttributesJSON,
		IdempotencyKey: spec.IdempotencyKey,
		CorrelationId:  "corr-audit-smoke",
		TraceId:        auth.GetTraceId(),
	}
}

func appendAuditRecordRequest(ctx context.Context, cfg config, client auditv1.AuditServiceClient, request *auditv1.AppendAuditRecordRequest) (*auditv1.AuditRecord, error) {
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	response, err := client.AppendAuditRecord(requestCtx, request)
	if err != nil {
		return nil, fmt.Errorf("append audit record %s: %w", request.GetSourceEventId(), err)
	}
	if response.GetRecord() == nil || strings.TrimSpace(response.GetRecord().GetAuditId()) == "" {
		return nil, errors.New("append audit record returned empty record")
	}
	return response.GetRecord(), nil
}

func protoCloneAuth(auth *auditv1.AuthContext) *auditv1.AuthContext {
	clone := *auth
	return &clone
}

func readOutboxSummary(ctx context.Context, pool *pgxpool.Pool, tenantID string) (outboxSummary, bool, error) {
	rows, err := pool.Query(ctx, `
SELECT status, payload_json::text
FROM audit_outbox
WHERE tenant_id = $1
`, tenantID)
	if err != nil {
		return outboxSummary{}, false, fmt.Errorf("read audit outbox: %w", err)
	}
	defer rows.Close()
	var summary outboxSummary
	safe := true
	for rows.Next() {
		var status string
		var payload string
		if err := rows.Scan(&status, &payload); err != nil {
			return outboxSummary{}, false, fmt.Errorf("scan audit outbox: %w", err)
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
	if err := rows.Err(); err != nil {
		return outboxSummary{}, false, fmt.Errorf("iterate audit outbox: %w", err)
	}
	return summary, safe, nil
}

func payloadLeaksSensitiveValue(payload string) bool {
	lowered := strings.ToLower(payload)
	for _, forbidden := range []string{
		"session-smoke-1",
		"raw_prompt",
		"evidencepack",
		"provider body",
		"password",
		"totp",
		"recovery code",
		"message body",
	} {
		if strings.Contains(lowered, strings.ToLower(forbidden)) {
			return true
		}
	}
	return false
}

func applyAuditMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	dir := filepath.Join("migrations", "postgres", "audit")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read audit migration dir: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".sql" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read audit migration %s: %w", entry.Name(), err)
		}
		if _, err := pool.Exec(ctx, string(content)); err != nil {
			return fmt.Errorf("apply audit migration %s: %w", entry.Name(), err)
		}
	}
	return nil
}

func cleanupTenant(ctx context.Context, pool *pgxpool.Pool, tenantID string) error {
	queries := []string{
		`DELETE FROM audit_outbox WHERE tenant_id = $1`,
		`DELETE FROM audit_hash_segments WHERE tenant_id = $1`,
		`DELETE FROM audit_records WHERE tenant_id = $1`,
	}
	for _, query := range queries {
		if _, err := pool.Exec(ctx, query, tenantID); err != nil {
			return fmt.Errorf("cleanup audit tenant: %w", err)
		}
	}
	return nil
}

func validateConfig(cfg config) error {
	if strings.TrimSpace(cfg.auditTarget) == "" {
		return errors.New("audit-target is required")
	}
	if strings.TrimSpace(cfg.pgDSN) == "" {
		return errors.New("pg-dsn is required")
	}
	if strings.TrimSpace(cfg.tenantID) == "" || strings.TrimSpace(cfg.userID) == "" || strings.TrimSpace(cfg.deviceID) == "" {
		return errors.New("tenant-id, user-id and device-id are required")
	}
	return nil
}

func writeSummary(resultDir string, result summary) error {
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(resultDir, "audit-grpc-summary.json")
	if err := os.WriteFile(path, append(encoded, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Println(string(encoded))
	fmt.Printf("summary: %s\n", path)
	return nil
}

func validateExternalResultDir(resultDir string) error {
	repo := gitOutput("rev-parse", "--show-toplevel")
	if repo == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		repo = cwd
	}
	resultFull, err := filepath.Abs(resultDir)
	if err != nil {
		return err
	}
	repoFull, err := filepath.Abs(repo)
	if err != nil {
		return err
	}
	if pathInside(resultFull, repoFull) {
		return fmt.Errorf("result-dir must not be inside repository; use %s or another external scratch directory", defaultResultRoot)
	}
	return nil
}

func pathInside(path string, root string) bool {
	path = strings.TrimRight(filepath.Clean(path), `\/`)
	root = strings.TrimRight(filepath.Clean(root), `\/`)
	if strings.EqualFold(path, root) {
		return true
	}
	prefix := root + string(os.PathSeparator)
	return strings.HasPrefix(strings.ToLower(path), strings.ToLower(prefix))
}

func sanitizeRunName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "audit-smoke"
	}
	var builder strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '-' || r == '_':
			builder.WriteRune(r)
		default:
			builder.WriteByte('-')
		}
	}
	return strings.Trim(builder.String(), "-")
}

func envOr(name string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}

func gitOutput(args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", args...).CombinedOutput()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
