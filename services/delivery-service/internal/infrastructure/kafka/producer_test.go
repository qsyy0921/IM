package kafka

import (
	"testing"
	"time"

	kafkago "github.com/segmentio/kafka-go"
)

func TestNewWriterProducerRequiresBrokers(t *testing.T) {
	if _, err := NewWriterProducer(nil); err == nil {
		t.Fatalf("expected brokers required error")
	}
}

func TestNewWriterProducerConfig(t *testing.T) {
	producer, err := NewWriterProducer([]string{"127.0.0.1:9092"})
	if err != nil {
		t.Fatalf("create producer: %v", err)
	}
	defer func() {
		if err := producer.Close(); err != nil {
			t.Fatalf("close producer: %v", err)
		}
	}()

	writer := producer.writer
	if writer == nil {
		t.Fatalf("expected writer")
	}
	if _, ok := writer.Balancer.(*kafkago.Hash); !ok {
		t.Fatalf("expected hash balancer, got %T", writer.Balancer)
	}
	if writer.RequiredAcks != kafkago.RequireAll {
		t.Fatalf("required acks = %v, want %v", writer.RequiredAcks, kafkago.RequireAll)
	}
	if writer.AllowAutoTopicCreation {
		t.Fatalf("auto topic creation must be disabled")
	}
	if writer.BatchSize != 100 {
		t.Fatalf("batch size = %d, want 100", writer.BatchSize)
	}
	if writer.BatchTimeout != 10*time.Millisecond {
		t.Fatalf("batch timeout = %v, want 10ms", writer.BatchTimeout)
	}
	if writer.MaxAttempts != kafkaProducerMaxAttempts {
		t.Fatalf("max attempts = %d, want package constant %d", writer.MaxAttempts, kafkaProducerMaxAttempts)
	}
	if writer.MaxAttempts != 5 {
		t.Fatalf("max attempts = %d, want 5", writer.MaxAttempts)
	}
	if writer.WriteBackoffMin != kafkaProducerWriteBackoffMin {
		t.Fatalf("write backoff min = %v, want package constant %v", writer.WriteBackoffMin, kafkaProducerWriteBackoffMin)
	}
	if writer.WriteBackoffMin != 100*time.Millisecond {
		t.Fatalf("write backoff min = %v, want 100ms", writer.WriteBackoffMin)
	}
	if writer.WriteBackoffMax != kafkaProducerWriteBackoffMax {
		t.Fatalf("write backoff max = %v, want package constant %v", writer.WriteBackoffMax, kafkaProducerWriteBackoffMax)
	}
	if writer.WriteBackoffMax != time.Second {
		t.Fatalf("write backoff max = %v, want 1s", writer.WriteBackoffMax)
	}
	if writer.WriteTimeout != 5*time.Second {
		t.Fatalf("write timeout = %v, want 5s", writer.WriteTimeout)
	}
	if writer.ReadTimeout != 5*time.Second {
		t.Fatalf("read timeout = %v, want 5s", writer.ReadTimeout)
	}
}

func TestNewWriterProducerWithConfigOverridesBatching(t *testing.T) {
	producer, err := NewWriterProducerWithConfig([]string{"127.0.0.1:9092"}, WriterProducerConfig{
		BatchSize:    750,
		BatchTimeout: 25 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("create producer: %v", err)
	}
	defer func() {
		if err := producer.Close(); err != nil {
			t.Fatalf("close producer: %v", err)
		}
	}()
	if producer.writer.BatchSize != 750 {
		t.Fatalf("batch size = %d, want 750", producer.writer.BatchSize)
	}
	if producer.writer.BatchTimeout != 25*time.Millisecond {
		t.Fatalf("batch timeout = %v, want 25ms", producer.writer.BatchTimeout)
	}
}
