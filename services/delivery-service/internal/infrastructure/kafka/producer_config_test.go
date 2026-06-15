package kafka

import (
	"testing"
	"time"

	kafkago "github.com/segmentio/kafka-go"
)

func TestNewWriterProducerRejectsEmptyBrokerList(t *testing.T) {
	if _, err := NewWriterProducer(nil); err == nil {
		t.Fatalf("expected brokers required error")
	}
}

func TestNewWriterProducerConfiguresRetryBudget(t *testing.T) {
	producer, err := NewWriterProducer([]string{"127.0.0.1:9092"})
	if err != nil {
		t.Fatalf("create producer: %v", err)
	}
	if producer.writer == nil {
		t.Fatalf("expected writer to be configured")
	}
	writer := producer.writer
	if writer.RequiredAcks != kafkago.RequireAll {
		t.Fatalf("expected acks=all, got %v", writer.RequiredAcks)
	}
	if writer.AllowAutoTopicCreation {
		t.Fatalf("expected auto topic creation disabled")
	}
	if kafkaProducerMaxAttempts != 5 {
		t.Fatalf("unexpected max attempts constant: %d", kafkaProducerMaxAttempts)
	}
	if writer.MaxAttempts != kafkaProducerMaxAttempts {
		t.Fatalf("unexpected max attempts: %d", writer.MaxAttempts)
	}
	if kafkaProducerWriteBackoffMin != 100*time.Millisecond {
		t.Fatalf("unexpected min write backoff constant: %s", kafkaProducerWriteBackoffMin)
	}
	if writer.WriteBackoffMin != kafkaProducerWriteBackoffMin {
		t.Fatalf("unexpected min write backoff: %s", writer.WriteBackoffMin)
	}
	if kafkaProducerWriteBackoffMax != time.Second {
		t.Fatalf("unexpected max write backoff constant: %s", kafkaProducerWriteBackoffMax)
	}
	if writer.WriteBackoffMax != kafkaProducerWriteBackoffMax {
		t.Fatalf("unexpected max write backoff: %s", writer.WriteBackoffMax)
	}
}
