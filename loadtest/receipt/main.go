package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math"
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
	receiptv1 "github.com/qsyy0921/IM/api/proto/nexusim/receipt/v1"
	"github.com/qsyy0921/IM/loadtest/internal/grpctls"
	receipteventsv1 "github.com/qsyy0921/IM/schemas/kafka/receipt/v1"
	kafkago "github.com/segmentio/kafka-go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

const (
	metadataTenantID  = "x-nexusim-tenant-id"
	metadataUserID    = "x-nexusim-user-id"
	metadataDeviceID  = "x-nexusim-device-id"
	metadataSessionID = "x-nexusim-session-id"
	metadataTraceID   = "x-nexusim-trace-id"
	metadataRequestID = "x-nexusim-request-id"
)

type config struct {
	conversationTarget   string
	messageTarget        string
	deliveryTarget       string
	receiptTarget        string
	conversationTLS      grpctls.Config
	messageTLS           grpctls.Config
	deliveryTLS          grpctls.Config
	receiptTLS           grpctls.Config
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
	receiptGroup         string
	kafkaBrokers         []string
	receiptEventsTopic   string
	receiptEventsGroup   string
	verifiedAuthMetadata bool
	cleanup              bool
}

type summary struct {
	Commit                                   string                  `json:"commit"`
	CommitFull                               string                  `json:"commit_full"`
	GitDirty                                 bool                    `json:"git_dirty"`
	GitStatusShort                           string                  `json:"git_status_short,omitempty"`
	ConversationTarget                       string                  `json:"conversation_target"`
	MessageTarget                            string                  `json:"message_target"`
	DeliveryTarget                           string                  `json:"delivery_target"`
	ReceiptTarget                            string                  `json:"receipt_target"`
	ConversationTLSEnabled                   bool                    `json:"conversation_tls_enabled"`
	MessageTLSEnabled                        bool                    `json:"message_tls_enabled"`
	DeliveryTLSEnabled                       bool                    `json:"delivery_tls_enabled"`
	ReceiptTLSEnabled                        bool                    `json:"receipt_tls_enabled"`
	VerifiedAuthMetadata                     bool                    `json:"verified_auth_metadata"`
	ResultDir                                string                  `json:"result_dir"`
	TenantID                                 string                  `json:"tenant_id"`
	ConversationID                           string                  `json:"conversation_id"`
	OwnerUserID                              string                  `json:"owner_user_id"`
	ReceiverUserID                           string                  `json:"receiver_user_id"`
	ReceiverDeviceID                         string                  `json:"receiver_device_id"`
	DeliveryConsumerGroup                    string                  `json:"delivery_consumer_group,omitempty"`
	ReceiptConsumerGroup                     string                  `json:"receipt_consumer_group,omitempty"`
	ReceiptEventsTopic                       string                  `json:"receipt_events_topic,omitempty"`
	ReceiptEventsGroup                       string                  `json:"receipt_events_group,omitempty"`
	StartedAt                                time.Time               `json:"started_at"`
	FinishedAt                               time.Time               `json:"finished_at"`
	Success                                  bool                    `json:"success"`
	Error                                    string                  `json:"error,omitempty"`
	MemberJoin                               memberJoinSummary       `json:"member_join"`
	SendMessage                              sendSummary             `json:"send_message"`
	PullInbox                                pullSummary             `json:"pull_inbox"`
	AckDelivery                              ackSummary              `json:"ack_delivery"`
	ReceiptBeforeReadBySeq                   receiptStateSummary     `json:"receipt_before_read_by_seq"`
	ConversationListBefore                   conversationListSummary `json:"conversation_list_before_read"`
	ConversationListUnreadBeforeRead         conversationListSummary `json:"conversation_list_unread_before_read"`
	ReceiptAfterReadBySeq                    receiptStateSummary     `json:"receipt_after_read_by_seq"`
	ReceiptAfterReadByMsgID                  receiptStateSummary     `json:"receipt_after_read_by_message_id"`
	ConversationListAfter                    conversationListSummary `json:"conversation_list_after_read"`
	ConversationListUnreadAfterRead          conversationListSummary `json:"conversation_list_unread_after_read"`
	ArchiveConversation                      archiveSummary          `json:"archive_conversation"`
	ConversationListArchivedDefault          conversationListSummary `json:"conversation_list_archived_default"`
	ConversationListArchivedIncluded         conversationListSummary `json:"conversation_list_archived_included"`
	SendMessageWhileArchived                 sendSummary             `json:"send_message_while_archived"`
	PullInboxWhileArchived                   pullSummary             `json:"pull_inbox_while_archived"`
	AckDeliveryWhileArchived                 ackSummary              `json:"ack_delivery_while_archived"`
	ConversationListAfterArchivedNewDefault  conversationListSummary `json:"conversation_list_after_archived_new_message_default"`
	ConversationListAfterArchivedNewIncluded conversationListSummary `json:"conversation_list_after_archived_new_message_included"`
	UnarchiveConversation                    archiveSummary          `json:"unarchive_conversation"`
	ConversationListAfterUnarchive           conversationListSummary `json:"conversation_list_after_unarchive"`
	PinConversation                          pinSummary              `json:"pin_conversation"`
	ConversationListAfterPin                 conversationListSummary `json:"conversation_list_after_pin"`
	UnpinConversation                        pinSummary              `json:"unpin_conversation"`
	ConversationListAfterUnpin               conversationListSummary `json:"conversation_list_after_unpin"`
	MuteConversation                         muteSummary             `json:"mute_conversation"`
	ConversationListAfterMute                conversationListSummary `json:"conversation_list_after_mute"`
	UnmuteConversation                       muteSummary             `json:"unmute_conversation"`
	ConversationListAfterUnmute              conversationListSummary `json:"conversation_list_after_unmute"`
	MarkRead                                 markReadSummary         `json:"mark_read"`
	MarkReadTooFar                           negativeCallSummary     `json:"mark_read_too_far"`
	ReceiptProjection                        receiptProjectionStats  `json:"receipt_projection"`
	ReceiptOutbox                            receiptOutboxStats      `json:"receipt_outbox"`
	ReceiptKafkaEvents                       []receiptKafkaEvent     `json:"receipt_kafka_events"`
	DeliveryOutbox                           outboxStats             `json:"delivery_outbox"`
	LatenciesMS                              map[string]float64      `json:"latencies_ms"`
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
	ItemCount int          `json:"item_count"`
	MaxSeq    int64        `json:"max_seq"`
	Items     []pulledItem `json:"items"`
	P95MS     float64      `json:"p95_ms"`
	P99MS     float64      `json:"p99_ms"`
}

type pulledItem struct {
	ConversationSeq int64  `json:"conversation_seq"`
	EventID         string `json:"event_id"`
	MessageID       string `json:"message_id"`
	SenderID        string `json:"sender_id"`
}

type ackSummary struct {
	LastReceivedSeq int64   `json:"last_received_seq"`
	LatencyMS       float64 `json:"latency_ms"`
}

type markReadSummary struct {
	LastReadSeq int64   `json:"last_read_seq"`
	LatencyMS   float64 `json:"latency_ms"`
}

type archiveSummary struct {
	Archived  bool    `json:"archived"`
	LatencyMS float64 `json:"latency_ms"`
}

type pinSummary struct {
	Pinned    bool    `json:"pinned"`
	LatencyMS float64 `json:"latency_ms"`
}

type muteSummary struct {
	Muted     bool    `json:"muted"`
	LatencyMS float64 `json:"latency_ms"`
}

type negativeCallSummary struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Passed  bool   `json:"passed"`
}

type receiptStateSummary struct {
	RequestBy         string             `json:"request_by"`
	ConversationSeq   int64              `json:"conversation_seq"`
	MessageID         string             `json:"message_id"`
	ReceivedUserCount int32              `json:"received_user_count"`
	ReadUserCount     int32              `json:"read_user_count"`
	VisibilityMode    string             `json:"visibility_mode"`
	Receivers         []receiptUserState `json:"receivers"`
}

type receiptUserState struct {
	UserID           string `json:"user_id"`
	ReceivedSeq      int64  `json:"received_seq"`
	ReceivedAtUnixMS int64  `json:"received_at_unix_ms"`
	ReadSeq          int64  `json:"read_seq"`
	ReadAtUnixMS     int64  `json:"read_at_unix_ms"`
}

type conversationListSummary struct {
	ItemCount           int                        `json:"item_count"`
	Items               []conversationSummaryItem  `json:"items"`
	NextPageCursor      string                     `json:"next_page_cursor,omitempty"`
	ProjectionWatermark projectionWatermarkSummary `json:"projection_watermark"`
	LatencyMS           float64                    `json:"latency_ms"`
}

type conversationSummaryItem struct {
	ConversationID  string `json:"conversation_id"`
	LastVisibleSeq  int64  `json:"last_visible_seq"`
	LastMessageID   string `json:"last_message_id"`
	LastSenderID    string `json:"last_sender_id"`
	UnreadCount     int64  `json:"unread_count"`
	LastReadSeq     int64  `json:"last_read_seq"`
	UpdatedAtUnixMS int64  `json:"updated_at_unix_ms"`
	Archived        bool   `json:"archived"`
	Pinned          bool   `json:"pinned"`
	Muted           bool   `json:"muted"`
}

type projectionWatermarkSummary struct {
	Source          string `json:"source"`
	OffsetValue     int64  `json:"offset_value"`
	UpdatedAtUnixMS int64  `json:"updated_at_unix_ms"`
}

type receiptProjectionStats struct {
	InboxProjectionCount     int64 `json:"inbox_projection_count"`
	InboxProjectionMinSeq    int64 `json:"inbox_projection_min_seq"`
	InboxProjectionMaxSeq    int64 `json:"inbox_projection_max_seq"`
	DeviceReceivedCursorSeq  int64 `json:"device_received_cursor_seq"`
	UserReceivedCursorSeq    int64 `json:"user_received_cursor_seq"`
	UserReadCursorSeq        int64 `json:"user_read_cursor_seq"`
	MessageReceiptStateCount int64 `json:"message_receipt_state_count"`
	ReceiverReceivedSeq      int64 `json:"receiver_received_seq"`
	ReceiverReadSeq          int64 `json:"receiver_read_seq"`
	ReceiptCheckpointOffset  int64 `json:"receipt_checkpoint_offset"`
	DeliveryCheckpointOffset int64 `json:"delivery_checkpoint_offset"`
}

type receiptOutboxStats struct {
	Total       int64            `json:"total"`
	Pending     int64            `json:"pending"`
	Published   int64            `json:"published"`
	DLQ         int64            `json:"dlq"`
	ByEventType map[string]int64 `json:"by_event_type"`
}

type receiptKafkaEvent struct {
	EventID          string `json:"event_id"`
	EventType        string `json:"event_type"`
	Partition        int    `json:"partition"`
	Offset           int64  `json:"offset"`
	AggregateVersion int64  `json:"aggregate_version"`
	PartitionKey     string `json:"partition_key"`
	PayloadType      string `json:"payload_type"`
	MessageID        string `json:"message_id"`
	UserID           string `json:"user_id"`
	DeviceID         string `json:"device_id"`
	CursorSeq        int64  `json:"cursor_seq"`
}

type outboxStats struct {
	Total     int64 `json:"total"`
	Pending   int64 `json:"pending"`
	Published int64 `json:"published"`
	DLQ       int64 `json:"dlq"`
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
	var cfg config
	flag.StringVar(&cfg.conversationTarget, "conversation-target", "127.0.0.1:10496", "conversation-service gRPC target")
	flag.StringVar(&cfg.messageTarget, "message-target", "127.0.0.1:10495", "message-service gRPC target")
	flag.StringVar(&cfg.deliveryTarget, "delivery-target", "127.0.0.1:10497", "delivery-service gRPC target")
	flag.StringVar(&cfg.receiptTarget, "receipt-target", "127.0.0.1:10499", "receipt-service gRPC target")
	registerTLSFlags("conversation-tls", "NEXUSIM_CONVERSATION_TLS", "conversation-service", &cfg.conversationTLS)
	registerTLSFlags("message-tls", "NEXUSIM_MESSAGE_TLS", "message-service", &cfg.messageTLS)
	registerTLSFlags("delivery-tls", "NEXUSIM_DELIVERY_TLS", "delivery-service", &cfg.deliveryTLS)
	registerTLSFlags("receipt-tls", "NEXUSIM_RECEIPT_TLS", "receipt-service", &cfg.receiptTLS)
	flag.StringVar(&cfg.resultDir, "result-dir", `H:\NexusIM\loadtest-results\receipt-smoke`, "result directory")
	flag.StringVar(&cfg.pgDSN, "pg-dsn", "", "PostgreSQL DSN")
	flag.DurationVar(&cfg.requestTimeout, "request-timeout", 3*time.Second, "per-request timeout")
	flag.DurationVar(&cfg.waitTimeout, "wait-timeout", 20*time.Second, "wait timeout")
	flag.DurationVar(&cfg.pollInterval, "poll-interval", 200*time.Millisecond, "poll interval")
	flag.StringVar(&cfg.tenantID, "tenant-id", "tenant-receipt-smoke", "tenant id")
	flag.StringVar(&cfg.conversationID, "conversation-id", "conv-receipt-smoke", "conversation id")
	flag.StringVar(&cfg.ownerUserID, "owner-user-id", "owner-1", "owner/sender user id")
	flag.StringVar(&cfg.receiverUserID, "receiver-user-id", "receipt-user-1", "receiver user id")
	flag.StringVar(&cfg.receiverDeviceID, "receiver-device-id", "receipt-device-1", "receiver device id")
	flag.StringVar(&cfg.deliveryGroup, "delivery-consumer-group", "", "delivery timeline consumer group")
	flag.StringVar(&cfg.receiptGroup, "receipt-consumer-group", "", "receipt delivery event consumer group")
	var kafkaBrokers string
	flag.StringVar(&kafkaBrokers, "kafka-brokers", "localhost:9092", "Kafka brokers for receipt event readback")
	flag.StringVar(&cfg.receiptEventsTopic, "receipt-events-topic", "im.receipt.events", "receipt events topic")
	flag.StringVar(&cfg.receiptEventsGroup, "receipt-events-consumer-group", "", "receipt event readback consumer group")
	flag.BoolVar(&cfg.verifiedAuthMetadata, "verified-auth-metadata", envBool(false, "NEXUSIM_RECEIPT_LOADTEST_VERIFIED_AUTH_METADATA"), "send gateway verified identity through user-facing gRPC metadata")
	flag.BoolVar(&cfg.cleanup, "cleanup", true, "delete existing rows for tenant before smoke")
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
	cfg.kafkaBrokers = splitCSV(kafkaBrokers)
	return cfg
}

func registerTLSFlags(prefix string, envPrefix string, serviceName string, config *grpctls.Config) {
	flag.StringVar(&config.CAFile, prefix+"-ca-file", os.Getenv(envPrefix+"_CA_FILE"), "CA PEM for "+serviceName+" gRPC TLS")
	flag.StringVar(&config.ServerName, prefix+"-server-name", os.Getenv(envPrefix+"_SERVER_NAME"), "override server name for "+serviceName+" gRPC TLS")
	flag.StringVar(&config.ClientCertFile, prefix+"-client-cert-file", os.Getenv(envPrefix+"_CLIENT_CERT_FILE"), "client certificate PEM for "+serviceName+" gRPC mTLS")
	flag.StringVar(&config.ClientKeyFile, prefix+"-client-key-file", os.Getenv(envPrefix+"_CLIENT_KEY_FILE"), "client private key PEM for "+serviceName+" gRPC mTLS")
}

func ownerAuth(cfg config, traceID string, requestID string) verifiedAuthIdentity {
	return verifiedAuthIdentity{
		tenantID:  cfg.tenantID,
		userID:    cfg.ownerUserID,
		deviceID:  "receipt-smoke-owner-device",
		sessionID: "receipt-smoke-owner-session",
		traceID:   traceID,
		requestID: requestID,
	}
}

func receiverAuth(cfg config, traceID string, requestID string) verifiedAuthIdentity {
	return verifiedAuthIdentity{
		tenantID:  cfg.tenantID,
		userID:    cfg.receiverUserID,
		deviceID:  cfg.receiverDeviceID,
		sessionID: "receipt-smoke",
		traceID:   traceID,
		requestID: requestID,
	}
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

func conversationAuth(auth verifiedAuthIdentity) *conversationv1.AuthContext {
	return &conversationv1.AuthContext{
		TenantId:  auth.tenantID,
		UserId:    auth.userID,
		DeviceId:  auth.deviceID,
		SessionId: auth.sessionID,
		TraceId:   auth.traceID,
		RequestId: auth.requestID,
	}
}

func messageAuth(auth verifiedAuthIdentity) *messagev1.AuthContext {
	return &messagev1.AuthContext{
		TenantId:  auth.tenantID,
		UserId:    auth.userID,
		DeviceId:  auth.deviceID,
		SessionId: auth.sessionID,
		TraceId:   auth.traceID,
		RequestId: auth.requestID,
	}
}

func deliveryAuth(auth verifiedAuthIdentity) *deliveryv1.AuthContext {
	return &deliveryv1.AuthContext{
		TenantId:  auth.tenantID,
		UserId:    auth.userID,
		DeviceId:  auth.deviceID,
		SessionId: auth.sessionID,
		TraceId:   auth.traceID,
		RequestId: auth.requestID,
	}
}

func receiptAuth(auth verifiedAuthIdentity) *receiptv1.AuthContext {
	return &receiptv1.AuthContext{
		TenantId:  auth.tenantID,
		UserId:    auth.userID,
		DeviceId:  auth.deviceID,
		SessionId: auth.sessionID,
		TraceId:   auth.traceID,
		RequestId: auth.requestID,
	}
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
		if err := cleanupTenant(ctx, pool, cfg); err != nil {
			return err
		}
	}
	if err := seedConversation(ctx, pool, cfg); err != nil {
		return err
	}

	conversationDialOption, err := grpctls.DialOption(cfg.conversationTLS, "conversation-tls")
	if err != nil {
		return fmt.Errorf("configure conversation-service TLS: %w", err)
	}
	conversationConn, err := grpc.NewClient(cfg.conversationTarget, conversationDialOption)
	if err != nil {
		return fmt.Errorf("dial conversation-service: %w", err)
	}
	defer conversationConn.Close()
	messageDialOption, err := grpctls.DialOption(cfg.messageTLS, "message-tls")
	if err != nil {
		return fmt.Errorf("configure message-service TLS: %w", err)
	}
	messageConn, err := grpc.NewClient(cfg.messageTarget, messageDialOption)
	if err != nil {
		return fmt.Errorf("dial message-service: %w", err)
	}
	defer messageConn.Close()
	deliveryDialOption, err := grpctls.DialOption(cfg.deliveryTLS, "delivery-tls")
	if err != nil {
		return fmt.Errorf("configure delivery-service TLS: %w", err)
	}
	deliveryConn, err := grpc.NewClient(cfg.deliveryTarget, deliveryDialOption)
	if err != nil {
		return fmt.Errorf("dial delivery-service: %w", err)
	}
	defer deliveryConn.Close()
	receiptDialOption, err := grpctls.DialOption(cfg.receiptTLS, "receipt-tls")
	if err != nil {
		return fmt.Errorf("configure receipt-service TLS: %w", err)
	}
	receiptConn, err := grpc.NewClient(cfg.receiptTarget, receiptDialOption)
	if err != nil {
		return fmt.Errorf("dial receipt-service: %w", err)
	}
	defer receiptConn.Close()

	conversationClient := conversationv1.NewConversationServiceClient(conversationConn)
	messageClient := messagev1.NewMessageServiceClient(messageConn)
	deliveryClient := deliveryv1.NewDeliveryServiceClient(deliveryConn)
	receiptClient := receiptv1.NewReceiptServiceClient(receiptConn)

	result := summary{
		Commit:                 shortCommit(),
		CommitFull:             fullCommit(),
		GitDirty:               gitDirty(),
		GitStatusShort:         gitStatusShort(),
		ConversationTarget:     cfg.conversationTarget,
		MessageTarget:          cfg.messageTarget,
		DeliveryTarget:         cfg.deliveryTarget,
		ReceiptTarget:          cfg.receiptTarget,
		ConversationTLSEnabled: cfg.conversationTLS.Enabled(),
		MessageTLSEnabled:      cfg.messageTLS.Enabled(),
		DeliveryTLSEnabled:     cfg.deliveryTLS.Enabled(),
		ReceiptTLSEnabled:      cfg.receiptTLS.Enabled(),
		VerifiedAuthMetadata:   cfg.verifiedAuthMetadata,
		ResultDir:              cfg.resultDir,
		TenantID:               cfg.tenantID,
		ConversationID:         cfg.conversationID,
		OwnerUserID:            cfg.ownerUserID,
		ReceiverUserID:         cfg.receiverUserID,
		ReceiverDeviceID:       cfg.receiverDeviceID,
		DeliveryConsumerGroup:  cfg.deliveryGroup,
		ReceiptConsumerGroup:   cfg.receiptGroup,
		ReceiptEventsTopic:     cfg.receiptEventsTopic,
		ReceiptEventsGroup:     cfg.receiptEventsGroup,
		StartedAt:              time.Now().UTC(),
		LatenciesMS:            map[string]float64{},
	}

	if err := executeSmoke(ctx, cfg, pool, conversationClient, messageClient, deliveryClient, receiptClient, &result); err != nil {
		result.Error = err.Error()
	}
	result.FinishedAt = time.Now().UTC()
	result.Success = result.Error == ""
	if err := writeSummary(cfg, result); err != nil {
		return err
	}
	if !result.Success {
		return fmt.Errorf("receipt smoke failed: %s", result.Error)
	}
	return nil
}

func executeSmoke(
	ctx context.Context,
	cfg config,
	pool *pgxpool.Pool,
	conversationClient conversationv1.ConversationServiceClient,
	messageClient messagev1.MessageServiceClient,
	deliveryClient deliveryv1.DeliveryServiceClient,
	receiptClient receiptv1.ReceiptServiceClient,
	result *summary,
) error {
	begin := time.Now()
	join, err := createReceiverJoin(ctx, cfg, conversationClient)
	result.LatenciesMS["create_member_join"] = elapsedMS(begin)
	if err != nil {
		return fmt.Errorf("create receiver join: %w", err)
	}
	result.MemberJoin = memberJoinSummary{
		ChangeID:          join.GetChangeId(),
		BoundarySeq:       join.GetBoundarySeq(),
		MemberVersion:     join.GetMemberVersion(),
		PermissionVersion: join.GetPermissionVersion(),
	}
	if err := waitMembership(ctx, pool, cfg); err != nil {
		return err
	}

	begin = time.Now()
	send, err := sendMessage(ctx, cfg, messageClient, 1)
	result.LatenciesMS["send_message"] = elapsedMS(begin)
	if err != nil {
		return fmt.Errorf("send message: %w", err)
	}
	result.SendMessage = sendSummary{
		MessageID:       send.GetMessageId(),
		ConversationSeq: send.GetConversationSeq(),
	}

	pull, err := pullInboxAtLeast(ctx, cfg, deliveryClient, send.GetConversationSeq())
	if err != nil {
		return fmt.Errorf("pull inbox: %w", err)
	}
	result.PullInbox = pull
	if pull.MaxSeq < send.GetConversationSeq() {
		return fmt.Errorf("pull inbox max seq %d did not reach sent seq %d", pull.MaxSeq, send.GetConversationSeq())
	}

	begin = time.Now()
	ackResponse, err := ackDelivery(ctx, cfg, deliveryClient, send.GetConversationSeq())
	result.AckDelivery.LatencyMS = elapsedMS(begin)
	if err != nil {
		return fmt.Errorf("ack delivery: %w", err)
	}
	result.AckDelivery.LastReceivedSeq = ackResponse.GetLastReceivedSeq()
	if result.AckDelivery.LastReceivedSeq < send.GetConversationSeq() {
		return fmt.Errorf("ack last_received_seq %d did not reach sent seq %d", result.AckDelivery.LastReceivedSeq, send.GetConversationSeq())
	}

	if err := waitReceiptReceived(ctx, pool, cfg, send.GetConversationSeq()); err != nil {
		return err
	}
	before, err := getReceiptBySeq(ctx, cfg, receiptClient, send.GetConversationSeq())
	if err != nil {
		return fmt.Errorf("get receipt before read by seq: %w", err)
	}
	result.ReceiptBeforeReadBySeq = before
	receiverBefore := findReceiver(before, cfg.receiverUserID)
	if receiverBefore.ReceivedSeq != send.GetConversationSeq() || receiverBefore.ReadSeq != 0 {
		return fmt.Errorf("unexpected receipt before read receiver=%+v", receiverBefore)
	}
	begin = time.Now()
	conversationListBefore, err := listConversations(ctx, cfg, receiptClient, false, false)
	conversationListBefore.LatencyMS = elapsedMS(begin)
	if err != nil {
		return fmt.Errorf("list conversations before read: %w", err)
	}
	result.ConversationListBefore = conversationListBefore
	if err := assertConversationListState(conversationListBefore, cfg.conversationID, send.GetConversationSeq(), 1, 0); err != nil {
		return fmt.Errorf("conversation list before read: %w", err)
	}

	begin = time.Now()
	conversationListUnreadBefore, err := listConversations(ctx, cfg, receiptClient, false, true)
	conversationListUnreadBefore.LatencyMS = elapsedMS(begin)
	if err != nil {
		return fmt.Errorf("list unread conversations before read: %w", err)
	}
	result.ConversationListUnreadBeforeRead = conversationListUnreadBefore
	if err := assertConversationListState(conversationListUnreadBefore, cfg.conversationID, send.GetConversationSeq(), 1, 0); err != nil {
		return fmt.Errorf("unread conversation list before read: %w", err)
	}

	begin = time.Now()
	markResponse, err := markRead(ctx, cfg, receiptClient, send.GetConversationSeq())
	result.MarkRead.LatencyMS = elapsedMS(begin)
	if err != nil {
		return fmt.Errorf("mark read: %w", err)
	}
	result.MarkRead.LastReadSeq = markResponse.GetLastReadSeq()
	if result.MarkRead.LastReadSeq != send.GetConversationSeq() {
		return fmt.Errorf("mark read last_read_seq %d did not match sent seq %d", result.MarkRead.LastReadSeq, send.GetConversationSeq())
	}

	afterSeq, err := getReceiptBySeq(ctx, cfg, receiptClient, send.GetConversationSeq())
	if err != nil {
		return fmt.Errorf("get receipt after read by seq: %w", err)
	}
	result.ReceiptAfterReadBySeq = afterSeq
	afterMessageID, err := getReceiptByMessageID(ctx, cfg, receiptClient, send.GetMessageId())
	if err != nil {
		return fmt.Errorf("get receipt after read by message_id: %w", err)
	}
	result.ReceiptAfterReadByMsgID = afterMessageID
	receiverAfter := findReceiver(afterSeq, cfg.receiverUserID)
	if receiverAfter.ReadSeq != send.GetConversationSeq() {
		return fmt.Errorf("unexpected receipt after read receiver=%+v", receiverAfter)
	}
	begin = time.Now()
	conversationListAfter, err := listConversations(ctx, cfg, receiptClient, false, false)
	conversationListAfter.LatencyMS = elapsedMS(begin)
	if err != nil {
		return fmt.Errorf("list conversations after read: %w", err)
	}
	result.ConversationListAfter = conversationListAfter
	if err := assertConversationListState(conversationListAfter, cfg.conversationID, send.GetConversationSeq(), 0, send.GetConversationSeq()); err != nil {
		return fmt.Errorf("conversation list after read: %w", err)
	}

	begin = time.Now()
	conversationListUnreadAfter, err := listConversations(ctx, cfg, receiptClient, false, true)
	conversationListUnreadAfter.LatencyMS = elapsedMS(begin)
	if err != nil {
		return fmt.Errorf("list unread conversations after read: %w", err)
	}
	result.ConversationListUnreadAfterRead = conversationListUnreadAfter
	if err := assertConversationListHidden(conversationListUnreadAfter); err != nil {
		return fmt.Errorf("unread conversation list after read: %w", err)
	}

	begin = time.Now()
	archiveResponse, err := archiveConversation(ctx, cfg, receiptClient, true)
	result.ArchiveConversation.LatencyMS = elapsedMS(begin)
	if err != nil {
		return fmt.Errorf("archive conversation: %w", err)
	}
	result.ArchiveConversation.Archived = archiveResponse.GetConversation().GetArchived()
	if !result.ArchiveConversation.Archived {
		return fmt.Errorf("archive response did not mark conversation archived: %+v", archiveResponse.GetConversation())
	}

	begin = time.Now()
	archivedDefault, err := listConversations(ctx, cfg, receiptClient, false, false)
	archivedDefault.LatencyMS = elapsedMS(begin)
	if err != nil {
		return fmt.Errorf("list conversations after archive default: %w", err)
	}
	result.ConversationListArchivedDefault = archivedDefault
	if err := assertConversationListHidden(archivedDefault); err != nil {
		return fmt.Errorf("conversation list archive default: %w", err)
	}

	begin = time.Now()
	archivedIncluded, err := listConversations(ctx, cfg, receiptClient, true, false)
	archivedIncluded.LatencyMS = elapsedMS(begin)
	if err != nil {
		return fmt.Errorf("list conversations after archive include_archived: %w", err)
	}
	result.ConversationListArchivedIncluded = archivedIncluded
	if err := assertConversationListArchived(archivedIncluded, cfg.conversationID, send.GetConversationSeq(), true); err != nil {
		return fmt.Errorf("conversation list archive included: %w", err)
	}

	begin = time.Now()
	sendWhileArchived, err := sendMessage(ctx, cfg, messageClient, 2)
	result.LatenciesMS["send_message_while_archived"] = elapsedMS(begin)
	if err != nil {
		return fmt.Errorf("send message while archived: %w", err)
	}
	result.SendMessageWhileArchived = sendSummary{
		MessageID:       sendWhileArchived.GetMessageId(),
		ConversationSeq: sendWhileArchived.GetConversationSeq(),
	}

	pullWhileArchived, err := pullInboxAtLeast(ctx, cfg, deliveryClient, sendWhileArchived.GetConversationSeq())
	if err != nil {
		return fmt.Errorf("pull inbox while archived: %w", err)
	}
	result.PullInboxWhileArchived = pullWhileArchived
	if pullWhileArchived.MaxSeq < sendWhileArchived.GetConversationSeq() {
		return fmt.Errorf("pull inbox while archived max seq %d did not reach sent seq %d", pullWhileArchived.MaxSeq, sendWhileArchived.GetConversationSeq())
	}

	begin = time.Now()
	ackWhileArchived, err := ackDelivery(ctx, cfg, deliveryClient, sendWhileArchived.GetConversationSeq())
	result.AckDeliveryWhileArchived.LatencyMS = elapsedMS(begin)
	if err != nil {
		return fmt.Errorf("ack delivery while archived: %w", err)
	}
	result.AckDeliveryWhileArchived.LastReceivedSeq = ackWhileArchived.GetLastReceivedSeq()
	if result.AckDeliveryWhileArchived.LastReceivedSeq < sendWhileArchived.GetConversationSeq() {
		return fmt.Errorf("ack while archived last_received_seq %d did not reach sent seq %d", result.AckDeliveryWhileArchived.LastReceivedSeq, sendWhileArchived.GetConversationSeq())
	}
	if err := waitReceiptReceived(ctx, pool, cfg, sendWhileArchived.GetConversationSeq()); err != nil {
		return err
	}

	begin = time.Now()
	archivedNewDefault, err := listConversations(ctx, cfg, receiptClient, false, false)
	archivedNewDefault.LatencyMS = elapsedMS(begin)
	if err != nil {
		return fmt.Errorf("list conversations after archived new message default: %w", err)
	}
	result.ConversationListAfterArchivedNewDefault = archivedNewDefault
	if err := assertConversationListHidden(archivedNewDefault); err != nil {
		return fmt.Errorf("conversation list after archived new message default: %w", err)
	}

	begin = time.Now()
	archivedNewIncluded, err := listConversations(ctx, cfg, receiptClient, true, false)
	archivedNewIncluded.LatencyMS = elapsedMS(begin)
	if err != nil {
		return fmt.Errorf("list conversations after archived new message include_archived: %w", err)
	}
	result.ConversationListAfterArchivedNewIncluded = archivedNewIncluded
	if err := assertConversationListArchived(archivedNewIncluded, cfg.conversationID, sendWhileArchived.GetConversationSeq(), true); err != nil {
		return fmt.Errorf("conversation list after archived new message included: %w", err)
	}

	begin = time.Now()
	unarchiveResponse, err := archiveConversation(ctx, cfg, receiptClient, false)
	result.UnarchiveConversation.LatencyMS = elapsedMS(begin)
	if err != nil {
		return fmt.Errorf("unarchive conversation: %w", err)
	}
	result.UnarchiveConversation.Archived = unarchiveResponse.GetConversation().GetArchived()
	if result.UnarchiveConversation.Archived {
		return fmt.Errorf("unarchive response still marked conversation archived: %+v", unarchiveResponse.GetConversation())
	}

	begin = time.Now()
	afterUnarchive, err := listConversations(ctx, cfg, receiptClient, false, false)
	afterUnarchive.LatencyMS = elapsedMS(begin)
	if err != nil {
		return fmt.Errorf("list conversations after unarchive: %w", err)
	}
	result.ConversationListAfterUnarchive = afterUnarchive
	if err := assertConversationListArchived(afterUnarchive, cfg.conversationID, sendWhileArchived.GetConversationSeq(), false); err != nil {
		return fmt.Errorf("conversation list after unarchive: %w", err)
	}

	begin = time.Now()
	pinResponse, err := pinConversation(ctx, cfg, receiptClient, true)
	result.PinConversation.LatencyMS = elapsedMS(begin)
	if err != nil {
		return fmt.Errorf("pin conversation: %w", err)
	}
	result.PinConversation.Pinned = pinResponse.GetConversation().GetPinned()
	if !result.PinConversation.Pinned {
		return fmt.Errorf("pin response did not mark conversation pinned: %+v", pinResponse.GetConversation())
	}

	begin = time.Now()
	afterPin, err := listConversations(ctx, cfg, receiptClient, false, false)
	afterPin.LatencyMS = elapsedMS(begin)
	if err != nil {
		return fmt.Errorf("list conversations after pin: %w", err)
	}
	result.ConversationListAfterPin = afterPin
	if err := assertConversationListPinned(afterPin, cfg.conversationID, sendWhileArchived.GetConversationSeq(), true); err != nil {
		return fmt.Errorf("conversation list after pin: %w", err)
	}

	begin = time.Now()
	unpinResponse, err := pinConversation(ctx, cfg, receiptClient, false)
	result.UnpinConversation.LatencyMS = elapsedMS(begin)
	if err != nil {
		return fmt.Errorf("unpin conversation: %w", err)
	}
	result.UnpinConversation.Pinned = unpinResponse.GetConversation().GetPinned()
	if result.UnpinConversation.Pinned {
		return fmt.Errorf("unpin response still marked conversation pinned: %+v", unpinResponse.GetConversation())
	}

	begin = time.Now()
	afterUnpin, err := listConversations(ctx, cfg, receiptClient, false, false)
	afterUnpin.LatencyMS = elapsedMS(begin)
	if err != nil {
		return fmt.Errorf("list conversations after unpin: %w", err)
	}
	result.ConversationListAfterUnpin = afterUnpin
	if err := assertConversationListPinned(afterUnpin, cfg.conversationID, sendWhileArchived.GetConversationSeq(), false); err != nil {
		return fmt.Errorf("conversation list after unpin: %w", err)
	}

	begin = time.Now()
	muteResponse, err := muteConversation(ctx, cfg, receiptClient, true)
	result.MuteConversation.LatencyMS = elapsedMS(begin)
	if err != nil {
		return fmt.Errorf("mute conversation: %w", err)
	}
	result.MuteConversation.Muted = muteResponse.GetConversation().GetMuted()
	if !result.MuteConversation.Muted {
		return fmt.Errorf("mute response did not mark conversation muted: %+v", muteResponse.GetConversation())
	}

	begin = time.Now()
	afterMute, err := listConversations(ctx, cfg, receiptClient, false, false)
	afterMute.LatencyMS = elapsedMS(begin)
	if err != nil {
		return fmt.Errorf("list conversations after mute: %w", err)
	}
	result.ConversationListAfterMute = afterMute
	if err := assertConversationListMuted(afterMute, cfg.conversationID, sendWhileArchived.GetConversationSeq(), 1, send.GetConversationSeq(), true); err != nil {
		return fmt.Errorf("conversation list after mute: %w", err)
	}

	begin = time.Now()
	unmuteResponse, err := muteConversation(ctx, cfg, receiptClient, false)
	result.UnmuteConversation.LatencyMS = elapsedMS(begin)
	if err != nil {
		return fmt.Errorf("unmute conversation: %w", err)
	}
	result.UnmuteConversation.Muted = unmuteResponse.GetConversation().GetMuted()
	if result.UnmuteConversation.Muted {
		return fmt.Errorf("unmute response still marked conversation muted: %+v", unmuteResponse.GetConversation())
	}

	begin = time.Now()
	afterUnmute, err := listConversations(ctx, cfg, receiptClient, false, false)
	afterUnmute.LatencyMS = elapsedMS(begin)
	if err != nil {
		return fmt.Errorf("list conversations after unmute: %w", err)
	}
	result.ConversationListAfterUnmute = afterUnmute
	if err := assertConversationListMuted(afterUnmute, cfg.conversationID, sendWhileArchived.GetConversationSeq(), 1, send.GetConversationSeq(), false); err != nil {
		return fmt.Errorf("conversation list after unmute: %w", err)
	}

	tooFar, err := markRead(ctx, cfg, receiptClient, sendWhileArchived.GetConversationSeq()+1)
	if err == nil {
		return fmt.Errorf("mark read too far unexpectedly succeeded: %+v", tooFar)
	}
	statusErr, ok := status.FromError(err)
	if !ok {
		return fmt.Errorf("mark read too far returned non-gRPC error: %w", err)
	}
	result.MarkReadTooFar = negativeCallSummary{
		Code:    statusErr.Code().String(),
		Message: statusErr.Message(),
		Passed:  statusErr.Code() == codes.FailedPrecondition,
	}
	if !result.MarkReadTooFar.Passed {
		return fmt.Errorf("mark read too far code=%s message=%s", statusErr.Code(), statusErr.Message())
	}

	if err := waitReceiptOutboxPublished(ctx, pool, cfg, 3); err != nil {
		return err
	}
	if err := fillPostgresStats(ctx, pool, cfg, result); err != nil {
		return err
	}
	events, err := readReceiptEvents(ctx, cfg, result.ReceiptOutbox.ByEventType)
	if err != nil {
		return err
	}
	result.ReceiptKafkaEvents = events
	return nil
}

func createReceiverJoin(
	ctx context.Context,
	cfg config,
	client conversationv1.ConversationServiceClient,
) (*conversationv1.CreateMemberChangeResponse, error) {
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	auth := ownerAuth(cfg, "receipt-smoke-join", "receipt-smoke-join")
	requestCtx = withVerifiedAuthMetadata(requestCtx, cfg, auth)
	return client.CreateMemberChange(requestCtx, &conversationv1.CreateMemberChangeRequest{
		AuthContext:           conversationAuth(auth),
		ConversationId:        cfg.conversationID,
		TargetUserId:          cfg.receiverUserID,
		ChangeType:            conversationv1.MemberChangeType_MEMBER_CHANGE_TYPE_JOIN,
		TargetRole:            conversationv1.MemberRole_MEMBER_ROLE_MEMBER,
		ExpectedMemberVersion: 1,
		IdempotencyKey:        "receipt-smoke-join-receiver",
		ConflictPolicy:        conversationv1.MemberChangeConflictPolicy_MEMBER_CHANGE_CONFLICT_POLICY_REJECT,
		Reason:                "receipt smoke receiver join",
	})
}

func sendMessage(ctx context.Context, cfg config, client messagev1.MessageServiceClient, index int) (*messagev1.SendMessageResponse, error) {
	payload, err := structpb.NewStruct(map[string]any{"text": fmt.Sprintf("receipt smoke %d", index)})
	if err != nil {
		return nil, err
	}
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	auth := ownerAuth(cfg, "receipt-smoke-send", fmt.Sprintf("receipt-smoke-send-%d", index))
	requestCtx = withVerifiedAuthMetadata(requestCtx, cfg, auth)
	return client.SendMessage(requestCtx, &messagev1.SendMessageRequest{
		AuthContext:    messageAuth(auth),
		ConversationId: cfg.conversationID,
		ClientMsgId:    fmt.Sprintf("receipt-smoke-client-message-%d", index),
		MessageType:    "TEXT",
		Payload:        payload,
	})
}

func pullInboxAtLeast(
	ctx context.Context,
	cfg config,
	client deliveryv1.DeliveryServiceClient,
	minSeq int64,
) (pullSummary, error) {
	deadline := time.Now().Add(cfg.waitTimeout)
	latencies := make([]float64, 0, 8)
	for {
		requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
		auth := receiverAuth(cfg, "receipt-smoke-pull", "receipt-smoke-pull")
		requestCtx = withVerifiedAuthMetadata(requestCtx, cfg, auth)
		begin := time.Now()
		response, err := client.PullInbox(requestCtx, &deliveryv1.PullInboxRequest{
			AuthContext:    deliveryAuth(auth),
			ConversationId: cfg.conversationID,
			AfterSeq:       0,
			Limit:          100,
		})
		latencies = append(latencies, elapsedMS(begin))
		cancel()
		if err != nil {
			return pullSummary{}, err
		}
		result := pullSummary{ItemCount: len(response.GetItems())}
		for _, inboxItem := range response.GetItems() {
			if inboxItem.GetConversationSeq() > result.MaxSeq {
				result.MaxSeq = inboxItem.GetConversationSeq()
			}
			result.Items = append(result.Items, pulledItem{
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
		if result.MaxSeq >= minSeq || time.Now().After(deadline) {
			if result.MaxSeq < minSeq {
				return result, fmt.Errorf("pull inbox timeout: max_seq=%d want>=%d", result.MaxSeq, minSeq)
			}
			return result, nil
		}
		time.Sleep(cfg.pollInterval)
	}
}

func ackDelivery(
	ctx context.Context,
	cfg config,
	client deliveryv1.DeliveryServiceClient,
	seq int64,
) (*deliveryv1.AckDeliveryResponse, error) {
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	auth := receiverAuth(cfg, "receipt-smoke-ack", "receipt-smoke-ack")
	requestCtx = withVerifiedAuthMetadata(requestCtx, cfg, auth)
	return client.AckDelivery(requestCtx, &deliveryv1.AckDeliveryRequest{
		AuthContext:    deliveryAuth(auth),
		ConversationId: cfg.conversationID,
		ReceivedSeq:    seq,
	})
}

func getReceiptBySeq(
	ctx context.Context,
	cfg config,
	client receiptv1.ReceiptServiceClient,
	seq int64,
) (receiptStateSummary, error) {
	response, err := getReceipt(ctx, cfg, client, "", seq)
	if err != nil {
		return receiptStateSummary{}, err
	}
	return summarizeReceipt("conversation_seq", response), nil
}

func getReceiptByMessageID(
	ctx context.Context,
	cfg config,
	client receiptv1.ReceiptServiceClient,
	messageID string,
) (receiptStateSummary, error) {
	response, err := getReceipt(ctx, cfg, client, messageID, 0)
	if err != nil {
		return receiptStateSummary{}, err
	}
	return summarizeReceipt("message_id", response), nil
}

func getReceipt(
	ctx context.Context,
	cfg config,
	client receiptv1.ReceiptServiceClient,
	messageID string,
	seq int64,
) (*receiptv1.GetReceiptStateResponse, error) {
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	auth := ownerAuth(cfg, "receipt-smoke-get", "receipt-smoke-get")
	requestCtx = withVerifiedAuthMetadata(requestCtx, cfg, auth)
	return client.GetReceiptState(requestCtx, &receiptv1.GetReceiptStateRequest{
		AuthContext:     receiptAuth(auth),
		ConversationId:  cfg.conversationID,
		MessageId:       messageID,
		ConversationSeq: seq,
	})
}

func listConversations(
	ctx context.Context,
	cfg config,
	client receiptv1.ReceiptServiceClient,
	includeArchived bool,
	unreadOnly bool,
) (conversationListSummary, error) {
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	auth := receiverAuth(cfg, "receipt-smoke-list", "receipt-smoke-list")
	requestCtx = withVerifiedAuthMetadata(requestCtx, cfg, auth)
	response, err := client.ListConversations(requestCtx, &receiptv1.ListConversationsRequest{
		AuthContext:     receiptAuth(auth),
		Limit:           10,
		IncludeArchived: includeArchived,
		UnreadOnly:      unreadOnly,
	})
	if err != nil {
		return conversationListSummary{}, err
	}
	return summarizeConversationList(response), nil
}

func archiveConversation(
	ctx context.Context,
	cfg config,
	client receiptv1.ReceiptServiceClient,
	archived bool,
) (*receiptv1.ArchiveConversationResponse, error) {
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	auth := receiverAuth(cfg, "receipt-smoke-archive", fmt.Sprintf("receipt-smoke-archive-%v", archived))
	requestCtx = withVerifiedAuthMetadata(requestCtx, cfg, auth)
	return client.ArchiveConversation(requestCtx, &receiptv1.ArchiveConversationRequest{
		AuthContext:    receiptAuth(auth),
		ConversationId: cfg.conversationID,
		Archived:       archived,
	})
}

func pinConversation(
	ctx context.Context,
	cfg config,
	client receiptv1.ReceiptServiceClient,
	pinned bool,
) (*receiptv1.PinConversationResponse, error) {
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	auth := receiverAuth(cfg, "receipt-smoke-pin", fmt.Sprintf("receipt-smoke-pin-%v", pinned))
	requestCtx = withVerifiedAuthMetadata(requestCtx, cfg, auth)
	return client.PinConversation(requestCtx, &receiptv1.PinConversationRequest{
		AuthContext:    receiptAuth(auth),
		ConversationId: cfg.conversationID,
		Pinned:         pinned,
	})
}

func muteConversation(
	ctx context.Context,
	cfg config,
	client receiptv1.ReceiptServiceClient,
	muted bool,
) (*receiptv1.MuteConversationResponse, error) {
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	auth := receiverAuth(cfg, "receipt-smoke-mute", fmt.Sprintf("receipt-smoke-mute-%v", muted))
	requestCtx = withVerifiedAuthMetadata(requestCtx, cfg, auth)
	return client.MuteConversation(requestCtx, &receiptv1.MuteConversationRequest{
		AuthContext:    receiptAuth(auth),
		ConversationId: cfg.conversationID,
		Muted:          muted,
	})
}

func markRead(
	ctx context.Context,
	cfg config,
	client receiptv1.ReceiptServiceClient,
	seq int64,
) (*receiptv1.MarkReadResponse, error) {
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	auth := receiverAuth(cfg, "receipt-smoke-mark-read", fmt.Sprintf("receipt-smoke-mark-read-%d", seq))
	requestCtx = withVerifiedAuthMetadata(requestCtx, cfg, auth)
	return client.MarkRead(requestCtx, &receiptv1.MarkReadRequest{
		AuthContext:    receiptAuth(auth),
		ConversationId: cfg.conversationID,
		ReadSeq:        seq,
	})
}

func summarizeConversationList(response *receiptv1.ListConversationsResponse) conversationListSummary {
	result := conversationListSummary{
		ItemCount:      len(response.GetItems()),
		Items:          []conversationSummaryItem{},
		NextPageCursor: response.GetNextPageCursor(),
	}
	if watermark := response.GetProjectionWatermark(); watermark != nil {
		result.ProjectionWatermark = projectionWatermarkSummary{
			Source:          watermark.GetSource(),
			OffsetValue:     watermark.GetOffsetValue(),
			UpdatedAtUnixMS: watermark.GetUpdatedAtUnixMs(),
		}
	}
	for _, item := range response.GetItems() {
		result.Items = append(result.Items, conversationSummaryItem{
			ConversationID:  item.GetConversationId(),
			LastVisibleSeq:  item.GetLastVisibleSeq(),
			LastMessageID:   item.GetLastMessageId(),
			LastSenderID:    item.GetLastSenderId(),
			UnreadCount:     item.GetUnreadCount(),
			LastReadSeq:     item.GetLastReadSeq(),
			UpdatedAtUnixMS: item.GetUpdatedAtUnixMs(),
			Archived:        item.GetArchived(),
			Pinned:          item.GetPinned(),
			Muted:           item.GetMuted(),
		})
	}
	return result
}

func assertConversationListState(
	state conversationListSummary,
	conversationID string,
	seq int64,
	unread int64,
	readSeq int64,
) error {
	if len(state.Items) != 1 {
		return fmt.Errorf("expected 1 item, got %d", len(state.Items))
	}
	item := state.Items[0]
	if item.ConversationID != conversationID {
		return fmt.Errorf("conversation_id=%s want=%s", item.ConversationID, conversationID)
	}
	if item.LastVisibleSeq != seq || item.UnreadCount != unread || item.LastReadSeq != readSeq {
		return fmt.Errorf("unexpected item state: %+v", item)
	}
	if item.Archived {
		return fmt.Errorf("expected visible item not archived: %+v", item)
	}
	if item.LastMessageID == "" || item.LastSenderID == "" {
		return fmt.Errorf("missing last message fields: %+v", item)
	}
	return nil
}

func assertConversationListHidden(state conversationListSummary) error {
	if len(state.Items) != 0 {
		return fmt.Errorf("expected archived conversation hidden, got %+v", state.Items)
	}
	return nil
}

func assertConversationListArchived(
	state conversationListSummary,
	conversationID string,
	seq int64,
	archived bool,
) error {
	if len(state.Items) != 1 {
		return fmt.Errorf("expected 1 item, got %d", len(state.Items))
	}
	item := state.Items[0]
	if item.ConversationID != conversationID {
		return fmt.Errorf("conversation_id=%s want=%s", item.ConversationID, conversationID)
	}
	if item.LastVisibleSeq != seq || item.Archived != archived {
		return fmt.Errorf("unexpected archived item state: %+v", item)
	}
	return nil
}

func assertConversationListPinned(
	state conversationListSummary,
	conversationID string,
	seq int64,
	pinned bool,
) error {
	if len(state.Items) != 1 {
		return fmt.Errorf("expected 1 item, got %d", len(state.Items))
	}
	item := state.Items[0]
	if item.ConversationID != conversationID {
		return fmt.Errorf("conversation_id=%s want=%s", item.ConversationID, conversationID)
	}
	if item.LastVisibleSeq != seq || item.Pinned != pinned || item.Archived {
		return fmt.Errorf("unexpected pinned item state: %+v", item)
	}
	return nil
}

func assertConversationListMuted(
	state conversationListSummary,
	conversationID string,
	seq int64,
	unread int64,
	readSeq int64,
	muted bool,
) error {
	if len(state.Items) != 1 {
		return fmt.Errorf("expected 1 item, got %d", len(state.Items))
	}
	item := state.Items[0]
	if item.ConversationID != conversationID {
		return fmt.Errorf("conversation_id=%s want=%s", item.ConversationID, conversationID)
	}
	if item.LastVisibleSeq != seq || item.UnreadCount != unread || item.LastReadSeq != readSeq || item.Muted != muted || item.Archived || item.Pinned {
		return fmt.Errorf("unexpected muted item state: %+v", item)
	}
	return nil
}

func summarizeReceipt(requestBy string, response *receiptv1.GetReceiptStateResponse) receiptStateSummary {
	result := receiptStateSummary{
		RequestBy:         requestBy,
		ConversationSeq:   response.GetConversationSeq(),
		MessageID:         response.GetMessageId(),
		ReceivedUserCount: response.GetReceivedUserCount(),
		ReadUserCount:     response.GetReadUserCount(),
		VisibilityMode:    response.GetVisibilityMode().String(),
	}
	for _, receiver := range response.GetReceivers() {
		result.Receivers = append(result.Receivers, receiptUserState{
			UserID:           receiver.GetUserId(),
			ReceivedSeq:      receiver.GetReceivedSeq(),
			ReceivedAtUnixMS: receiver.GetReceivedAtUnixMs(),
			ReadSeq:          receiver.GetReadSeq(),
			ReadAtUnixMS:     receiver.GetReadAtUnixMs(),
		})
	}
	return result
}

func findReceiver(state receiptStateSummary, userID string) receiptUserState {
	for _, receiver := range state.Receivers {
		if receiver.UserID == userID {
			return receiver
		}
	}
	return receiptUserState{}
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

func waitReceiptReceived(ctx context.Context, pool *pgxpool.Pool, cfg config, seq int64) error {
	deadline := time.Now().Add(cfg.waitTimeout)
	for time.Now().Before(deadline) {
		var receivedSeq int64
		err := pool.QueryRow(ctx, `
SELECT COALESCE(MAX(CASE WHEN received_at IS NULL THEN 0 ELSE conversation_seq END), 0)
FROM message_receipt_states
WHERE tenant_id = $1
  AND conversation_id = $2
  AND conversation_seq = $3
  AND user_id = $4
`, cfg.tenantID, cfg.conversationID, seq, cfg.receiverUserID).Scan(&receivedSeq)
		if err == nil && receivedSeq >= seq {
			return nil
		}
		time.Sleep(cfg.pollInterval)
	}
	return errors.New("receipt received projection timeout")
}

func waitReceiptOutboxPublished(ctx context.Context, pool *pgxpool.Pool, cfg config, wantPublished int64) error {
	deadline := time.Now().Add(cfg.waitTimeout)
	for time.Now().Before(deadline) {
		var total int64
		var published int64
		var dlq int64
		err := pool.QueryRow(ctx, `
SELECT
    COUNT(*),
    COUNT(*) FILTER (WHERE status = 'PUBLISHED'),
    COUNT(*) FILTER (WHERE status = 'DLQ')
FROM receipt_outbox
WHERE tenant_id = $1
  AND conversation_id = $2
`, cfg.tenantID, cfg.conversationID).Scan(&total, &published, &dlq)
		if err != nil {
			return fmt.Errorf("query receipt outbox publish state: %w", err)
		}
		if dlq > 0 {
			return fmt.Errorf("receipt outbox reached DLQ: dlq=%d", dlq)
		}
		if total >= wantPublished && published >= wantPublished {
			return nil
		}
		time.Sleep(cfg.pollInterval)
	}
	return fmt.Errorf("receipt outbox publish timeout: want_published=%d", wantPublished)
}

func cleanupTenant(ctx context.Context, pool *pgxpool.Pool, cfg config) error {
	statements := []string{
		`DELETE FROM conversation_summary_checkpoints WHERE consumer_group = $1`,
		`DELETE FROM user_conversation_summaries WHERE tenant_id = $1`,
		`DELETE FROM receipt_outbox WHERE tenant_id = $1`,
		`DELETE FROM receipt_kafka_checkpoints WHERE consumer_group = $1`,
		`DELETE FROM message_receipt_states WHERE tenant_id = $1`,
		`DELETE FROM user_read_cursors WHERE tenant_id = $1`,
		`DELETE FROM user_received_cursors WHERE tenant_id = $1`,
		`DELETE FROM device_received_cursors WHERE tenant_id = $1`,
		`DELETE FROM receipt_inbox_projection WHERE tenant_id = $1`,
		`DELETE FROM delivery_outbox WHERE tenant_id = $1`,
		`DELETE FROM device_delivery_cursors WHERE tenant_id = $1`,
		`DELETE FROM user_inbox WHERE tenant_id = $1`,
		`DELETE FROM delivery_membership_projection WHERE tenant_id = $1`,
		`DELETE FROM delivery_kafka_checkpoints WHERE consumer_group = $1`,
		`DELETE FROM message_outbox WHERE tenant_id = $1`,
		`DELETE FROM conversation_timeline_events WHERE tenant_id = $1`,
		`DELETE FROM message_log WHERE tenant_id = $1`,
		`DELETE FROM conversation_seq WHERE tenant_id = $1`,
		`DELETE FROM member_change_saga WHERE tenant_id = $1`,
		`DELETE FROM conversation_members WHERE tenant_id = $1`,
		`DELETE FROM conversations WHERE tenant_id = $1`,
	}
	for _, statement := range statements {
		arg := any(cfg.tenantID)
		if strings.Contains(statement, "receipt_kafka_checkpoints") || strings.Contains(statement, "conversation_summary_checkpoints") {
			arg = cfg.receiptGroup
		}
		if strings.Contains(statement, "delivery_kafka_checkpoints") {
			arg = cfg.deliveryGroup
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
	assign := func(target *int64, query string, args ...any) error {
		var value int64
		if err := pool.QueryRow(ctx, query, args...).Scan(&value); err != nil {
			return err
		}
		*target = value
		return nil
	}
	if err := assign(&result.ReceiptProjection.InboxProjectionCount, `
SELECT COUNT(*) FROM receipt_inbox_projection
WHERE tenant_id = $1 AND conversation_id = $2 AND user_id = $3
`, cfg.tenantID, cfg.conversationID, cfg.receiverUserID); err != nil {
		return fmt.Errorf("query receipt projection count: %w", err)
	}
	if err := pool.QueryRow(ctx, `
SELECT COALESCE(MIN(conversation_seq), 0), COALESCE(MAX(conversation_seq), 0)
FROM receipt_inbox_projection
WHERE tenant_id = $1 AND conversation_id = $2 AND user_id = $3
`, cfg.tenantID, cfg.conversationID, cfg.receiverUserID).Scan(
		&result.ReceiptProjection.InboxProjectionMinSeq,
		&result.ReceiptProjection.InboxProjectionMaxSeq,
	); err != nil {
		return fmt.Errorf("query receipt projection min/max: %w", err)
	}
	if err := assign(&result.ReceiptProjection.DeviceReceivedCursorSeq, `
SELECT COALESCE(MAX(last_received_seq), 0)
FROM device_received_cursors
WHERE tenant_id = $1 AND conversation_id = $2 AND user_id = $3 AND device_id = $4
`, cfg.tenantID, cfg.conversationID, cfg.receiverUserID, cfg.receiverDeviceID); err != nil {
		return fmt.Errorf("query device received cursor: %w", err)
	}
	if err := assign(&result.ReceiptProjection.UserReceivedCursorSeq, `
SELECT COALESCE(MAX(last_received_seq), 0)
FROM user_received_cursors
WHERE tenant_id = $1 AND conversation_id = $2 AND user_id = $3
`, cfg.tenantID, cfg.conversationID, cfg.receiverUserID); err != nil {
		return fmt.Errorf("query user received cursor: %w", err)
	}
	if err := assign(&result.ReceiptProjection.UserReadCursorSeq, `
SELECT COALESCE(MAX(last_read_seq), 0)
FROM user_read_cursors
WHERE tenant_id = $1 AND conversation_id = $2 AND user_id = $3
`, cfg.tenantID, cfg.conversationID, cfg.receiverUserID); err != nil {
		return fmt.Errorf("query user read cursor: %w", err)
	}
	if err := assign(&result.ReceiptProjection.MessageReceiptStateCount, `
SELECT COUNT(*) FROM message_receipt_states
WHERE tenant_id = $1 AND conversation_id = $2
`, cfg.tenantID, cfg.conversationID); err != nil {
		return fmt.Errorf("query message receipt state count: %w", err)
	}
	if err := pool.QueryRow(ctx, `
SELECT
    COALESCE(MAX(CASE WHEN received_at IS NULL THEN 0 ELSE conversation_seq END), 0),
    COALESCE(MAX(CASE WHEN read_at IS NULL THEN 0 ELSE conversation_seq END), 0)
FROM message_receipt_states
WHERE tenant_id = $1 AND conversation_id = $2 AND user_id = $3
`, cfg.tenantID, cfg.conversationID, cfg.receiverUserID).Scan(
		&result.ReceiptProjection.ReceiverReceivedSeq,
		&result.ReceiptProjection.ReceiverReadSeq,
	); err != nil {
		return fmt.Errorf("query receiver receipt state: %w", err)
	}
	if cfg.receiptGroup != "" {
		if err := assign(&result.ReceiptProjection.ReceiptCheckpointOffset, `
SELECT COALESCE(MAX(offset_value), 0)
FROM receipt_kafka_checkpoints
WHERE consumer_group = $1 AND topic = 'im.delivery.events'
`, cfg.receiptGroup); err != nil {
			return fmt.Errorf("query receipt checkpoint: %w", err)
		}
	}
	if cfg.deliveryGroup != "" {
		if err := assign(&result.ReceiptProjection.DeliveryCheckpointOffset, `
SELECT COALESCE(MAX(offset_value), 0)
FROM delivery_kafka_checkpoints
WHERE consumer_group = $1
`, cfg.deliveryGroup); err != nil {
			return fmt.Errorf("query delivery checkpoint: %w", err)
		}
	}
	if err := fillReceiptOutboxStats(ctx, pool, cfg, &result.ReceiptOutbox); err != nil {
		return err
	}
	if err := fillDeliveryOutboxStats(ctx, pool, cfg, &result.DeliveryOutbox); err != nil {
		return err
	}
	return nil
}

func fillReceiptOutboxStats(ctx context.Context, pool *pgxpool.Pool, cfg config, stats *receiptOutboxStats) error {
	if err := pool.QueryRow(ctx, `
SELECT
    COUNT(*),
    COUNT(*) FILTER (WHERE status = 'PENDING'),
    COUNT(*) FILTER (WHERE status = 'PUBLISHED'),
    COUNT(*) FILTER (WHERE status = 'DLQ')
FROM receipt_outbox
WHERE tenant_id = $1 AND conversation_id = $2
`, cfg.tenantID, cfg.conversationID).Scan(&stats.Total, &stats.Pending, &stats.Published, &stats.DLQ); err != nil {
		return fmt.Errorf("query receipt outbox stats: %w", err)
	}
	rows, err := pool.Query(ctx, `
SELECT event_type, COUNT(*)
FROM receipt_outbox
WHERE tenant_id = $1 AND conversation_id = $2
GROUP BY event_type
ORDER BY event_type
`, cfg.tenantID, cfg.conversationID)
	if err != nil {
		return fmt.Errorf("query receipt outbox by type: %w", err)
	}
	defer rows.Close()
	stats.ByEventType = map[string]int64{}
	for rows.Next() {
		var eventType string
		var count int64
		if err := rows.Scan(&eventType, &count); err != nil {
			return fmt.Errorf("scan receipt outbox by type: %w", err)
		}
		stats.ByEventType[eventType] = count
	}
	return rows.Err()
}

func fillDeliveryOutboxStats(ctx context.Context, pool *pgxpool.Pool, cfg config, stats *outboxStats) error {
	if err := pool.QueryRow(ctx, `
SELECT
    COUNT(*),
    COUNT(*) FILTER (WHERE status = 'PENDING'),
    COUNT(*) FILTER (WHERE status = 'PUBLISHED'),
    COUNT(*) FILTER (WHERE status = 'DLQ')
FROM delivery_outbox
WHERE tenant_id = $1 AND conversation_id = $2
`, cfg.tenantID, cfg.conversationID).Scan(&stats.Total, &stats.Pending, &stats.Published, &stats.DLQ); err != nil {
		return fmt.Errorf("query delivery outbox stats: %w", err)
	}
	return nil
}

func readReceiptEvents(ctx context.Context, cfg config, wantByType map[string]int64) ([]receiptKafkaEvent, error) {
	wantTotal := 0
	for _, count := range wantByType {
		wantTotal += int(count)
	}
	if wantTotal == 0 {
		return nil, nil
	}
	if cfg.receiptEventsTopic == "" || len(cfg.kafkaBrokers) == 0 || cfg.receiptEventsGroup == "" {
		return nil, errors.New("receipt event readback requires kafka brokers, topic, and consumer group")
	}
	reader := kafkago.NewReader(kafkago.ReaderConfig{
		Brokers: cfg.kafkaBrokers,
		Topic:   cfg.receiptEventsTopic,
		GroupID: cfg.receiptEventsGroup,
	})
	defer reader.Close()

	deadline := time.Now().Add(cfg.waitTimeout)
	events := make([]receiptKafkaEvent, 0, wantTotal)
	seen := map[string]struct{}{}
	for len(events) < wantTotal && time.Now().Before(deadline) {
		readCtx, cancel := context.WithTimeout(ctx, cfg.pollInterval)
		message, err := reader.ReadMessage(readCtx)
		cancel()
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				continue
			}
			return events, fmt.Errorf("read receipt event: %w", err)
		}
		var event receipteventsv1.ReceiptEvent
		if err := proto.Unmarshal(message.Value, &event); err != nil {
			return events, fmt.Errorf("decode receipt event: %w", err)
		}
		if event.TenantId != cfg.tenantID || event.AggregateId != cfg.conversationID {
			continue
		}
		if _, ok := wantByType[event.EventType]; !ok {
			continue
		}
		if _, ok := seen[event.EventId]; ok {
			continue
		}
		seen[event.EventId] = struct{}{}
		events = append(events, summarizeReceiptKafkaEvent(message, &event))
	}
	if len(events) < wantTotal {
		return events, fmt.Errorf("receipt event readback timeout: got=%d want=%d", len(events), wantTotal)
	}
	return events, nil
}

func summarizeReceiptKafkaEvent(message kafkago.Message, event *receipteventsv1.ReceiptEvent) receiptKafkaEvent {
	result := receiptKafkaEvent{
		EventID:          event.EventId,
		EventType:        event.EventType,
		Partition:        message.Partition,
		Offset:           message.Offset,
		AggregateVersion: event.AggregateVersion,
		PartitionKey:     event.PartitionKey,
	}
	if payload := event.GetMessageReceived(); payload != nil {
		result.PayloadType = "message_received"
		result.MessageID = payload.MessageId
		result.UserID = payload.UserId
		result.DeviceID = payload.DeviceId
		result.CursorSeq = payload.CursorSeq
		return result
	}
	if payload := event.GetMessageRead(); payload != nil {
		result.PayloadType = "message_read"
		result.MessageID = payload.MessageId
		result.UserID = payload.UserId
		result.DeviceID = payload.DeviceId
		result.CursorSeq = payload.CursorSeq
	}
	return result
}

func writeSummary(cfg config, result summary) error {
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("encode summary: %w", err)
	}
	path := filepath.Join(cfg.resultDir, "receipt-summary.json")
	if err := os.WriteFile(path, encoded, 0o644); err != nil {
		return fmt.Errorf("write summary: %w", err)
	}
	fmt.Println(string(encoded))
	fmt.Printf("summary: %s\n", path)
	return nil
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

func envBool(fallback bool, names ...string) bool {
	for _, name := range names {
		value := strings.TrimSpace(os.Getenv(name))
		if value == "" {
			continue
		}
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return fallback
		}
		return parsed
	}
	return fallback
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
