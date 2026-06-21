package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	conversationv1 "github.com/qsyy0921/IM/api/proto/nexusim/conversation/v1"
	gatewayv1 "github.com/qsyy0921/IM/api/proto/nexusim/gateway/v1"
	identityv1 "github.com/qsyy0921/IM/api/proto/nexusim/identity/v1"
	gatewayauth "github.com/qsyy0921/IM/internal/gatewayauth"
	"github.com/qsyy0921/IM/loadtest/internal/grpctls"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
	nhooyr "nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

const (
	opClientHello    = "client.hello"
	opDeliveryNotify = "delivery.notify"
	opServerHello    = "server.hello"
)

func main() {
	cfg := parseConfig()
	ctx := context.Background()
	if err := run(ctx, cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cfg config) error {
	started := time.Now().UTC()
	result := summary{
		Commit:         gitOutput("rev-parse", "--short", "HEAD"),
		CommitFull:     gitOutput("rev-parse", "HEAD"),
		GitDirty:       gitOutput("status", "--short") != "",
		ResultDir:      cfg.resultDir,
		TenantID:       cfg.tenantID,
		ConversationID: cfg.conversationID,
		SenderUserID:   cfg.senderUserID,
		ReceiverUserID: cfg.receiverUserID,
		ReceiverDevice: cfg.receiverDeviceID,
		BFFBaseURL:     strings.TrimRight(cfg.bffBaseURL, "/"),
		PushURL:        cfg.pushURL,
		StartedAt:      started,
	}
	runErr := runClientWebSmoke(ctx, cfg, &result)
	return finish(cfg, &result, runErr)
}

func runClientWebSmoke(ctx context.Context, cfg config, result *summary) error {
	if strings.TrimSpace(cfg.gatewayAuthHMACSecret) == "" {
		return errors.New("--gateway-auth-hmac-secret is required")
	}
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
	if err := seedConversation(ctx, pool, cfg); err != nil {
		return err
	}
	if err := setupIdentityUsers(ctx, cfg, result); err != nil {
		return err
	}
	join, err := createReceiverJoin(ctx, cfg)
	if err != nil {
		return fmt.Errorf("create receiver join: %w", err)
	}
	result.Setup.MemberChangeID = join.GetChangeId()
	result.Setup.MemberBoundarySeq = join.GetBoundarySeq()
	if err := waitMembership(ctx, pool, cfg); err != nil {
		return err
	}

	sender, err := bffLogin(ctx, cfg, cfg.senderUserID, cfg.senderPassword, cfg.senderDeviceID)
	if err != nil {
		return fmt.Errorf("bff sender login: %w", err)
	}
	result.SenderLogin = loginSummaryFromSession(sender)
	receiver, err := bffLogin(ctx, cfg, cfg.receiverUserID, cfg.receiverPassword, cfg.receiverDeviceID)
	if err != nil {
		return fmt.Errorf("bff receiver login: %w", err)
	}
	result.ReceiverLogin = loginSummaryFromSession(receiver)

	conn, hello, err := connectPush(ctx, cfg, receiver)
	if err != nil {
		return fmt.Errorf("connect push websocket: %w", err)
	}
	defer conn.CloseNow()
	result.ServerHello = hello

	sent, err := bffSendMessage(ctx, cfg, sender)
	if err != nil {
		return fmt.Errorf("bff send message: %w", err)
	}
	result.SendMessage = sent
	notify, err := waitNotify(ctx, cfg, conn, sent.ConversationSeq, sent.MessageID)
	if err != nil {
		return fmt.Errorf("wait push notify: %w", err)
	}
	result.Notify = notify
	pull, err := waitPullInbox(ctx, cfg, receiver, sent.ConversationSeq)
	if err != nil {
		return fmt.Errorf("bff pull inbox: %w", err)
	}
	result.PullInbox = pull
	conversations, err := waitConversationList(ctx, cfg, receiver, sent.ConversationSeq)
	if err != nil {
		return fmt.Errorf("bff list conversations: %w", err)
	}
	result.ListConversations = conversations
	ack, err := bffAckDelivery(ctx, cfg, receiver, pull.MaxSeq)
	if err != nil {
		return fmt.Errorf("bff ack delivery: %w", err)
	}
	result.AckDelivery = ack
	if err := waitDeliveryCursor(ctx, pool, cfg, pull.MaxSeq); err != nil {
		return err
	}
	if err := fillPostgresStats(ctx, pool, cfg, result); err != nil {
		return err
	}
	result.Success = true
	return nil
}

func setupIdentityUsers(ctx context.Context, cfg config, result *summary) error {
	dialOption, err := grpctls.DialOption(cfg.identityTLS, "identity-tls")
	if err != nil {
		return fmt.Errorf("configure identity TLS: %w", err)
	}
	conn, err := grpc.NewClient(cfg.identityTarget, dialOption)
	if err != nil {
		return fmt.Errorf("connect identity-service: %w", err)
	}
	defer conn.Close()
	client := identityv1.NewIdentityServiceClient(conn)
	if err := registerUser(ctx, cfg, client, cfg.senderUserID, cfg.senderPassword); err != nil {
		return fmt.Errorf("register sender: %w", err)
	}
	result.Setup.SenderRegistered = true
	if err := registerUser(ctx, cfg, client, cfg.receiverUserID, cfg.receiverPassword); err != nil {
		return fmt.Errorf("register receiver: %w", err)
	}
	result.Setup.ReceiverRegistered = true
	return nil
}

func registerUser(ctx context.Context, cfg config, client identityv1.IdentityServiceClient, userID string, password string) error {
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	_, err := client.RegisterUser(requestCtx, &identityv1.RegisterUserRequest{
		TenantId:  cfg.tenantID,
		UserId:    userID,
		Password:  password,
		TraceId:   "client-web-smoke-register",
		RequestId: "client-web-smoke-register-" + userID,
	})
	if status.Code(err) == codes.AlreadyExists {
		return nil
	}
	return err
}

func createReceiverJoin(ctx context.Context, cfg config) (*conversationv1.CreateMemberChangeResponse, error) {
	dialOption, err := grpctls.DialOption(cfg.gatewayTLS, "gateway-tls")
	if err != nil {
		return nil, fmt.Errorf("configure api-gateway TLS: %w", err)
	}
	conn, err := grpc.NewClient(cfg.gatewayTarget, dialOption)
	if err != nil {
		return nil, fmt.Errorf("connect api-gateway: %w", err)
	}
	defer conn.Close()
	token, err := signSetupToken(cfg, cfg.senderUserID, cfg.senderDeviceID, "client-web-smoke-sender-session")
	if err != nil {
		return nil, err
	}
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	requestCtx = metadata.NewOutgoingContext(requestCtx, metadata.Pairs(
		"authorization", "Bearer "+token,
		"x-nexusim-request-id", "client-web-smoke-join",
		"x-nexusim-trace-id", "client-web-smoke-join",
	))
	return gatewayv1.NewGatewayServiceClient(conn).CreateMemberChange(requestCtx, &conversationv1.CreateMemberChangeRequest{
		ConversationId:        cfg.conversationID,
		TargetUserId:          cfg.receiverUserID,
		ChangeType:            conversationv1.MemberChangeType_MEMBER_CHANGE_TYPE_JOIN,
		TargetRole:            conversationv1.MemberRole_MEMBER_ROLE_MEMBER,
		ExpectedMemberVersion: 1,
		IdempotencyKey:        "client-web-smoke-join-" + cfg.receiverUserID,
		ConflictPolicy:        conversationv1.MemberChangeConflictPolicy_MEMBER_CHANGE_CONFLICT_POLICY_REJECT,
		Reason:                "client web smoke receiver join",
	})
}

func signSetupToken(cfg config, userID string, deviceID string, sessionID string) (string, error) {
	return gatewayauth.SignGatewayToken(cfg.gatewayAuthHMACSecret, map[string]string{
		"tenant_id":  cfg.tenantID,
		"user_id":    userID,
		"device_id":  deviceID,
		"session_id": sessionID,
		"trace_id":   "client-web-smoke",
		"aud":        cfg.gatewayAuthAudience,
	}, time.Now().Add(cfg.gatewayAuthTokenTTL))
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
`, cfg.tenantID, cfg.conversationID, cfg.senderUserID)
	if err != nil {
		return fmt.Errorf("seed sender member: %w", err)
	}
	return nil
}

func cleanupTenant(ctx context.Context, pool *pgxpool.Pool, tenantID string) error {
	queries := []string{
		`DELETE FROM identity_challenge_delivery_repair_audit WHERE tenant_id = $1`,
		`DELETE FROM identity_challenge_delivery_outbox WHERE tenant_id = $1`,
		`DELETE FROM identity_challenge_request_limits WHERE tenant_id = $1`,
		`DELETE FROM identity_mfa_recovery_codes WHERE tenant_id = $1`,
		`DELETE FROM identity_mfa_factors WHERE tenant_id = $1`,
		`DELETE FROM identity_challenges WHERE tenant_id = $1`,
		`DELETE FROM identity_outbox WHERE tenant_id = $1`,
		`DELETE FROM identity_refresh_tokens WHERE tenant_id = $1`,
		`DELETE FROM identity_sessions WHERE tenant_id = $1`,
		`DELETE FROM identity_devices WHERE tenant_id = $1`,
		`DELETE FROM identity_users WHERE tenant_id = $1`,
		`DELETE FROM user_conversation_summaries WHERE tenant_id = $1`,
		`DELETE FROM receipt_outbox WHERE tenant_id = $1`,
		`DELETE FROM message_receipt_states WHERE tenant_id = $1`,
		`DELETE FROM user_read_cursors WHERE tenant_id = $1`,
		`DELETE FROM user_received_cursors WHERE tenant_id = $1`,
		`DELETE FROM device_received_cursors WHERE tenant_id = $1`,
		`DELETE FROM receipt_inbox_projection WHERE tenant_id = $1`,
		`DELETE FROM delivery_outbox WHERE tenant_id = $1`,
		`DELETE FROM device_delivery_cursors WHERE tenant_id = $1`,
		`DELETE FROM user_inbox WHERE tenant_id = $1`,
		`DELETE FROM delivery_membership_projection WHERE tenant_id = $1`,
		`DELETE FROM message_outbox WHERE tenant_id = $1`,
		`DELETE FROM conversation_timeline_events WHERE tenant_id = $1`,
		`DELETE FROM message_log WHERE tenant_id = $1`,
		`DELETE FROM conversation_seq WHERE tenant_id = $1`,
		`DELETE FROM member_change_saga WHERE tenant_id = $1`,
		`DELETE FROM conversation_members WHERE tenant_id = $1`,
		`DELETE FROM conversations WHERE tenant_id = $1`,
	}
	for _, query := range queries {
		if _, err := pool.Exec(ctx, query, tenantID); err != nil {
			return fmt.Errorf("cleanup query %q: %w", query, err)
		}
	}
	return nil
}

func bffLogin(ctx context.Context, cfg config, userID string, password string, deviceID string) (authSession, error) {
	var response struct {
		TenantID               string          `json:"tenant_id"`
		UserID                 string          `json:"user_id"`
		DeviceID               string          `json:"device_id"`
		SessionID              string          `json:"session_id"`
		TokenType              string          `json:"token_type"`
		GatewayToken           string          `json:"gateway_token"`
		PushGatewayToken       string          `json:"push_gateway_token"`
		RefreshToken           string          `json:"refresh_token"`
		GatewayExpiresAtUnixMS json.RawMessage `json:"gateway_expires_at_unix_ms"`
		PushExpiresAtUnixMS    json.RawMessage `json:"push_gateway_expires_at_unix_ms"`
	}
	err := bffJSON(ctx, cfg, http.MethodPost, "/api/auth/login", "", map[string]any{
		"tenant_id":  cfg.tenantID,
		"user_id":    userID,
		"password":   password,
		"device_id":  deviceID,
		"audience":   "api-gateway",
		"trace_id":   "client-web-smoke-login",
		"request_id": "client-web-smoke-login-" + userID,
	}, &response)
	if err != nil {
		return authSession{}, err
	}
	expires, _ := int64JSON(response.GatewayExpiresAtUnixMS)
	pushExpires, _ := int64JSON(response.PushExpiresAtUnixMS)
	session := authSession{
		TenantID:       response.TenantID,
		UserID:         response.UserID,
		DeviceID:       response.DeviceID,
		SessionID:      response.SessionID,
		TokenType:      response.TokenType,
		GatewayToken:   response.GatewayToken,
		PushToken:      response.PushGatewayToken,
		RefreshToken:   response.RefreshToken,
		GatewayExpires: expires,
		PushExpires:    pushExpires,
	}
	if session.GatewayToken == "" || session.RefreshToken == "" || session.SessionID == "" {
		return authSession{}, errors.New("BFF login returned incomplete session")
	}
	if session.PushToken == "" {
		return authSession{}, errors.New("BFF login returned no push gateway token")
	}
	return session, nil
}

func bffSendMessage(ctx context.Context, cfg config, session authSession) (sendSummary, error) {
	payload, err := structpb.NewStruct(map[string]any{"text": "NexusIM client web smoke message"})
	if err != nil {
		return sendSummary{}, err
	}
	var response struct {
		MessageID       string          `json:"message_id"`
		ConversationID  string          `json:"conversation_id"`
		ConversationSeq json.RawMessage `json:"conversation_seq"`
	}
	err = bffJSON(ctx, cfg, http.MethodPost, "/api/messages/send", session.GatewayToken, map[string]any{
		"conversation_id": cfg.conversationID,
		"client_msg_id":   fmt.Sprintf("client-web-smoke-%d", time.Now().UnixNano()),
		"message_type":    "TEXT",
		"payload":         payload.AsMap(),
		"attachment_ids":  []string{},
	}, &response)
	if err != nil {
		return sendSummary{}, err
	}
	seq, err := int64JSON(response.ConversationSeq)
	if err != nil {
		return sendSummary{}, fmt.Errorf("parse conversation_seq: %w", err)
	}
	if response.MessageID == "" || response.ConversationID != cfg.conversationID || seq <= 0 {
		return sendSummary{}, fmt.Errorf("BFF send returned invalid response: %+v seq=%d", response, seq)
	}
	return sendSummary{MessageID: response.MessageID, ConversationSeq: seq}, nil
}

func waitPullInbox(ctx context.Context, cfg config, session authSession, minSeq int64) (pullSummary, error) {
	deadline := time.Now().Add(cfg.waitTimeout)
	var last pullSummary
	for {
		pull, err := bffPullInbox(ctx, cfg, session, 0)
		if err != nil {
			return pullSummary{}, err
		}
		last = pull
		if pull.MaxSeq >= minSeq {
			return pull, nil
		}
		if time.Now().After(deadline) {
			return last, fmt.Errorf("pull inbox max seq %d before expected seq %d", last.MaxSeq, minSeq)
		}
		time.Sleep(cfg.pollInterval)
	}
}

func bffPullInbox(ctx context.Context, cfg config, session authSession, afterSeq int64) (pullSummary, error) {
	var response struct {
		Items []struct {
			ConversationID  string          `json:"conversation_id"`
			ConversationSeq json.RawMessage `json:"conversation_seq"`
			EventID         string          `json:"event_id"`
			EventType       string          `json:"event_type"`
			MessageID       string          `json:"message_id"`
			SenderID        string          `json:"sender_id"`
		} `json:"items"`
		NextSeq json.RawMessage `json:"next_seq"`
	}
	path := fmt.Sprintf("/api/conversations/%s/messages?after_seq=%d&limit=100", url.PathEscape(cfg.conversationID), afterSeq)
	if err := bffJSON(ctx, cfg, http.MethodGet, path, session.GatewayToken, nil, &response); err != nil {
		return pullSummary{}, err
	}
	result := pullSummary{Items: []inboxItem{}}
	for _, item := range response.Items {
		seq, err := int64JSON(item.ConversationSeq)
		if err != nil {
			return pullSummary{}, fmt.Errorf("parse inbox seq: %w", err)
		}
		if seq > result.MaxSeq {
			result.MaxSeq = seq
		}
		result.Items = append(result.Items, inboxItem{
			ConversationSeq: seq,
			EventID:         item.EventID,
			EventType:       item.EventType,
			MessageID:       item.MessageID,
			SenderID:        item.SenderID,
		})
	}
	result.ItemCount = len(result.Items)
	return result, nil
}

func waitConversationList(ctx context.Context, cfg config, session authSession, minSeq int64) (conversationSummary, error) {
	deadline := time.Now().Add(cfg.waitTimeout)
	var last conversationSummary
	for {
		list, err := bffListConversations(ctx, cfg, session)
		if err != nil {
			return conversationSummary{}, err
		}
		last = list
		for _, item := range list.Items {
			if item.ConversationID == cfg.conversationID && item.LastVisibleSeq >= minSeq {
				return list, nil
			}
		}
		if time.Now().After(deadline) {
			return last, fmt.Errorf("conversation list did not reach seq %d", minSeq)
		}
		time.Sleep(cfg.pollInterval)
	}
}

func bffListConversations(ctx context.Context, cfg config, session authSession) (conversationSummary, error) {
	var response struct {
		Items []struct {
			ConversationID string          `json:"conversation_id"`
			LastVisibleSeq json.RawMessage `json:"last_visible_seq"`
			LastMessageID  string          `json:"last_message_id"`
			UnreadCount    json.RawMessage `json:"unread_count"`
		} `json:"items"`
	}
	if err := bffJSON(ctx, cfg, http.MethodGet, "/api/conversations?limit=20", session.GatewayToken, nil, &response); err != nil {
		return conversationSummary{}, err
	}
	result := conversationSummary{Items: []conversationSummaryItem{}}
	for _, item := range response.Items {
		seq, err := int64JSON(item.LastVisibleSeq)
		if err != nil {
			return conversationSummary{}, fmt.Errorf("parse last visible seq: %w", err)
		}
		unread, err := int64JSON(item.UnreadCount)
		if err != nil {
			return conversationSummary{}, fmt.Errorf("parse unread count: %w", err)
		}
		result.Items = append(result.Items, conversationSummaryItem{
			ConversationID: item.ConversationID,
			LastVisibleSeq: seq,
			LastMessageID:  item.LastMessageID,
			UnreadCount:    unread,
		})
	}
	result.ItemCount = len(result.Items)
	return result, nil
}

func bffAckDelivery(ctx context.Context, cfg config, session authSession, seq int64) (ackSummary, error) {
	var response struct {
		ConversationID  string          `json:"conversation_id"`
		LastReceivedSeq json.RawMessage `json:"last_received_seq"`
	}
	err := bffJSON(ctx, cfg, http.MethodPost, "/api/delivery/ack", session.GatewayToken, map[string]any{
		"conversation_id": cfg.conversationID,
		"received_seq":    seq,
	}, &response)
	if err != nil {
		return ackSummary{}, err
	}
	acked, err := int64JSON(response.LastReceivedSeq)
	if err != nil {
		return ackSummary{}, fmt.Errorf("parse ack seq: %w", err)
	}
	if acked < seq {
		return ackSummary{}, fmt.Errorf("ack seq %d before expected %d", acked, seq)
	}
	return ackSummary{ConversationID: response.ConversationID, LastReceivedSeq: acked}, nil
}

func bffJSON(ctx context.Context, cfg config, method string, path string, bearerToken string, body any, output any) error {
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	request, err := http.NewRequestWithContext(requestCtx, method, strings.TrimRight(cfg.bffBaseURL, "/")+path, reader)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("X-NexusIM-Request-ID", "client-web-smoke")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if strings.TrimSpace(bearerToken) != "" {
		request.Header.Set("Authorization", "Bearer "+bearerToken)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("BFF %s %s returned %d: %s", method, path, response.StatusCode, strings.TrimSpace(string(data)))
	}
	if output == nil {
		return nil
	}
	if err := json.Unmarshal(data, output); err != nil {
		return fmt.Errorf("decode BFF response: %w body=%s", err, string(data))
	}
	return nil
}

func connectPush(ctx context.Context, cfg config, session authSession) (*nhooyr.Conn, serverFrame, error) {
	u, err := url.Parse(cfg.pushURL)
	if err != nil {
		return nil, serverFrame{}, err
	}
	query := u.Query()
	query.Set("token", session.PushToken)
	query.Set("tenant_id", session.TenantID)
	query.Set("user_id", session.UserID)
	query.Set("device_id", session.DeviceID)
	u.RawQuery = query.Encode()
	options := &nhooyr.DialOptions{}
	if u.Scheme == "wss" {
		options.HTTPClient = &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}}}
	}
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	conn, _, err := nhooyr.Dial(requestCtx, u.String(), options)
	if err != nil {
		return nil, serverFrame{}, err
	}
	if err := wsjson.Write(requestCtx, conn, clientFrame{
		Op:        opClientHello,
		RequestID: "client-web-smoke-hello",
		DeviceID:  session.DeviceID,
	}); err != nil {
		conn.CloseNow()
		return nil, serverFrame{}, err
	}
	var hello serverFrame
	if err := wsjson.Read(requestCtx, conn, &hello); err != nil {
		conn.CloseNow()
		return nil, serverFrame{}, err
	}
	if hello.Op != opServerHello {
		conn.CloseNow()
		return nil, serverFrame{}, fmt.Errorf("unexpected hello frame: %+v", hello)
	}
	return conn, hello, nil
}

func waitNotify(ctx context.Context, cfg config, conn *nhooyr.Conn, seq int64, messageID string) (serverFrame, error) {
	deadline := time.Now().Add(cfg.waitTimeout)
	var last serverFrame
	for time.Now().Before(deadline) {
		requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
		var frame serverFrame
		err := wsjson.Read(requestCtx, conn, &frame)
		cancel()
		if err != nil {
			if time.Now().After(deadline) {
				return last, err
			}
			continue
		}
		last = frame
		if frame.Op == opDeliveryNotify && frame.ConversationSeq == seq && frame.MessageID == messageID {
			return frame, nil
		}
	}
	return last, fmt.Errorf("did not receive delivery.notify seq=%d message_id=%s", seq, messageID)
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
	return fmt.Errorf("delivery membership projection not ready for %s", cfg.receiverUserID)
}

func waitDeliveryCursor(ctx context.Context, pool *pgxpool.Pool, cfg config, seq int64) error {
	deadline := time.Now().Add(cfg.waitTimeout)
	for time.Now().Before(deadline) {
		var got int64
		err := pool.QueryRow(ctx, `
SELECT COALESCE(MAX(last_received_seq), 0)
FROM device_delivery_cursors
WHERE tenant_id = $1
  AND conversation_id = $2
  AND user_id = $3
  AND device_id = $4
`, cfg.tenantID, cfg.conversationID, cfg.receiverUserID, cfg.receiverDeviceID).Scan(&got)
		if err == nil && got >= seq {
			return nil
		}
		time.Sleep(cfg.pollInterval)
	}
	return fmt.Errorf("delivery cursor did not reach seq %d", seq)
}

func fillPostgresStats(ctx context.Context, pool *pgxpool.Pool, cfg config, result *summary) error {
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM user_inbox WHERE tenant_id = $1 AND conversation_id = $2 AND user_id = $3`, cfg.tenantID, cfg.conversationID, cfg.receiverUserID).Scan(&result.Postgres.UserInboxCount); err != nil {
		return err
	}
	return pool.QueryRow(ctx, `
SELECT COALESCE(MAX(last_received_seq), 0)
FROM device_delivery_cursors
WHERE tenant_id = $1 AND conversation_id = $2 AND user_id = $3 AND device_id = $4
`, cfg.tenantID, cfg.conversationID, cfg.receiverUserID, cfg.receiverDeviceID).Scan(&result.Postgres.DeviceDeliveryCursorSeq)
}

func loginSummaryFromSession(session authSession) loginSummary {
	return loginSummary{
		SessionID:       session.SessionID,
		TokenType:       session.TokenType,
		GatewayTokenSet: session.GatewayToken != "",
		PushTokenSet:    session.PushToken != "",
		RefreshTokenSet: session.RefreshToken != "",
	}
}

func int64JSON(raw json.RawMessage) (int64, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return 0, nil
	}
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		if strings.TrimSpace(asString) == "" {
			return 0, nil
		}
		var parsed int64
		if _, err := fmt.Sscan(asString, &parsed); err != nil {
			return 0, err
		}
		return parsed, nil
	}
	var asNumber int64
	if err := json.Unmarshal(raw, &asNumber); err != nil {
		return 0, err
	}
	return asNumber, nil
}

func finish(cfg config, result *summary, runErr error) error {
	result.FinishedAt = time.Now().UTC()
	if runErr != nil {
		result.Error = runErr.Error()
	}
	if err := os.MkdirAll(cfg.resultDir, 0o755); err != nil {
		return fmt.Errorf("create result dir: %w", err)
	}
	payload, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal summary: %w", err)
	}
	if err := os.WriteFile(filepath.Join(cfg.resultDir, "client-web-summary.json"), payload, 0o644); err != nil {
		return fmt.Errorf("write summary: %w", err)
	}
	if runErr != nil {
		return runErr
	}
	return nil
}

func gitOutput(args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "git", args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}
