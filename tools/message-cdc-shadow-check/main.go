package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	conversationtimelinev1 "github.com/qsyy0921/IM/schemas/kafka"
	kafkago "github.com/segmentio/kafka-go"
	"google.golang.org/protobuf/proto"
)

type expectedEvent struct {
	EventID string `json:"event_id"`
	Seq     int64  `json:"seq"`
}

type observedEvent struct {
	EventID   string `json:"event_id"`
	Seq       int64  `json:"seq"`
	Partition int    `json:"partition"`
	Offset    int64  `json:"offset"`
}

type summary struct {
	TenantID             string          `json:"tenant_id"`
	ConversationID       string          `json:"conversation_id"`
	Topic                string          `json:"topic"`
	ExpectedCount        int             `json:"expected_count"`
	ObservedCount        int             `json:"observed_count"`
	ScannedObservedCount int             `json:"scanned_observed_count,omitempty"`
	IgnoredObservedCount int             `json:"ignored_observed_count,omitempty"`
	MissingEventIDs      []string        `json:"missing_event_ids,omitempty"`
	UnexpectedIDs        []string        `json:"unexpected_event_ids,omitempty"`
	DuplicateIDs         []string        `json:"duplicate_event_ids,omitempty"`
	OutOfOrder           []observedEvent `json:"out_of_order,omitempty"`
	Expected             []expectedEvent `json:"expected,omitempty"`
	Observed             []observedEvent `json:"observed,omitempty"`
}

func main() {
	var dsn string
	var brokersCSV string
	var topic string
	var tenantID string
	var conversationID string
	var createdAfterRaw string
	var timeout time.Duration
	var includeRows bool
	var matchExpectedOnly bool

	flag.StringVar(&dsn, "pg-dsn", getenv("NEXUSIM_PG_DSN", "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable"), "PostgreSQL DSN")
	flag.StringVar(&brokersCSV, "brokers", getenv("NEXUSIM_KAFKA_BROKERS", "localhost:9092"), "comma-separated Kafka brokers")
	flag.StringVar(&topic, "topic", getenv("NEXUSIM_MESSAGE_CDC_TARGET_TOPIC", "conversation.timeline.events.cdc"), "CDC Kafka topic")
	flag.StringVar(&tenantID, "tenant-id", "", "tenant id to verify")
	flag.StringVar(&conversationID, "conversation-id", "", "conversation id to verify")
	flag.StringVar(&createdAfterRaw, "created-after", "", "RFC3339 lower bound for PG timeline rows")
	flag.DurationVar(&timeout, "timeout", 10*time.Second, "Kafka scan timeout")
	flag.BoolVar(&includeRows, "include-rows", false, "include expected/observed rows in JSON output")
	flag.BoolVar(&matchExpectedOnly, "match-expected-only", false, "ignore historical Kafka events whose event_id is not in the expected PG row set")
	flag.Parse()

	if tenantID == "" || conversationID == "" {
		log.Fatal("-tenant-id and -conversation-id are required")
	}
	createdAfter := time.Time{}
	if createdAfterRaw != "" {
		parsed, err := time.Parse(time.RFC3339Nano, createdAfterRaw)
		if err != nil {
			log.Fatalf("parse created-after: %v", err)
		}
		createdAfter = parsed
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout+5*time.Second)
	defer cancel()

	expected, err := loadExpected(ctx, dsn, tenantID, conversationID, createdAfter)
	if err != nil {
		log.Fatal(err)
	}
	observed, err := loadObserved(strings.Split(brokersCSV, ","), topic, tenantID, conversationID, timeout)
	if err != nil {
		log.Fatal(err)
	}
	result := compare(tenantID, conversationID, topic, expected, observed, includeRows, matchExpectedOnly)
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(encoded))
	if result.ExpectedCount != result.ObservedCount ||
		len(result.MissingEventIDs) > 0 ||
		len(result.UnexpectedIDs) > 0 ||
		len(result.DuplicateIDs) > 0 ||
		len(result.OutOfOrder) > 0 {
		os.Exit(1)
	}
}

func loadExpected(ctx context.Context, dsn string, tenantID string, conversationID string, createdAfter time.Time) ([]expectedEvent, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	defer pool.Close()

	rows, err := pool.Query(ctx, `
SELECT event_id, seq
FROM conversation_timeline_events
WHERE tenant_id = $1
  AND conversation_id = $2
  AND ($3::timestamptz IS NULL OR created_at >= $3)
ORDER BY seq
`, tenantID, conversationID, nullableTime(createdAfter))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []expectedEvent
	for rows.Next() {
		var event expectedEvent
		if err := rows.Scan(&event.EventID, &event.Seq); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func loadObserved(brokers []string, topic string, tenantID string, conversationID string, timeout time.Duration) ([]observedEvent, error) {
	brokers = cleanCSV(brokers)
	if len(brokers) == 0 {
		return nil, errors.New("kafka brokers are required")
	}
	dialer := &kafkago.Dialer{Timeout: 5 * time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := dialer.DialContext(ctx, "tcp", brokers[0])
	if err != nil {
		return nil, err
	}
	partitions, err := conn.ReadPartitions(topic)
	_ = conn.Close()
	if err != nil {
		return nil, err
	}
	partitionIDs := uniquePartitions(partitions)
	deadline := time.Now().Add(timeout)
	var observed []observedEvent
	for _, partitionID := range partitionIDs {
		firstOffset, lastOffset, err := partitionOffsets(dialer, brokers[0], topic, partitionID)
		if err != nil {
			return nil, err
		}
		if firstOffset >= lastOffset {
			continue
		}
		partitionEvents, err := readPartition(brokers[0], topic, partitionID, tenantID, conversationID, firstOffset, lastOffset, deadline)
		if err != nil {
			return nil, err
		}
		observed = append(observed, partitionEvents...)
	}
	sort.SliceStable(observed, func(i, j int) bool {
		if observed[i].Partition == observed[j].Partition {
			return observed[i].Offset < observed[j].Offset
		}
		return observed[i].Partition < observed[j].Partition
	})
	return observed, nil
}

func partitionOffsets(dialer *kafkago.Dialer, broker string, topic string, partitionID int) (int64, int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := dialer.DialLeader(ctx, "tcp", broker, topic, partitionID)
	if err != nil {
		return 0, 0, err
	}
	defer conn.Close()
	firstOffset, err := conn.ReadFirstOffset()
	if err != nil {
		return 0, 0, err
	}
	lastOffset, err := conn.ReadLastOffset()
	if err != nil {
		return 0, 0, err
	}
	return firstOffset, lastOffset, nil
}

func readPartition(broker string, topic string, partitionID int, tenantID string, conversationID string, firstOffset int64, lastOffset int64, deadline time.Time) ([]observedEvent, error) {
	reader := kafkago.NewReader(kafkago.ReaderConfig{
		Brokers:     []string{broker},
		Topic:       topic,
		Partition:   partitionID,
		StartOffset: firstOffset,
		MinBytes:    1,
		MaxBytes:    10e6,
		MaxWait:     500 * time.Millisecond,
	})
	defer reader.Close()
	var observed []observedEvent
	for time.Now().Before(deadline) {
		if len(observed) > 0 && observed[len(observed)-1].Offset >= lastOffset-1 {
			return observed, nil
		}
		readCtx, cancel := context.WithDeadline(context.Background(), minTime(deadline, time.Now().Add(500*time.Millisecond)))
		msg, err := reader.ReadMessage(readCtx)
		cancel()
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "i/o timeout") {
				continue
			}
			return nil, err
		}
		var event conversationtimelinev1.ConversationTimelineEvent
		if err := proto.Unmarshal(msg.Value, &event); err != nil {
			if msg.Offset >= lastOffset-1 {
				return observed, nil
			}
			continue
		}
		if event.GetTenantId() != tenantID || event.GetAggregateId() != conversationID {
			if msg.Offset >= lastOffset-1 {
				return observed, nil
			}
			continue
		}
		observed = append(observed, observedEvent{
			EventID:   event.GetEventId(),
			Seq:       event.GetAggregateVersion(),
			Partition: partitionID,
			Offset:    msg.Offset,
		})
		if msg.Offset >= lastOffset-1 {
			return observed, nil
		}
	}
	return observed, nil
}

func minTime(a time.Time, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func compare(tenantID string, conversationID string, topic string, expected []expectedEvent, observed []observedEvent, includeRows bool, matchExpectedOnly bool) summary {
	expectedIDs := make(map[string]expectedEvent, len(expected))
	for _, event := range expected {
		expectedIDs[event.EventID] = event
	}
	scannedObservedCount := len(observed)
	ignoredObservedCount := 0
	if matchExpectedOnly {
		filtered := make([]observedEvent, 0, len(expected))
		for _, event := range observed {
			if _, ok := expectedIDs[event.EventID]; ok {
				filtered = append(filtered, event)
				continue
			}
			ignoredObservedCount++
		}
		observed = filtered
	}
	observedCounts := make(map[string]int, len(observed))
	for _, event := range observed {
		observedCounts[event.EventID]++
	}
	var missing []string
	for _, event := range expected {
		if observedCounts[event.EventID] == 0 {
			missing = append(missing, event.EventID)
		}
	}
	var duplicates []string
	for eventID, count := range observedCounts {
		if count > 1 {
			duplicates = append(duplicates, eventID)
		}
	}
	sort.Strings(duplicates)
	var unexpected []string
	if !matchExpectedOnly {
		seenUnexpected := map[string]struct{}{}
		for _, event := range observed {
			if _, ok := expectedIDs[event.EventID]; ok {
				continue
			}
			if _, seen := seenUnexpected[event.EventID]; seen {
				continue
			}
			seenUnexpected[event.EventID] = struct{}{}
			unexpected = append(unexpected, event.EventID)
		}
		sort.Strings(unexpected)
	}
	var outOfOrder []observedEvent
	lastSeqByPartition := map[int]int64{}
	for _, event := range observed {
		if _, ok := expectedIDs[event.EventID]; !ok {
			continue
		}
		if lastSeq := lastSeqByPartition[event.Partition]; lastSeq > 0 && event.Seq <= lastSeq {
			outOfOrder = append(outOfOrder, event)
		}
		lastSeqByPartition[event.Partition] = event.Seq
	}
	result := summary{
		TenantID:             tenantID,
		ConversationID:       conversationID,
		Topic:                topic,
		ExpectedCount:        len(expected),
		ObservedCount:        len(observed),
		ScannedObservedCount: scannedObservedCount,
		IgnoredObservedCount: ignoredObservedCount,
		MissingEventIDs:      missing,
		UnexpectedIDs:        unexpected,
		DuplicateIDs:         duplicates,
		OutOfOrder:           outOfOrder,
	}
	if includeRows {
		result.Expected = expected
		result.Observed = observed
	}
	return result
}

func uniquePartitions(partitions []kafkago.Partition) []int {
	seen := map[int]struct{}{}
	for _, partition := range partitions {
		seen[partition.ID] = struct{}{}
	}
	ids := make([]int, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	return ids
}

func cleanCSV(values []string) []string {
	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			cleaned = append(cleaned, value)
		}
	}
	return cleaned
}

func nullableTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value.UTC()
}

func getenv(key string, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
