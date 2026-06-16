package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	deliveryeventsv1 "github.com/qsyy0921/IM/schemas/kafka/delivery/v1"
	kafkago "github.com/segmentio/kafka-go"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const topicDeliveryEvents = "im.delivery.events"

type attemptRecord struct {
	EventID   string `json:"event_id"`
	Sequence  int    `json:"sequence"`
	Acked     bool   `json:"acked"`
	Error     string `json:"error,omitempty"`
	StartedAt string `json:"started_at"`
	EndedAt   string `json:"ended_at"`
}

type produceSummary struct {
	RunName   string          `json:"run_name"`
	Topic     string          `json:"topic"`
	Attempted int             `json:"attempted"`
	Acked     int             `json:"acked"`
	Failed    int             `json:"failed"`
	StartedAt string          `json:"started_at"`
	EndedAt   string          `json:"ended_at"`
	Attempts  []attemptRecord `json:"attempts"`
}

func main() {
	brokersArg := flag.String("brokers", "127.0.0.1:19092,127.0.0.1:19093,127.0.0.1:19094", "comma-separated Kafka brokers")
	topic := flag.String("topic", topicDeliveryEvents, "Kafka topic")
	runName := flag.String("run", "", "run name")
	count := flag.Int("count", 1, "record count")
	interval := flag.Duration("interval", 0, "produce interval")
	outputPath := flag.String("output", "", "JSON output path")
	flag.Parse()

	if strings.TrimSpace(*runName) == "" || strings.TrimSpace(*outputPath) == "" {
		fail("run and output are required")
	}
	if strings.TrimSpace(*topic) != topicDeliveryEvents {
		fail("delivery event probe may only target im.delivery.events")
	}
	brokers := splitCSV(*brokersArg)
	if len(brokers) == 0 {
		fail("at least one Kafka broker is required")
	}
	if err := runProduce(brokers, *topic, *runName, *count, *interval, *outputPath); err != nil {
		fail(err.Error())
	}
}

func runProduce(brokers []string, topic string, runName string, count int, interval time.Duration, outputPath string) error {
	if count < 1 {
		return fmt.Errorf("count must be >= 1")
	}
	writer := &kafkago.Writer{
		Addr:                   kafkago.TCP(brokers...),
		Balancer:               &kafkago.Hash{},
		RequiredAcks:           kafkago.RequireAll,
		AllowAutoTopicCreation: false,
		BatchSize:              100,
		BatchTimeout:           10 * time.Millisecond,
		MaxAttempts:            5,
		WriteBackoffMin:        100 * time.Millisecond,
		WriteBackoffMax:        time.Second,
		WriteTimeout:           5 * time.Second,
		ReadTimeout:            5 * time.Second,
	}
	defer writer.Close()

	startedAt := time.Now().UTC()
	summary := produceSummary{
		RunName:   runName,
		Topic:     topic,
		Attempted: count,
		StartedAt: startedAt.Format(time.RFC3339Nano),
		Attempts:  make([]attemptRecord, 0, count),
	}

	for sequence := 1; sequence <= count; sequence++ {
		eventID := fmt.Sprintf("%s-delivery-event-%06d", runName, sequence)
		event, err := buildDeliveryEvent(runName, eventID, sequence)
		if err != nil {
			return err
		}
		encoded, err := proto.Marshal(event)
		if err != nil {
			return err
		}

		attempt := attemptRecord{
			EventID:   eventID,
			Sequence:  sequence,
			StartedAt: time.Now().UTC().Format(time.RFC3339Nano),
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		err = writer.WriteMessages(ctx, kafkago.Message{
			Topic: topic,
			Key:   []byte(event.GetPartitionKey()),
			Value: encoded,
		})
		cancel()
		attempt.EndedAt = time.Now().UTC().Format(time.RFC3339Nano)
		if err != nil {
			attempt.Error = err.Error()
			summary.Failed++
		} else {
			attempt.Acked = true
			summary.Acked++
		}
		summary.Attempts = append(summary.Attempts, attempt)

		if interval > 0 {
			time.Sleep(interval)
		}
	}

	summary.EndedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return writeJSON(outputPath, summary)
}

func buildDeliveryEvent(runName string, eventID string, sequence int) (*deliveryeventsv1.DeliveryEvent, error) {
	if sequence <= 0 {
		return nil, fmt.Errorf("sequence must be positive")
	}
	tenantID := "tenant-kafka-churn"
	userID := "user-kafka-churn"
	conversationID := "conversation-kafka-churn"
	partitionKey := tenantID + ":" + conversationID
	return &deliveryeventsv1.DeliveryEvent{
		EventId:          eventID,
		EventType:        "delivery.inbox_item.created.v1",
		EventVersion:     "1.0.0",
		TenantId:         tenantID,
		AggregateType:    "delivery",
		AggregateId:      conversationID,
		AggregateVersion: int64(sequence),
		PartitionKey:     partitionKey,
		MappingVersion:   1,
		TraceId:          "trace-" + runName,
		CorrelationId:    "correlation-" + runName,
		CausationId:      "causation-" + eventID,
		Producer:         "kafka-delivery-event-probe",
		OccurredAt:       timestamppb.Now(),
		Payload: &deliveryeventsv1.DeliveryEvent_InboxItemCreated{
			InboxItemCreated: &deliveryeventsv1.DeliveryInboxItemCreatedV1{
				TenantId:        tenantID,
				UserId:          userID,
				ConversationId:  conversationID,
				ConversationSeq: int64(sequence),
				SourceEventId:   "source-" + eventID,
				MessageId:       "message-" + eventID,
				SenderId:        "sender-kafka-churn",
				SourceEventType: "message.persisted.v1",
			},
		},
	}, nil
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func writeJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func fail(message string) {
	fmt.Fprintln(os.Stderr, message)
	os.Exit(1)
}
