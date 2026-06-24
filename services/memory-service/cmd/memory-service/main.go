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
	memorygrpc "github.com/qsyy0921/IM/services/memory-service/internal/api/grpc"
	"github.com/qsyy0921/IM/services/memory-service/internal/app"
	kafkainfra "github.com/qsyy0921/IM/services/memory-service/internal/infrastructure/kafka"
	postgresinfra "github.com/qsyy0921/IM/services/memory-service/internal/infrastructure/postgres"
	"github.com/qsyy0921/IM/services/memory-service/internal/trigger/timeline"
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
	mode := memoryServiceModeFromEnv()
	if err := validateMemoryServiceMode(mode); err != nil {
		return err
	}
	switch mode {
	case "noop":
		return runNoop(ctx)
	case "grpc":
		return runGRPC(ctx)
	case "timeline-consumer":
		return runTimelineConsumer(ctx)
	default:
		return fmt.Errorf("unsupported NEXUSIM_MEMORY_SERVICE_MODE %q", mode)
	}
}

func runNoop(ctx context.Context) error {
	debugAddr, err := memoryDebugAddrFromEnv()
	if err != nil {
		return err
	}
	stopDebug, err := startDebugServer(ctx, debugAddr)
	if err != nil {
		return err
	}
	defer stopDebug()

	log.Println("memory-service noop mode: set NEXUSIM_MEMORY_SERVICE_MODE=grpc or timeline-consumer to start runtime roles")
	<-ctx.Done()
	return nil
}

func memoryServiceModeFromEnv() string {
	mode := os.Getenv("NEXUSIM_MEMORY_SERVICE_MODE")
	if mode == "" {
		mode = "noop"
	}
	return mode
}

func validateMemoryServiceMode(mode string) error {
	switch mode {
	case "noop", "grpc", "timeline-consumer":
		return nil
	default:
		return fmt.Errorf("unsupported NEXUSIM_MEMORY_SERVICE_MODE %q", mode)
	}
}

func runGRPC(ctx context.Context) error {
	debugAddr, err := memoryDebugAddrFromEnv()
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

	addr := envString("NEXUSIM_MEMORY_GRPC_ADDR", "127.0.0.1:10580")
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	repository := postgresinfra.NewRepository(pool)
	server := grpc.NewServer()
	memorygrpc.Register(server, memorygrpc.NewServer(
		app.NewQueryMemoryEventsUseCase(repository),
		app.NewGetMemoryEventUseCase(repository),
		app.NewListProfileAggregatesUseCase(repository),
		app.NewRecomputeProfileAggregateUseCase(repository),
		app.NewSubmitMemoryCandidateUseCase(repository),
		app.NewReviewMemoryCandidateUseCase(repository),
	))

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.Serve(listener)
	}()
	log.Printf("memory-service grpc listening on %s", addr)

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

func runTimelineConsumer(ctx context.Context) error {
	debugAddr, err := memoryDebugAddrFromEnv()
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
	topic := envString("NEXUSIM_TIMELINE_TOPIC", timeline.TopicConversationTimelineEvents)
	groupID := envString("NEXUSIM_MEMORY_CONSUMER_GROUP", "nexusim-memory-service")
	consumer, err := kafkainfra.NewReaderConsumer(kafkainfra.ReaderConfig{
		Brokers: brokers,
		Topic:   topic,
		GroupID: groupID,
	})
	if err != nil {
		return err
	}
	defer consumer.Close()

	repository := postgresinfra.NewRepository(pool)
	worker := timeline.NewWorker(
		consumer,
		app.NewProjectTimelineEventUseCase(repository),
		groupID,
		timeline.Config{
			ErrorBackoff: envDuration("NEXUSIM_MEMORY_TIMELINE_CONSUMER_ERROR_BACKOFF", time.Second),
			Logf:         log.Printf,
		},
	)
	log.Printf("memory-service timeline consumer started topic=%s group=%s", topic, groupID)
	return worker.Run(ctx)
}

func memoryDebugAddr() string {
	if addr := strings.TrimSpace(os.Getenv("NEXUSIM_MEMORY_DEBUG_ADDR")); addr != "" {
		return addr
	}
	return strings.TrimSpace(os.Getenv("NEXUSIM_DEBUG_ADDR"))
}

func memoryDebugAddrFromEnv() (string, error) {
	addr := memoryDebugAddr()
	allowPublic, _, err := envOptionalBool("NEXUSIM_MEMORY_DEBUG_ALLOW_PUBLIC")
	if err != nil {
		return "", err
	}
	return addr, validateMemoryDebugListenerConfig(addr, allowPublic)
}

func validateMemoryDebugListenerConfig(addr string, allowPublic bool) error {
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
	return errors.New("memory-service debug listener address is non-private; set NEXUSIM_MEMORY_DEBUG_ALLOW_PUBLIC=true to allow")
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

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func envDuration(name string, defaultValue time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return defaultValue
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
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
			log.Printf("memory-service debug server stopped: %v", err)
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
		_, _ = writer.Write([]byte("nexusim_memory_service_info 1\n"))
	}
	mux.HandleFunc("/debug/metrics", metricsHandler)
	mux.HandleFunc("/metrics", metricsHandler)
	return mux
}
