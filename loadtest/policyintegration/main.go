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
	action                 string
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
	seedPolicyRule         bool
}

type summary struct {
	Commit                 string        `json:"commit"`
	CommitFull             string        `json:"commit_full"`
	GitDirty               bool          `json:"git_dirty"`
	GitStatusShort         string        `json:"git_status_short,omitempty"`
	Target                 string        `json:"target"`
	ResultDir              string        `json:"result_dir"`
	Scenario               string        `json:"scenario"`
	Action                 string        `json:"action"`
	TenantID               string        `json:"tenant_id"`
	UserID                 string        `json:"user_id"`
	ConversationID         string        `json:"conversation_id"`
	StartedAt              time.Time     `json:"started_at"`
	FinishedAt             time.Time     `json:"finished_at"`
	Success                bool          `json:"success"`
	Error                  string        `json:"error,omitempty"`
	ExpectedPermissionVer  int64         `json:"expected_permission_version"`
	ExpectedClassification string        `json:"expected_classification"`
	ExpectedReason         string        `json:"expected_reason,omitempty"`
	SendMessage            sendSummary   `json:"send_message"`
	ChangeMessage          changeSummary `json:"change_message,omitempty"`
	MessageError           errorSummary  `json:"message_error,omitempty"`
	PolicyRuleSeeded       bool          `json:"policy_rule_seeded"`
	PolicyRule             policyRule    `json:"policy_rule,omitempty"`
	PolicyRules            []policyRule  `json:"policy_rules,omitempty"`
	DBBefore               dbStats       `json:"db_before"`
	DBBeforeAction         dbStats       `json:"db_before_action,omitempty"`
	DBAfter                dbStats       `json:"db_after"`
	MessageRow             messageRow    `json:"message_row,omitempty"`
	ChangeRow              changeRow     `json:"change_row,omitempty"`
	LatencyMS              float64       `json:"latency_ms"`
}

type sendSummary struct {
	MessageID        string `json:"message_id,omitempty"`
	ConversationSeq  int64  `json:"conversation_seq,omitempty"`
	IdempotentReplay bool   `json:"idempotent_replay,omitempty"`
	GRPCCode         string `json:"grpc_code"`
}

type changeSummary struct {
	MessageID        string `json:"message_id,omitempty"`
	ConversationSeq  int64  `json:"conversation_seq,omitempty"`
	ChangeVersion    int32  `json:"change_version,omitempty"`
	IdempotentReplay bool   `json:"idempotent_replay,omitempty"`
	GRPCCode         string `json:"grpc_code"`
}

type errorSummary struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

type policyRule struct {
	TenantID          string `json:"tenant_id,omitempty"`
	UserID            string `json:"user_id,omitempty"`
	ConversationID    string `json:"conversation_id,omitempty"`
	Action            string `json:"action,omitempty"`
	Allowed           bool   `json:"allowed"`
	PermissionVersion int64  `json:"permission_version,omitempty"`
	Classification    string `json:"classification,omitempty"`
	Reason            string `json:"reason,omitempty"`
}

type dbStats struct {
	MessageLog           int64 `json:"message_log"`
	TimelineEvents       int64 `json:"conversation_timeline_events"`
	MessageOutbox        int64 `json:"message_outbox"`
	MessageChangeHistory int64 `json:"message_change_history"`
	CommandIdempotency   int64 `json:"message_command_idempotency"`
	ConversationSeq      int64 `json:"conversation_seq"`
}

type messageRow struct {
	MessageID                 string `json:"message_id"`
	ConversationSeq           int64  `json:"conversation_seq"`
	MessageStatus             string `json:"message_status"`
	MessagePayload            string `json:"message_payload,omitempty"`
	MessagePermissionVersion  int64  `json:"message_permission_version"`
	MessageClassification     string `json:"message_classification"`
	TimelinePermissionVersion int64  `json:"timeline_permission_version"`
	TimelineClassification    string `json:"timeline_classification"`
	FanoutPolicyVersion       int64  `json:"fanout_policy_version"`
	OutboxStatus              string `json:"outbox_status"`
}

type changeRow struct {
	MessageID                 string `json:"message_id"`
	MessageStatus             string `json:"message_status"`
	MessagePayload            string `json:"message_payload,omitempty"`
	TimelineEventType         string `json:"timeline_event_type"`
	TimelinePermissionVersion int64  `json:"timeline_permission_version"`
	TimelineClassification    string `json:"timeline_classification"`
	OutboxStatus              string `json:"outbox_status"`
	ChangeHistoryRows         int64  `json:"message_change_history_rows"`
	ChangeHistoryType         string `json:"message_change_history_type,omitempty"`
	ChangeHistoryBeforeStatus string `json:"message_change_history_before_status,omitempty"`
	ChangeHistoryAfterStatus  string `json:"message_change_history_after_status,omitempty"`
	EditedAtSet               bool   `json:"edited_at_set,omitempty"`
	RevokedAtSet              bool   `json:"revoked_at_set,omitempty"`
	DeletedAtSet              bool   `json:"deleted_at_set,omitempty"`
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
	flag.StringVar(&cfg.action, "action", "send", "message action: send, edit, revoke, or delete")
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
	flag.BoolVar(&cfg.seedPolicyRule, "seed-policy-rule", false, "seed exact policy_message_action_rules row for this scenario")
	flag.Parse()
	cfg.scenario = strings.ToLower(strings.TrimSpace(cfg.scenario))
	cfg.action = strings.ToLower(strings.TrimSpace(cfg.action))
	if cfg.requestTimeout <= 0 {
		cfg.requestTimeout = 5 * time.Second
	}
	return cfg
}

func run(cfg config) error {
	if cfg.scenario != "allow" && cfg.scenario != "deny" {
		return fmt.Errorf("unsupported scenario %q", cfg.scenario)
	}
	if !isSupportedAction(cfg.action) {
		return fmt.Errorf("unsupported action %q", cfg.action)
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
	if cfg.seedPolicyRule {
		if err := seedPolicyRules(ctx, pool, cfg); err != nil {
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
		Action:                 cfg.action,
		TenantID:               cfg.tenantID,
		UserID:                 cfg.userID,
		ConversationID:         cfg.conversationID,
		StartedAt:              started,
		ExpectedPermissionVer:  cfg.expectedPermissionVer,
		ExpectedClassification: cfg.expectedClassification,
		ExpectedReason:         cfg.expectedReason,
		PolicyRuleSeeded:       cfg.seedPolicyRule,
	}
	if cfg.seedPolicyRule {
		s.PolicyRules = expectedPolicyRules(cfg)
		if len(s.PolicyRules) > 0 {
			s.PolicyRule = s.PolicyRules[len(s.PolicyRules)-1]
		}
	}
	s.GitDirty = strings.TrimSpace(s.GitStatusShort) != ""
	defer func() {
		s.FinishedAt = time.Now().UTC()
		_ = writeSummary(cfg.resultDir, s)
	}()

	before, err := readDBStats(ctx, pool, cfg)
	if err != nil {
		s.Error = err.Error()
		return err
	}
	s.DBBefore = before

	if cfg.action == "send" {
		if err := runSendScenario(ctx, pool, cfg, &s); err != nil {
			s.Error = err.Error()
			return err
		}
	} else {
		if err := runChangeScenario(ctx, pool, cfg, &s); err != nil {
			s.Error = err.Error()
			return err
		}
	}

	after, err := readDBStats(ctx, pool, cfg)
	if err != nil {
		s.Error = err.Error()
		return err
	}
	s.DBAfter = after
	if cfg.action == "send" && cfg.scenario == "deny" && after != before {
		err := fmt.Errorf("deny scenario changed DB counts before=%+v after=%+v", before, after)
		s.Error = err.Error()
		return err
	}
	s.Success = true
	return nil
}

func runSendScenario(ctx context.Context, pool *pgxpool.Pool, cfg config, s *summary) error {
	response, callErr, latencyMS := sendMessage(cfg)
	s.LatencyMS = latencyMS
	if cfg.scenario == "allow" {
		if callErr != nil {
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
			return err
		}
		if err := validateSendAllow(cfg, response, row); err != nil {
			return err
		}
		s.MessageRow = row
		return nil
	}

	errorSummary, err := validateDeny(callErr)
	if err != nil {
		return err
	}
	s.SendMessage = sendSummary{GRPCCode: codes.PermissionDenied.String()}
	s.MessageError = errorSummary
	return nil
}

func runChangeScenario(ctx context.Context, pool *pgxpool.Pool, cfg config, s *summary) error {
	response, callErr, latencyMS := sendMessage(cfg)
	s.LatencyMS = latencyMS
	if callErr != nil {
		return fmt.Errorf("base SendMessage for %s scenario: %w", cfg.action, callErr)
	}
	s.SendMessage = sendSummary{
		MessageID:        response.GetMessageId(),
		ConversationSeq:  response.GetConversationSeq(),
		IdempotentReplay: response.GetIdempotentReplay(),
		GRPCCode:         codes.OK.String(),
	}
	row, err := readMessageRow(ctx, pool, cfg, response.GetMessageId())
	if err != nil {
		return err
	}
	if err := validateBaseSend(cfg, response, row); err != nil {
		return err
	}
	s.MessageRow = row

	beforeAction, err := readDBStats(ctx, pool, cfg)
	if err != nil {
		return err
	}
	s.DBBeforeAction = beforeAction

	change, callErr, changeLatencyMS := changeMessage(cfg, response.GetMessageId())
	s.LatencyMS += changeLatencyMS
	if cfg.scenario == "allow" {
		if callErr != nil {
			return callErr
		}
		s.ChangeMessage = changeSummary{
			MessageID:        change.GetMessageId(),
			ConversationSeq:  change.GetConversationSeq(),
			ChangeVersion:    change.GetChangeVersion(),
			IdempotentReplay: change.GetIdempotentReplay(),
			GRPCCode:         codes.OK.String(),
		}
		changeRow, err := readChangeRow(ctx, pool, cfg, response.GetMessageId(), change.GetConversationSeq())
		if err != nil {
			return err
		}
		if err := validateChangeAllow(cfg, response, change, changeRow); err != nil {
			return err
		}
		s.ChangeRow = changeRow
		return nil
	}

	errorSummary, err := validateDeny(callErr)
	if err != nil {
		return err
	}
	s.ChangeMessage = changeSummary{GRPCCode: codes.PermissionDenied.String()}
	s.MessageError = errorSummary
	afterAction, err := readDBStats(ctx, pool, cfg)
	if err != nil {
		return err
	}
	if afterAction != beforeAction {
		return fmt.Errorf("%s deny changed DB counts before=%+v after=%+v", cfg.action, beforeAction, afterAction)
	}
	afterRow, err := readMessageRow(ctx, pool, cfg, response.GetMessageId())
	if err != nil {
		return err
	}
	if afterRow != row {
		return fmt.Errorf("%s deny changed base message row before=%+v after=%+v", cfg.action, row, afterRow)
	}
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

func changeMessage(cfg config, messageID string) (*messagev1.MessageChangeResponse, error, float64) {
	conn, err := grpc.NewClient(cfg.target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("dial message-service: %w", err), 0
	}
	defer conn.Close()
	client := messagev1.NewMessageServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), cfg.requestTimeout)
	defer cancel()
	started := time.Now()
	var response *messagev1.MessageChangeResponse
	switch cfg.action {
	case "edit":
		payload, err := structpb.NewStruct(map[string]any{"text": "policy integration smoke edited"})
		if err != nil {
			return nil, err, 0
		}
		response, err = client.EditMessage(ctx, &messagev1.EditMessageRequest{
			AuthContext:    authContext(cfg, "policy-message-edit-"+cfg.scenario),
			ConversationId: cfg.conversationID,
			MessageId:      messageID,
			IdempotencyKey: "policy-message-edit-" + cfg.scenario,
			Payload:        payload,
			Reason:         "policy integration smoke",
		})
		return response, err, float64(time.Since(started).Microseconds()) / 1000.0
	case "revoke":
		response, err = client.RevokeMessage(ctx, &messagev1.RevokeMessageRequest{
			AuthContext:    authContext(cfg, "policy-message-revoke-"+cfg.scenario),
			ConversationId: cfg.conversationID,
			MessageId:      messageID,
			IdempotencyKey: "policy-message-revoke-" + cfg.scenario,
			Reason:         "policy integration smoke",
		})
		return response, err, float64(time.Since(started).Microseconds()) / 1000.0
	case "delete":
		response, err = client.DeleteMessage(ctx, &messagev1.DeleteMessageRequest{
			AuthContext:    authContext(cfg, "policy-message-delete-"+cfg.scenario),
			ConversationId: cfg.conversationID,
			MessageId:      messageID,
			IdempotencyKey: "policy-message-delete-" + cfg.scenario,
			DeleteScope:    messagev1.DeleteScope_DELETE_SCOPE_CONVERSATION_VIEW,
			Reason:         "policy integration smoke",
		})
		return response, err, float64(time.Since(started).Microseconds()) / 1000.0
	default:
		return nil, fmt.Errorf("unsupported change action %q", cfg.action), 0
	}
}

func authContext(cfg config, requestID string) *messagev1.AuthContext {
	return &messagev1.AuthContext{
		TenantId:  cfg.tenantID,
		UserId:    cfg.userID,
		DeviceId:  cfg.deviceID,
		SessionId: cfg.sessionID,
		TraceId:   "trace-policy-message-smoke",
		RequestId: requestID,
	}
}

func validateSendAllow(cfg config, response *messagev1.SendMessageResponse, row messageRow) error {
	if response.GetMessageId() == "" || response.GetConversationSeq() <= 0 {
		return fmt.Errorf("allow returned invalid response message_id=%q seq=%d", response.GetMessageId(), response.GetConversationSeq())
	}
	if response.GetAcceptedAt() == nil {
		return fmt.Errorf("allow returned nil accepted_at")
	}
	if response.GetIdempotentReplay() {
		return fmt.Errorf("allow unexpectedly returned idempotent replay")
	}
	if row.MessageID != response.GetMessageId() || row.ConversationSeq != response.GetConversationSeq() {
		return fmt.Errorf("message row does not match response row=%+v response_message_id=%q response_seq=%d", row, response.GetMessageId(), response.GetConversationSeq())
	}
	if row.MessageStatus != "NORMAL" {
		return fmt.Errorf("message status=%q expected NORMAL", row.MessageStatus)
	}
	if !strings.Contains(row.MessagePayload, "policy integration smoke") {
		return fmt.Errorf("message payload does not contain smoke text: %s", row.MessagePayload)
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

func validateBaseSend(cfg config, response *messagev1.SendMessageResponse, row messageRow) error {
	if row.MessageID == "" || row.ConversationSeq <= 0 {
		return fmt.Errorf("base SendMessage did not persist a message: %+v", row)
	}
	if response.GetMessageId() == "" || response.GetConversationSeq() <= 0 {
		return fmt.Errorf("base SendMessage returned invalid response message_id=%q seq=%d", response.GetMessageId(), response.GetConversationSeq())
	}
	if response.GetAcceptedAt() == nil {
		return fmt.Errorf("base SendMessage returned nil accepted_at")
	}
	if response.GetIdempotentReplay() {
		return fmt.Errorf("base SendMessage unexpectedly returned idempotent replay")
	}
	if row.MessageID != response.GetMessageId() || row.ConversationSeq != response.GetConversationSeq() {
		return fmt.Errorf("base SendMessage row does not match response row=%+v response_message_id=%q response_seq=%d", row, response.GetMessageId(), response.GetConversationSeq())
	}
	if row.MessageStatus != "NORMAL" {
		return fmt.Errorf("base SendMessage status=%q expected NORMAL", row.MessageStatus)
	}
	if !strings.Contains(row.MessagePayload, "policy integration smoke") {
		return fmt.Errorf("base SendMessage payload does not contain smoke text: %s", row.MessagePayload)
	}
	if row.MessagePermissionVersion != cfg.expectedPermissionVer || row.TimelinePermissionVersion != cfg.expectedPermissionVer {
		return fmt.Errorf("base SendMessage permission_version mismatch row=%+v expected=%d", row, cfg.expectedPermissionVer)
	}
	if row.MessageClassification != "POLICY_SEND_SEED" || row.TimelineClassification != "POLICY_SEND_SEED" {
		return fmt.Errorf("base SendMessage classification mismatch row=%+v expected POLICY_SEND_SEED", row)
	}
	if row.OutboxStatus != "PENDING" && row.OutboxStatus != "PUBLISHED" {
		return fmt.Errorf("unexpected base SendMessage outbox status %q", row.OutboxStatus)
	}
	return nil
}

func validateChangeAllow(
	cfg config,
	send *messagev1.SendMessageResponse,
	change *messagev1.MessageChangeResponse,
	row changeRow,
) error {
	if change.GetMessageId() != send.GetMessageId() {
		return fmt.Errorf("change message_id=%q expected %q", change.GetMessageId(), send.GetMessageId())
	}
	if change.GetConversationId() != cfg.conversationID {
		return fmt.Errorf("change conversation_id=%q expected %q", change.GetConversationId(), cfg.conversationID)
	}
	if change.GetConversationSeq() != send.GetConversationSeq()+1 {
		return fmt.Errorf("change seq=%d expected send seq + 1 (%d)", change.GetConversationSeq(), send.GetConversationSeq()+1)
	}
	if change.GetChangeVersion() <= 0 {
		return fmt.Errorf("change version must be positive, got %d", change.GetChangeVersion())
	}
	if change.GetAcceptedAt() == nil {
		return fmt.Errorf("change returned nil accepted_at")
	}
	if change.GetIdempotentReplay() {
		return fmt.Errorf("change unexpectedly returned idempotent replay")
	}
	expectedStatus := map[string]string{
		"edit":   "EDITED",
		"revoke": "REVOKED",
		"delete": "DELETED",
	}[cfg.action]
	expectedEventType := map[string]string{
		"edit":   "message.edited.v1",
		"revoke": "message.revoked.v1",
		"delete": "message.deleted.v1",
	}[cfg.action]
	if row.MessageStatus != expectedStatus {
		return fmt.Errorf("message status=%s expected %s", row.MessageStatus, expectedStatus)
	}
	if row.MessageID != send.GetMessageId() {
		return fmt.Errorf("change row message_id=%q expected %q", row.MessageID, send.GetMessageId())
	}
	if row.TimelineEventType != expectedEventType {
		return fmt.Errorf("timeline event_type=%s expected %s", row.TimelineEventType, expectedEventType)
	}
	if row.TimelinePermissionVersion != cfg.expectedPermissionVer {
		return fmt.Errorf("timeline permission_version=%d expected %d", row.TimelinePermissionVersion, cfg.expectedPermissionVer)
	}
	if row.TimelineClassification != cfg.expectedClassification {
		return fmt.Errorf("timeline classification=%q expected %q", row.TimelineClassification, cfg.expectedClassification)
	}
	if row.ChangeHistoryRows <= 0 {
		return fmt.Errorf("expected change history row, got %d", row.ChangeHistoryRows)
	}
	expectedChangeType := strings.ToUpper(cfg.action)
	if row.ChangeHistoryType != expectedChangeType {
		return fmt.Errorf("change_history type=%q expected %q", row.ChangeHistoryType, expectedChangeType)
	}
	if row.ChangeHistoryBeforeStatus != "NORMAL" || row.ChangeHistoryAfterStatus != expectedStatus {
		return fmt.Errorf("unexpected change_history statuses before=%q after=%q expected before=NORMAL after=%s", row.ChangeHistoryBeforeStatus, row.ChangeHistoryAfterStatus, expectedStatus)
	}
	if row.OutboxStatus != "PENDING" && row.OutboxStatus != "PUBLISHED" {
		return fmt.Errorf("unexpected change outbox status %q", row.OutboxStatus)
	}
	switch cfg.action {
	case "edit":
		if !row.EditedAtSet || row.RevokedAtSet || row.DeletedAtSet {
			return fmt.Errorf("unexpected edit timestamp flags edited=%v revoked=%v deleted=%v", row.EditedAtSet, row.RevokedAtSet, row.DeletedAtSet)
		}
		if !strings.Contains(row.MessagePayload, "policy integration smoke edited") {
			return fmt.Errorf("edited payload does not contain updated text: %s", row.MessagePayload)
		}
	case "revoke":
		if row.EditedAtSet || !row.RevokedAtSet || row.DeletedAtSet {
			return fmt.Errorf("unexpected revoke timestamp flags edited=%v revoked=%v deleted=%v", row.EditedAtSet, row.RevokedAtSet, row.DeletedAtSet)
		}
	case "delete":
		if row.EditedAtSet || row.RevokedAtSet || !row.DeletedAtSet {
			return fmt.Errorf("unexpected delete timestamp flags edited=%v revoked=%v deleted=%v", row.EditedAtSet, row.RevokedAtSet, row.DeletedAtSet)
		}
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

func readDBStats(ctx context.Context, pool *pgxpool.Pool, cfg config) (dbStats, error) {
	var stats dbStats
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM message_log WHERE tenant_id = $1`, cfg.tenantID).Scan(&stats.MessageLog); err != nil {
		return stats, err
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM conversation_timeline_events WHERE tenant_id = $1`, cfg.tenantID).Scan(&stats.TimelineEvents); err != nil {
		return stats, err
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM message_outbox WHERE tenant_id = $1`, cfg.tenantID).Scan(&stats.MessageOutbox); err != nil {
		return stats, err
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM message_change_history WHERE tenant_id = $1`, cfg.tenantID).Scan(&stats.MessageChangeHistory); err != nil {
		return stats, err
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM message_command_idempotency WHERE tenant_id = $1`, cfg.tenantID).Scan(&stats.CommandIdempotency); err != nil {
		return stats, err
	}
	if err := pool.QueryRow(ctx, `
SELECT COALESCE(MAX(current_seq), 0)
FROM conversation_seq
WHERE tenant_id = $1
  AND conversation_id = $2
`, cfg.tenantID, cfg.conversationID).Scan(&stats.ConversationSeq); err != nil {
		return stats, err
	}
	return stats, nil
}

func readMessageRow(ctx context.Context, pool *pgxpool.Pool, cfg config, messageID string) (messageRow, error) {
	row := pool.QueryRow(ctx, `
SELECT
    ml.message_id,
    ml.conversation_seq,
    ml.status,
    ml.payload_json::text,
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
		&result.MessageStatus,
		&result.MessagePayload,
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

func readChangeRow(
	ctx context.Context,
	pool *pgxpool.Pool,
	cfg config,
	messageID string,
	conversationSeq int64,
) (changeRow, error) {
	row := pool.QueryRow(ctx, `
SELECT
    ml.message_id,
    ml.status,
    ml.payload_json::text,
    te.event_type,
    te.permission_version,
    COALESCE(te.classification, ''),
    mo.status,
    (
      SELECT COUNT(*)
      FROM message_change_history mch
      WHERE mch.tenant_id = ml.tenant_id
        AND mch.conversation_id = ml.conversation_id
        AND mch.message_id = ml.message_id
    ),
    COALESCE(latest.change_type, ''),
    COALESCE(latest.before_status, ''),
    COALESCE(latest.after_status, ''),
    ml.edited_at IS NOT NULL,
    ml.revoked_at IS NOT NULL,
    ml.deleted_at IS NOT NULL
FROM message_log ml
JOIN conversation_timeline_events te
  ON te.tenant_id = ml.tenant_id
 AND te.conversation_id = ml.conversation_id
 AND te.seq = $3
LEFT JOIN message_outbox mo
  ON mo.tenant_id = te.tenant_id
 AND mo.conversation_id = te.conversation_id
 AND mo.aggregate_version = te.seq
LEFT JOIN LATERAL (
  SELECT mch.change_type, mch.before_status, mch.after_status
  FROM message_change_history mch
  WHERE mch.tenant_id = ml.tenant_id
    AND mch.conversation_id = ml.conversation_id
    AND mch.message_id = ml.message_id
  ORDER BY mch.change_version DESC
  LIMIT 1
) latest ON TRUE
WHERE ml.tenant_id = $1
  AND ml.message_id = $2
`, cfg.tenantID, messageID, conversationSeq)
	var result changeRow
	if err := row.Scan(
		&result.MessageID,
		&result.MessageStatus,
		&result.MessagePayload,
		&result.TimelineEventType,
		&result.TimelinePermissionVersion,
		&result.TimelineClassification,
		&result.OutboxStatus,
		&result.ChangeHistoryRows,
		&result.ChangeHistoryType,
		&result.ChangeHistoryBeforeStatus,
		&result.ChangeHistoryAfterStatus,
		&result.EditedAtSet,
		&result.RevokedAtSet,
		&result.DeletedAtSet,
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

func seedPolicyRules(ctx context.Context, pool *pgxpool.Pool, cfg config) error {
	if _, err := pool.Exec(ctx, `DELETE FROM policy_decision_audit_outbox WHERE tenant_id = $1`, cfg.tenantID); err != nil {
		return fmt.Errorf("cleanup policy decision audit outbox: %w", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM policy_message_action_rules WHERE tenant_id = $1`, cfg.tenantID); err != nil {
		return fmt.Errorf("cleanup policy rules: %w", err)
	}
	for _, rule := range expectedPolicyRules(cfg) {
		if err := seedOnePolicyRule(ctx, pool, rule); err != nil {
			return err
		}
	}
	return nil
}

func seedOnePolicyRule(ctx context.Context, pool *pgxpool.Pool, rule policyRule) error {
	_, err := pool.Exec(ctx, `
INSERT INTO policy_message_action_rules (
    tenant_id,
    user_id,
    conversation_id,
    action,
    allowed,
    permission_version,
    classification,
    reason,
    source
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'loadtest')
ON CONFLICT (tenant_id, user_id, conversation_id, action) DO UPDATE
SET allowed = EXCLUDED.allowed,
    permission_version = EXCLUDED.permission_version,
    classification = EXCLUDED.classification,
    reason = EXCLUDED.reason,
    source = EXCLUDED.source,
    updated_at = now()
`, rule.TenantID, rule.UserID, rule.ConversationID, rule.Action, rule.Allowed, rule.PermissionVersion, rule.Classification, rule.Reason)
	if err != nil {
		return fmt.Errorf("seed policy rule: %w", err)
	}
	return nil
}

func expectedPolicyRules(cfg config) []policyRule {
	rules := make([]policyRule, 0, 2)
	if cfg.action != "send" {
		rules = append(rules, policyRule{
			TenantID:          cfg.tenantID,
			UserID:            cfg.userID,
			ConversationID:    cfg.conversationID,
			Action:            "SEND",
			Allowed:           true,
			PermissionVersion: cfg.expectedPermissionVer,
			Classification:    "POLICY_SEND_SEED",
		})
	}
	allowed := cfg.scenario == "allow"
	reason := cfg.expectedReason
	if !allowed && strings.TrimSpace(reason) == "" {
		reason = "policy denied"
	}
	rules = append(rules, policyRule{
		TenantID:          cfg.tenantID,
		UserID:            cfg.userID,
		ConversationID:    cfg.conversationID,
		Action:            strings.ToUpper(cfg.action),
		Allowed:           allowed,
		PermissionVersion: cfg.expectedPermissionVer,
		Classification:    cfg.expectedClassification,
		Reason:            reason,
	})
	return rules
}

func isSupportedAction(action string) bool {
	switch action {
	case "send", "edit", "revoke", "delete":
		return true
	default:
		return false
	}
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
