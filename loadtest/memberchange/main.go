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
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	conversationv1 "github.com/qsyy0921/IM/api/proto/nexusim/conversation/v1"
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
	target            string
	tls               grpctls.Config
	vus               int
	duration          time.Duration
	requestTimeout    time.Duration
	resultDir         string
	tenantID          string
	conversationID    string
	operatorUserID    string
	listUserID        string
	targetPrefix      string
	targetUserID      string
	changeType        string
	targetRole        string
	idempotencyPrefix string
	requestCount      int64
	pgDSN             string
	statsWait         time.Duration
	expectedVersion   int64
	verifiedMetadata  bool
}

type summary struct {
	Commit                            string            `json:"commit"`
	CommitFull                        string            `json:"commit_full"`
	GitDirty                          bool              `json:"git_dirty"`
	GitStatusShort                    string            `json:"git_status_short,omitempty"`
	Target                            string            `json:"target"`
	TLSEnabled                        bool              `json:"tls_enabled"`
	VerifiedAuthMetadata              bool              `json:"verified_auth_metadata"`
	VUs                               int               `json:"vus"`
	Duration                          string            `json:"duration"`
	RequestCount                      int64             `json:"request_count"`
	SuccessCount                      int64             `json:"success_count"`
	ErrorCount                        int64             `json:"error_count"`
	SuccessRate                       float64           `json:"success_rate"`
	RPS                               float64           `json:"rps"`
	AvgMS                             float64           `json:"avg_ms"`
	P95MS                             float64           `json:"p95_ms"`
	P99MS                             float64           `json:"p99_ms"`
	ErrorTopN                         []errorCount      `json:"error_topn,omitempty"`
	TenantID                          string            `json:"tenant_id"`
	ConversationID                    string            `json:"conversation_id"`
	OperatorUserID                    string            `json:"operator_user_id"`
	ListUserID                        string            `json:"list_user_id"`
	ChangeType                        string            `json:"change_type"`
	TargetRole                        string            `json:"target_role,omitempty"`
	TargetUserID                      string            `json:"target_user_id,omitempty"`
	SagaCount                         *int64            `json:"saga_count,omitempty"`
	SagaDoneCount                     *int64            `json:"saga_done_count,omitempty"`
	TimelineCount                     *int64            `json:"timeline_count,omitempty"`
	OutboxTotalCount                  *int64            `json:"outbox_total_count,omitempty"`
	OutboxPendingCount                *int64            `json:"outbox_pending_count,omitempty"`
	OutboxPublishedCount              *int64            `json:"outbox_published_count,omitempty"`
	OutboxDLQCount                    *int64            `json:"outbox_dlq_count,omitempty"`
	ConversationSeqCurrent            *int64            `json:"conversation_seq_current,omitempty"`
	SampleChangeID                    string            `json:"sample_change_id,omitempty"`
	SampleGetStatus                   string            `json:"sample_get_status,omitempty"`
	SampleGetError                    string            `json:"sample_get_error,omitempty"`
	MemberListCount                   *int64            `json:"member_list_count,omitempty"`
	MemberListNextPage                string            `json:"member_list_next_page_token,omitempty"`
	MemberListSampleUsers             []string          `json:"member_list_sample_users,omitempty"`
	MemberListTargetPresent           *bool             `json:"member_list_target_present,omitempty"`
	MemberListTargetAbsentVerified    *bool             `json:"member_list_target_absent_verified,omitempty"`
	MemberListTargetRole              string            `json:"member_list_target_role,omitempty"`
	MemberListTargetStatus            string            `json:"member_list_target_status,omitempty"`
	MemberListTargetMemberVersion     int64             `json:"member_list_target_member_version,omitempty"`
	MemberListTargetPermissionVersion int64             `json:"member_list_target_permission_version,omitempty"`
	MemberListError                   string            `json:"member_list_error,omitempty"`
	OwnerTransferPreviousOwnerUserID  string            `json:"owner_transfer_previous_owner_user_id,omitempty"`
	OwnerTransferNewOwnerUserID       string            `json:"owner_transfer_new_owner_user_id,omitempty"`
	OwnerTransferPreviousOwnerRole    string            `json:"owner_transfer_previous_owner_role,omitempty"`
	OwnerTransferPreviousOwnerStatus  string            `json:"owner_transfer_previous_owner_status,omitempty"`
	OwnerTransferNewOwnerRole         string            `json:"owner_transfer_new_owner_role,omitempty"`
	OwnerTransferNewOwnerStatus       string            `json:"owner_transfer_new_owner_status,omitempty"`
	OwnerTransferOwnerCount           *int64            `json:"owner_transfer_owner_count,omitempty"`
	StartedAt                         time.Time         `json:"started_at"`
	FinishedAt                        time.Time         `json:"finished_at"`
	Stats                             map[string]string `json:"stats,omitempty"`
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
	registerTLSFlags("conversation-tls", "NEXUSIM_CONVERSATION_TLS", "conversation-service", &cfg.tls)
	flag.IntVar(&cfg.vus, "vus", 1, "concurrent workers")
	flag.DurationVar(&cfg.duration, "duration", 3*time.Second, "test duration")
	flag.DurationVar(&cfg.requestTimeout, "request-timeout", 2*time.Second, "per-request timeout")
	flag.StringVar(&cfg.resultDir, "result-dir", "loadtest/results/memberchange-smoke", "result directory")
	flag.StringVar(&cfg.tenantID, "tenant-id", "tenant-member-smoke", "tenant id")
	flag.StringVar(&cfg.conversationID, "conversation-id", "conv-member-smoke", "conversation id")
	flag.StringVar(&cfg.operatorUserID, "operator-user-id", "owner-1", "operator user id")
	flag.StringVar(&cfg.listUserID, "list-user-id", "", "user id used for post-run GetMemberChange/ListConversationMembers; defaults to operator-user-id")
	flag.StringVar(&cfg.targetPrefix, "target-prefix", "target-user", "target user prefix")
	flag.StringVar(&cfg.targetUserID, "target-user-id", "", "fixed target user id; when set, use with --request-count 1 for deterministic smoke")
	flag.StringVar(&cfg.changeType, "change-type", "join", "member change type: join, leave, remove, role-changed, or owner-transfer")
	flag.StringVar(&cfg.targetRole, "target-role", "member", "target role for join/role-changed: owner, admin, or member")
	flag.StringVar(&cfg.idempotencyPrefix, "idempotency-prefix", "idem", "idempotency key prefix")
	flag.Int64Var(&cfg.requestCount, "request-count", 0, "fixed request count; 0 means run until duration elapses")
	flag.StringVar(&cfg.pgDSN, "pg-dsn", "", "optional PostgreSQL DSN for post-run stats")
	flag.DurationVar(&cfg.statsWait, "stats-wait", 0, "wait before querying PostgreSQL stats")
	flag.Int64Var(&cfg.expectedVersion, "expected-member-version", 0, "expected member version, 0 disables optimistic check")
	flag.BoolVar(&cfg.verifiedMetadata, "verified-auth-metadata", envBool(false, "NEXUSIM_MEMBERCHANGE_VERIFIED_AUTH_METADATA", "NEXUSIM_CONVERSATION_LOADTEST_VERIFIED_AUTH_METADATA"), "send gateway verified identity through conversation-service gRPC metadata")
	flag.Parse()
	return normalizeConfigDefaults(cfg)
}

func registerTLSFlags(prefix string, envPrefix string, serviceName string, config *grpctls.Config) {
	flag.StringVar(&config.CAFile, prefix+"-ca-file", os.Getenv(envPrefix+"_CA_FILE"), "CA PEM for "+serviceName+" gRPC TLS")
	flag.StringVar(&config.ServerName, prefix+"-server-name", os.Getenv(envPrefix+"_SERVER_NAME"), "override server name for "+serviceName+" gRPC TLS")
	flag.StringVar(&config.ClientCertFile, prefix+"-client-cert-file", os.Getenv(envPrefix+"_CLIENT_CERT_FILE"), "client certificate PEM for "+serviceName+" gRPC mTLS")
	flag.StringVar(&config.ClientKeyFile, prefix+"-client-key-file", os.Getenv(envPrefix+"_CLIENT_KEY_FILE"), "client private key PEM for "+serviceName+" gRPC mTLS")
}

func normalizeConfigDefaults(cfg config) config {
	if cfg.vus <= 0 {
		cfg.vus = 1
	}
	if cfg.duration <= 0 {
		cfg.duration = time.Second
	}
	if cfg.requestTimeout <= 0 {
		cfg.requestTimeout = 2 * time.Second
	}
	if cfg.listUserID == "" {
		cfg.listUserID = cfg.operatorUserID
	}
	return cfg
}

type verifiedAuthIdentity struct {
	tenantID  string
	userID    string
	deviceID  string
	sessionID string
	traceID   string
	requestID string
}

func withVerifiedAuthMetadata(ctx context.Context, cfg config, auth verifiedAuthIdentity) context.Context {
	if !cfg.verifiedMetadata {
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

func conversationAuth(auth verifiedAuthIdentity) *conversationv1.AuthContext {
	return &conversationv1.AuthContext{
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
	ownerTransfer := isOwnerTransferChange(cfg.changeType)
	var changeType conversationv1.MemberChangeType
	var changeTypeName string
	if ownerTransfer {
		changeTypeName = "OWNER_TRANSFER"
	} else {
		var err error
		changeType, changeTypeName, err = parseMemberChangeType(cfg.changeType)
		if err != nil {
			return err
		}
	}
	targetRole, targetRoleName, err := parseMemberRole(cfg.targetRole)
	if err != nil {
		return err
	}
	dialOption, err := grpctls.DialOption(cfg.tls, "conversation-tls")
	if err != nil {
		return fmt.Errorf("configure conversation-service TLS: %w", err)
	}
	conn, err := grpc.NewClient(cfg.target, dialOption)
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
	var samplePreviousOwnerUserID atomic.Value
	var sampleNewOwnerUserID atomic.Value
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
				seq, ok := nextSequence(&sequence, cfg.requestCount)
				if !ok {
					return
				}
				targetUserID := cfg.targetUserID
				if targetUserID == "" {
					targetUserID = fmt.Sprintf("%s-%d", cfg.targetPrefix, seq)
				}
				requestCtx, requestCancel := context.WithTimeout(context.Background(), cfg.requestTimeout)
				begin := time.Now()
				auth := verifiedAuthIdentity{
					tenantID:  cfg.tenantID,
					userID:    cfg.operatorUserID,
					deviceID:  fmt.Sprintf("vu-%d", vu),
					sessionID: fmt.Sprintf("session-%d", vu),
					traceID:   fmt.Sprintf("trace-%d", seq),
					requestID: fmt.Sprintf("memberchange-%d", seq),
				}
				requestCtx = withVerifiedAuthMetadata(requestCtx, cfg, auth)
				var changeID string
				var previousOwnerUserID string
				var newOwnerUserID string
				var err error
				if ownerTransfer {
					var response *conversationv1.TransferConversationOwnerResponse
					response, err = client.TransferConversationOwner(requestCtx, &conversationv1.TransferConversationOwnerRequest{
						AuthContext:           conversationAuth(auth),
						ConversationId:        cfg.conversationID,
						NewOwnerUserId:        targetUserID,
						ExpectedMemberVersion: cfg.expectedVersion,
						IdempotencyKey:        fmt.Sprintf("%s-%d", cfg.idempotencyPrefix, seq),
						Reason:                "smoke owner transfer",
					})
					if response != nil {
						changeID = response.GetChangeId()
						previousOwnerUserID = response.GetPreviousOwnerUserId()
						newOwnerUserID = response.GetNewOwnerUserId()
					}
				} else {
					var response *conversationv1.CreateMemberChangeResponse
					response, err = client.CreateMemberChange(requestCtx, &conversationv1.CreateMemberChangeRequest{
						AuthContext:           conversationAuth(auth),
						ConversationId:        cfg.conversationID,
						TargetUserId:          targetUserID,
						ChangeType:            changeType,
						TargetRole:            targetRole,
						ExpectedMemberVersion: cfg.expectedVersion,
						IdempotencyKey:        fmt.Sprintf("%s-%d", cfg.idempotencyPrefix, seq),
						ConflictPolicy:        conversationv1.MemberChangeConflictPolicy_MEMBER_CHANGE_CONFLICT_POLICY_REJECT,
						Reason:                fmt.Sprintf("smoke %s", strings.ToLower(changeTypeName)),
					})
					if response != nil {
						changeID = response.GetChangeId()
					}
				}
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
				if changeID != "" && sampleChangeID.Load() == nil {
					sampleChangeID.Store(changeID)
				}
				if previousOwnerUserID != "" && samplePreviousOwnerUserID.Load() == nil {
					samplePreviousOwnerUserID.Store(previousOwnerUserID)
				}
				if newOwnerUserID != "" && sampleNewOwnerUserID.Load() == nil {
					sampleNewOwnerUserID.Store(newOwnerUserID)
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
		Commit:               shortCommit(),
		CommitFull:           fullCommit(),
		GitDirty:             gitDirty(),
		GitStatusShort:       gitStatusShort(),
		Target:               cfg.target,
		TLSEnabled:           cfg.tls.Enabled(),
		VerifiedAuthMetadata: cfg.verifiedMetadata,
		VUs:                  cfg.vus,
		Duration:             cfg.duration.String(),
		RequestCount:         atomic.LoadInt64(&sequence),
		SuccessCount:         atomic.LoadInt64(&successCount),
		ErrorCount:           atomic.LoadInt64(&errorCountTotal),
		TenantID:             cfg.tenantID,
		ConversationID:       cfg.conversationID,
		OperatorUserID:       cfg.operatorUserID,
		ListUserID:           cfg.listUserID,
		ChangeType:           changeTypeName,
		TargetRole:           targetRoleName,
		TargetUserID:         cfg.targetUserID,
		StartedAt:            startedAt,
		FinishedAt:           finishedAt,
	}
	if ownerTransfer {
		result.TargetRole = ""
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
	if value := samplePreviousOwnerUserID.Load(); value != nil {
		result.OwnerTransferPreviousOwnerUserID = value.(string)
	}
	if value := sampleNewOwnerUserID.Load(); value != nil {
		result.OwnerTransferNewOwnerUserID = value.(string)
	}
	if result.SampleChangeID != "" {
		status, err := getMemberChangeStatus(context.Background(), client, cfg, result.SampleChangeID)
		if err != nil {
			result.SampleGetError = err.Error()
		} else {
			result.SampleGetStatus = status
		}
	}
	if err := fillMemberListSample(context.Background(), client, cfg, &result); err != nil {
		result.MemberListError = err.Error()
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

func parseMemberChangeType(value string) (conversationv1.MemberChangeType, string, error) {
	normalized := normalizeEnumName(value)
	switch normalized {
	case "JOIN":
		return conversationv1.MemberChangeType_MEMBER_CHANGE_TYPE_JOIN, "JOIN", nil
	case "LEAVE":
		return conversationv1.MemberChangeType_MEMBER_CHANGE_TYPE_LEAVE, "LEAVE", nil
	case "REMOVE":
		return conversationv1.MemberChangeType_MEMBER_CHANGE_TYPE_REMOVE, "REMOVE", nil
	case "ROLE_CHANGED", "ROLECHANGE", "ROLECHANGED":
		return conversationv1.MemberChangeType_MEMBER_CHANGE_TYPE_ROLE_CHANGED, "ROLE_CHANGED", nil
	default:
		return conversationv1.MemberChangeType_MEMBER_CHANGE_TYPE_UNSPECIFIED, "", fmt.Errorf("unsupported change type %q", value)
	}
}

func isOwnerTransferChange(value string) bool {
	normalized := normalizeEnumName(value)
	return normalized == "OWNER_TRANSFER" || normalized == "OWNERTRANSFER"
}

func parseMemberRole(value string) (conversationv1.MemberRole, string, error) {
	normalized := normalizeEnumName(value)
	switch normalized {
	case "OWNER":
		return conversationv1.MemberRole_MEMBER_ROLE_OWNER, "OWNER", nil
	case "ADMIN":
		return conversationv1.MemberRole_MEMBER_ROLE_ADMIN, "ADMIN", nil
	case "MEMBER":
		return conversationv1.MemberRole_MEMBER_ROLE_MEMBER, "MEMBER", nil
	default:
		return conversationv1.MemberRole_MEMBER_ROLE_UNSPECIFIED, "", fmt.Errorf("unsupported target role %q", value)
	}
}

func normalizeEnumName(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "-", "_")
	value = strings.ReplaceAll(value, " ", "_")
	return strings.ToUpper(value)
}

func nextSequence(counter *int64, max int64) (int64, bool) {
	for {
		current := atomic.LoadInt64(counter)
		if max > 0 && current >= max {
			return 0, false
		}
		next := current + 1
		if atomic.CompareAndSwapInt64(counter, current, next) {
			return next, true
		}
	}
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
	if isOwnerTransferChange(cfg.changeType) {
		if err := assign(&result.OwnerTransferOwnerCount, `
SELECT COUNT(*) FROM conversation_members WHERE tenant_id = $1 AND conversation_id = $2 AND role = 'OWNER' AND status = 'ACTIVE'
`, cfg.tenantID, cfg.conversationID); err != nil {
			return fmt.Errorf("query owner count: %w", err)
		}
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
	auth := verifiedAuthIdentity{
		tenantID:  cfg.tenantID,
		userID:    cfg.listUserID,
		deviceID:  "memberchange-list",
		sessionID: "memberchange-list",
		traceID:   "memberchange-get",
		requestID: "memberchange-get-" + changeID,
	}
	requestCtx = withVerifiedAuthMetadata(requestCtx, cfg, auth)
	response, err := client.GetMemberChange(requestCtx, &conversationv1.GetMemberChangeRequest{
		AuthContext:    conversationAuth(auth),
		ConversationId: cfg.conversationID,
		ChangeId:       changeID,
	})
	if err != nil {
		return "", err
	}
	return response.GetStatus().String(), nil
}

func fillMemberListSample(
	ctx context.Context,
	client conversationv1.ConversationServiceClient,
	cfg config,
	result *summary,
) error {
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	var count int64
	pageToken := ""
	targetPresent := false
	result.MemberListSampleUsers = make([]string, 0, 10)
	for {
		auth := verifiedAuthIdentity{
			tenantID:  cfg.tenantID,
			userID:    cfg.listUserID,
			deviceID:  "memberchange-list",
			sessionID: "memberchange-list",
			traceID:   "memberchange-list",
			requestID: fmt.Sprintf("memberchange-list-%d", count),
		}
		callCtx := withVerifiedAuthMetadata(requestCtx, cfg, auth)
		response, err := client.ListConversationMembers(callCtx, &conversationv1.ListConversationMembersRequest{
			AuthContext:    conversationAuth(auth),
			ConversationId: cfg.conversationID,
			PageSize:       10,
			PageToken:      pageToken,
		})
		if err != nil {
			return err
		}
		for _, member := range response.GetMembers() {
			count++
			if len(result.MemberListSampleUsers) < 10 {
				result.MemberListSampleUsers = append(result.MemberListSampleUsers, member.GetUserId())
			}
			if cfg.targetUserID != "" && member.GetUserId() == cfg.targetUserID {
				targetPresent = true
				result.MemberListTargetRole = member.GetRole().String()
				result.MemberListTargetStatus = member.GetStatus().String()
				result.MemberListTargetMemberVersion = member.GetMemberVersion()
				result.MemberListTargetPermissionVersion = member.GetPermissionVersion()
			}
			if isOwnerTransferChange(cfg.changeType) {
				if member.GetUserId() == cfg.operatorUserID {
					result.OwnerTransferPreviousOwnerRole = member.GetRole().String()
					result.OwnerTransferPreviousOwnerStatus = member.GetStatus().String()
				}
				if cfg.targetUserID != "" && member.GetUserId() == cfg.targetUserID {
					result.OwnerTransferNewOwnerRole = member.GetRole().String()
					result.OwnerTransferNewOwnerStatus = member.GetStatus().String()
				}
			}
		}
		pageToken = response.GetNextPageToken()
		if pageToken == "" {
			break
		}
	}
	result.MemberListCount = &count
	result.MemberListNextPage = pageToken
	if cfg.targetUserID != "" {
		result.MemberListTargetPresent = &targetPresent
		targetAbsentVerified := !targetPresent
		result.MemberListTargetAbsentVerified = &targetAbsentVerified
	}
	return nil
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
