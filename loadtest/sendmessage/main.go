package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	messagev1 "github.com/qsyy0921/IM/api/proto/nexusim/message/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
)

type config struct {
	Target             string
	VUs                int
	Duration           time.Duration
	ResultDir          string
	RequestTimeout     time.Duration
	StatsWait          time.Duration
	TenantID           string
	ConversationPrefix string
	ConversationCount  int
	PGDSN              string
	ServiceMetricsURL  string
	RelayMetricsURL    string
}

type sample struct {
	latency time.Duration
	err     error
}

type loadClient struct {
	Target string
	Client messagev1.MessageServiceClient
}

type summary struct {
	Commit                        string                     `json:"commit"`
	CommitFull                    string                     `json:"commit_full"`
	GitDirty                      bool                       `json:"git_dirty"`
	GitStatusShort                string                     `json:"git_status_short"`
	Target                        string                     `json:"target"`
	Targets                       []string                   `json:"targets,omitempty"`
	TenantID                      string                     `json:"tenant_id"`
	VUs                           int                        `json:"vus"`
	Duration                      string                     `json:"duration"`
	StatsWait                     string                     `json:"stats_wait"`
	ConversationCount             int                        `json:"conversation_count"`
	RequestCount                  int64                      `json:"request_count"`
	SuccessCount                  int64                      `json:"success_count"`
	ErrorCount                    int64                      `json:"error_count"`
	RetryableErrorCount           int64                      `json:"retryable_error_count"`
	ServiceOverloadedCount        int64                      `json:"service_overloaded_count"`
	SuccessRate                   float64                    `json:"success_rate"`
	AvgMS                         float64                    `json:"avg_ms"`
	P50MS                         float64                    `json:"p50_ms"`
	P95MS                         float64                    `json:"p95_ms"`
	P99MS                         float64                    `json:"p99_ms"`
	SendMessageLatencyMS          *float64                   `json:"send_message_latency_ms"`
	SendMessageP95MS              *float64                   `json:"send_message_p95_ms"`
	SendMessageP99MS              *float64                   `json:"send_message_p99_ms"`
	RepositoryAppendLatencyMS     *float64                   `json:"repository_append_latency_ms"`
	RepositoryAppendP95MS         *float64                   `json:"repository_append_p95_ms"`
	RepositoryAppendP99MS         *float64                   `json:"repository_append_p99_ms"`
	RepositoryCommitLatencyMS     *float64                   `json:"repository_commit_latency_ms"`
	RepositoryCommitP95MS         *float64                   `json:"repository_commit_p95_ms"`
	RepositoryCommitP99MS         *float64                   `json:"repository_commit_p99_ms"`
	ConversationSeqAllocLatencyMS *float64                   `json:"conversation_seq_alloc_latency_ms"`
	ConversationSeqAllocP95MS     *float64                   `json:"conversation_seq_alloc_p95_ms"`
	ConversationSeqAllocP99MS     *float64                   `json:"conversation_seq_alloc_p99_ms"`
	OutboxTotalCount              *int64                     `json:"outbox_total_count"`
	OutboxPublishedCount          *int64                     `json:"outbox_published_count"`
	OutboxPendingCount            *int64                     `json:"outbox_pending_count"`
	OutboxDLQCount                *int64                     `json:"outbox_dlq_count"`
	OutboxOldestPendingAgeSeconds *float64                   `json:"outbox_oldest_pending_age_seconds"`
	KafkaPublishLatencyMS         *float64                   `json:"kafka_publish_latency_ms"`
	KafkaPublishP95MS             *float64                   `json:"kafka_publish_p95_ms"`
	KafkaPublishP99MS             *float64                   `json:"kafka_publish_p99_ms"`
	ServicePGPool                 *pgPoolStats               `json:"service_pg_pool,omitempty"`
	RelayPGPool                   *pgPoolStats               `json:"relay_pg_pool,omitempty"`
	ServiceMetrics                []processMetrics           `json:"service_metrics,omitempty"`
	RelayMetrics                  []processMetrics           `json:"relay_metrics,omitempty"`
	ServiceLatencyMetrics         map[string]latencySnapshot `json:"service_latency_metrics,omitempty"`
	RelayLatencyMetrics           map[string]latencySnapshot `json:"relay_latency_metrics,omitempty"`
	ErrorTopN                     []errorCount               `json:"error_topn"`
	MessageErrorCounts            []messageErrorCount        `json:"message_error_counts,omitempty"`
	StartedAt                     string                     `json:"started_at"`
	FinishedAt                    string                     `json:"finished_at"`
	ResultFile                    string                     `json:"result_file"`
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

func main() {
	if err := run(os.Args[1:], os.Getenv); err != nil {
		fmt.Fprintf(os.Stderr, "sendmessage loadtest failed: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, getenv func(string) string) error {
	cfg, err := parseConfig(args, getenv)
	if err != nil {
		return err
	}
	grpcTargets, err := normalizeTargets(cfg.Target)
	if err != nil {
		return err
	}

	clients := make([]loadClient, 0, len(grpcTargets))
	conns := make([]*grpc.ClientConn, 0, len(grpcTargets))
	for _, grpcTarget := range grpcTargets {
		conn, err := grpc.NewClient(grpcTarget, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			for _, existing := range conns {
				_ = existing.Close()
			}
			return err
		}
		conns = append(conns, conn)
		clients = append(clients, loadClient{
			Target: grpcTarget,
			Client: messagev1.NewMessageServiceClient(conn),
		})
	}
	defer func() {
		for _, conn := range conns {
			_ = conn.Close()
		}
	}()

	result, err := executeLoad(context.Background(), cfg, clients)
	if err != nil {
		return err
	}
	result.Target = strings.Join(grpcTargets, ",")
	result.Targets = grpcTargets
	if cfg.StatsWait > 0 {
		time.Sleep(cfg.StatsWait)
	}
	if cfg.PGDSN != "" {
		outboxStats, statsErr := readOutboxStats(context.Background(), cfg.PGDSN, cfg.TenantID)
		if statsErr != nil {
			return statsErr
		}
		result.OutboxTotalCount = &outboxStats.Total
		result.OutboxPublishedCount = &outboxStats.Published
		result.OutboxPendingCount = &outboxStats.Pending
		result.OutboxDLQCount = &outboxStats.DLQ
		result.OutboxOldestPendingAgeSeconds = &outboxStats.OldestPendingAgeSeconds
	}
	if cfg.ServiceMetricsURL != "" {
		metricURLs, metricsErr := normalizeMetricsURLs(cfg.ServiceMetricsURL)
		if metricsErr != nil {
			return metricsErr
		}
		for _, metricURL := range metricURLs {
			metrics, metricsErr := readMetricsSnapshot(context.Background(), metricURL)
			if metricsErr != nil {
				return metricsErr
			}
			result.ServiceMetrics = append(result.ServiceMetrics, processMetrics{URL: metricURL, Snapshot: metrics})
		}
		result.ServiceLatencyMetrics = aggregateProcessLatencyMetrics(result.ServiceMetrics)
		result.ServicePGPool = aggregatePGPool(result.ServiceMetrics)
		applyLatency(
			&result.SendMessageLatencyMS,
			&result.SendMessageP95MS,
			&result.SendMessageP99MS,
			result.ServiceLatencyMetrics["send_message_latency_ms"],
		)
		applyLatency(
			&result.RepositoryAppendLatencyMS,
			&result.RepositoryAppendP95MS,
			&result.RepositoryAppendP99MS,
			result.ServiceLatencyMetrics["repository_append_latency_ms"],
		)
		applyLatency(
			&result.RepositoryCommitLatencyMS,
			&result.RepositoryCommitP95MS,
			&result.RepositoryCommitP99MS,
			result.ServiceLatencyMetrics["repository_commit_latency_ms"],
		)
		applyLatency(
			&result.ConversationSeqAllocLatencyMS,
			&result.ConversationSeqAllocP95MS,
			&result.ConversationSeqAllocP99MS,
			result.ServiceLatencyMetrics["conversation_seq_alloc_latency_ms"],
		)
	}
	if cfg.RelayMetricsURL != "" {
		metricURLs, metricsErr := normalizeMetricsURLs(cfg.RelayMetricsURL)
		if metricsErr != nil {
			return metricsErr
		}
		for _, metricURL := range metricURLs {
			metrics, metricsErr := readMetricsSnapshot(context.Background(), metricURL)
			if metricsErr != nil {
				return metricsErr
			}
			result.RelayMetrics = append(result.RelayMetrics, processMetrics{URL: metricURL, Snapshot: metrics})
		}
		result.RelayLatencyMetrics = aggregateProcessLatencyMetrics(result.RelayMetrics)
		result.RelayPGPool = aggregatePGPool(result.RelayMetrics)
		applyLatency(
			&result.KafkaPublishLatencyMS,
			&result.KafkaPublishP95MS,
			&result.KafkaPublishP99MS,
			result.RelayLatencyMetrics["kafka_publish_latency_ms"],
		)
	}

	if err := writeSummary(cfg.ResultDir, &result); err != nil {
		return err
	}
	fmt.Printf(
		"requests=%d success_rate=%.4f p95_ms=%.2f p99_ms=%.2f result=%s\n",
		result.RequestCount,
		result.SuccessRate,
		result.P95MS,
		result.P99MS,
		result.ResultFile,
	)
	return nil
}

func parseConfig(args []string, getenv func(string) string) (config, error) {
	defaultResultDir := filepath.Join("loadtest", "results", time.Now().Format("20060102-150405"))
	cfg := config{}
	flags := flag.NewFlagSet("sendmessage", flag.ContinueOnError)
	flags.StringVar(&cfg.Target, "target", envString(getenv, "NEXUSIM_TARGET", "127.0.0.1:10495"), "gRPC target or comma-separated targets, such as 127.0.0.1:10495 or 127.0.0.1:10495,127.0.0.1:10501")
	flags.IntVar(&cfg.VUs, "vus", envInt(getenv, "NEXUSIM_VUS", 10), "virtual users")
	flags.DurationVar(&cfg.Duration, "duration", envDuration(getenv, "NEXUSIM_DURATION", 30*time.Second), "test duration")
	flags.StringVar(&cfg.ResultDir, "result-dir", envString(getenv, "NEXUSIM_RESULT_DIR", defaultResultDir), "result output directory")
	flags.DurationVar(&cfg.RequestTimeout, "request-timeout", envDuration(getenv, "NEXUSIM_REQUEST_TIMEOUT", 2*time.Second), "per-request timeout")
	flags.DurationVar(&cfg.StatsWait, "stats-wait", envDuration(getenv, "NEXUSIM_STATS_WAIT", 0), "wait after traffic before reading external stats")
	flags.StringVar(&cfg.TenantID, "tenant-id", envString(getenv, "NEXUSIM_TENANT_ID", "tenant-loadtest-"+time.Now().Format("20060102150405")), "tenant id")
	flags.StringVar(&cfg.ConversationPrefix, "conversation-prefix", envString(getenv, "NEXUSIM_CONVERSATION_PREFIX", "conv-loadtest"), "conversation id prefix")
	flags.IntVar(&cfg.ConversationCount, "conversation-count", envInt(getenv, "NEXUSIM_CONVERSATION_COUNT", 1), "number of conversations to spread requests across")
	flags.StringVar(&cfg.PGDSN, "pg-dsn", envString(getenv, "NEXUSIM_PG_DSN", ""), "optional PostgreSQL DSN for outbox stats")
	flags.StringVar(&cfg.ServiceMetricsURL, "service-metrics-url", envString(getenv, "NEXUSIM_SERVICE_METRICS_URL", ""), "optional message-service gRPC process metrics URL or comma-separated URLs")
	flags.StringVar(&cfg.RelayMetricsURL, "relay-metrics-url", envString(getenv, "NEXUSIM_RELAY_METRICS_URL", ""), "optional message-service relay process metrics URL or comma-separated URLs")
	if err := flags.Parse(args); err != nil {
		return config{}, err
	}
	if cfg.Target == "" {
		return config{}, errors.New("target is required")
	}
	if cfg.VUs <= 0 {
		return config{}, errors.New("vus must be positive")
	}
	if cfg.Duration <= 0 {
		return config{}, errors.New("duration must be positive")
	}
	if cfg.ResultDir == "" {
		return config{}, errors.New("result-dir is required")
	}
	if cfg.RequestTimeout <= 0 {
		return config{}, errors.New("request-timeout must be positive")
	}
	if cfg.ConversationCount <= 0 {
		return config{}, errors.New("conversation-count must be positive")
	}
	return cfg, nil
}

func executeLoad(ctx context.Context, cfg config, clients []loadClient) (summary, error) {
	if len(clients) == 0 {
		return summary{}, errors.New("at least one gRPC client is required")
	}
	runID := time.Now().UTC().Format("20060102150405")
	started := time.Now().UTC()
	loadCtx, cancel := context.WithTimeout(ctx, cfg.Duration)
	defer cancel()

	var sequence uint64
	records := make(chan sample, cfg.VUs*16)
	var wg sync.WaitGroup
	for vu := 0; vu < cfg.VUs; vu++ {
		wg.Add(1)
		go func(vu int) {
			defer wg.Done()
			for {
				select {
				case <-loadCtx.Done():
					return
				default:
				}
				requestSeq := atomic.AddUint64(&sequence, 1)
				targetClient := clients[int((requestSeq-1)%uint64(len(clients)))]
				requestCtx, requestCancel := context.WithTimeout(ctx, cfg.RequestTimeout)
				request := buildRequest(cfg, runID, vu, requestSeq)
				before := time.Now()
				_, err := targetClient.Client.SendMessage(requestCtx, request)
				latency := time.Since(before)
				requestCancel()
				records <- sample{latency: latency, err: err}
			}
		}(vu)
	}
	go func() {
		wg.Wait()
		close(records)
	}()

	latencies := make([]time.Duration, 0, cfg.VUs*128)
	errorCounts := map[string]int64{}
	messageErrorCounts := map[messageErrorKey]int64{}
	var successCount int64
	var retryableErrorCount int64
	var serviceOverloadedCount int64
	var totalLatency time.Duration
	for record := range records {
		latencies = append(latencies, record.latency)
		totalLatency += record.latency
		if record.err != nil {
			errorCounts[errorKey(record.err)]++
			if detail, ok := messageErrorDetail(record.err); ok {
				messageErrorCounts[messageErrorKey{Code: detail.GetCode(), Retryable: detail.GetRetryable()}]++
				if detail.GetRetryable() {
					retryableErrorCount++
				}
				if detail.GetCode() == messagev1.MessageErrorCode_MESSAGE_ERROR_CODE_SERVICE_OVERLOADED {
					serviceOverloadedCount++
				}
			}
			continue
		}
		successCount++
	}
	finished := time.Now().UTC()
	requestCount := int64(len(latencies))
	errorCountValue := requestCount - successCount
	successRate := 0.0
	avgMS := 0.0
	if requestCount > 0 {
		successRate = float64(successCount) / float64(requestCount)
		avgMS = durationMS(totalLatency) / float64(requestCount)
	}

	commit := currentCommit()
	return summary{
		Commit:                        commit.Short,
		CommitFull:                    commit.Full,
		GitDirty:                      commit.Dirty,
		GitStatusShort:                commit.StatusShort,
		Target:                        cfg.Target,
		Targets:                       clientTargets(clients),
		TenantID:                      cfg.TenantID,
		VUs:                           cfg.VUs,
		Duration:                      cfg.Duration.String(),
		StatsWait:                     cfg.StatsWait.String(),
		ConversationCount:             cfg.ConversationCount,
		RequestCount:                  requestCount,
		SuccessCount:                  successCount,
		ErrorCount:                    errorCountValue,
		RetryableErrorCount:           retryableErrorCount,
		ServiceOverloadedCount:        serviceOverloadedCount,
		SuccessRate:                   successRate,
		AvgMS:                         avgMS,
		P50MS:                         durationMS(percentile(latencies, 0.50)),
		P95MS:                         durationMS(percentile(latencies, 0.95)),
		P99MS:                         durationMS(percentile(latencies, 0.99)),
		ErrorTopN:                     topErrors(errorCounts, 10),
		MessageErrorCounts:            topMessageErrors(messageErrorCounts, 10),
		StartedAt:                     started.Format(time.RFC3339Nano),
		FinishedAt:                    finished.Format(time.RFC3339Nano),
		ConversationSeqAllocLatencyMS: nil,
		ConversationSeqAllocP95MS:     nil,
		ConversationSeqAllocP99MS:     nil,
		KafkaPublishLatencyMS:         nil,
		KafkaPublishP95MS:             nil,
		KafkaPublishP99MS:             nil,
	}, nil
}

func buildRequest(cfg config, runID string, vu int, seq uint64) *messagev1.SendMessageRequest {
	conversationIndex := int(seq % uint64(cfg.ConversationCount))
	payload, err := structpb.NewStruct(map[string]any{
		"text": fmt.Sprintf("loadtest message %s %d", runID, seq),
	})
	if err != nil {
		panic(err)
	}
	return &messagev1.SendMessageRequest{
		AuthContext: &messagev1.AuthContext{
			TenantId:  cfg.TenantID,
			UserId:    fmt.Sprintf("user-%d", vu),
			DeviceId:  fmt.Sprintf("device-%d", vu),
			SessionId: fmt.Sprintf("session-%d", vu),
			TraceId:   fmt.Sprintf("trace-%s-%d", runID, seq),
			RequestId: fmt.Sprintf("request-%s-%d", runID, seq),
		},
		ConversationId: fmt.Sprintf("%s-%d", cfg.ConversationPrefix, conversationIndex),
		ClientMsgId:    fmt.Sprintf("client-%s-%d", runID, seq),
		MessageType:    "TEXT",
		Payload:        payload,
	}
}

func normalizeTarget(target string) (string, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", errors.New("target is required")
	}
	if strings.Contains(target, "://") {
		parsed, err := url.Parse(target)
		if err != nil {
			return "", err
		}
		if parsed.Host == "" {
			return "", errors.New("target URL must include host")
		}
		return parsed.Host, nil
	}
	return strings.TrimRight(target, "/"), nil
}

func normalizeTargets(targets string) ([]string, error) {
	parts := splitCSV(targets)
	if len(parts) == 0 {
		return nil, errors.New("target is required")
	}
	normalized := make([]string, 0, len(parts))
	for _, part := range parts {
		target, err := normalizeTarget(part)
		if err != nil {
			return nil, err
		}
		normalized = append(normalized, target)
	}
	return normalized, nil
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func clientTargets(clients []loadClient) []string {
	targets := make([]string, 0, len(clients))
	for _, client := range clients {
		targets = append(targets, client.Target)
	}
	return targets
}

func percentile(values []time.Duration, p float64) time.Duration {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]time.Duration(nil), values...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i] < sorted[j]
	})
	index := int(math.Ceil(float64(len(sorted))*p)) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

func topErrors(counts map[string]int64, limit int) []errorCount {
	errors := make([]errorCount, 0, len(counts))
	for err, count := range counts {
		errors = append(errors, errorCount{Error: err, Count: count})
	}
	sort.Slice(errors, func(i, j int) bool {
		if errors[i].Count == errors[j].Count {
			return errors[i].Error < errors[j].Error
		}
		return errors[i].Count > errors[j].Count
	})
	if len(errors) > limit {
		return errors[:limit]
	}
	return errors
}

func topMessageErrors(counts map[messageErrorKey]int64, limit int) []messageErrorCount {
	errors := make([]messageErrorCount, 0, len(counts))
	for key, count := range counts {
		errors = append(errors, messageErrorCount{
			Code:      key.Code.String(),
			Retryable: key.Retryable,
			Count:     count,
		})
	}
	sort.Slice(errors, func(i, j int) bool {
		if errors[i].Count == errors[j].Count {
			if errors[i].Code == errors[j].Code {
				return !errors[i].Retryable && errors[j].Retryable
			}
			return errors[i].Code < errors[j].Code
		}
		return errors[i].Count > errors[j].Count
	})
	if len(errors) > limit {
		return errors[:limit]
	}
	return errors
}

func errorKey(err error) string {
	st, ok := status.FromError(err)
	if ok {
		return st.Code().String() + ": " + st.Message()
	}
	return err.Error()
}

func messageErrorDetail(err error) (*messagev1.MessageError, bool) {
	st, ok := status.FromError(err)
	if !ok {
		return nil, false
	}
	for _, detail := range st.Details() {
		messageError, ok := detail.(*messagev1.MessageError)
		if ok {
			return messageError, true
		}
	}
	return nil, false
}

type outboxStats struct {
	Total                   int64
	Published               int64
	Pending                 int64
	DLQ                     int64
	OldestPendingAgeSeconds float64
}

type metricsSnapshot struct {
	SendMessageLatencyMS               latencySnapshot `json:"send_message_latency_ms"`
	RepositoryAppendLatencyMS          latencySnapshot `json:"repository_append_latency_ms"`
	RepositoryBeginLatencyMS           latencySnapshot `json:"repository_begin_latency_ms"`
	RepositoryPoolAcquireLatencyMS     latencySnapshot `json:"repository_pool_acquire_latency_ms"`
	RepositoryTxBeginLatencyMS         latencySnapshot `json:"repository_tx_begin_latency_ms"`
	RepositoryIdempotencyLockLatencyMS latencySnapshot `json:"repository_idempotency_lock_latency_ms"`
	RepositoryFindExistingLatencyMS    latencySnapshot `json:"repository_find_existing_latency_ms"`
	RepositoryEnsureSeqLatencyMS       latencySnapshot `json:"repository_ensure_seq_latency_ms"`
	RepositoryAllocateSeqLatencyMS     latencySnapshot `json:"repository_allocate_seq_latency_ms"`
	RepositoryInsertMessageLatencyMS   latencySnapshot `json:"repository_insert_message_latency_ms"`
	RepositoryInsertTimelineLatencyMS  latencySnapshot `json:"repository_insert_timeline_latency_ms"`
	RepositoryInsertOutboxLatencyMS    latencySnapshot `json:"repository_insert_outbox_latency_ms"`
	RepositoryCommitLatencyMS          latencySnapshot `json:"repository_commit_latency_ms"`
	ConversationSeqAllocLatencyMS      latencySnapshot `json:"conversation_seq_alloc_latency_ms"`
	KafkaPublishLatencyMS              latencySnapshot `json:"kafka_publish_latency_ms"`
	PGPool                             *pgPoolStats    `json:"pg_pool"`
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

func readOutboxStats(ctx context.Context, dsn string, tenantID string) (outboxStats, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return outboxStats{}, err
	}
	defer pool.Close()

	var stats outboxStats
	err = pool.QueryRow(ctx, `
SELECT
    count(*)::bigint AS total_count,
    count(*) FILTER (WHERE status = 'PUBLISHED')::bigint AS published_count,
    count(*) FILTER (WHERE status = 'PENDING' AND published_at IS NULL)::bigint AS pending_count,
    count(*) FILTER (WHERE status = 'DLQ')::bigint AS dlq_count,
    COALESCE(
        EXTRACT(EPOCH FROM now() - MIN(available_at) FILTER (WHERE status = 'PENDING' AND published_at IS NULL)),
        0
    )::float8 AS oldest_pending_age_seconds
FROM message_outbox
WHERE tenant_id = $1
`, tenantID).Scan(
		&stats.Total,
		&stats.Published,
		&stats.Pending,
		&stats.DLQ,
		&stats.OldestPendingAgeSeconds,
	)
	if err != nil {
		return outboxStats{}, err
	}
	return stats, nil
}

func readMetricsSnapshot(ctx context.Context, metricsURL string) (metricsSnapshot, error) {
	normalized, err := normalizeMetricsURL(metricsURL)
	if err != nil {
		return metricsSnapshot{}, err
	}
	requestCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, normalized, nil)
	if err != nil {
		return metricsSnapshot{}, err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return metricsSnapshot{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return metricsSnapshot{}, fmt.Errorf("read metrics %s: unexpected status %d", normalized, response.StatusCode)
	}
	var snapshot metricsSnapshot
	if err := json.NewDecoder(response.Body).Decode(&snapshot); err != nil {
		return metricsSnapshot{}, err
	}
	return snapshot, nil
}

func normalizeMetricsURL(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("metrics URL is required")
	}
	if !strings.Contains(value, "://") {
		value = "http://" + value
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return "", err
	}
	if parsed.Host == "" {
		return "", errors.New("metrics URL must include host")
	}
	if parsed.Path == "" || parsed.Path == "/" {
		parsed.Path = "/debug/metrics"
	}
	return parsed.String(), nil
}

func normalizeMetricsURLs(values string) ([]string, error) {
	parts := splitCSV(values)
	if len(parts) == 0 {
		return nil, errors.New("metrics URL is required")
	}
	normalized := make([]string, 0, len(parts))
	for _, part := range parts {
		value, err := normalizeMetricsURL(part)
		if err != nil {
			return nil, err
		}
		normalized = append(normalized, value)
	}
	return normalized, nil
}

func latencyMetrics(snapshot metricsSnapshot) map[string]latencySnapshot {
	metrics := map[string]latencySnapshot{}
	addLatency(metrics, "send_message_latency_ms", snapshot.SendMessageLatencyMS)
	addLatency(metrics, "repository_append_latency_ms", snapshot.RepositoryAppendLatencyMS)
	addLatency(metrics, "repository_begin_latency_ms", snapshot.RepositoryBeginLatencyMS)
	addLatency(metrics, "repository_pool_acquire_latency_ms", snapshot.RepositoryPoolAcquireLatencyMS)
	addLatency(metrics, "repository_tx_begin_latency_ms", snapshot.RepositoryTxBeginLatencyMS)
	addLatency(metrics, "repository_idempotency_lock_latency_ms", snapshot.RepositoryIdempotencyLockLatencyMS)
	addLatency(metrics, "repository_find_existing_latency_ms", snapshot.RepositoryFindExistingLatencyMS)
	addLatency(metrics, "repository_ensure_seq_latency_ms", snapshot.RepositoryEnsureSeqLatencyMS)
	addLatency(metrics, "repository_allocate_seq_latency_ms", snapshot.RepositoryAllocateSeqLatencyMS)
	addLatency(metrics, "repository_insert_message_latency_ms", snapshot.RepositoryInsertMessageLatencyMS)
	addLatency(metrics, "repository_insert_timeline_latency_ms", snapshot.RepositoryInsertTimelineLatencyMS)
	addLatency(metrics, "repository_insert_outbox_latency_ms", snapshot.RepositoryInsertOutboxLatencyMS)
	addLatency(metrics, "repository_commit_latency_ms", snapshot.RepositoryCommitLatencyMS)
	addLatency(metrics, "conversation_seq_alloc_latency_ms", snapshot.ConversationSeqAllocLatencyMS)
	addLatency(metrics, "kafka_publish_latency_ms", snapshot.KafkaPublishLatencyMS)
	if len(metrics) == 0 {
		return nil
	}
	return metrics
}

func aggregateProcessLatencyMetrics(processes []processMetrics) map[string]latencySnapshot {
	aggregated := map[string]latencySnapshot{}
	for _, process := range processes {
		for name, snapshot := range latencyMetrics(process.Snapshot) {
			aggregated[name] = mergeLatency(aggregated[name], snapshot)
		}
	}
	if len(aggregated) == 0 {
		return nil
	}
	return aggregated
}

func mergeLatency(left latencySnapshot, right latencySnapshot) latencySnapshot {
	if left.Count <= 0 {
		return right
	}
	if right.Count <= 0 {
		return left
	}
	totalCount := left.Count + right.Count
	avg := ((left.AvgMS * float64(left.Count)) + (right.AvgMS * float64(right.Count))) / float64(totalCount)
	return latencySnapshot{
		Count: totalCount,
		AvgMS: avg,
		P95MS: math.Max(left.P95MS, right.P95MS),
		P99MS: math.Max(left.P99MS, right.P99MS),
	}
}

func addLatency(metrics map[string]latencySnapshot, name string, snapshot latencySnapshot) {
	if snapshot.Count > 0 {
		metrics[name] = snapshot
	}
}

func aggregatePGPool(processes []processMetrics) *pgPoolStats {
	var aggregated pgPoolStats
	hasStats := false
	for _, process := range processes {
		stats := process.Snapshot.PGPool
		if stats == nil {
			continue
		}
		hasStats = true
		aggregated.AcquireCount += stats.AcquireCount
		aggregated.AcquireDurationMS += stats.AcquireDurationMS
		aggregated.AcquiredConns += stats.AcquiredConns
		aggregated.CanceledAcquireCount += stats.CanceledAcquireCount
		aggregated.ConstructingConns += stats.ConstructingConns
		aggregated.EmptyAcquireCount += stats.EmptyAcquireCount
		aggregated.IdleConns += stats.IdleConns
		aggregated.MaxConns += stats.MaxConns
		aggregated.TotalConns += stats.TotalConns
	}
	if !hasStats {
		return nil
	}
	return &aggregated
}

func applyLatency(avgTarget **float64, p95Target **float64, p99Target **float64, snapshot latencySnapshot) {
	if snapshot.Count <= 0 {
		return
	}
	avg := snapshot.AvgMS
	p95 := snapshot.P95MS
	p99 := snapshot.P99MS
	*avgTarget = &avg
	*p95Target = &p95
	*p99Target = &p99
}

func writeSummary(resultDir string, result *summary) error {
	if err := os.MkdirAll(resultDir, 0755); err != nil {
		return err
	}
	result.ResultFile = filepath.Join(resultDir, "sendmessage-summary.json")
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(result.ResultFile, encoded, 0644)
}

type commitInfo struct {
	Short       string
	Full        string
	Dirty       bool
	StatusShort string
}

func currentCommit() commitInfo {
	if override := commitInfoFromEnv(); override.Short != "" {
		return override
	}
	shortOutput, err := exec.Command("git", "rev-parse", "--short", "HEAD").Output()
	if err != nil {
		return commitInfo{Short: "unknown", Full: "unknown"}
	}
	fullOutput, err := exec.Command("git", "rev-parse", "HEAD").Output()
	full := "unknown"
	if err == nil {
		full = strings.TrimSpace(string(fullOutput))
	}
	statusOutput, err := exec.Command("git", "status", "--short").Output()
	statusShort := ""
	if err == nil {
		statusShort = strings.TrimSpace(string(statusOutput))
	}
	short := strings.TrimSpace(string(shortOutput))
	dirty := statusShort != ""
	if dirty {
		short += "-dirty"
	}
	return commitInfo{
		Short:       short,
		Full:        full,
		Dirty:       dirty,
		StatusShort: statusShort,
	}
}

func commitInfoFromEnv() commitInfo {
	short := strings.TrimSpace(os.Getenv("NEXUSIM_COMMIT"))
	if short == "" {
		return commitInfo{}
	}
	full := strings.TrimSpace(os.Getenv("NEXUSIM_COMMIT_FULL"))
	if full == "" {
		full = short
	}
	statusShort := strings.TrimSpace(os.Getenv("NEXUSIM_GIT_STATUS_SHORT"))
	dirty, _ := strconv.ParseBool(strings.TrimSpace(os.Getenv("NEXUSIM_GIT_DIRTY")))
	if statusShort != "" {
		dirty = true
	}
	if dirty && !strings.HasSuffix(short, "-dirty") {
		short += "-dirty"
	}
	return commitInfo{
		Short:       short,
		Full:        full,
		Dirty:       dirty,
		StatusShort: statusShort,
	}
}

func durationMS(value time.Duration) float64 {
	return float64(value) / float64(time.Millisecond)
}

func envString(getenv func(string) string, name string, fallback string) string {
	value := strings.TrimSpace(getenv(name))
	if value == "" {
		return fallback
	}
	return value
}

func envInt(getenv func(string) string, name string, fallback int) int {
	value := strings.TrimSpace(getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func envDuration(getenv func(string) string, name string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}
