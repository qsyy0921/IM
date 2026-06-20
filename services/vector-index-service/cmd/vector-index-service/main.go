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
	vectorgrpc "github.com/qsyy0921/IM/services/vector-index-service/internal/api/grpc"
	"github.com/qsyy0921/IM/services/vector-index-service/internal/app"
	embeddinginfra "github.com/qsyy0921/IM/services/vector-index-service/internal/infrastructure/embedding"
	kafkainfra "github.com/qsyy0921/IM/services/vector-index-service/internal/infrastructure/kafka"
	postgresinfra "github.com/qsyy0921/IM/services/vector-index-service/internal/infrastructure/postgres"
	rpcinfra "github.com/qsyy0921/IM/services/vector-index-service/internal/infrastructure/rpc"
	"github.com/qsyy0921/IM/services/vector-index-service/internal/trigger/embedding"
	"github.com/qsyy0921/IM/services/vector-index-service/internal/trigger/outbox"
	"github.com/qsyy0921/IM/services/vector-index-service/internal/trigger/rebuild"
	"github.com/qsyy0921/IM/services/vector-index-service/internal/types"
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
	mode := vectorIndexModeFromEnv()
	if err := validateVectorIndexMode(mode); err != nil {
		return err
	}
	switch mode {
	case "noop":
		return runNoop(ctx)
	case "grpc":
		return runGRPC(ctx)
	case "outbox-relay":
		return runOutboxRelay(ctx)
	case "rebuild-worker":
		return runRebuildWorker(ctx)
	case "embedding-worker":
		return runEmbeddingWorker(ctx)
	default:
		return fmt.Errorf("unsupported NEXUSIM_VECTOR_INDEX_SERVICE_MODE %q", mode)
	}
}

func runNoop(ctx context.Context) error {
	debugAddr, err := vectorIndexDebugAddrFromEnv()
	if err != nil {
		return err
	}
	stopDebug, err := startDebugServer(ctx, debugAddr)
	if err != nil {
		return err
	}
	defer stopDebug()
	log.Println("vector-index-service noop mode: set NEXUSIM_VECTOR_INDEX_SERVICE_MODE=grpc to start runtime role")
	<-ctx.Done()
	return nil
}

func runGRPC(ctx context.Context) error {
	debugAddr, err := vectorIndexDebugAddrFromEnv()
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

	addr := envString("NEXUSIM_VECTOR_INDEX_GRPC_ADDR", "127.0.0.1:10760")
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	repository := postgresinfra.NewRepository(pool)
	ids := app.NewRandomIDGenerator()
	server := grpc.NewServer()
	vectorgrpc.Register(server, vectorgrpc.NewServer(
		app.NewUpsertVectorItemUseCase(repository, ids),
		app.NewTombstoneVectorItemUseCase(repository, ids),
		app.NewSearchVectorsUseCase(repository),
		app.NewRequestVectorRebuildUseCase(repository, ids),
		app.NewGetVectorIndexJobUseCase(repository),
	))

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.Serve(listener)
	}()
	log.Printf("vector-index-service grpc listening on %s", addr)

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

func runOutboxRelay(ctx context.Context) error {
	debugAddr, err := vectorIndexDebugAddrFromEnv()
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
			log.Printf("vector-index-service outbox relay producer close failed: %v", closeErr)
		}
	}()

	relay := outbox.NewRelay(
		postgresinfra.NewOutboxStore(pool),
		producer,
		outbox.Config{
			Topic:          envString("NEXUSIM_VECTOR_EVENTS_TOPIC", outbox.TopicVectorEvents),
			BatchSize:      envInt("NEXUSIM_VECTOR_OUTBOX_BATCH_SIZE", 500),
			PollInterval:   envDuration("NEXUSIM_VECTOR_OUTBOX_POLL_INTERVAL", time.Second),
			MaxAttempts:    envInt("NEXUSIM_VECTOR_OUTBOX_MAX_ATTEMPTS", 5),
			RetryBaseDelay: envDuration("NEXUSIM_VECTOR_OUTBOX_RETRY_BASE_DELAY", time.Second),
			ErrorBackoff:   envDuration("NEXUSIM_VECTOR_OUTBOX_ERROR_BACKOFF", time.Second),
			Logf:           log.Printf,
		},
	)
	log.Printf("vector-index-service outbox relay publishing to %s via brokers %s", envString("NEXUSIM_VECTOR_EVENTS_TOPIC", outbox.TopicVectorEvents), strings.Join(brokers, ","))
	return relay.Run(ctx)
}

func runRebuildWorker(ctx context.Context) error {
	debugAddr, err := vectorIndexDebugAddrFromEnv()
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

	worker := rebuild.NewWorker(
		postgresinfra.NewRepository(pool),
		rebuild.Config{
			BatchSize:    envInt("NEXUSIM_VECTOR_REBUILD_BATCH_SIZE", 50),
			PollInterval: envDuration("NEXUSIM_VECTOR_REBUILD_POLL_INTERVAL", time.Second),
			ErrorBackoff: envDuration("NEXUSIM_VECTOR_REBUILD_ERROR_BACKOFF", time.Second),
			Logf:         log.Printf,
		},
	)
	log.Printf("vector-index-service rebuild worker started")
	return worker.Run(ctx)
}

func runEmbeddingWorker(ctx context.Context) error {
	debugAddr, err := vectorIndexDebugAddrFromEnv()
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

	taskFile := strings.TrimSpace(os.Getenv("NEXUSIM_VECTOR_EMBEDDING_TASKS_FILE"))
	source, err := embeddinginfra.NewFileTaskSource(taskFile)
	if err != nil {
		return err
	}
	modelAddr := strings.TrimSpace(os.Getenv("NEXUSIM_MODEL_GATEWAY_GRPC_ADDR"))
	modelClient, closeModel, err := rpcinfra.DialModelGatewayClient(
		ctx,
		modelAddr,
		envDuration("NEXUSIM_VECTOR_EMBEDDING_MODEL_TIMEOUT", 5*time.Second),
	)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := closeModel(); closeErr != nil {
			log.Printf("vector-index-service model-gateway client close failed: %v", closeErr)
		}
	}()

	repository := postgresinfra.NewRepository(pool)
	worker := embedding.NewWorker(
		source,
		modelClient,
		vectorUpsertAdapter{useCase: app.NewUpsertVectorItemUseCase(repository, app.NewRandomIDGenerator())},
		embedding.Config{
			BatchSize:    envInt("NEXUSIM_VECTOR_EMBEDDING_BATCH_SIZE", 50),
			PollInterval: envDuration("NEXUSIM_VECTOR_EMBEDDING_POLL_INTERVAL", time.Second),
			ErrorBackoff: envDuration("NEXUSIM_VECTOR_EMBEDDING_ERROR_BACKOFF", time.Second),
			Logf:         log.Printf,
		},
	)
	log.Printf("vector-index-service embedding worker started with file task source %s", taskFile)
	return worker.Run(ctx)
}

func vectorIndexModeFromEnv() string {
	mode := strings.TrimSpace(os.Getenv("NEXUSIM_VECTOR_INDEX_SERVICE_MODE"))
	if mode == "" {
		mode = "noop"
	}
	return mode
}

func validateVectorIndexMode(mode string) error {
	switch mode {
	case "noop", "grpc", "outbox-relay", "rebuild-worker", "embedding-worker":
		return nil
	default:
		return fmt.Errorf("unsupported NEXUSIM_VECTOR_INDEX_SERVICE_MODE %q", mode)
	}
}

func vectorIndexDebugAddr() string {
	if addr := strings.TrimSpace(os.Getenv("NEXUSIM_VECTOR_INDEX_DEBUG_ADDR")); addr != "" {
		return addr
	}
	return strings.TrimSpace(os.Getenv("NEXUSIM_DEBUG_ADDR"))
}

func vectorIndexDebugAddrFromEnv() (string, error) {
	addr := vectorIndexDebugAddr()
	allowPublic, _, err := envOptionalBool("NEXUSIM_VECTOR_INDEX_DEBUG_ALLOW_PUBLIC")
	if err != nil {
		return "", err
	}
	return addr, validateVectorIndexDebugListenerConfig(addr, allowPublic)
}

func validateVectorIndexDebugListenerConfig(addr string, allowPublic bool) error {
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
	return errors.New("vector-index debug listener address is non-private; set NEXUSIM_VECTOR_INDEX_DEBUG_ALLOW_PUBLIC=true to allow")
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
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func envDuration(name string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	value, err := time.ParseDuration(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
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
			log.Printf("vector-index debug server stopped: %v", err)
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
		_, _ = writer.Write([]byte("# HELP nexusim_vector_index_service_info Static vector-index-service info metric.\n"))
		_, _ = writer.Write([]byte("# TYPE nexusim_vector_index_service_info gauge\n"))
		_, _ = writer.Write([]byte("nexusim_vector_index_service_info 1\n"))
	}
	mux.HandleFunc("/metrics", metricsHandler)
	mux.HandleFunc("/debug/metrics", metricsHandler)
	return mux
}

type vectorUpsertAdapter struct {
	useCase app.UpsertVectorItemUseCase
}

func (adapter vectorUpsertAdapter) UpsertVectorItem(ctx context.Context, command types.UpsertVectorItemCommand) error {
	_, err := adapter.useCase.Execute(ctx, command)
	return err
}
