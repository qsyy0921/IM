package admission

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	metricsinfra "github.com/qsyy0921/IM/services/message-service/internal/infrastructure/metrics"
	"github.com/qsyy0921/IM/services/message-service/internal/types"
)

type Config struct {
	Enabled                       bool
	MinAvailableConns             int32
	ReleaseAvailableConns         int32
	MaxPoolAcquireP95             time.Duration
	MaxInFlight                   int64
	MaxOutboxPending              int64
	ReleaseOutboxPending          int64
	MaxRelayProcessReadyActiveP95 time.Duration
	MinOutboxFetchedPerCall       float64
	MinKafkaPublishRecordsPerCall float64
	MinMetricSamples              int64
	SampleInterval                time.Duration
	RelayMetricsURL               string
	HTTPTimeout                   time.Duration
	RetryBaseDelay                time.Duration
	RetryMaxDelay                 time.Duration
}

type PoolStats struct {
	AcquiredConns int32
	MaxConns      int32
}

type PoolStatsProvider interface {
	PoolStats() PoolStats
}

type MetricsProvider interface {
	Snapshot() metricsinfra.Snapshot
}

type OutboxBacklogProvider interface {
	PendingOutboxCount(ctx context.Context) (int64, error)
}

type Controller struct {
	config         Config
	poolStats      PoolStatsProvider
	serviceMetrics MetricsProvider
	outboxBacklog  OutboxBacklogProvider
	httpClient     *http.Client

	outboxPending atomic.Int64
	relaySnapshot atomic.Value
	overloaded    atomic.Bool
	inFlight      atomic.Int64
}

func NewController(
	config Config,
	poolStats PoolStatsProvider,
	serviceMetrics MetricsProvider,
	outboxBacklog OutboxBacklogProvider,
) *Controller {
	if config.SampleInterval <= 0 {
		config.SampleInterval = time.Second
	}
	if config.HTTPTimeout <= 0 {
		config.HTTPTimeout = time.Second
	}
	if config.RetryBaseDelay <= 0 {
		config.RetryBaseDelay = 500 * time.Millisecond
	}
	if config.RetryMaxDelay <= 0 {
		config.RetryMaxDelay = 2 * time.Second
	}
	if config.MinMetricSamples <= 0 {
		config.MinMetricSamples = 20
	}
	if config.ReleaseAvailableConns <= config.MinAvailableConns {
		config.ReleaseAvailableConns = config.MinAvailableConns + 4
	}
	if config.MaxOutboxPending > 0 && config.ReleaseOutboxPending <= 0 {
		config.ReleaseOutboxPending = config.MaxOutboxPending / 2
	}
	controller := &Controller{
		config:         config,
		poolStats:      poolStats,
		serviceMetrics: serviceMetrics,
		outboxBacklog:  outboxBacklog,
		httpClient:     &http.Client{Timeout: config.HTTPTimeout},
	}
	controller.outboxPending.Store(-1)
	return controller
}

func (c *Controller) Start(ctx context.Context) {
	if c == nil || !c.config.Enabled {
		return
	}
	go c.run(ctx)
}

func (c *Controller) CheckSendMessage(ctx context.Context) error {
	permit, err := c.AdmitSendMessage(ctx)
	if permit != nil {
		permit.Release()
	}
	return err
}

func (c *Controller) AdmitSendMessage(ctx context.Context) (types.AdmissionPermit, error) {
	if c == nil || !c.config.Enabled {
		return nil, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if c.overloaded.Load() {
		blockers := c.recoveryBlockers()
		if len(blockers) > 0 {
			return nil, c.newServiceOverloaded("adaptive limit recovering", blockers, true)
		}
		c.overloaded.Store(false)
	}
	reasons := c.overloadReasons()
	if len(reasons) == 0 {
		permit, err := c.tryAcquirePermit()
		if err != nil {
			return nil, err
		}
		return permit, nil
	}
	c.overloaded.Store(true)
	return nil, c.newServiceOverloaded("adaptive limit", reasons, false)
}

func (c *Controller) tryAcquirePermit() (types.AdmissionPermit, error) {
	if c.config.MaxInFlight <= 0 {
		return nil, nil
	}
	for {
		current := c.inFlight.Load()
		if current >= c.config.MaxInFlight {
			reason := fmt.Sprintf("send_message_in_flight=%d max_in_flight=%d", current, c.config.MaxInFlight)
			return nil, c.newServiceOverloaded("adaptive concurrency limit", []string{reason}, false)
		}
		if c.inFlight.CompareAndSwap(current, current+1) {
			return &permit{release: func() { c.inFlight.Add(-1) }}, nil
		}
	}
}

func (c *Controller) newServiceOverloaded(prefix string, reasons []string, recovering bool) error {
	reasonCount := len(reasons)
	if recovering {
		reasonCount++
	}
	if reasonCount < 1 {
		reasonCount = 1
	}
	delay := time.Duration(reasonCount) * c.config.RetryBaseDelay
	if delay > c.config.RetryMaxDelay {
		delay = c.config.RetryMaxDelay
	}
	return types.NewServiceOverloadedWithRetryDelay(prefix+": "+strings.Join(reasons, "; "), delay)
}

func (c *Controller) recoveryBlockers() []string {
	blockers := make([]string, 0, 2)
	if c.poolStats != nil {
		stats := c.poolStats.PoolStats()
		if stats.MaxConns > 0 {
			available := stats.MaxConns - stats.AcquiredConns
			if available <= c.config.ReleaseAvailableConns {
				blockers = append(blockers, fmt.Sprintf(
					"pg pool available=%d release_available=%d",
					available,
					c.config.ReleaseAvailableConns,
				))
			}
		}
	}
	pending := c.outboxPending.Load()
	if c.config.ReleaseOutboxPending > 0 && pending >= c.config.ReleaseOutboxPending {
		blockers = append(blockers, fmt.Sprintf(
			"outbox_pending=%d release_pending=%d",
			pending,
			c.config.ReleaseOutboxPending,
		))
	}
	return blockers
}

func (c *Controller) overloadReasons() []string {
	reasons := make([]string, 0, 4)
	poolPressure := false
	if c.poolStats != nil {
		stats := c.poolStats.PoolStats()
		if stats.MaxConns > 0 {
			available := stats.MaxConns - stats.AcquiredConns
			if available <= c.config.MinAvailableConns {
				poolPressure = true
				reasons = append(reasons, fmt.Sprintf(
					"pg pool available=%d max=%d min_available=%d",
					available,
					stats.MaxConns,
					c.config.MinAvailableConns,
				))
			}
		}
	}

	if c.serviceMetrics != nil && c.config.MaxPoolAcquireP95 > 0 {
		snapshot := c.serviceMetrics.Snapshot()
		poolAcquire := preferRecentLatency(snapshot.RepositoryPoolAcquireRecentLatencyMS, snapshot.RepositoryPoolAcquireLatencyMS)
		if poolPressure && latencyAbove(poolAcquire, c.config.MaxPoolAcquireP95, c.config.MinMetricSamples) {
			reasons = append(reasons, fmt.Sprintf(
				"repository_pool_acquire_p95=%.2fms threshold=%.2fms",
				poolAcquire.P95MS,
				ms(c.config.MaxPoolAcquireP95),
			))
		}
	}

	pending := c.outboxPending.Load()
	if c.config.MaxOutboxPending > 0 && pending >= c.config.MaxOutboxPending {
		reasons = append(reasons, fmt.Sprintf(
			"outbox_pending=%d threshold=%d",
			pending,
			c.config.MaxOutboxPending,
		))
	}

	if value := c.relaySnapshot.Load(); value != nil {
		snapshot := value.(metricsinfra.Snapshot)
		relayActive := preferRecentLatency(snapshot.OutboxProcessReadyActiveRecentLatencyMS, snapshot.OutboxProcessReadyActiveLatencyMS)
		if pending > 0 && latencyAbove(relayActive, c.config.MaxRelayProcessReadyActiveP95, c.config.MinMetricSamples) {
			reasons = append(reasons, fmt.Sprintf(
				"outbox_process_ready_active_p95=%.2fms threshold=%.2fms",
				relayActive.P95MS,
				ms(c.config.MaxRelayProcessReadyActiveP95),
			))
		}
		fetchedPerCall := preferRecentValue(snapshot.OutboxFetchedPerCallRecent, snapshot.OutboxFetchedPerCall)
		if pending > 0 && valueBelow(fetchedPerCall, c.config.MinOutboxFetchedPerCall, c.config.MinMetricSamples) {
			reasons = append(reasons, fmt.Sprintf(
				"outbox_fetched_per_call_avg=%.2f threshold=%.2f",
				fetchedPerCall.Avg,
				c.config.MinOutboxFetchedPerCall,
			))
		}
		kafkaRecordsPerCall := preferRecentValue(snapshot.KafkaPublishRecordsPerCallRecent, snapshot.KafkaPublishRecordsPerCall)
		if pending > 0 && valueBelow(kafkaRecordsPerCall, c.config.MinKafkaPublishRecordsPerCall, c.config.MinMetricSamples) {
			reasons = append(reasons, fmt.Sprintf(
				"kafka_publish_records_per_call_avg=%.2f threshold=%.2f",
				kafkaRecordsPerCall.Avg,
				c.config.MinKafkaPublishRecordsPerCall,
			))
		}
	}

	return reasons
}

func (c *Controller) run(ctx context.Context) {
	c.sample(ctx)
	ticker := time.NewTicker(c.config.SampleInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.sample(ctx)
		}
	}
}

func (c *Controller) sample(ctx context.Context) {
	if c.outboxBacklog != nil && c.config.MaxOutboxPending > 0 {
		sampleCtx, cancel := context.WithTimeout(ctx, c.config.HTTPTimeout)
		pending, err := c.outboxBacklog.PendingOutboxCount(sampleCtx)
		cancel()
		if err == nil {
			c.outboxPending.Store(pending)
		}
	}
	if strings.TrimSpace(c.config.RelayMetricsURL) != "" {
		snapshot, err := c.fetchRelaySnapshot(ctx)
		if err == nil {
			c.relaySnapshot.Store(snapshot)
		}
	}
}

func (c *Controller) fetchRelaySnapshot(ctx context.Context) (metricsinfra.Snapshot, error) {
	requestCtx, cancel := context.WithTimeout(ctx, c.config.HTTPTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, c.config.RelayMetricsURL, nil)
	if err != nil {
		return metricsinfra.Snapshot{}, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return metricsinfra.Snapshot{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return metricsinfra.Snapshot{}, fmt.Errorf("relay metrics status %d", resp.StatusCode)
	}
	var snapshot metricsinfra.Snapshot
	if err := json.NewDecoder(resp.Body).Decode(&snapshot); err != nil {
		return metricsinfra.Snapshot{}, err
	}
	return snapshot, nil
}

func latencyAbove(snapshot metricsinfra.LatencySnapshot, threshold time.Duration, minSamples int64) bool {
	if threshold <= 0 || snapshot.Count < minSamples {
		return false
	}
	return snapshot.P95MS >= ms(threshold)
}

func valueBelow(snapshot metricsinfra.ValueSnapshot, threshold float64, minSamples int64) bool {
	if threshold <= 0 || snapshot.Count < minSamples {
		return false
	}
	return snapshot.Avg <= threshold
}

func preferRecentLatency(recent metricsinfra.LatencySnapshot, cumulative metricsinfra.LatencySnapshot) metricsinfra.LatencySnapshot {
	if recent.Count > 0 {
		return recent
	}
	return cumulative
}

func preferRecentValue(recent metricsinfra.ValueSnapshot, cumulative metricsinfra.ValueSnapshot) metricsinfra.ValueSnapshot {
	if recent.Count > 0 {
		return recent
	}
	return cumulative
}

func ms(duration time.Duration) float64 {
	return float64(duration) / float64(time.Millisecond)
}

type permit struct {
	once    sync.Once
	release func()
}

func (p *permit) Release() {
	if p == nil || p.release == nil {
		return
	}
	p.once.Do(p.release)
}

type PGXPoolStatsProvider struct {
	pool *pgxpool.Pool
}

func NewPGXPoolStatsProvider(pool *pgxpool.Pool) PGXPoolStatsProvider {
	return PGXPoolStatsProvider{pool: pool}
}

func (p PGXPoolStatsProvider) PoolStats() PoolStats {
	if p.pool == nil {
		return PoolStats{}
	}
	stats := p.pool.Stat()
	return PoolStats{
		AcquiredConns: stats.AcquiredConns(),
		MaxConns:      stats.MaxConns(),
	}
}

type PostgresOutboxBacklogProvider struct {
	pool *pgxpool.Pool
}

func NewPostgresOutboxBacklogProvider(pool *pgxpool.Pool) PostgresOutboxBacklogProvider {
	return PostgresOutboxBacklogProvider{pool: pool}
}

func (p PostgresOutboxBacklogProvider) PendingOutboxCount(ctx context.Context) (int64, error) {
	if p.pool == nil {
		return 0, nil
	}
	var count int64
	err := p.pool.QueryRow(ctx, `
SELECT count(*)
FROM message_outbox
WHERE status = 'PENDING'
`).Scan(&count)
	return count, err
}
