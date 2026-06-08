package kafka

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestNewWriterProducerRequiresBrokers(t *testing.T) {
	if _, err := NewWriterProducer(nil); err == nil {
		t.Fatalf("expected brokers required error")
	}
}

func TestWriterProducerPublishesIntegration(t *testing.T) {
	brokersEnv := os.Getenv("NEXUSIM_KAFKA_BROKERS")
	if brokersEnv == "" {
		t.Skip("set NEXUSIM_KAFKA_BROKERS to run Kafka producer integration test")
	}
	topic := os.Getenv("NEXUSIM_KAFKA_TOPIC")
	if topic == "" {
		topic = "conversation.timeline.events"
	}
	producer, err := NewWriterProducer(splitCSV(brokersEnv))
	if err != nil {
		t.Fatalf("create producer: %v", err)
	}
	defer func() {
		if err := producer.Close(); err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("close producer: %v", err)
		}
	}()

	if err := producer.Publish(
		context.Background(),
		topic,
		[]byte("tenant-it:conversation-it"),
		[]byte("kafka-producer-integration"),
	); err != nil {
		t.Fatalf("publish kafka message: %v", err)
	}
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
