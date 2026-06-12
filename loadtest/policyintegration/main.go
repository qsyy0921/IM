package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	messagev1 "github.com/qsyy0921/IM/api/proto/nexusim/message/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
)

type config struct {
	target                 string
	resultDir              string
	pgDSN                  string
	scenario               string
	requestTimeout         time.Duration
	tenantID               string
	userID                 string
	deviceID               string
	sessionID              string
	conversationID         string
	clientMsgID            string
	expectedPermissionVer  int64
	expectedClassification string
	expectedReason         string
	cleanup                bool
}

type summary struct {
	Commit                 string       `json:"commit"`
	CommitFull             string       `json:"commit_full"`
	GitDirty               bool         `json:"git_dirty"`
	GitStatusShort         string       `json:"git_status_short,omitempty"`
	Target                 string       `json:"target"`
	ResultDir              string       `json:"result_dir"`
	Scenario               string       `json:"scenario"`
	TenantID               string       `json:"tenant_id"`
	UserID                 string       `json:"user_id"`
	ConversationID         string       `json:"conversation_id"`
	StartedAt              time.Time    `json:"started_at"`
	FinishedAt             time.Time    `json:"finished_at"`
	Success                bool         `json:"success"`
	Error                  string       `json:"error,omitempty"`
	ExpectedPermissionVer  int64        `json:"expected_permission_version"`
	ExpectedClassification string       `json:"expected_classification"`
	ExpectedReason         string       `json:"expected_reason,omitempty"`
	SendMessage            sendSummary  `json:"send_message"`
	MessageError           errorSummary `json:"message_error,omitempty"`
	DBBefore               dbStats      `json:"db_before"`
	DBAfter                dbStats      `json:"db_after"`
	MessageRow             messageRow   `json:"message_row,omitempty"`
	LatencyMS              float64      `json:"latency_ms"`
}

type sendSummary struct {
	MessageID        string `json:"message_id,omitempty"`
	ConversationSeq  int64  `json:"conversation_seq,omitempty"`
	IdempotentReplay bool   `json:"idempotent_replay,omitempty"`
	GRPCCode         string `json:"grpc_code"`
}

type errorSummary struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

type dbStats struct {
	MessageLog     int64 `json:"message_log"`
	TimelineEvents int64 `json:"conversation_timeline_events"`
	MessageOutbox  int64 `json:"message_outbox"`
}

type messageRow struct {
	MessageID                 string `json:"message_id"`
	ConversationSeq           int64  `json:"conversation_seq"`
	MessagePermissionVersion  int64  `json:"message_permission_version"`
	MessageClassification     string `json:"message_classification"`
	TimelinePermissionVersion int64  `json:"timeline_permission_version"`
	TimelineClassification    string `json:"timeline_classification"`
	FanoutPolicyVersion       int64  `json:"fanout_policy_version"`
	OutboxStatus              string `json:"outbox_status"`
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
	flag.StringVar(&cfg.target, "target", "127.0.0.1:10495", "message-service gRPC target")
	flag.StringVar(&cfg.resultDir, "result-dir", "H:\\NexusIM\\loadtest-results\\policy-message-smoke", "result directory")
	flag.StringVar(&cfg.pgDSN, "pg-dsn", "", "PostgreSQL DSN")
	flag.StringVar(&cfg.scenario, "scenario", "allow", "scenario: allow or deny")
	flag.DurationVar(&cfg.requestTimeout, "request-timeout", 5*time.Second, "per-request timeout")
	flag.StringVar(&cfg.tenantID, "tenant-id", "tenant-policy-message-smoke", "tenant id")
	flag.StringVar(&cfg.userID, "user-id", "policy-message-user", "user id")
	flag.StringVar(&cfg.deviceID, "device-id", "policy-message-device-1", "device id")
	flag.StringVar(&cfg.sessionID, "session-id", "policy-message-session-1", "session id")
	flag.StringVar(&cfg.conversationID, "conversation-id", "policy-message-conversation", "conversation id")
	flag.StringVar(&cfg.clientMsgID, "client-msg-id", "policy-message-client-msg-1", "client message id")
	flag.Int64Var(&cfg.expectedPermissionVer, "expected-permission-version", 1, "expected permission version")
	flag.StringVar(&cfg.expectedClassification, "expected-classification", "INTERNAL", "expected classification")
	flag.StringVar(&cfg.expectedReason, "expected-reason", "", "expected deny reason")
	flag.BoolVar(&cfg.cleanup, "cleanup", false, "delete message rows for tenant before running")
	flag.Parse()
	cfg.scenario = strings.ToLower(strings.TrimSpace(cfg.scenario))
	if cfg.requestTimeout <= 0 {
		cfg.requestTimeout = 5 * time.Second
	}
	return cfg
}

func run(cfg config) error {
	if cfg.scenario != "allow" && cfg.scenario != "deny" {
		return fmt.Errorf("unsupported scenario %q", cfg.scenario)
	}
	if cfg.pgDSN == "" {
		return fmt.Errorf("--pg-dsn is required")
	}
	if err := os.MkdirAll(cfg.resultDir, 0o755); err != nil {
		return fmt.Errorf("create result dir: %w", err)
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, cfg.pgDSN)
	if err != nil {
		return fmt.Errorf("connect postgres: %w", err)
	}
	defer pool.Close()
	if cfg.cleanup {
		if err := cleanupTenant(ctx, pool, cfg.tenantID); err != nil {
			return err
		}
	}

	started := time.Now().UTC()
	s := summary{
		Commit:                 gitOutput("rev-parse", "--short", "HEAD"),
		CommitFull:             gitOutput("rev-parse", "HEAD"),
		GitStatusShort:         gitOutput("status", "--short"),
		Target:                 cfg.target,
		ResultDir:              cfg.resultDir,
		Scenario:               cfg.scenario,
		TenantID:               cfg.tenantID,
		UserID:                 cfg.userID,
		ConversationID:         cfg.conversationID,
		StartedAt:              started,
		ExpectedPermissionVer:  cfg.expectedPermissionVer,
		ExpectedClassification: cfg.expectedClassification,
		ExpectedReason:         cfg.expectedReason,
	}
	s.GitDirty = strings.TrimSpace(s.GitStatusShort) != ""
	defer func() {
		s.FinishedAt = time.Now().UTC()
		_ = writeSummary(cfg.resultDir, s)
	}()

	before, err := readDBStats(ctx, pool, cfg.tenantID)
	if err != nil {
		s.Error = err.Error()
		return err
	}
	s.DBBefore = before

	response, callErr, latencyMS := sendMessage(cfg)
	s.LatencyMS = latencyMS
	if cfg.scenario == "allow" {
		if callErr != nil {
			s.Error = callErr.Error()
			return callErr
		}
		s.SendMessage = sendSummary{
			MessageID:        response.GetMessageId(),
			ConversationSeq:  response.GetConversationSeq(),
			IdempotentReplay: response.GetIdempotentReplay(),
			GRPCCode:         codes.OK.String(),
		}
		row, err := readMessageRow(ctx, pool, cfg, response.GetMessageId())
		if err != nil {
			s.Error = err.Error()
			return err
		}
		if err := validateAllow(cfg, response, row); err != nil {
			s.Error = err.Error()
			return err
		}
		s.MessageRow = row
	} else {
		errorSummary, err := validateDeny(callErr)
		if err != nil {
			s.Error = err.Error()
			return err
		}
		s.SendMessage = sendSummary{GRPCCode: codes.PermissionDenied.String()}
		s.MessageError = errorSummary
	}

	after, err := readDBStats(ctx, pool, cfg.tenantID)
	if err != nil {
		s.Error = err.Error()
		return err
	}
	s.DBAfter = after
	if cfg.scenario == "deny" && after != before {
		err := fmt.Errorf("deny scenario changed DB counts before=%+v after=%+v", before, after)
		s.Error = err.Error()
		return err
	}
	s.Success = true
	return nil
}

func sendMessage(cfg config) (*messagev1.SendMessageResponse, error, float64) {
	conn, err := grpc.NewClient(cfg.target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("dial message-service: %w", err), 0
	}
	defer conn.Close()

	payload, err := structpb.NewStruct(map[string]any{"text": "policy integration smoke"})
	if err != nil {
		return nil, err, 0
	}
	ctx, cancel := context.WithTimeout(context.Background(), cfg.requestTimeout)
	defer cancel()
	started := time.Now()
	response, err := messagev1.NewMessageServiceClient(conn).SendMessage(ctx, &messagev1.SendMessageRequest{
		AuthContext: &messagev1.AuthContext{
			TenantId:  cfg.tenantID,
			UserId:    cfg.userID,
			DeviceId:  cfg.deviceID,
			SessionId: cfg.sessionID,
			TraceId:   "trace-policy-message-smoke",
			RequestId: "policy-message-smoke-" + cfg.scenario,
		},
		ConversationId: cfg.conversationID,
		ClientMsgId:    cfg.clientMsgID,
		MessageType:    "TEXT",
		Payload:        payload,
	})
	return response, err, float64(time.Since(started).Microseconds()) / 1000.0
}

func validateAllow(cfg config, response *messagev1.SendMessageResponse, row messageRow) error {
	if response.GetMessageId() == "" || response.GetConversationSeq() <= 0 {
		return fmt.Errorf("allow returned invalid response message_id=%q seq=%d", response.GetMessageId(), response.GetConversationSeq())
	}
	if row.MessagePermissionVersion != cfg.expectedPermissionVer ||
		row.TimelinePermissionVersion != cfg.expectedPermissionVer {
		return fmt.Errorf("permission_version mismatch row=%+v expected=%d", row, cfg.expectedPermissionVer)
	}
	if row.MessageClassification != cfg.expectedClassification ||
		row.TimelineClassification != cfg.expectedClassification {
		return fmt.Errorf("classification mismatch row=%+v expected=%q", row, cfg.expectedClassification)
	}
	if row.OutboxStatus != "PENDING" && row.OutboxStatus != "PUBLISHED" {
		return fmt.Errorf("unexpected outbox status %q", row.OutboxStatus)
	}
	return nil
}

func validateDeny(err error) (errorSummary, error) {
	if err == nil {
		return errorSummary{}, fmt.Errorf("deny scenario unexpectedly succeeded")
	}
	st, ok := status.FromError(err)
	if !ok {
		return errorSummary{}, fmt.Errorf("deny returned non-grpc error: %w", err)
	}
	if st.Code() != codes.PermissionDenied {
		return errorSummary{}, fmt.Errorf("deny grpc code=%s, expected %s", st.Code(), codes.PermissionDenied)
	}
	var result errorSummary
	for _, detail := range st.Details() {
		if messageError, ok := detail.(*messagev1.MessageError); ok {
			if messageError.GetCode() != messagev1.MessageErrorCode_MESSAGE_ERROR_CODE_PERMISSION_DENIED {
				return errorSummary{}, fmt.Errorf("deny message error code=%s", messageError.GetCode())
			}
			if messageError.GetRetryable() {
				return errorSummary{}, fmt.Errorf("deny message error unexpectedly retryable")
			}
			result = errorSummary{
				Code:      messageError.GetCode().String(),
				Message:   messageError.GetMessage(),
				Retryable: messageError.GetRetryable(),
			}
			break
		}
	}
	if result.Code == "" {
		return errorSummary{}, fmt.Errorf("deny response missing MessageError detail")
	}
	return result, nil
}

func readDBStats(ctx context.Context, pool *pgxpool.Pool, tenantID string) (dbStats, error) {
	var stats dbStats
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM message_log WHERE tenant_id = $1`, tenantID).Scan(&stats.MessageLog); err != nil {
		return stats, err
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM conversation_timeline_events WHERE tenant_id = $1`, tenantID).Scan(&stats.TimelineEvents); err != nil {
		return stats, err
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM message_outbox WHERE tenant_id = $1`, tenantID).Scan(&stats.MessageOutbox); err != nil {
		return stats, err
	}
	return stats, nil
}

func readMessageRow(ctx context.Context, pool *pgxpool.Pool, cfg config, messageID string) (messageRow, error) {
	row := pool.QueryRow(ctx, `
SELECT
    ml.message_id,
    ml.conversation_seq,
    ml.permission_version,
    ml.classification,
    te.permission_version,
    COALESCE(te.classification, ''),
    te.fanout_policy_version,
    mo.status
FROM message_log ml
JOIN conversation_timeline_events te
  ON te.tenant_id = ml.tenant_id
 AND te.conversation_id = ml.conversation_id
 AND te.seq = ml.conversation_seq
LEFT JOIN message_outbox mo
  ON mo.tenant_id = ml.tenant_id
 AND mo.conversation_id = ml.conversation_id
 AND mo.aggregate_version = ml.conversation_seq
WHERE ml.tenant_id = $1
  AND ml.message_id = $2
`, cfg.tenantID, messageID)
	var result messageRow
	if err := row.Scan(
		&result.MessageID,
		&result.ConversationSeq,
		&result.MessagePermissionVersion,
		&result.MessageClassification,
		&result.TimelinePermissionVersion,
		&result.TimelineClassification,
		&result.FanoutPolicyVersion,
		&result.OutboxStatus,
	); err != nil {
		return result, err
	}
	return result, nil
}

func cleanupTenant(ctx context.Context, pool *pgxpool.Pool, tenantID string) error {
	for _, statement := range []string{
		`DELETE FROM message_outbox WHERE tenant_id = $1`,
		`DELETE FROM conversation_timeline_events WHERE tenant_id = $1`,
		`DELETE FROM message_change_history WHERE tenant_id = $1`,
		`DELETE FROM message_command_idempotency WHERE tenant_id = $1`,
		`DELETE FROM message_log WHERE tenant_id = $1`,
		`DELETE FROM conversation_seq WHERE tenant_id = $1`,
		`DELETE FROM seq_allocation_journal WHERE tenant_id = $1`,
		`DELETE FROM timeline_gap_markers WHERE tenant_id = $1`,
	} {
		if _, err := pool.Exec(ctx, statement, tenantID); err != nil {
			return fmt.Errorf("cleanup tenant: %w", err)
		}
	}
	return nil
}

func writeSummary(resultDir string, s summary) error {
	path := filepath.Join(resultDir, "policy-message-summary.json")
	bytes, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(bytes, '\n'), 0o644)
}

func gitOutput(args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", args...).CombinedOutput()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
