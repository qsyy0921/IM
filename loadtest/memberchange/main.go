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
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	conversationv1 "github.com/qsyy0921/IM/api/proto/nexusim/conversation/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type config struct {
	target          string
	vus             int
	duration        time.Duration
	requestTimeout  time.Duration
	resultDir       string
	tenantID        string
	conversationID  string
	operatorUserID  string
	targetPrefix    string
	pgDSN           string
	statsWait       time.Duration
	expectedVersion int64
}

type summary struct {
	Commit                 string            `json:"commit"`
	CommitFull             string            `json:"commit_full"`
	GitDirty               bool              `json:"git_dirty"`
	GitStatusShort         string            `json:"git_status_short,omitempty"`
	Target                 string            `json:"target"`
	VUs                    int               `json:"vus"`
	Duration               string            `json:"duration"`
	RequestCount           int64             `json:"request_count"`
	SuccessCount           int64             `json:"success_count"`
	ErrorCount             int64             `json:"error_count"`
	SuccessRate            float64           `json:"success_rate"`
	RPS                    float64           `json:"rps"`
	AvgMS                  float64           `json:"avg_ms"`
	P95MS                  float64           `json:"p95_ms"`
	P99MS                  float64           `json:"p99_ms"`
	ErrorTopN              []errorCount      `json:"error_topn,omitempty"`
	TenantID               string            `json:"tenant_id"`
	ConversationID         string            `json:"conversation_id"`
	SagaCount              *int64            `json:"saga_count,omitempty"`
	SagaDoneCount          *int64            `json:"saga_done_count,omitempty"`
	TimelineCount          *int64            `json:"timeline_count,omitempty"`
	OutboxTotalCount       *int64            `json:"outbox_total_count,omitempty"`
	OutboxPendingCount     *int64            `json:"outbox_pending_count,omitempty"`
	OutboxPublishedCount   *int64            `json:"outbox_published_count,omitempty"`
	OutboxDLQCount         *int64            `json:"outbox_dlq_count,omitempty"`
	ConversationSeqCurrent *int64            `json:"conversation_seq_current,omitempty"`
	SampleChangeID         string            `json:"sample_change_id,omitempty"`
	SampleGetStatus        string            `json:"sample_get_status,omitempty"`
	SampleGetError         string            `json:"sample_get_error,omitempty"`
	StartedAt              time.Time         `json:"started_at"`
	FinishedAt             time.Time         `json:"finished_at"`
	Stats                  map[string]string `json:"stats,omitempty"`
}

type errorCount struct {
	Error string `json:"error"`
	Count int64  `json:"count"`
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
	flag.StringVar(&cfg.target, "target", "127.0.0.1:10496", "conversation-service gRPC target")
	flag.IntVar(&cfg.vus, "vus", 1, "concurrent workers")
	flag.DurationVar(&cfg.duration, "duration", 3*time.Second, "test duration")
	flag.DurationVar(&cfg.requestTimeout, "request-timeout", 2*time.Second, "per-request timeout")
	flag.StringVar(&cfg.resultDir, "result-dir", "loadtest/results/memberchange-smoke", "result directory")
	flag.StringVar(&cfg.tenantID, "tenant-id", "tenant-member-smoke", "tenant id")
	flag.StringVar(&cfg.conversationID, "conversation-id", "conv-member-smoke", "conversation id")
	flag.StringVar(&cfg.operatorUserID, "operator-user-id", "owner-1", "operator user id")
	flag.StringVar(&cfg.targetPrefix, "target-prefix", "target-user", "target user prefix")
	flag.StringVar(&cfg.pgDSN, "pg-dsn", "", "optional PostgreSQL DSN for post-run stats")
	flag.DurationVar(&cfg.statsWait, "stats-wait", 0, "wait before querying PostgreSQL stats")
	flag.Int64Var(&cfg.expectedVersion, "expected-member-version", 0, "expected member version, 0 disables optimistic check")
	flag.Parse()
	if cfg.vus <= 0 {
		cfg.vus = 1
	}
	if cfg.duration <= 0 {
		cfg.duration = time.Second
	}
	if cfg.requestTimeout <= 0 {
		cfg.requestTimeout = 2 * time.Second
	}
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
	client := conversationv1.NewConversationServiceClient(conn)

	startedAt := time.Now().UTC()
	ctx, cancel := context.WithTimeout(context.Background(), cfg.duration)
	defer cancel()

	var sequence int64
	var successCount int64
	var errorCountTotal int64
	var sampleChangeID atomic.Value
	var latencyMu sync.Mutex
	latencies := make([]float64, 0, 1024)
	errorCounts := make(map[string]int64)
	var errorMu sync.Mutex

	var wg sync.WaitGroup
	for vu := 0; vu < cfg.vus; vu++ {
		wg.Add(1)
		go func(vu int) {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
				}
				seq := atomic.AddInt64(&sequence, 1)
				targetUserID := fmt.Sprintf("%s-%d", cfg.targetPrefix, seq)
				requestCtx, requestCancel := context.WithTimeout(context.Background(), cfg.requestTimeout)
				begin := time.Now()
				response, err := client.CreateMemberChange(requestCtx, &conversationv1.CreateMemberChangeRequest{
					AuthContext: &conversationv1.AuthContext{
						TenantId:  cfg.tenantID,
						UserId:    cfg.operatorUserID,
						DeviceId:  fmt.Sprintf("vu-%d", vu),
						SessionId: fmt.Sprintf("session-%d", vu),
						TraceId:   fmt.Sprintf("trace-%d", seq),
						RequestId: fmt.Sprintf("memberchange-%d", seq),
					},
					ConversationId:        cfg.conversationID,
					TargetUserId:          targetUserID,
					ChangeType:            conversationv1.MemberChangeType_MEMBER_CHANGE_TYPE_JOIN,
					TargetRole:            conversationv1.MemberRole_MEMBER_ROLE_MEMBER,
					ExpectedMemberVersion: cfg.expectedVersion,
					IdempotencyKey:        fmt.Sprintf("idem-%d", seq),
					ConflictPolicy:        conversationv1.MemberChangeConflictPolicy_MEMBER_CHANGE_CONFLICT_POLICY_REJECT,
					Reason:                "smoke join",
				})
				elapsedMS := float64(time.Since(begin).Microseconds()) / 1000
				requestCancel()
				latencyMu.Lock()
				latencies = append(latencies, elapsedMS)
				latencyMu.Unlock()
				if err != nil {
					atomic.AddInt64(&errorCountTotal, 1)
					errorMu.Lock()
					errorCounts[err.Error()]++
					errorMu.Unlock()
					continue
				}
				if response.GetChangeId() != "" && sampleChangeID.Load() == nil {
					sampleChangeID.Store(response.GetChangeId())
				}
				atomic.AddInt64(&successCount, 1)
			}
		}(vu)
	}
	wg.Wait()
	finishedAt := time.Now().UTC()

	if cfg.statsWait > 0 {
		time.Sleep(cfg.statsWait)
	}
	result := summary{
		Commit:         shortCommit(),
		CommitFull:     fullCommit(),
		GitDirty:       gitDirty(),
		GitStatusShort: gitStatusShort(),
		Target:         cfg.target,
		VUs:            cfg.vus,
		Duration:       cfg.duration.String(),
		RequestCount:   atomic.LoadInt64(&sequence),
		SuccessCount:   atomic.LoadInt64(&successCount),
		ErrorCount:     atomic.LoadInt64(&errorCountTotal),
		TenantID:       cfg.tenantID,
		ConversationID: cfg.conversationID,
		StartedAt:      startedAt,
		FinishedAt:     finishedAt,
	}
	if result.RequestCount > 0 {
		result.SuccessRate = float64(result.SuccessCount) / float64(result.RequestCount)
		result.RPS = float64(result.RequestCount) / cfg.duration.Seconds()
	}
	result.AvgMS, result.P95MS, result.P99MS = summarizeLatencies(latencies)
	result.ErrorTopN = topErrors(errorCounts, 5)
	if cfg.pgDSN != "" {
		if err := fillPostgresStats(context.Background(), cfg, &result); err != nil {
			if result.Stats == nil {
				result.Stats = make(map[string]string)
			}
			result.Stats["postgres_error"] = err.Error()
		}
	}
	if value := sampleChangeID.Load(); value != nil {
		result.SampleChangeID = value.(string)
	}
	if result.SampleChangeID != "" {
		status, err := getMemberChangeStatus(context.Background(), client, cfg, result.SampleChangeID)
		if err != nil {
			result.SampleGetError = err.Error()
		} else {
			result.SampleGetStatus = status
		}
	}
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("encode summary: %w", err)
	}
	path := filepath.Join(cfg.resultDir, "memberchange-summary.json")
	if err := os.WriteFile(path, encoded, 0o644); err != nil {
		return fmt.Errorf("write summary: %w", err)
	}
	fmt.Println(string(encoded))
	fmt.Printf("summary: %s\n", path)
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

func topErrors(counts map[string]int64, limit int) []errorCount {
	result := make([]errorCount, 0, len(counts))
	for key, count := range counts {
		result = append(result, errorCount{Error: key, Count: count})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Count == result[j].Count {
			return result[i].Error < result[j].Error
		}
		return result[i].Count > result[j].Count
	})
	if len(result) > limit {
		result = result[:limit]
	}
	return result
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
	if err := assign(&result.SagaCount, `
SELECT COUNT(*) FROM member_change_saga WHERE tenant_id = $1 AND conversation_id = $2
`, cfg.tenantID, cfg.conversationID); err != nil {
		return fmt.Errorf("query saga count: %w", err)
	}
	if err := assign(&result.SagaDoneCount, `
SELECT COUNT(*) FROM member_change_saga WHERE tenant_id = $1 AND conversation_id = $2 AND status = 'DONE'
`, cfg.tenantID, cfg.conversationID); err != nil {
		return fmt.Errorf("query saga done count: %w", err)
	}
	if err := assign(&result.TimelineCount, `
SELECT COUNT(*) FROM conversation_timeline_events WHERE tenant_id = $1 AND conversation_id = $2
`, cfg.tenantID, cfg.conversationID); err != nil {
		return fmt.Errorf("query timeline count: %w", err)
	}
	if err := assign(&result.OutboxTotalCount, `
SELECT COUNT(*) FROM message_outbox WHERE tenant_id = $1 AND conversation_id = $2
`, cfg.tenantID, cfg.conversationID); err != nil {
		return fmt.Errorf("query outbox total: %w", err)
	}
	if err := assign(&result.OutboxPendingCount, `
SELECT COUNT(*) FROM message_outbox WHERE tenant_id = $1 AND conversation_id = $2 AND status = 'PENDING'
`, cfg.tenantID, cfg.conversationID); err != nil {
		return fmt.Errorf("query outbox pending: %w", err)
	}
	if err := assign(&result.OutboxPublishedCount, `
SELECT COUNT(*) FROM message_outbox WHERE tenant_id = $1 AND conversation_id = $2 AND status = 'PUBLISHED'
`, cfg.tenantID, cfg.conversationID); err != nil {
		return fmt.Errorf("query outbox published: %w", err)
	}
	if err := assign(&result.OutboxDLQCount, `
SELECT COUNT(*) FROM message_outbox WHERE tenant_id = $1 AND conversation_id = $2 AND status = 'DLQ'
`, cfg.tenantID, cfg.conversationID); err != nil {
		return fmt.Errorf("query outbox dlq: %w", err)
	}
	var currentSeq int64
	if err := pool.QueryRow(ctx, `
SELECT COALESCE(current_seq, 0)
FROM conversation_seq
WHERE tenant_id = $1 AND conversation_id = $2
`, cfg.tenantID, cfg.conversationID).Scan(&currentSeq); err != nil {
		result.ConversationSeqCurrent = nil
	} else {
		result.ConversationSeqCurrent = &currentSeq
	}
	return nil
}

func getMemberChangeStatus(
	ctx context.Context,
	client conversationv1.ConversationServiceClient,
	cfg config,
	changeID string,
) (string, error) {
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	response, err := client.GetMemberChange(requestCtx, &conversationv1.GetMemberChangeRequest{
		AuthContext: &conversationv1.AuthContext{
			TenantId: cfg.tenantID,
			UserId:   cfg.operatorUserID,
		},
		ConversationId: cfg.conversationID,
		ChangeId:       changeID,
	})
	if err != nil {
		return "", err
	}
	return response.GetStatus().String(), nil
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
