package monitoring

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/qsyy0921/IM/services/identity-service/internal/types"
)

type ChallengeNotifier interface {
	SendChallenge(context.Context, types.ChallengeNotification) error
}

type ChallengeDeliveryMetrics struct {
	mu             sync.Mutex
	mode           string
	count          int64
	failureCount   int64
	failureClasses map[string]int64
	totalLatencyMS int64
	maxLatencyMS   int64
	lastSuccessMS  int64
	lastFailureMS  int64
}

func NewChallengeDeliveryMetrics(mode string) *ChallengeDeliveryMetrics {
	mode = strings.TrimSpace(mode)
	if mode == "" {
		mode = "noop"
	}
	return &ChallengeDeliveryMetrics{mode: mode, failureClasses: make(map[string]int64)}
}

func (metrics *ChallengeDeliveryMetrics) Snapshot() ChallengeDeliverySnapshot {
	if metrics == nil {
		return ChallengeDeliverySnapshot{}
	}
	metrics.mu.Lock()
	defer metrics.mu.Unlock()
	failureClasses := make(map[string]int64, len(metrics.failureClasses))
	for class, count := range metrics.failureClasses {
		failureClasses[class] = count
	}
	return ChallengeDeliverySnapshot{
		Mode:              metrics.mode,
		TotalRequests:     metrics.count,
		FailureCount:      metrics.failureCount,
		FailureClasses:    failureClasses,
		SuccessCount:      metrics.count - metrics.failureCount,
		LatencyAvgMS:      averageLatency(metrics.totalLatencyMS, metrics.count),
		LatencyMaxMS:      metrics.maxLatencyMS,
		LastSuccessUnixMS: metrics.lastSuccessMS,
		LastFailureUnixMS: metrics.lastFailureMS,
	}
}

func (metrics *ChallengeDeliveryMetrics) record(latencyMS int64, err error, now time.Time) {
	if metrics == nil {
		return
	}
	metrics.mu.Lock()
	defer metrics.mu.Unlock()
	metrics.count++
	metrics.totalLatencyMS += latencyMS
	if latencyMS > metrics.maxLatencyMS {
		metrics.maxLatencyMS = latencyMS
	}
	if err != nil {
		metrics.failureCount++
		if metrics.failureClasses == nil {
			metrics.failureClasses = make(map[string]int64)
		}
		metrics.failureClasses[classifyChallengeDeliveryFailure(err)]++
		metrics.lastFailureMS = now.UTC().UnixMilli()
		return
	}
	metrics.lastSuccessMS = now.UTC().UnixMilli()
}

type ChallengeDeliverySnapshot struct {
	Mode              string           `json:"mode"`
	TotalRequests     int64            `json:"total_requests"`
	SuccessCount      int64            `json:"success_count"`
	FailureCount      int64            `json:"failure_count"`
	FailureClasses    map[string]int64 `json:"failure_classes,omitempty"`
	LatencyAvgMS      int64            `json:"latency_avg_ms"`
	LatencyMaxMS      int64            `json:"latency_max_ms"`
	LastSuccessUnixMS int64            `json:"last_success_unix_ms,omitempty"`
	LastFailureUnixMS int64            `json:"last_failure_unix_ms,omitempty"`
}

type InstrumentedChallengeNotifier struct {
	notifier ChallengeNotifier
	metrics  *ChallengeDeliveryMetrics
	now      func() time.Time
}

func NewInstrumentedChallengeNotifier(notifier ChallengeNotifier, metrics *ChallengeDeliveryMetrics) *InstrumentedChallengeNotifier {
	return &InstrumentedChallengeNotifier{
		notifier: notifier,
		metrics:  metrics,
		now:      time.Now,
	}
}

func (notifier *InstrumentedChallengeNotifier) SendChallenge(ctx context.Context, notification types.ChallengeNotification) error {
	if notifier == nil {
		return types.NewChallengeDeliveryFailed("identity challenge notifier is not configured")
	}
	now := notifier.now
	if now == nil {
		now = time.Now
	}
	if notifier.notifier == nil {
		err := types.NewChallengeDeliveryFailed("identity challenge notifier is not configured")
		notifier.metrics.record(0, err, now())
		return err
	}
	started := now()
	err := notifier.notifier.SendChallenge(ctx, notification)
	completed := now()
	notifier.metrics.record(completed.Sub(started).Milliseconds(), err, completed)
	return err
}

func classifyChallengeDeliveryFailure(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "not configured") ||
		strings.Contains(message, "url is required") ||
		strings.Contains(message, "key is required"):
		return "configuration"
	case strings.Contains(message, "non-success status"):
		return "provider_non_success"
	case strings.Contains(message, "timeout") ||
		strings.Contains(message, "deadline exceeded"):
		return "timeout"
	case strings.Contains(message, "connection refused") ||
		strings.Contains(message, "no such host") ||
		strings.Contains(message, "network") ||
		strings.Contains(message, "temporary failure"):
		return "network"
	case strings.Contains(message, "marshal") ||
		strings.Contains(message, "json"):
		return "serialization"
	case errors.Is(err, types.ErrChallengeDeliveryFailed):
		return "delivery_failed"
	default:
		return "unknown"
	}
}
