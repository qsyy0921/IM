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
	agentgrpc "github.com/qsyy0921/IM/services/agent-service/internal/api/grpc"
	"github.com/qsyy0921/IM/services/agent-service/internal/app"
	kafkainfra "github.com/qsyy0921/IM/services/agent-service/internal/infrastructure/kafka"
	postgresinfra "github.com/qsyy0921/IM/services/agent-service/internal/infrastructure/postgres"
	rpcinfra "github.com/qsyy0921/IM/services/agent-service/internal/infrastructure/rpc"
	"github.com/qsyy0921/IM/services/agent-service/internal/trigger/outbox"
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
	mode := agentServiceModeFromEnv()
	if err := validateAgentServiceMode(mode); err != nil {
		return err
	}
	switch mode {
	case "noop":
		return runNoop(ctx)
	case "grpc":
		return runGRPC(ctx)
	case "approval-outbox-relay":
		return runApprovalOutboxRelay(ctx)
	default:
		return fmt.Errorf("unsupported NEXUSIM_AGENT_SERVICE_MODE %q", mode)
	}
}

func runNoop(ctx context.Context) error {
	debugAddr, err := agentDebugAddrFromEnv()
	if err != nil {
		return err
	}
	stopDebug, err := startDebugServer(ctx, debugAddr)
	if err != nil {
		return err
	}
	defer stopDebug()

	log.Println("agent-service noop mode: set NEXUSIM_AGENT_SERVICE_MODE=grpc to start runtime role")
	<-ctx.Done()
	return nil
}

func runGRPC(ctx context.Context) error {
	debugAddr, err := agentDebugAddrFromEnv()
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

	timeout := envDuration("NEXUSIM_AGENT_DEPENDENCY_TIMEOUT", 500*time.Millisecond)
	retrievalClient, closeRetrieval, err := rpcinfra.DialRetrievalClient(
		ctx,
		envString("NEXUSIM_RETRIEVAL_GRPC_ADDR", "127.0.0.1:10590"),
		timeout,
	)
	if err != nil {
		return err
	}
	defer closeRetrieval()
	mcpClient, closeMCPGateway, err := rpcinfra.DialMCPGatewayClient(
		ctx,
		envString("NEXUSIM_MCP_GATEWAY_GRPC_ADDR", "127.0.0.1:10650"),
		timeout,
	)
	if err != nil {
		return err
	}
	defer closeMCPGateway()

	addr := envString("NEXUSIM_AGENT_GRPC_ADDR", "127.0.0.1:10630")
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	repository := postgresinfra.NewRepository(pool)
	server := grpc.NewServer()
	agentgrpc.Register(server, agentgrpc.NewServerWithWorkflows(
		app.NewCreateAgentProposalUseCaseWithRepository(retrievalClient, mcpClient, app.ExtractiveProposalProvider{}, repository),
		app.NewApproveAgentProposalUseCase(repository),
		app.NewVerifyApprovedAgentProposalUseCase(repository),
	))

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.Serve(listener)
	}()
	log.Printf("agent-service grpc listening on %s", addr)

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

func runApprovalOutboxRelay(ctx context.Context) error {
	debugAddr, err := agentDebugAddrFromEnv()
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

	producer, err := kafkainfra.NewWriterProducer(splitCSV(os.Getenv("NEXUSIM_KAFKA_BROKERS")))
	if err != nil {
		return err
	}
	defer producer.Close()

	topic := envString("NEXUSIM_AGENT_EVENTS_TOPIC", outbox.TopicAgentEvents)
	relay := outbox.NewRelay(
		postgresinfra.NewOutboxStore(pool),
		producer,
		outbox.Config{
			Topic:          topic,
			BatchSize:      envInt("NEXUSIM_AGENT_APPROVAL_OUTBOX_BATCH_SIZE", 500),
			PollInterval:   envDuration("NEXUSIM_AGENT_APPROVAL_OUTBOX_POLL_INTERVAL", time.Second),
			MaxAttempts:    envInt("NEXUSIM_AGENT_APPROVAL_OUTBOX_MAX_ATTEMPTS", 5),
			RetryBaseDelay: envDuration("NEXUSIM_AGENT_APPROVAL_OUTBOX_RETRY_BASE_DELAY", time.Second),
			ErrorBackoff:   envDuration("NEXUSIM_AGENT_APPROVAL_OUTBOX_RELAY_ERROR_BACKOFF", time.Second),
			Logf:           log.Printf,
		},
	)
	log.Printf("agent-service approval outbox relay started topic=%s", topic)
	return relay.Run(ctx)
}

func agentServiceModeFromEnv() string {
	mode := strings.TrimSpace(os.Getenv("NEXUSIM_AGENT_SERVICE_MODE"))
	if mode == "" {
		mode = "noop"
	}
	return mode
}

func validateAgentServiceMode(mode string) error {
	switch mode {
	case "noop", "grpc", "approval-outbox-relay":
		return nil
	default:
		return fmt.Errorf("unsupported NEXUSIM_AGENT_SERVICE_MODE %q", mode)
	}
}

func agentDebugAddr() string {
	if addr := strings.TrimSpace(os.Getenv("NEXUSIM_AGENT_DEBUG_ADDR")); addr != "" {
		return addr
	}
	return strings.TrimSpace(os.Getenv("NEXUSIM_DEBUG_ADDR"))
}

func agentDebugAddrFromEnv() (string, error) {
	addr := agentDebugAddr()
	allowPublic, _, err := envOptionalBool("NEXUSIM_AGENT_DEBUG_ALLOW_PUBLIC")
	if err != nil {
		return "", err
	}
	return addr, validateAgentDebugListenerConfig(addr, allowPublic)
}

func validateAgentDebugListenerConfig(addr string, allowPublic bool) error {
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
	return errors.New("agent-service debug listener address is non-private; set NEXUSIM_AGENT_DEBUG_ALLOW_PUBLIC=true to allow")
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

func envDuration(name string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envInt(name string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
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
			log.Printf("agent-service debug server stopped: %v", err)
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
		_, _ = writer.Write([]byte("nexusim_agent_service_info 1\n"))
	}
	mux.HandleFunc("/debug/metrics", metricsHandler)
	mux.HandleFunc("/metrics", metricsHandler)
	return mux
}
