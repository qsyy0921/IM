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
	knowledgegrpc "github.com/qsyy0921/IM/services/knowledge-ingestion-service/internal/api/grpc"
	"github.com/qsyy0921/IM/services/knowledge-ingestion-service/internal/app"
	kafkainfra "github.com/qsyy0921/IM/services/knowledge-ingestion-service/internal/infrastructure/kafka"
	postgresinfra "github.com/qsyy0921/IM/services/knowledge-ingestion-service/internal/infrastructure/postgres"
	"github.com/qsyy0921/IM/services/knowledge-ingestion-service/internal/trigger/outbox"
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
	mode := knowledgeIngestionModeFromEnv()
	if err := validateKnowledgeIngestionMode(mode); err != nil {
		return err
	}
	switch mode {
	case "noop":
		return runNoop(ctx)
	case "grpc":
		return runGRPC(ctx)
	case "outbox-relay":
		return runOutboxRelay(ctx)
	default:
		return fmt.Errorf("unsupported NEXUSIM_KNOWLEDGE_INGESTION_SERVICE_MODE %q", mode)
	}
}

func runNoop(ctx context.Context) error {
	debugAddr, err := knowledgeIngestionDebugAddrFromEnv()
	if err != nil {
		return err
	}
	stopDebug, err := startDebugServer(ctx, debugAddr)
	if err != nil {
		return err
	}
	defer stopDebug()
	log.Println("knowledge-ingestion-service noop mode: set NEXUSIM_KNOWLEDGE_INGESTION_SERVICE_MODE=grpc to start runtime role")
	<-ctx.Done()
	return nil
}

func runGRPC(ctx context.Context) error {
	debugAddr, err := knowledgeIngestionDebugAddrFromEnv()
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

	addr := envString("NEXUSIM_KNOWLEDGE_INGESTION_GRPC_ADDR", "127.0.0.1:10740")
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	repository := postgresinfra.NewRepository(pool)
	ids := app.NewRandomIDGenerator()
	server := grpc.NewServer()
	knowledgegrpc.Register(server, knowledgegrpc.NewServer(
		app.NewCreateKnowledgeSourceUseCase(repository, ids),
		app.NewSubmitIngestionJobUseCase(repository, ids),
		app.NewGetIngestionJobUseCase(repository),
		app.NewListKnowledgeChunksUseCase(repository),
	))

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.Serve(listener)
	}()
	log.Printf("knowledge-ingestion-service grpc listening on %s", addr)

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
	debugAddr, err := knowledgeIngestionDebugAddrFromEnv()
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
			log.Printf("knowledge-ingestion-service outbox relay producer close failed: %v", closeErr)
		}
	}()

	relay := outbox.NewRelay(
		postgresinfra.NewOutboxStore(pool),
		producer,
		outbox.Config{
			Topic:          envString("NEXUSIM_KNOWLEDGE_EVENTS_TOPIC", outbox.TopicKnowledgeEvents),
			BatchSize:      envInt("NEXUSIM_KNOWLEDGE_OUTBOX_BATCH_SIZE", 500),
			PollInterval:   envDuration("NEXUSIM_KNOWLEDGE_OUTBOX_POLL_INTERVAL", time.Second),
			MaxAttempts:    envInt("NEXUSIM_KNOWLEDGE_OUTBOX_MAX_ATTEMPTS", 5),
			RetryBaseDelay: envDuration("NEXUSIM_KNOWLEDGE_OUTBOX_RETRY_BASE_DELAY", time.Second),
			ErrorBackoff:   envDuration("NEXUSIM_KNOWLEDGE_OUTBOX_ERROR_BACKOFF", time.Second),
			Logf:           log.Printf,
		},
	)
	log.Printf("knowledge-ingestion-service outbox relay publishing to %s via brokers %s", envString("NEXUSIM_KNOWLEDGE_EVENTS_TOPIC", outbox.TopicKnowledgeEvents), strings.Join(brokers, ","))
	return relay.Run(ctx)
}

func knowledgeIngestionModeFromEnv() string {
	mode := strings.TrimSpace(os.Getenv("NEXUSIM_KNOWLEDGE_INGESTION_SERVICE_MODE"))
	if mode == "" {
		mode = "noop"
	}
	return mode
}

func validateKnowledgeIngestionMode(mode string) error {
	switch mode {
	case "noop", "grpc", "outbox-relay":
		return nil
	default:
		return fmt.Errorf("unsupported NEXUSIM_KNOWLEDGE_INGESTION_SERVICE_MODE %q", mode)
	}
}

func knowledgeIngestionDebugAddr() string {
	if addr := strings.TrimSpace(os.Getenv("NEXUSIM_KNOWLEDGE_INGESTION_DEBUG_ADDR")); addr != "" {
		return addr
	}
	return strings.TrimSpace(os.Getenv("NEXUSIM_DEBUG_ADDR"))
}

func knowledgeIngestionDebugAddrFromEnv() (string, error) {
	addr := knowledgeIngestionDebugAddr()
	allowPublic, _, err := envOptionalBool("NEXUSIM_KNOWLEDGE_INGESTION_DEBUG_ALLOW_PUBLIC")
	if err != nil {
		return "", err
	}
	return addr, validateKnowledgeIngestionDebugListenerConfig(addr, allowPublic)
}

func validateKnowledgeIngestionDebugListenerConfig(addr string, allowPublic bool) error {
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
	return errors.New("knowledge-ingestion debug listener address is non-private; set NEXUSIM_KNOWLEDGE_INGESTION_DEBUG_ALLOW_PUBLIC=true to allow")
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
			log.Printf("knowledge-ingestion debug server stopped: %v", err)
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
		_, _ = writer.Write([]byte("# HELP nexusim_knowledge_ingestion_service_info Static knowledge-ingestion-service info metric.\n"))
		_, _ = writer.Write([]byte("# TYPE nexusim_knowledge_ingestion_service_info gauge\n"))
		_, _ = writer.Write([]byte("nexusim_knowledge_ingestion_service_info 1\n"))
	}
	mux.HandleFunc("/metrics", metricsHandler)
	mux.HandleFunc("/debug/metrics", metricsHandler)
	return mux
}
