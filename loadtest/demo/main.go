package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
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
	"github.com/qsyy0921/IM/loadtest/internal/grpctls"
	policyeventsv1 "github.com/qsyy0921/IM/schemas/kafka/policy/v1"
	kafkago "github.com/segmentio/kafka-go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
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

	metadataTenantID  = "x-nexusim-tenant-id"
	metadataUserID    = "x-nexusim-user-id"
	metadataDeviceID  = "x-nexusim-device-id"
	metadataSessionID = "x-nexusim-session-id"
	metadataTraceID   = "x-nexusim-trace-id"
	metadataRequestID = "x-nexusim-request-id"
)

type config struct {
	conversationTarget string
	messageTarget      string
	deliveryTarget     string
	receiptTarget      string
	conversationTLS    grpctls.Config
	messageTLS         grpctls.Config
	deliveryTLS        grpctls.Config
	receiptTLS         grpctls.Config
	pushTLS            grpctls.Config
	pushURL            string
	resultDir          string
	pgDSN              string
	policyKafkaBrokers []string
	policyTopic        string
	policyReadbackMin  int64

	tenantID       string
	conversationID string
	senderUserID   string
	receiverUserID string
	receiverDevice string

	requestTimeout time.Duration
	waitTimeout    time.Duration
	pollInterval   time.Duration
	cleanup        bool

	verifiedAuthMetadata bool
	pushAuthMode         string
	pushAuthHMACSecret   string
	pushAuthTokenTTL     time.Duration
}

type summary struct {
	Commit                 string                   `json:"commit"`
	CommitFull             string                   `json:"commit_full"`
	GitDirty               bool                     `json:"git_dirty"`
	ResultDir              string                   `json:"result_dir"`
	TenantID               string                   `json:"tenant_id"`
	ConversationID         string                   `json:"conversation_id"`
	SenderUserID           string                   `json:"sender_user_id"`
	ReceiverUserID         string                   `json:"receiver_user_id"`
	ReceiverDeviceID       string                   `json:"receiver_device_id"`
	ConversationTLSEnabled bool                     `json:"conversation_tls_enabled"`
	MessageTLSEnabled      bool                     `json:"message_tls_enabled"`
	DeliveryTLSEnabled     bool                     `json:"delivery_tls_enabled"`
	ReceiptTLSEnabled      bool                     `json:"receipt_tls_enabled"`
	PushTLSEnabled         bool                     `json:"push_tls_enabled"`
	VerifiedAuthMetadata   bool                     `json:"verified_auth_metadata"`
	StartedAt              time.Time                `json:"started_at"`
	FinishedAt             time.Time                `json:"finished_at"`
	Success                bool                     `json:"success"`
	Error                  string                   `json:"error,omitempty"`
	ServerHello            serverFrame              `json:"server_hello"`
	MemberJoin             memberJoinSummary        `json:"member_join"`
	SendMessage            sendSummary              `json:"send_message"`
	Notify                 serverFrame              `json:"delivery_notify"`
	PullInbox              pullSummary              `json:"pull_inbox"`
	WebSocketAck           serverFrame              `json:"websocket_ack"`
	MarkRead               markReadSummary          `json:"mark_read"`
	ListBeforeRead         conversationListSummary  `json:"list_conversations_before_read"`
	ListAfterRead          conversationListSummary  `json:"list_conversations_after_read"`
	Postgres               postgresSummary          `json:"postgres"`
	PolicyAuditKafka       *policyAuditKafkaSummary `json:"policy_audit_kafka,omitempty"`
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

type policyAuditKafkaSummary struct {
	Topic             string `json:"topic"`
	EventCount        int64  `json:"event_count"`
	EventID           string `json:"event_id"`
	EventType         string `json:"event_type"`
	Producer          string `json:"producer"`
	Allowed           bool   `json:"allowed"`
	PermissionVersion int64  `json:"permission_version"`
	Classification    string `json:"classification"`
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
	registerTLSFlags("conversation-tls", "NEXUSIM_CONVERSATION_TLS", "conversation-service", &cfg.conversationTLS)
	registerTLSFlags("message-tls", "NEXUSIM_MESSAGE_TLS", "message-service", &cfg.messageTLS)
	registerTLSFlags("delivery-tls", "NEXUSIM_DELIVERY_TLS", "delivery-service", &cfg.deliveryTLS)
	registerTLSFlags("receipt-tls", "NEXUSIM_RECEIPT_TLS", "receipt-service", &cfg.receiptTLS)
	registerTLSFlags("push-tls", "NEXUSIM_PUSH_WS_TLS", "push-gateway WebSocket", &cfg.pushTLS)
	flag.StringVar(&cfg.pushURL, "push-url", "ws://127.0.0.1:10498", "push-gateway WebSocket URL")
	flag.StringVar(&cfg.resultDir, "result-dir", `H:\NexusIM\loadtest-results\e2e-demo`, "result directory")
	flag.StringVar(&cfg.pgDSN, "pg-dsn", "", "PostgreSQL DSN used only for local demo seed/cleanup/statistics")
	policyKafkaBrokers := flag.String("policy-kafka-brokers", "", "comma-separated Kafka brokers for optional policy audit event readback")
	flag.StringVar(&cfg.policyTopic, "policy-topic", "", "Kafka topic for optional policy audit event readback")
	flag.Int64Var(&cfg.policyReadbackMin, "policy-readback-min", 0, "minimum policy audit Kafka events to read back for this tenant")
	flag.StringVar(&cfg.tenantID, "tenant-id", "tenant-e2e-demo", "tenant id")
	flag.StringVar(&cfg.conversationID, "conversation-id", "conv-e2e-demo", "conversation id")
	flag.StringVar(&cfg.senderUserID, "sender-user-id", "demo-sender", "sender user id")
	flag.StringVar(&cfg.receiverUserID, "receiver-user-id", "demo-receiver", "receiver user id")
	flag.StringVar(&cfg.receiverDevice, "receiver-device-id", "demo-device-1", "receiver device id")
	flag.DurationVar(&cfg.requestTimeout, "request-timeout", 3*time.Second, "per request timeout")
	flag.DurationVar(&cfg.waitTimeout, "wait-timeout", 20*time.Second, "async wait timeout")
	flag.DurationVar(&cfg.pollInterval, "poll-interval", 200*time.Millisecond, "poll interval")
	flag.BoolVar(&cfg.cleanup, "cleanup", true, "delete existing rows for tenant before local demo")
	flag.BoolVar(&cfg.verifiedAuthMetadata, "verified-auth-metadata", envBool("NEXUSIM_DEMO_VERIFIED_AUTH_METADATA", false), "send gateway verified identity through gRPC metadata for user-facing service RPCs")
	flag.StringVar(&cfg.pushAuthMode, "push-auth-mode", "mock", "push-gateway auth mode: mock or hmac")
	flag.StringVar(&cfg.pushAuthHMACSecret, "push-auth-hmac-secret", "", "HMAC secret used to sign push gateway demo token when --push-auth-mode=hmac")
	flag.DurationVar(&cfg.pushAuthTokenTTL, "push-auth-token-ttl", 10*time.Minute, "TTL for generated HMAC push token")
	flag.Parse()
	cfg.policyKafkaBrokers = splitCSV(*policyKafkaBrokers)

	if err := run(context.Background(), cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func registerTLSFlags(prefix string, envPrefix string, serviceName string, config *grpctls.Config) {
	flag.StringVar(&config.CAFile, prefix+"-ca-file", os.Getenv(envPrefix+"_CA_FILE"), "CA PEM for "+serviceName+" gRPC TLS")
	flag.StringVar(&config.ServerName, prefix+"-server-name", os.Getenv(envPrefix+"_SERVER_NAME"), "override server name for "+serviceName+" gRPC TLS")
	flag.StringVar(&config.ClientCertFile, prefix+"-client-cert-file", os.Getenv(envPrefix+"_CLIENT_CERT_FILE"), "client certificate PEM for "+serviceName+" gRPC mTLS")
	flag.StringVar(&config.ClientKeyFile, prefix+"-client-key-file", os.Getenv(envPrefix+"_CLIENT_KEY_FILE"), "client private key PEM for "+serviceName+" gRPC mTLS")
}

func envBool(name string, fallback bool) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "":
		return fallback
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return fallback
	}
}

type demoAuth struct {
	tenantID  string
	userID    string
	deviceID  string
	sessionID string
	traceID   string
	requestID string
}

func withVerifiedAuthMetadata(ctx context.Context, cfg config, auth demoAuth) context.Context {
	if !cfg.verifiedAuthMetadata {
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

func run(ctx context.Context, cfg config) error {
	if strings.TrimSpace(cfg.pgDSN) == "" {
		return fmt.Errorf("--pg-dsn is required for local demo seed and evidence collection")
	}
	if cfg.pushAuthMode == "hmac" && strings.TrimSpace(cfg.pushAuthHMACSecret) == "" {
		return fmt.Errorf("--push-auth-hmac-secret is required when --push-auth-mode=hmac")
	}
	started := time.Now().UTC()
	result := summary{
		Commit:                 gitOutput("rev-parse", "--short", "HEAD"),
		CommitFull:             gitOutput("rev-parse", "HEAD"),
		GitDirty:               strings.TrimSpace(gitOutput("status", "--short")) != "",
		ResultDir:              cfg.resultDir,
		TenantID:               cfg.tenantID,
		ConversationID:         cfg.conversationID,
		SenderUserID:           cfg.senderUserID,
		ReceiverUserID:         cfg.receiverUserID,
		ReceiverDeviceID:       cfg.receiverDevice,
		ConversationTLSEnabled: cfg.conversationTLS.Enabled(),
		MessageTLSEnabled:      cfg.messageTLS.Enabled(),
		DeliveryTLSEnabled:     cfg.deliveryTLS.Enabled(),
		ReceiptTLSEnabled:      cfg.receiptTLS.Enabled(),
		PushTLSEnabled:         cfg.pushTLS.Enabled(),
		VerifiedAuthMetadata:   cfg.verifiedAuthMetadata,
		StartedAt:              started,
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

	conversationDialOption, err := grpctls.DialOption(cfg.conversationTLS, "conversation-tls")
	if err != nil {
		return finish(cfg, &result, fmt.Errorf("configure conversation-service TLS: %w", err))
	}
	conversationConn, err := grpc.NewClient(cfg.conversationTarget, conversationDialOption)
	if err != nil {
		return finish(cfg, &result, fmt.Errorf("connect conversation-service: %w", err))
	}
	defer conversationConn.Close()
	messageDialOption, err := grpctls.DialOption(cfg.messageTLS, "message-tls")
	if err != nil {
		return finish(cfg, &result, fmt.Errorf("configure message-service TLS: %w", err))
	}
	messageConn, err := grpc.NewClient(cfg.messageTarget, messageDialOption)
	if err != nil {
		return finish(cfg, &result, fmt.Errorf("connect message-service: %w", err))
	}
	defer messageConn.Close()
	deliveryDialOption, err := grpctls.DialOption(cfg.deliveryTLS, "delivery-tls")
	if err != nil {
		return finish(cfg, &result, fmt.Errorf("configure delivery-service TLS: %w", err))
	}
	deliveryConn, err := grpc.NewClient(cfg.deliveryTarget, deliveryDialOption)
	if err != nil {
		return finish(cfg, &result, fmt.Errorf("connect delivery-service: %w", err))
	}
	defer deliveryConn.Close()
	receiptDialOption, err := grpctls.DialOption(cfg.receiptTLS, "receipt-tls")
	if err != nil {
		return finish(cfg, &result, fmt.Errorf("configure receipt-service TLS: %w", err))
	}
	receiptConn, err := grpc.NewClient(cfg.receiptTarget, receiptDialOption)
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
	if cfg.policyReadbackMin > 0 {
		readback, err := waitPolicyAuditKafkaReadback(ctx, cfg)
		if err != nil {
			return finish(cfg, &result, err)
		}
		result.PolicyAuditKafka = &readback
	}

	result.Success = true
	return finish(cfg, &result, nil)
}

func createReceiverJoin(ctx context.Context, cfg config, client conversationv1.ConversationServiceClient) (*conversationv1.CreateMemberChangeResponse, error) {
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	auth := demoAuth{
		tenantID:  cfg.tenantID,
		userID:    cfg.senderUserID,
		deviceID:  "demo-sender-device",
		sessionID: "demo-sender-session",
		traceID:   "e2e-demo-join",
		requestID: "e2e-demo-join",
	}
	requestCtx = withVerifiedAuthMetadata(requestCtx, cfg, auth)
	return client.CreateMemberChange(requestCtx, &conversationv1.CreateMemberChangeRequest{
		AuthContext: &conversationv1.AuthContext{
			TenantId:  auth.tenantID,
			UserId:    auth.userID,
			DeviceId:  auth.deviceID,
			SessionId: auth.sessionID,
			TraceId:   auth.traceID,
			RequestId: auth.requestID,
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
	auth := demoAuth{
		tenantID:  cfg.tenantID,
		userID:    cfg.senderUserID,
		deviceID:  "demo-sender-device",
		sessionID: "demo-sender-session",
		traceID:   "e2e-demo-send",
		requestID: "e2e-demo-send",
	}
	requestCtx = withVerifiedAuthMetadata(requestCtx, cfg, auth)
	return client.SendMessage(requestCtx, &messagev1.SendMessageRequest{
		AuthContext: &messagev1.AuthContext{
			TenantId:  auth.tenantID,
			UserId:    auth.userID,
			DeviceId:  auth.deviceID,
			SessionId: auth.sessionID,
			TraceId:   auth.traceID,
			RequestId: auth.requestID,
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
		auth := demoAuth{
			tenantID:  cfg.tenantID,
			userID:    cfg.receiverUserID,
			deviceID:  cfg.receiverDevice,
			sessionID: "e2e-demo-receiver",
			traceID:   "e2e-demo-pull",
			requestID: "e2e-demo-pull",
		}
		requestCtx = withVerifiedAuthMetadata(requestCtx, cfg, auth)
		response, err := client.PullInbox(requestCtx, &deliveryv1.PullInboxRequest{
			AuthContext: &deliveryv1.AuthContext{
				TenantId:  auth.tenantID,
				UserId:    auth.userID,
				DeviceId:  auth.deviceID,
				SessionId: auth.sessionID,
				TraceId:   auth.traceID,
				RequestId: auth.requestID,
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
	auth := demoAuth{
		tenantID:  cfg.tenantID,
		userID:    cfg.receiverUserID,
		deviceID:  cfg.receiverDevice,
		sessionID: "e2e-demo-receiver",
		traceID:   "e2e-demo-mark-read",
		requestID: "e2e-demo-mark-read",
	}
	requestCtx = withVerifiedAuthMetadata(requestCtx, cfg, auth)
	return client.MarkRead(requestCtx, &receiptv1.MarkReadRequest{
		AuthContext: &receiptv1.AuthContext{
			TenantId:  auth.tenantID,
			UserId:    auth.userID,
			DeviceId:  auth.deviceID,
			SessionId: auth.sessionID,
			TraceId:   auth.traceID,
			RequestId: auth.requestID,
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
	auth := demoAuth{
		tenantID:  cfg.tenantID,
		userID:    cfg.receiverUserID,
		deviceID:  cfg.receiverDevice,
		sessionID: "e2e-demo-receiver",
		traceID:   "e2e-demo-list",
		requestID: "e2e-demo-list",
	}
	requestCtx = withVerifiedAuthMetadata(requestCtx, cfg, auth)
	response, err := client.ListConversations(requestCtx, &receiptv1.ListConversationsRequest{
		AuthContext: &receiptv1.AuthContext{
			TenantId:  auth.tenantID,
			UserId:    auth.userID,
			DeviceId:  auth.deviceID,
			SessionId: auth.sessionID,
			TraceId:   auth.traceID,
			RequestId: auth.requestID,
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
	var header http.Header
	switch cfg.pushAuthMode {
	case "", "mock":
		query.Set("tenant_id", cfg.tenantID)
		query.Set("user_id", cfg.receiverUserID)
	case "hmac":
		token, err := signPushGatewayToken(cfg)
		if err != nil {
			return nil, serverFrame{}, err
		}
		header = http.Header{"Authorization": []string{"Bearer " + token}}
	default:
		return nil, serverFrame{}, fmt.Errorf("unsupported push auth mode: %s", cfg.pushAuthMode)
	}
	dialOptions, err := webSocketDialOptions(cfg, header)
	if err != nil {
		return nil, serverFrame{}, err
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

func webSocketDialOptions(cfg config, header http.Header) (*nhooyr.DialOptions, error) {
	options := &nhooyr.DialOptions{}
	if header != nil {
		options.HTTPHeader = header
	}
	tlsConfig, err := webSocketTLSConfig(cfg.pushTLS, "push-tls")
	if err != nil {
		return nil, err
	}
	if tlsConfig != nil {
		options.HTTPClient = &http.Client{
			Transport: &http.Transport{TLSClientConfig: tlsConfig},
		}
	}
	if options.HTTPHeader == nil && options.HTTPClient == nil {
		return nil, nil
	}
	return options, nil
}

func webSocketTLSConfig(config grpctls.Config, flagPrefix string) (*tls.Config, error) {
	if !config.Enabled() {
		return nil, nil
	}
	caFile := strings.TrimSpace(config.CAFile)
	if caFile == "" {
		return nil, errors.New("--" + flagPrefix + "-ca-file is required when push WebSocket TLS is configured")
	}
	clientCertFile := strings.TrimSpace(config.ClientCertFile)
	clientKeyFile := strings.TrimSpace(config.ClientKeyFile)
	if (clientCertFile == "") != (clientKeyFile == "") {
		return nil, errors.New("--" + flagPrefix + "-client-cert-file and --" + flagPrefix + "-client-key-file must be configured together")
	}
	pemBytes, err := os.ReadFile(caFile)
	if err != nil {
		return nil, err
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(pemBytes) {
		return nil, errors.New("--" + flagPrefix + "-ca-file does not contain a valid PEM certificate")
	}
	tlsConfig := &tls.Config{
		RootCAs:    roots,
		ServerName: strings.TrimSpace(config.ServerName),
		MinVersion: tls.VersionTLS12,
	}
	if clientCertFile != "" {
		cert, err := tls.LoadX509KeyPair(clientCertFile, clientKeyFile)
		if err != nil {
			return nil, err
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}
	return tlsConfig, nil
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

func waitPolicyAuditKafkaReadback(ctx context.Context, cfg config) (policyAuditKafkaSummary, error) {
	if len(cfg.policyKafkaBrokers) == 0 {
		return policyAuditKafkaSummary{}, fmt.Errorf("--policy-kafka-brokers is required when --policy-readback-min > 0")
	}
	if strings.TrimSpace(cfg.policyTopic) == "" {
		return policyAuditKafkaSummary{}, fmt.Errorf("--policy-topic is required when --policy-readback-min > 0")
	}
	groupID := "nexusim-e2e-demo-policy-readback-" + sanitizeID(cfg.tenantID) + "-" + time.Now().UTC().Format("20060102150405")
	reader := kafkago.NewReader(kafkago.ReaderConfig{
		Brokers:     cfg.policyKafkaBrokers,
		Topic:       cfg.policyTopic,
		GroupID:     groupID,
		MinBytes:    1,
		MaxBytes:    1 << 20,
		MaxWait:     100 * time.Millisecond,
		StartOffset: kafkago.FirstOffset,
	})
	defer reader.Close()
	readCtx, cancel := context.WithTimeout(ctx, cfg.waitTimeout)
	defer cancel()
	summary := policyAuditKafkaSummary{Topic: cfg.policyTopic}
	for summary.EventCount < cfg.policyReadbackMin {
		message, err := reader.ReadMessage(readCtx)
		if err != nil {
			return summary, fmt.Errorf("read policy audit kafka event: %w", err)
		}
		var event policyeventsv1.PolicyEvent
		if err := proto.Unmarshal(message.Value, &event); err != nil {
			return summary, fmt.Errorf("decode policy audit kafka event: %w", err)
		}
		if event.GetTenantId() != cfg.tenantID {
			continue
		}
		decision := event.GetMessageActionDecision()
		if decision == nil {
			return summary, fmt.Errorf("policy audit kafka event %s missing message_action_decision payload", event.GetEventId())
		}
		summary.EventCount++
		summary.EventID = event.GetEventId()
		summary.EventType = event.GetEventType()
		summary.Producer = event.GetProducer()
		summary.Allowed = decision.GetAllowed()
		summary.PermissionVersion = decision.GetPermissionVersion()
		summary.Classification = decision.GetClassification()
	}
	return summary, nil
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func sanitizeID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "run"
	}
	var builder strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			builder.WriteRune(r)
			continue
		}
		builder.WriteByte('-')
	}
	return builder.String()
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
