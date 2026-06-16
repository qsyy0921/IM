package main

import (
	"math"
	"net/url"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

func parseDeviceIDs(list string, fallback string) []string {
	if strings.TrimSpace(list) == "" {
		return []string{fallback}
	}
	seen := map[string]struct{}{}
	result := make([]string, 0)
	for _, raw := range strings.Split(list, ",") {
		deviceID := strings.TrimSpace(raw)
		if deviceID == "" {
			continue
		}
		if _, ok := seen[deviceID]; ok {
			continue
		}
		seen[deviceID] = struct{}{}
		result = append(result, deviceID)
	}
	if len(result) == 0 {
		return []string{fallback}
	}
	return result
}

func derivePushMetricsURL(pushURL string) string {
	parsed, err := url.Parse(pushURL)
	if err != nil {
		return ""
	}
	switch parsed.Scheme {
	case "ws":
		parsed.Scheme = "http"
	case "wss":
		parsed.Scheme = "https"
	default:
		return ""
	}
	parsed.Path = "/debug/metrics"
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

func snapshotFrame(frame serverFrame) frameSnapshot {
	return frameSnapshot{
		Op:              frame.Op,
		RequestID:       frame.RequestID,
		SessionID:       frame.SessionID,
		ResumeToken:     frame.ResumeToken,
		EventID:         frame.EventID,
		ConversationID:  frame.ConversationID,
		ConversationSeq: frame.ConversationSeq,
		SourceEventID:   frame.SourceEventID,
		SourceEventType: frame.SourceEventType,
		MessageID:       frame.MessageID,
		PullRequired:    frame.PullRequired,
		LastReceivedSeq: frame.LastReceivedSeq,
		Code:            frame.Code,
		Message:         frame.Message,
		Reason:          frame.Reason,
		Retryable:       frame.Retryable,
	}
}

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
	durationMS := float64(duration.Microseconds()) / 1000
	durationSeconds := duration.Seconds()

	messageCount := observedMessageCount(result)
	notifyCount := observedNotifyFrameCount(result)
	ackCount := observedAckFrameCount(result)
	pullItemCount := result.PullInbox.ItemCount
	if result.SlowClient != nil && result.SlowClient.RecoveryPullInbox.ItemCount > pullItemCount {
		pullItemCount = result.SlowClient.RecoveryPullInbox.ItemCount
	}
	if result.RedisFault != nil && result.RedisFault.RecoveryPullInbox.ItemCount > pullItemCount {
		pullItemCount = result.RedisFault.RecoveryPullInbox.ItemCount
	}
	published := int64(0)
	if result.DeliveryOutboxPublished != nil {
		published = *result.DeliveryOutboxPublished
	}

	return &capacitySummary{
		DurationMS:              durationMS,
		DeviceCount:             observedDeviceCount(result),
		MessageCount:            messageCount,
		NotifyFrameCount:        notifyCount,
		AckFrameCount:           ackCount,
		PullInboxItemCount:      pullItemCount,
		DeliveryOutboxPublished: published,
		MessagesPerSecond:       ratePerSecond(messageCount, durationSeconds),
		NotifyFramesPerSecond:   ratePerSecond(notifyCount, durationSeconds),
		AckFramesPerSecond:      ratePerSecond(ackCount, durationSeconds),
		PullItemsPerSecond:      ratePerSecond(pullItemCount, durationSeconds),
	}
}

func observedDeviceCount(result *summary) int {
	if len(result.DeviceNotifications) > 0 {
		return len(result.DeviceNotifications)
	}
	if len(result.ReceiverDeviceIDs) > 0 {
		return len(result.ReceiverDeviceIDs)
	}
	if result.ReceiverDeviceID != "" {
		return 1
	}
	return 0
}

func observedMessageCount(result *summary) int {
	if result.SlowClient != nil && result.SlowClient.MessageCount > 0 {
		return result.SlowClient.MessageCount
	}
	if result.RedisResumeNegative != nil && result.RedisResumeNegative.GapMessageCount > 0 {
		return result.RedisResumeNegative.GapMessageCount
	}
	if result.SendMessage.MessageID != "" || result.SendMessage.ConversationSeq > 0 {
		return 1
	}
	return 0
}

func observedNotifyFrameCount(result *summary) int {
	count := 0
	for _, device := range result.DeviceNotifications {
		if device.DeliveryNotify.Op == opDeliveryNotify {
			count++
		}
	}
	if count == 0 && result.DeliveryNotify.Op == opDeliveryNotify {
		count++
	}
	if result.ChangeDeliveryNotify.Op == opDeliveryNotify {
		count++
	}
	if result.SlowClient != nil {
		count += result.SlowClient.NotifyFramesRead + result.SlowClient.ReplayFramesRead
	}
	if result.RedisFault != nil && result.RedisFault.NotifyReceived {
		count++
	}
	return count
}

func observedAckFrameCount(result *summary) int {
	count := 0
	for _, device := range result.DeviceNotifications {
		if device.DeliveryAckOK.Op == opDeliveryAckOK {
			count++
		}
	}
	if count == 0 && result.DeliveryAckOK.Op == opDeliveryAckOK {
		count++
	}
	if result.SlowClient != nil && result.SlowClient.AckOK.Op == opDeliveryAckOK && result.DeliveryAckOK.Op != opDeliveryAckOK {
		count++
	}
	if result.RedisFault != nil && result.RedisFault.AckOK.Op == opDeliveryAckOK && result.DeliveryAckOK.Op != opDeliveryAckOK {
		count++
	}
	return count
}

func ratePerSecond(count int, durationSeconds float64) float64 {
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
