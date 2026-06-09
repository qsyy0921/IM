package admission

import (
	"context"
	"errors"
	"testing"
	"time"

	metricsinfra "github.com/qsyy0921/IM/services/message-service/internal/infrastructure/metrics"
	"github.com/qsyy0921/IM/services/message-service/internal/types"
)

func TestControllerAllowsWhenDisabled(t *testing.T) {
	controller := NewController(Config{}, fakePoolStats{stats: PoolStats{AcquiredConns: 1, MaxConns: 1}}, nil, nil)

	if err := controller.CheckSendMessage(context.Background()); err != nil {
		t.Fatalf("expected disabled controller to allow request, got %v", err)
	}
}

func TestControllerRejectsWhenPoolAvailableBelowFloor(t *testing.T) {
	controller := NewController(
		Config{Enabled: true, MinAvailableConns: 2, RetryBaseDelay: 250 * time.Millisecond},
		fakePoolStats{stats: PoolStats{AcquiredConns: 7, MaxConns: 8}},
		nil,
		nil,
	)

	err := controller.CheckSendMessage(context.Background())
	if !errors.Is(err, types.ErrServiceOverloaded) {
		t.Fatalf("expected service overloaded, got %v", err)
	}
	if delay, ok := types.ServiceOverloadedRetryDelay(err); !ok || delay != 250*time.Millisecond {
		t.Fatalf("expected dynamic retry delay, delay=%s ok=%t", delay, ok)
	}
}

func TestControllerMaxInFlightRejectsUntilPermitReleased(t *testing.T) {
	controller := NewController(
		Config{Enabled: true, MaxInFlight: 2, RetryBaseDelay: 250 * time.Millisecond},
		fakePoolStats{stats: PoolStats{AcquiredConns: 1, MaxConns: 8}},
		nil,
		nil,
	)

	first, err := controller.AdmitSendMessage(context.Background())
	if err != nil {
		t.Fatalf("first admission: %v", err)
	}
	second, err := controller.AdmitSendMessage(context.Background())
	if err != nil {
		t.Fatalf("second admission: %v", err)
	}

	err = controller.CheckSendMessage(context.Background())
	if !errors.Is(err, types.ErrServiceOverloaded) {
		t.Fatalf("expected max in-flight overload, got %v", err)
	}
	if delay, ok := types.ServiceOverloadedRetryDelay(err); !ok || delay != 250*time.Millisecond {
		t.Fatalf("expected token retry delay, delay=%s ok=%t", delay, ok)
	}

	first.Release()
	first.Release()
	third, err := controller.AdmitSendMessage(context.Background())
	if err != nil {
		t.Fatalf("expected admission after release, got %v", err)
	}
	second.Release()
	third.Release()
	if got := controller.inFlight.Load(); got != 0 {
		t.Fatalf("expected all permits released, got in-flight=%d", got)
	}
}

func TestControllerCheckSendMessageDoesNotLeakPermit(t *testing.T) {
	controller := NewController(
		Config{Enabled: true, MaxInFlight: 1},
		fakePoolStats{stats: PoolStats{AcquiredConns: 1, MaxConns: 8}},
		nil,
		nil,
	)

	if err := controller.CheckSendMessage(context.Background()); err != nil {
		t.Fatalf("expected check to allow request, got %v", err)
	}
	if got := controller.inFlight.Load(); got != 0 {
		t.Fatalf("expected check to release temporary permit, got in-flight=%d", got)
	}
}

func TestControllerHysteresisWaitsForReleaseAvailableConns(t *testing.T) {
	pool := &fakePoolStats{stats: PoolStats{AcquiredConns: 6, MaxConns: 8}}
	controller := NewController(
		Config{
			Enabled:               true,
			MinAvailableConns:     2,
			ReleaseAvailableConns: 4,
		},
		pool,
		nil,
		nil,
	)

	if err := controller.CheckSendMessage(context.Background()); !errors.Is(err, types.ErrServiceOverloaded) {
		t.Fatalf("expected initial overload, got %v", err)
	}
	pool.stats = PoolStats{AcquiredConns: 5, MaxConns: 8}
	if err := controller.CheckSendMessage(context.Background()); !errors.Is(err, types.ErrServiceOverloaded) {
		t.Fatalf("expected overload while below release threshold, got %v", err)
	}
	pool.stats = PoolStats{AcquiredConns: 3, MaxConns: 8}
	if err := controller.CheckSendMessage(context.Background()); err != nil {
		t.Fatalf("expected release after recovery, got %v", err)
	}
}

func TestControllerAllowsCumulativeAcquireP95WithoutCurrentPoolPressure(t *testing.T) {
	controller := NewController(
		Config{
			Enabled:           true,
			MinAvailableConns: 2,
			MaxPoolAcquireP95: 100 * time.Millisecond,
			MinMetricSamples:  2,
		},
		fakePoolStats{stats: PoolStats{AcquiredConns: 1, MaxConns: 8}},
		fakeMetrics{snapshot: metricsinfra.Snapshot{
			RepositoryPoolAcquireLatencyMS: metricsinfra.LatencySnapshot{
				Count: 2,
				P95MS: 150,
			},
		}},
		nil,
	)

	if err := controller.CheckSendMessage(context.Background()); err != nil {
		t.Fatalf("expected cumulative acquire p95 without current pool pressure to be allowed, got %v", err)
	}
}

func TestControllerRejectsWhenAcquireP95IsHighAndPoolIsPressured(t *testing.T) {
	controller := NewController(
		Config{
			Enabled:           true,
			MinAvailableConns: 2,
			MaxPoolAcquireP95: 100 * time.Millisecond,
			MinMetricSamples:  2,
		},
		fakePoolStats{stats: PoolStats{AcquiredConns: 6, MaxConns: 8}},
		fakeMetrics{snapshot: metricsinfra.Snapshot{
			RepositoryPoolAcquireLatencyMS: metricsinfra.LatencySnapshot{
				Count: 2,
				P95MS: 150,
			},
		}},
		nil,
	)

	err := controller.CheckSendMessage(context.Background())
	if !errors.Is(err, types.ErrServiceOverloaded) {
		t.Fatalf("expected service overloaded, got %v", err)
	}
}

func TestControllerRejectsWhenOutboxOrRelaySignalsTrip(t *testing.T) {
	controller := NewController(
		Config{
			Enabled:                       true,
			MaxOutboxPending:              1000,
			MaxRelayProcessReadyActiveP95: 80 * time.Millisecond,
			MinOutboxFetchedPerCall:       5,
			MinKafkaPublishRecordsPerCall: 10,
			MinMetricSamples:              2,
		},
		fakePoolStats{stats: PoolStats{AcquiredConns: 1, MaxConns: 8}},
		nil,
		nil,
	)
	controller.outboxPending.Store(1200)
	controller.relaySnapshot.Store(metricsinfra.Snapshot{
		OutboxProcessReadyActiveLatencyMS: metricsinfra.LatencySnapshot{Count: 2, P95MS: 120},
		OutboxFetchedPerCall:              metricsinfra.ValueSnapshot{Count: 2, Avg: 3},
		KafkaPublishRecordsPerCall:        metricsinfra.ValueSnapshot{Count: 2, Avg: 8},
	})

	err := controller.CheckSendMessage(context.Background())
	if !errors.Is(err, types.ErrServiceOverloaded) {
		t.Fatalf("expected service overloaded, got %v", err)
	}
}

func TestControllerIgnoresLowFetchedPerCallWhenOutboxHasNoPending(t *testing.T) {
	controller := NewController(
		Config{
			Enabled:                 true,
			MinOutboxFetchedPerCall: 5,
			MinMetricSamples:        2,
		},
		fakePoolStats{stats: PoolStats{AcquiredConns: 1, MaxConns: 8}},
		nil,
		nil,
	)
	controller.outboxPending.Store(0)
	controller.relaySnapshot.Store(metricsinfra.Snapshot{
		OutboxFetchedPerCall: metricsinfra.ValueSnapshot{Count: 2, Avg: 0},
	})

	if err := controller.CheckSendMessage(context.Background()); err != nil {
		t.Fatalf("expected idle relay samples to be allowed, got %v", err)
	}
}

func TestControllerRejectsLowFetchedPerCallWhenOutboxHasPending(t *testing.T) {
	controller := NewController(
		Config{
			Enabled:                 true,
			MinOutboxFetchedPerCall: 5,
			MinMetricSamples:        2,
		},
		fakePoolStats{stats: PoolStats{AcquiredConns: 1, MaxConns: 8}},
		nil,
		nil,
	)
	controller.outboxPending.Store(10)
	controller.relaySnapshot.Store(metricsinfra.Snapshot{
		OutboxFetchedPerCall: metricsinfra.ValueSnapshot{Count: 2, Avg: 3},
	})

	err := controller.CheckSendMessage(context.Background())
	if !errors.Is(err, types.ErrServiceOverloaded) {
		t.Fatalf("expected service overloaded, got %v", err)
	}
}

func TestControllerUsesRecentRelayLatencyBeforeCumulative(t *testing.T) {
	controller := NewController(
		Config{
			Enabled:                       true,
			MaxRelayProcessReadyActiveP95: 100 * time.Millisecond,
			MinMetricSamples:              2,
		},
		fakePoolStats{stats: PoolStats{AcquiredConns: 1, MaxConns: 8}},
		nil,
		nil,
	)
	controller.outboxPending.Store(10)
	controller.relaySnapshot.Store(metricsinfra.Snapshot{
		OutboxProcessReadyActiveLatencyMS:       metricsinfra.LatencySnapshot{Count: 2, P95MS: 150},
		OutboxProcessReadyActiveRecentLatencyMS: metricsinfra.LatencySnapshot{Count: 2, P95MS: 20},
	})

	if err := controller.CheckSendMessage(context.Background()); err != nil {
		t.Fatalf("expected recovered recent relay latency to be allowed, got %v", err)
	}
}

type fakePoolStats struct {
	stats PoolStats
}

func (f fakePoolStats) PoolStats() PoolStats {
	return f.stats
}

type fakeMetrics struct {
	snapshot metricsinfra.Snapshot
}

func (f fakeMetrics) Snapshot() metricsinfra.Snapshot {
	return f.snapshot
}
