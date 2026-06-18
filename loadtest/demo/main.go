package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	conversationv1 "github.com/qsyy0921/IM/api/proto/nexusim/conversation/v1"
	deliveryv1 "github.com/qsyy0921/IM/api/proto/nexusim/delivery/v1"
	gatewayv1 "github.com/qsyy0921/IM/api/proto/nexusim/gateway/v1"
	messagev1 "github.com/qsyy0921/IM/api/proto/nexusim/message/v1"
	receiptv1 "github.com/qsyy0921/IM/api/proto/nexusim/receipt/v1"
	gatewayauth "github.com/qsyy0921/IM/internal/gatewayauth"
	"github.com/qsyy0921/IM/loadtest/internal/grpctls"
	policyeventsv1 "github.com/qsyy0921/IM/schemas/kafka/policy/v1"
	kafkago "github.com/segmentio/kafka-go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	nhooyr "nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

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
	flag.IntVar(&cfg.virtualUsers, "vus", 1, "logical send batch size used by capacity mode")
	flag.DurationVar(&cfg.duration, "duration", 0, "run the gateway facade demo send/notify loop until this duration elapses; zero keeps single demo mode")
	flag.IntVar(&cfg.messageCount, "message-count", 1, "number of messages sent in single demo mode")
	flag.DurationVar(&cfg.requestTimeout, "request-timeout", 3*time.Second, "per request timeout")
	flag.DurationVar(&cfg.waitTimeout, "wait-timeout", 20*time.Second, "async wait timeout")
	flag.DurationVar(&cfg.pollInterval, "poll-interval", 200*time.Millisecond, "poll interval")
	flag.BoolVar(&cfg.cleanup, "cleanup", true, "delete existing rows for tenant before local demo")
	flag.BoolVar(&cfg.verifiedAuthMetadata, "verified-auth-metadata", envBool("NEXUSIM_DEMO_VERIFIED_AUTH_METADATA", false), "send gateway verified identity through gRPC metadata for user-facing service RPCs")
	flag.BoolVar(&cfg.gatewayFacade, "gateway-facade", envBool("NEXUSIM_DEMO_GATEWAY_FACADE", false), "use nexusim.gateway.v1.GatewayService for conversation/message/delivery/receipt user-facing RPCs")
	flag.StringVar(&cfg.gatewayAuthMode, "gateway-auth-mode", os.Getenv("NEXUSIM_DEMO_GATEWAY_AUTH_MODE"), "api-gateway auth mode for user-facing gRPC calls: empty, mock, or hmac")
	flag.StringVar(&cfg.gatewayAuthHMACSecret, "gateway-auth-hmac-secret", os.Getenv("NEXUSIM_DEMO_GATEWAY_AUTH_HMAC_SECRET"), "HMAC secret used to sign api-gateway demo token when --gateway-auth-mode=hmac")
	flag.StringVar(&cfg.gatewayAuthAudience, "gateway-auth-audience", envString("NEXUSIM_DEMO_GATEWAY_AUTH_AUDIENCE", "api-gateway"), "audience claim used for generated api-gateway demo token")
	flag.DurationVar(&cfg.gatewayAuthTokenTTL, "gateway-auth-token-ttl", 10*time.Minute, "TTL for generated HMAC api-gateway token")
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

func run(ctx context.Context, cfg config) error {
	cfg.gatewayAuthMode = strings.ToLower(strings.TrimSpace(cfg.gatewayAuthMode))
	cfg.gatewayAuthAudience = normalizedGatewayAuthAudience(cfg.gatewayAuthAudience)
	cfg.pushAuthMode = strings.ToLower(strings.TrimSpace(cfg.pushAuthMode))
	if cfg.virtualUsers <= 0 {
		cfg.virtualUsers = 1
	}
	if cfg.messageCount <= 0 {
		cfg.messageCount = 1
	}
	if cfg.duration < 0 {
		return fmt.Errorf("--duration must not be negative")
	}
	if strings.TrimSpace(cfg.pgDSN) == "" {
		return fmt.Errorf("--pg-dsn is required for local demo seed and evidence collection")
	}
	if cfg.gatewayAuthMode != "" && cfg.verifiedAuthMetadata {
		return fmt.Errorf("--gateway-auth-mode and --verified-auth-metadata cannot be combined")
	}
	if cfg.gatewayAuthMode == "hmac" && strings.TrimSpace(cfg.gatewayAuthHMACSecret) == "" {
		return fmt.Errorf("--gateway-auth-hmac-secret is required when --gateway-auth-mode=hmac")
	}
	if cfg.gatewayAuthMode != "" && cfg.gatewayAuthMode != "mock" && cfg.gatewayAuthMode != "hmac" {
		return fmt.Errorf("unsupported gateway auth mode: %s", cfg.gatewayAuthMode)
	}
	if cfg.pushAuthMode == "hmac" && strings.TrimSpace(cfg.pushAuthHMACSecret) == "" {
		return fmt.Errorf("--push-auth-hmac-secret is required when --push-auth-mode=hmac")
	}
	if cfg.pushAuthMode != "" && cfg.pushAuthMode != "mock" && cfg.pushAuthMode != "hmac" {
		return fmt.Errorf("unsupported push auth mode: %s", cfg.pushAuthMode)
	}
	started := time.Now().UTC()
	result := summary{
		Commit:                  gitOutput("rev-parse", "--short", "HEAD"),
		CommitFull:              gitOutput("rev-parse", "HEAD"),
		GitDirty:                strings.TrimSpace(gitOutput("status", "--short")) != "",
		ResultDir:               cfg.resultDir,
		TenantID:                cfg.tenantID,
		ConversationID:          cfg.conversationID,
		SenderUserID:            cfg.senderUserID,
		ReceiverUserID:          cfg.receiverUserID,
		ReceiverDeviceID:        cfg.receiverDevice,
		ConversationTLSEnabled:  cfg.conversationTLS.Enabled(),
		MessageTLSEnabled:       cfg.messageTLS.Enabled(),
		DeliveryTLSEnabled:      cfg.deliveryTLS.Enabled(),
		ReceiptTLSEnabled:       cfg.receiptTLS.Enabled(),
		PushTLSEnabled:          cfg.pushTLS.Enabled(),
		VerifiedAuthMetadata:    cfg.verifiedAuthMetadata,
		GatewayFacade:           cfg.gatewayFacade,
		GatewayAuthMode:         cfg.gatewayAuthMode,
		GatewayAuthAudience:     gatewayAuthAudienceSummary(cfg.gatewayAuthMode, cfg.gatewayAuthAudience),
		CapacityMode:            cfg.duration > 0,
		CapacityDurationSeconds: cfg.duration.Seconds(),
		VirtualUsers:            cfg.virtualUsers,
		StartedAt:               started,
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

	conversationClient := conversationv1.NewConversationServiceClient(conversationConn)
	messageClient := messagev1.NewMessageServiceClient(messageConn)
	deliveryClient := deliveryv1.NewDeliveryServiceClient(deliveryConn)
	receiptClient := receiptv1.NewReceiptServiceClient(receiptConn)
	if cfg.gatewayFacade {
		if cfg.messageTarget != cfg.conversationTarget || cfg.deliveryTarget != cfg.conversationTarget || cfg.receiptTarget != cfg.conversationTarget {
			return finish(cfg, &result, fmt.Errorf("--gateway-facade requires conversation/message/delivery/receipt targets to point at the same api-gateway endpoint"))
		}
		facadeClient := gatewayv1.NewGatewayServiceClient(conversationConn)
		conversationClient = gatewayConversationClient{GatewayServiceClient: facadeClient}
		messageClient = facadeClient
		deliveryClient = facadeClient
		receiptClient = facadeClient
	}

	join, err := createReceiverJoin(ctx, cfg, conversationClient)
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

	sent, notify, messageCount, err := runSendNotifyLoop(ctx, cfg, messageClient, conn)
	if err != nil {
		return finish(cfg, &result, err)
	}
	result.SendMessage = sendSummary{MessageID: sent.GetMessageId(), ConversationSeq: sent.GetConversationSeq()}
	result.Notify = notify
	result.MessageCount = messageCount
	result.NotifyFrameCount = messageCount

	pullAfterSeq := int64(0)
	if cfg.duration > 0 {
		pullAfterSeq = sent.GetConversationSeq() - 1
	}
	pull, err := pullInboxAtLeast(ctx, cfg, deliveryClient, pullAfterSeq, sent.GetConversationSeq())
	if err != nil {
		return finish(cfg, &result, fmt.Errorf("pull inbox: %w", err))
	}
	result.PullInbox = pull

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

func runSendNotifyLoop(
	ctx context.Context,
	cfg config,
	client messagev1.MessageServiceClient,
	conn *nhooyr.Conn,
) (*messagev1.SendMessageResponse, serverFrame, int, error) {
	deadline := time.Time{}
	if cfg.duration > 0 {
		deadline = time.Now().Add(cfg.duration)
	}
	var lastSent *messagev1.SendMessageResponse
	var lastNotify serverFrame
	messageIndex := 0
	for {
		if cfg.duration == 0 && messageIndex >= cfg.messageCount {
			break
		}
		if cfg.duration > 0 && messageIndex > 0 && time.Now().After(deadline) {
			break
		}
		batchSize := 1
		if cfg.duration > 0 {
			batchSize = cfg.virtualUsers
		}
		for i := 0; i < batchSize; i++ {
			if cfg.duration == 0 && messageIndex >= cfg.messageCount {
				break
			}
			messageIndex++
			sent, err := sendMessage(ctx, cfg, client, messageIndex)
			if err != nil {
				return nil, serverFrame{}, messageIndex - 1, fmt.Errorf("send message %d: %w", messageIndex, err)
			}
			notify, err := waitNotify(ctx, cfg, conn, sent.GetConversationSeq(), sent.GetMessageId())
			if err != nil {
				return nil, serverFrame{}, messageIndex - 1, fmt.Errorf("wait notify %d: %w", messageIndex, err)
			}
			lastSent = sent
			lastNotify = notify
		}
	}
	if lastSent == nil {
		return nil, serverFrame{}, 0, errors.New("no message was sent")
	}
	return lastSent, lastNotify, messageIndex, nil
}

type gatewayConversationClient struct {
	gatewayv1.GatewayServiceClient
}

func (client gatewayConversationClient) GetSendContext(context.Context, *conversationv1.GetSendContextRequest, ...grpc.CallOption) (*conversationv1.GetSendContextResponse, error) {
	return nil, status.Error(codes.Unimplemented, "GetSendContext is service-internal")
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
	requestCtx, err := withUserFacingAuthMetadata(requestCtx, cfg, auth)
	if err != nil {
		return nil, err
	}
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

func sendMessage(ctx context.Context, cfg config, client messagev1.MessageServiceClient, index int) (*messagev1.SendMessageResponse, error) {
	payload, err := structpb.NewStruct(map[string]any{"text": fmt.Sprintf("NexusIM e2e demo message %d", index)})
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
	requestCtx, err = withUserFacingAuthMetadata(requestCtx, cfg, auth)
	if err != nil {
		return nil, err
	}
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
		ClientMsgId:    fmt.Sprintf("e2e-demo-message-%d", index),
		MessageType:    "TEXT",
		Payload:        payload,
	})
}

func pullInboxAtLeast(ctx context.Context, cfg config, client deliveryv1.DeliveryServiceClient, afterSeq int64, minSeq int64) (pullSummary, error) {
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
		requestCtx, err := withUserFacingAuthMetadata(requestCtx, cfg, auth)
		if err != nil {
			cancel()
			return pullSummary{}, err
		}
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
			AfterSeq:       afterSeq,
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
	requestCtx, err := withUserFacingAuthMetadata(requestCtx, cfg, auth)
	if err != nil {
		return nil, err
	}
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
	requestCtx, err := withUserFacingAuthMetadata(requestCtx, cfg, auth)
	if err != nil {
		return conversationListSummary{}, err
	}
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

func signPushGatewayToken(cfg config) (string, error) {
	return signGatewayAuthToken(cfg.pushAuthHMACSecret, cfg.pushAuthTokenTTL, demoAuth{
		tenantID:  cfg.tenantID,
		userID:    cfg.receiverUserID,
		deviceID:  cfg.receiverDevice,
		sessionID: "e2e-demo-receiver",
		traceID:   "e2e-demo-auth",
	}, "push-gateway")
}

func signGatewayAuthToken(secret string, ttl time.Duration, auth demoAuth, audience string) (string, error) {
	return gatewayauth.SignGatewayToken(secret, map[string]string{
		"tenant_id":  auth.tenantID,
		"user_id":    auth.userID,
		"device_id":  auth.deviceID,
		"session_id": auth.sessionID,
		"trace_id":   auth.traceID,
		"aud":        strings.TrimSpace(audience),
	}, time.Now().Add(ttl))
}

func gatewayAuthAudienceSummary(mode string, audience string) string {
	if strings.TrimSpace(mode) == "" {
		return ""
	}
	return normalizedGatewayAuthAudience(audience)
}

func normalizedGatewayAuthAudience(audience string) string {
	audience = strings.TrimSpace(audience)
	if audience == "" {
		return "api-gateway"
	}
	return audience
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
