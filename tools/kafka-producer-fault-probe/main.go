package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	kafkago "github.com/segmentio/kafka-go"
)

type attemptRecord struct {
	ID        string `json:"id"`
	Sequence  int    `json:"sequence"`
	Acked     bool   `json:"acked"`
	Error     string `json:"error,omitempty"`
	StartedAt string `json:"started_at"`
	EndedAt   string `json:"ended_at"`
}

type produceSummary struct {
	Mode       string          `json:"mode"`
	RunName    string          `json:"run_name"`
	Topic      string          `json:"topic"`
	Attempted  int             `json:"attempted"`
	Acked      int             `json:"acked"`
	Failed     int             `json:"failed"`
	StartedAt  string          `json:"started_at"`
	EndedAt    string          `json:"ended_at"`
	Attempts   []attemptRecord `json:"attempts"`
}

type messageValue struct {
	RunName   string `json:"run_name"`
	ID        string `json:"id"`
	Sequence  int    `json:"sequence"`
	CreatedAt string `json:"created_at"`
}

type consumedMessage struct {
	ID        string `json:"id"`
	Sequence  int    `json:"sequence"`
	Partition int    `json:"partition"`
	Offset    int64  `json:"offset"`
}

type consumeSummary struct {
	Mode             string            `json:"mode"`
	RunName          string            `json:"run_name"`
	Topic            string            `json:"topic"`
	ObservedTotal    int               `json:"observed_total"`
	ObservedUnique   int               `json:"observed_unique"`
	DuplicateCount   int               `json:"duplicate_count"`
	StartedAt        string            `json:"started_at"`
	EndedAt          string            `json:"ended_at"`
	Messages         []consumedMessage `json:"messages"`
	OccurrenceByID   map[string]int    `json:"occurrence_by_id"`
	LastReadErrorText string            `json:"last_read_error,omitempty"`
}

func main() {
	mode := flag.String("mode", "", "produce or consume")
	brokersArg := flag.String("brokers", "127.0.0.1:19092,127.0.0.1:19093,127.0.0.1:19094", "comma-separated Kafka brokers")
	topic := flag.String("topic", "", "Kafka topic")
	runName := flag.String("run", "", "run name")
	count := flag.Int("count", 120, "record count for produce mode")
	interval := flag.Duration("interval", 50*time.Millisecond, "produce interval")
	timeout := flag.Duration("timeout", 45*time.Second, "consume timeout")
	idleTimeout := flag.Duration("idle-timeout", 3*time.Second, "consume idle timeout after at least one message")
	outputPath := flag.String("output", "", "JSON output path")
	flag.Parse()

	if strings.TrimSpace(*mode) == "" || strings.TrimSpace(*topic) == "" || strings.TrimSpace(*runName) == "" || strings.TrimSpace(*outputPath) == "" {
		fail("mode, topic, run, and output are required")
	}
	brokers := splitCSV(*brokersArg)
	if len(brokers) == 0 {
		fail("at least one Kafka broker is required")
	}

	var err error
	switch *mode {
	case "produce":
		err = runProduce(brokers, *topic, *runName, *count, *interval, *outputPath)
	case "consume":
		err = runConsume(brokers, *topic, *runName, *timeout, *idleTimeout, *outputPath)
	default:
		err = fmt.Errorf("unsupported mode %q", *mode)
	}
	if err != nil {
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
		Mode:      "produce",
		RunName:   runName,
		Topic:     topic,
		Attempted: count,
		StartedAt: startedAt.Format(time.RFC3339Nano),
		Attempts:  make([]attemptRecord, 0, count),
	}

	for sequence := 1; sequence <= count; sequence++ {
		id := fmt.Sprintf("%s-%06d", runName, sequence)
		value := messageValue{
			RunName:   runName,
			ID:        id,
			Sequence:  sequence,
			CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			return err
		}

		attempt := attemptRecord{
			ID:        id,
			Sequence:  sequence,
			StartedAt: time.Now().UTC().Format(time.RFC3339Nano),
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		err = writer.WriteMessages(ctx, kafkago.Message{
			Topic: topic,
			Key:   []byte(id),
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

func runConsume(brokers []string, topic string, runName string, timeout time.Duration, idleTimeout time.Duration, outputPath string) error {
	reader := kafkago.NewReader(kafkago.ReaderConfig{
		Brokers:        brokers,
		GroupID:        "nexusim-kafka-producer-fault-probe-" + runName,
		Topic:          topic,
		StartOffset:    kafkago.FirstOffset,
		CommitInterval: 0,
		MaxWait:        500 * time.Millisecond,
	})
	defer reader.Close()

	startedAt := time.Now().UTC()
	deadline := time.Now().Add(timeout)
	lastMessageAt := time.Now()
	seenAny := false
	summary := consumeSummary{
		Mode:           "consume",
		RunName:        runName,
		Topic:          topic,
		StartedAt:      startedAt.Format(time.RFC3339Nano),
		Messages:       []consumedMessage{},
		OccurrenceByID: map[string]int{},
	}

	for time.Now().Before(deadline) {
		readCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		message, err := reader.ReadMessage(readCtx)
		cancel()
		if err != nil {
			summary.LastReadErrorText = err.Error()
			if seenAny && time.Since(lastMessageAt) >= idleTimeout {
				break
			}
			continue
		}
		var value messageValue
		if err := json.Unmarshal(message.Value, &value); err != nil {
			continue
		}
		if value.RunName != runName || value.ID == "" {
			continue
		}
		seenAny = true
		lastMessageAt = time.Now()
		summary.ObservedTotal++
		summary.OccurrenceByID[value.ID]++
		summary.Messages = append(summary.Messages, consumedMessage{
			ID:        value.ID,
			Sequence:  value.Sequence,
			Partition: message.Partition,
			Offset:    message.Offset,
		})
	}

	for _, occurrences := range summary.OccurrenceByID {
		if occurrences > 1 {
			summary.DuplicateCount += occurrences - 1
		}
	}
	summary.ObservedUnique = len(summary.OccurrenceByID)
	summary.EndedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return writeJSON(outputPath, summary)
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
	if err := os.MkdirAll(filepathDir(path), 0o755); err != nil {
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

func filepathDir(path string) string {
	index := strings.LastIndexAny(path, `/\`)
	if index < 0 {
		return "."
	}
	return path[:index]
}

func fail(message string) {
	fmt.Fprintln(os.Stderr, message)
	os.Exit(1)
}
