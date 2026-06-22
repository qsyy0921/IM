package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	conversationv1 "github.com/qsyy0921/IM/api/proto/nexusim/conversation/v1"
	deliveryv1 "github.com/qsyy0921/IM/api/proto/nexusim/delivery/v1"
	messagev1 "github.com/qsyy0921/IM/api/proto/nexusim/message/v1"
	"github.com/qsyy0921/IM/loadtest/internal/grpctls"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/structpb"
)

const (
	eventMessagePersisted = "message.persisted.v1"
	eventMessageEdited    = "message.edited.v1"
	originalMessageText   = "message edit smoke original"
	editedMessageText     = "message edit smoke updated"
	metadataTenantID      = "x-nexusim-tenant-id"
	metadataUserID        = "x-nexusim-user-id"
	metadataDeviceID      = "x-nexusim-device-id"
	metadataSessionID     = "x-nexusim-session-id"
	metadataTraceID       = "x-nexusim-trace-id"
	metadataRequestID     = "x-nexusim-request-id"
)

type config struct {
	conversationTarget   string
	messageTarget        string
	deliveryTarget       string
	conversationTLS      grpctls.Config
	messageTLS           grpctls.Config
	deliveryTLS          grpctls.Config
	resultDir            string
	pgDSN                string
	requestTimeout       time.Duration
	waitTimeout          time.Duration
	pollInterval         time.Duration
	tenantID             string
	conversationID       string
	ownerUserID          string
	receiverUserID       string
	receiverDeviceID     string
	deliveryGroup        string
	verifiedAuthMetadata bool
	cleanup              bool
}

type summary struct {
	Commit                  string           `json:"commit"`
	CommitFull              string           `json:"commit_full"`
	GitDirty                bool             `json:"git_dirty"`
	GitStatusShort          string           `json:"git_status_short,omitempty"`
	ConversationTarget      string           `json:"conversation_target"`
	MessageTarget           string           `json:"message_target"`
	DeliveryTarget          string           `json:"delivery_target"`
	ConversationTLSEnabled  bool             `json:"conversation_tls_enabled"`
	MessageTLSEnabled       bool             `json:"message_tls_enabled"`
	DeliveryTLSEnabled      bool             `json:"delivery_tls_enabled"`
	VerifiedAuthMetadata    bool             `json:"verified_auth_metadata"`
	TenantID                string           `json:"tenant_id"`
	ConversationID          string           `json:"conversation_id"`
	OwnerUserID             string           `json:"owner_user_id"`
	ReceiverUserID          string           `json:"receiver_user_id"`
	ReceiverDeviceID        string           `json:"receiver_device_id"`
	DeliveryConsumerGroup   string           `json:"delivery_consumer_group,omitempty"`
	StartedAt               time.Time        `json:"started_at"`
	FinishedAt              time.Time        `json:"finished_at"`
	Success                 bool             `json:"success"`
	Error                   string           `json:"error,omitempty"`
	MemberJoin              memberJoin       `json:"member_join"`
	SendMessage             sendMessage      `json:"send_message"`
	PullPersisted           pullResult       `json:"pull_persisted"`
	EditMessage             editMessage      `json:"edit_message"`
	PullEdited              pullResult       `json:"pull_edited"`
	AckDelivery             ackDelivery      `json:"ack_delivery"`
	MessageLogStatus        string           `json:"message_log_status,omitempty"`
	MessageLogPayload       string           `json:"message_log_payload,omitempty"`
	MessageLogEditedAt      string           `json:"message_log_edited_at,omitempty"`
	MessageChangeRows       *int64           `json:"message_change_rows,omitempty"`
	MessageChangeHistory    changeHistory    `json:"message_change_history,omitempty"`
	TimelineEventCounts     map[string]int64 `json:"timeline_event_counts,omitempty"`
	MessageOutboxCounts     map[string]int64 `json:"message_outbox_counts,omitempty"`
	UserInboxEventCounts    map[string]int64 `json:"user_inbox_event_counts,omitempty"`
	DeliveryOutboxCounts    map[string]int64 `json:"delivery_outbox_counts,omitempty"`
	CursorLastReceivedSeq   *int64           `json:"cursor_last_received_seq,omitempty"`
	DeliveryCheckpointValue *int64           `json:"delivery_checkpoint_offset_value,omitempty"`
}

type memberJoin struct {
	ChangeID         string `json:"change_id"`
	BoundarySeq      int64  `json:"boundary_seq"`
	SagaStatus       string `json:"saga_status"`
	IdempotentReplay bool   `json:"idempotent_replay"`
}

type sendMessage struct {
	MessageID        string `json:"message_id"`
	ConversationSeq  int64  `json:"conversation_seq"`
	IdempotentReplay bool   `json:"idempotent_replay"`
}

type editMessage struct {
	MessageID        string `json:"message_id"`
	ConversationSeq  int64  `json:"conversation_seq"`
	ChangeVersion    int32  `json:"change_version"`
	IdempotentReplay bool   `json:"idempotent_replay"`
}

type changeHistory struct {
	ChangeType    string `json:"change_type,omitempty"`
	BeforePayload string `json:"before_payload,omitempty"`
	AfterPayload  string `json:"after_payload,omitempty"`
	BeforeStatus  string `json:"before_status,omitempty"`
	AfterStatus   string `json:"after_status,omitempty"`
	ChangeVersion int32  `json:"change_version,omitempty"`
}

type ackDelivery struct {
	LastReceivedSeq int64 `json:"last_received_seq"`
}

type pullResult struct {
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

type verifiedAuthIdentity struct {
	tenantID  string
	userID    string
	deviceID  string
	sessionID string
	traceID   string
	requestID string
}

func main() {
	cfg := parseConfig()
	if err := run(cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func parseConfig() config {
	now := time.Now().Format("20060102-150405")
	var cfg config
	flag.StringVar(&cfg.conversationTarget, "conversation-target", "127.0.0.1:11596", "conversation-service gRPC target")
	flag.StringVar(&cfg.messageTarget, "message-target", "127.0.0.1:11595", "message-service gRPC target")
	flag.StringVar(&cfg.deliveryTarget, "delivery-target", "127.0.0.1:11597", "delivery-service gRPC target")
	registerTLSFlags("conversation-tls", "NEXUSIM_CONVERSATION_TLS", "conversation-service", &cfg.conversationTLS)
	registerTLSFlags("message-tls", "NEXUSIM_MESSAGE_TLS", "message-service", &cfg.messageTLS)
	registerTLSFlags("delivery-tls", "NEXUSIM_DELIVERY_TLS", "delivery-service", &cfg.deliveryTLS)
	flag.StringVar(&cfg.resultDir, "result-dir", filepath.Join(`H:\NexusIM\loadtest-results`, "message-edit-smoke-"+now), "result directory")
	flag.StringVar(&cfg.pgDSN, "pg-dsn", "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable", "PostgreSQL DSN")
	flag.DurationVar(&cfg.requestTimeout, "request-timeout", 3*time.Second, "per-request timeout")
	flag.DurationVar(&cfg.waitTimeout, "wait-timeout", 20*time.Second, "poll wait timeout")
	flag.DurationVar(&cfg.pollInterval, "poll-interval", 200*time.Millisecond, "poll interval")
	flag.StringVar(&cfg.tenantID, "tenant-id", "tenant-message-edit-smoke", "tenant id")
	flag.StringVar(&cfg.conversationID, "conversation-id", "conv-message-edit-smoke", "conversation id")
	flag.StringVar(&cfg.ownerUserID, "owner-user-id", "owner-1", "owner user id")
	flag.StringVar(&cfg.receiverUserID, "receiver-user-id", "edit-user-1", "receiver user id")
	flag.StringVar(&cfg.receiverDeviceID, "receiver-device-id", "edit-device-1", "receiver device id")
	flag.StringVar(&cfg.deliveryGroup, "delivery-consumer-group", "", "optional delivery timeline consumer group for checkpoint stats")
	flag.BoolVar(&cfg.verifiedAuthMetadata, "verified-auth-metadata", envBool(false, "NEXUSIM_MESSAGE_EDIT_VERIFIED_AUTH_METADATA", "NEXUSIM_MESSAGE_MUTATION_VERIFIED_AUTH_METADATA"), "send gateway verified identity through user-facing gRPC metadata")
	flag.BoolVar(&cfg.cleanup, "cleanup", true, "delete tenant test data and seed a fresh conversation before running")
	flag.Parse()
	if cfg.requestTimeout <= 0 {
		cfg.requestTimeout = 3 * time.Second
	}
	if cfg.waitTimeout <= 0 {
		cfg.waitTimeout = 20 * time.Second
	}
	if cfg.pollInterval <= 0 {
		cfg.pollInterval = 200 * time.Millisecond
	}
	return cfg
}

func registerTLSFlags(prefix string, envPrefix string, serviceName string, config *grpctls.Config) {
	flag.StringVar(&config.CAFile, prefix+"-ca-file", os.Getenv(envPrefix+"_CA_FILE"), "CA PEM for "+serviceName+" gRPC TLS")
	flag.StringVar(&config.ServerName, prefix+"-server-name", os.Getenv(envPrefix+"_SERVER_NAME"), "override server name for "+serviceName+" gRPC TLS")
	flag.StringVar(&config.ClientCertFile, prefix+"-client-cert-file", os.Getenv(envPrefix+"_CLIENT_CERT_FILE"), "client certificate PEM for "+serviceName+" gRPC mTLS")
	flag.StringVar(&config.ClientKeyFile, prefix+"-client-key-file", os.Getenv(envPrefix+"_CLIENT_KEY_FILE"), "client private key PEM for "+serviceName+" gRPC mTLS")
}

func run(cfg config) error {
	if err := os.MkdirAll(cfg.resultDir, 0o755); err != nil {
		return fmt.Errorf("create result dir: %w", err)
	}
	ctx := context.Background()
	result := summary{
		Commit:                 shortCommit(),
		CommitFull:             fullCommit(),
		GitDirty:               gitDirty(),
		GitStatusShort:         gitStatusShort(),
		ConversationTarget:     cfg.conversationTarget,
		MessageTarget:          cfg.messageTarget,
		DeliveryTarget:         cfg.deliveryTarget,
		ConversationTLSEnabled: cfg.conversationTLS.Enabled(),
		MessageTLSEnabled:      cfg.messageTLS.Enabled(),
		DeliveryTLSEnabled:     cfg.deliveryTLS.Enabled(),
		VerifiedAuthMetadata:   cfg.verifiedAuthMetadata,
		TenantID:               cfg.tenantID,
		ConversationID:         cfg.conversationID,
		OwnerUserID:            cfg.ownerUserID,
		ReceiverUserID:         cfg.receiverUserID,
		ReceiverDeviceID:       cfg.receiverDeviceID,
		DeliveryConsumerGroup:  cfg.deliveryGroup,
		StartedAt:              time.Now().UTC(),
	}
	var runErr error
	defer func() {
		_ = finish(cfg, &result, runErr)
	}()

	pool, err := pgxpool.New(ctx, cfg.pgDSN)
	if err != nil {
		runErr = fmt.Errorf("connect postgres: %w", err)
		return runErr
	}
	defer pool.Close()
	if cfg.cleanup {
		if err := cleanupTenant(ctx, pool, cfg.tenantID); err != nil {
			runErr = err
			return runErr
		}
		if err := seedConversation(ctx, pool, cfg); err != nil {
			runErr = err
			return runErr
		}
	}

	conversationDialOption, err := grpctls.DialOption(cfg.conversationTLS, "conversation-tls")
	if err != nil {
		runErr = fmt.Errorf("configure conversation-service TLS: %w", err)
		return runErr
	}
	conversationConn, err := grpc.NewClient(cfg.conversationTarget, conversationDialOption)
	if err != nil {
		runErr = fmt.Errorf("dial conversation-service: %w", err)
		return runErr
	}
	defer conversationConn.Close()
	messageDialOption, err := grpctls.DialOption(cfg.messageTLS, "message-tls")
	if err != nil {
		runErr = fmt.Errorf("configure message-service TLS: %w", err)
		return runErr
	}
	messageConn, err := grpc.NewClient(cfg.messageTarget, messageDialOption)
	if err != nil {
		runErr = fmt.Errorf("dial message-service: %w", err)
		return runErr
	}
	defer messageConn.Close()
	deliveryDialOption, err := grpctls.DialOption(cfg.deliveryTLS, "delivery-tls")
	if err != nil {
		runErr = fmt.Errorf("configure delivery-service TLS: %w", err)
		return runErr
	}
	deliveryConn, err := grpc.NewClient(cfg.deliveryTarget, deliveryDialOption)
	if err != nil {
		runErr = fmt.Errorf("dial delivery-service: %w", err)
		return runErr
	}
	defer deliveryConn.Close()

	conversationClient := conversationv1.NewConversationServiceClient(conversationConn)
	messageClient := messagev1.NewMessageServiceClient(messageConn)
	deliveryClient := deliveryv1.NewDeliveryServiceClient(deliveryConn)

	join, err := createReceiverJoin(ctx, cfg, conversationClient)
	if err != nil {
		runErr = fmt.Errorf("create receiver join: %w", err)
		return runErr
	}
	result.MemberJoin = memberJoin{
		ChangeID:         join.GetChangeId(),
		BoundarySeq:      join.GetBoundarySeq(),
		SagaStatus:       join.GetStatus().String(),
		IdempotentReplay: join.GetIdempotentReplay(),
	}
	if err := waitMembership(ctx, pool, cfg); err != nil {
		runErr = err
		return runErr
	}

	send, err := sendMessageOnce(ctx, cfg, messageClient)
	if err != nil {
		runErr = fmt.Errorf("send message: %w", err)
		return runErr
	}
	result.SendMessage = sendMessage{
		MessageID:        send.GetMessageId(),
		ConversationSeq:  send.GetConversationSeq(),
		IdempotentReplay: send.GetIdempotentReplay(),
	}
	persistedPull, err := pullInboxUntil(ctx, cfg, deliveryClient, 0, send.GetConversationSeq(), eventMessagePersisted, send.GetMessageId())
	if err != nil {
		runErr = fmt.Errorf("pull persisted inbox: %w", err)
		return runErr
	}
	result.PullPersisted = persistedPull

	edit, err := editMessageOnce(ctx, cfg, messageClient, send.GetMessageId())
	if err != nil {
		runErr = fmt.Errorf("edit message: %w", err)
		return runErr
	}
	result.EditMessage = editMessage{
		MessageID:        edit.GetMessageId(),
		ConversationSeq:  edit.GetConversationSeq(),
		ChangeVersion:    edit.GetChangeVersion(),
		IdempotentReplay: edit.GetIdempotentReplay(),
	}
	editedPull, err := pullInboxUntil(ctx, cfg, deliveryClient, send.GetConversationSeq(), edit.GetConversationSeq(), eventMessageEdited, send.GetMessageId())
	if err != nil {
		runErr = fmt.Errorf("pull edited inbox: %w", err)
		return runErr
	}
	result.PullEdited = editedPull

	ack, err := ackDeliveryOnce(ctx, cfg, deliveryClient, edit.GetConversationSeq())
	if err != nil {
		runErr = fmt.Errorf("ack edit seq: %w", err)
		return runErr
	}
	result.AckDelivery = ackDelivery{LastReceivedSeq: ack.GetLastReceivedSeq()}
	if err := waitDeliveryOutboxDrained(ctx, pool, cfg); err != nil {
		runErr = err
		return runErr
	}
	if err := fillPostgresStats(ctx, pool, cfg, send.GetMessageId(), &result); err != nil {
		runErr = err
		return runErr
	}
	runErr = nil
	return nil
}

func createReceiverJoin(ctx context.Context, cfg config, client conversationv1.ConversationServiceClient) (*conversationv1.CreateMemberChangeResponse, error) {
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	auth := verifiedAuthIdentity{
		tenantID:  cfg.tenantID,
		userID:    cfg.ownerUserID,
		deviceID:  "message-edit-owner-device",
		sessionID: "message-edit-owner-session",
		traceID:   "message-edit-join",
		requestID: "message-edit-join",
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
		IdempotencyKey:        "message-edit-join-receiver",
		ConflictPolicy:        conversationv1.MemberChangeConflictPolicy_MEMBER_CHANGE_CONFLICT_POLICY_REJECT,
		Reason:                "message edit smoke receiver join",
	})
}

func sendMessageOnce(ctx context.Context, cfg config, client messagev1.MessageServiceClient) (*messagev1.SendMessageResponse, error) {
	payload, err := structpb.NewStruct(map[string]any{"text": originalMessageText})
	if err != nil {
		return nil, err
	}
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	auth := verifiedAuthIdentity{
		tenantID:  cfg.tenantID,
		userID:    cfg.ownerUserID,
		deviceID:  "message-edit-owner-device",
		sessionID: "message-edit-owner-session",
		traceID:   "message-edit-send",
		requestID: "message-edit-send",
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
		ClientMsgId:    "message-edit-client-message-1",
		MessageType:    "TEXT",
		Payload:        payload,
	})
}

func editMessageOnce(ctx context.Context, cfg config, client messagev1.MessageServiceClient, messageID string) (*messagev1.MessageChangeResponse, error) {
	payload, err := structpb.NewStruct(map[string]any{"text": editedMessageText})
	if err != nil {
		return nil, err
	}
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	auth := verifiedAuthIdentity{
		tenantID:  cfg.tenantID,
		userID:    cfg.ownerUserID,
		deviceID:  "message-edit-owner-device",
		sessionID: "message-edit-owner-session",
		traceID:   "message-edit-edit",
		requestID: "message-edit-edit",
	}
	requestCtx = withVerifiedAuthMetadata(requestCtx, cfg, auth)
	return client.EditMessage(requestCtx, &messagev1.EditMessageRequest{
		AuthContext: &messagev1.AuthContext{
			TenantId:  auth.tenantID,
			UserId:    auth.userID,
			DeviceId:  auth.deviceID,
			SessionId: auth.sessionID,
			TraceId:   auth.traceID,
			RequestId: auth.requestID,
		},
		ConversationId: cfg.conversationID,
		MessageId:      messageID,
		Payload:        payload,
		IdempotencyKey: "message-edit-edit-1",
		Reason:         "message edit smoke",
	})
}

func pullInboxUntil(
	ctx context.Context,
	cfg config,
	client deliveryv1.DeliveryServiceClient,
	afterSeq int64,
	minSeq int64,
	eventType string,
	messageID string,
) (pullResult, error) {
	deadline := time.Now().Add(cfg.waitTimeout)
	var last pullResult
	for {
		requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
		auth := verifiedAuthIdentity{
			tenantID:  cfg.tenantID,
			userID:    cfg.receiverUserID,
			deviceID:  cfg.receiverDeviceID,
			sessionID: "message-edit-pull",
			traceID:   "message-edit-pull",
			requestID: "message-edit-pull",
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
			AfterSeq:       afterSeq,
			Limit:          100,
		})
		cancel()
		if err != nil {
			return pullResult{}, err
		}
		last = summarizePull(response.GetItems())
		if containsInboxItem(last.Items, minSeq, eventType, messageID) {
			return last, nil
		}
		if time.Now().After(deadline) {
			return last, fmt.Errorf("timed out waiting for inbox event_type=%s message_id=%s min_seq=%d; last=%+v", eventType, messageID, minSeq, last)
		}
		time.Sleep(cfg.pollInterval)
	}
}

func summarizePull(items []*deliveryv1.InboxItem) pullResult {
	result := pullResult{ItemCount: len(items)}
	for _, pbItem := range items {
		if pbItem.GetConversationSeq() > result.MaxSeq {
			result.MaxSeq = pbItem.GetConversationSeq()
		}
		result.Items = append(result.Items, inboxItem{
			ConversationSeq: pbItem.GetConversationSeq(),
			EventID:         pbItem.GetEventId(),
			EventType:       pbItem.GetEventType(),
			MessageID:       pbItem.GetMessageId(),
			SenderID:        pbItem.GetSenderId(),
		})
	}
	sort.Slice(result.Items, func(i, j int) bool {
		return result.Items[i].ConversationSeq < result.Items[j].ConversationSeq
	})
	return result
}

func containsInboxItem(items []inboxItem, minSeq int64, eventType string, messageID string) bool {
	for _, item := range items {
		if item.ConversationSeq >= minSeq && item.EventType == eventType && item.MessageID == messageID {
			return true
		}
	}
	return false
}

func ackDeliveryOnce(ctx context.Context, cfg config, client deliveryv1.DeliveryServiceClient, seq int64) (*deliveryv1.AckDeliveryResponse, error) {
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	auth := verifiedAuthIdentity{
		tenantID:  cfg.tenantID,
		userID:    cfg.receiverUserID,
		deviceID:  cfg.receiverDeviceID,
		sessionID: "message-edit-ack",
		traceID:   "message-edit-ack",
		requestID: "message-edit-ack",
	}
	requestCtx = withVerifiedAuthMetadata(requestCtx, cfg, auth)
	return client.AckDelivery(requestCtx, &deliveryv1.AckDeliveryRequest{
		AuthContext: &deliveryv1.AuthContext{
			TenantId:  auth.tenantID,
			UserId:    auth.userID,
			DeviceId:  auth.deviceID,
			SessionId: auth.sessionID,
			TraceId:   auth.traceID,
			RequestId: auth.requestID,
		},
		ConversationId: cfg.conversationID,
		ReceivedSeq:    seq,
	})
}

func waitDeliveryOutboxDrained(ctx context.Context, pool *pgxpool.Pool, cfg config) error {
	deadline := time.Now().Add(cfg.waitTimeout)
	for time.Now().Before(deadline) {
		var pending int64
		err := pool.QueryRow(ctx, `
SELECT COUNT(*) FROM delivery_outbox
WHERE tenant_id = $1 AND conversation_id = $2 AND status = 'PENDING'
`, cfg.tenantID, cfg.conversationID).Scan(&pending)
		if err != nil {
			return fmt.Errorf("query delivery outbox pending: %w", err)
		}
		if pending == 0 {
			return nil
		}
		time.Sleep(cfg.pollInterval)
	}
	return errors.New("timed out waiting for delivery outbox to drain")
}

func cleanupTenant(ctx context.Context, pool *pgxpool.Pool, tenantID string) error {
	statements := []string{
		`DELETE FROM delivery_outbox WHERE tenant_id = $1`,
		`DELETE FROM device_delivery_cursors WHERE tenant_id = $1`,
		`DELETE FROM user_inbox WHERE tenant_id = $1`,
		`DELETE FROM delivery_membership_projection WHERE tenant_id = $1`,
		`DELETE FROM message_outbox WHERE tenant_id = $1`,
		`DELETE FROM conversation_timeline_events WHERE tenant_id = $1`,
		`DELETE FROM message_change_history WHERE tenant_id = $1`,
		`DELETE FROM message_command_idempotency WHERE tenant_id = $1`,
		`DELETE FROM message_log WHERE tenant_id = $1`,
		`DELETE FROM conversation_seq WHERE tenant_id = $1`,
		`DELETE FROM member_change_saga WHERE tenant_id = $1`,
		`DELETE FROM conversation_members WHERE tenant_id = $1`,
		`DELETE FROM conversations WHERE tenant_id = $1`,
	}
	for _, statement := range statements {
		if _, err := pool.Exec(ctx, statement, tenantID); err != nil {
			return fmt.Errorf("cleanup tenant using %q: %w", statement, err)
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
		if err != nil {
			return fmt.Errorf("wait delivery membership projection: %w", err)
		}
		time.Sleep(cfg.pollInterval)
	}
	return errors.New("timed out waiting for delivery membership projection")
}

func fillPostgresStats(ctx context.Context, pool *pgxpool.Pool, cfg config, messageID string, result *summary) error {
	var (
		status        string
		payload       string
		editedAt      *time.Time
		changeType    string
		beforePayload string
		afterPayload  string
		beforeStatus  string
		afterStatus   string
		changeVersion int32
	)
	if err := pool.QueryRow(ctx, `
SELECT
    ml.status,
    ml.payload_json::text,
    ml.edited_at,
    mch.change_type,
    mch.before_payload_json::text,
    mch.after_payload_json::text,
    mch.before_status,
    mch.after_status,
    mch.change_version
FROM message_log ml
JOIN message_change_history mch
  ON mch.tenant_id = ml.tenant_id
 AND mch.conversation_id = ml.conversation_id
 AND mch.message_id = ml.message_id
WHERE ml.tenant_id = $1
  AND ml.conversation_id = $2
  AND ml.message_id = $3
ORDER BY mch.change_version DESC
LIMIT 1
`, cfg.tenantID, cfg.conversationID, messageID).Scan(
		&status,
		&payload,
		&editedAt,
		&changeType,
		&beforePayload,
		&afterPayload,
		&beforeStatus,
		&afterStatus,
		&changeVersion,
	); err != nil {
		return fmt.Errorf("query edited message facts: %w", err)
	}
	result.MessageLogStatus = status
	result.MessageLogPayload = payload
	if editedAt != nil {
		result.MessageLogEditedAt = editedAt.UTC().Format(time.RFC3339Nano)
	}
	result.MessageChangeHistory = changeHistory{
		ChangeType:    changeType,
		BeforePayload: beforePayload,
		AfterPayload:  afterPayload,
		BeforeStatus:  beforeStatus,
		AfterStatus:   afterStatus,
		ChangeVersion: changeVersion,
	}
	if status != "EDITED" {
		return fmt.Errorf("message status = %s, want EDITED", status)
	}
	if editedAt == nil {
		return errors.New("message edited_at is null")
	}
	if changeType != "EDIT" || beforeStatus != "NORMAL" || afterStatus != "EDITED" {
		return fmt.Errorf("unexpected change history: type=%s before_status=%s after_status=%s", changeType, beforeStatus, afterStatus)
	}
	if !jsonTextPayloadEquals(payload, editedMessageText) {
		return fmt.Errorf("message payload = %s, want text=%q", payload, editedMessageText)
	}
	if !jsonTextPayloadEquals(beforePayload, originalMessageText) {
		return fmt.Errorf("before payload = %s, want text=%q", beforePayload, originalMessageText)
	}
	if !jsonTextPayloadEquals(afterPayload, editedMessageText) {
		return fmt.Errorf("after payload = %s, want text=%q", afterPayload, editedMessageText)
	}
	assignInt64 := func(target **int64, query string, args ...any) error {
		var value int64
		if err := pool.QueryRow(ctx, query, args...).Scan(&value); err != nil {
			return err
		}
		*target = &value
		return nil
	}
	if err := assignInt64(&result.MessageChangeRows, `
SELECT COUNT(*) FROM message_change_history
WHERE tenant_id = $1 AND conversation_id = $2 AND message_id = $3
`, cfg.tenantID, cfg.conversationID, messageID); err != nil {
		return fmt.Errorf("query message change rows: %w", err)
	}
	var err error
	result.TimelineEventCounts, err = queryCounts(ctx, pool, `
SELECT event_type, COUNT(*) FROM conversation_timeline_events
WHERE tenant_id = $1 AND conversation_id = $2
GROUP BY event_type
`, cfg.tenantID, cfg.conversationID)
	if err != nil {
		return fmt.Errorf("query timeline event counts: %w", err)
	}
	result.MessageOutboxCounts, err = queryCounts(ctx, pool, `
SELECT status, COUNT(*) FROM message_outbox
WHERE tenant_id = $1 AND conversation_id = $2
GROUP BY status
`, cfg.tenantID, cfg.conversationID)
	if err != nil {
		return fmt.Errorf("query message outbox counts: %w", err)
	}
	result.UserInboxEventCounts, err = queryCounts(ctx, pool, `
SELECT event_type, COUNT(*) FROM user_inbox
WHERE tenant_id = $1 AND conversation_id = $2 AND user_id = $3
GROUP BY event_type
`, cfg.tenantID, cfg.conversationID, cfg.receiverUserID)
	if err != nil {
		return fmt.Errorf("query user inbox event counts: %w", err)
	}
	result.DeliveryOutboxCounts, err = queryCounts(ctx, pool, `
SELECT status, COUNT(*) FROM delivery_outbox
WHERE tenant_id = $1 AND conversation_id = $2
GROUP BY status
`, cfg.tenantID, cfg.conversationID)
	if err != nil {
		return fmt.Errorf("query delivery outbox counts: %w", err)
	}
	if err := assignInt64(&result.CursorLastReceivedSeq, `
SELECT COALESCE(MAX(last_received_seq), 0)
FROM device_delivery_cursors
WHERE tenant_id = $1 AND conversation_id = $2 AND user_id = $3 AND device_id = $4
`, cfg.tenantID, cfg.conversationID, cfg.receiverUserID, cfg.receiverDeviceID); err != nil {
		return fmt.Errorf("query cursor: %w", err)
	}
	if cfg.deliveryGroup != "" {
		if err := assignInt64(&result.DeliveryCheckpointValue, `
SELECT COALESCE(MAX(offset_value), 0)
FROM delivery_kafka_checkpoints
WHERE consumer_group = $1
`, cfg.deliveryGroup); err != nil {
			return fmt.Errorf("query delivery checkpoint: %w", err)
		}
	}
	return nil
}

func jsonTextPayloadEquals(raw string, expectedText string) bool {
	var payload map[string]string
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return false
	}
	return len(payload) == 1 && payload["text"] == expectedText
}

func queryCounts(ctx context.Context, pool *pgxpool.Pool, query string, args ...any) (map[string]int64, error) {
	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]int64{}
	for rows.Next() {
		var key string
		var value int64
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		result[key] = value
	}
	return result, rows.Err()
}

func withVerifiedAuthMetadata(ctx context.Context, cfg config, auth verifiedAuthIdentity) context.Context {
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

func envBool(defaultValue bool, names ...string) bool {
	for _, name := range names {
		value := strings.TrimSpace(os.Getenv(name))
		if value == "" {
			continue
		}
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return defaultValue
		}
		return parsed
	}
	return defaultValue
}

func finish(cfg config, result *summary, runErr error) error {
	result.FinishedAt = time.Now().UTC()
	if runErr != nil {
		result.Success = false
		result.Error = runErr.Error()
	} else {
		result.Success = true
	}
	path := filepath.Join(cfg.resultDir, "message-edit-summary.json")
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	if writeErr := os.WriteFile(path, data, 0o644); writeErr != nil {
		return writeErr
	}
	if runErr != nil {
		return runErr
	}
	return nil
}

func shortCommit() string {
	value := strings.TrimSpace(commandOutput("git", "rev-parse", "--short", "HEAD"))
	if value == "" {
		return "unknown"
	}
	if gitDirty() {
		return value + "-dirty"
	}
	return value
}

func fullCommit() string {
	value := strings.TrimSpace(commandOutput("git", "rev-parse", "HEAD"))
	if value == "" {
		return "unknown"
	}
	return value
}

func gitDirty() bool {
	return strings.TrimSpace(gitStatusShort()) != ""
}

func gitStatusShort() string {
	return strings.TrimSpace(commandOutput("git", "status", "--short"))
}

func commandOutput(name string, args ...string) string {
	cmd := exec.Command(name, args...)
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return string(output)
}
