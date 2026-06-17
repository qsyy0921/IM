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
