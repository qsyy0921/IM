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
