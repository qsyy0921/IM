package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	messagev1 "github.com/qsyy0921/IM/api/proto/nexusim/message/v1"
	"github.com/qsyy0921/IM/loadtest/internal/grpctls"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
)

func main() {
	cfg := parseConfig()
	if err := run(cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
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
	if shouldValidatePolicyAudit(cfg) {
		if err := cleanupPolicyAudit(ctx, pool, cfg.tenantID); err != nil {
			return err
		}
	}
	if cfg.seedPolicyRule {
		if err := seedPolicyRules(ctx, pool, cfg); err != nil {
			return err
		}
	}
	if cfg.seedTenantPolicyRule {
		if err := seedTenantPolicyRules(ctx, pool, cfg); err != nil {
			return err
		}
	}
	if cfg.seedConversationRole {
		if err := seedConversationRoleGate(ctx, pool, cfg); err != nil {
			return err
		}
	}
	if cfg.seedOwnershipOverride {
		if err := seedOwnershipOverrideRule(ctx, pool, cfg); err != nil {
			return err
		}
	}

	started := time.Now().UTC()
	s := summary{
		Commit:                  gitOutput("rev-parse", "--short", "HEAD"),
		CommitFull:              gitOutput("rev-parse", "HEAD"),
		GitStatusShort:          gitOutput("status", "--short"),
		Target:                  cfg.target,
		MessageTLSEnabled:       cfg.messageTLS.Enabled(),
		VerifiedAuthMetadata:    cfg.verifiedMetadata,
		ResultDir:               cfg.resultDir,
		Scenario:                cfg.scenario,
		Action:                  cfg.action,
		TenantID:                cfg.tenantID,
		UserID:                  cfg.userID,
		ChangeUserID:            cfg.changeUserID,
		ConversationID:          cfg.conversationID,
		StartedAt:               started,
		ExpectedPermissionVer:   cfg.expectedPermissionVer,
		ExpectedClassification:  cfg.expectedClassification,
		ExpectedReason:          cfg.expectedReason,
		PolicyRuleSeeded:        cfg.seedPolicyRule,
		TenantPolicyRuleSeeded:  cfg.seedTenantPolicyRule,
		ConversationRoleSeeded:  cfg.seedConversationRole,
		OwnershipOverrideSeeded: cfg.seedOwnershipOverride,
		PolicyAuditExpected:     shouldValidatePolicyAudit(cfg),
		ExpectedAuditRows:       expectedPolicyAuditRows(cfg),
	}
	if cfg.seedPolicyRule || cfg.seedTenantPolicyRule {
		s.PolicyRules = expectedPolicyRules(cfg, cfg.seedPolicyRule, cfg.seedTenantPolicyRule)
		if len(s.PolicyRules) > 0 {
			s.PolicyRule = s.PolicyRules[len(s.PolicyRules)-1]
		}
	}
	if cfg.seedConversationRole {
		s.ConversationRoleRule = expectedRoleRule(cfg)
		s.ConversationMember = expectedConversationMember(cfg)
	}
	if cfg.seedOwnershipOverride {
		s.OwnershipOverrideRule = expectedOwnershipOverrideRule(cfg)
		s.ConversationMember = expectedConversationMember(cfg)
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
		if shouldValidatePolicyAudit(cfg) {
			audit, err := readPolicyAudit(ctx, pool, cfg)
			if err != nil {
				return err
			}
			if err := validatePolicyAudit(cfg, audit); err != nil {
				return err
			}
			s.PolicyAudit = audit
		}
		return nil
	}

	errorSummary, err := validateDeny(callErr)
	if err != nil {
		return err
	}
	s.SendMessage = sendSummary{GRPCCode: codes.PermissionDenied.String()}
	s.MessageError = errorSummary
	if shouldValidatePolicyAudit(cfg) {
		audit, err := readPolicyAudit(ctx, pool, cfg)
		if err != nil {
			return err
		}
		if err := validatePolicyAudit(cfg, audit); err != nil {
			return err
		}
		s.PolicyAudit = audit
	}
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
		if shouldValidatePolicyAudit(cfg) {
			audit, err := readPolicyAudit(ctx, pool, cfg)
			if err != nil {
				return err
			}
			if err := validatePolicyAudit(cfg, audit); err != nil {
				return err
			}
			s.PolicyAudit = audit
		}
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
	if shouldValidatePolicyAudit(cfg) {
		audit, err := readPolicyAudit(ctx, pool, cfg)
		if err != nil {
			return err
		}
		if err := validatePolicyAudit(cfg, audit); err != nil {
			return err
		}
		s.PolicyAudit = audit
	}
	return nil
}

func sendMessage(cfg config) (*messagev1.SendMessageResponse, error, float64) {
	conn, err := dialMessageService(cfg)
	if err != nil {
		return nil, err, 0
	}
	defer conn.Close()

	payload, err := structpb.NewStruct(map[string]any{"text": "policy integration smoke"})
	if err != nil {
		return nil, err, 0
	}
	ctx, cancel := context.WithTimeout(context.Background(), cfg.requestTimeout)
	defer cancel()
	auth := sendAuth(cfg, "policy-message-smoke-"+cfg.scenario)
	ctx = withVerifiedAuthMetadata(ctx, cfg, auth)
	started := time.Now()
	response, err := messagev1.NewMessageServiceClient(conn).SendMessage(ctx, &messagev1.SendMessageRequest{
		AuthContext:    messageAuth(auth),
		ConversationId: cfg.conversationID,
		ClientMsgId:    cfg.clientMsgID,
		MessageType:    "TEXT",
		Payload:        payload,
	})
	return response, err, float64(time.Since(started).Microseconds()) / 1000.0
}

func changeMessage(cfg config, messageID string) (*messagev1.MessageChangeResponse, error, float64) {
	conn, err := dialMessageService(cfg)
	if err != nil {
		return nil, err, 0
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
		auth := changeAuth(cfg, "policy-message-edit-"+cfg.scenario)
		callCtx := withVerifiedAuthMetadata(ctx, cfg, auth)
		response, err = client.EditMessage(callCtx, &messagev1.EditMessageRequest{
			AuthContext:    messageAuth(auth),
			ConversationId: cfg.conversationID,
			MessageId:      messageID,
			IdempotencyKey: "policy-message-edit-" + cfg.scenario,
			Payload:        payload,
			Reason:         "policy integration smoke",
		})
		return response, err, float64(time.Since(started).Microseconds()) / 1000.0
	case "revoke":
		auth := changeAuth(cfg, "policy-message-revoke-"+cfg.scenario)
		callCtx := withVerifiedAuthMetadata(ctx, cfg, auth)
		response, err = client.RevokeMessage(callCtx, &messagev1.RevokeMessageRequest{
			AuthContext:    messageAuth(auth),
			ConversationId: cfg.conversationID,
			MessageId:      messageID,
			IdempotencyKey: "policy-message-revoke-" + cfg.scenario,
			Reason:         "policy integration smoke",
		})
		return response, err, float64(time.Since(started).Microseconds()) / 1000.0
	case "delete":
		auth := changeAuth(cfg, "policy-message-delete-"+cfg.scenario)
		callCtx := withVerifiedAuthMetadata(ctx, cfg, auth)
		response, err = client.DeleteMessage(callCtx, &messagev1.DeleteMessageRequest{
			AuthContext:    messageAuth(auth),
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

func dialMessageService(cfg config) (*grpc.ClientConn, error) {
	dialOption, err := grpctls.DialOption(cfg.messageTLS, "message-tls")
	if err != nil {
		return nil, fmt.Errorf("configure message-service TLS: %w", err)
	}
	conn, err := grpc.NewClient(cfg.target, dialOption)
	if err != nil {
		return nil, fmt.Errorf("dial message-service: %w", err)
	}
	return conn, nil
}

func authContext(cfg config, requestID string) *messagev1.AuthContext {
	return messageAuth(changeAuth(cfg, requestID))
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
	expectedClassification := expectedBaseSendClassification(cfg)
	if row.MessageClassification != expectedClassification || row.TimelineClassification != expectedClassification {
		return fmt.Errorf("base SendMessage classification mismatch row=%+v expected %s", row, expectedClassification)
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

func validatePolicyAudit(cfg config, audit policyAudit) error {
	expectedAllowed := cfg.scenario == "allow"
	expectedClassification := cfg.expectedClassification
	expectedReasonCode := ""
	if !expectedAllowed {
		if cfg.seedConversationRole {
			expectedClassification = expectedRoleRule(cfg).Classification
		}
		expectedReasonCode = expectedClassification
	}
	if expectedRows := expectedPolicyAuditRows(cfg); expectedRows > 0 && audit.RowCount != expectedRows {
		return fmt.Errorf("policy audit row count=%d expected %d", audit.RowCount, expectedRows)
	}
	expectedAction := strings.ToUpper(cfg.action)
	if audit.Action != expectedAction {
		return fmt.Errorf("policy audit action=%q expected %q", audit.Action, expectedAction)
	}
	if cfg.action == "send" {
		if audit.MessageIDPresent || audit.MessageKeyPresent {
			return fmt.Errorf("policy audit send message context present=%v key_present=%v expected false", audit.MessageIDPresent, audit.MessageKeyPresent)
		}
	} else if !audit.MessageIDPresent || !audit.MessageKeyPresent {
		return fmt.Errorf("policy audit message context present=%v key_present=%v expected true", audit.MessageIDPresent, audit.MessageKeyPresent)
	}
	if audit.Allowed != expectedAllowed {
		return fmt.Errorf("policy audit allowed=%v expected %v", audit.Allowed, expectedAllowed)
	}
	if audit.PermissionVersion != cfg.expectedPermissionVer {
		return fmt.Errorf("policy audit permission_version=%d expected %d", audit.PermissionVersion, cfg.expectedPermissionVer)
	}
	if audit.Classification != expectedClassification {
		return fmt.Errorf("policy audit classification=%q expected %q", audit.Classification, expectedClassification)
	}
	if audit.ReasonCode != expectedReasonCode {
		return fmt.Errorf("policy audit reason_code=%q expected %q", audit.ReasonCode, expectedReasonCode)
	}
	if audit.Status != "PENDING" && audit.Status != "PUBLISHED" {
		return fmt.Errorf("policy audit status=%q expected PENDING or PUBLISHED", audit.Status)
	}
	return nil
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

func readPolicyAudit(ctx context.Context, pool *pgxpool.Pool, cfg config) (policyAudit, error) {
	var result policyAudit
	if err := pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM policy_decision_audit_outbox
WHERE tenant_id = $1
`, cfg.tenantID).Scan(&result.RowCount); err != nil {
		return result, fmt.Errorf("read policy audit count: %w", err)
	}
	if result.RowCount < 1 {
		return result, fmt.Errorf("policy audit row count=%d expected at least 1", result.RowCount)
	}
	if err := pool.QueryRow(ctx, `
SELECT
    event_id,
    action,
    message_id_present,
    message_key <> '',
    allowed,
    permission_version,
    classification,
    reason_code,
    status
FROM policy_decision_audit_outbox
WHERE tenant_id = $1
ORDER BY created_at DESC, id DESC
LIMIT 1
`, cfg.tenantID).Scan(
		&result.EventID,
		&result.Action,
		&result.MessageIDPresent,
		&result.MessageKeyPresent,
		&result.Allowed,
		&result.PermissionVersion,
		&result.Classification,
		&result.ReasonCode,
		&result.Status,
	); err != nil {
		return result, fmt.Errorf("read policy audit row: %w", err)
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

func cleanupPolicyAudit(ctx context.Context, pool *pgxpool.Pool, tenantID string) error {
	if _, err := pool.Exec(ctx, `DELETE FROM policy_decision_audit_outbox WHERE tenant_id = $1`, tenantID); err != nil {
		return fmt.Errorf("cleanup policy decision audit outbox: %w", err)
	}
	return nil
}

func seedPolicyRules(ctx context.Context, pool *pgxpool.Pool, cfg config) error {
	if err := cleanupPolicyAudit(ctx, pool, cfg.tenantID); err != nil {
		return err
	}
	if _, err := pool.Exec(ctx, `DELETE FROM policy_message_action_rules WHERE tenant_id = $1`, cfg.tenantID); err != nil {
		return fmt.Errorf("cleanup policy rules: %w", err)
	}
	for _, rule := range expectedPolicyRules(cfg, true, false) {
		if err := seedOnePolicyRule(ctx, pool, rule); err != nil {
			return err
		}
	}
	return nil
}

func seedTenantPolicyRules(ctx context.Context, pool *pgxpool.Pool, cfg config) error {
	if err := cleanupPolicyAudit(ctx, pool, cfg.tenantID); err != nil {
		return err
	}
	if _, err := pool.Exec(ctx, `DELETE FROM policy_tenant_message_action_rules WHERE tenant_id = $1`, cfg.tenantID); err != nil {
		return fmt.Errorf("cleanup tenant policy rules: %w", err)
	}
	for _, rule := range expectedPolicyRules(cfg, false, true) {
		if err := seedOneTenantPolicyRule(ctx, pool, rule); err != nil {
			return err
		}
	}
	return nil
}

func seedConversationRoleGate(ctx context.Context, pool *pgxpool.Pool, cfg config) error {
	if cfg.action != "send" {
		return fmt.Errorf("conversation role gate integration smoke currently supports send only")
	}
	if err := cleanupPolicyAudit(ctx, pool, cfg.tenantID); err != nil {
		return err
	}
	if _, err := pool.Exec(ctx, `DELETE FROM policy_conversation_role_action_rules WHERE tenant_id = $1`, cfg.tenantID); err != nil {
		return fmt.Errorf("cleanup conversation role rules: %w", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM policy_conversation_members_projection WHERE tenant_id = $1`, cfg.tenantID); err != nil {
		return fmt.Errorf("cleanup conversation member projection: %w", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM policy_tenant_message_action_rules WHERE tenant_id = $1`, cfg.tenantID); err != nil {
		return fmt.Errorf("cleanup tenant policy rules: %w", err)
	}
	rule := expectedRoleRule(cfg)
	if _, err := pool.Exec(ctx, `
INSERT INTO policy_conversation_role_action_rules (
    tenant_id,
    action,
    min_role,
    classification,
    reason,
    source
) VALUES ($1, $2, $3, $4, $5, 'policy-message-role-smoke')
ON CONFLICT (tenant_id, action) DO UPDATE
SET min_role = EXCLUDED.min_role,
    classification = EXCLUDED.classification,
    reason = EXCLUDED.reason,
    source = EXCLUDED.source,
    updated_at = now()
`, rule.TenantID, rule.Action, rule.MinRole, rule.Classification, rule.Reason); err != nil {
		return fmt.Errorf("seed conversation role rule: %w", err)
	}
	member := expectedConversationMember(cfg)
	if _, err := pool.Exec(ctx, `
INSERT INTO policy_conversation_members_projection (
    tenant_id,
    conversation_id,
    user_id,
    role,
    status,
    member_version,
    permission_version,
    updated_by_event_id
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (tenant_id, conversation_id, user_id) DO UPDATE
SET role = EXCLUDED.role,
    status = EXCLUDED.status,
    member_version = EXCLUDED.member_version,
    permission_version = EXCLUDED.permission_version,
    updated_by_event_id = EXCLUDED.updated_by_event_id,
    updated_at = now()
`, member.TenantID, member.ConversationID, member.UserID, member.Role, member.Status, member.MemberVersion, member.PermissionVersion, member.UpdatedByEventID); err != nil {
		return fmt.Errorf("seed conversation member projection: %w", err)
	}
	allowRule := tenantPolicyRule(cfg, "SEND", true, cfg.expectedClassification, "")
	if cfg.scenario == "deny" {
		allowRule.Classification = "ROLE_GATE_TENANT_ALLOW_SHOULD_NOT_APPEAR"
		allowRule.Reason = ""
	}
	if err := seedOneTenantPolicyRule(ctx, pool, allowRule); err != nil {
		return err
	}
	return nil
}

func seedOwnershipOverrideRule(ctx context.Context, pool *pgxpool.Pool, cfg config) error {
	if cfg.action == "send" {
		return fmt.Errorf("ownership override integration smoke supports edit/revoke/delete only")
	}
	if err := cleanupPolicyAudit(ctx, pool, cfg.tenantID); err != nil {
		return err
	}
	if _, err := pool.Exec(ctx, `DELETE FROM policy_message_ownership_override_rules WHERE tenant_id = $1`, cfg.tenantID); err != nil {
		return fmt.Errorf("cleanup ownership override rules: %w", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM policy_conversation_members_projection WHERE tenant_id = $1`, cfg.tenantID); err != nil {
		return fmt.Errorf("cleanup conversation member projection: %w", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM policy_tenant_message_action_rules WHERE tenant_id = $1`, cfg.tenantID); err != nil {
		return fmt.Errorf("cleanup tenant policy rules: %w", err)
	}
	rule := expectedOwnershipOverrideRule(cfg)
	if _, err := pool.Exec(ctx, `
INSERT INTO policy_message_ownership_override_rules (
    tenant_id,
    action,
    min_role,
    classification,
    reason,
    source
) VALUES ($1, $2, $3, $4, $5, 'policy-message-ownership-override-smoke')
ON CONFLICT (tenant_id, action) DO UPDATE
SET min_role = EXCLUDED.min_role,
    classification = EXCLUDED.classification,
    reason = EXCLUDED.reason,
    source = EXCLUDED.source,
    updated_at = now()
`, rule.TenantID, rule.Action, rule.MinRole, rule.Classification, rule.Reason); err != nil {
		return fmt.Errorf("seed ownership override rule: %w", err)
	}
	member := expectedConversationMember(cfg)
	if _, err := pool.Exec(ctx, `
INSERT INTO policy_conversation_members_projection (
    tenant_id,
    conversation_id,
    user_id,
    role,
    status,
    member_version,
    permission_version,
    updated_by_event_id
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (tenant_id, conversation_id, user_id) DO UPDATE
SET role = EXCLUDED.role,
    status = EXCLUDED.status,
    member_version = EXCLUDED.member_version,
    permission_version = EXCLUDED.permission_version,
    updated_by_event_id = EXCLUDED.updated_by_event_id,
    updated_at = now()
`, member.TenantID, member.ConversationID, member.UserID, member.Role, member.Status, member.MemberVersion, member.PermissionVersion, member.UpdatedByEventID); err != nil {
		return fmt.Errorf("seed conversation member projection: %w", err)
	}
	allowRule := tenantPolicyRule(cfg, "SEND", true, "POLICY_SEND_SEED", "")
	if err := seedOneTenantPolicyRule(ctx, pool, allowRule); err != nil {
		return err
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

func seedOneTenantPolicyRule(ctx context.Context, pool *pgxpool.Pool, rule policyRule) error {
	_, err := pool.Exec(ctx, `
INSERT INTO policy_tenant_message_action_rules (
    tenant_id,
    action,
    allowed,
    permission_version,
    classification,
    reason,
    source
) VALUES ($1, $2, $3, $4, $5, $6, 'loadtest')
ON CONFLICT (tenant_id, action) DO UPDATE
SET allowed = EXCLUDED.allowed,
    permission_version = EXCLUDED.permission_version,
    classification = EXCLUDED.classification,
    reason = EXCLUDED.reason,
    source = EXCLUDED.source,
    updated_at = now()
`, rule.TenantID, rule.Action, rule.Allowed, rule.PermissionVersion, rule.Classification, rule.Reason)
	if err != nil {
		return fmt.Errorf("seed tenant policy rule: %w", err)
	}
	return nil
}

func expectedPolicyRules(cfg config, includeExact bool, includeTenant bool) []policyRule {
	rules := make([]policyRule, 0, 4)
	if includeTenant && cfg.action != "send" {
		rules = append(rules, tenantPolicyRule(cfg, "SEND", true, "POLICY_SEND_SEED", ""))
	}
	if includeExact && cfg.action != "send" {
		rules = append(rules, exactPolicyRule(cfg, "SEND", true, "POLICY_SEND_SEED", ""))
	}
	allowed := cfg.scenario == "allow"
	reason := cfg.expectedReason
	if !allowed && strings.TrimSpace(reason) == "" {
		reason = "policy denied"
	}
	action := strings.ToUpper(cfg.action)
	if includeTenant {
		rules = append(rules, tenantPolicyRule(cfg, action, allowed, cfg.expectedClassification, reason))
	}
	if includeExact {
		rules = append(rules, exactPolicyRule(cfg, action, allowed, cfg.expectedClassification, reason))
	}
	return rules
}

func expectedRoleRule(cfg config) roleRule {
	return roleRule{
		TenantID:       cfg.tenantID,
		Action:         strings.ToUpper(cfg.action),
		MinRole:        "ADMIN",
		Classification: "CONVERSATION_ROLE_DENIED",
		Reason:         "conversation role policy denied",
	}
}

func expectedOwnershipOverrideRule(cfg config) roleRule {
	return roleRule{
		TenantID:       cfg.tenantID,
		Action:         strings.ToUpper(cfg.action),
		MinRole:        "ADMIN",
		Classification: "MESSAGE_OWNERSHIP_ROLE_OVERRIDE",
		Reason:         "",
	}
}

func expectedConversationMember(cfg config) memberRow {
	role := "ADMIN"
	if cfg.scenario == "deny" {
		role = "MEMBER"
	}
	return memberRow{
		TenantID:          cfg.tenantID,
		ConversationID:    cfg.conversationID,
		UserID:            expectedConversationMemberUserID(cfg),
		Role:              role,
		Status:            "ACTIVE",
		MemberVersion:     cfg.expectedPermissionVer,
		PermissionVersion: cfg.expectedPermissionVer,
		UpdatedByEventID:  expectedConversationMemberEventID(cfg),
	}
}

func expectedConversationMemberUserID(cfg config) string {
	if cfg.seedOwnershipOverride {
		return cfg.changeUserID
	}
	return cfg.userID
}

func expectedConversationMemberEventID(cfg config) string {
	if cfg.seedOwnershipOverride {
		return "policy-message-ownership-override-smoke-" + cfg.scenario
	}
	return "policy-message-role-smoke-" + cfg.scenario
}

func exactPolicyRule(cfg config, action string, allowed bool, classification string, reason string) policyRule {
	rule := tenantPolicyRule(cfg, action, allowed, classification, reason)
	rule.UserID = cfg.userID
	if action != "SEND" {
		rule.UserID = cfg.changeUserID
	}
	rule.ConversationID = cfg.conversationID
	return rule
}

func tenantPolicyRule(cfg config, action string, allowed bool, classification string, reason string) policyRule {
	return policyRule{
		TenantID:          cfg.tenantID,
		Action:            action,
		Allowed:           allowed,
		PermissionVersion: cfg.expectedPermissionVer,
		Classification:    classification,
		Reason:            reason,
	}
}

func isSupportedAction(action string) bool {
	switch action {
	case "send", "edit", "revoke", "delete":
		return true
	default:
		return false
	}
}

func shouldValidatePolicyAudit(cfg config) bool {
	return cfg.expectPolicyAudit || cfg.seedConversationRole || cfg.seedOwnershipOverride
}

func expectedPolicyAuditRows(cfg config) int64 {
	if !shouldValidatePolicyAudit(cfg) {
		return 0
	}
	if cfg.expectedAuditRows > 0 {
		return cfg.expectedAuditRows
	}
	return 1
}

func expectedBaseSendClassification(cfg config) string {
	if strings.TrimSpace(cfg.expectedBaseClass) != "" {
		return strings.TrimSpace(cfg.expectedBaseClass)
	}
	if cfg.action != "send" && (cfg.seedPolicyRule || cfg.seedTenantPolicyRule || cfg.seedConversationRole || cfg.seedOwnershipOverride) {
		return "POLICY_SEND_SEED"
	}
	return cfg.expectedClassification
}
