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
	admingrpc "github.com/qsyy0921/IM/services/admin-service/internal/api/grpc"
	"github.com/qsyy0921/IM/services/admin-service/internal/app"
	executorinfra "github.com/qsyy0921/IM/services/admin-service/internal/infrastructure/executor"
	kafkainfra "github.com/qsyy0921/IM/services/admin-service/internal/infrastructure/kafka"
	postgresinfra "github.com/qsyy0921/IM/services/admin-service/internal/infrastructure/postgres"
	rpcinfra "github.com/qsyy0921/IM/services/admin-service/internal/infrastructure/rpc"
	"github.com/qsyy0921/IM/services/admin-service/internal/trigger/operation"
	"github.com/qsyy0921/IM/services/admin-service/internal/trigger/outbox"
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
	mode := adminModeFromEnv()
	if err := validateAdminMode(mode); err != nil {
		return err
	}
	switch mode {
	case "noop":
		return runNoop(ctx)
	case "grpc":
		return runGRPC(ctx)
	case "operation-worker":
		return runOperationWorker(ctx)
	case "outbox-relay":
		return runOutboxRelay(ctx)
	case "compensation-request":
		return runCompensationRequest(ctx)
	default:
		return fmt.Errorf("unsupported NEXUSIM_ADMIN_SERVICE_MODE %q", mode)
	}
}

func runNoop(ctx context.Context) error {
	debugAddr, err := adminDebugAddrFromEnv()
	if err != nil {
		return err
	}
	stopDebug, err := startDebugServer(ctx, debugAddr)
	if err != nil {
		return err
	}
	defer stopDebug()
	log.Println("admin-service noop mode: set NEXUSIM_ADMIN_SERVICE_MODE=grpc to start runtime role")
	<-ctx.Done()
	return nil
}

func runGRPC(ctx context.Context) error {
	debugAddr, err := adminDebugAddrFromEnv()
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

	addr := envString("NEXUSIM_ADMIN_GRPC_ADDR", "127.0.0.1:10770")
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	repository := postgresinfra.NewRepository(pool)
	ids := app.NewRandomIDGenerator()
	server := grpc.NewServer()
	admingrpc.Register(server, admingrpc.NewServer(
		app.NewCreateAdminOperationUseCase(repository, ids),
		app.NewApproveAdminOperationUseCase(repository, ids),
		app.NewGetAdminOperationUseCase(repository),
		app.NewListAdminOperationsUseCase(repository),
	))

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.Serve(listener)
	}()
	log.Printf("admin-service grpc listening on %s", addr)

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

func runOperationWorker(ctx context.Context) error {
	debugAddr, err := adminDebugAddrFromEnv()
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

	localExecutor := executorinfra.OperationExecutor(
		executorinfra.NewNoopExecutor(envString("NEXUSIM_ADMIN_OPERATION_EXECUTOR_ID", "local-noop")),
	)
	controlPlaneAddr := strings.TrimSpace(os.Getenv("NEXUSIM_CONTROL_PLANE_GRPC_ADDR"))
	if controlPlaneAddr != "" {
		executor, closeControlPlane, err := rpcinfra.DialControlPlaneConfigPublishExecutor(
			ctx,
			controlPlaneAddr,
			envDuration("NEXUSIM_ADMIN_CONTROL_PLANE_RPC_TIMEOUT", time.Second),
		)
		if err != nil {
			return err
		}
		defer func() {
			if closeErr := closeControlPlane(); closeErr != nil {
				log.Printf("admin-service control-plane client close failed: %v", closeErr)
			}
		}()
		localExecutor = executorinfra.NewOperationTypeRoutingExecutor(localExecutor, map[string]executorinfra.OperationExecutor{
			executorinfra.OperationTypeConfigPublish:     executor,
			executorinfra.OperationTypeConfigRollback:    executor,
			executorinfra.OperationTypeTenantQuotaChange: executor,
			executorinfra.OperationTypePolicyRuleChange:  executor,
		})
		log.Printf("admin-service operation worker routing CONFIG_PUBLISH/CONFIG_ROLLBACK/TENANT_QUOTA_CHANGE/POLICY_RULE_CHANGE to control-plane-service at %s", controlPlaneAddr)
	}
	auditAddr := strings.TrimSpace(os.Getenv("NEXUSIM_AUDIT_GRPC_ADDR"))
	if auditAddr != "" {
		executor, closeAudit, err := rpcinfra.DialAuditExportExecutor(
			ctx,
			auditAddr,
			envDuration("NEXUSIM_ADMIN_AUDIT_RPC_TIMEOUT", time.Second),
		)
		if err != nil {
			return err
		}
		defer func() {
			if closeErr := closeAudit(); closeErr != nil {
				log.Printf("admin-service audit client close failed: %v", closeErr)
			}
		}()
		localExecutor = executorinfra.NewOperationTypeRoutingExecutor(localExecutor, map[string]executorinfra.OperationExecutor{
			executorinfra.OperationTypeAuditExportRequest: executor,
		})
		log.Printf("admin-service operation worker routing AUDIT_EXPORT_REQUEST to audit-service at %s", auditAddr)
	}
	var workflowExecutor executorinfra.OperationExecutor
	workflowAddr := strings.TrimSpace(os.Getenv("NEXUSIM_WORKFLOW_GRPC_ADDR"))
	if workflowAddr != "" {
		executor, closeWorkflow, err := rpcinfra.DialWorkflowExecutor(
			ctx,
			workflowAddr,
			envDuration("NEXUSIM_ADMIN_WORKFLOW_RPC_TIMEOUT", time.Second),
		)
		if err != nil {
			return err
		}
		defer func() {
			if closeErr := closeWorkflow(); closeErr != nil {
				log.Printf("admin-service workflow client close failed: %v", closeErr)
			}
		}()
		workflowExecutor = executor
		log.Printf("admin-service operation worker routing workflow-required operations to %s", workflowAddr)
	} else {
		log.Println("admin-service operation worker has no workflow-service address; workflow-required operations fail closed")
	}

	worker := operation.NewWorker(
		postgresinfra.NewRepository(pool),
		executorinfra.NewRiskRoutingExecutor(localExecutor, workflowExecutor),
		operation.Config{
			BatchSize:      envInt("NEXUSIM_ADMIN_OPERATION_BATCH_SIZE", 50),
			PollInterval:   envDuration("NEXUSIM_ADMIN_OPERATION_POLL_INTERVAL", time.Second),
			StaleAfter:     envDuration("NEXUSIM_ADMIN_OPERATION_STALE_AFTER", 5*time.Minute),
			ErrorBackoff:   envDuration("NEXUSIM_ADMIN_OPERATION_ERROR_BACKOFF", time.Second),
			ResultIDPrefix: envString("NEXUSIM_ADMIN_OPERATION_RESULT_ID_PREFIX", "admres_"),
			Logf:           log.Printf,
		},
	)
	log.Println("admin-service operation worker started with local executor")
	return worker.Run(ctx)
}

func runOutboxRelay(ctx context.Context) error {
	debugAddr, err := adminDebugAddrFromEnv()
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

	brokers := splitCSV(os.Getenv("NEXUSIM_KAFKA_BROKERS"))
	if len(brokers) == 0 {
		return errors.New("NEXUSIM_KAFKA_BROKERS is required")
	}
	producer, err := kafkainfra.NewWriterProducer(brokers)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := producer.Close(); closeErr != nil {
			log.Printf("admin-service outbox relay producer close failed: %v", closeErr)
		}
	}()

	relay := outbox.NewRelay(
		postgresinfra.NewOutboxStore(pool),
		producer,
		outbox.Config{
			Topic:          envString("NEXUSIM_ADMIN_EVENTS_TOPIC", outbox.TopicAdminEvents),
			BatchSize:      envInt("NEXUSIM_ADMIN_OUTBOX_BATCH_SIZE", 500),
			PollInterval:   envDuration("NEXUSIM_ADMIN_OUTBOX_POLL_INTERVAL", time.Second),
			MaxAttempts:    envInt("NEXUSIM_ADMIN_OUTBOX_MAX_ATTEMPTS", 5),
			RetryBaseDelay: envDuration("NEXUSIM_ADMIN_OUTBOX_RETRY_BASE_DELAY", time.Second),
			ErrorBackoff:   envDuration("NEXUSIM_ADMIN_OUTBOX_ERROR_BACKOFF", time.Second),
			Logf:           log.Printf,
		},
	)
	log.Printf("admin-service outbox relay publishing to %s via brokers %s", envString("NEXUSIM_ADMIN_EVENTS_TOPIC", outbox.TopicAdminEvents), strings.Join(brokers, ","))
	return relay.Run(ctx)
}

func adminModeFromEnv() string {
	mode := strings.TrimSpace(os.Getenv("NEXUSIM_ADMIN_SERVICE_MODE"))
	if mode == "" {
		mode = "noop"
	}
	return mode
}

func validateAdminMode(mode string) error {
	switch mode {
	case "noop", "grpc", "operation-worker", "outbox-relay", "compensation-request":
		return nil
	default:
		return fmt.Errorf("unsupported NEXUSIM_ADMIN_SERVICE_MODE %q", mode)
	}
}

func adminDebugAddr() string {
	if addr := strings.TrimSpace(os.Getenv("NEXUSIM_ADMIN_DEBUG_ADDR")); addr != "" {
		return addr
	}
	return strings.TrimSpace(os.Getenv("NEXUSIM_DEBUG_ADDR"))
}

func adminDebugAddrFromEnv() (string, error) {
	addr := adminDebugAddr()
	allowPublic, _, err := envOptionalBool("NEXUSIM_ADMIN_DEBUG_ALLOW_PUBLIC")
	if err != nil {
		return "", err
	}
	return addr, validateAdminDebugListenerConfig(addr, allowPublic)
}

func validateAdminDebugListenerConfig(addr string, allowPublic bool) error {
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
	return errors.New("admin debug listener address is non-private; set NEXUSIM_ADMIN_DEBUG_ALLOW_PUBLIC=true to allow")
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
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return defaultValue
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return defaultValue
	}
	return value
}

func envDuration(name string, defaultValue time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return defaultValue
	}
	value, err := time.ParseDuration(raw)
	if err != nil || value <= 0 {
		return defaultValue
	}
	return value
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			items = append(items, part)
		}
	}
	return items
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
			log.Printf("admin debug server stopped: %v", err)
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
		_, _ = writer.Write([]byte("# HELP nexusim_admin_service_info Static admin-service info metric.\n"))
		_, _ = writer.Write([]byte("# TYPE nexusim_admin_service_info gauge\n"))
		_, _ = writer.Write([]byte("nexusim_admin_service_info 1\n"))
	}
	mux.HandleFunc("/metrics", metricsHandler)
	mux.HandleFunc("/debug/metrics", metricsHandler)
	return mux
}
