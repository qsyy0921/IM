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
	grpcapi "github.com/qsyy0921/IM/services/receipt-service/internal/api/grpc"
	"github.com/qsyy0921/IM/services/receipt-service/internal/app"
	accessinfra "github.com/qsyy0921/IM/services/receipt-service/internal/infrastructure/access"
	kafkainfra "github.com/qsyy0921/IM/services/receipt-service/internal/infrastructure/kafka"
	postgresinfra "github.com/qsyy0921/IM/services/receipt-service/internal/infrastructure/postgres"
	"github.com/qsyy0921/IM/services/receipt-service/internal/trigger/delivery"
	"github.com/qsyy0921/IM/services/receipt-service/internal/trigger/outbox"
	"google.golang.org/grpc"
)

func main() {
	if err := run(); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatal(err)
	}
}

func run() error {
	mode := strings.TrimSpace(os.Getenv("NEXUSIM_RECEIPT_SERVICE_MODE"))
	switch mode {
	case "", "noop":
		log.Println("receipt-service runtime wiring is idle; set NEXUSIM_RECEIPT_SERVICE_MODE=grpc or delivery-consumer")
		return nil
	case "grpc":
		return runGRPCServer()
	case "delivery-consumer":
		return runDeliveryConsumer()
	case "outbox-relay":
		return runOutboxRelay()
	default:
		return errors.New("unsupported NEXUSIM_RECEIPT_SERVICE_MODE")
	}
}

func runGRPCServer() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := openPGPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	listenAddr := envString("NEXUSIM_RECEIPT_GRPC_ADDR", "0.0.0.0:10499")
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return err
	}
	server := grpc.NewServer()
	repository := postgresinfra.NewRepository(pool)
	access := accessinfra.NewStaticAllowAccess()
	grpcapi.Register(
		server,
		grpcapi.NewServer(
			app.NewMarkReadUseCase(repository, access),
			app.NewGetReceiptStateUseCase(repository, access),
		),
	)

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.Serve(listener)
	}()
	log.Printf("receipt-service gRPC server started on %s", listenAddr)

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

func runDeliveryConsumer() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := openPGPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	brokers := splitCSV(os.Getenv("NEXUSIM_KAFKA_BROKERS"))
	topic := envString("NEXUSIM_DELIVERY_EVENTS_TOPIC", "im.delivery.events")
	groupID := envString("NEXUSIM_RECEIPT_CONSUMER_GROUP", "nexusim-receipt-service")
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
	worker := delivery.NewWorker(
		consumer,
		app.NewProjectDeliveryEventUseCase(repository),
		groupID,
	)
	log.Printf("receipt-service delivery consumer started topic=%s group=%s", topic, groupID)
	return worker.Run(ctx)
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

	topic := envString("NEXUSIM_RECEIPT_EVENTS_TOPIC", outbox.TopicReceiptEvents)
	relay := outbox.NewRelay(
		postgresinfra.NewOutboxStore(pool),
		producer,
		outbox.Config{
			Topic:          topic,
			BatchSize:      envInt("NEXUSIM_RECEIPT_OUTBOX_BATCH_SIZE", 500),
			PollInterval:   envDuration("NEXUSIM_RECEIPT_OUTBOX_POLL_INTERVAL", time.Second),
			MaxAttempts:    envInt("NEXUSIM_RECEIPT_OUTBOX_MAX_ATTEMPTS", 5),
			RetryBaseDelay: envDuration("NEXUSIM_RECEIPT_OUTBOX_RETRY_BASE_DELAY", time.Second),
		},
	)
	log.Printf("receipt-service outbox relay started topic=%s", topic)
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
	if maxConns := envInt("NEXUSIM_RECEIPT_PG_MAX_CONNS", 0); maxConns > 0 {
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
