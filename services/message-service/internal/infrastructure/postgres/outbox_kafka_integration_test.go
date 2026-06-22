package postgres

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	kafkainfra "github.com/qsyy0921/IM/services/message-service/internal/infrastructure/kafka"
	"github.com/qsyy0921/IM/services/message-service/internal/trigger/outbox"
	"github.com/qsyy0921/IM/services/message-service/internal/types"
)

func TestOutboxRelayPublishesToKafkaIntegration(t *testing.T) {
	brokersEnv := os.Getenv("NEXUSIM_KAFKA_BROKERS")
	if brokersEnv == "" {
		t.Skip("set NEXUSIM_KAFKA_BROKERS to run outbox relay Kafka integration test")
	}
	ctx := context.Background()
	pool := openIntegrationPool(t, ctx)
	defer pool.Close()
	applyMessageMigration(t, ctx, pool)
	resetMessageCoreTables(t, ctx, pool)

	input := testAppendInput(types.TenantID(fmt.Sprintf("tenant-outbox-kafka-%d", time.Now().UnixNano())), "client-1", []byte(`{"text":"hello"}`))
	repo := NewMessageRepository(pool)
	if _, err := repo.AppendMessage(ctx, input); err != nil {
		t.Fatalf("append message: %v", err)
	}

	producer, err := kafkainfra.NewWriterProducer(splitCSV(brokersEnv))
	if err != nil {
		t.Fatalf("create kafka producer: %v", err)
	}
	defer producer.Close()

	relay := outbox.NewRelay(
		NewOutboxStore(pool),
		producer,
		outbox.Config{
			Topic:          envOr("NEXUSIM_KAFKA_TOPIC", outbox.TopicConversationTimelineEvents),
			BatchSize:      10,
			MaxAttempts:    3,
			RetryBaseDelay: time.Millisecond,
		},
	)
	stats, err := relay.RunOnce(ctx)
	if err != nil {
		t.Fatalf("run outbox relay once: %v", err)
	}
	if stats.Fetched != 1 || stats.Published != 1 {
		t.Fatalf("unexpected relay stats: %+v", stats)
	}
	if status := readOutboxStatus(t, ctx, pool, input.Command.AuthContext.TenantID); status.Status != types.OutboxStatusPublished {
		t.Fatalf("expected outbox published, got %+v", status)
	}
}

func envOr(name string, defaultValue string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return defaultValue
	}
	return value
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
