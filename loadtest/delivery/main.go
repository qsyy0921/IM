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
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	deliveryv1 "github.com/qsyy0921/IM/api/proto/nexusim/delivery/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type config struct {
	target         string
	resultDir      string
	requestTimeout time.Duration
	waitTimeout    time.Duration
	pollInterval   time.Duration
	tenantID       string
	userID         string
	deviceID       string
	conversationID string
	afterSeq       int64
	limit          int32
	expectedCount  int
	pgDSN          string
	ack            bool
}

type summary struct {
	Commit                string       `json:"commit"`
	CommitFull            string       `json:"commit_full"`
	GitDirty              bool         `json:"git_dirty"`
	GitStatusShort        string       `json:"git_status_short,omitempty"`
	Target                string       `json:"target"`
	TenantID              string       `json:"tenant_id"`
	UserID                string       `json:"user_id"`
	DeviceID              string       `json:"device_id"`
	ConversationID        string       `json:"conversation_id"`
	AfterSeq              int64        `json:"after_seq"`
	Limit                 int32        `json:"limit"`
	ExpectedCount         int          `json:"expected_count"`
	PollCount             int          `json:"poll_count"`
	ItemCount             int          `json:"item_count"`
	MaxSeq                int64        `json:"max_seq"`
	HasMore               bool         `json:"has_more"`
	AckEnabled            bool         `json:"ack_enabled"`
	AckLastReceivedSeq    int64        `json:"ack_last_received_seq,omitempty"`
	Success               bool         `json:"success"`
	Error                 string       `json:"error,omitempty"`
	WaitTimeout           string       `json:"wait_timeout"`
	PollInterval          string       `json:"poll_interval"`
	PullAvgMS             float64      `json:"pull_avg_ms"`
	PullP95MS             float64      `json:"pull_p95_ms"`
	PullP99MS             float64      `json:"pull_p99_ms"`
	AckLatencyMS          float64      `json:"ack_latency_ms,omitempty"`
	InboxCount            *int64       `json:"inbox_count,omitempty"`
	DeliveryOutboxTotal   *int64       `json:"delivery_outbox_total,omitempty"`
	DeliveryOutboxPending *int64       `json:"delivery_outbox_pending,omitempty"`
	DeliveryOutboxDLQ     *int64       `json:"delivery_outbox_dlq,omitempty"`
	CursorLastReceivedSeq *int64       `json:"cursor_last_received_seq,omitempty"`
	CheckpointOffsetValue *int64       `json:"checkpoint_offset_value,omitempty"`
	StartedAt             time.Time    `json:"started_at"`
	FinishedAt            time.Time    `json:"finished_at"`
	Items                 []pulledItem `json:"items,omitempty"`
}

type pulledItem struct {
	ConversationSeq int64  `json:"conversation_seq"`
	EventID         string `json:"event_id"`
	EventType       string `json:"event_type"`
	MessageID       string `json:"message_id"`
	SenderID        string `json:"sender_id"`
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
	flag.StringVar(&cfg.resultDir, "result-dir", "loadtest/results/delivery-smoke", "result directory")
	flag.DurationVar(&cfg.requestTimeout, "request-timeout", 2*time.Second, "per-request timeout")
	flag.DurationVar(&cfg.waitTimeout, "wait-timeout", 10*time.Second, "max wait for expected inbox items")
	flag.DurationVar(&cfg.pollInterval, "poll-interval", 200*time.Millisecond, "pull retry interval while waiting")
	flag.StringVar(&cfg.tenantID, "tenant-id", "tenant-delivery-smoke", "tenant id")
	flag.StringVar(&cfg.userID, "user-id", "delivery-user-1", "inbox owner user id")
	flag.StringVar(&cfg.deviceID, "device-id", "delivery-device-1", "ack device id")
	flag.StringVar(&cfg.conversationID, "conversation-id", "conv-delivery-smoke", "conversation id")
	flag.Int64Var(&cfg.afterSeq, "after-seq", 0, "PullInbox after_seq")
	flag.IntVar(&limit, "limit", 100, "PullInbox limit")
	flag.IntVar(&cfg.expectedCount, "expected-count", 1, "minimum inbox items expected before success")
	flag.StringVar(&cfg.pgDSN, "pg-dsn", "", "optional PostgreSQL DSN for stats")
	flag.BoolVar(&cfg.ack, "ack", true, "ack max pulled conversation seq")
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
	if limit <= 0 {
		limit = 100
	}
	cfg.limit = int32(limit)
	return cfg
}

func run(cfg config) error {
	if err := os.MkdirAll(cfg.resultDir, 0o755); err != nil {
		return fmt.Errorf("create result dir: %w", err)
	}
	conn, err := grpc.NewClient(cfg.target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("dial target: %w", err)
	}
	defer conn.Close()
	client := deliveryv1.NewDeliveryServiceClient(conn)

	result := summary{
		Commit:         shortCommit(),
		CommitFull:     fullCommit(),
		GitDirty:       gitDirty(),
		GitStatusShort: gitStatusShort(),
		Target:         cfg.target,
		TenantID:       cfg.tenantID,
		UserID:         cfg.userID,
		DeviceID:       cfg.deviceID,
		ConversationID: cfg.conversationID,
		AfterSeq:       cfg.afterSeq,
		Limit:          cfg.limit,
		ExpectedCount:  cfg.expectedCount,
		AckEnabled:     cfg.ack,
		WaitTimeout:    cfg.waitTimeout.String(),
		PollInterval:   cfg.pollInterval.String(),
		StartedAt:      time.Now().UTC(),
	}

	deadline := time.Now().Add(cfg.waitTimeout)
	latencies := make([]float64, 0, 16)
	var lastResponse *deliveryv1.PullInboxResponse
	for {
		pullCtx, cancel := context.WithTimeout(context.Background(), cfg.requestTimeout)
		begin := time.Now()
		response, err := client.PullInbox(pullCtx, &deliveryv1.PullInboxRequest{
			AuthContext: &deliveryv1.AuthContext{
				TenantId:  cfg.tenantID,
				UserId:    cfg.userID,
				DeviceId:  cfg.deviceID,
				SessionId: "delivery-smoke",
				TraceId:   fmt.Sprintf("delivery-pull-%d", result.PollCount+1),
				RequestId: fmt.Sprintf("delivery-pull-%d", result.PollCount+1),
			},
			ConversationId: cfg.conversationID,
			AfterSeq:       cfg.afterSeq,
			Limit:          cfg.limit,
		})
		elapsedMS := float64(time.Since(begin).Microseconds()) / 1000
		cancel()
		result.PollCount++
		latencies = append(latencies, elapsedMS)
		if err != nil {
			result.Error = err.Error()
			break
		}
		lastResponse = response
		if len(response.GetItems()) >= cfg.expectedCount || time.Now().After(deadline) {
			break
		}
		time.Sleep(cfg.pollInterval)
	}
	if lastResponse != nil {
		result.ItemCount = len(lastResponse.GetItems())
		result.HasMore = lastResponse.GetHasMore()
		for _, item := range lastResponse.GetItems() {
			if item.GetConversationSeq() > result.MaxSeq {
				result.MaxSeq = item.GetConversationSeq()
			}
			result.Items = append(result.Items, pulledItem{
				ConversationSeq: item.GetConversationSeq(),
				EventID:         item.GetEventId(),
				EventType:       item.GetEventType(),
				MessageID:       item.GetMessageId(),
				SenderID:        item.GetSenderId(),
			})
		}
	}
	result.PullAvgMS, result.PullP95MS, result.PullP99MS = summarizeLatencies(latencies)
	result.Success = result.Error == "" && result.ItemCount >= cfg.expectedCount

	if result.Success && cfg.ack && result.MaxSeq > 0 {
		ackCtx, cancel := context.WithTimeout(context.Background(), cfg.requestTimeout)
		begin := time.Now()
		response, err := client.AckDelivery(ackCtx, &deliveryv1.AckDeliveryRequest{
			AuthContext: &deliveryv1.AuthContext{
				TenantId:  cfg.tenantID,
				UserId:    cfg.userID,
				DeviceId:  cfg.deviceID,
				SessionId: "delivery-smoke",
				TraceId:   "delivery-ack",
				RequestId: "delivery-ack",
			},
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
	if err := pool.QueryRow(ctx, `
SELECT COALESCE(MAX(offset_value), 0)
FROM delivery_kafka_checkpoints
WHERE topic = 'conversation.timeline.events'
`).Scan(&checkpoint); err != nil {
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
