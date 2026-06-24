package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	searchgrpc "github.com/qsyy0921/IM/services/search-service/internal/api/grpc"
	"github.com/qsyy0921/IM/services/search-service/internal/app"
	kafkainfra "github.com/qsyy0921/IM/services/search-service/internal/infrastructure/kafka"
	opensearchinfra "github.com/qsyy0921/IM/services/search-service/internal/infrastructure/opensearch"
	postgresinfra "github.com/qsyy0921/IM/services/search-service/internal/infrastructure/postgres"
	"github.com/qsyy0921/IM/services/search-service/internal/trigger/timeline"
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
	mode := searchServiceModeFromEnv()
	if err := validateSearchServiceMode(mode); err != nil {
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
		return fmt.Errorf("unsupported NEXUSIM_SEARCH_SERVICE_MODE %q", mode)
	}
}

func runNoop(ctx context.Context) error {
	debugAddr, err := searchDebugAddrFromEnv()
	if err != nil {
		return err
	}
	stopDebug, err := startDebugServer(ctx, debugAddr)
	if err != nil {
		return err
	}
	defer stopDebug()

	log.Println("search-service noop mode: set NEXUSIM_SEARCH_SERVICE_MODE=grpc or timeline-consumer to start runtime roles")
	<-ctx.Done()
	return nil
}

func searchServiceModeFromEnv() string {
	mode := os.Getenv("NEXUSIM_SEARCH_SERVICE_MODE")
	if mode == "" {
		mode = "noop"
	}
	return mode
}

func validateSearchServiceMode(mode string) error {
	switch mode {
	case "noop", "grpc", "timeline-consumer":
		return nil
	default:
		return fmt.Errorf("unsupported NEXUSIM_SEARCH_SERVICE_MODE %q", mode)
	}
}

func runGRPC(ctx context.Context) error {
	debugAddr, err := searchDebugAddrFromEnv()
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

	addr := envString("NEXUSIM_SEARCH_GRPC_ADDR", "127.0.0.1:10570")
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	postgresRepository := postgresinfra.NewRepository(pool)
	repository, err := newSearchMessagesRepositoryFromEnv(postgresRepository)
	if err != nil {
		return err
	}
	server := grpc.NewServer()
	searchgrpc.Register(server, searchgrpc.NewServer(app.NewSearchMessagesUseCase(repository)))

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.Serve(listener)
	}()
	log.Printf("search-service grpc listening on %s", addr)

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
	debugAddr, err := searchDebugAddrFromEnv()
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
	groupID := envString("NEXUSIM_SEARCH_CONSUMER_GROUP", "nexusim-search-service")
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
			ErrorBackoff: envDuration("NEXUSIM_SEARCH_TIMELINE_CONSUMER_ERROR_BACKOFF", time.Second),
			Logf:         log.Printf,
		},
	)
	log.Printf("search-service timeline consumer started topic=%s group=%s", topic, groupID)
	return worker.Run(ctx)
}

func newSearchMessagesRepositoryFromEnv(postgresRepository *postgresinfra.Repository) (app.SearchMessagesRepository, error) {
	backend := searchBackendFromEnv()
	switch backend {
	case "postgres":
		return postgresRepository, nil
	case "opensearch":
		config, err := opensearchConfigFromEnv()
		if err != nil {
			return nil, err
		}
		return opensearchinfra.NewRepository(config, postgresRepository)
	default:
		return nil, fmt.Errorf("unsupported NEXUSIM_SEARCH_BACKEND %q", backend)
	}
}

func searchBackendFromEnv() string {
	return strings.ToLower(envString("NEXUSIM_SEARCH_BACKEND", "postgres"))
}

func opensearchConfigFromEnv() (opensearchinfra.Config, error) {
	endpoint := strings.TrimSpace(os.Getenv("NEXUSIM_SEARCH_OPENSEARCH_ENDPOINT"))
	allowInsecureHTTP, _, err := envOptionalBool("NEXUSIM_SEARCH_OPENSEARCH_ALLOW_INSECURE_HTTP")
	if err != nil {
		return opensearchinfra.Config{}, err
	}
	if err := validateOpenSearchEndpointSecurity(endpoint, allowInsecureHTTP); err != nil {
		return opensearchinfra.Config{}, err
	}
	candidateOverfetchFactor, err := envInt("NEXUSIM_SEARCH_OPENSEARCH_CANDIDATE_OVERFETCH_FACTOR", 5)
	if err != nil {
		return opensearchinfra.Config{}, err
	}
	maxCandidateFetch, err := envInt("NEXUSIM_SEARCH_OPENSEARCH_MAX_CANDIDATE_FETCH", 500)
	if err != nil {
		return opensearchinfra.Config{}, err
	}
	return opensearchinfra.Config{
		Endpoint:                 endpoint,
		Index:                    os.Getenv("NEXUSIM_SEARCH_OPENSEARCH_INDEX"),
		Username:                 os.Getenv("NEXUSIM_SEARCH_OPENSEARCH_USERNAME"),
		Password:                 os.Getenv("NEXUSIM_SEARCH_OPENSEARCH_PASSWORD"),
		APIKey:                   os.Getenv("NEXUSIM_SEARCH_OPENSEARCH_API_KEY"),
		Timeout:                  envDuration("NEXUSIM_SEARCH_OPENSEARCH_TIMEOUT", 2*time.Second),
		CandidateOverfetchFactor: candidateOverfetchFactor,
		MaxCandidateFetch:        maxCandidateFetch,
	}, nil
}

func validateOpenSearchEndpointSecurity(raw string, allowInsecureHTTP bool) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return errors.New("NEXUSIM_SEARCH_OPENSEARCH_ENDPOINT is required")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("NEXUSIM_SEARCH_OPENSEARCH_ENDPOINT must use http or https")
	}
	if parsed.Host == "" {
		return errors.New("NEXUSIM_SEARCH_OPENSEARCH_ENDPOINT host is required")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("NEXUSIM_SEARCH_OPENSEARCH_ENDPOINT must not include credentials, query, or fragment")
	}
	if parsed.Scheme != "http" || allowInsecureHTTP {
		return nil
	}
	host := parsed.Hostname()
	if host == "localhost" {
		return nil
	}
	ip := net.ParseIP(host)
	if ip != nil && (ip.IsLoopback() || ip.IsPrivate()) {
		return nil
	}
	return errors.New("NEXUSIM_SEARCH_OPENSEARCH_ENDPOINT uses non-private http; set NEXUSIM_SEARCH_OPENSEARCH_ALLOW_INSECURE_HTTP=true to allow")
}

func searchDebugAddr() string {
	if addr := strings.TrimSpace(os.Getenv("NEXUSIM_SEARCH_DEBUG_ADDR")); addr != "" {
		return addr
	}
	return strings.TrimSpace(os.Getenv("NEXUSIM_DEBUG_ADDR"))
}

func searchDebugAddrFromEnv() (string, error) {
	addr := searchDebugAddr()
	allowPublic, _, err := envOptionalBool("NEXUSIM_SEARCH_DEBUG_ALLOW_PUBLIC")
	if err != nil {
		return "", err
	}
	return addr, validateSearchDebugListenerConfig(addr, allowPublic)
}

func validateSearchDebugListenerConfig(addr string, allowPublic bool) error {
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
	return errors.New("search-service debug listener address is non-private; set NEXUSIM_SEARCH_DEBUG_ALLOW_PUBLIC=true to allow")
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

func envInt(name string, defaultValue int) (int, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return defaultValue, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, err
	}
	if value <= 0 {
		return 0, fmt.Errorf("%s must be positive", name)
	}
	return value, nil
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
			log.Printf("search-service debug server stopped: %v", err)
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
		_, _ = writer.Write([]byte("nexusim_search_service_info 1\n"))
	}
	mux.HandleFunc("/debug/metrics", metricsHandler)
	mux.HandleFunc("/metrics", metricsHandler)
	return mux
}
