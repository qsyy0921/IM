package kafka

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/qsyy0921/IM/services/audit-service/internal/types"
	kafkago "github.com/segmentio/kafka-go"
)

type ReaderConfig struct {
	Brokers []string
	Topic   string
	GroupID string
}

type ReaderConsumer struct {
	reader *kafkago.Reader
}

func NewReaderConsumer(config ReaderConfig) (*ReaderConsumer, error) {
	brokers := normalizeBrokers(config.Brokers)
	if len(brokers) == 0 {
		return nil, errors.New("kafka brokers are required")
	}
	if strings.TrimSpace(config.Topic) == "" {
		return nil, errors.New("kafka topic is required")
	}
	if strings.TrimSpace(config.GroupID) == "" {
		return nil, errors.New("kafka group id is required")
	}
	return &ReaderConsumer{
		reader: kafkago.NewReader(kafkago.ReaderConfig{
			Brokers:        brokers,
			Topic:          strings.TrimSpace(config.Topic),
			GroupID:        strings.TrimSpace(config.GroupID),
			MinBytes:       1,
			MaxBytes:       10e6,
			MaxWait:        500 * time.Millisecond,
			CommitInterval: 0,
			StartOffset:    kafkago.FirstOffset,
		}),
	}, nil
}

func (consumer *ReaderConsumer) Fetch(ctx context.Context) (types.AdminEventMessage, error) {
	if consumer == nil || consumer.reader == nil {
		return types.AdminEventMessage{}, errors.New("kafka reader consumer is not configured")
	}
	message, err := consumer.reader.FetchMessage(ctx)
	if err != nil {
		return types.AdminEventMessage{}, err
	}
	return types.AdminEventMessage{
		Topic:     message.Topic,
		Partition: message.Partition,
		Offset:    message.Offset,
		Key:       message.Key,
		Value:     message.Value,
	}, nil
}

func (consumer *ReaderConsumer) Commit(ctx context.Context, message types.AdminEventMessage) error {
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

func normalizeBrokers(values []string) []string {
	brokers := make([]string, 0, len(values))
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				brokers = append(brokers, part)
			}
		}
	}
	return brokers
}
