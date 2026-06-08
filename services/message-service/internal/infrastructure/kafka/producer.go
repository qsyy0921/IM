package kafka

import (
	"context"
	"errors"
	"time"

	kafkago "github.com/segmentio/kafka-go"
)

type Producer interface {
	Publish(ctx context.Context, topic string, key []byte, value []byte) error
}

type WriterProducer struct {
	writer *kafkago.Writer
}

func NewWriterProducer(brokers []string) (*WriterProducer, error) {
	if len(brokers) == 0 {
		return nil, errors.New("kafka brokers are required")
	}
	// kafka-go does not expose Kafka's enable.idempotence producer flag. This
	// first-phase writer enforces acks=all and relies on outbox/event_id
	// idempotency; production hardening must revisit the client choice or lower
	// level transactional producer support.
	return &WriterProducer{
		writer: &kafkago.Writer{
			Addr:                   kafkago.TCP(brokers...),
			Balancer:               &kafkago.Hash{},
			RequiredAcks:           kafkago.RequireAll,
			AllowAutoTopicCreation: false,
			BatchSize:              1,
			BatchTimeout:           10 * time.Millisecond,
			WriteTimeout:           5 * time.Second,
			ReadTimeout:            5 * time.Second,
		},
	}, nil
}

func (p *WriterProducer) Publish(ctx context.Context, topic string, key []byte, value []byte) error {
	if p == nil || p.writer == nil {
		return errors.New("kafka writer producer is not configured")
	}
	return p.writer.WriteMessages(ctx, kafkago.Message{
		Topic: topic,
		Key:   key,
		Value: value,
	})
}

func (p *WriterProducer) Close() error {
	if p == nil || p.writer == nil {
		return nil
	}
	return p.writer.Close()
}
