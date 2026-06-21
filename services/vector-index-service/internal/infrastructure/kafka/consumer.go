package kafka

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/qsyy0921/IM/services/vector-index-service/internal/types"
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
	if strings.TrimSpace(config.Topic) == "" {
		return nil, errors.New("kafka topic is required")
	}
	if strings.TrimSpace(config.GroupID) == "" {
		return nil, errors.New("kafka group id is required")
	}
	return &ReaderConsumer{
		reader: kafkago.NewReader(kafkago.ReaderConfig{
			Brokers:        config.Brokers,
			Topic:          config.Topic,
			GroupID:        config.GroupID,
			StartOffset:    kafkago.FirstOffset,
			MinBytes:       1,
			MaxBytes:       10e6,
			CommitInterval: 0,
			MaxWait:        500 * time.Millisecond,
		}),
	}, nil
}

func (consumer *ReaderConsumer) Fetch(ctx context.Context) (types.ChunkEventMessage, error) {
	if consumer == nil || consumer.reader == nil {
		return types.ChunkEventMessage{}, errors.New("kafka reader consumer is not configured")
	}
	message, err := consumer.reader.FetchMessage(ctx)
	if err != nil {
		return types.ChunkEventMessage{}, err
	}
	return types.ChunkEventMessage{
		Topic:     message.Topic,
		Partition: message.Partition,
		Offset:    message.Offset,
		EventType: eventTypeFromHeaders(message.Headers),
		Value:     message.Value,
	}, nil
}

func (consumer *ReaderConsumer) Commit(ctx context.Context, message types.ChunkEventMessage) error {
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

func eventTypeFromHeaders(headers []kafkago.Header) string {
	for _, header := range headers {
		key := strings.ToLower(strings.TrimSpace(header.Key))
		if key == "event_type" || key == "event-type" || key == "nexusim-event-type" {
			return strings.TrimSpace(string(header.Value))
		}
	}
	return ""
}
