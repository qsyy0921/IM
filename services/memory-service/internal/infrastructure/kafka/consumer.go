package kafka

import (
	"context"
	"strings"

	kafkago "github.com/segmentio/kafka-go"

	"github.com/qsyy0921/IM/services/memory-service/internal/types"
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
		brokers = []string{"localhost:9092"}
	}
	reader := kafkago.NewReader(kafkago.ReaderConfig{
		Brokers:        brokers,
		Topic:          strings.TrimSpace(config.Topic),
		GroupID:        strings.TrimSpace(config.GroupID),
		MinBytes:       1,
		MaxBytes:       10e6,
		CommitInterval: 0,
		StartOffset:    kafkago.FirstOffset,
	})
	return &ReaderConsumer{reader: reader}, nil
}

func (consumer *ReaderConsumer) Fetch(ctx context.Context) (types.TimelineMessage, error) {
	message, err := consumer.reader.FetchMessage(ctx)
	if err != nil {
		return types.TimelineMessage{}, err
	}
	return types.TimelineMessage{
		Topic:     message.Topic,
		Partition: message.Partition,
		Offset:    message.Offset,
		Key:       message.Key,
		Value:     message.Value,
	}, nil
}

func (consumer *ReaderConsumer) Commit(ctx context.Context, message types.TimelineMessage) error {
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
	out := make([]string, 0, len(values))
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				out = append(out, part)
			}
		}
	}
	return out
}
