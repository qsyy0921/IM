package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	receipteventsv1 "github.com/qsyy0921/IM/schemas/kafka/receipt/v1"
	kafkago "github.com/segmentio/kafka-go"
	"google.golang.org/protobuf/proto"
)

func readReceiptEvents(ctx context.Context, cfg config, wantByType map[string]int64) ([]receiptKafkaEvent, error) {
	wantTotal := 0
	for _, count := range wantByType {
		wantTotal += int(count)
	}
	if wantTotal == 0 {
		return nil, nil
	}
	if cfg.receiptEventsTopic == "" || len(cfg.kafkaBrokers) == 0 || cfg.receiptEventsGroup == "" {
		return nil, errors.New("receipt event readback requires kafka brokers, topic, and consumer group")
	}
	reader := kafkago.NewReader(kafkago.ReaderConfig{
		Brokers: cfg.kafkaBrokers,
		Topic:   cfg.receiptEventsTopic,
		GroupID: cfg.receiptEventsGroup,
	})
	defer reader.Close()

	deadline := time.Now().Add(cfg.waitTimeout)
	events := make([]receiptKafkaEvent, 0, wantTotal)
	seen := map[string]struct{}{}
	for len(events) < wantTotal && time.Now().Before(deadline) {
		readCtx, cancel := context.WithTimeout(ctx, cfg.pollInterval)
		message, err := reader.ReadMessage(readCtx)
		cancel()
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				continue
			}
			return events, fmt.Errorf("read receipt event: %w", err)
		}
		var event receipteventsv1.ReceiptEvent
		if err := proto.Unmarshal(message.Value, &event); err != nil {
			return events, fmt.Errorf("decode receipt event: %w", err)
		}
		if event.TenantId != cfg.tenantID || event.AggregateId != cfg.conversationID {
			continue
		}
		if _, ok := wantByType[event.EventType]; !ok {
			continue
		}
		if _, ok := seen[event.EventId]; ok {
			continue
		}
		seen[event.EventId] = struct{}{}
		events = append(events, summarizeReceiptKafkaEvent(message, &event))
	}
	if len(events) < wantTotal {
		return events, fmt.Errorf("receipt event readback timeout: got=%d want=%d", len(events), wantTotal)
	}
	return events, nil
}

func summarizeReceiptKafkaEvent(message kafkago.Message, event *receipteventsv1.ReceiptEvent) receiptKafkaEvent {
	result := receiptKafkaEvent{
		EventID:          event.EventId,
		EventType:        event.EventType,
		Partition:        message.Partition,
		Offset:           message.Offset,
		AggregateVersion: event.AggregateVersion,
		PartitionKey:     event.PartitionKey,
	}
	if payload := event.GetMessageReceived(); payload != nil {
		result.PayloadType = "message_received"
		result.MessageID = payload.MessageId
		result.UserID = payload.UserId
		result.DeviceID = payload.DeviceId
		result.CursorSeq = payload.CursorSeq
		return result
	}
	if payload := event.GetMessageRead(); payload != nil {
		result.PayloadType = "message_read"
		result.MessageID = payload.MessageId
		result.UserID = payload.UserId
		result.DeviceID = payload.DeviceId
		result.CursorSeq = payload.CursorSeq
	}
	return result
}

func readReceiptEventCount(ctx context.Context, cfg config, wantTotal int, conversationIDs []string) (int, error) {
	if wantTotal == 0 {
		return 0, nil
	}
	if cfg.receiptEventsTopic == "" || len(cfg.kafkaBrokers) == 0 || cfg.receiptEventsGroup == "" {
		return 0, errors.New("receipt event readback requires kafka brokers, topic, and consumer group")
	}
	allowedConversations := make(map[string]struct{}, len(conversationIDs))
	for _, conversationID := range conversationIDs {
		allowedConversations[conversationID] = struct{}{}
	}
	reader := kafkago.NewReader(kafkago.ReaderConfig{
		Brokers: cfg.kafkaBrokers,
		Topic:   cfg.receiptEventsTopic,
		GroupID: cfg.receiptEventsGroup,
	})
	defer reader.Close()

	deadline := time.Now().Add(cfg.waitTimeout)
	seen := map[string]struct{}{}
	for len(seen) < wantTotal && time.Now().Before(deadline) {
		readCtx, cancel := context.WithTimeout(ctx, cfg.pollInterval)
		message, err := reader.ReadMessage(readCtx)
		cancel()
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				continue
			}
			return len(seen), fmt.Errorf("read receipt event count: %w", err)
		}
		var event receipteventsv1.ReceiptEvent
		if err := proto.Unmarshal(message.Value, &event); err != nil {
			return len(seen), fmt.Errorf("decode receipt event count: %w", err)
		}
		if event.TenantId != cfg.tenantID {
			continue
		}
		if _, ok := allowedConversations[event.AggregateId]; !ok {
			continue
		}
		switch event.EventType {
		case "receipt.message.received.v1", "receipt.message.read.v1":
		default:
			continue
		}
		if _, ok := seen[event.EventId]; ok {
			continue
		}
		seen[event.EventId] = struct{}{}
	}
	if len(seen) < wantTotal {
		return len(seen), fmt.Errorf("receipt event count timeout: got=%d want=%d", len(seen), wantTotal)
	}
	return len(seen), nil
}
