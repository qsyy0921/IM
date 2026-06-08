package metrics

import (
	"encoding/json"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Collector struct {
	sendMessage      latencySamples
	repositoryAppend latencySamples
	repositoryCommit latencySamples
	seqAlloc         latencySamples
	kafka            latencySamples
}

func NewCollector() *Collector {
	return &Collector{}
}

func (c *Collector) ObserveSendMessage(duration time.Duration) {
	if c == nil {
		return
	}
	c.sendMessage.observe(duration)
}

func (c *Collector) ObserveRepositoryAppend(duration time.Duration) {
	if c == nil {
		return
	}
	c.repositoryAppend.observe(duration)
}

func (c *Collector) ObserveRepositoryCommit(duration time.Duration) {
	if c == nil {
		return
	}
	c.repositoryCommit.observe(duration)
}

func (c *Collector) ObserveConversationSeqAlloc(duration time.Duration) {
	if c == nil {
		return
	}
	c.seqAlloc.observe(duration)
}

func (c *Collector) ObserveKafkaPublish(duration time.Duration) {
	if c == nil {
		return
	}
	c.kafka.observe(duration)
}

func (c *Collector) Snapshot() Snapshot {
	if c == nil {
		return Snapshot{}
	}
	return Snapshot{
		SendMessageLatencyMS:          c.sendMessage.snapshot(),
		RepositoryAppendLatencyMS:     c.repositoryAppend.snapshot(),
		RepositoryCommitLatencyMS:     c.repositoryCommit.snapshot(),
		ConversationSeqAllocLatencyMS: c.seqAlloc.snapshot(),
		KafkaPublishLatencyMS:         c.kafka.snapshot(),
	}
}

func (c *Collector) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/debug/metrics" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(c.Snapshot())
}

type Snapshot struct {
	SendMessageLatencyMS          LatencySnapshot `json:"send_message_latency_ms"`
	RepositoryAppendLatencyMS     LatencySnapshot `json:"repository_append_latency_ms"`
	RepositoryCommitLatencyMS     LatencySnapshot `json:"repository_commit_latency_ms"`
	ConversationSeqAllocLatencyMS LatencySnapshot `json:"conversation_seq_alloc_latency_ms"`
	KafkaPublishLatencyMS         LatencySnapshot `json:"kafka_publish_latency_ms"`
	PGPool                        *PGPoolSnapshot `json:"pg_pool,omitempty"`
}

type PGPoolSnapshot struct {
	AcquireCount         int64 `json:"acquire_count"`
	AcquireDurationMS    int64 `json:"acquire_duration_ms"`
	AcquiredConns        int32 `json:"acquired_conns"`
	CanceledAcquireCount int64 `json:"canceled_acquire_count"`
	ConstructingConns    int32 `json:"constructing_conns"`
	EmptyAcquireCount    int64 `json:"empty_acquire_count"`
	IdleConns            int32 `json:"idle_conns"`
	MaxConns             int32 `json:"max_conns"`
	TotalConns           int32 `json:"total_conns"`
}

type Handler struct {
	collector *Collector
	pool      *pgxpool.Pool
}

func NewHandler(collector *Collector, pool *pgxpool.Pool) *Handler {
	return &Handler{collector: collector, pool: pool}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/debug/metrics" {
		http.NotFound(w, r)
		return
	}
	snapshot := h.collector.Snapshot()
	if h.pool != nil {
		stats := h.pool.Stat()
		snapshot.PGPool = &PGPoolSnapshot{
			AcquireCount:         stats.AcquireCount(),
			AcquireDurationMS:    int64(stats.AcquireDuration() / time.Millisecond),
			AcquiredConns:        stats.AcquiredConns(),
			CanceledAcquireCount: stats.CanceledAcquireCount(),
			ConstructingConns:    stats.ConstructingConns(),
			EmptyAcquireCount:    stats.EmptyAcquireCount(),
			IdleConns:            stats.IdleConns(),
			MaxConns:             stats.MaxConns(),
			TotalConns:           stats.TotalConns(),
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(snapshot)
}

type LatencySnapshot struct {
	Count int64   `json:"count"`
	AvgMS float64 `json:"avg_ms"`
	P50MS float64 `json:"p50_ms"`
	P95MS float64 `json:"p95_ms"`
	P99MS float64 `json:"p99_ms"`
	MaxMS float64 `json:"max_ms"`
}

type latencySamples struct {
	mu      sync.Mutex
	count   int64
	totalMS float64
	maxMS   float64
	values  []float64
}

func (s *latencySamples) observe(duration time.Duration) {
	if duration < 0 {
		duration = 0
	}
	value := float64(duration) / float64(time.Millisecond)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.count++
	s.totalMS += value
	if value > s.maxMS {
		s.maxMS = value
	}
	s.values = append(s.values, value)
}

func (s *latencySamples) snapshot() LatencySnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.count == 0 {
		return LatencySnapshot{}
	}
	values := append([]float64(nil), s.values...)
	sort.Float64s(values)
	return LatencySnapshot{
		Count: s.count,
		AvgMS: s.totalMS / float64(s.count),
		P50MS: percentile(values, 0.50),
		P95MS: percentile(values, 0.95),
		P99MS: percentile(values, 0.99),
		MaxMS: s.maxMS,
	}
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	index := int(float64(len(sorted))*p+0.999999999) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}
