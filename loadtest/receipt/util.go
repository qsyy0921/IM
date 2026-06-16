package main

import (
	"math"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

func elapsedMS(begin time.Time) float64 {
	return float64(time.Since(begin).Microseconds()) / 1000
}

func buildCapacitySummary(result *summary) *capacitySummary {
	if result == nil || result.StartedAt.IsZero() || result.FinishedAt.IsZero() {
		return nil
	}
	duration := result.FinishedAt.Sub(result.StartedAt)
	if duration <= 0 {
		return nil
	}
	durationSeconds := duration.Seconds()
	messageCount := receiptMessageCount(result)
	receiptEventCount := len(result.ReceiptKafkaEvents)
	operationCount := messageCount +
		receiptPullItemCount(result) +
		receiptAckCount(result) +
		receiptMarkReadCount(result) +
		receiptStateQueryCount(result) +
		receiptConversationListCallCount(result) +
		receiptStateMutationCount(result)
	return &capacitySummary{
		DurationMS:                float64(duration.Microseconds()) / 1000,
		MessageCount:              messageCount,
		PullItemCount:             receiptPullItemCount(result),
		AckCount:                  receiptAckCount(result),
		MarkReadCount:             receiptMarkReadCount(result),
		ReceiptStateQueryCount:    receiptStateQueryCount(result),
		ConversationListCallCount: receiptConversationListCallCount(result),
		StateMutationCount:        receiptStateMutationCount(result),
		ReceiptKafkaEventCount:    receiptEventCount,
		ReceiptOutboxPublished:    result.ReceiptOutbox.Published,
		ReceiptOutboxPending:      result.ReceiptOutbox.Pending,
		ReceiptOutboxDLQ:          result.ReceiptOutbox.DLQ,
		DeliveryOutboxPublished:   result.DeliveryOutbox.Published,
		DeliveryOutboxPending:     result.DeliveryOutbox.Pending,
		DeliveryOutboxDLQ:         result.DeliveryOutbox.DLQ,
		OperationsPerSecond:       ratePerSecond(int64(operationCount), durationSeconds),
		MessagesPerSecond:         ratePerSecond(int64(messageCount), durationSeconds),
		ReceiptEventsPerSecond:    ratePerSecond(int64(receiptEventCount), durationSeconds),
	}
}

func receiptMessageCount(result *summary) int {
	count := 0
	if result.SendMessage.ConversationSeq > 0 || result.SendMessage.MessageID != "" {
		count++
	}
	if result.SendMessageWhileArchived.ConversationSeq > 0 || result.SendMessageWhileArchived.MessageID != "" {
		count++
	}
	return count
}

func receiptPullItemCount(result *summary) int {
	return result.PullInbox.ItemCount + result.PullInboxWhileArchived.ItemCount
}

func receiptAckCount(result *summary) int {
	count := 0
	if result.AckDelivery.LastReceivedSeq > 0 {
		count++
	}
	if result.AckDeliveryWhileArchived.LastReceivedSeq > 0 {
		count++
	}
	return count
}

func receiptMarkReadCount(result *summary) int {
	if result.MarkRead.LastReadSeq > 0 {
		return 1
	}
	return 0
}

func receiptStateQueryCount(result *summary) int {
	count := 0
	if result.ReceiptBeforeReadBySeq.ConversationSeq > 0 || result.ReceiptBeforeReadBySeq.MessageID != "" {
		count++
	}
	if result.ReceiptAfterReadBySeq.ConversationSeq > 0 || result.ReceiptAfterReadBySeq.MessageID != "" {
		count++
	}
	if result.ReceiptAfterReadByMsgID.ConversationSeq > 0 || result.ReceiptAfterReadByMsgID.MessageID != "" {
		count++
	}
	return count
}

func receiptConversationListCallCount(result *summary) int {
	lists := []conversationListSummary{
		result.ConversationListBefore,
		result.ConversationListUnreadBeforeRead,
		result.ConversationListAfter,
		result.ConversationListUnreadAfterRead,
		result.ConversationListArchivedDefault,
		result.ConversationListArchivedIncluded,
		result.ConversationListAfterArchivedNewDefault,
		result.ConversationListAfterArchivedNewIncluded,
		result.ConversationListAfterUnarchive,
		result.ConversationListAfterPin,
		result.ConversationListAfterUnpin,
		result.ConversationListAfterMute,
		result.ConversationListAfterUnmute,
	}
	count := 0
	for _, list := range lists {
		if list.LatencyMS > 0 || list.ItemCount > 0 || list.NextPageCursor != "" || list.ProjectionWatermark.OffsetValue > 0 {
			count++
		}
	}
	return count
}

func receiptStateMutationCount(result *summary) int {
	count := 0
	if result.ArchiveConversation.LatencyMS > 0 {
		count++
	}
	if result.UnarchiveConversation.LatencyMS > 0 {
		count++
	}
	if result.PinConversation.LatencyMS > 0 {
		count++
	}
	if result.UnpinConversation.LatencyMS > 0 {
		count++
	}
	if result.MuteConversation.LatencyMS > 0 {
		count++
	}
	if result.UnmuteConversation.LatencyMS > 0 {
		count++
	}
	if result.MarkReadTooFar.Passed {
		count++
	}
	return count
}

func ratePerSecond(count int64, durationSeconds float64) float64 {
	if count <= 0 || durationSeconds <= 0 {
		return 0
	}
	return float64(count) / durationSeconds
}

func percentile(values []float64, quantile float64) float64 {
	if len(values) == 0 {
		return 0
	}
	copied := append([]float64(nil), values...)
	sort.Float64s(copied)
	index := int(math.Ceil(float64(len(copied))*quantile)) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(copied) {
		index = len(copied) - 1
	}
	return copied[index]
}

func shortCommit() string {
	value := fullCommit()
	if len(value) > 7 {
		return value[:7]
	}
	return value
}

func fullCommit() string {
	out, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func gitDirty() bool {
	return strings.TrimSpace(gitStatusShort()) != ""
}

func gitStatusShort() string {
	out, err := exec.Command("git", "status", "--short").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func envBool(fallback bool, names ...string) bool {
	for _, name := range names {
		value := strings.TrimSpace(os.Getenv(name))
		if value == "" {
			continue
		}
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return fallback
		}
		return parsed
	}
	return fallback
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}
