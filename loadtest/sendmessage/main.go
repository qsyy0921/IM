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

type summary struct {
	Commit                        string       `json:"commit"`
	CommitFull                    string       `json:"commit_full"`
	GitDirty                      bool         `json:"git_dirty"`
	GitStatusShort                string       `json:"git_status_short"`
	Target                        string       `json:"target"`
	TenantID                      string       `json:"tenant_id"`
	VUs                           int          `json:"vus"`
	Duration                      string       `json:"duration"`
	StatsWait                     string       `json:"stats_wait"`
	ConversationCount             int          `json:"conversation_count"`
	RequestCount                  int64        `json:"request_count"`
	SuccessCount                  int64        `json:"success_count"`
	ErrorCount                    int64        `json:"error_count"`
	SuccessRate                   float64      `json:"success_rate"`
	AvgMS                         float64      `json:"avg_ms"`
	P50MS                         float64      `json:"p50_ms"`
	P95MS                         float64      `json:"p95_ms"`
	P99MS                         float64      `json:"p99_ms"`
	ConversationSeqAllocLatencyMS *float64     `json:"conversation_seq_alloc_latency_ms"`
	ConversationSeqAllocP95MS     *float64     `json:"conversation_seq_alloc_p95_ms"`
	OutboxTotalCount              *int64       `json:"outbox_total_count"`
	OutboxPublishedCount          *int64       `json:"outbox_published_count"`
	OutboxPendingCount            *int64       `json:"outbox_pending_count"`
	OutboxDLQCount                *int64       `json:"outbox_dlq_count"`
	OutboxOldestPendingAgeSeconds *float64     `json:"outbox_oldest_pending_age_seconds"`
	KafkaPublishLatencyMS         *float64     `json:"kafka_publish_latency_ms"`
	KafkaPublishP95MS             *float64     `json:"kafka_publish_p95_ms"`
	ErrorTopN                     []errorCount `json:"error_topn"`
	StartedAt                     string       `json:"started_at"`
	FinishedAt                    string       `json:"finished_at"`
	ResultFile                    string       `json:"result_file"`
}

type errorCount struct {
	Error string `json:"error"`
	Count int64  `json:"count"`
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
	grpcTarget, err := normalizeTarget(cfg.Target)
	if err != nil {
		return err
	}

	conn, err := grpc.NewClient(grpcTarget, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	defer conn.Close()

	client := messagev1.NewMessageServiceClient(conn)
	result, err := executeLoad(context.Background(), cfg, client)
	if err != nil {
		return err
	}
	result.Target = grpcTarget
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
		metrics, metricsErr := readMetricsSnapshot(context.Background(), cfg.ServiceMetricsURL)
		if metricsErr != nil {
			return metricsErr
		}
		applyLatency(&result.ConversationSeqAllocLatencyMS, &result.ConversationSeqAllocP95MS, metrics.ConversationSeqAllocLatencyMS)
	}
	if cfg.RelayMetricsURL != "" {
		metrics, metricsErr := readMetricsSnapshot(context.Background(), cfg.RelayMetricsURL)
		if metricsErr != nil {
			return metricsErr
		}
		applyLatency(&result.KafkaPublishLatencyMS, &result.KafkaPublishP95MS, metrics.KafkaPublishLatencyMS)
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
	flags.StringVar(&cfg.Target, "target", envString(getenv, "NEXUSIM_TARGET", "127.0.0.1:10495"), "gRPC target, such as 127.0.0.1:10495 or http://192.168.0.141:10495")
	flags.IntVar(&cfg.VUs, "vus", envInt(getenv, "NEXUSIM_VUS", 10), "virtual users")
	flags.DurationVar(&cfg.Duration, "duration", envDuration(getenv, "NEXUSIM_DURATION", 30*time.Second), "test duration")
	flags.StringVar(&cfg.ResultDir, "result-dir", envString(getenv, "NEXUSIM_RESULT_DIR", defaultResultDir), "result output directory")
	flags.DurationVar(&cfg.RequestTimeout, "request-timeout", envDuration(getenv, "NEXUSIM_REQUEST_TIMEOUT", 2*time.Second), "per-request timeout")
	flags.DurationVar(&cfg.StatsWait, "stats-wait", envDuration(getenv, "NEXUSIM_STATS_WAIT", 0), "wait after traffic before reading external stats")
	flags.StringVar(&cfg.TenantID, "tenant-id", envString(getenv, "NEXUSIM_TENANT_ID", "tenant-loadtest-"+time.Now().Format("20060102150405")), "tenant id")
	flags.StringVar(&cfg.ConversationPrefix, "conversation-prefix", envString(getenv, "NEXUSIM_CONVERSATION_PREFIX", "conv-loadtest"), "conversation id prefix")
	flags.IntVar(&cfg.ConversationCount, "conversation-count", envInt(getenv, "NEXUSIM_CONVERSATION_COUNT", 1), "number of conversations to spread requests across")
	flags.StringVar(&cfg.PGDSN, "pg-dsn", envString(getenv, "NEXUSIM_PG_DSN", ""), "optional PostgreSQL DSN for outbox stats")
	flags.StringVar(&cfg.ServiceMetricsURL, "service-metrics-url", envString(getenv, "NEXUSIM_SERVICE_METRICS_URL", ""), "optional message-service gRPC process metrics URL")
	flags.StringVar(&cfg.RelayMetricsURL, "relay-metrics-url", envString(getenv, "NEXUSIM_RELAY_METRICS_URL", ""), "optional message-service relay process metrics URL")
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

func executeLoad(ctx context.Context, cfg config, client messagev1.MessageServiceClient) (summary, error) {
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
				requestCtx, requestCancel := context.WithTimeout(ctx, cfg.RequestTimeout)
				request := buildRequest(cfg, runID, vu, requestSeq)
				before := time.Now()
				_, err := client.SendMessage(requestCtx, request)
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
	var successCount int64
	var totalLatency time.Duration
	for record := range records {
		latencies = append(latencies, record.latency)
		totalLatency += record.latency
		if record.err != nil {
			errorCounts[errorKey(record.err)]++
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
		TenantID:                      cfg.TenantID,
		VUs:                           cfg.VUs,
		Duration:                      cfg.Duration.String(),
		StatsWait:                     cfg.StatsWait.String(),
		ConversationCount:             cfg.ConversationCount,
		RequestCount:                  requestCount,
		SuccessCount:                  successCount,
		ErrorCount:                    errorCountValue,
		SuccessRate:                   successRate,
		AvgMS:                         avgMS,
		P50MS:                         durationMS(percentile(latencies, 0.50)),
		P95MS:                         durationMS(percentile(latencies, 0.95)),
		P99MS:                         durationMS(percentile(latencies, 0.99)),
		ErrorTopN:                     topErrors(errorCounts, 10),
		StartedAt:                     started.Format(time.RFC3339Nano),
		FinishedAt:                    finished.Format(time.RFC3339Nano),
		ConversationSeqAllocLatencyMS: nil,
		ConversationSeqAllocP95MS:     nil,
		KafkaPublishLatencyMS:         nil,
		KafkaPublishP95MS:             nil,
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

func errorKey(err error) string {
	st, ok := status.FromError(err)
	if ok {
		return st.Code().String() + ": " + st.Message()
	}
	return err.Error()
}

type outboxStats struct {
	Total                   int64
	Published               int64
	Pending                 int64
	DLQ                     int64
	OldestPendingAgeSeconds float64
}

type metricsSnapshot struct {
	ConversationSeqAllocLatencyMS latencySnapshot `json:"conversation_seq_alloc_latency_ms"`
	KafkaPublishLatencyMS         latencySnapshot `json:"kafka_publish_latency_ms"`
}

type latencySnapshot struct {
	Count int64   `json:"count"`
	AvgMS float64 `json:"avg_ms"`
	P95MS float64 `json:"p95_ms"`
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

func applyLatency(avgTarget **float64, p95Target **float64, snapshot latencySnapshot) {
	if snapshot.Count <= 0 {
		return
	}
	avg := snapshot.AvgMS
	p95 := snapshot.P95MS
	*avgTarget = &avg
	*p95Target = &p95
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
