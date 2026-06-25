package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	workflowgrpc "github.com/qsyy0921/IM/services/workflow-service/internal/api/grpc"
	"github.com/qsyy0921/IM/services/workflow-service/internal/app"
	postgresinfra "github.com/qsyy0921/IM/services/workflow-service/internal/infrastructure/postgres"
	rpcinfra "github.com/qsyy0921/IM/services/workflow-service/internal/infrastructure/rpc"
	"github.com/qsyy0921/IM/services/workflow-service/internal/trigger/compensation"
	externalcallback "github.com/qsyy0921/IM/services/workflow-service/internal/trigger/externalcallback"
	timertrigger "github.com/qsyy0921/IM/services/workflow-service/internal/trigger/timer"
	"github.com/qsyy0921/IM/services/workflow-service/internal/types"
	"google.golang.org/grpc"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context) error {
	mode := workflowModeFromEnv()
	if err := validateWorkflowMode(mode); err != nil {
		return err
	}
	switch mode {
	case "noop":
		return runNoop(ctx)
	case "grpc":
		return runGRPC(ctx)
	case "timer-worker":
		return runTimerWorker(ctx)
	case "compensation-worker":
		return runCompensationWorker(ctx)
	case "compensation-executor":
		return runCompensationExecutor(ctx)
	case "compensation-instruction-import":
		return runCompensationInstructionImport(ctx)
	case "external-callback-delivery-import":
		return runExternalCallbackDeliveryImport(ctx)
	case "external-callback-delivery-redrive":
		return runExternalCallbackDeliveryRedrive(ctx)
	case "external-callback-delivery-worker":
		return runExternalCallbackDeliveryWorker(ctx)
	default:
		return fmt.Errorf("unsupported NEXUSIM_WORKFLOW_SERVICE_MODE %q", mode)
	}
}

func runTimerWorker(ctx context.Context) error {
	debugAddr, err := workflowDebugAddrFromEnv()
	if err != nil {
		return err
	}
	stopDebug, err := startDebugServer(ctx, debugAddr)
	if err != nil {
		return err
	}
	defer stopDebug()

	pool, err := openPGPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	worker := timertrigger.NewWorker(postgresinfra.NewRepository(pool), timertrigger.Config{
		BatchSize:    envInt("NEXUSIM_WORKFLOW_TIMER_BATCH_SIZE", 50),
		PollInterval: envDuration("NEXUSIM_WORKFLOW_TIMER_POLL_INTERVAL", time.Second),
		ErrorBackoff: envDuration("NEXUSIM_WORKFLOW_TIMER_ERROR_BACKOFF", time.Second),
		Logf:         log.Printf,
	})
	log.Println("workflow-service timer-worker started")
	if err := worker.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}

func runNoop(ctx context.Context) error {
	debugAddr, err := workflowDebugAddrFromEnv()
	if err != nil {
		return err
	}
	stopDebug, err := startDebugServer(ctx, debugAddr)
	if err != nil {
		return err
	}
	defer stopDebug()
	log.Println("workflow-service noop mode: set NEXUSIM_WORKFLOW_SERVICE_MODE=grpc to start runtime role")
	<-ctx.Done()
	return nil
}

func runGRPC(ctx context.Context) error {
	debugAddr, err := workflowDebugAddrFromEnv()
	if err != nil {
		return err
	}
	stopDebug, err := startDebugServer(ctx, debugAddr)
	if err != nil {
		return err
	}
	defer stopDebug()

	pool, err := openPGPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	addr := envString("NEXUSIM_WORKFLOW_GRPC_ADDR", "127.0.0.1:10750")
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	repository := postgresinfra.NewRepository(pool)
	ids := app.NewRandomIDGenerator()
	server := grpc.NewServer()
	workflowgrpc.Register(server, workflowgrpc.NewServer(
		app.NewCreateWorkflowUseCase(repository, ids),
		app.NewRecordWorkflowDecisionUseCase(repository, ids),
		app.NewGetWorkflowUseCase(repository),
		app.NewListWorkflowsUseCase(repository),
		app.NewListWorkflowCompensationsUseCase(repository),
		app.NewListWorkflowCompensationInstructionsUseCase(repository),
	))

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.Serve(listener)
	}()
	log.Printf("workflow-service grpc listening on %s", addr)

	select {
	case err := <-serveErr:
		if errors.Is(err, grpc.ErrServerStopped) {
			return context.Canceled
		}
		return err
	case <-ctx.Done():
		server.GracefulStop()
		err := <-serveErr
		if err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			return err
		}
		return context.Canceled
	}
}

func runCompensationWorker(ctx context.Context) error {
	debugAddr, err := workflowDebugAddrFromEnv()
	if err != nil {
		return err
	}
	stopDebug, err := startDebugServer(ctx, debugAddr)
	if err != nil {
		return err
	}
	defer stopDebug()

	pool, err := openPGPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	worker := compensation.NewWorker(postgresinfra.NewRepository(pool), compensation.Config{
		BatchSize:    envInt("NEXUSIM_WORKFLOW_COMPENSATION_BATCH_SIZE", 50),
		PollInterval: envDuration("NEXUSIM_WORKFLOW_COMPENSATION_POLL_INTERVAL", time.Second),
		ErrorBackoff: envDuration("NEXUSIM_WORKFLOW_COMPENSATION_ERROR_BACKOFF", time.Second),
		Logf:         log.Printf,
	})
	log.Println("workflow-service compensation-worker started")
	if err := worker.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}

func runCompensationExecutor(ctx context.Context) error {
	debugAddr, err := workflowDebugAddrFromEnv()
	if err != nil {
		return err
	}
	stopDebug, err := startDebugServer(ctx, debugAddr)
	if err != nil {
		return err
	}
	defer stopDebug()

	pool, err := openPGPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	repository := postgresinfra.NewRepository(pool)
	executor, cleanup, err := workflowCompensationExecutorFromEnv(ctx, repository)
	if err != nil {
		return err
	}
	if cleanup != nil {
		defer func() { _ = cleanup() }()
	}
	worker := compensation.NewExecutionWorker(repository, executor, compensation.Config{
		BatchSize:    envInt("NEXUSIM_WORKFLOW_COMPENSATION_EXECUTION_BATCH_SIZE", 50),
		PollInterval: envDuration("NEXUSIM_WORKFLOW_COMPENSATION_EXECUTION_POLL_INTERVAL", time.Second),
		ErrorBackoff: envDuration("NEXUSIM_WORKFLOW_COMPENSATION_EXECUTION_ERROR_BACKOFF", time.Second),
		Logf:         log.Printf,
	})
	log.Println("workflow-service compensation-executor started")
	if err := worker.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}

func runCompensationInstructionImport(ctx context.Context) error {
	pool, err := openPGPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	tenantID := strings.TrimSpace(os.Getenv("NEXUSIM_WORKFLOW_COMPENSATION_INSTRUCTION_TENANT_ID"))
	if tenantID == "" {
		return errors.New("NEXUSIM_WORKFLOW_COMPENSATION_INSTRUCTION_TENANT_ID is required")
	}
	instructions, err := rpcinfra.LoadControlPlaneRollbackInstructions(os.Getenv("NEXUSIM_WORKFLOW_COMPENSATION_INSTRUCTION_FILE"))
	if err != nil {
		return err
	}
	workflowInstructions, err := rpcinfra.ControlPlaneRollbackInstructionsForTenant(types.TenantID(tenantID), instructions)
	if err != nil {
		return err
	}
	count, err := postgresinfra.NewRepository(pool).UpsertWorkflowCompensationInstructions(ctx, workflowInstructions)
	if err != nil {
		return err
	}
	log.Printf("workflow-service imported %d compensation instructions", count)
	return nil
}

func runExternalCallbackDeliveryImport(ctx context.Context) error {
	pool, err := openPGPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	tenantID := strings.TrimSpace(os.Getenv("NEXUSIM_WORKFLOW_EXTERNAL_CALLBACK_DELIVERY_TENANT_ID"))
	if tenantID == "" {
		return errors.New("NEXUSIM_WORKFLOW_EXTERNAL_CALLBACK_DELIVERY_TENANT_ID is required")
	}
	delivery, err := rpcinfra.LoadExternalCallbackDeliveryPlan(
		os.Getenv("NEXUSIM_WORKFLOW_EXTERNAL_CALLBACK_DELIVERY_PLAN_FILE"),
		types.TenantID(tenantID),
	)
	if err != nil {
		return err
	}
	registered, replayed, err := postgresinfra.NewRepository(pool).RegisterExternalCallbackDelivery(ctx, delivery)
	if err != nil {
		return err
	}
	log.Printf(
		"workflow-service registered external callback delivery delivery_id=%s workflow_id=%s replayed=%v status=%s",
		registered.DeliveryID,
		registered.WorkflowID,
		replayed,
		registered.Status,
	)
	return nil
}

func runExternalCallbackDeliveryRedrive(ctx context.Context) error {
	pool, err := openPGPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	tenantID := strings.TrimSpace(os.Getenv("NEXUSIM_WORKFLOW_EXTERNAL_CALLBACK_DELIVERY_TENANT_ID"))
	if tenantID == "" {
		return errors.New("NEXUSIM_WORKFLOW_EXTERNAL_CALLBACK_DELIVERY_TENANT_ID is required")
	}
	plan, err := rpcinfra.LoadExternalCallbackRedrivePlan(
		os.Getenv("NEXUSIM_WORKFLOW_EXTERNAL_CALLBACK_REDRIVE_PLAN_FILE"),
		types.TenantID(tenantID),
	)
	if err != nil {
		return err
	}
	summaryPath := strings.TrimSpace(os.Getenv("NEXUSIM_WORKFLOW_EXTERNAL_CALLBACK_REDRIVE_SUMMARY_FILE"))
	if err := prepareExternalCallbackRedriveExecutionSummaryPath(summaryPath); err != nil {
		return err
	}
	redriven, err := postgresinfra.NewRepository(pool).RedriveExternalCallbackDelivery(ctx, plan)
	if err != nil {
		return err
	}
	if err := writeExternalCallbackRedriveExecutionSummary(summaryPath, plan, redriven); err != nil {
		return err
	}
	log.Printf(
		"workflow-service redriven external callback delivery delivery_id=%s workflow_id=%s status=%s redrive_count=%d",
		redriven.DeliveryID,
		redriven.WorkflowID,
		redriven.Status,
		redriven.RedriveCount,
	)
	return nil
}

type externalCallbackRedriveExecutionSummary struct {
	SchemaVersion              string `json:"schema_version"`
	Mode                       string `json:"mode"`
	GeneratedAt                string `json:"generated_at"`
	RedrivePlanID              string `json:"redrive_plan_id"`
	RedrivePlanSha256          string `json:"redrive_plan_sha256"`
	SourceDeliveryStatusSha256 string `json:"source_delivery_status_sha256"`
	SourceDeliveryPlanSha256   string `json:"source_delivery_plan_sha256"`
	TenantID                   string `json:"tenant_id"`
	WorkflowID                 string `json:"workflow_id"`
	StepID                     string `json:"step_id"`
	DeliveryID                 string `json:"delivery_id"`
	TargetService              string `json:"target_service"`
	TargetOperation            string `json:"target_operation"`
	TargetRefHash              string `json:"target_ref_hash"`
	PayloadSchemaVersion       string `json:"payload_schema_version"`
	PayloadRefHash             string `json:"payload_ref_hash"`
	ApprovalPolicyRef          string `json:"approval_policy_ref"`
	DecisionPolicyRef          string `json:"decision_policy_ref"`
	DeliveryStatus             string `json:"delivery_status"`
	RedriveCount               int    `json:"redrive_count"`
	LastRedrivePlanSha256      string `json:"last_redrive_plan_sha256"`
	LastRedriveReasonRef       string `json:"last_redrive_reason_ref"`
	OutboxEventType            string `json:"outbox_event_type"`
	ExecutedRedrive            bool   `json:"executed_redrive"`
	RecordsDecision            bool   `json:"records_decision"`
	CallsProvider              bool   `json:"calls_provider"`
	ExecutesTarget             bool   `json:"executes_target"`
	MutatesDeliveryFact        bool   `json:"mutates_delivery_fact"`
}

func writeExternalCallbackRedriveExecutionSummaryFromEnv(
	plan types.WorkflowExternalCallbackRedrivePlan,
	redriven types.WorkflowExternalCallbackDelivery,
) error {
	path := strings.TrimSpace(os.Getenv("NEXUSIM_WORKFLOW_EXTERNAL_CALLBACK_REDRIVE_SUMMARY_FILE"))
	return writeExternalCallbackRedriveExecutionSummary(path, plan, redriven)
}

func prepareExternalCallbackRedriveExecutionSummaryPath(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	directory := strings.TrimSpace(filepath.Dir(path))
	if directory != "" {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			return fmt.Errorf("create workflow external callback redrive summary directory: %w", err)
		}
	}
	probePath := filepath.Join(directory, ".nexusim-redrive-summary-write-probe-"+strconv.FormatInt(time.Now().UnixNano(), 10))
	if err := os.WriteFile(probePath, []byte("probe\n"), 0o644); err != nil {
		return fmt.Errorf("preflight workflow external callback redrive summary path: %w", err)
	}
	if err := os.Remove(probePath); err != nil {
		return fmt.Errorf("cleanup workflow external callback redrive summary path probe: %w", err)
	}
	return nil
}

func writeExternalCallbackRedriveExecutionSummary(
	path string,
	plan types.WorkflowExternalCallbackRedrivePlan,
	redriven types.WorkflowExternalCallbackDelivery,
) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	summary := externalCallbackRedriveExecutionSummary{
		SchemaVersion:              "nexusim.workflow.external_callback_redrive_execution_summary.v1",
		Mode:                       "external-callback-delivery-redrive",
		GeneratedAt:                time.Now().UTC().Format(time.RFC3339Nano),
		RedrivePlanID:              plan.RedrivePlanID,
		RedrivePlanSha256:          plan.RedrivePlanSha256,
		SourceDeliveryStatusSha256: plan.SourceDeliveryStatusSha256,
		SourceDeliveryPlanSha256:   plan.SourceDeliveryPlanSha256,
		TenantID:                   string(redriven.TenantID),
		WorkflowID:                 redriven.WorkflowID,
		StepID:                     redriven.StepID,
		DeliveryID:                 redriven.DeliveryID,
		TargetService:              redriven.TargetService,
		TargetOperation:            redriven.TargetOperation,
		TargetRefHash:              redriven.TargetRefHash,
		PayloadSchemaVersion:       redriven.PayloadSchemaVersion,
		PayloadRefHash:             redriven.PayloadRefHash,
		ApprovalPolicyRef:          redriven.ApprovalPolicyRef,
		DecisionPolicyRef:          redriven.DecisionPolicyRef,
		DeliveryStatus:             redriven.Status,
		RedriveCount:               redriven.RedriveCount,
		LastRedrivePlanSha256:      redriven.LastRedrivePlanSha256,
		LastRedriveReasonRef:       redriven.LastRedriveReasonRef,
		OutboxEventType:            types.WorkflowEventExternalCallbackRedriven,
		ExecutedRedrive:            true,
		RecordsDecision:            false,
		CallsProvider:              false,
		ExecutesTarget:             false,
		MutatesDeliveryFact:        true,
	}
	encoded, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return fmt.Errorf("encode workflow external callback redrive summary: %w", err)
	}
	if err := os.WriteFile(path, append(encoded, '\n'), 0o644); err != nil {
		return fmt.Errorf("write workflow external callback redrive summary: %w", err)
	}
	return nil
}

func runExternalCallbackDeliveryWorker(ctx context.Context) error {
	debugAddr, err := workflowDebugAddrFromEnv()
	if err != nil {
		return err
	}
	stopDebug, err := startDebugServer(ctx, debugAddr)
	if err != nil {
		return err
	}
	defer stopDebug()

	pool, err := openPGPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	endpoints, err := rpcinfra.LoadExternalCallbackEndpoints(os.Getenv("NEXUSIM_WORKFLOW_EXTERNAL_CALLBACK_ENDPOINTS_FILE"))
	if err != nil {
		return err
	}
	provider, err := rpcinfra.NewExternalCallbackHTTPProvider(
		nil,
		endpoints,
		envDuration("NEXUSIM_WORKFLOW_EXTERNAL_CALLBACK_HTTP_TIMEOUT", time.Second),
	)
	if err != nil {
		return err
	}
	worker := externalcallback.NewWorker(postgresinfra.NewRepository(pool), provider, externalcallback.Config{
		BatchSize:      envInt("NEXUSIM_WORKFLOW_EXTERNAL_CALLBACK_BATCH_SIZE", 50),
		PollInterval:   envDuration("NEXUSIM_WORKFLOW_EXTERNAL_CALLBACK_POLL_INTERVAL", time.Second),
		ErrorBackoff:   envDuration("NEXUSIM_WORKFLOW_EXTERNAL_CALLBACK_ERROR_BACKOFF", time.Second),
		LeaseDuration:  envDuration("NEXUSIM_WORKFLOW_EXTERNAL_CALLBACK_LEASE_DURATION", 30*time.Second),
		RetryBaseDelay: envDuration("NEXUSIM_WORKFLOW_EXTERNAL_CALLBACK_RETRY_BASE_DELAY", time.Second),
		Logf:           log.Printf,
	})
	log.Println("workflow-service external-callback-delivery-worker started")
	if err := worker.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}

func workflowCompensationExecutorFromEnv(
	ctx context.Context,
	instructionResolver rpcinfra.ControlPlaneRollbackInstructionResolver,
) (compensation.Executor, func() error, error) {
	mode := envString("NEXUSIM_WORKFLOW_COMPENSATION_EXECUTOR_MODE", "")
	switch mode {
	case "control-plane-rollback-file":
		instructions, err := rpcinfra.LoadControlPlaneRollbackInstructions(os.Getenv("NEXUSIM_WORKFLOW_COMPENSATION_INSTRUCTION_FILE"))
		if err != nil {
			return nil, nil, err
		}
		return rpcinfra.DialControlPlaneCompensationExecutor(
			ctx,
			os.Getenv("NEXUSIM_CONTROL_PLANE_GRPC_ADDR"),
			envDuration("NEXUSIM_WORKFLOW_COMPENSATION_RPC_TIMEOUT", time.Second),
			instructions,
		)
	case "control-plane-rollback-store":
		if instructionResolver == nil {
			return nil, nil, errors.New("workflow compensation instruction resolver is required")
		}
		return rpcinfra.DialControlPlaneCompensationExecutorWithResolver(
			ctx,
			os.Getenv("NEXUSIM_CONTROL_PLANE_GRPC_ADDR"),
			envDuration("NEXUSIM_WORKFLOW_COMPENSATION_RPC_TIMEOUT", time.Second),
			instructionResolver,
		)
	case "":
		return nil, nil, errors.New("NEXUSIM_WORKFLOW_COMPENSATION_EXECUTOR_MODE is required")
	default:
		return nil, nil, fmt.Errorf("unsupported NEXUSIM_WORKFLOW_COMPENSATION_EXECUTOR_MODE %q", mode)
	}
}

func workflowModeFromEnv() string {
	mode := strings.TrimSpace(os.Getenv("NEXUSIM_WORKFLOW_SERVICE_MODE"))
	if mode == "" {
		mode = "noop"
	}
	return mode
}

func validateWorkflowMode(mode string) error {
	switch mode {
	case "noop", "grpc", "timer-worker", "compensation-worker", "compensation-executor", "compensation-instruction-import",
		"external-callback-delivery-import", "external-callback-delivery-redrive", "external-callback-delivery-worker":
		return nil
	default:
		return fmt.Errorf("unsupported NEXUSIM_WORKFLOW_SERVICE_MODE %q", mode)
	}
}

func workflowDebugAddr() string {
	if addr := strings.TrimSpace(os.Getenv("NEXUSIM_WORKFLOW_DEBUG_ADDR")); addr != "" {
		return addr
	}
	return strings.TrimSpace(os.Getenv("NEXUSIM_DEBUG_ADDR"))
}

func workflowDebugAddrFromEnv() (string, error) {
	addr := workflowDebugAddr()
	allowPublic, _, err := envOptionalBool("NEXUSIM_WORKFLOW_DEBUG_ALLOW_PUBLIC")
	if err != nil {
		return "", err
	}
	return addr, validateWorkflowDebugListenerConfig(addr, allowPublic)
}

func validateWorkflowDebugListenerConfig(addr string, allowPublic bool) error {
	if strings.TrimSpace(addr) == "" {
		return nil
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	if host == "localhost" {
		return nil
	}
	ip := net.ParseIP(host)
	if ip != nil && (ip.IsLoopback() || ip.IsPrivate()) {
		return nil
	}
	if allowPublic {
		return nil
	}
	return errors.New("workflow debug listener address is non-private; set NEXUSIM_WORKFLOW_DEBUG_ALLOW_PUBLIC=true to allow")
}

func envOptionalBool(name string) (bool, bool, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return false, false, nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, true, err
	}
	return value, true, nil
}

func envString(name string, defaultValue string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return defaultValue
	}
	return value
}

func envInt(name string, defaultValue int) int {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return defaultValue
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return defaultValue
	}
	return parsed
}

func envDuration(name string, defaultValue time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return defaultValue
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return defaultValue
	}
	return parsed
}

func openPGPool(ctx context.Context) (*pgxpool.Pool, error) {
	dsn := strings.TrimSpace(os.Getenv("NEXUSIM_PG_DSN"))
	if dsn == "" {
		return nil, errors.New("NEXUSIM_PG_DSN is required")
	}
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	connectCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return pgxpool.NewWithConfig(connectCtx, config)
}

func startDebugServer(ctx context.Context, addr string) (func(), error) {
	if strings.TrimSpace(addr) == "" {
		return func() {}, nil
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	server := &http.Server{
		Handler:           newDebugHandler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("workflow debug server stopped: %v", err)
		}
	}()
	return func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}, nil
}

func newDebugHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte("ok\n"))
	})
	mux.HandleFunc("/readyz", func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte("ok\n"))
	})
	metricsHandler := func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = writer.Write([]byte("# HELP nexusim_workflow_service_info Static workflow-service info metric.\n"))
		_, _ = writer.Write([]byte("# TYPE nexusim_workflow_service_info gauge\n"))
		_, _ = writer.Write([]byte("nexusim_workflow_service_info 1\n"))
	}
	mux.HandleFunc("/metrics", metricsHandler)
	mux.HandleFunc("/debug/metrics", metricsHandler)
	return mux
}
