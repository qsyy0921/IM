package admission

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	metricsinfra "github.com/qsyy0921/IM/services/message-service/internal/infrastructure/metrics"
	"github.com/qsyy0921/IM/services/message-service/internal/types"
)

type Config struct {
	Enabled                       bool
	MinAvailableConns             int32
	MaxPoolAcquireP95             time.Duration
	MaxOutboxPending              int64
	MaxRelayProcessReadyActiveP95 time.Duration
	MinOutboxFetchedPerCall       float64
	MinKafkaPublishRecordsPerCall float64
	MinMetricSamples              int64
	SampleInterval                time.Duration
	RelayMetricsURL               string
	HTTPTimeout                   time.Duration
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
	if config.MinMetricSamples <= 0 {
		config.MinMetricSamples = 20
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
	if c == nil || !c.config.Enabled {
		return nil
	}
	reasons := c.overloadReasons()
	if len(reasons) == 0 {
		return nil
	}
	return types.NewServiceOverloaded("adaptive limit: " + strings.Join(reasons, "; "))
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
		if poolPressure && latencyAbove(snapshot.RepositoryPoolAcquireLatencyMS, c.config.MaxPoolAcquireP95, c.config.MinMetricSamples) {
			reasons = append(reasons, fmt.Sprintf(
				"repository_pool_acquire_p95=%.2fms threshold=%.2fms",
				snapshot.RepositoryPoolAcquireLatencyMS.P95MS,
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
		if pending > 0 && latencyAbove(snapshot.OutboxProcessReadyActiveLatencyMS, c.config.MaxRelayProcessReadyActiveP95, c.config.MinMetricSamples) {
			reasons = append(reasons, fmt.Sprintf(
				"outbox_process_ready_active_p95=%.2fms threshold=%.2fms",
				snapshot.OutboxProcessReadyActiveLatencyMS.P95MS,
				ms(c.config.MaxRelayProcessReadyActiveP95),
			))
		}
		if pending > 0 && valueBelow(snapshot.OutboxFetchedPerCall, c.config.MinOutboxFetchedPerCall, c.config.MinMetricSamples) {
			reasons = append(reasons, fmt.Sprintf(
				"outbox_fetched_per_call_avg=%.2f threshold=%.2f",
				snapshot.OutboxFetchedPerCall.Avg,
				c.config.MinOutboxFetchedPerCall,
			))
		}
		if pending > 0 && valueBelow(snapshot.KafkaPublishRecordsPerCall, c.config.MinKafkaPublishRecordsPerCall, c.config.MinMetricSamples) {
			reasons = append(reasons, fmt.Sprintf(
				"kafka_publish_records_per_call_avg=%.2f threshold=%.2f",
				snapshot.KafkaPublishRecordsPerCall.Avg,
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

func ms(duration time.Duration) float64 {
	return float64(duration) / float64(time.Millisecond)
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
