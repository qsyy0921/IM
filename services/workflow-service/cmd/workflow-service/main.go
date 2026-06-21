package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
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
	case "compensation-worker":
		return runCompensationWorker(ctx)
	case "compensation-executor":
		return runCompensationExecutor(ctx)
	case "compensation-instruction-import":
		return runCompensationInstructionImport(ctx)
	default:
		return fmt.Errorf("unsupported NEXUSIM_WORKFLOW_SERVICE_MODE %q", mode)
	}
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
	case "noop", "grpc", "compensation-worker", "compensation-executor", "compensation-instruction-import":
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

func envString(name string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}

func envInt(name string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func envDuration(name string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return fallback
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
