package kafka

import (
	"context"
	"errors"
	"time"

	"github.com/qsyy0921/IM/services/policy-service/internal/types"
	kafkago "github.com/segmentio/kafka-go"
)

type ReaderConsumer struct {
	reader *kafkago.Reader
}

type ReaderConfig struct {
	Brokers []string
	Topic   string
	GroupID string
}

func NewReaderConsumer(config ReaderConfig) (*ReaderConsumer, error) {
	if len(config.Brokers) == 0 {
		return nil, errors.New("kafka brokers are required")
	}
	if config.Topic == "" {
		return nil, errors.New("kafka topic is required")
	}
	if config.GroupID == "" {
		return nil, errors.New("kafka group id is required")
	}
	return &ReaderConsumer{
		reader: kafkago.NewReader(kafkago.ReaderConfig{
			Brokers:        config.Brokers,
			Topic:          config.Topic,
			GroupID:        config.GroupID,
			MinBytes:       1,
			MaxBytes:       10e6,
			CommitInterval: 0,
			MaxWait:        500 * time.Millisecond,
			StartOffset:    kafkago.FirstOffset,
		}),
	}, nil
}

func (consumer *ReaderConsumer) Fetch(ctx context.Context) (types.ContactMessage, error) {
	if consumer == nil || consumer.reader == nil {
		return types.ContactMessage{}, errors.New("kafka reader consumer is not configured")
	}
	message, err := consumer.reader.FetchMessage(ctx)
	if err != nil {
		return types.ContactMessage{}, err
	}
	return types.ContactMessage{
		Topic:     message.Topic,
		Partition: message.Partition,
		Offset:    message.Offset,
		Value:     message.Value,
	}, nil
}

func (consumer *ReaderConsumer) Commit(ctx context.Context, message types.ContactMessage) error {
	if consumer == nil || consumer.reader == nil {
		return errors.New("kafka reader consumer is not configured")
	}
	return consumer.reader.CommitMessages(ctx, kafkago.Message{
		Topic:     message.Topic,
		Partition: message.Partition,
		Offset:    message.Offset,
	})
}

func (consumer *ReaderConsumer) Close() error {
	if consumer == nil || consumer.reader == nil {
		return nil
	}
	return consumer.reader.Close()
}
