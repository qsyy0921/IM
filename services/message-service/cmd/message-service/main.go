package main

import (
	"context"
	"errors"
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
	grpcapi "github.com/qsyy0921/IM/services/message-service/internal/api/grpc"
	"github.com/qsyy0921/IM/services/message-service/internal/app"
	kafkainfra "github.com/qsyy0921/IM/services/message-service/internal/infrastructure/kafka"
	metricsinfra "github.com/qsyy0921/IM/services/message-service/internal/infrastructure/metrics"
	postgresinfra "github.com/qsyy0921/IM/services/message-service/internal/infrastructure/postgres"
	rpcinfra "github.com/qsyy0921/IM/services/message-service/internal/infrastructure/rpc"
	"github.com/qsyy0921/IM/services/message-service/internal/trigger/outbox"
	"google.golang.org/grpc"
)

func main() {
	if err := run(); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatal(err)
	}
}

func run() error {
	mode := strings.TrimSpace(os.Getenv("NEXUSIM_MESSAGE_SERVICE_MODE"))
	switch mode {
	case "", "noop":
		log.Println("message-service runtime wiring is idle; set NEXUSIM_MESSAGE_SERVICE_MODE=grpc or outbox-relay")
		return nil
	case "grpc":
		return runGRPCServer()
	case "outbox-relay":
		return runOutboxRelay()
	default:
		return errors.New("unsupported NEXUSIM_MESSAGE_SERVICE_MODE")
	}
}

func runGRPCServer() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dsn := strings.TrimSpace(os.Getenv("NEXUSIM_PG_DSN"))
	if dsn == "" {
		return errors.New("NEXUSIM_PG_DSN is required")
	}
	listenAddr := envString("NEXUSIM_GRPC_ADDR", "0.0.0.0:10495")

	pool, err := openPGPool(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()

	metrics := metricsinfra.NewCollector()
	stopDebug, err := startDebugServer(ctx, envString("NEXUSIM_DEBUG_ADDR", ""), metricsinfra.NewHandler(metrics, pool))
	if err != nil {
		return err
	}
	defer stopDebug()

	policy := rpcinfra.NewStaticPolicy()
	policy.Allowed = envBool("NEXUSIM_MOCK_POLICY_ALLOWED", policy.Allowed)
	policy.PermissionVersion = envInt64("NEXUSIM_MOCK_PERMISSION_VERSION", policy.PermissionVersion)
	policy.Classification = envString("NEXUSIM_MOCK_CLASSIFICATION", policy.Classification)

	conversation := rpcinfra.NewStaticConversation()
	conversation.MemberVersion = envInt64("NEXUSIM_MOCK_MEMBER_VERSION", conversation.MemberVersion)
	conversation.PermissionVersion = policy.PermissionVersion
	conversation.FanoutPolicyVersion = envInt64("NEXUSIM_MOCK_FANOUT_POLICY_VERSION", conversation.FanoutPolicyVersion)

	repositoryOptions := []postgresinfra.MessageRepositoryOption{postgresinfra.WithMetrics(metrics)}
	if envBool("NEXUSIM_PG_BACKPRESSURE_ENABLED", false) {
		repositoryOptions = append(repositoryOptions, postgresinfra.WithBackpressure(postgresinfra.BackpressureConfig{
			Enabled:           true,
			MinAvailableConns: int32(envInt("NEXUSIM_PG_BACKPRESSURE_MIN_AVAILABLE_CONNS", 0)),
		}))
	}

	useCase := app.NewSendMessageUseCase(
		policy,
		conversation,
		rpcinfra.NoopSequencer{},
		postgresinfra.NewMessageRepository(pool, repositoryOptions...),
	)

	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return err
	}
	server := grpc.NewServer()
	grpcapi.Register(server, grpcapi.NewServer(useCase, grpcapi.WithMetrics(metrics)))

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.Serve(listener)
	}()
	log.Printf("message-service gRPC server started on %s", listenAddr)

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

func runOutboxRelay() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dsn := strings.TrimSpace(os.Getenv("NEXUSIM_PG_DSN"))
	if dsn == "" {
		return errors.New("NEXUSIM_PG_DSN is required")
	}
	brokers := splitCSV(os.Getenv("NEXUSIM_KAFKA_BROKERS"))
	if len(brokers) == 0 {
		return errors.New("NEXUSIM_KAFKA_BROKERS is required")
	}

	pool, err := openPGPool(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()

	metrics := metricsinfra.NewCollector()
	stopDebug, err := startDebugServer(ctx, envString("NEXUSIM_DEBUG_ADDR", ""), metricsinfra.NewHandler(metrics, pool))
	if err != nil {
		return err
	}
	defer stopDebug()

	producer, err := kafkainfra.NewWriterProducer(brokers)
	if err != nil {
		return err
	}
	defer func() {
		if err := producer.Close(); err != nil {
			log.Printf("close kafka producer: %v", err)
		}
	}()

	pollInterval := envDuration("NEXUSIM_OUTBOX_POLL_INTERVAL", time.Second)
	relay := outbox.NewRelay(
		postgresinfra.NewOutboxStore(pool),
		producer,
		outbox.Config{
			Topic:          envString("NEXUSIM_KAFKA_TOPIC", outbox.TopicConversationTimelineEvents),
			BatchSize:      envInt("NEXUSIM_OUTBOX_BATCH_SIZE", 500),
			WorkerCount:    envInt("NEXUSIM_OUTBOX_WORKERS", 1),
			PollInterval:   pollInterval,
			FailureBackoff: envDuration("NEXUSIM_OUTBOX_FAILURE_BACKOFF", pollInterval),
			MaxAttempts:    envInt("NEXUSIM_OUTBOX_MAX_ATTEMPTS", 5),
			RetryBaseDelay: envDuration("NEXUSIM_OUTBOX_RETRY_BASE_DELAY", time.Second),
			Metrics:        metrics,
		},
	)
	log.Printf(
		"message-service outbox relay started workers=%d batch_size=%d poll_interval=%s failure_backoff=%s",
		envInt("NEXUSIM_OUTBOX_WORKERS", 1),
		envInt("NEXUSIM_OUTBOX_BATCH_SIZE", 500),
		pollInterval,
		envDuration("NEXUSIM_OUTBOX_FAILURE_BACKOFF", pollInterval),
	)
	return relay.Run(ctx)
}

func startDebugServer(ctx context.Context, addr string, handler http.Handler) (func(), error) {
	if strings.TrimSpace(addr) == "" {
		return func() {}, nil
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	server := &http.Server{Handler: handler}
	done := make(chan struct{})
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	go func() {
		defer close(done)
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("message-service debug server stopped with error: %v", err)
		}
	}()
	log.Printf("message-service debug server started on %s", addr)
	return func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		<-done
	}, nil
}

func openPGPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	if maxConns := envInt("NEXUSIM_PG_MAX_CONNS", 0); maxConns > 0 {
		config.MaxConns = int32(maxConns)
	}
	if minConns := envInt("NEXUSIM_PG_MIN_CONNS", 0); minConns > 0 {
		config.MinConns = int32(minConns)
	}
	return pgxpool.NewWithConfig(ctx, config)
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

func envInt64(name string, fallback int64) int64 {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func envBool(name string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
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

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}
