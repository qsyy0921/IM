package main

import (
	"context"
	"errors"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	contactsgrpc "github.com/qsyy0921/IM/services/contacts-service/internal/api/grpc"
	"github.com/qsyy0921/IM/services/contacts-service/internal/app"
	kafkainfra "github.com/qsyy0921/IM/services/contacts-service/internal/infrastructure/kafka"
	postgresinfra "github.com/qsyy0921/IM/services/contacts-service/internal/infrastructure/postgres"
	"github.com/qsyy0921/IM/services/contacts-service/internal/trigger/outbox"
	"google.golang.org/grpc"
)

func main() {
	if err := run(); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatal(err)
	}
}

func run() error {
	mode := strings.TrimSpace(os.Getenv("NEXUSIM_CONTACTS_SERVICE_MODE"))
	switch mode {
	case "", "noop":
		log.Println("contacts-service runtime wiring is idle; set NEXUSIM_CONTACTS_SERVICE_MODE=grpc or outbox-relay")
		return nil
	case "grpc":
		return runGRPC()
	case "outbox-relay":
		return runOutboxRelay()
	default:
		return errors.New("unsupported NEXUSIM_CONTACTS_SERVICE_MODE")
	}
}

func runGRPC() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := openPGPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	addr := envString("NEXUSIM_CONTACTS_GRPC_ADDR", "0.0.0.0:10500")
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	repository := postgresinfra.NewRepository(pool)
	server := grpc.NewServer()
	contactsgrpc.Register(server, contactsgrpc.NewServer(
		app.NewSendContactRequestUseCase(repository),
		app.NewRespondContactRequestUseCase(repository),
		app.NewListContactsUseCase(repository),
		app.NewGetContactStateUseCase(repository),
		app.NewDeleteContactUseCase(repository),
		app.NewBlockContactUseCase(repository),
		app.NewUpdateContactRemarkUseCase(repository),
	))

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.Serve(listener)
	}()
	log.Printf("contacts-service grpc listening on %s", addr)

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

	pool, err := openPGPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	brokers := splitCSV(os.Getenv("NEXUSIM_KAFKA_BROKERS"))
	producer, err := kafkainfra.NewWriterProducer(brokers)
	if err != nil {
		return err
	}
	defer producer.Close()

	topic := envString("NEXUSIM_CONTACT_EVENTS_TOPIC", outbox.TopicContactEvents)
	relay := outbox.NewRelay(
		postgresinfra.NewOutboxStore(pool),
		producer,
		outbox.Config{
			Topic:          topic,
			BatchSize:      envInt("NEXUSIM_CONTACTS_OUTBOX_BATCH_SIZE", 500),
			PollInterval:   envDuration("NEXUSIM_CONTACTS_OUTBOX_POLL_INTERVAL", time.Second),
			MaxAttempts:    envInt("NEXUSIM_CONTACTS_OUTBOX_MAX_ATTEMPTS", 5),
			RetryBaseDelay: envDuration("NEXUSIM_CONTACTS_OUTBOX_RETRY_BASE_DELAY", time.Second),
		},
	)
	log.Printf("contacts-service outbox relay started topic=%s", topic)
	return relay.Run(ctx)
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
	if maxConns := envInt("NEXUSIM_CONTACTS_PG_MAX_CONNS", 0); maxConns > 0 {
		config.MaxConns = int32(maxConns)
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
