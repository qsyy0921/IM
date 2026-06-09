package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math"
	"net/url"
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
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
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
	opError          = "error"
)

type clientFrame struct {
	Op             string `json:"op"`
	RequestID      string `json:"request_id,omitempty"`
	DeviceID       string `json:"device_id,omitempty"`
	ConversationID string `json:"conversation_id,omitempty"`
	ReceivedSeq    int64  `json:"received_seq,omitempty"`
}

type serverFrame struct {
	Op              string `json:"op"`
	RequestID       string `json:"request_id,omitempty"`
	SessionID       string `json:"session_id,omitempty"`
	EventID         string `json:"event_id,omitempty"`
	ConversationID  string `json:"conversation_id,omitempty"`
	ConversationSeq int64  `json:"conversation_seq,omitempty"`
	SourceEventID   string `json:"source_event_id,omitempty"`
	MessageID       string `json:"message_id,omitempty"`
	PullRequired    bool   `json:"pull_required,omitempty"`
	LastReceivedSeq int64  `json:"last_received_seq,omitempty"`
	Code            string `json:"code,omitempty"`
	Message         string `json:"message,omitempty"`
	Retryable       bool   `json:"retryable"`
}

type config struct {
	conversationTarget string
	messageTarget      string
	deliveryTarget     string
	pushURL            string
	resultDir          string
	pgDSN              string
	requestTimeout     time.Duration
	waitTimeout        time.Duration
	pollInterval       time.Duration
	tenantID           string
	conversationID     string
	ownerUserID        string
	receiverUserID     string
	receiverDeviceID   string
	receiverDeviceIDs  []string
	cleanup            bool
}

type summary struct {
	Commit                  string             `json:"commit"`
	CommitFull              string             `json:"commit_full"`
	GitDirty                bool               `json:"git_dirty"`
	GitStatusShort          string             `json:"git_status_short,omitempty"`
	ConversationTarget      string             `json:"conversation_target"`
	MessageTarget           string             `json:"message_target"`
	DeliveryTarget          string             `json:"delivery_target"`
	PushURL                 string             `json:"push_url"`
	TenantID                string             `json:"tenant_id"`
	ConversationID          string             `json:"conversation_id"`
	OwnerUserID             string             `json:"owner_user_id"`
	ReceiverUserID          string             `json:"receiver_user_id"`
	ReceiverDeviceID        string             `json:"receiver_device_id"`
	ReceiverDeviceIDs       []string           `json:"receiver_device_ids,omitempty"`
	StartedAt               time.Time          `json:"started_at"`
	FinishedAt              time.Time          `json:"finished_at"`
	Success                 bool               `json:"success"`
	Error                   string             `json:"error,omitempty"`
	ServerHello             frameSnapshot      `json:"server_hello"`
	MemberJoin              memberJoinSummary  `json:"member_join"`
	SendMessage             sendSummary        `json:"send_message"`
	DeliveryNotify          frameSnapshot      `json:"delivery_notify"`
	DeviceNotifications     []deviceSummary    `json:"device_notifications,omitempty"`
	PullInbox               pullSummary        `json:"pull_inbox"`
	DeliveryAckOK           frameSnapshot      `json:"delivery_ack_ok"`
	CursorLastReceivedSeq   *int64             `json:"cursor_last_received_seq,omitempty"`
	UserInboxCount          *int64             `json:"user_inbox_count,omitempty"`
	DeliveryOutboxTotal     *int64             `json:"delivery_outbox_total,omitempty"`
	DeliveryOutboxPending   *int64             `json:"delivery_outbox_pending,omitempty"`
	DeliveryOutboxPublished *int64             `json:"delivery_outbox_published,omitempty"`
	DeliveryOutboxDLQ       *int64             `json:"delivery_outbox_dlq,omitempty"`
	Latencies               map[string]float64 `json:"latencies_ms"`
}

type deviceSummary struct {
	DeviceID              string        `json:"device_id"`
	ServerHello           frameSnapshot `json:"server_hello"`
	DeliveryNotify        frameSnapshot `json:"delivery_notify"`
	DeliveryAckOK         frameSnapshot `json:"delivery_ack_ok"`
	CursorLastReceivedSeq *int64        `json:"cursor_last_received_seq,omitempty"`
}

type frameSnapshot struct {
	Op              string `json:"op"`
	RequestID       string `json:"request_id,omitempty"`
	SessionID       string `json:"session_id,omitempty"`
	EventID         string `json:"event_id,omitempty"`
	ConversationID  string `json:"conversation_id,omitempty"`
	ConversationSeq int64  `json:"conversation_seq,omitempty"`
	SourceEventID   string `json:"source_event_id,omitempty"`
	MessageID       string `json:"message_id,omitempty"`
	PullRequired    bool   `json:"pull_required,omitempty"`
	LastReceivedSeq int64  `json:"last_received_seq,omitempty"`
	Code            string `json:"code,omitempty"`
	Message         string `json:"message,omitempty"`
	Retryable       bool   `json:"retryable,omitempty"`
}

type memberJoinSummary struct {
	ChangeID          string `json:"change_id"`
	BoundarySeq       int64  `json:"boundary_seq"`
	MemberVersion     int64  `json:"member_version"`
	PermissionVersion int64  `json:"permission_version"`
}

type sendSummary struct {
	MessageID       string `json:"message_id"`
	ConversationSeq int64  `json:"conversation_seq"`
}

type pullSummary struct {
	ItemCount int     `json:"item_count"`
	MaxSeq    int64   `json:"max_seq"`
	Items     []item  `json:"items"`
	P95MS     float64 `json:"p95_ms"`
	P99MS     float64 `json:"p99_ms"`
}

type item struct {
	ConversationSeq int64  `json:"conversation_seq"`
	EventID         string `json:"event_id"`
	MessageID       string `json:"message_id"`
	SenderID        string `json:"sender_id"`
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
	flag.StringVar(&cfg.conversationTarget, "conversation-target", "127.0.0.1:11596", "conversation-service gRPC target")
	flag.StringVar(&cfg.messageTarget, "message-target", "127.0.0.1:11595", "message-service gRPC target")
	flag.StringVar(&cfg.deliveryTarget, "delivery-target", "127.0.0.1:11597", "delivery-service gRPC target")
	flag.StringVar(&cfg.pushURL, "push-url", "ws://127.0.0.1:11598", "push-gateway WebSocket URL")
	flag.StringVar(&cfg.resultDir, "result-dir", "H:/NexusIM/loadtest-results/push-gateway-smoke", "result directory")
	flag.StringVar(&cfg.pgDSN, "pg-dsn", "", "PostgreSQL DSN")
	flag.DurationVar(&cfg.requestTimeout, "request-timeout", 3*time.Second, "per request timeout")
	flag.DurationVar(&cfg.waitTimeout, "wait-timeout", 20*time.Second, "wait timeout for async chain")
	flag.DurationVar(&cfg.pollInterval, "poll-interval", 200*time.Millisecond, "poll interval")
	flag.StringVar(&cfg.tenantID, "tenant-id", "tenant-push-smoke", "tenant id")
	flag.StringVar(&cfg.conversationID, "conversation-id", "conv-push-smoke", "conversation id")
	flag.StringVar(&cfg.ownerUserID, "owner-user-id", "owner-1", "owner/sender user id")
	flag.StringVar(&cfg.receiverUserID, "receiver-user-id", "push-user-1", "online receiver user id")
	flag.StringVar(&cfg.receiverDeviceID, "receiver-device-id", "push-device-1", "online receiver device id")
	var receiverDeviceIDs string
	flag.StringVar(&receiverDeviceIDs, "receiver-device-ids", "", "comma separated online receiver device ids; overrides receiver-device-id when set")
	flag.BoolVar(&cfg.cleanup, "cleanup", true, "delete existing rows for tenant before running")
	flag.Parse()
	cfg.receiverDeviceIDs = parseDeviceIDs(receiverDeviceIDs, cfg.receiverDeviceID)
	cfg.receiverDeviceID = cfg.receiverDeviceIDs[0]
	return cfg
}

func run(cfg config) error {
	if cfg.pgDSN == "" {
		return errors.New("pg-dsn is required")
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

	conversationConn, err := grpc.NewClient(cfg.conversationTarget, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("dial conversation-service: %w", err)
	}
	defer conversationConn.Close()
	conversationClient := conversationv1.NewConversationServiceClient(conversationConn)

	messageConn, err := grpc.NewClient(cfg.messageTarget, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("dial message-service: %w", err)
	}
	defer messageConn.Close()
	messageClient := messagev1.NewMessageServiceClient(messageConn)

	deliveryConn, err := grpc.NewClient(cfg.deliveryTarget, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("dial delivery-service: %w", err)
	}
	defer deliveryConn.Close()
	deliveryClient := deliveryv1.NewDeliveryServiceClient(deliveryConn)

	result := summary{
		Commit:             shortCommit(),
		CommitFull:         fullCommit(),
		GitDirty:           gitDirty(),
		GitStatusShort:     gitStatusShort(),
		ConversationTarget: cfg.conversationTarget,
		MessageTarget:      cfg.messageTarget,
		DeliveryTarget:     cfg.deliveryTarget,
		PushURL:            cfg.pushURL,
		TenantID:           cfg.tenantID,
		ConversationID:     cfg.conversationID,
		OwnerUserID:        cfg.ownerUserID,
		ReceiverUserID:     cfg.receiverUserID,
		ReceiverDeviceID:   cfg.receiverDeviceID,
		ReceiverDeviceIDs:  cfg.receiverDeviceIDs,
		StartedAt:          time.Now().UTC(),
		Latencies:          map[string]float64{},
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
	send, err := sendMessage(ctx, cfg, messageClient)
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
		ackOK, err := ackViaWebSocket(ctx, cfg, device.conn, send.GetConversationSeq())
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

func connectWebSocket(ctx context.Context, cfg config, deviceID string) (*nhooyr.Conn, serverFrame, error) {
	u, err := url.Parse(cfg.pushURL)
	if err != nil {
		return nil, serverFrame{}, err
	}
	query := u.Query()
	query.Set("tenant_id", cfg.tenantID)
	query.Set("user_id", cfg.receiverUserID)
	query.Set("device_id", deviceID)
	u.RawQuery = query.Encode()
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	conn, _, err := nhooyr.Dial(requestCtx, u.String(), nil)
	if err != nil {
		return nil, serverFrame{}, err
	}
	if err := wsjson.Write(requestCtx, conn, clientFrame{
		Op:        opClientHello,
		RequestID: "push-smoke-hello-" + deviceID,
		DeviceID:  deviceID,
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
		return nil, serverFrame{}, fmt.Errorf("unexpected hello: %+v", hello)
	}
	return conn, hello, nil
}

func createReceiverJoin(
	ctx context.Context,
	cfg config,
	client conversationv1.ConversationServiceClient,
) (*conversationv1.CreateMemberChangeResponse, error) {
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	return client.CreateMemberChange(requestCtx, &conversationv1.CreateMemberChangeRequest{
		AuthContext: &conversationv1.AuthContext{
			TenantId:  cfg.tenantID,
			UserId:    cfg.ownerUserID,
			DeviceId:  "push-smoke-owner-device",
			SessionId: "push-smoke-owner-session",
			TraceId:   "push-smoke-join",
			RequestId: "push-smoke-join",
		},
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
) (*messagev1.SendMessageResponse, error) {
	payload, err := structpb.NewStruct(map[string]any{"text": "push gateway smoke"})
	if err != nil {
		return nil, err
	}
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	return client.SendMessage(requestCtx, &messagev1.SendMessageRequest{
		AuthContext: &messagev1.AuthContext{
			TenantId:  cfg.tenantID,
			UserId:    cfg.ownerUserID,
			DeviceId:  "push-smoke-owner-device",
			SessionId: "push-smoke-owner-session",
			TraceId:   "push-smoke-send",
			RequestId: "push-smoke-send",
		},
		ConversationId: cfg.conversationID,
		ClientMsgId:    "push-smoke-client-message-1",
		MessageType:    "TEXT",
		Payload:        payload,
	})
}

func waitNotify(ctx context.Context, cfg config, conn *nhooyr.Conn) (serverFrame, error) {
	readCtx, cancel := context.WithTimeout(ctx, cfg.waitTimeout)
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

func pullInbox(ctx context.Context, cfg config, client deliveryv1.DeliveryServiceClient) (pullSummary, error) {
	deadline := time.Now().Add(cfg.waitTimeout)
	latencies := make([]float64, 0, 8)
	for {
		requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
		begin := time.Now()
		response, err := client.PullInbox(requestCtx, &deliveryv1.PullInboxRequest{
			AuthContext: &deliveryv1.AuthContext{
				TenantId:  cfg.tenantID,
				UserId:    cfg.receiverUserID,
				DeviceId:  cfg.receiverDeviceID,
				SessionId: "push-smoke",
				TraceId:   "push-smoke-pull",
				RequestId: "push-smoke-pull",
			},
			ConversationId: cfg.conversationID,
			AfterSeq:       0,
			Limit:          100,
		})
		latencies = append(latencies, elapsedMS(begin))
		cancel()
		if err != nil {
			return pullSummary{}, err
		}
		if len(response.GetItems()) > 0 || time.Now().After(deadline) {
			result := pullSummary{ItemCount: len(response.GetItems())}
			for _, inboxItem := range response.GetItems() {
				if inboxItem.GetConversationSeq() > result.MaxSeq {
					result.MaxSeq = inboxItem.GetConversationSeq()
				}
				result.Items = append(result.Items, item{
					ConversationSeq: inboxItem.GetConversationSeq(),
					EventID:         inboxItem.GetEventId(),
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

func ackViaWebSocket(
	ctx context.Context,
	cfg config,
	conn *nhooyr.Conn,
	seq int64,
) (serverFrame, error) {
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	if err := wsjson.Write(requestCtx, conn, clientFrame{
		Op:             opDeliveryAck,
		RequestID:      "push-smoke-ack",
		ConversationID: cfg.conversationID,
		ReceivedSeq:    seq,
	}); err != nil {
		return serverFrame{}, err
	}
	var frame serverFrame
	if err := wsjson.Read(requestCtx, conn, &frame); err != nil {
		return serverFrame{}, err
	}
	if frame.Op != opDeliveryAckOK {
		return frame, fmt.Errorf("unexpected ack frame: %+v", frame)
	}
	return frame, nil
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
	return nil
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

func parseDeviceIDs(list string, fallback string) []string {
	if strings.TrimSpace(list) == "" {
		return []string{fallback}
	}
	seen := map[string]struct{}{}
	result := make([]string, 0)
	for _, raw := range strings.Split(list, ",") {
		deviceID := strings.TrimSpace(raw)
		if deviceID == "" {
			continue
		}
		if _, ok := seen[deviceID]; ok {
			continue
		}
		seen[deviceID] = struct{}{}
		result = append(result, deviceID)
	}
	if len(result) == 0 {
		return []string{fallback}
	}
	return result
}

func finish(cfg config, result *summary, runErr error) error {
	result.FinishedAt = time.Now().UTC()
	if runErr != nil {
		result.Success = false
		result.Error = runErr.Error()
	} else {
		result.Success = true
	}
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

func snapshotFrame(frame serverFrame) frameSnapshot {
	return frameSnapshot{
		Op:              frame.Op,
		RequestID:       frame.RequestID,
		SessionID:       frame.SessionID,
		EventID:         frame.EventID,
		ConversationID:  frame.ConversationID,
		ConversationSeq: frame.ConversationSeq,
		SourceEventID:   frame.SourceEventID,
		MessageID:       frame.MessageID,
		PullRequired:    frame.PullRequired,
		LastReceivedSeq: frame.LastReceivedSeq,
		Code:            frame.Code,
		Message:         frame.Message,
		Retryable:       frame.Retryable,
	}
}

func elapsedMS(begin time.Time) float64 {
	return float64(time.Since(begin).Microseconds()) / 1000
}

func percentile(values []float64, quantile float64) float64 {
	if len(values) == 0 {
		return 0
	}
	copied := append([]float64(nil), values...)
	sort.Float64s(copied)
	index := int(math.Ceil(float64(len(copied))*quantile)) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(copied) {
		index = len(copied) - 1
	}
	return copied[index]
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
