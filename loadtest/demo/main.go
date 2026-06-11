package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	conversationv1 "github.com/qsyy0921/IM/api/proto/nexusim/conversation/v1"
	deliveryv1 "github.com/qsyy0921/IM/api/proto/nexusim/delivery/v1"
	messagev1 "github.com/qsyy0921/IM/api/proto/nexusim/message/v1"
	receiptv1 "github.com/qsyy0921/IM/api/proto/nexusim/receipt/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
	nhooyr "nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

const (
	opClientHello    = "client.hello"
	opDeliveryAck    = "delivery.ack"
	opServerHello    = "server.hello"
	opDeliveryNotify = "delivery.notify"
	opDeliveryAckOK  = "delivery.ack.ok"
	opResumeHint     = "server.resume_hint"
)

type config struct {
	conversationTarget string
	messageTarget      string
	deliveryTarget     string
	receiptTarget      string
	pushURL            string
	resultDir          string
	pgDSN              string

	tenantID       string
	conversationID string
	senderUserID   string
	receiverUserID string
	receiverDevice string

	requestTimeout time.Duration
	waitTimeout    time.Duration
	pollInterval   time.Duration
	cleanup        bool

	pushAuthMode       string
	pushAuthHMACSecret string
	pushAuthTokenTTL   time.Duration
}

type summary struct {
	Commit           string                  `json:"commit"`
	CommitFull       string                  `json:"commit_full"`
	GitDirty         bool                    `json:"git_dirty"`
	ResultDir        string                  `json:"result_dir"`
	TenantID         string                  `json:"tenant_id"`
	ConversationID   string                  `json:"conversation_id"`
	SenderUserID     string                  `json:"sender_user_id"`
	ReceiverUserID   string                  `json:"receiver_user_id"`
	ReceiverDeviceID string                  `json:"receiver_device_id"`
	StartedAt        time.Time               `json:"started_at"`
	FinishedAt       time.Time               `json:"finished_at"`
	Success          bool                    `json:"success"`
	Error            string                  `json:"error,omitempty"`
	ServerHello      serverFrame             `json:"server_hello"`
	MemberJoin       memberJoinSummary       `json:"member_join"`
	SendMessage      sendSummary             `json:"send_message"`
	Notify           serverFrame             `json:"delivery_notify"`
	PullInbox        pullSummary             `json:"pull_inbox"`
	WebSocketAck     serverFrame             `json:"websocket_ack"`
	MarkRead         markReadSummary         `json:"mark_read"`
	ListBeforeRead   conversationListSummary `json:"list_conversations_before_read"`
	ListAfterRead    conversationListSummary `json:"list_conversations_after_read"`
	Postgres         postgresSummary         `json:"postgres"`
}

type memberJoinSummary struct {
	ChangeID    string `json:"change_id"`
	BoundarySeq int64  `json:"boundary_seq"`
	Status      string `json:"status"`
}

type sendSummary struct {
	MessageID       string `json:"message_id"`
	ConversationSeq int64  `json:"conversation_seq"`
}

type pullSummary struct {
	ItemCount int         `json:"item_count"`
	MaxSeq    int64       `json:"max_seq"`
	Items     []inboxItem `json:"items"`
}

type inboxItem struct {
	ConversationSeq int64  `json:"conversation_seq"`
	EventID         string `json:"event_id"`
	EventType       string `json:"event_type"`
	MessageID       string `json:"message_id"`
	SenderID        string `json:"sender_id"`
}

type markReadSummary struct {
	LastReadSeq int64 `json:"last_read_seq"`
}

type conversationListSummary struct {
	ItemCount int                       `json:"item_count"`
	Items     []conversationSummaryItem `json:"items"`
}

type conversationSummaryItem struct {
	ConversationID string `json:"conversation_id"`
	LastVisibleSeq int64  `json:"last_visible_seq"`
	LastMessageID  string `json:"last_message_id"`
	UnreadCount    int64  `json:"unread_count"`
	LastReadSeq    int64  `json:"last_read_seq"`
}

type postgresSummary struct {
	UserInboxCount            int64 `json:"user_inbox_count"`
	DeviceDeliveryCursorSeq   int64 `json:"device_delivery_cursor_seq"`
	UserReadCursorSeq         int64 `json:"user_read_cursor_seq"`
	UserConversationSummaries int64 `json:"user_conversation_summaries"`
}

type clientFrame struct {
	Op             string               `json:"op"`
	RequestID      string               `json:"request_id,omitempty"`
	DeviceID       string               `json:"device_id,omitempty"`
	ResumeToken    string               `json:"resume_token,omitempty"`
	LastReceived   []conversationCursor `json:"last_received,omitempty"`
	ConversationID string               `json:"conversation_id,omitempty"`
	ReceivedSeq    int64                `json:"received_seq,omitempty"`
}

type serverFrame struct {
	Op                string               `json:"op"`
	RequestID         string               `json:"request_id,omitempty"`
	SessionID         string               `json:"session_id,omitempty"`
	ResumeToken       string               `json:"resume_token,omitempty"`
	HeartbeatInterval int64                `json:"heartbeat_interval_ms,omitempty"`
	EventID           string               `json:"event_id,omitempty"`
	TenantID          string               `json:"tenant_id,omitempty"`
	ConversationID    string               `json:"conversation_id,omitempty"`
	ConversationSeq   int64                `json:"conversation_seq,omitempty"`
	SourceEventID     string               `json:"source_event_id,omitempty"`
	SourceEventType   string               `json:"source_event_type,omitempty"`
	MessageID         string               `json:"message_id,omitempty"`
	PullRequired      bool                 `json:"pull_required,omitempty"`
	LastReceivedSeq   int64                `json:"last_received_seq,omitempty"`
	Reason            string               `json:"reason,omitempty"`
	Conversations     []conversationCursor `json:"conversations,omitempty"`
	Code              string               `json:"code,omitempty"`
	Message           string               `json:"message,omitempty"`
	Retryable         bool                 `json:"retryable"`
}

type conversationCursor struct {
	ConversationID string `json:"conversation_id"`
	Seq            int64  `json:"seq"`
}

func main() {
	var cfg config
	flag.StringVar(&cfg.conversationTarget, "conversation-target", "127.0.0.1:10496", "conversation-service gRPC target")
	flag.StringVar(&cfg.messageTarget, "message-target", "127.0.0.1:10495", "message-service gRPC target")
	flag.StringVar(&cfg.deliveryTarget, "delivery-target", "127.0.0.1:10497", "delivery-service gRPC target")
	flag.StringVar(&cfg.receiptTarget, "receipt-target", "127.0.0.1:10499", "receipt-service gRPC target")
	flag.StringVar(&cfg.pushURL, "push-url", "ws://127.0.0.1:10498", "push-gateway WebSocket URL")
	flag.StringVar(&cfg.resultDir, "result-dir", `H:\NexusIM\loadtest-results\e2e-demo`, "result directory")
	flag.StringVar(&cfg.pgDSN, "pg-dsn", "", "PostgreSQL DSN used only for local demo seed/cleanup/statistics")
	flag.StringVar(&cfg.tenantID, "tenant-id", "tenant-e2e-demo", "tenant id")
	flag.StringVar(&cfg.conversationID, "conversation-id", "conv-e2e-demo", "conversation id")
	flag.StringVar(&cfg.senderUserID, "sender-user-id", "demo-sender", "sender user id")
	flag.StringVar(&cfg.receiverUserID, "receiver-user-id", "demo-receiver", "receiver user id")
	flag.StringVar(&cfg.receiverDevice, "receiver-device-id", "demo-device-1", "receiver device id")
	flag.DurationVar(&cfg.requestTimeout, "request-timeout", 3*time.Second, "per request timeout")
	flag.DurationVar(&cfg.waitTimeout, "wait-timeout", 20*time.Second, "async wait timeout")
	flag.DurationVar(&cfg.pollInterval, "poll-interval", 200*time.Millisecond, "poll interval")
	flag.BoolVar(&cfg.cleanup, "cleanup", true, "delete existing rows for tenant before local demo")
	flag.StringVar(&cfg.pushAuthMode, "push-auth-mode", "mock", "push-gateway auth mode: mock or hmac")
	flag.StringVar(&cfg.pushAuthHMACSecret, "push-auth-hmac-secret", "", "HMAC secret used to sign push gateway demo token when --push-auth-mode=hmac")
	flag.DurationVar(&cfg.pushAuthTokenTTL, "push-auth-token-ttl", 10*time.Minute, "TTL for generated HMAC push token")
	flag.Parse()

	if err := run(context.Background(), cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cfg config) error {
	if strings.TrimSpace(cfg.pgDSN) == "" {
		return fmt.Errorf("--pg-dsn is required for local demo seed and evidence collection")
	}
	if cfg.pushAuthMode == "hmac" && strings.TrimSpace(cfg.pushAuthHMACSecret) == "" {
		return fmt.Errorf("--push-auth-hmac-secret is required when --push-auth-mode=hmac")
	}
	started := time.Now().UTC()
	result := summary{
		Commit:           gitOutput("rev-parse", "--short", "HEAD"),
		CommitFull:       gitOutput("rev-parse", "HEAD"),
		GitDirty:         strings.TrimSpace(gitOutput("status", "--short")) != "",
		ResultDir:        cfg.resultDir,
		TenantID:         cfg.tenantID,
		ConversationID:   cfg.conversationID,
		SenderUserID:     cfg.senderUserID,
		ReceiverUserID:   cfg.receiverUserID,
		ReceiverDeviceID: cfg.receiverDevice,
		StartedAt:        started,
	}

	pool, err := pgxpool.New(ctx, cfg.pgDSN)
	if err != nil {
		return finish(cfg, &result, fmt.Errorf("connect postgres: %w", err))
	}
	defer pool.Close()
	if cfg.cleanup {
		if err := cleanupTenant(ctx, pool, cfg.tenantID); err != nil {
			return finish(cfg, &result, fmt.Errorf("cleanup tenant: %w", err))
		}
	}
	if err := seedConversation(ctx, pool, cfg); err != nil {
		return finish(cfg, &result, err)
	}

	conversationConn, err := grpc.NewClient(cfg.conversationTarget, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return finish(cfg, &result, fmt.Errorf("connect conversation-service: %w", err))
	}
	defer conversationConn.Close()
	messageConn, err := grpc.NewClient(cfg.messageTarget, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return finish(cfg, &result, fmt.Errorf("connect message-service: %w", err))
	}
	defer messageConn.Close()
	deliveryConn, err := grpc.NewClient(cfg.deliveryTarget, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return finish(cfg, &result, fmt.Errorf("connect delivery-service: %w", err))
	}
	defer deliveryConn.Close()
	receiptConn, err := grpc.NewClient(cfg.receiptTarget, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return finish(cfg, &result, fmt.Errorf("connect receipt-service: %w", err))
	}
	defer receiptConn.Close()

	join, err := createReceiverJoin(ctx, cfg, conversationv1.NewConversationServiceClient(conversationConn))
	if err != nil {
		return finish(cfg, &result, fmt.Errorf("create receiver join: %w", err))
	}
	result.MemberJoin = memberJoinSummary{
		ChangeID:    join.GetChangeId(),
		BoundarySeq: join.GetBoundarySeq(),
		Status:      join.GetStatus().String(),
	}
	if err := waitMembership(ctx, pool, cfg); err != nil {
		return finish(cfg, &result, err)
	}

	conn, hello, err := connectWebSocket(ctx, cfg)
	if err != nil {
		return finish(cfg, &result, fmt.Errorf("connect websocket: %w", err))
	}
	defer conn.CloseNow()
	result.ServerHello = hello

	sent, err := sendMessage(ctx, cfg, messagev1.NewMessageServiceClient(messageConn))
	if err != nil {
		return finish(cfg, &result, fmt.Errorf("send message: %w", err))
	}
	result.SendMessage = sendSummary{MessageID: sent.GetMessageId(), ConversationSeq: sent.GetConversationSeq()}

	notify, err := waitNotify(ctx, cfg, conn, sent.GetConversationSeq(), sent.GetMessageId())
	if err != nil {
		return finish(cfg, &result, fmt.Errorf("wait notify: %w", err))
	}
	result.Notify = notify

	deliveryClient := deliveryv1.NewDeliveryServiceClient(deliveryConn)
	pull, err := pullInboxAtLeast(ctx, cfg, deliveryClient, sent.GetConversationSeq())
	if err != nil {
		return finish(cfg, &result, fmt.Errorf("pull inbox: %w", err))
	}
	result.PullInbox = pull

	receiptClient := receiptv1.NewReceiptServiceClient(receiptConn)
	beforeRead, err := waitConversationSummary(ctx, cfg, receiptClient, pull.MaxSeq, 1, false)
	if err != nil {
		return finish(cfg, &result, fmt.Errorf("wait list conversations before read: %w", err))
	}
	result.ListBeforeRead = beforeRead

	ack, err := ackViaWebSocket(ctx, cfg, conn, pull.MaxSeq)
	if err != nil {
		return finish(cfg, &result, fmt.Errorf("websocket ack: %w", err))
	}
	result.WebSocketAck = ack

	markReadResponse, err := waitMarkRead(ctx, cfg, receiptClient, pull.MaxSeq)
	if err != nil {
		return finish(cfg, &result, fmt.Errorf("mark read: %w", err))
	}
	result.MarkRead = markReadSummary{LastReadSeq: markReadResponse.GetLastReadSeq()}

	afterRead, err := waitConversationSummary(ctx, cfg, receiptClient, pull.MaxSeq, 0, true)
	if err != nil {
		return finish(cfg, &result, fmt.Errorf("wait list conversations after read: %w", err))
	}
	result.ListAfterRead = afterRead
	if err := waitReadCursor(ctx, pool, cfg, pull.MaxSeq); err != nil {
		return finish(cfg, &result, err)
	}
	if err := fillPostgresStats(ctx, pool, cfg, &result); err != nil {
		return finish(cfg, &result, err)
	}

	result.Success = true
	return finish(cfg, &result, nil)
}

func createReceiverJoin(ctx context.Context, cfg config, client conversationv1.ConversationServiceClient) (*conversationv1.CreateMemberChangeResponse, error) {
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	return client.CreateMemberChange(requestCtx, &conversationv1.CreateMemberChangeRequest{
		AuthContext: &conversationv1.AuthContext{
			TenantId:  cfg.tenantID,
			UserId:    cfg.senderUserID,
			DeviceId:  "demo-sender-device",
			SessionId: "demo-sender-session",
			TraceId:   "e2e-demo-join",
			RequestId: "e2e-demo-join",
		},
		ConversationId:        cfg.conversationID,
		TargetUserId:          cfg.receiverUserID,
		ChangeType:            conversationv1.MemberChangeType_MEMBER_CHANGE_TYPE_JOIN,
		TargetRole:            conversationv1.MemberRole_MEMBER_ROLE_MEMBER,
		ExpectedMemberVersion: 1,
		IdempotencyKey:        "e2e-demo-join-receiver",
		ConflictPolicy:        conversationv1.MemberChangeConflictPolicy_MEMBER_CHANGE_CONFLICT_POLICY_REJECT,
		Reason:                "e2e demo receiver join",
	})
}

func sendMessage(ctx context.Context, cfg config, client messagev1.MessageServiceClient) (*messagev1.SendMessageResponse, error) {
	payload, err := structpb.NewStruct(map[string]any{"text": "NexusIM e2e demo message"})
	if err != nil {
		return nil, err
	}
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	return client.SendMessage(requestCtx, &messagev1.SendMessageRequest{
		AuthContext: &messagev1.AuthContext{
			TenantId:  cfg.tenantID,
			UserId:    cfg.senderUserID,
			DeviceId:  "demo-sender-device",
			SessionId: "demo-sender-session",
			TraceId:   "e2e-demo-send",
			RequestId: "e2e-demo-send",
		},
		ConversationId: cfg.conversationID,
		ClientMsgId:    "e2e-demo-message-1",
		MessageType:    "TEXT",
		Payload:        payload,
	})
}

func pullInboxAtLeast(ctx context.Context, cfg config, client deliveryv1.DeliveryServiceClient, minSeq int64) (pullSummary, error) {
	deadline := time.Now().Add(cfg.waitTimeout)
	for {
		requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
		response, err := client.PullInbox(requestCtx, &deliveryv1.PullInboxRequest{
			AuthContext: &deliveryv1.AuthContext{
				TenantId:  cfg.tenantID,
				UserId:    cfg.receiverUserID,
				DeviceId:  cfg.receiverDevice,
				SessionId: "e2e-demo-receiver",
				TraceId:   "e2e-demo-pull",
				RequestId: "e2e-demo-pull",
			},
			ConversationId: cfg.conversationID,
			AfterSeq:       0,
			Limit:          100,
		})
		cancel()
		if err != nil {
			return pullSummary{}, err
		}
		result := summarizePull(response)
		if result.MaxSeq >= minSeq || time.Now().After(deadline) {
			if result.MaxSeq < minSeq {
				return result, fmt.Errorf("pull inbox max seq %d before expected seq %d", result.MaxSeq, minSeq)
			}
			return result, nil
		}
		time.Sleep(cfg.pollInterval)
	}
}

func summarizePull(response *deliveryv1.PullInboxResponse) pullSummary {
	result := pullSummary{ItemCount: len(response.GetItems()), Items: []inboxItem{}}
	for _, item := range response.GetItems() {
		if item.GetConversationSeq() > result.MaxSeq {
			result.MaxSeq = item.GetConversationSeq()
		}
		result.Items = append(result.Items, inboxItem{
			ConversationSeq: item.GetConversationSeq(),
			EventID:         item.GetEventId(),
			EventType:       item.GetEventType(),
			MessageID:       item.GetMessageId(),
			SenderID:        item.GetSenderId(),
		})
	}
	return result
}

func markRead(ctx context.Context, cfg config, client receiptv1.ReceiptServiceClient, seq int64) (*receiptv1.MarkReadResponse, error) {
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	return client.MarkRead(requestCtx, &receiptv1.MarkReadRequest{
		AuthContext: &receiptv1.AuthContext{
			TenantId:  cfg.tenantID,
			UserId:    cfg.receiverUserID,
			DeviceId:  cfg.receiverDevice,
			SessionId: "e2e-demo-receiver",
			TraceId:   "e2e-demo-mark-read",
			RequestId: "e2e-demo-mark-read",
		},
		ConversationId: cfg.conversationID,
		ReadSeq:        seq,
	})
}

func waitMarkRead(ctx context.Context, cfg config, client receiptv1.ReceiptServiceClient, seq int64) (*receiptv1.MarkReadResponse, error) {
	deadline := time.Now().Add(cfg.waitTimeout)
	for {
		response, err := markRead(ctx, cfg, client, seq)
		if err == nil {
			return response, nil
		}
		if status.Code(err) != codes.FailedPrecondition || time.Now().After(deadline) {
			return nil, err
		}
		time.Sleep(cfg.pollInterval)
	}
}

func listConversations(ctx context.Context, cfg config, client receiptv1.ReceiptServiceClient) (conversationListSummary, error) {
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	response, err := client.ListConversations(requestCtx, &receiptv1.ListConversationsRequest{
		AuthContext: &receiptv1.AuthContext{
			TenantId:  cfg.tenantID,
			UserId:    cfg.receiverUserID,
			DeviceId:  cfg.receiverDevice,
			SessionId: "e2e-demo-receiver",
			TraceId:   "e2e-demo-list",
			RequestId: "e2e-demo-list",
		},
		Limit: 10,
	})
	if err != nil {
		return conversationListSummary{}, err
	}
	result := conversationListSummary{ItemCount: len(response.GetItems()), Items: []conversationSummaryItem{}}
	for _, item := range response.GetItems() {
		result.Items = append(result.Items, conversationSummaryItem{
			ConversationID: item.GetConversationId(),
			LastVisibleSeq: item.GetLastVisibleSeq(),
			LastMessageID:  item.GetLastMessageId(),
			UnreadCount:    item.GetUnreadCount(),
			LastReadSeq:    item.GetLastReadSeq(),
		})
	}
	return result, nil
}

func waitConversationSummary(ctx context.Context, cfg config, client receiptv1.ReceiptServiceClient, minSeq int64, expectedUnread int64, requireRead bool) (conversationListSummary, error) {
	deadline := time.Now().Add(cfg.waitTimeout)
	var last conversationListSummary
	for {
		result, err := listConversations(ctx, cfg, client)
		if err != nil {
			return result, err
		}
		last = result
		for _, item := range result.Items {
			if item.ConversationID != cfg.conversationID {
				continue
			}
			if item.LastVisibleSeq < minSeq {
				continue
			}
			if item.UnreadCount != expectedUnread {
				continue
			}
			if requireRead && item.LastReadSeq < minSeq {
				continue
			}
			return result, nil
		}
		if time.Now().After(deadline) {
			return last, fmt.Errorf("conversation summary did not reach seq=%d unread=%d require_read=%t", minSeq, expectedUnread, requireRead)
		}
		time.Sleep(cfg.pollInterval)
	}
}

func connectWebSocket(ctx context.Context, cfg config) (*nhooyr.Conn, serverFrame, error) {
	u, err := url.Parse(cfg.pushURL)
	if err != nil {
		return nil, serverFrame{}, err
	}
	query := u.Query()
	query.Set("device_id", cfg.receiverDevice)
	var dialOptions *nhooyr.DialOptions
	switch cfg.pushAuthMode {
	case "", "mock":
		query.Set("tenant_id", cfg.tenantID)
		query.Set("user_id", cfg.receiverUserID)
	case "hmac":
		token, err := signPushGatewayToken(cfg)
		if err != nil {
			return nil, serverFrame{}, err
		}
		dialOptions = &nhooyr.DialOptions{HTTPHeader: http.Header{"Authorization": []string{"Bearer " + token}}}
	default:
		return nil, serverFrame{}, fmt.Errorf("unsupported push auth mode: %s", cfg.pushAuthMode)
	}
	u.RawQuery = query.Encode()
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	conn, _, err := nhooyr.Dial(requestCtx, u.String(), dialOptions)
	if err != nil {
		return nil, serverFrame{}, err
	}
	if err := wsjson.Write(requestCtx, conn, clientFrame{
		Op:        opClientHello,
		RequestID: "e2e-demo-hello",
		DeviceID:  cfg.receiverDevice,
	}); err != nil {
		conn.CloseNow()
		return nil, serverFrame{}, err
	}
	var hello serverFrame
	if err := wsjson.Read(requestCtx, conn, &hello); err != nil {
		conn.CloseNow()
		return nil, serverFrame{}, err
	}
	if hello.Op != opServerHello || hello.SessionID == "" {
		conn.CloseNow()
		return nil, serverFrame{}, fmt.Errorf("unexpected hello frame: %+v", hello)
	}
	return conn, hello, nil
}

func waitNotify(ctx context.Context, cfg config, conn *nhooyr.Conn, seq int64, messageID string) (serverFrame, error) {
	deadline := time.Now().Add(cfg.waitTimeout)
	for {
		requestCtx, cancel := context.WithTimeout(ctx, time.Until(deadline))
		var frame serverFrame
		err := wsjson.Read(requestCtx, conn, &frame)
		cancel()
		if err != nil {
			return serverFrame{}, err
		}
		if frame.Op == opDeliveryNotify && frame.ConversationSeq == seq && frame.MessageID == messageID {
			return frame, nil
		}
		if time.Now().After(deadline) {
			return serverFrame{}, fmt.Errorf("notify timeout waiting seq=%d message_id=%s", seq, messageID)
		}
	}
}

func ackViaWebSocket(ctx context.Context, cfg config, conn *nhooyr.Conn, seq int64) (serverFrame, error) {
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	if err := wsjson.Write(requestCtx, conn, clientFrame{
		Op:             opDeliveryAck,
		RequestID:      "e2e-demo-ack",
		ConversationID: cfg.conversationID,
		ReceivedSeq:    seq,
	}); err != nil {
		return serverFrame{}, err
	}
	for {
		var frame serverFrame
		if err := wsjson.Read(requestCtx, conn, &frame); err != nil {
			return serverFrame{}, err
		}
		switch frame.Op {
		case opDeliveryAckOK:
			return frame, nil
		case opDeliveryNotify, opResumeHint:
			continue
		default:
			return frame, fmt.Errorf("unexpected ack frame: %+v", frame)
		}
	}
}

type pushGatewayTokenClaims struct {
	TenantID string `json:"tenant_id"`
	UserID   string `json:"user_id"`
	DeviceID string `json:"device_id,omitempty"`
	TraceID  string `json:"trace_id,omitempty"`
	Audience string `json:"aud"`
	Expires  int64  `json:"exp"`
}

func signPushGatewayToken(cfg config) (string, error) {
	claims := pushGatewayTokenClaims{
		TenantID: cfg.tenantID,
		UserID:   cfg.receiverUserID,
		DeviceID: cfg.receiverDevice,
		TraceID:  "e2e-demo-auth",
		Audience: "push-gateway",
		Expires:  time.Now().Add(cfg.pushAuthTokenTTL).Unix(),
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	payloadPart := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, []byte(cfg.pushAuthHMACSecret))
	_, _ = mac.Write([]byte(payloadPart))
	return payloadPart + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
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

func waitReadCursor(ctx context.Context, pool *pgxpool.Pool, cfg config, seq int64) error {
	deadline := time.Now().Add(cfg.waitTimeout)
	for time.Now().Before(deadline) {
		var current int64
		err := pool.QueryRow(ctx, `
SELECT COALESCE(MAX(last_read_seq), 0)
FROM user_read_cursors
WHERE tenant_id = $1 AND conversation_id = $2 AND user_id = $3
`, cfg.tenantID, cfg.conversationID, cfg.receiverUserID).Scan(&current)
		if err == nil && current >= seq {
			return nil
		}
		time.Sleep(cfg.pollInterval)
	}
	return fmt.Errorf("user_read_cursors did not reach seq %d", seq)
}

func fillPostgresStats(ctx context.Context, pool *pgxpool.Pool, cfg config, result *summary) error {
	assign := func(target *int64, query string, args ...any) error {
		var value int64
		if err := pool.QueryRow(ctx, query, args...).Scan(&value); err != nil {
			return err
		}
		*target = value
		return nil
	}
	if err := assign(&result.Postgres.UserInboxCount, `
SELECT COUNT(*) FROM user_inbox
WHERE tenant_id = $1 AND conversation_id = $2 AND user_id = $3
`, cfg.tenantID, cfg.conversationID, cfg.receiverUserID); err != nil {
		return fmt.Errorf("query user inbox count: %w", err)
	}
	if err := assign(&result.Postgres.DeviceDeliveryCursorSeq, `
SELECT COALESCE(MAX(last_received_seq), 0)
FROM device_delivery_cursors
WHERE tenant_id = $1 AND conversation_id = $2 AND user_id = $3 AND device_id = $4
`, cfg.tenantID, cfg.conversationID, cfg.receiverUserID, cfg.receiverDevice); err != nil {
		return fmt.Errorf("query delivery cursor: %w", err)
	}
	if err := assign(&result.Postgres.UserReadCursorSeq, `
SELECT COALESCE(MAX(last_read_seq), 0)
FROM user_read_cursors
WHERE tenant_id = $1 AND conversation_id = $2 AND user_id = $3
`, cfg.tenantID, cfg.conversationID, cfg.receiverUserID); err != nil {
		return fmt.Errorf("query read cursor: %w", err)
	}
	if err := assign(&result.Postgres.UserConversationSummaries, `
SELECT COUNT(*) FROM user_conversation_summaries
WHERE tenant_id = $1 AND conversation_id = $2 AND user_id = $3
`, cfg.tenantID, cfg.conversationID, cfg.receiverUserID); err != nil {
		return fmt.Errorf("query conversation summaries: %w", err)
	}
	return nil
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
	if err := os.WriteFile(filepath.Join(cfg.resultDir, "e2e-demo-summary.json"), payload, 0o644); err != nil {
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
