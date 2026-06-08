package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	kafkainfra "github.com/qsyy0921/IM/services/message-service/internal/infrastructure/kafka"
	postgresinfra "github.com/qsyy0921/IM/services/message-service/internal/infrastructure/postgres"
	"github.com/qsyy0921/IM/services/message-service/internal/trigger/outbox"
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
		log.Println("message-service runtime wiring is idle; set NEXUSIM_MESSAGE_SERVICE_MODE=outbox-relay to run the outbox relay")
		return nil
	case "outbox-relay":
		return runOutboxRelay()
	default:
		return errors.New("unsupported NEXUSIM_MESSAGE_SERVICE_MODE")
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

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()

	producer, err := kafkainfra.NewWriterProducer(brokers)
	if err != nil {
		return err
	}
	defer func() {
		if err := producer.Close(); err != nil {
			log.Printf("close kafka producer: %v", err)
		}
	}()

	relay := outbox.NewRelay(
		postgresinfra.NewOutboxStore(pool),
		producer,
		outbox.Config{
			Topic:          envString("NEXUSIM_KAFKA_TOPIC", outbox.TopicConversationTimelineEvents),
			BatchSize:      envInt("NEXUSIM_OUTBOX_BATCH_SIZE", 500),
			PollInterval:   envDuration("NEXUSIM_OUTBOX_POLL_INTERVAL", time.Second),
			MaxAttempts:    envInt("NEXUSIM_OUTBOX_MAX_ATTEMPTS", 5),
			RetryBaseDelay: envDuration("NEXUSIM_OUTBOX_RETRY_BASE_DELAY", time.Second),
		},
	)
	log.Println("message-service outbox relay started")
	return relay.Run(ctx)
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
