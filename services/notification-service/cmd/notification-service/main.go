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
	notificationgrpc "github.com/qsyy0921/IM/services/notification-service/internal/api/grpc"
	"github.com/qsyy0921/IM/services/notification-service/internal/app"
	"github.com/qsyy0921/IM/services/notification-service/internal/infrastructure/destinationhash"
	kafkainfra "github.com/qsyy0921/IM/services/notification-service/internal/infrastructure/kafka"
	postgresinfra "github.com/qsyy0921/IM/services/notification-service/internal/infrastructure/postgres"
	providerinfra "github.com/qsyy0921/IM/services/notification-service/internal/infrastructure/provider"
	"github.com/qsyy0921/IM/services/notification-service/internal/trigger/delivery"
	"github.com/qsyy0921/IM/services/notification-service/internal/trigger/outbox"
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
	mode := notificationServiceModeFromEnv()
	if err := validateNotificationServiceMode(mode); err != nil {
		return err
	}
	switch mode {
	case "noop":
		return runNoop(ctx)
	case "grpc":
		return runGRPC(ctx)
	case "delivery-worker":
		return runDeliveryWorker(ctx)
	case "outbox-relay":
		return runOutboxRelay(ctx)
	default:
		return fmt.Errorf("unsupported NEXUSIM_NOTIFICATION_SERVICE_MODE %q", mode)
	}
}

func runNoop(ctx context.Context) error {
	debugAddr, err := notificationDebugAddrFromEnv()
	if err != nil {
		return err
	}
	stopDebug, err := startDebugServer(ctx, debugAddr)
	if err != nil {
		return err
	}
	defer stopDebug()

	log.Println("notification-service noop mode: set NEXUSIM_NOTIFICATION_SERVICE_MODE=grpc to start runtime role")
	<-ctx.Done()
	return nil
}

func runGRPC(ctx context.Context) error {
	debugAddr, err := notificationDebugAddrFromEnv()
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

	hasher, err := destinationhash.NewHMACHasher(envString("NEXUSIM_NOTIFICATION_DESTINATION_HASH_KEY", "local-notification-destination-hash-key"))
	if err != nil {
		return err
	}
	addr := envString("NEXUSIM_NOTIFICATION_GRPC_ADDR", "127.0.0.1:10690")
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	repository := postgresinfra.NewRepository(pool)
	server := grpc.NewServer()
	notificationgrpc.Register(server, notificationgrpc.NewServer(
		app.NewCreateNotificationRequestUseCase(repository, hasher, app.NewRandomRequestIDGenerator()),
		app.NewGetNotificationStatusUseCase(repository),
		app.NewCancelNotificationRequestUseCase(repository),
	))

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.Serve(listener)
	}()
	log.Printf("notification-service grpc listening on %s", addr)

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

func runDeliveryWorker(ctx context.Context) error {
	debugAddr, err := notificationDebugAddrFromEnv()
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

	provider, classifier, providerID, err := notificationProviderFromEnv()
	if err != nil {
		return err
	}
	worker := delivery.NewWorker(
		postgresinfra.NewDeliveryStore(pool),
		provider,
		classifier,
		delivery.Config{
			ProviderID:     providerID,
			BatchSize:      envInt("NEXUSIM_NOTIFICATION_DELIVERY_BATCH_SIZE", 50),
			PollInterval:   envDuration("NEXUSIM_NOTIFICATION_DELIVERY_POLL_INTERVAL", time.Second),
			MaxAttempts:    envInt("NEXUSIM_NOTIFICATION_DELIVERY_MAX_ATTEMPTS", 3),
			RetryBaseDelay: envDuration("NEXUSIM_NOTIFICATION_DELIVERY_RETRY_BASE_DELAY", time.Second),
			ErrorBackoff:   envDuration("NEXUSIM_NOTIFICATION_DELIVERY_ERROR_BACKOFF", time.Second),
			Logf:           log.Printf,
		},
	)
	log.Printf("notification-service delivery worker using provider %s", providerID)
	return worker.Run(ctx)
}

func runOutboxRelay(ctx context.Context) error {
	debugAddr, err := notificationDebugAddrFromEnv()
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
			log.Printf("notification-service outbox relay producer close failed: %v", closeErr)
		}
	}()

	relay := outbox.NewRelay(
		postgresinfra.NewOutboxStore(pool),
		producer,
		outbox.Config{
			Topic:          envString("NEXUSIM_NOTIFICATION_EVENTS_TOPIC", outbox.TopicNotificationEvents),
			BatchSize:      envInt("NEXUSIM_NOTIFICATION_OUTBOX_BATCH_SIZE", 500),
			PollInterval:   envDuration("NEXUSIM_NOTIFICATION_OUTBOX_POLL_INTERVAL", time.Second),
			MaxAttempts:    envInt("NEXUSIM_NOTIFICATION_OUTBOX_MAX_ATTEMPTS", 5),
			RetryBaseDelay: envDuration("NEXUSIM_NOTIFICATION_OUTBOX_RETRY_BASE_DELAY", time.Second),
			ErrorBackoff:   envDuration("NEXUSIM_NOTIFICATION_OUTBOX_ERROR_BACKOFF", time.Second),
			Logf:           log.Printf,
		},
	)
	log.Printf("notification-service outbox relay publishing to %s via brokers %s", envString("NEXUSIM_NOTIFICATION_EVENTS_TOPIC", outbox.TopicNotificationEvents), strings.Join(brokers, ","))
	return relay.Run(ctx)
}

func notificationServiceModeFromEnv() string {
	mode := strings.TrimSpace(os.Getenv("NEXUSIM_NOTIFICATION_SERVICE_MODE"))
	if mode == "" {
		mode = "noop"
	}
	return mode
}

func validateNotificationServiceMode(mode string) error {
	switch mode {
	case "noop", "grpc", "delivery-worker", "outbox-relay":
		return nil
	default:
		return fmt.Errorf("unsupported NEXUSIM_NOTIFICATION_SERVICE_MODE %q", mode)
	}
}

func notificationProviderFromEnv() (delivery.Provider, delivery.FailureClassifier, string, error) {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("NEXUSIM_NOTIFICATION_PROVIDER_MODE")))
	if mode == "" {
		mode = "noop"
	}
	providerID := envString("NEXUSIM_NOTIFICATION_PROVIDER_ID", "local-"+mode)
	switch mode {
	case "noop", "fake":
		return providerinfra.NewNoopProvider(providerID), nil, providerID, nil
	case "webhook":
		provider, err := providerinfra.NewWebhookProvider(providerinfra.WebhookConfig{
			URL:         envString("NEXUSIM_NOTIFICATION_WEBHOOK_URL", ""),
			BearerToken: envString("NEXUSIM_NOTIFICATION_WEBHOOK_BEARER_TOKEN", ""),
			ProviderID:  providerID,
			Timeout:     envDuration("NEXUSIM_NOTIFICATION_WEBHOOK_TIMEOUT", 5*time.Second),
		})
		if err != nil {
			return nil, nil, "", err
		}
		return provider, providerinfra.NewWebhookFailureClassifier(), providerID, nil
	default:
		return nil, nil, "", fmt.Errorf("unsupported NEXUSIM_NOTIFICATION_PROVIDER_MODE %q", mode)
	}
}

func notificationDebugAddr() string {
	if addr := strings.TrimSpace(os.Getenv("NEXUSIM_NOTIFICATION_DEBUG_ADDR")); addr != "" {
		return addr
	}
	return strings.TrimSpace(os.Getenv("NEXUSIM_DEBUG_ADDR"))
}

func notificationDebugAddrFromEnv() (string, error) {
	addr := notificationDebugAddr()
	allowPublic, _, err := envOptionalBool("NEXUSIM_NOTIFICATION_DEBUG_ALLOW_PUBLIC")
	if err != nil {
		return "", err
	}
	return addr, validateNotificationDebugListenerConfig(addr, allowPublic)
}

func validateNotificationDebugListenerConfig(addr string, allowPublic bool) error {
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
	return errors.New("notification-service debug listener address is non-private; set NEXUSIM_NOTIFICATION_DEBUG_ALLOW_PUBLIC=true to allow")
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
			log.Printf("notification-service debug server stopped: %v", err)
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
		_, _ = writer.Write([]byte("# HELP nexusim_notification_service_info Static notification-service info metric.\n"))
		_, _ = writer.Write([]byte("# TYPE nexusim_notification_service_info gauge\n"))
		_, _ = writer.Write([]byte("nexusim_notification_service_info 1\n"))
	}
	mux.HandleFunc("/metrics", metricsHandler)
	mux.HandleFunc("/debug/metrics", metricsHandler)
	return mux
}
