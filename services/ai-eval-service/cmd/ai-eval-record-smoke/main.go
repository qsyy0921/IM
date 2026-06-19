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
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	aievalv1 "github.com/qsyy0921/IM/api/proto/nexusim/aieval/v1"
	aievalgrpc "github.com/qsyy0921/IM/services/ai-eval-service/internal/api/grpc"
	"github.com/qsyy0921/IM/services/ai-eval-service/internal/app"
	postgresinfra "github.com/qsyy0921/IM/services/ai-eval-service/internal/infrastructure/postgres"
	grpcgo "google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type config struct {
	summaryPath    string
	outputPath     string
	pgDSN          string
	tenantID       string
	userID         string
	deviceID       string
	sessionID      string
	runID          string
	suiteID        string
	stage          string
	adapter        string
	reportRef      string
	applyMigration bool
	timeout        time.Duration
}

type smokeSummary struct {
	SchemaVersion   int    `json:"schema_version"`
	Status          string `json:"status"`
	Scope           string `json:"scope"`
	SummaryRef      string `json:"summary_ref"`
	RunID           string `json:"run_id"`
	SuiteID         string `json:"suite_id"`
	Stage           string `json:"stage"`
	Adapter         string `json:"adapter"`
	EvalRunStatus   string `json:"eval_run_status"`
	CaseCount       int32  `json:"case_count"`
	PassedCount     int32  `json:"passed_count"`
	FailedCount     int32  `json:"failed_count"`
	SkippedCount    int32  `json:"skipped_count"`
	GetRunMatched   bool   `json:"get_run_matched"`
	ListContainsRun bool   `json:"list_contains_run"`
}

func main() {
	ctx := context.Background()
	if err := run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	cfg, err := parseFlags(args)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, cfg.timeout)
	defer cancel()

	summaryBytes, summary, err := loadSummary(cfg.summaryPath)
	if err != nil {
		return err
	}
	run, err := buildEvalRunFromSummary(cfg, summaryBytes, summary)
	if err != nil {
		return err
	}

	pool, err := openPool(ctx, cfg.pgDSN)
	if err != nil {
		return err
	}
	defer pool.Close()
	if cfg.applyMigration {
		if err := applyMigration(ctx, pool); err != nil {
			return err
		}
	}

	client, stop, err := startInProcessClient(ctx, pool)
	if err != nil {
		return err
	}
	defer stop()

	auth := &aievalv1.AuthContext{
		TenantId:  cfg.tenantID,
		UserId:    cfg.userID,
		DeviceId:  cfg.deviceID,
		SessionId: cfg.sessionID,
		RequestId: "ai-eval-record-smoke",
	}
	recorded, err := client.RecordEvalRun(ctx, &aievalv1.RecordEvalRunRequest{
		AuthContext: auth,
		Run:         run,
	})
	if err != nil {
		return fmt.Errorf("record eval run: %w", err)
	}
	got, err := client.GetEvalRun(ctx, &aievalv1.GetEvalRunRequest{
		AuthContext: auth,
		RunId:       recorded.GetRun().GetRunId(),
	})
	if err != nil {
		return fmt.Errorf("get eval run: %w", err)
	}
	listed, err := client.ListEvalRuns(ctx, &aievalv1.ListEvalRunsRequest{
		AuthContext: auth,
		SuiteId:     recorded.GetRun().GetSuiteId(),
		Status:      recorded.GetRun().GetStatus(),
		Limit:       20,
	})
	if err != nil {
		return fmt.Errorf("list eval runs: %w", err)
	}

	result := smokeSummary{
		SchemaVersion:   1,
		Status:          "passed",
		Scope:           "first-stage ai-eval-service RecordEvalRun smoke; records low-sensitive eval run summary refs only",
		SummaryRef:      recorded.GetRun().GetSummaryRef(),
		RunID:           recorded.GetRun().GetRunId(),
		SuiteID:         recorded.GetRun().GetSuiteId(),
		Stage:           recorded.GetRun().GetStage(),
		Adapter:         recorded.GetRun().GetAdapter(),
		EvalRunStatus:   evalRunStatusName(recorded.GetRun().GetStatus()),
		CaseCount:       recorded.GetRun().GetCaseCount(),
		PassedCount:     recorded.GetRun().GetPassedCount(),
		FailedCount:     recorded.GetRun().GetFailedCount(),
		SkippedCount:    recorded.GetRun().GetSkippedCount(),
		GetRunMatched:   got.GetRun().GetRunId() == recorded.GetRun().GetRunId(),
		ListContainsRun: containsRun(listed.GetRuns(), recorded.GetRun().GetRunId()),
	}
	if !result.GetRunMatched || !result.ListContainsRun {
		return errors.New("recorded eval run was not readable through get/list")
	}
	return writeSmokeSummary(cfg.outputPath, result)
}

func parseFlags(args []string) (config, error) {
	flags := flag.NewFlagSet("ai-eval-record-smoke", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)

	cfg := config{}
	flags.StringVar(&cfg.summaryPath, "summary", "", "Path to a low-sensitive AI eval summary JSON")
	flags.StringVar(&cfg.outputPath, "output", "", "Optional low-sensitive smoke summary output JSON")
	flags.StringVar(&cfg.pgDSN, "pg-dsn", strings.TrimSpace(os.Getenv("NEXUSIM_PG_DSN")), "PostgreSQL DSN")
	flags.StringVar(&cfg.tenantID, "tenant-id", "nexusim-local", "Smoke tenant id")
	flags.StringVar(&cfg.userID, "user-id", "ai-eval-smoke", "Smoke user id")
	flags.StringVar(&cfg.deviceID, "device-id", "ai-eval-smoke-device", "Smoke device id")
	flags.StringVar(&cfg.sessionID, "session-id", "ai-eval-smoke-session", "Smoke session id")
	flags.StringVar(&cfg.runID, "run-id", "", "Optional run id override")
	flags.StringVar(&cfg.suiteID, "suite-id", "", "Optional suite id override")
	flags.StringVar(&cfg.stage, "stage", "", "Optional stage override")
	flags.StringVar(&cfg.adapter, "adapter", "", "Optional adapter override")
	flags.StringVar(&cfg.reportRef, "report-ref", "", "Optional report reference")
	flags.BoolVar(&cfg.applyMigration, "apply-migration", true, "Apply ai-eval-service migration before recording")
	timeout := flags.Duration("timeout", 15*time.Second, "Smoke timeout")
	if err := flags.Parse(args); err != nil {
		return config{}, err
	}
	cfg.timeout = *timeout
	if strings.TrimSpace(cfg.summaryPath) == "" {
		return config{}, errors.New("-summary is required")
	}
	if strings.TrimSpace(cfg.pgDSN) == "" {
		return config{}, errors.New("-pg-dsn or NEXUSIM_PG_DSN is required")
	}
	if strings.TrimSpace(cfg.tenantID) == "" || strings.TrimSpace(cfg.userID) == "" || strings.TrimSpace(cfg.deviceID) == "" {
		return config{}, errors.New("tenant-id, user-id and device-id are required")
	}
	return cfg, nil
}

func loadSummary(path string) ([]byte, map[string]any, error) {
	resolved, err := filepath.Abs(path)
	if err != nil {
		return nil, nil, err
	}
	content, err := os.ReadFile(resolved)
	if err != nil {
		return nil, nil, fmt.Errorf("read summary: %w", err)
	}
	content = bytes.TrimPrefix(content, []byte{0xEF, 0xBB, 0xBF})
	var decoded map[string]any
	if err := json.Unmarshal(content, &decoded); err != nil {
		return nil, nil, fmt.Errorf("decode summary json: %w", err)
	}
	return content, decoded, nil
}

func buildEvalRunFromSummary(cfg config, summaryBytes []byte, summary map[string]any) (*aievalv1.EvalRun, error) {
	summaryRef, err := filepath.Abs(cfg.summaryPath)
	if err != nil {
		return nil, err
	}
	adapter := firstNonEmpty(cfg.adapter, stringValue(summary, "adapter"), strings.TrimSuffix(filepath.Base(summaryRef), filepath.Ext(summaryRef)))
	runName := firstNonEmpty(stringValue(summary, "run_name"), stringValue(summary, "run_id"))
	runID := firstNonEmpty(cfg.runID, sanitizeID(runName), sanitizeID(strings.TrimSuffix(filepath.Base(summaryRef), filepath.Ext(summaryRef))))
	if runID == "" {
		runID = "ai-eval-run-" + time.Now().UTC().Format("20060102-150405")
	}
	stage := firstNonEmpty(cfg.stage, stringValue(summary, "stage"), firstCaseString(summary, "stage"), adapter)
	suiteID := firstNonEmpty(cfg.suiteID, stringValue(summary, "suite_id"), stringValue(summary, "suite"), "ai-eval-"+sanitizeID(adapter))
	reportRef := firstNonEmpty(cfg.reportRef, stringValue(summary, "report_ref"), stringValue(summary, "report_path"))

	caseCount, passedCount, failedCount, skippedCount := countsFromSummary(summary)
	status := statusFromSummary(summary, caseCount, passedCount, failedCount, skippedCount)
	metadata, err := metadataJSON(summary, summaryBytes, summaryRef)
	if err != nil {
		return nil, err
	}

	return &aievalv1.EvalRun{
		TenantId:     cfg.tenantID,
		RunId:        runID,
		SuiteId:      suiteID,
		Stage:        stage,
		Adapter:      adapter,
		Status:       status,
		CaseCount:    int32(caseCount),
		PassedCount:  int32(passedCount),
		FailedCount:  int32(failedCount),
		SkippedCount: int32(skippedCount),
		SummaryRef:   summaryRef,
		ReportRef:    reportRef,
		MetadataJson: metadata,
	}, nil
}

func countsFromSummary(summary map[string]any) (int, int, int, int) {
	caseCount, hasCaseCount := intValue(summary, "case_count")
	passedCount, hasPassed := intValue(summary, "passed_count")
	failedCount, hasFailed := intValue(summary, "failed_count")
	skippedCount, hasSkipped := intValue(summary, "skipped_count")

	cases, _ := summary["cases"].([]any)
	if !hasCaseCount && len(cases) > 0 {
		caseCount = len(cases)
	}
	if !hasPassed && !hasFailed && !hasSkipped && len(cases) > 0 {
		for _, item := range cases {
			caseMap, ok := item.(map[string]any)
			if !ok {
				continue
			}
			status := strings.ToLower(strings.TrimSpace(stringValue(caseMap, "status")))
			switch {
			case boolValue(caseMap, "passed") || status == "passed" || status == "succeeded" || status == "ok":
				passedCount++
			case status == "skipped":
				skippedCount++
			case status == "failed" || status == "error" || status == "errored":
				failedCount++
			}
		}
	}
	if caseCount > 0 && passedCount+failedCount+skippedCount == 0 {
		status := strings.ToLower(strings.TrimSpace(stringValue(summary, "status")))
		switch {
		case status == "passed" || status == "pass" || status == "ok" || status == "succeeded":
			passedCount = caseCount
		case status == "failed" || status == "fail" || status == "error":
			failedCount = caseCount
		}
	}
	return caseCount, passedCount, failedCount, skippedCount
}

func statusFromSummary(summary map[string]any, caseCount, passedCount, failedCount, skippedCount int) aievalv1.EvalRunStatus {
	status := strings.ToLower(strings.TrimSpace(stringValue(summary, "status")))
	switch {
	case status == "running":
		return aievalv1.EvalRunStatus_EVAL_RUN_STATUS_RUNNING
	case status == "pending":
		return aievalv1.EvalRunStatus_EVAL_RUN_STATUS_PENDING
	case status == "failed" || status == "fail" || status == "error" || failedCount > 0:
		return aievalv1.EvalRunStatus_EVAL_RUN_STATUS_FAILED
	case status == "passed" || status == "pass" || status == "ok" || status == "succeeded":
		return aievalv1.EvalRunStatus_EVAL_RUN_STATUS_PASSED
	case caseCount > 0 && passedCount+skippedCount == caseCount:
		return aievalv1.EvalRunStatus_EVAL_RUN_STATUS_PASSED
	default:
		return aievalv1.EvalRunStatus_EVAL_RUN_STATUS_PENDING
	}
}

func metadataJSON(summary map[string]any, summaryBytes []byte, summaryRef string) (string, error) {
	digest := sha256.Sum256(summaryBytes)
	metadata := map[string]any{
		"source":                 "ai-eval-record-smoke",
		"smoke_schema_version":   1,
		"summary_sha256":         hex.EncodeToString(digest[:]),
		"summary_basename":       filepath.Base(summaryRef),
		"summary_schema_version": intOrStringValue(summary, "schema_version"),
	}
	if value := stringValue(summary, "scope"); value != "" {
		metadata["scope"] = value
	}
	if value := stringValue(summary, "case_path"); value != "" {
		metadata["case_path"] = value
	}
	content, err := json.Marshal(metadata)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func openPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	return pgxpool.NewWithConfig(ctx, config)
}

func applyMigration(ctx context.Context, pool *pgxpool.Pool) error {
	root, err := findRepoRoot()
	if err != nil {
		return err
	}
	path := filepath.Join(root, "migrations", "postgres", "ai-eval-service", "000001_ai_eval_core.sql")
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read migration: %w", err)
	}
	if _, err := pool.Exec(ctx, string(content)); err != nil {
		return fmt.Errorf("apply migration: %w", err)
	}
	return nil
}

func startInProcessClient(ctx context.Context, pool *pgxpool.Pool) (aievalv1.AIEvalServiceClient, func(), error) {
	repository := postgresinfra.NewRepository(pool)
	server := grpcgo.NewServer()
	aievalgrpc.Register(server, aievalgrpc.NewServer(
		app.NewRecordEvalRunUseCase(repository),
		app.NewGetEvalRunUseCase(repository),
		app.NewListEvalRunsUseCase(repository),
	))
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, nil, err
	}
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.Serve(listener)
	}()
	conn, err := grpcgo.DialContext(ctx, listener.Addr().String(),
		grpcgo.WithTransportCredentials(insecure.NewCredentials()),
		grpcgo.WithBlock(),
	)
	if err != nil {
		server.Stop()
		_ = listener.Close()
		return nil, nil, err
	}
	stop := func() {
		_ = conn.Close()
		server.GracefulStop()
		err := <-serveErr
		if err != nil && !errors.Is(err, grpcgo.ErrServerStopped) {
			fmt.Fprintf(os.Stderr, "ai eval smoke grpc server stopped: %v\n", err)
		}
	}
	return aievalv1.NewAIEvalServiceClient(conn), stop, nil
}

func writeSmokeSummary(path string, result smokeSummary) error {
	content, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	if strings.TrimSpace(path) == "" {
		fmt.Println(string(content))
		return nil
	}
	resolved, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(resolved, append(content, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Printf("OK   ai-eval RecordEvalRun smoke summary written: %s\n", resolved)
	return nil
}

func containsRun(runs []*aievalv1.EvalRun, runID string) bool {
	for _, run := range runs {
		if run.GetRunId() == runID {
			return true
		}
	}
	return false
}

func evalRunStatusName(status aievalv1.EvalRunStatus) string {
	return strings.TrimPrefix(status.String(), "EVAL_RUN_STATUS_")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func stringValue(values map[string]any, key string) string {
	value, ok := values[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case json.Number:
		return typed.String()
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case bool:
		if typed {
			return "true"
		}
		return "false"
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func intValue(values map[string]any, key string) (int, bool) {
	value, ok := values[key]
	if !ok || value == nil {
		return 0, false
	}
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	case json.Number:
		parsed, err := typed.Int64()
		if err != nil {
			return 0, false
		}
		return int(parsed), true
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}

func boolValue(values map[string]any, key string) bool {
	value, ok := values[key]
	if !ok || value == nil {
		return false
	}
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		parsed, _ := strconv.ParseBool(strings.TrimSpace(typed))
		return parsed
	default:
		return false
	}
}

func intOrStringValue(values map[string]any, key string) any {
	if parsed, ok := intValue(values, key); ok {
		return parsed
	}
	if value := stringValue(values, key); value != "" {
		return value
	}
	return nil
}

func firstCaseString(summary map[string]any, key string) string {
	cases, _ := summary["cases"].([]any)
	for _, item := range cases {
		caseMap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if value := stringValue(caseMap, key); value != "" {
			return value
		}
	}
	return ""
}

var invalidIDCharacters = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func sanitizeID(value string) string {
	value = strings.TrimSpace(value)
	value = invalidIDCharacters.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-_.")
	return value
}

func findRepoRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(wd, "go.mod")); err == nil {
			if _, err := os.Stat(filepath.Join(wd, "migrations", "postgres", "ai-eval-service", "000001_ai_eval_core.sql")); err == nil {
				return wd, nil
			}
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			return "", errors.New("repository root not found")
		}
		wd = parent
	}
}
