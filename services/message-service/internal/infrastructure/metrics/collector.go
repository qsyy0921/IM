package metrics

import (
	"encoding/json"
	"net/http"
	"sort"
	"sync"
	"time"
)

type Collector struct {
	seqAlloc latencySamples
	kafka    latencySamples
}

func NewCollector() *Collector {
	return &Collector{}
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
	ConversationSeqAllocLatencyMS LatencySnapshot `json:"conversation_seq_alloc_latency_ms"`
	KafkaPublishLatencyMS         LatencySnapshot `json:"kafka_publish_latency_ms"`
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
