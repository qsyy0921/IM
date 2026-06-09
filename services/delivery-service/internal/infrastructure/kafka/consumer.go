package kafka

import (
	"context"
	"errors"
	"time"

	"github.com/qsyy0921/IM/services/delivery-service/internal/types"
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
		}),
	}, nil
}

func (consumer *ReaderConsumer) Fetch(ctx context.Context) (types.TimelineMessage, error) {
	if consumer == nil || consumer.reader == nil {
		return types.TimelineMessage{}, errors.New("kafka reader consumer is not configured")
	}
	message, err := consumer.reader.FetchMessage(ctx)
	if err != nil {
		return types.TimelineMessage{}, err
	}
	return types.TimelineMessage{
		Topic:     message.Topic,
		Partition: message.Partition,
		Offset:    message.Offset,
		Value:     message.Value,
	}, nil
}

func (consumer *ReaderConsumer) Commit(ctx context.Context, message types.TimelineMessage) error {
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
