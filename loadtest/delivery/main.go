package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	deliveryv1 "github.com/qsyy0921/IM/api/proto/nexusim/delivery/v1"
	"github.com/qsyy0921/IM/loadtest/internal/grpctls"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const (
	metadataTenantID  = "x-nexusim-tenant-id"
	metadataUserID    = "x-nexusim-user-id"
	metadataDeviceID  = "x-nexusim-device-id"
	metadataSessionID = "x-nexusim-session-id"
	metadataTraceID   = "x-nexusim-trace-id"
	metadataRequestID = "x-nexusim-request-id"
)

type config struct {
	target               string
	tls                  grpctls.Config
	resultDir            string
	requestTimeout       time.Duration
	waitTimeout          time.Duration
	pollInterval         time.Duration
	duration             time.Duration
	vus                  int
	tenantID             string
	userID               string
	deviceID             string
	conversationID       string
	afterSeq             int64
	limit                int32
	expectedCount        int
	pgDSN                string
	consumerGroup        string
	ack                  bool
	verifiedAuthMetadata bool
}

type summary struct {
	Commit                string           `json:"commit"`
	CommitFull            string           `json:"commit_full"`
	GitDirty              bool             `json:"git_dirty"`
	GitStatusShort        string           `json:"git_status_short,omitempty"`
	Target                string           `json:"target"`
	TLSEnabled            bool             `json:"tls_enabled"`
	VerifiedAuthMetadata  bool             `json:"verified_auth_metadata"`
	TenantID              string           `json:"tenant_id"`
	UserID                string           `json:"user_id"`
	DeviceID              string           `json:"device_id"`
	ConversationID        string           `json:"conversation_id"`
	AfterSeq              int64            `json:"after_seq"`
	Limit                 int32            `json:"limit"`
	ExpectedCount         int              `json:"expected_count"`
	RequestedDurationSec  float64          `json:"requested_duration_seconds,omitempty"`
	VUs                   int              `json:"vus,omitempty"`
	ConsumerGroup         string           `json:"consumer_group,omitempty"`
	PollCount             int              `json:"poll_count"`
	ItemCount             int              `json:"item_count"`
	MaxSeq                int64            `json:"max_seq"`
	HasMore               bool             `json:"has_more"`
	AckEnabled            bool             `json:"ack_enabled"`
	AckLastReceivedSeq    int64            `json:"ack_last_received_seq,omitempty"`
	Success               bool             `json:"success"`
	Error                 string           `json:"error,omitempty"`
	WaitTimeout           string           `json:"wait_timeout"`
	PollInterval          string           `json:"poll_interval"`
	PullAvgMS             float64          `json:"pull_avg_ms"`
	PullP95MS             float64          `json:"pull_p95_ms"`
	PullP99MS             float64          `json:"pull_p99_ms"`
	AckLatencyMS          float64          `json:"ack_latency_ms,omitempty"`
	InboxCount            *int64           `json:"inbox_count,omitempty"`
	DeliveryOutboxTotal   *int64           `json:"delivery_outbox_total,omitempty"`
	DeliveryOutboxPending *int64           `json:"delivery_outbox_pending,omitempty"`
	DeliveryOutboxDLQ     *int64           `json:"delivery_outbox_dlq,omitempty"`
	CursorLastReceivedSeq *int64           `json:"cursor_last_received_seq,omitempty"`
	CheckpointOffsetValue *int64           `json:"checkpoint_offset_value,omitempty"`
	Capacity              *capacitySummary `json:"capacity_summary,omitempty"`
	StartedAt             time.Time        `json:"started_at"`
	FinishedAt            time.Time        `json:"finished_at"`
	Items                 []pulledItem     `json:"items,omitempty"`
}

type capacitySummary struct {
	DurationMS            float64 `json:"duration_ms"`
	PollCount             int     `json:"poll_count"`
	ItemCount             int     `json:"item_count"`
	ExpectedCount         int     `json:"expected_count"`
	PullsPerSecond        float64 `json:"pulls_per_second"`
	ItemsPerSecond        float64 `json:"items_per_second"`
	PullP95MS             float64 `json:"pull_p95_ms"`
	PullP99MS             float64 `json:"pull_p99_ms"`
	AckEnabled            bool    `json:"ack_enabled"`
	AckLatencyMS          float64 `json:"ack_latency_ms,omitempty"`
	InboxCount            *int64  `json:"inbox_count,omitempty"`
	DeliveryOutboxTotal   *int64  `json:"delivery_outbox_total,omitempty"`
	DeliveryOutboxPending *int64  `json:"delivery_outbox_pending,omitempty"`
	DeliveryOutboxDLQ     *int64  `json:"delivery_outbox_dlq,omitempty"`
	CheckpointOffsetValue *int64  `json:"checkpoint_offset_value,omitempty"`
}

type pulledItem struct {
	ConversationSeq int64  `json:"conversation_seq"`
	EventID         string `json:"event_id"`
	EventType       string `json:"event_type"`
	MessageID       string `json:"message_id"`
	SenderID        string `json:"sender_id"`
}

type verifiedAuthIdentity struct {
	tenantID  string
	userID    string
	deviceID  string
	sessionID string
	traceID   string
	requestID string
}

func main() {
	cfg := parseConfig()
	if err := run(cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func parseConfig() config {
	var cfg config
	var limit int
	flag.StringVar(&cfg.target, "target", "127.0.0.1:10497", "delivery-service gRPC target")
	registerTLSFlags("delivery-tls", "NEXUSIM_DELIVERY_TLS", "delivery-service", &cfg.tls)
	flag.StringVar(&cfg.resultDir, "result-dir", `H:\NexusIM\loadtest-results\delivery-smoke`, "result directory")
	flag.DurationVar(&cfg.requestTimeout, "request-timeout", 2*time.Second, "per-request timeout")
	flag.DurationVar(&cfg.waitTimeout, "wait-timeout", 10*time.Second, "max wait for expected inbox items")
	flag.DurationVar(&cfg.pollInterval, "poll-interval", 200*time.Millisecond, "pull retry interval while waiting")
	flag.DurationVar(&cfg.duration, "duration", 0, "capacity run duration; zero waits until expected count")
	flag.IntVar(&cfg.vus, "vus", 1, "virtual users for duration mode")
	flag.StringVar(&cfg.tenantID, "tenant-id", "tenant-delivery-smoke", "tenant id")
	flag.StringVar(&cfg.userID, "user-id", "delivery-user-1", "inbox owner user id")
	flag.StringVar(&cfg.deviceID, "device-id", "delivery-device-1", "ack device id")
	flag.StringVar(&cfg.conversationID, "conversation-id", "conv-delivery-smoke", "conversation id")
	flag.Int64Var(&cfg.afterSeq, "after-seq", 0, "PullInbox after_seq")
	flag.IntVar(&limit, "limit", 100, "PullInbox limit")
	flag.IntVar(&cfg.expectedCount, "expected-count", 1, "minimum inbox items expected before success")
	flag.StringVar(&cfg.pgDSN, "pg-dsn", "", "optional PostgreSQL DSN for stats")
	flag.StringVar(&cfg.consumerGroup, "consumer-group", "", "optional delivery timeline consumer group for checkpoint stats")
	flag.BoolVar(&cfg.ack, "ack", true, "ack max pulled conversation seq")
	flag.BoolVar(&cfg.verifiedAuthMetadata, "verified-auth-metadata", envBool(false, "NEXUSIM_DELIVERY_LOADTEST_VERIFIED_AUTH_METADATA"), "send gateway verified identity through delivery gRPC metadata")
	flag.Parse()
	if cfg.requestTimeout <= 0 {
		cfg.requestTimeout = 2 * time.Second
	}
	if cfg.waitTimeout <= 0 {
		cfg.waitTimeout = 10 * time.Second
	}
	if cfg.pollInterval <= 0 {
		cfg.pollInterval = 200 * time.Millisecond
	}
	if cfg.duration < 0 {
		cfg.duration = 0
	}
	if cfg.vus <= 0 {
		cfg.vus = 1
	}
	if limit <= 0 {
		limit = 100
	}
	cfg.limit = int32(limit)
	return cfg
}

func registerTLSFlags(prefix string, envPrefix string, serviceName string, config *grpctls.Config) {
	flag.StringVar(&config.CAFile, prefix+"-ca-file", os.Getenv(envPrefix+"_CA_FILE"), "CA PEM for "+serviceName+" gRPC TLS")
	flag.StringVar(&config.ServerName, prefix+"-server-name", os.Getenv(envPrefix+"_SERVER_NAME"), "override server name for "+serviceName+" gRPC TLS")
	flag.StringVar(&config.ClientCertFile, prefix+"-client-cert-file", os.Getenv(envPrefix+"_CLIENT_CERT_FILE"), "client certificate PEM for "+serviceName+" gRPC mTLS")
	flag.StringVar(&config.ClientKeyFile, prefix+"-client-key-file", os.Getenv(envPrefix+"_CLIENT_KEY_FILE"), "client private key PEM for "+serviceName+" gRPC mTLS")
}

func deliverySmokeAuth(cfg config, traceID string, requestID string) verifiedAuthIdentity {
	return verifiedAuthIdentity{
		tenantID:  cfg.tenantID,
		userID:    cfg.userID,
		deviceID:  cfg.deviceID,
		sessionID: "delivery-smoke",
		traceID:   traceID,
		requestID: requestID,
	}
}

func withVerifiedAuthMetadata(ctx context.Context, cfg config, auth verifiedAuthIdentity) context.Context {
	if !cfg.verifiedAuthMetadata {
		return ctx
	}
	pairs := []string{
		metadataTenantID, auth.tenantID,
		metadataUserID, auth.userID,
		metadataDeviceID, auth.deviceID,
	}
	if auth.sessionID != "" {
		pairs = append(pairs, metadataSessionID, auth.sessionID)
	}
	if auth.traceID != "" {
		pairs = append(pairs, metadataTraceID, auth.traceID)
	}
	if auth.requestID != "" {
		pairs = append(pairs, metadataRequestID, auth.requestID)
	}
	return metadata.NewOutgoingContext(ctx, metadata.Pairs(pairs...))
}

func deliveryAuth(auth verifiedAuthIdentity) *deliveryv1.AuthContext {
	return &deliveryv1.AuthContext{
		TenantId:  auth.tenantID,
		UserId:    auth.userID,
		DeviceId:  auth.deviceID,
		SessionId: auth.sessionID,
		TraceId:   auth.traceID,
		RequestId: auth.requestID,
	}
}

func run(cfg config) error {
	if err := os.MkdirAll(cfg.resultDir, 0o755); err != nil {
		return fmt.Errorf("create result dir: %w", err)
	}
	dialOption, err := grpctls.DialOption(cfg.tls, "delivery-tls")
	if err != nil {
		return fmt.Errorf("configure delivery-service TLS: %w", err)
	}
	conn, err := grpc.NewClient(cfg.target, dialOption)
	if err != nil {
		return fmt.Errorf("dial target: %w", err)
	}
	defer conn.Close()
	client := deliveryv1.NewDeliveryServiceClient(conn)

	result := summary{
		Commit:               shortCommit(),
		CommitFull:           fullCommit(),
		GitDirty:             gitDirty(),
		GitStatusShort:       gitStatusShort(),
		Target:               cfg.target,
		TLSEnabled:           cfg.tls.Enabled(),
		VerifiedAuthMetadata: cfg.verifiedAuthMetadata,
		TenantID:             cfg.tenantID,
		UserID:               cfg.userID,
		DeviceID:             cfg.deviceID,
		ConversationID:       cfg.conversationID,
		AfterSeq:             cfg.afterSeq,
		Limit:                cfg.limit,
		ExpectedCount:        cfg.expectedCount,
		RequestedDurationSec: cfg.duration.Seconds(),
		VUs:                  cfg.vus,
		ConsumerGroup:        cfg.consumerGroup,
		AckEnabled:           cfg.ack,
		WaitTimeout:          cfg.waitTimeout.String(),
		PollInterval:         cfg.pollInterval.String(),
		StartedAt:            time.Now().UTC(),
	}

	latencies := make([]float64, 0, 16)
	if cfg.duration > 0 {
		if err := runDurationPulls(cfg, client, &result, &latencies); err != nil {
			result.Error = err.Error()
		}
	} else {
		if err := runWaitPulls(cfg, client, &result, &latencies); err != nil {
			result.Error = err.Error()
		}
	}
	result.PullAvgMS, result.PullP95MS, result.PullP99MS = summarizeLatencies(latencies)
	result.Success = result.Error == "" && result.ItemCount >= cfg.expectedCount

	if result.Success && cfg.ack && result.MaxSeq > 0 {
		ackCtx, cancel := context.WithTimeout(context.Background(), cfg.requestTimeout)
		begin := time.Now()
		auth := deliverySmokeAuth(cfg, "delivery-ack", "delivery-ack")
		ackCtx = withVerifiedAuthMetadata(ackCtx, cfg, auth)
		response, err := client.AckDelivery(ackCtx, &deliveryv1.AckDeliveryRequest{
			AuthContext:    deliveryAuth(auth),
			ConversationId: cfg.conversationID,
			ReceivedSeq:    result.MaxSeq,
		})
		result.AckLatencyMS = float64(time.Since(begin).Microseconds()) / 1000
		cancel()
		if err != nil {
			result.Success = false
			result.Error = err.Error()
		} else {
			result.AckLastReceivedSeq = response.GetLastReceivedSeq()
		}
	}
	if cfg.pgDSN != "" {
		if err := fillPostgresStats(context.Background(), cfg, &result); err != nil && result.Error == "" {
			result.Error = err.Error()
			result.Success = false
		}
	}
	result.FinishedAt = time.Now().UTC()
	result.Capacity = buildCapacitySummary(&result)

	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("encode summary: %w", err)
	}
	path := filepath.Join(cfg.resultDir, "delivery-summary.json")
	if err := os.WriteFile(path, encoded, 0o644); err != nil {
		return fmt.Errorf("write summary: %w", err)
	}
	fmt.Println(string(encoded))
	fmt.Printf("summary: %s\n", path)
	if !result.Success {
		return fmt.Errorf("delivery smoke failed: %s", firstNonEmpty(result.Error, "expected item count not reached"))
	}
	return nil
}

type pullResult struct {
	response  *deliveryv1.PullInboxResponse
	latencyMS float64
}

func runWaitPulls(
	cfg config,
	client deliveryv1.DeliveryServiceClient,
	result *summary,
	latencies *[]float64,
) error {
	deadline := time.Now().Add(cfg.waitTimeout)
	for {
		pulled, err := pullInbox(context.Background(), cfg, client, fmt.Sprintf("delivery-pull-%d", result.PollCount+1))
		result.PollCount++
		*latencies = append(*latencies, pulled.latencyMS)
		if err != nil {
			return err
		}
		recordPullResponse(result, pulled.response, false)
		if len(pulled.response.GetItems()) >= cfg.expectedCount || time.Now().After(deadline) {
			return nil
		}
		time.Sleep(cfg.pollInterval)
	}
}

func runDurationPulls(
	cfg config,
	client deliveryv1.DeliveryServiceClient,
	result *summary,
	latencies *[]float64,
) error {
	ctx, cancel := context.WithTimeout(context.Background(), cfg.duration)
	defer cancel()

	var mu sync.Mutex
	var wg sync.WaitGroup
	var firstErr error
	for workerID := 0; workerID < cfg.vus; workerID++ {
		workerID := workerID
		wg.Add(1)
		go func() {
			defer wg.Done()
			iteration := 0
			for {
				if ctx.Err() != nil {
					return
				}
				requestID := fmt.Sprintf("delivery-pull-vu-%d-%d", workerID, iteration)
				pulled, err := pullInbox(ctx, cfg, client, requestID)
				if err != nil {
					if ctx.Err() != nil {
						return
					}
					mu.Lock()
					if firstErr == nil {
						firstErr = err
						cancel()
					}
					mu.Unlock()
					return
				}
				mu.Lock()
				result.PollCount++
				*latencies = append(*latencies, pulled.latencyMS)
				recordPullResponse(result, pulled.response, true)
				mu.Unlock()
				iteration++
			}
		}()
	}
	wg.Wait()
	if firstErr != nil {
		return firstErr
	}
	if result.PollCount == 0 {
		return fmt.Errorf("delivery capacity run produced no pulls")
	}
	return nil
}

func pullInbox(
	parent context.Context,
	cfg config,
	client deliveryv1.DeliveryServiceClient,
	requestID string,
) (pullResult, error) {
	pullCtx, cancel := context.WithTimeout(parent, cfg.requestTimeout)
	defer cancel()
	begin := time.Now()
	auth := deliverySmokeAuth(cfg, requestID, requestID)
	pullCtx = withVerifiedAuthMetadata(pullCtx, cfg, auth)
	response, err := client.PullInbox(pullCtx, &deliveryv1.PullInboxRequest{
		AuthContext:    deliveryAuth(auth),
		ConversationId: cfg.conversationID,
		AfterSeq:       cfg.afterSeq,
		Limit:          cfg.limit,
	})
	elapsedMS := float64(time.Since(begin).Microseconds()) / 1000
	if err != nil {
		return pullResult{latencyMS: elapsedMS}, fmt.Errorf("pull inbox: %w", err)
	}
	return pullResult{response: response, latencyMS: elapsedMS}, nil
}

func recordPullResponse(result *summary, response *deliveryv1.PullInboxResponse, aggregate bool) {
	if response == nil {
		return
	}
	const maxItemSamples = 100
	items := response.GetItems()
	if aggregate {
		result.ItemCount += len(items)
	} else {
		result.ItemCount = len(items)
		result.MaxSeq = 0
		result.Items = result.Items[:0]
	}
	result.HasMore = response.GetHasMore()
	for _, item := range items {
		if item.GetConversationSeq() > result.MaxSeq {
			result.MaxSeq = item.GetConversationSeq()
		}
		if len(result.Items) < maxItemSamples {
			result.Items = append(result.Items, pulledItem{
				ConversationSeq: item.GetConversationSeq(),
				EventID:         item.GetEventId(),
				EventType:       item.GetEventType(),
				MessageID:       item.GetMessageId(),
				SenderID:        item.GetSenderId(),
			})
		}
	}
}

func fillPostgresStats(ctx context.Context, cfg config, result *summary) error {
	pool, err := pgxpool.New(ctx, cfg.pgDSN)
	if err != nil {
		return fmt.Errorf("open pg pool: %w", err)
	}
	defer pool.Close()
	assign := func(target **int64, query string, args ...any) error {
		var value int64
		if err := pool.QueryRow(ctx, query, args...).Scan(&value); err != nil {
			return err
		}
		*target = &value
		return nil
	}
	if err := assign(&result.InboxCount, `
SELECT COUNT(*) FROM user_inbox
WHERE tenant_id = $1 AND conversation_id = $2 AND user_id = $3
`, cfg.tenantID, cfg.conversationID, cfg.userID); err != nil {
		return fmt.Errorf("query inbox count: %w", err)
	}
	if err := assign(&result.DeliveryOutboxTotal, `
SELECT COUNT(*) FROM delivery_outbox
WHERE tenant_id = $1 AND conversation_id = $2
`, cfg.tenantID, cfg.conversationID); err != nil {
		return fmt.Errorf("query delivery outbox total: %w", err)
	}
	if err := assign(&result.DeliveryOutboxPending, `
SELECT COUNT(*) FROM delivery_outbox
WHERE tenant_id = $1 AND conversation_id = $2 AND status = 'PENDING'
`, cfg.tenantID, cfg.conversationID); err != nil {
		return fmt.Errorf("query delivery outbox pending: %w", err)
	}
	if err := assign(&result.DeliveryOutboxDLQ, `
SELECT COUNT(*) FROM delivery_outbox
WHERE tenant_id = $1 AND conversation_id = $2 AND status = 'DLQ'
`, cfg.tenantID, cfg.conversationID); err != nil {
		return fmt.Errorf("query delivery outbox dlq: %w", err)
	}
	var cursor int64
	if err := pool.QueryRow(ctx, `
SELECT COALESCE(MAX(last_received_seq), 0)
FROM device_delivery_cursors
WHERE tenant_id = $1 AND conversation_id = $2 AND user_id = $3 AND device_id = $4
`, cfg.tenantID, cfg.conversationID, cfg.userID, cfg.deviceID).Scan(&cursor); err != nil {
		return fmt.Errorf("query cursor: %w", err)
	}
	result.CursorLastReceivedSeq = &cursor
	var checkpoint int64
	checkpointQuery := `
SELECT COALESCE(MAX(offset_value), 0)
FROM delivery_kafka_checkpoints
WHERE topic = 'conversation.timeline.events'
`
	checkpointArgs := []any{}
	if cfg.consumerGroup != "" {
		checkpointQuery = `
SELECT COALESCE(MAX(offset_value), 0)
FROM delivery_kafka_checkpoints
WHERE consumer_group = $1 AND topic = 'conversation.timeline.events'
`
		checkpointArgs = append(checkpointArgs, cfg.consumerGroup)
	}
	if err := pool.QueryRow(ctx, checkpointQuery, checkpointArgs...).Scan(&checkpoint); err != nil {
		return fmt.Errorf("query checkpoint: %w", err)
	}
	result.CheckpointOffsetValue = &checkpoint
	return nil
}

func summarizeLatencies(values []float64) (float64, float64, float64) {
	if len(values) == 0 {
		return 0, 0, 0
	}
	copied := append([]float64(nil), values...)
	sort.Float64s(copied)
	var total float64
	for _, value := range copied {
		total += value
	}
	return total / float64(len(copied)), percentile(copied, 0.95), percentile(copied, 0.99)
}

func buildCapacitySummary(result *summary) *capacitySummary {
	if result == nil || result.StartedAt.IsZero() || result.FinishedAt.IsZero() {
		return nil
	}
	duration := result.FinishedAt.Sub(result.StartedAt)
	if duration <= 0 {
		return nil
	}
	durationSeconds := duration.Seconds()
	return &capacitySummary{
		DurationMS:            float64(duration.Microseconds()) / 1000,
		PollCount:             result.PollCount,
		ItemCount:             result.ItemCount,
		ExpectedCount:         result.ExpectedCount,
		PullsPerSecond:        ratePerSecond(int64(result.PollCount), durationSeconds),
		ItemsPerSecond:        ratePerSecond(int64(result.ItemCount), durationSeconds),
		PullP95MS:             result.PullP95MS,
		PullP99MS:             result.PullP99MS,
		AckEnabled:            result.AckEnabled,
		AckLatencyMS:          result.AckLatencyMS,
		InboxCount:            result.InboxCount,
		DeliveryOutboxTotal:   result.DeliveryOutboxTotal,
		DeliveryOutboxPending: result.DeliveryOutboxPending,
		DeliveryOutboxDLQ:     result.DeliveryOutboxDLQ,
		CheckpointOffsetValue: result.CheckpointOffsetValue,
	}
}

func ratePerSecond(count int64, durationSeconds float64) float64 {
	if count <= 0 || durationSeconds <= 0 {
		return 0
	}
	return float64(count) / durationSeconds
}

func percentile(sorted []float64, quantile float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	index := int(math.Ceil(float64(len(sorted))*quantile)) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

func shortCommit() string {
	value := fullCommit()
	if len(value) > 7 {
		return value[:7]
	}
	return value
}

func fullCommit() string {
	out, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func gitDirty() bool {
	return strings.TrimSpace(gitStatusShort()) != ""
}

func gitStatusShort() string {
	out, err := exec.Command("git", "status", "--short").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func envBool(fallback bool, names ...string) bool {
	for _, name := range names {
		value := strings.TrimSpace(os.Getenv(name))
		if value == "" {
			continue
		}
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return fallback
		}
		return parsed
	}
	return fallback
}
