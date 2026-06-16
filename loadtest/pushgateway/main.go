package main

import (
	"context"
	"crypto/pbkdf2"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	conversationv1 "github.com/qsyy0921/IM/api/proto/nexusim/conversation/v1"
	deliveryv1 "github.com/qsyy0921/IM/api/proto/nexusim/delivery/v1"
	messagev1 "github.com/qsyy0921/IM/api/proto/nexusim/message/v1"
	"google.golang.org/protobuf/types/known/structpb"
	nhooyr "nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

func main() {
	cfg := parseConfig()
	if err := run(cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(cfg config) error {
	if cfg.pgDSN == "" {
		return errors.New("pg-dsn is required")
	}
	if cfg.pushAuthMode == "hmac" && strings.TrimSpace(cfg.pushAuthHMACSecret) == "" {
		return fmt.Errorf("--push-auth-hmac-secret is required when --push-auth-mode=hmac")
	}
	if cfg.pushAuthMode == "jwt" && strings.TrimSpace(cfg.identityTarget) == "" {
		return fmt.Errorf("--identity-target is required when --push-auth-mode=jwt")
	}
	if strings.TrimSpace(cfg.identityTarget) != "" && cfg.identityTokenMethod != "issue_gateway_token" && cfg.identityTokenMethod != "login" && cfg.identityTokenMethod != "register_login" {
		return fmt.Errorf("--identity-token-method must be issue_gateway_token, login, or register_login")
	}
	if strings.TrimSpace(cfg.identityTarget) != "" && (cfg.identityTokenMethod == "login" || cfg.identityTokenMethod == "register_login") && strings.TrimSpace(cfg.identityLoginPassword) == "" {
		return fmt.Errorf("--identity-login-password is required when --identity-token-method=login or register_login")
	}
	if cfg.pushAuthTokenTTL <= 0 {
		return fmt.Errorf("--push-auth-token-ttl must be positive")
	}
	if cfg.scenario == "cross-instance-resume" || cfg.scenario == "redis-sentinel-failover" || cfg.scenario == "redis-sentinel-master-stop" {
		if cfg.routeBackend != "redis" {
			return fmt.Errorf("%s scenario requires --route-backend redis", cfg.scenario)
		}
		if cfg.reconnectPushURL == "" || cfg.reconnectPushURL == cfg.pushURL {
			return fmt.Errorf("%s scenario requires --reconnect-push-url to point at a different gateway", cfg.scenario)
		}
	}
	if cfg.scenario == "redis-resume-negative" && cfg.routeBackend != "redis" {
		return fmt.Errorf("%s scenario requires --route-backend redis", cfg.scenario)
	}
	if err := os.MkdirAll(cfg.resultDir, 0o755); err != nil {
		return fmt.Errorf("create result dir: %w", err)
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, cfg.pgDSN)
	if err != nil {
		return fmt.Errorf("open pg pool: %w", err)
	}
	defer pool.Close()
	if cfg.cleanup {
		if err := cleanupTenant(ctx, pool, cfg.tenantID); err != nil {
			return err
		}
	}
	if err := seedConversation(ctx, pool, cfg); err != nil {
		return err
	}
	if strings.TrimSpace(cfg.identityTarget) != "" && cfg.identityTokenMethod == "register_login" {
		if err := registerIdentityCredential(ctx, cfg); err != nil {
			return err
		}
	}
	if strings.TrimSpace(cfg.identityTarget) != "" && cfg.identityTokenMethod == "login" {
		if err := seedIdentityCredential(ctx, pool, cfg); err != nil {
			return err
		}
	}

	conversationConn, err := dialGRPCService(cfg.conversationTarget, cfg.conversationTLS, "conversation-tls", "conversation-service")
	if err != nil {
		return err
	}
	defer conversationConn.Close()
	conversationClient := conversationv1.NewConversationServiceClient(conversationConn)

	messageConn, err := dialGRPCService(cfg.messageTarget, cfg.messageTLS, "message-tls", "message-service")
	if err != nil {
		return err
	}
	defer messageConn.Close()
	messageClient := messagev1.NewMessageServiceClient(messageConn)

	deliveryConn, err := dialGRPCService(cfg.deliveryTarget, cfg.deliveryTLS, "delivery-tls", "delivery-service")
	if err != nil {
		return err
	}
	defer deliveryConn.Close()
	deliveryClient := deliveryv1.NewDeliveryServiceClient(deliveryConn)

	result := summary{
		Commit:                                  shortCommit(),
		CommitFull:                              fullCommit(),
		GitDirty:                                gitDirty(),
		GitStatusShort:                          gitStatusShort(),
		ConversationTarget:                      cfg.conversationTarget,
		MessageTarget:                           cfg.messageTarget,
		DeliveryTarget:                          cfg.deliveryTarget,
		IdentityTarget:                          cfg.identityTarget,
		ConversationTLSEnabled:                  cfg.conversationTLS.Enabled(),
		MessageTLSEnabled:                       cfg.messageTLS.Enabled(),
		DeliveryTLSEnabled:                      cfg.deliveryTLS.Enabled(),
		IdentityTLSEnabled:                      cfg.identityTLS.Enabled(),
		PushTLSEnabled:                          cfg.pushTLS.Enabled(),
		VerifiedAuthMetadata:                    cfg.verifiedAuthMetadata,
		PushURL:                                 cfg.pushURL,
		ReconnectPushURL:                        cfg.reconnectPushURL,
		PushMetricsURL:                          cfg.pushMetricsURL,
		ReconnectPushMetricsURL:                 cfg.reconnectMetricsURL,
		PushConsumerMetricsURL:                  cfg.consumerMetricsURL,
		RouteBackend:                            cfg.routeBackend,
		PushAuthMode:                            cfg.pushAuthMode,
		PushAuthTokenTransport:                  pushAuthTokenTransport(cfg),
		PushAuthTokenSource:                     pushAuthTokenSource(cfg),
		IdentityGatewayTokenFormat:              identityGatewayTokenFormat(cfg),
		IdentityTokenMethod:                     identityTokenMethod(cfg),
		PushAuthTokenTTLSeconds:                 int64(cfg.pushAuthTokenTTL.Seconds()),
		PushAuthSecretConfigured:                strings.TrimSpace(cfg.pushAuthHMACSecret) != "",
		PushAuthPreviousSecretsConfigured:       strings.TrimSpace(cfg.pushAuthHMACPreviousSecrets) != "",
		PushAuthTokenSigningSecretExplicit:      cfg.pushAuthTokenSigningSecretExplicit,
		PushAuthTokenSignedWithNonCurrentSecret: pushAuthTokenSignedWithNonCurrentSecret(cfg),
		PushAuthQueryIdentitySent:               pushAuthQueryIdentitySent(cfg),
		RedisKeyPrefix:                          cfg.redisKeyPrefix,
		PushWSGatewayID:                         cfg.pushWSGatewayID,
		PushReconnectGatewayID:                  cfg.pushReconnectGatewayID,
		PushConsumerGatewayID:                   cfg.pushConsumerGatewayID,
		Scenario:                                cfg.scenario,
		TenantID:                                cfg.tenantID,
		ConversationID:                          cfg.conversationID,
		OwnerUserID:                             cfg.ownerUserID,
		ReceiverUserID:                          cfg.receiverUserID,
		ReceiverDeviceID:                        cfg.receiverDeviceID,
		ReceiverDeviceIDs:                       cfg.receiverDeviceIDs,
		StartedAt:                               time.Now().UTC(),
		Latencies:                               map[string]float64{},
	}

	if metrics, err := fetchPushMetrics(ctx, cfg.pushMetricsURL); err == nil {
		result.PushMetricsBefore = &metrics
	}

	switch cfg.scenario {
	case "full":
	case "message-change-notify":
		return runMessageChangeNotifyScenario(ctx, cfg, pool, conversationClient, messageClient, deliveryClient, &result)
	case "resume-replay":
		return runResumeReplayScenario(ctx, cfg, pool, conversationClient, messageClient, deliveryClient, &result)
	case "redis-resume-negative":
		return runRedisResumeNegativeScenario(ctx, cfg, pool, conversationClient, messageClient, deliveryClient, &result)
	case "cross-instance-resume":
		return runResumeReplayScenario(ctx, cfg, pool, conversationClient, messageClient, deliveryClient, &result)
	case "redis-sentinel-failover":
		return runResumeReplayScenario(ctx, cfg, pool, conversationClient, messageClient, deliveryClient, &result)
	case "redis-sentinel-master-stop":
		return runResumeReplayScenario(ctx, cfg, pool, conversationClient, messageClient, deliveryClient, &result)
	case "redis-sentinel-quorum-loss":
		return runRedisFaultScenario(ctx, cfg, pool, conversationClient, messageClient, deliveryClient, &result)
	case "redis-sentinel-network-partition":
		return runRedisFaultScenario(ctx, cfg, pool, conversationClient, messageClient, deliveryClient, &result)
	case "slow-client":
		return runSlowClientScenario(ctx, cfg, pool, conversationClient, messageClient, deliveryClient, &result)
	case "redis-fault":
		return runRedisFaultScenario(ctx, cfg, pool, conversationClient, messageClient, deliveryClient, &result)
	case "redis-cluster-node-stop":
		return runRedisFaultScenario(ctx, cfg, pool, conversationClient, messageClient, deliveryClient, &result)
	case "identity-revoke":
		return runIdentityRevokeScenario(ctx, cfg, &result)
	default:
		return finish(cfg, &result, fmt.Errorf("unsupported scenario: %s", cfg.scenario))
	}

	type onlineDevice struct {
		deviceID string
		conn     *nhooyr.Conn
	}
	devices := make([]onlineDevice, 0, len(cfg.receiverDeviceIDs))
	for _, deviceID := range cfg.receiverDeviceIDs {
		conn, hello, err := connectWebSocket(ctx, cfg, deviceID)
		if err != nil {
			return finish(cfg, &result, fmt.Errorf("connect websocket %s: %w", deviceID, err))
		}
		defer conn.Close(nhooyr.StatusNormalClosure, "")
		devices = append(devices, onlineDevice{deviceID: deviceID, conn: conn})
		deviceResult := deviceSummary{DeviceID: deviceID, ServerHello: snapshotFrame(hello)}
		result.DeviceNotifications = append(result.DeviceNotifications, deviceResult)
	}
	result.ServerHello = result.DeviceNotifications[0].ServerHello

	begin := time.Now()
	join, err := createReceiverJoin(ctx, cfg, conversationClient)
	result.Latencies["create_member_join"] = elapsedMS(begin)
	if err != nil {
		return finish(cfg, &result, fmt.Errorf("create receiver join: %w", err))
	}
	result.MemberJoin = memberJoinSummary{
		ChangeID:          join.GetChangeId(),
		BoundarySeq:       join.GetBoundarySeq(),
		MemberVersion:     join.GetMemberVersion(),
		PermissionVersion: join.GetPermissionVersion(),
	}
	if err := waitMembership(ctx, pool, cfg); err != nil {
		return finish(cfg, &result, err)
	}

	begin = time.Now()
	send, err := sendMessage(ctx, cfg, messageClient, 1)
	result.Latencies["send_message"] = elapsedMS(begin)
	if err != nil {
		return finish(cfg, &result, fmt.Errorf("send message: %w", err))
	}
	result.SendMessage = sendSummary{MessageID: send.GetMessageId(), ConversationSeq: send.GetConversationSeq()}

	for i, device := range devices {
		notify, err := waitNotify(ctx, cfg, device.conn)
		if err != nil {
			return finish(cfg, &result, fmt.Errorf("wait notify %s: %w", device.deviceID, err))
		}
		result.DeviceNotifications[i].DeliveryNotify = snapshotFrame(notify)
		if notify.ConversationSeq != send.GetConversationSeq() || notify.MessageID != send.GetMessageId() {
			return finish(cfg, &result, fmt.Errorf("notify mismatch for %s: notify=%+v send=%+v", device.deviceID, notify, send))
		}
	}
	result.DeliveryNotify = result.DeviceNotifications[0].DeliveryNotify

	pull, err := pullInbox(ctx, cfg, deliveryClient)
	if err != nil {
		return finish(cfg, &result, fmt.Errorf("pull inbox: %w", err))
	}
	result.PullInbox = pull
	if pull.ItemCount == 0 || pull.MaxSeq < send.GetConversationSeq() {
		return finish(cfg, &result, fmt.Errorf("pull inbox did not include notify seq: %+v", pull))
	}

	for i, device := range devices {
		ackOK, err := ackViaWebSocket(ctx, cfg, device.conn, device.deviceID, send.GetConversationSeq())
		if err != nil {
			return finish(cfg, &result, fmt.Errorf("websocket ack %s: %w", device.deviceID, err))
		}
		result.DeviceNotifications[i].DeliveryAckOK = snapshotFrame(ackOK)
		if ackOK.LastReceivedSeq != send.GetConversationSeq() {
			return finish(cfg, &result, fmt.Errorf("ack seq mismatch for %s: %+v", device.deviceID, ackOK))
		}
		if err := waitCursor(ctx, pool, cfg, device.deviceID, send.GetConversationSeq()); err != nil {
			return finish(cfg, &result, err)
		}
		cursor, err := queryCursor(ctx, pool, cfg, device.deviceID)
		if err != nil {
			return finish(cfg, &result, err)
		}
		result.DeviceNotifications[i].CursorLastReceivedSeq = &cursor
	}
	result.DeliveryAckOK = result.DeviceNotifications[0].DeliveryAckOK
	if err := waitDeliveryOutboxDrain(ctx, pool, cfg); err != nil {
		return finish(cfg, &result, err)
	}
	if err := fillPostgresStats(ctx, pool, cfg, &result); err != nil {
		return finish(cfg, &result, err)
	}
	result.Success = true
	return finish(cfg, &result, nil)
}

func createReceiverJoin(
	ctx context.Context,
	cfg config,
	client conversationv1.ConversationServiceClient,
) (*conversationv1.CreateMemberChangeResponse, error) {
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	auth := ownerAuth(cfg, "push-smoke-join", "push-smoke-join")
	requestCtx = withVerifiedAuthMetadata(requestCtx, cfg, auth)
	return client.CreateMemberChange(requestCtx, &conversationv1.CreateMemberChangeRequest{
		AuthContext:           conversationAuth(auth),
		ConversationId:        cfg.conversationID,
		TargetUserId:          cfg.receiverUserID,
		ChangeType:            conversationv1.MemberChangeType_MEMBER_CHANGE_TYPE_JOIN,
		TargetRole:            conversationv1.MemberRole_MEMBER_ROLE_MEMBER,
		ExpectedMemberVersion: 1,
		IdempotencyKey:        "push-smoke-join-receiver",
		ConflictPolicy:        conversationv1.MemberChangeConflictPolicy_MEMBER_CHANGE_CONFLICT_POLICY_REJECT,
		Reason:                "push gateway smoke receiver join",
	})
}

func sendMessage(
	ctx context.Context,
	cfg config,
	client messagev1.MessageServiceClient,
	index int,
) (*messagev1.SendMessageResponse, error) {
	payload, err := structpb.NewStruct(map[string]any{"text": fmt.Sprintf("push gateway smoke %d", index)})
	if err != nil {
		return nil, err
	}
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	auth := ownerAuth(cfg, "push-smoke-send", "push-smoke-send")
	requestCtx = withVerifiedAuthMetadata(requestCtx, cfg, auth)
	return client.SendMessage(requestCtx, &messagev1.SendMessageRequest{
		AuthContext:    messageAuth(auth),
		ConversationId: cfg.conversationID,
		ClientMsgId:    fmt.Sprintf("push-smoke-client-message-%d", index),
		MessageType:    "TEXT",
		Payload:        payload,
	})
}

func changeMessage(
	ctx context.Context,
	cfg config,
	client messagev1.MessageServiceClient,
	messageID string,
) (*messagev1.MessageChangeResponse, error) {
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	auth := ownerAuth(cfg, "push-smoke-message-change", "push-smoke-message-change")
	requestCtx = withVerifiedAuthMetadata(requestCtx, cfg, auth)
	switch cfg.messageChangeAction {
	case "edit":
		payload, err := structpb.NewStruct(map[string]any{"text": "push gateway edited message"})
		if err != nil {
			return nil, err
		}
		return client.EditMessage(requestCtx, &messagev1.EditMessageRequest{
			AuthContext:    messageAuth(auth),
			ConversationId: cfg.conversationID,
			MessageId:      messageID,
			IdempotencyKey: "push-smoke-edit-1",
			Payload:        payload,
			Reason:         "push gateway message-change notify smoke",
		})
	case "revoke":
		return client.RevokeMessage(requestCtx, &messagev1.RevokeMessageRequest{
			AuthContext:    messageAuth(auth),
			ConversationId: cfg.conversationID,
			MessageId:      messageID,
			IdempotencyKey: "push-smoke-revoke-1",
			Reason:         "push gateway message-change notify smoke",
		})
	case "delete":
		return client.DeleteMessage(requestCtx, &messagev1.DeleteMessageRequest{
			AuthContext:    messageAuth(auth),
			ConversationId: cfg.conversationID,
			MessageId:      messageID,
			IdempotencyKey: "push-smoke-delete-1",
			DeleteScope:    messagev1.DeleteScope_DELETE_SCOPE_CONVERSATION_VIEW,
			Reason:         "push gateway message-change notify smoke",
		})
	default:
		return nil, fmt.Errorf("unsupported message-change-action: %s", cfg.messageChangeAction)
	}
}

func sourceEventTypeForAction(action string) (string, error) {
	switch action {
	case "edit":
		return "message.edited.v1", nil
	case "revoke":
		return "message.revoked.v1", nil
	case "delete":
		return "message.deleted.v1", nil
	default:
		return "", fmt.Errorf("unsupported message-change-action: %s", action)
	}
}

func waitNotify(ctx context.Context, cfg config, conn *nhooyr.Conn) (serverFrame, error) {
	return waitNotifyFor(ctx, cfg, conn, cfg.waitTimeout)
}

func waitNotifyFor(ctx context.Context, cfg config, conn *nhooyr.Conn, timeout time.Duration) (serverFrame, error) {
	readCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for {
		var frame serverFrame
		err := wsjson.Read(readCtx, conn, &frame)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return serverFrame{}, errors.New("notify timeout")
			}
			return serverFrame{}, err
		}
		if frame.Op == opError {
			return frame, fmt.Errorf("error frame: %+v", frame)
		}
		if frame.Op == opDeliveryNotify {
			return frame, nil
		}
	}
}

func executeCommand(ctx context.Context, cfg config, command string) (string, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return "", nil
	}
	runCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, "powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", command)
	output, err := cmd.CombinedOutput()
	trimmed := strings.TrimSpace(string(output))
	if err != nil {
		return trimmed, fmt.Errorf("%w: %s", err, trimmed)
	}
	return trimmed, nil
}

type slowReadResult struct {
	notifyFrames int
	resumeHint   serverFrame
	closeStatus  string
}

func readUntilResumeHintOrClose(ctx context.Context, cfg config, conn *nhooyr.Conn) slowReadResult {
	readCtx, cancel := context.WithTimeout(ctx, cfg.waitTimeout)
	defer cancel()
	result := slowReadResult{}
	for {
		var frame serverFrame
		err := wsjson.Read(readCtx, conn, &frame)
		if err != nil {
			status := nhooyr.CloseStatus(err)
			if status != -1 {
				result.closeStatus = status.String()
			} else {
				result.closeStatus = err.Error()
			}
			return result
		}
		switch frame.Op {
		case opDeliveryNotify:
			result.notifyFrames++
		case opResumeHint:
			result.resumeHint = frame
		}
	}
}

func pullInbox(ctx context.Context, cfg config, client deliveryv1.DeliveryServiceClient) (pullSummary, error) {
	return pullInboxWithLimit(ctx, cfg, client, 0, 100)
}

func pullInboxWithLimit(
	ctx context.Context,
	cfg config,
	client deliveryv1.DeliveryServiceClient,
	afterSeq int64,
	limit int,
) (pullSummary, error) {
	return pullInboxAtLeast(ctx, cfg, client, afterSeq, limit, 1, 0)
}

func pullInboxAtLeast(
	ctx context.Context,
	cfg config,
	client deliveryv1.DeliveryServiceClient,
	afterSeq int64,
	limit int,
	minItems int,
	minSeq int64,
) (pullSummary, error) {
	deadline := time.Now().Add(cfg.waitTimeout)
	latencies := make([]float64, 0, 8)
	if limit <= 0 {
		limit = 100
	}
	for {
		requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
		begin := time.Now()
		auth := receiverAuth(cfg, cfg.receiverDeviceID, "push-smoke-pull", "push-smoke-pull")
		requestCtx = withVerifiedAuthMetadata(requestCtx, cfg, auth)
		response, err := client.PullInbox(requestCtx, &deliveryv1.PullInboxRequest{
			AuthContext:    deliveryAuth(auth),
			ConversationId: cfg.conversationID,
			AfterSeq:       afterSeq,
			Limit:          int32(limit),
		})
		latencies = append(latencies, elapsedMS(begin))
		cancel()
		if err != nil {
			return pullSummary{}, err
		}
		if len(response.GetItems()) >= minItems || maxInboxSeq(response.GetItems()) >= minSeq || time.Now().After(deadline) {
			result := pullSummary{ItemCount: len(response.GetItems())}
			for _, inboxItem := range response.GetItems() {
				if inboxItem.GetConversationSeq() > result.MaxSeq {
					result.MaxSeq = inboxItem.GetConversationSeq()
				}
				result.Items = append(result.Items, item{
					ConversationSeq: inboxItem.GetConversationSeq(),
					EventID:         inboxItem.GetEventId(),
					EventType:       inboxItem.GetEventType(),
					MessageID:       inboxItem.GetMessageId(),
					SenderID:        inboxItem.GetSenderId(),
				})
			}
			sort.Slice(result.Items, func(i, j int) bool {
				return result.Items[i].ConversationSeq < result.Items[j].ConversationSeq
			})
			result.P95MS = percentile(latencies, 0.95)
			result.P99MS = percentile(latencies, 0.99)
			return result, nil
		}
		time.Sleep(cfg.pollInterval)
	}
}

func pullInboxUntilEvent(
	ctx context.Context,
	cfg config,
	client deliveryv1.DeliveryServiceClient,
	afterSeq int64,
	eventType string,
	messageID string,
	seq int64,
) (pullSummary, error) {
	deadline := time.Now().Add(cfg.waitTimeout)
	var last pullSummary
	for {
		pull, err := pullInboxAtLeast(ctx, cfg, client, afterSeq, 100, 0, seq)
		if err != nil {
			return pullSummary{}, err
		}
		last = pull
		for _, item := range pull.Items {
			if item.EventType == eventType &&
				item.MessageID == messageID &&
				item.ConversationSeq == seq {
				return pull, nil
			}
		}
		if time.Now().After(deadline) {
			return last, fmt.Errorf("pull inbox missing %s item for message %s seq %d: %+v", eventType, messageID, seq, last)
		}
		time.Sleep(cfg.pollInterval)
	}
}

func maxInboxSeq(items []*deliveryv1.InboxItem) int64 {
	var maxSeq int64
	for _, inboxItem := range items {
		if inboxItem.GetConversationSeq() > maxSeq {
			maxSeq = inboxItem.GetConversationSeq()
		}
	}
	return maxSeq
}

func ackViaWebSocket(
	ctx context.Context,
	cfg config,
	conn *nhooyr.Conn,
	deviceID string,
	seq int64,
) (serverFrame, error) {
	frame, _, err := ackViaWebSocketWithSkipped(ctx, cfg, conn, deviceID, seq)
	return frame, err
}

func ackViaWebSocketWithSkipped(
	ctx context.Context,
	cfg config,
	conn *nhooyr.Conn,
	deviceID string,
	seq int64,
) (serverFrame, int, error) {
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	if err := wsjson.Write(requestCtx, conn, clientFrame{
		Op:             opDeliveryAck,
		RequestID:      "push-smoke-ack-" + deviceID,
		ConversationID: cfg.conversationID,
		ReceivedSeq:    seq,
	}); err != nil {
		return serverFrame{}, 0, err
	}
	skipped := 0
	for {
		var frame serverFrame
		if err := wsjson.Read(requestCtx, conn, &frame); err != nil {
			return serverFrame{}, skipped, err
		}
		switch frame.Op {
		case opDeliveryAckOK:
			return frame, skipped, nil
		case opDeliveryNotify, opResumeHint:
			skipped++
			continue
		default:
			return frame, skipped, fmt.Errorf("unexpected ack frame: %+v", frame)
		}
	}
}

func waitMembership(ctx context.Context, pool *pgxpool.Pool, cfg config) error {
	deadline := time.Now().Add(cfg.waitTimeout)
	for time.Now().Before(deadline) {
		var count int
		err := pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM delivery_membership_projection
WHERE tenant_id = $1
  AND conversation_id = $2
  AND user_id = $3
  AND status = 'ACTIVE'
`, cfg.tenantID, cfg.conversationID, cfg.receiverUserID).Scan(&count)
		if err == nil && count > 0 {
			return nil
		}
		time.Sleep(cfg.pollInterval)
	}
	return errors.New("delivery membership projection timeout")
}

func waitCursor(ctx context.Context, pool *pgxpool.Pool, cfg config, deviceID string, seq int64) error {
	deadline := time.Now().Add(cfg.waitTimeout)
	for time.Now().Before(deadline) {
		current, err := queryCursor(ctx, pool, cfg, deviceID)
		if err == nil && current >= seq {
			return nil
		}
		time.Sleep(cfg.pollInterval)
	}
	return errors.New("delivery cursor timeout")
}

func queryCursor(ctx context.Context, pool *pgxpool.Pool, cfg config, deviceID string) (int64, error) {
	var current int64
	err := pool.QueryRow(ctx, `
SELECT COALESCE(MAX(last_received_seq), 0)
FROM device_delivery_cursors
WHERE tenant_id = $1
  AND conversation_id = $2
  AND user_id = $3
  AND device_id = $4
`, cfg.tenantID, cfg.conversationID, cfg.receiverUserID, deviceID).Scan(&current)
	return current, err
}

func waitDeliveryOutboxDrain(ctx context.Context, pool *pgxpool.Pool, cfg config) error {
	deadline := time.Now().Add(cfg.waitTimeout)
	for time.Now().Before(deadline) {
		var pending int64
		err := pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM delivery_outbox
WHERE tenant_id = $1
  AND conversation_id = $2
  AND status = 'PENDING'
`, cfg.tenantID, cfg.conversationID).Scan(&pending)
		if err == nil && pending == 0 {
			return nil
		}
		time.Sleep(cfg.pollInterval)
	}
	return errors.New("delivery outbox drain timeout")
}

func waitPushEviction(ctx context.Context, cfg config, previous uint64) (pushMetrics, error) {
	deadline := time.Now().Add(cfg.waitTimeout)
	var last pushMetrics
	var lastErr error
	for time.Now().Before(deadline) {
		metrics, err := fetchPushMetrics(ctx, cfg.pushMetricsURL)
		if err == nil {
			last = metrics
			if metrics.SlowSessionEvictedCount > previous {
				return metrics, nil
			}
		} else {
			lastErr = err
		}
		time.Sleep(cfg.pollInterval)
	}
	if lastErr != nil {
		return last, fmt.Errorf("wait push eviction: last metrics error: %w", lastErr)
	}
	return last, fmt.Errorf("wait push eviction timeout: metrics=%+v previous_evicted=%d", last, previous)
}

func fetchPushMetrics(ctx context.Context, metricsURL string) (pushMetrics, error) {
	if strings.TrimSpace(metricsURL) == "" {
		return pushMetrics{}, errors.New("push metrics url is empty")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, metricsURL, nil)
	if err != nil {
		return pushMetrics{}, err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return pushMetrics{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return pushMetrics{}, fmt.Errorf("push metrics returned status %d", response.StatusCode)
	}
	var metrics pushMetrics
	if err := json.NewDecoder(response.Body).Decode(&metrics); err != nil {
		return pushMetrics{}, err
	}
	return metrics, nil
}

func cleanupTenant(ctx context.Context, pool *pgxpool.Pool, tenantID string) error {
	statements := []string{
		`DELETE FROM delivery_outbox WHERE tenant_id = $1`,
		`DELETE FROM device_delivery_cursors WHERE tenant_id = $1`,
		`DELETE FROM user_inbox WHERE tenant_id = $1`,
		`DELETE FROM delivery_membership_projection WHERE tenant_id = $1`,
		`DELETE FROM delivery_kafka_checkpoints WHERE consumer_group LIKE $1`,
		`DELETE FROM message_outbox WHERE tenant_id = $1`,
		`DELETE FROM conversation_timeline_events WHERE tenant_id = $1`,
		`DELETE FROM message_log WHERE tenant_id = $1`,
		`DELETE FROM conversation_seq WHERE tenant_id = $1`,
		`DELETE FROM member_change_saga WHERE tenant_id = $1`,
		`DELETE FROM conversation_members WHERE tenant_id = $1`,
		`DELETE FROM conversations WHERE tenant_id = $1`,
	}
	for _, statement := range statements {
		arg := any(tenantID)
		if strings.Contains(statement, "consumer_group LIKE") {
			arg = "nexusim-%push-smoke%"
		}
		if _, err := pool.Exec(ctx, statement, arg); err != nil {
			return fmt.Errorf("cleanup tenant: %w", err)
		}
	}
	optionalStatements := []string{
		`DELETE FROM identity_outbox WHERE tenant_id = $1`,
		`DELETE FROM identity_refresh_tokens WHERE tenant_id = $1`,
		`DELETE FROM identity_sessions WHERE tenant_id = $1`,
		`DELETE FROM identity_devices WHERE tenant_id = $1`,
		`DELETE FROM identity_users WHERE tenant_id = $1`,
	}
	for _, statement := range optionalStatements {
		tableName := strings.Fields(strings.TrimPrefix(statement, "DELETE FROM "))[0]
		exists, err := tableExists(ctx, pool, tableName)
		if err != nil {
			return fmt.Errorf("check optional table %s: %w", tableName, err)
		}
		if !exists {
			continue
		}
		if _, err := pool.Exec(ctx, statement, tenantID); err != nil {
			return fmt.Errorf("cleanup optional tenant table %s: %w", tableName, err)
		}
	}
	return nil
}

func tableExists(ctx context.Context, pool *pgxpool.Pool, name string) (bool, error) {
	var exists bool
	err := pool.QueryRow(ctx, `SELECT to_regclass($1) IS NOT NULL`, name).Scan(&exists)
	return exists, err
}

func seedConversation(ctx context.Context, pool *pgxpool.Pool, cfg config) error {
	_, err := pool.Exec(ctx, `
INSERT INTO conversations (
    tenant_id, conversation_id, conversation_type, status, conversation_mode,
    fanout_mode, fanout_policy_version, member_version, permission_version, current_seq_shard
) VALUES ($1, $2, 'GROUP', 'ACTIVE', 'LOCAL_ROW_LOCK', 'WRITE_FANOUT', 1, 1, 1, 'local')
`, cfg.tenantID, cfg.conversationID)
	if err != nil {
		return fmt.Errorf("seed conversation: %w", err)
	}
	_, err = pool.Exec(ctx, `
INSERT INTO conversation_members (
    tenant_id, conversation_id, user_id, role, status, member_version, permission_version
) VALUES ($1, $2, $3, 'OWNER', 'ACTIVE', 1, 1)
`, cfg.tenantID, cfg.conversationID, cfg.ownerUserID)
	if err != nil {
		return fmt.Errorf("seed owner member: %w", err)
	}
	return nil
}

func seedIdentityCredential(ctx context.Context, pool *pgxpool.Pool, cfg config) error {
	passwordHash, err := smokePasswordHash(cfg.identityLoginPassword)
	if err != nil {
		return err
	}
	_, err = pool.Exec(ctx, `
INSERT INTO identity_users (tenant_id, user_id, status, password_hash, password_updated_at, created_at, updated_at)
VALUES ($1, $2, 'ACTIVE', $3, now(), now(), now())
ON CONFLICT (tenant_id, user_id) DO UPDATE
SET status = 'ACTIVE',
    password_hash = EXCLUDED.password_hash,
    password_updated_at = EXCLUDED.password_updated_at,
    updated_at = EXCLUDED.updated_at
`, cfg.tenantID, cfg.receiverUserID, passwordHash)
	if err != nil {
		return fmt.Errorf("seed identity credential: %w", err)
	}
	return nil
}

func smokePasswordHash(password string) (string, error) {
	const iterations = 10_000
	salt := []byte("nexusim-push-smoke")
	key, err := pbkdf2.Key(sha256.New, password, salt, iterations, 32)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(
		"pbkdf2-sha256$%d$%s$%s",
		iterations,
		base64.RawURLEncoding.EncodeToString(salt),
		base64.RawURLEncoding.EncodeToString(key),
	), nil
}

func fillPostgresStats(ctx context.Context, pool *pgxpool.Pool, cfg config, result *summary) error {
	assign := func(target **int64, query string, args ...any) error {
		var value int64
		if err := pool.QueryRow(ctx, query, args...).Scan(&value); err != nil {
			return err
		}
		*target = &value
		return nil
	}
	if err := assign(&result.CursorLastReceivedSeq, `
SELECT COALESCE(MAX(last_received_seq), 0)
FROM device_delivery_cursors
WHERE tenant_id = $1 AND conversation_id = $2 AND user_id = $3 AND device_id = $4
`, cfg.tenantID, cfg.conversationID, cfg.receiverUserID, cfg.receiverDeviceID); err != nil {
		return fmt.Errorf("query cursor: %w", err)
	}
	if err := assign(&result.UserInboxCount, `
SELECT COUNT(*) FROM user_inbox
WHERE tenant_id = $1 AND conversation_id = $2 AND user_id = $3
`, cfg.tenantID, cfg.conversationID, cfg.receiverUserID); err != nil {
		return fmt.Errorf("query inbox: %w", err)
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
	if err := assign(&result.DeliveryOutboxPublished, `
SELECT COUNT(*) FROM delivery_outbox
WHERE tenant_id = $1 AND conversation_id = $2 AND status = 'PUBLISHED'
`, cfg.tenantID, cfg.conversationID); err != nil {
		return fmt.Errorf("query delivery outbox published: %w", err)
	}
	if err := assign(&result.DeliveryOutboxDLQ, `
SELECT COUNT(*) FROM delivery_outbox
WHERE tenant_id = $1 AND conversation_id = $2 AND status = 'DLQ'
`, cfg.tenantID, cfg.conversationID); err != nil {
		return fmt.Errorf("query delivery outbox dlq: %w", err)
	}
	return nil
}

func finish(cfg config, result *summary, runErr error) error {
	result.FinishedAt = time.Now().UTC()
	if runErr != nil {
		result.Success = false
		result.Error = runErr.Error()
	} else {
		result.Success = true
	}
	if cfg.consumerMetricsURL != "" {
		ctx, cancel := context.WithTimeout(context.Background(), cfg.requestTimeout)
		defer cancel()
		if metrics, err := fetchPushMetrics(ctx, cfg.consumerMetricsURL); err == nil {
			result.PushConsumerMetrics = &metrics
		}
	}
	result.Capacity = buildCapacitySummary(result)
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("encode summary: %w", err)
	}
	path := filepath.Join(cfg.resultDir, "pushgateway-summary.json")
	if err := os.WriteFile(path, encoded, 0o644); err != nil {
		return fmt.Errorf("write summary: %w", err)
	}
	fmt.Println(string(encoded))
	fmt.Printf("summary: %s\n", path)
	if runErr != nil {
		return runErr
	}
	return nil
}
