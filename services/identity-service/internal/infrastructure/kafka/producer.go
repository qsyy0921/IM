package kafka

import (
	"context"
	"errors"
	"time"

	"github.com/qsyy0921/IM/services/identity-service/internal/types"
	kafkago "github.com/segmentio/kafka-go"
)

type WriterProducer struct {
	writer *kafkago.Writer
}

const (
	kafkaProducerMaxAttempts     = 5
	kafkaProducerWriteBackoffMin = 100 * time.Millisecond
	kafkaProducerWriteBackoffMax = time.Second
)

func NewWriterProducer(brokers []string) (*WriterProducer, error) {
	if len(brokers) == 0 {
		return nil, errors.New("kafka brokers are required")
	}
	// kafka-go does not expose Kafka's enable.idempotence producer flag. This
	// first-phase writer enforces acks=all, explicit bounded retry/backoff, and
	// outbox/event_id idempotency. Production hardening must revisit the client
	// choice or lower-level transactional producer support.
	return &WriterProducer{
		writer: &kafkago.Writer{
			Addr:                   kafkago.TCP(brokers...),
			Balancer:               &kafkago.Hash{},
			RequiredAcks:           kafkago.RequireAll,
			AllowAutoTopicCreation: false,
			BatchSize:              100,
			BatchTimeout:           10 * time.Millisecond,
			MaxAttempts:            kafkaProducerMaxAttempts,
			WriteBackoffMin:        kafkaProducerWriteBackoffMin,
			WriteBackoffMax:        kafkaProducerWriteBackoffMax,
			WriteTimeout:           5 * time.Second,
			ReadTimeout:            5 * time.Second,
		},
	}, nil
}

func (producer *WriterProducer) PublishBatch(ctx context.Context, topic string, records []types.KafkaPublishRecord) error {
	if producer == nil || producer.writer == nil {
		return errors.New("kafka writer producer is not configured")
	}
	if len(records) == 0 {
		return nil
	}
	messages := make([]kafkago.Message, 0, len(records))
	for _, record := range records {
		messages = append(messages, kafkago.Message{
			Topic: topic,
			Key:   record.Key,
			Value: record.Value,
		})
	}
	return producer.writer.WriteMessages(ctx, messages...)
}

func (producer *WriterProducer) Close() error {
	if producer == nil || producer.writer == nil {
		return nil
	}
	return producer.writer.Close()
}
