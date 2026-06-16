package main

import (
	"time"

	messagev1 "github.com/qsyy0921/IM/api/proto/nexusim/message/v1"
)

type sample struct {
	latency        time.Duration
	logicalLatency time.Duration
	err            error
	attempt        bool
	retryAttempt   bool
	logicalFinal   bool
	retried        bool
	retryDelay     time.Duration
}

type loadClient struct {
	Target string
	Client messagev1.MessageServiceClient
}

type summary struct {
	Commit                                string                     `json:"commit"`
	CommitFull                            string                     `json:"commit_full"`
	GitDirty                              bool                       `json:"git_dirty"`
	GitStatusShort                        string                     `json:"git_status_short"`
	Target                                string                     `json:"target"`
	Targets                               []string                   `json:"targets,omitempty"`
	MessageTLSEnabled                     bool                       `json:"message_tls_enabled"`
	VerifiedAuthMetadata                  bool                       `json:"verified_auth_metadata"`
	TenantID                              string                     `json:"tenant_id"`
	VUs                                   int                        `json:"vus"`
	Duration                              string                     `json:"duration"`
	StatsWait                             string                     `json:"stats_wait"`
	ConversationCount                     int                        `json:"conversation_count"`
	RetryOverloaded                       bool                       `json:"retry_overloaded"`
	MaxRetries                            int                        `json:"max_retries"`
	RetryJitter                           string                     `json:"retry_jitter"`
	LogicalRequestCount                   int64                      `json:"logical_request_count"`
	LogicalSuccessCount                   int64                      `json:"logical_success_count"`
	LogicalErrorCount                     int64                      `json:"logical_error_count"`
	LogicalSuccessRate                    float64                    `json:"logical_success_rate"`
	LogicalAvgMS                          float64                    `json:"logical_avg_ms"`
	LogicalP50MS                          float64                    `json:"logical_p50_ms"`
	LogicalP95MS                          float64                    `json:"logical_p95_ms"`
	LogicalP99MS                          float64                    `json:"logical_p99_ms"`
	LogicalSuccessAvgMS                   float64                    `json:"logical_success_avg_ms"`
	LogicalSuccessP50MS                   float64                    `json:"logical_success_p50_ms"`
	LogicalSuccessP95MS                   float64                    `json:"logical_success_p95_ms"`
	LogicalSuccessP99MS                   float64                    `json:"logical_success_p99_ms"`
	LogicalErrorAvgMS                     float64                    `json:"logical_error_avg_ms"`
	LogicalErrorP50MS                     float64                    `json:"logical_error_p50_ms"`
	LogicalErrorP95MS                     float64                    `json:"logical_error_p95_ms"`
	LogicalErrorP99MS                     float64                    `json:"logical_error_p99_ms"`
	RequestCount                          int64                      `json:"request_count"`
	RetryAttemptCount                     int64                      `json:"retry_attempt_count"`
	RetriedRequestCount                   int64                      `json:"retried_request_count"`
	RetryDelayCount                       int64                      `json:"retry_delay_count"`
	RetryDelayAvgMS                       float64                    `json:"retry_delay_avg_ms"`
	RetryDelayP95MS                       float64                    `json:"retry_delay_p95_ms"`
	RetryDelayP99MS                       float64                    `json:"retry_delay_p99_ms"`
	SuccessCount                          int64                      `json:"success_count"`
	ErrorCount                            int64                      `json:"error_count"`
	RetryableErrorCount                   int64                      `json:"retryable_error_count"`
	ServiceOverloadedCount                int64                      `json:"service_overloaded_count"`
	RequestRPS                            float64                    `json:"request_rps"`
	AcceptedRPS                           float64                    `json:"accepted_rps"`
	ErrorRPS                              float64                    `json:"error_rps"`
	RetryableErrorRate                    float64                    `json:"retryable_error_rate"`
	OverloadRate                          float64                    `json:"overload_rate"`
	SuccessRate                           float64                    `json:"success_rate"`
	AvgMS                                 float64                    `json:"avg_ms"`
	P50MS                                 float64                    `json:"p50_ms"`
	P95MS                                 float64                    `json:"p95_ms"`
	P99MS                                 float64                    `json:"p99_ms"`
	SuccessAvgMS                          float64                    `json:"success_avg_ms"`
	SuccessP50MS                          float64                    `json:"success_p50_ms"`
	SuccessP95MS                          float64                    `json:"success_p95_ms"`
	SuccessP99MS                          float64                    `json:"success_p99_ms"`
	ErrorAvgMS                            float64                    `json:"error_avg_ms"`
	ErrorP50MS                            float64                    `json:"error_p50_ms"`
	ErrorP95MS                            float64                    `json:"error_p95_ms"`
	ErrorP99MS                            float64                    `json:"error_p99_ms"`
	SendMessageLatencyMS                  *float64                   `json:"send_message_latency_ms"`
	SendMessageP95MS                      *float64                   `json:"send_message_p95_ms"`
	SendMessageP99MS                      *float64                   `json:"send_message_p99_ms"`
	RepositoryAppendLatencyMS             *float64                   `json:"repository_append_latency_ms"`
	RepositoryAppendP95MS                 *float64                   `json:"repository_append_p95_ms"`
	RepositoryAppendP99MS                 *float64                   `json:"repository_append_p99_ms"`
	RepositoryPoolAcquireRecentMS         *float64                   `json:"repository_pool_acquire_recent_latency_ms"`
	RepositoryPoolAcquireRecentP95MS      *float64                   `json:"repository_pool_acquire_recent_p95_ms"`
	RepositoryPoolAcquireRecentP99MS      *float64                   `json:"repository_pool_acquire_recent_p99_ms"`
	RepositoryPoolAcquireRecentCount      *int64                     `json:"repository_pool_acquire_recent_sample_count"`
	RepositoryCommitLatencyMS             *float64                   `json:"repository_commit_latency_ms"`
	RepositoryCommitP95MS                 *float64                   `json:"repository_commit_p95_ms"`
	RepositoryCommitP99MS                 *float64                   `json:"repository_commit_p99_ms"`
	ConversationSeqAllocLatencyMS         *float64                   `json:"conversation_seq_alloc_latency_ms"`
	ConversationSeqAllocP95MS             *float64                   `json:"conversation_seq_alloc_p95_ms"`
	ConversationSeqAllocP99MS             *float64                   `json:"conversation_seq_alloc_p99_ms"`
	OutboxTotalCount                      *int64                     `json:"outbox_total_count"`
	OutboxPublishedCount                  *int64                     `json:"outbox_published_count"`
	OutboxPendingCount                    *int64                     `json:"outbox_pending_count"`
	OutboxDLQCount                        *int64                     `json:"outbox_dlq_count"`
	OutboxOldestPendingAgeSeconds         *float64                   `json:"outbox_oldest_pending_age_seconds"`
	KafkaPublishLatencyMS                 *float64                   `json:"kafka_publish_latency_ms"`
	KafkaPublishP95MS                     *float64                   `json:"kafka_publish_p95_ms"`
	KafkaPublishP99MS                     *float64                   `json:"kafka_publish_p99_ms"`
	KafkaPublishCallLatencyMS             *float64                   `json:"kafka_publish_call_latency_ms"`
	KafkaPublishCallP95MS                 *float64                   `json:"kafka_publish_call_p95_ms"`
	KafkaPublishCallP99MS                 *float64                   `json:"kafka_publish_call_p99_ms"`
	KafkaPublishRecordsPerCall            *float64                   `json:"kafka_publish_records_per_call"`
	KafkaPublishRecordsPerCallP95         *float64                   `json:"kafka_publish_records_per_call_p95"`
	KafkaPublishRecordsPerCallP99         *float64                   `json:"kafka_publish_records_per_call_p99"`
	KafkaPublishRecordsPerCallRecent      *float64                   `json:"kafka_publish_records_per_call_recent"`
	KafkaPublishRecordsPerCallRecentP95   *float64                   `json:"kafka_publish_records_per_call_recent_p95"`
	KafkaPublishRecordsPerCallRecentP99   *float64                   `json:"kafka_publish_records_per_call_recent_p99"`
	KafkaPublishRecordsPerCallRecentCount *int64                     `json:"kafka_publish_records_per_call_recent_sample_count"`
	KafkaPublishRecordEstimateMS          *float64                   `json:"kafka_publish_record_latency_estimate_ms"`
	KafkaPublishRecordEstimateP95MS       *float64                   `json:"kafka_publish_record_latency_estimate_p95_ms"`
	KafkaPublishRecordEstimateP99MS       *float64                   `json:"kafka_publish_record_latency_estimate_p99_ms"`
	OutboxProcessReadyLatencyMS           *float64                   `json:"outbox_process_ready_latency_ms"`
	OutboxProcessReadyP95MS               *float64                   `json:"outbox_process_ready_p95_ms"`
	OutboxProcessReadyP99MS               *float64                   `json:"outbox_process_ready_p99_ms"`
	OutboxProcessReadyActiveMS            *float64                   `json:"outbox_process_ready_active_latency_ms"`
	OutboxProcessReadyActiveP95MS         *float64                   `json:"outbox_process_ready_active_p95_ms"`
	OutboxProcessReadyActiveP99MS         *float64                   `json:"outbox_process_ready_active_p99_ms"`
	OutboxProcessReadyActiveRecentMS      *float64                   `json:"outbox_process_ready_active_recent_latency_ms"`
	OutboxProcessReadyActiveRecentP95MS   *float64                   `json:"outbox_process_ready_active_recent_p95_ms"`
	OutboxProcessReadyActiveRecentP99MS   *float64                   `json:"outbox_process_ready_active_recent_p99_ms"`
	OutboxProcessReadyActiveRecentCount   *int64                     `json:"outbox_process_ready_active_recent_sample_count"`
	OutboxProcessReadyIdleMS              *float64                   `json:"outbox_process_ready_idle_latency_ms"`
	OutboxProcessReadyIdleP95MS           *float64                   `json:"outbox_process_ready_idle_p95_ms"`
	OutboxProcessReadyIdleP99MS           *float64                   `json:"outbox_process_ready_idle_p99_ms"`
	OutboxFetchedPerCall                  *float64                   `json:"outbox_fetched_per_call"`
	OutboxFetchedPerCallP95               *float64                   `json:"outbox_fetched_per_call_p95"`
	OutboxFetchedPerCallP99               *float64                   `json:"outbox_fetched_per_call_p99"`
	OutboxFetchedPerCallRecent            *float64                   `json:"outbox_fetched_per_call_recent"`
	OutboxFetchedPerCallRecentP95         *float64                   `json:"outbox_fetched_per_call_recent_p95"`
	OutboxFetchedPerCallRecentP99         *float64                   `json:"outbox_fetched_per_call_recent_p99"`
	OutboxFetchedPerCallRecentCount       *int64                     `json:"outbox_fetched_per_call_recent_sample_count"`
	OutboxFetchReadyLatencyMS             *float64                   `json:"outbox_fetch_ready_latency_ms"`
	OutboxFetchReadyP95MS                 *float64                   `json:"outbox_fetch_ready_p95_ms"`
	OutboxFetchReadyP99MS                 *float64                   `json:"outbox_fetch_ready_p99_ms"`
	OutboxMarkPublishedLatencyMS          *float64                   `json:"outbox_mark_published_latency_ms"`
	OutboxMarkPublishedP95MS              *float64                   `json:"outbox_mark_published_p95_ms"`
	OutboxMarkPublishedP99MS              *float64                   `json:"outbox_mark_published_p99_ms"`
	OutboxCommitLatencyMS                 *float64                   `json:"outbox_commit_latency_ms"`
	OutboxCommitP95MS                     *float64                   `json:"outbox_commit_p95_ms"`
	OutboxCommitP99MS                     *float64                   `json:"outbox_commit_p99_ms"`
	ServicePGPool                         *pgPoolStats               `json:"service_pg_pool,omitempty"`
	RelayPGPool                           *pgPoolStats               `json:"relay_pg_pool,omitempty"`
	ServiceMetrics                        []processMetrics           `json:"service_metrics,omitempty"`
	RelayMetrics                          []processMetrics           `json:"relay_metrics,omitempty"`
	ServiceLatencyMetrics                 map[string]latencySnapshot `json:"service_latency_metrics,omitempty"`
	RelayLatencyMetrics                   map[string]latencySnapshot `json:"relay_latency_metrics,omitempty"`
	RelayValueMetrics                     map[string]valueSnapshot   `json:"relay_value_metrics,omitempty"`
	Capacity                              *capacitySummary           `json:"capacity_summary,omitempty"`
	ErrorTopN                             []errorCount               `json:"error_topn"`
	MessageErrorCounts                    []messageErrorCount        `json:"message_error_counts,omitempty"`
	StartedAt                             string                     `json:"started_at"`
	FinishedAt                            string                     `json:"finished_at"`
	ResultFile                            string                     `json:"result_file"`
}

type capacitySummary struct {
	DurationMS            float64 `json:"duration_ms"`
	TargetCount           int     `json:"target_count"`
	VUs                   int     `json:"vus"`
	ConversationCount     int     `json:"conversation_count"`
	RequestCount          int64   `json:"request_count"`
	SuccessCount          int64   `json:"success_count"`
	ErrorCount            int64   `json:"error_count"`
	LogicalRequestCount   int64   `json:"logical_request_count"`
	LogicalSuccessCount   int64   `json:"logical_success_count"`
	RequestRPS            float64 `json:"request_rps"`
	AcceptedRPS           float64 `json:"accepted_rps"`
	ErrorRPS              float64 `json:"error_rps"`
	LogicalRequestRPS     float64 `json:"logical_request_rps"`
	LogicalAcceptedRPS    float64 `json:"logical_accepted_rps"`
	SuccessRate           float64 `json:"success_rate"`
	LogicalSuccessRate    float64 `json:"logical_success_rate"`
	P95MS                 float64 `json:"p95_ms"`
	P99MS                 float64 `json:"p99_ms"`
	LogicalP95MS          float64 `json:"logical_p95_ms"`
	LogicalP99MS          float64 `json:"logical_p99_ms"`
	OutboxPublishedCount  *int64  `json:"outbox_published_count,omitempty"`
	OutboxPendingCount    *int64  `json:"outbox_pending_count,omitempty"`
	OutboxDLQCount        *int64  `json:"outbox_dlq_count,omitempty"`
	ServicePGPoolMaxConns *int32  `json:"service_pg_pool_max_conns,omitempty"`
	RelayPGPoolMaxConns   *int32  `json:"relay_pg_pool_max_conns,omitempty"`
}

type errorCount struct {
	Error string `json:"error"`
	Count int64  `json:"count"`
}

type messageErrorCount struct {
	Code      string `json:"code"`
	Retryable bool   `json:"retryable"`
	Count     int64  `json:"count"`
}

type messageErrorKey struct {
	Code      messagev1.MessageErrorCode
	Retryable bool
}

type latencyBreakdown struct {
	AvgMS float64
	P50MS float64
	P95MS float64
	P99MS float64
}

type outboxStats struct {
	Total                   int64
	Published               int64
	Pending                 int64
	DLQ                     int64
	OldestPendingAgeSeconds float64
}

type metricsSnapshot struct {
	SendMessageLatencyMS                    latencySnapshot `json:"send_message_latency_ms"`
	RepositoryAppendLatencyMS               latencySnapshot `json:"repository_append_latency_ms"`
	RepositoryBeginLatencyMS                latencySnapshot `json:"repository_begin_latency_ms"`
	RepositoryPoolAcquireLatencyMS          latencySnapshot `json:"repository_pool_acquire_latency_ms"`
	RepositoryPoolAcquireRecentLatencyMS    latencySnapshot `json:"repository_pool_acquire_recent_latency_ms"`
	RepositoryTxBeginLatencyMS              latencySnapshot `json:"repository_tx_begin_latency_ms"`
	RepositoryIdempotencyLockLatencyMS      latencySnapshot `json:"repository_idempotency_lock_latency_ms"`
	RepositoryFindExistingLatencyMS         latencySnapshot `json:"repository_find_existing_latency_ms"`
	RepositoryEnsureSeqLatencyMS            latencySnapshot `json:"repository_ensure_seq_latency_ms"`
	RepositoryAllocateSeqLatencyMS          latencySnapshot `json:"repository_allocate_seq_latency_ms"`
	RepositoryInsertMessageLatencyMS        latencySnapshot `json:"repository_insert_message_latency_ms"`
	RepositoryInsertTimelineLatencyMS       latencySnapshot `json:"repository_insert_timeline_latency_ms"`
	RepositoryInsertOutboxLatencyMS         latencySnapshot `json:"repository_insert_outbox_latency_ms"`
	RepositoryCommitLatencyMS               latencySnapshot `json:"repository_commit_latency_ms"`
	ConversationSeqAllocLatencyMS           latencySnapshot `json:"conversation_seq_alloc_latency_ms"`
	KafkaPublishLatencyMS                   latencySnapshot `json:"kafka_publish_latency_ms"`
	KafkaPublishCallLatencyMS               latencySnapshot `json:"kafka_publish_call_latency_ms"`
	KafkaPublishRecordLatencyEstimateMS     latencySnapshot `json:"kafka_publish_record_latency_estimate_ms"`
	KafkaPublishRecordsPerCall              valueSnapshot   `json:"kafka_publish_records_per_call"`
	KafkaPublishRecordsPerCallRecent        valueSnapshot   `json:"kafka_publish_records_per_call_recent"`
	OutboxProcessReadyLatencyMS             latencySnapshot `json:"outbox_process_ready_latency_ms"`
	OutboxProcessReadyActiveLatencyMS       latencySnapshot `json:"outbox_process_ready_active_latency_ms"`
	OutboxProcessReadyActiveRecentLatencyMS latencySnapshot `json:"outbox_process_ready_active_recent_latency_ms"`
	OutboxProcessReadyIdleLatencyMS         latencySnapshot `json:"outbox_process_ready_idle_latency_ms"`
	OutboxFetchedPerCall                    valueSnapshot   `json:"outbox_fetched_per_call"`
	OutboxFetchedPerCallRecent              valueSnapshot   `json:"outbox_fetched_per_call_recent"`
	OutboxFetchReadyLatencyMS               latencySnapshot `json:"outbox_fetch_ready_latency_ms"`
	OutboxMarkPublishedLatencyMS            latencySnapshot `json:"outbox_mark_published_latency_ms"`
	OutboxCommitLatencyMS                   latencySnapshot `json:"outbox_commit_latency_ms"`
	PGPool                                  *pgPoolStats    `json:"pg_pool"`
}

type processMetrics struct {
	URL      string          `json:"url"`
	Snapshot metricsSnapshot `json:"snapshot"`
}

type latencySnapshot struct {
	Count int64   `json:"count"`
	AvgMS float64 `json:"avg_ms"`
	P95MS float64 `json:"p95_ms"`
	P99MS float64 `json:"p99_ms"`
}

type valueSnapshot struct {
	Count int64   `json:"count"`
	Avg   float64 `json:"avg"`
	P95   float64 `json:"p95"`
	P99   float64 `json:"p99"`
}

type pgPoolStats struct {
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
