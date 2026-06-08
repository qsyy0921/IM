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
		Config{Enabled: true, MinAvailableConns: 2},
		fakePoolStats{stats: PoolStats{AcquiredConns: 7, MaxConns: 8}},
		nil,
		nil,
	)

	err := controller.CheckSendMessage(context.Background())
	if !errors.Is(err, types.ErrServiceOverloaded) {
		t.Fatalf("expected service overloaded, got %v", err)
	}
}

func TestControllerRejectsWhenServiceAcquireP95IsHigh(t *testing.T) {
	controller := NewController(
		Config{
			Enabled:           true,
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
