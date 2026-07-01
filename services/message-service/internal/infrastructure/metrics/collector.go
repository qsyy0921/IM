package metrics

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	monitoringinfra "github.com/qsyy0921/IM/services/message-service/internal/infrastructure/monitoring"
	"github.com/qsyy0921/IM/services/message-service/internal/types"
)

type Collector struct {
	sendMessage               latencySamples
	sendMessageCommandBuild   latencySamples
	sendMessageAdmission      latencySamples
	sendMessageDependencyRead latencySamples
	sendMessageConversation   latencySamples
	sendMessagePolicy         latencySamples
	sendMessageSeqFloor       latencySamples
	sendMessageSequencer      latencySamples
	sendMessageRepositoryCall latencySamples
	repositoryAppend          latencySamples
	repositoryBegin           latencySamples
	repositoryPoolAcquire     latencySamples
	repositoryTxBegin         latencySamples
	repositoryIdempotencyLock latencySamples
	repositoryFindExisting    latencySamples
	repositoryEnsureSeq       latencySamples
	repositoryAllocateSeq     latencySamples
	repositoryInsertMessage   latencySamples
	repositoryInsertTimeline  latencySamples
	repositoryInsertOutbox    latencySamples
	repositoryCommit          latencySamples
	seqAlloc                  latencySamples
	kafka                     latencySamples
	kafkaPublishCall          latencySamples
	kafkaRecordEstimate       latencySamples
	kafkaRecordsPerCall       valueSamples
	outboxProcessReady        latencySamples
	outboxProcessReadyActive  latencySamples
	outboxProcessReadyIdle    latencySamples
	outboxFetchedPerCall      valueSamples
	outboxFetchReady          latencySamples
	outboxMarkPublished       latencySamples
	outboxCommit              latencySamples
}

const recentSampleLimit = 4096

func NewCollector() *Collector {
	return &Collector{}
}

func (c *Collector) ObserveSendMessage(duration time.Duration) {
	if c == nil {
		return
	}
	c.sendMessage.observe(duration)
}

func (c *Collector) ObserveSendMessageCommandBuild(duration time.Duration) {
	if c == nil {
		return
	}
	c.sendMessageCommandBuild.observe(duration)
}

func (c *Collector) ObserveSendMessageAdmission(duration time.Duration) {
	if c == nil {
		return
	}
	c.sendMessageAdmission.observe(duration)
}

func (c *Collector) ObserveSendMessageDependencyRead(duration time.Duration) {
	if c == nil {
		return
	}
	c.sendMessageDependencyRead.observe(duration)
}

func (c *Collector) ObserveSendMessageConversationContext(duration time.Duration) {
	if c == nil {
		return
	}
	c.sendMessageConversation.observe(duration)
}

func (c *Collector) ObserveSendMessagePolicyCheck(duration time.Duration) {
	if c == nil {
		return
	}
	c.sendMessagePolicy.observe(duration)
}

func (c *Collector) ObserveSendMessageSeqFloor(duration time.Duration) {
	if c == nil {
		return
	}
	c.sendMessageSeqFloor.observe(duration)
}

func (c *Collector) ObserveSendMessageSequencerAllocate(duration time.Duration) {
	if c == nil {
		return
	}
	c.sendMessageSequencer.observe(duration)
}

func (c *Collector) ObserveSendMessageRepositoryAppendCall(duration time.Duration) {
	if c == nil {
		return
	}
	c.sendMessageRepositoryCall.observe(duration)
}

func (c *Collector) ObserveRepositoryAppend(duration time.Duration) {
	if c == nil {
		return
	}
	c.repositoryAppend.observe(duration)
}

func (c *Collector) ObserveRepositoryBegin(duration time.Duration) {
	if c == nil {
		return
	}
	c.repositoryBegin.observe(duration)
}

func (c *Collector) ObserveRepositoryPoolAcquire(duration time.Duration) {
	if c == nil {
		return
	}
	c.repositoryPoolAcquire.observe(duration)
}

func (c *Collector) ObserveRepositoryTxBegin(duration time.Duration) {
	if c == nil {
		return
	}
	c.repositoryTxBegin.observe(duration)
}

func (c *Collector) ObserveRepositoryIdempotencyLock(duration time.Duration) {
	if c == nil {
		return
	}
	c.repositoryIdempotencyLock.observe(duration)
}

func (c *Collector) ObserveRepositoryFindExisting(duration time.Duration) {
	if c == nil {
		return
	}
	c.repositoryFindExisting.observe(duration)
}

func (c *Collector) ObserveRepositoryEnsureSeq(duration time.Duration) {
	if c == nil {
		return
	}
	c.repositoryEnsureSeq.observe(duration)
}

func (c *Collector) ObserveRepositoryAllocateSeq(duration time.Duration) {
	if c == nil {
		return
	}
	c.repositoryAllocateSeq.observe(duration)
}

func (c *Collector) ObserveRepositoryInsertMessage(duration time.Duration) {
	if c == nil {
		return
	}
	c.repositoryInsertMessage.observe(duration)
}

func (c *Collector) ObserveRepositoryInsertTimeline(duration time.Duration) {
	if c == nil {
		return
	}
	c.repositoryInsertTimeline.observe(duration)
}

func (c *Collector) ObserveRepositoryInsertOutbox(duration time.Duration) {
	if c == nil {
		return
	}
	c.repositoryInsertOutbox.observe(duration)
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
	c.ObserveKafkaPublishCall(duration, 1)
}

func (c *Collector) ObserveKafkaPublishCall(duration time.Duration, recordCount int) {
	if c == nil {
		return
	}
	if recordCount < 0 {
		recordCount = 0
	}
	c.kafka.observe(duration)
	c.kafkaPublishCall.observe(duration)
	if recordCount > 0 {
		c.kafkaRecordsPerCall.observe(float64(recordCount))
		c.kafkaRecordEstimate.observe(duration / time.Duration(recordCount))
	}
}

func (c *Collector) ObserveOutboxProcessReady(duration time.Duration) {
	c.ObserveOutboxProcessReadyResult(duration, -1)
}

func (c *Collector) ObserveOutboxProcessReadyResult(duration time.Duration, fetched int) {
	if c == nil {
		return
	}
	c.outboxProcessReady.observe(duration)
	if fetched >= 0 {
		c.outboxFetchedPerCall.observe(float64(fetched))
	}
	if fetched > 0 {
		c.outboxProcessReadyActive.observe(duration)
	} else if fetched == 0 {
		c.outboxProcessReadyIdle.observe(duration)
	}
}

func (c *Collector) ObserveOutboxFetchReady(duration time.Duration) {
	if c == nil {
		return
	}
	c.outboxFetchReady.observe(duration)
}

func (c *Collector) ObserveOutboxMarkPublished(duration time.Duration) {
	if c == nil {
		return
	}
	c.outboxMarkPublished.observe(duration)
}

func (c *Collector) ObserveOutboxCommit(duration time.Duration) {
	if c == nil {
		return
	}
	c.outboxCommit.observe(duration)
}

func (c *Collector) Snapshot() Snapshot {
	if c == nil {
		return Snapshot{}
	}
	return Snapshot{
		SendMessageLatencyMS:                     c.sendMessage.snapshot(),
		SendMessageRecentLatencyMS:               c.sendMessage.recentSnapshot(),
		SendMessageCommandBuildLatencyMS:         c.sendMessageCommandBuild.snapshot(),
		SendMessageCommandBuildRecentLatencyMS:   c.sendMessageCommandBuild.recentSnapshot(),
		SendMessageAdmissionLatencyMS:            c.sendMessageAdmission.snapshot(),
		SendMessageAdmissionRecentLatencyMS:      c.sendMessageAdmission.recentSnapshot(),
		SendMessageDependencyReadLatencyMS:       c.sendMessageDependencyRead.snapshot(),
		SendMessageDependencyReadRecentLatencyMS: c.sendMessageDependencyRead.recentSnapshot(),
		SendMessageConversationLatencyMS:         c.sendMessageConversation.snapshot(),
		SendMessageConversationRecentLatencyMS:   c.sendMessageConversation.recentSnapshot(),
		SendMessagePolicyLatencyMS:               c.sendMessagePolicy.snapshot(),
		SendMessagePolicyRecentLatencyMS:         c.sendMessagePolicy.recentSnapshot(),
		SendMessageSeqFloorLatencyMS:             c.sendMessageSeqFloor.snapshot(),
		SendMessageSeqFloorRecentLatencyMS:       c.sendMessageSeqFloor.recentSnapshot(),
		SendMessageSequencerLatencyMS:            c.sendMessageSequencer.snapshot(),
		SendMessageSequencerRecentLatencyMS:      c.sendMessageSequencer.recentSnapshot(),
		SendMessageRepositoryCallLatencyMS:       c.sendMessageRepositoryCall.snapshot(),
		SendMessageRepositoryCallRecentLatencyMS: c.sendMessageRepositoryCall.recentSnapshot(),
		RepositoryAppendLatencyMS:                c.repositoryAppend.snapshot(),
		RepositoryAppendRecentLatencyMS:          c.repositoryAppend.recentSnapshot(),
		RepositoryBeginLatencyMS:                 c.repositoryBegin.snapshot(),
		RepositoryBeginRecentLatencyMS:           c.repositoryBegin.recentSnapshot(),
		RepositoryPoolAcquireLatencyMS:           c.repositoryPoolAcquire.snapshot(),
		RepositoryPoolAcquireRecentLatencyMS:     c.repositoryPoolAcquire.recentSnapshot(),
		RepositoryTxBeginLatencyMS:               c.repositoryTxBegin.snapshot(),
		RepositoryTxBeginRecentLatencyMS:         c.repositoryTxBegin.recentSnapshot(),
		RepositoryIdempotencyLockLatencyMS:       c.repositoryIdempotencyLock.snapshot(),
		RepositoryIdempotencyLockRecentLatencyMS: c.repositoryIdempotencyLock.recentSnapshot(),
		RepositoryFindExistingLatencyMS:          c.repositoryFindExisting.snapshot(),
		RepositoryFindExistingRecentLatencyMS:    c.repositoryFindExisting.recentSnapshot(),
		RepositoryEnsureSeqLatencyMS:             c.repositoryEnsureSeq.snapshot(),
		RepositoryEnsureSeqRecentLatencyMS:       c.repositoryEnsureSeq.recentSnapshot(),
		RepositoryAllocateSeqLatencyMS:           c.repositoryAllocateSeq.snapshot(),
		RepositoryAllocateSeqRecentLatencyMS:     c.repositoryAllocateSeq.recentSnapshot(),
		RepositoryInsertMessageLatencyMS:         c.repositoryInsertMessage.snapshot(),
		RepositoryInsertMessageRecentLatencyMS:   c.repositoryInsertMessage.recentSnapshot(),
		RepositoryInsertTimelineLatencyMS:        c.repositoryInsertTimeline.snapshot(),
		RepositoryInsertTimelineRecentLatencyMS:  c.repositoryInsertTimeline.recentSnapshot(),
		RepositoryInsertOutboxLatencyMS:          c.repositoryInsertOutbox.snapshot(),
		RepositoryInsertOutboxRecentLatencyMS:    c.repositoryInsertOutbox.recentSnapshot(),
		RepositoryCommitLatencyMS:                c.repositoryCommit.snapshot(),
		RepositoryCommitRecentLatencyMS:          c.repositoryCommit.recentSnapshot(),
		ConversationSeqAllocLatencyMS:            c.seqAlloc.snapshot(),
		ConversationSeqAllocRecentLatencyMS:      c.seqAlloc.recentSnapshot(),
		KafkaPublishLatencyMS:                    c.kafka.snapshot(),
		KafkaPublishCallLatencyMS:                c.kafkaPublishCall.snapshot(),
		KafkaPublishRecordLatencyEstimateMS:      c.kafkaRecordEstimate.snapshot(),
		KafkaPublishRecordsPerCall:               c.kafkaRecordsPerCall.snapshot(),
		KafkaPublishRecordsPerCallRecent:         c.kafkaRecordsPerCall.recentSnapshot(),
		OutboxProcessReadyLatencyMS:              c.outboxProcessReady.snapshot(),
		OutboxProcessReadyActiveLatencyMS:        c.outboxProcessReadyActive.snapshot(),
		OutboxProcessReadyActiveRecentLatencyMS:  c.outboxProcessReadyActive.recentSnapshot(),
		OutboxProcessReadyIdleLatencyMS:          c.outboxProcessReadyIdle.snapshot(),
		OutboxFetchedPerCall:                     c.outboxFetchedPerCall.snapshot(),
		OutboxFetchedPerCallRecent:               c.outboxFetchedPerCall.recentSnapshot(),
		OutboxFetchReadyLatencyMS:                c.outboxFetchReady.snapshot(),
		OutboxMarkPublishedLatencyMS:             c.outboxMarkPublished.snapshot(),
		OutboxCommitLatencyMS:                    c.outboxCommit.snapshot(),
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
	SendMessageLatencyMS                     LatencySnapshot                  `json:"send_message_latency_ms"`
	SendMessageRecentLatencyMS               LatencySnapshot                  `json:"send_message_recent_latency_ms"`
	SendMessageCommandBuildLatencyMS         LatencySnapshot                  `json:"send_message_command_build_latency_ms"`
	SendMessageCommandBuildRecentLatencyMS   LatencySnapshot                  `json:"send_message_command_build_recent_latency_ms"`
	SendMessageAdmissionLatencyMS            LatencySnapshot                  `json:"send_message_admission_latency_ms"`
	SendMessageAdmissionRecentLatencyMS      LatencySnapshot                  `json:"send_message_admission_recent_latency_ms"`
	SendMessageDependencyReadLatencyMS       LatencySnapshot                  `json:"send_message_dependency_read_latency_ms"`
	SendMessageDependencyReadRecentLatencyMS LatencySnapshot                  `json:"send_message_dependency_read_recent_latency_ms"`
	SendMessageConversationLatencyMS         LatencySnapshot                  `json:"send_message_conversation_context_latency_ms"`
	SendMessageConversationRecentLatencyMS   LatencySnapshot                  `json:"send_message_conversation_context_recent_latency_ms"`
	SendMessagePolicyLatencyMS               LatencySnapshot                  `json:"send_message_policy_check_latency_ms"`
	SendMessagePolicyRecentLatencyMS         LatencySnapshot                  `json:"send_message_policy_check_recent_latency_ms"`
	SendMessageSeqFloorLatencyMS             LatencySnapshot                  `json:"send_message_seq_floor_latency_ms"`
	SendMessageSeqFloorRecentLatencyMS       LatencySnapshot                  `json:"send_message_seq_floor_recent_latency_ms"`
	SendMessageSequencerLatencyMS            LatencySnapshot                  `json:"send_message_sequencer_allocate_latency_ms"`
	SendMessageSequencerRecentLatencyMS      LatencySnapshot                  `json:"send_message_sequencer_allocate_recent_latency_ms"`
	SendMessageRepositoryCallLatencyMS       LatencySnapshot                  `json:"send_message_repository_append_call_latency_ms"`
	SendMessageRepositoryCallRecentLatencyMS LatencySnapshot                  `json:"send_message_repository_append_call_recent_latency_ms"`
	RepositoryAppendLatencyMS                LatencySnapshot                  `json:"repository_append_latency_ms"`
	RepositoryAppendRecentLatencyMS          LatencySnapshot                  `json:"repository_append_recent_latency_ms"`
	RepositoryBeginLatencyMS                 LatencySnapshot                  `json:"repository_begin_latency_ms"`
	RepositoryBeginRecentLatencyMS           LatencySnapshot                  `json:"repository_begin_recent_latency_ms"`
	RepositoryPoolAcquireLatencyMS           LatencySnapshot                  `json:"repository_pool_acquire_latency_ms"`
	RepositoryPoolAcquireRecentLatencyMS     LatencySnapshot                  `json:"repository_pool_acquire_recent_latency_ms"`
	RepositoryTxBeginLatencyMS               LatencySnapshot                  `json:"repository_tx_begin_latency_ms"`
	RepositoryTxBeginRecentLatencyMS         LatencySnapshot                  `json:"repository_tx_begin_recent_latency_ms"`
	RepositoryIdempotencyLockLatencyMS       LatencySnapshot                  `json:"repository_idempotency_lock_latency_ms"`
	RepositoryIdempotencyLockRecentLatencyMS LatencySnapshot                  `json:"repository_idempotency_lock_recent_latency_ms"`
	RepositoryFindExistingLatencyMS          LatencySnapshot                  `json:"repository_find_existing_latency_ms"`
	RepositoryFindExistingRecentLatencyMS    LatencySnapshot                  `json:"repository_find_existing_recent_latency_ms"`
	RepositoryEnsureSeqLatencyMS             LatencySnapshot                  `json:"repository_ensure_seq_latency_ms"`
	RepositoryEnsureSeqRecentLatencyMS       LatencySnapshot                  `json:"repository_ensure_seq_recent_latency_ms"`
	RepositoryAllocateSeqLatencyMS           LatencySnapshot                  `json:"repository_allocate_seq_latency_ms"`
	RepositoryAllocateSeqRecentLatencyMS     LatencySnapshot                  `json:"repository_allocate_seq_recent_latency_ms"`
	RepositoryInsertMessageLatencyMS         LatencySnapshot                  `json:"repository_insert_message_latency_ms"`
	RepositoryInsertMessageRecentLatencyMS   LatencySnapshot                  `json:"repository_insert_message_recent_latency_ms"`
	RepositoryInsertTimelineLatencyMS        LatencySnapshot                  `json:"repository_insert_timeline_latency_ms"`
	RepositoryInsertTimelineRecentLatencyMS  LatencySnapshot                  `json:"repository_insert_timeline_recent_latency_ms"`
	RepositoryInsertOutboxLatencyMS          LatencySnapshot                  `json:"repository_insert_outbox_latency_ms"`
	RepositoryInsertOutboxRecentLatencyMS    LatencySnapshot                  `json:"repository_insert_outbox_recent_latency_ms"`
	RepositoryCommitLatencyMS                LatencySnapshot                  `json:"repository_commit_latency_ms"`
	RepositoryCommitRecentLatencyMS          LatencySnapshot                  `json:"repository_commit_recent_latency_ms"`
	ConversationSeqAllocLatencyMS            LatencySnapshot                  `json:"conversation_seq_alloc_latency_ms"`
	ConversationSeqAllocRecentLatencyMS      LatencySnapshot                  `json:"conversation_seq_alloc_recent_latency_ms"`
	KafkaPublishLatencyMS                    LatencySnapshot                  `json:"kafka_publish_latency_ms"`
	KafkaPublishCallLatencyMS                LatencySnapshot                  `json:"kafka_publish_call_latency_ms"`
	KafkaPublishRecordLatencyEstimateMS      LatencySnapshot                  `json:"kafka_publish_record_latency_estimate_ms"`
	KafkaPublishRecordsPerCall               ValueSnapshot                    `json:"kafka_publish_records_per_call"`
	KafkaPublishRecordsPerCallRecent         ValueSnapshot                    `json:"kafka_publish_records_per_call_recent"`
	OutboxProcessReadyLatencyMS              LatencySnapshot                  `json:"outbox_process_ready_latency_ms"`
	OutboxProcessReadyActiveLatencyMS        LatencySnapshot                  `json:"outbox_process_ready_active_latency_ms"`
	OutboxProcessReadyActiveRecentLatencyMS  LatencySnapshot                  `json:"outbox_process_ready_active_recent_latency_ms"`
	OutboxProcessReadyIdleLatencyMS          LatencySnapshot                  `json:"outbox_process_ready_idle_latency_ms"`
	OutboxFetchedPerCall                     ValueSnapshot                    `json:"outbox_fetched_per_call"`
	OutboxFetchedPerCallRecent               ValueSnapshot                    `json:"outbox_fetched_per_call_recent"`
	OutboxFetchReadyLatencyMS                LatencySnapshot                  `json:"outbox_fetch_ready_latency_ms"`
	OutboxMarkPublishedLatencyMS             LatencySnapshot                  `json:"outbox_mark_published_latency_ms"`
	OutboxCommitLatencyMS                    LatencySnapshot                  `json:"outbox_commit_latency_ms"`
	PGPool                                   *PGPoolSnapshot                  `json:"pg_pool,omitempty"`
	OutboxRelay                              *types.OutboxRelayWorkerSnapshot `json:"outbox_relay,omitempty"`
	Trace                                    *monitoringinfra.TraceSnapshot   `json:"trace,omitempty"`
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
	collector        *Collector
	pool             *pgxpool.Pool
	outboxRelayStats func() types.OutboxRelayWorkerSnapshot
	traceStats       func() monitoringinfra.TraceSnapshot
}

func NewHandler(collector *Collector, pool *pgxpool.Pool) *Handler {
	return &Handler{collector: collector, pool: pool}
}

func (h *Handler) WithOutboxRelayStats(statsFunc func() types.OutboxRelayWorkerSnapshot) *Handler {
	h.outboxRelayStats = statsFunc
	return h
}

func (h *Handler) WithTraceStats(statsFunc func() monitoringinfra.TraceSnapshot) *Handler {
	h.traceStats = statsFunc
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/healthz":
		writeJSON(w, http.StatusOK, healthResponse{Service: "message-service", Status: "ok"})
		return
	case "/readyz":
		h.handleReady(w, r)
		return
	case "/debug/metrics":
		writeJSON(w, http.StatusOK, h.snapshot())
		return
	case "/metrics":
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(renderPrometheus(h.snapshot())))
		return
	default:
		http.NotFound(w, r)
		return
	}
}

func (h *Handler) snapshot() Snapshot {
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
	if h.outboxRelayStats != nil {
		relaySnapshot := h.outboxRelayStats()
		snapshot.OutboxRelay = &relaySnapshot
	}
	if h.traceStats != nil {
		traceSnapshot := h.traceStats()
		snapshot.Trace = &traceSnapshot
	}
	return snapshot
}

func (h *Handler) handleReady(w http.ResponseWriter, r *http.Request) {
	if h.pool == nil {
		writeJSON(w, http.StatusServiceUnavailable, healthResponse{Service: "message-service", Status: "unready", Error: "postgres pool is not configured"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), time.Second)
	defer cancel()
	if err := h.pool.Ping(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, healthResponse{Service: "message-service", Status: "unready", Error: "postgres ping failed"})
		return
	}
	writeJSON(w, http.StatusOK, healthResponse{Service: "message-service", Status: "ready"})
}

type healthResponse struct {
	Service string `json:"service"`
	Status  string `json:"status"`
	Error   string `json:"error,omitempty"`
}

type LatencySnapshot struct {
	Count int64   `json:"count"`
	AvgMS float64 `json:"avg_ms"`
	P50MS float64 `json:"p50_ms"`
	P95MS float64 `json:"p95_ms"`
	P99MS float64 `json:"p99_ms"`
	MaxMS float64 `json:"max_ms"`
}

type ValueSnapshot struct {
	Count int64   `json:"count"`
	Avg   float64 `json:"avg"`
	P50   float64 `json:"p50"`
	P95   float64 `json:"p95"`
	P99   float64 `json:"p99"`
	Max   float64 `json:"max"`
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

type latencySamples struct {
	mu         sync.Mutex
	count      int64
	totalMS    float64
	maxMS      float64
	values     []float64
	recent     []float64
	recentNext int
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
	s.observeRecentLocked(value)
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

func (s *latencySamples) recentSnapshot() LatencySnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()

	values := append([]float64(nil), s.recent...)
	return latencySnapshotFromValues(values)
}

func (s *latencySamples) observeRecentLocked(value float64) {
	if recentSampleLimit <= 0 {
		return
	}
	if len(s.recent) < recentSampleLimit {
		s.recent = append(s.recent, value)
		return
	}
	s.recent[s.recentNext] = value
	s.recentNext = (s.recentNext + 1) % recentSampleLimit
}

func latencySnapshotFromValues(values []float64) LatencySnapshot {
	if len(values) == 0 {
		return LatencySnapshot{}
	}
	var total float64
	var max float64
	for _, value := range values {
		total += value
		if value > max {
			max = value
		}
	}
	sort.Float64s(values)
	count := int64(len(values))
	return LatencySnapshot{
		Count: count,
		AvgMS: total / float64(count),
		P50MS: percentile(values, 0.50),
		P95MS: percentile(values, 0.95),
		P99MS: percentile(values, 0.99),
		MaxMS: max,
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

type valueSamples struct {
	mu         sync.Mutex
	count      int64
	total      float64
	max        float64
	values     []float64
	recent     []float64
	recentNext int
}

func (s *valueSamples) observe(value float64) {
	if value < 0 {
		value = 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.count++
	s.total += value
	if value > s.max {
		s.max = value
	}
	s.values = append(s.values, value)
	s.observeRecentLocked(value)
}

func (s *valueSamples) snapshot() ValueSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.count == 0 {
		return ValueSnapshot{}
	}
	values := append([]float64(nil), s.values...)
	sort.Float64s(values)
	return ValueSnapshot{
		Count: s.count,
		Avg:   s.total / float64(s.count),
		P50:   percentile(values, 0.50),
		P95:   percentile(values, 0.95),
		P99:   percentile(values, 0.99),
		Max:   s.max,
	}
}

func (s *valueSamples) recentSnapshot() ValueSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()

	values := append([]float64(nil), s.recent...)
	return valueSnapshotFromValues(values)
}

func (s *valueSamples) observeRecentLocked(value float64) {
	if recentSampleLimit <= 0 {
		return
	}
	if len(s.recent) < recentSampleLimit {
		s.recent = append(s.recent, value)
		return
	}
	s.recent[s.recentNext] = value
	s.recentNext = (s.recentNext + 1) % recentSampleLimit
}

func valueSnapshotFromValues(values []float64) ValueSnapshot {
	if len(values) == 0 {
		return ValueSnapshot{}
	}
	var total float64
	var max float64
	for _, value := range values {
		total += value
		if value > max {
			max = value
		}
	}
	sort.Float64s(values)
	count := int64(len(values))
	return ValueSnapshot{
		Count: count,
		Avg:   total / float64(count),
		P50:   percentile(values, 0.50),
		P95:   percentile(values, 0.95),
		P99:   percentile(values, 0.99),
		Max:   max,
	}
}
