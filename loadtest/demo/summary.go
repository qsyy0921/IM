package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func sanitizeID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "run"
	}
	var builder strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			builder.WriteRune(r)
			continue
		}
		builder.WriteByte('-')
	}
	return builder.String()
}

func buildCapacitySummary(s summary) *capacitySummary {
	duration := s.FinishedAt.Sub(s.StartedAt).Seconds()
	if duration <= 0 {
		return nil
	}
	operationCount := userFacingOperationCount(s)
	policyEvents := int64(0)
	if s.PolicyAuditKafka != nil {
		policyEvents = s.PolicyAuditKafka.EventCount
	}
	return &capacitySummary{
		DurationSeconds:          duration,
		GatewayFacade:            s.GatewayFacade,
		GatewayAuthMode:          s.GatewayAuthMode,
		UserFacingOperationCount: operationCount,
		WebSocketFrameCount:      webSocketFrameCount(s),
		MessageCount:             observedMessageCount(s),
		NotifyFrameCount:         observedNotifyFrameCount(s),
		ItemsPulled:              s.PullInbox.ItemCount,
		MaxConversationSeq:       maxConversationSeq(s),
		UnreadBeforeRead:         unreadCount(s.ListBeforeRead),
		UnreadAfterRead:          unreadCount(s.ListAfterRead),
		PostgresUserInboxCount:   s.Postgres.UserInboxCount,
		PostgresSummaryCount:     s.Postgres.UserConversationSummaries,
		PolicyAuditKafkaEvents:   policyEvents,
		OperationsPerSecond:      ratePerSecond(operationCount, duration),
	}
}

func userFacingOperationCount(s summary) int {
	count := 0
	if s.MemberJoin.ChangeID != "" {
		count++
	}
	count += observedMessageCount(s)
	if s.PullInbox.ItemCount > 0 {
		count++
	}
	if s.WebSocketAck.Op != "" {
		count++
	}
	if s.MarkRead.LastReadSeq > 0 {
		count++
	}
	if s.ListBeforeRead.ItemCount > 0 {
		count++
	}
	if s.ListAfterRead.ItemCount > 0 {
		count++
	}
	return count
}

func webSocketFrameCount(s summary) int {
	count := 0
	if s.ServerHello.Op != "" {
		count++
	}
	count += observedNotifyFrameCount(s)
	if s.WebSocketAck.Op != "" {
		count++
	}
	return count
}

func observedMessageCount(s summary) int {
	if s.MessageCount > 0 {
		return s.MessageCount
	}
	if s.SendMessage.MessageID != "" {
		return 1
	}
	return 0
}

func observedNotifyFrameCount(s summary) int {
	if s.NotifyFrameCount > 0 {
		return s.NotifyFrameCount
	}
	if s.Notify.Op != "" {
		return 1
	}
	return 0
}

func maxConversationSeq(s summary) int64 {
	maxSeq := s.MemberJoin.BoundarySeq
	if s.SendMessage.ConversationSeq > maxSeq {
		maxSeq = s.SendMessage.ConversationSeq
	}
	if s.PullInbox.MaxSeq > maxSeq {
		maxSeq = s.PullInbox.MaxSeq
	}
	if s.MarkRead.LastReadSeq > maxSeq {
		maxSeq = s.MarkRead.LastReadSeq
	}
	for _, item := range s.ListBeforeRead.Items {
		if item.LastVisibleSeq > maxSeq {
			maxSeq = item.LastVisibleSeq
		}
	}
	for _, item := range s.ListAfterRead.Items {
		if item.LastVisibleSeq > maxSeq {
			maxSeq = item.LastVisibleSeq
		}
	}
	return maxSeq
}

func unreadCount(summary conversationListSummary) int64 {
	var total int64
	for _, item := range summary.Items {
		total += item.UnreadCount
	}
	return total
}

func ratePerSecond(count int, durationSeconds float64) float64 {
	if count <= 0 || durationSeconds <= 0 {
		return 0
	}
	return float64(count) / durationSeconds
}

func finish(cfg config, result *summary, runErr error) error {
	result.FinishedAt = time.Now().UTC()
	if runErr != nil {
		result.Error = runErr.Error()
	}
	result.Capacity = buildCapacitySummary(*result)
	if err := os.MkdirAll(cfg.resultDir, 0o755); err != nil {
		return fmt.Errorf("create result dir: %w", err)
	}
	payload, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal summary: %w", err)
	}
	if err := os.WriteFile(filepath.Join(cfg.resultDir, "e2e-demo-summary.json"), payload, 0o644); err != nil {
		return fmt.Errorf("write summary: %w", err)
	}
	if runErr != nil {
		return runErr
	}
	return nil
}

func gitOutput(args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "git", args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}
