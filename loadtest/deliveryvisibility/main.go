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
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	conversationv1 "github.com/qsyy0921/IM/api/proto/nexusim/conversation/v1"
	deliveryv1 "github.com/qsyy0921/IM/api/proto/nexusim/delivery/v1"
	"github.com/qsyy0921/IM/loadtest/internal/grpctls"
	conversationtimelinev1 "github.com/qsyy0921/IM/schemas/kafka"
	kafkago "github.com/segmentio/kafka-go"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type config struct {
	conversationTarget string
	deliveryTarget     string
	conversationTLS    grpctls.Config
	deliveryTLS        grpctls.Config
	kafkaBrokers       string
	timelineTopic      string
	resultDir          string
	pgDSN              string
	requestTimeout     time.Duration
	waitTimeout        time.Duration
	pollInterval       time.Duration
	tenantID           string
	conversationPrefix string
	consumerGroup      string
	operatorUserID     string
	senderUserID       string
	targetUserID       string
	targetDeviceID     string
	scenarios          string
	cleanup            bool
}

type summary struct {
	Commit                 string           `json:"commit"`
	CommitFull             string           `json:"commit_full"`
	GitDirty               bool             `json:"git_dirty"`
	GitStatusShort         string           `json:"git_status_short,omitempty"`
	ConversationTarget     string           `json:"conversation_target"`
	DeliveryTarget         string           `json:"delivery_target"`
	ConversationTLSEnabled bool             `json:"conversation_tls_enabled"`
	DeliveryTLSEnabled     bool             `json:"delivery_tls_enabled"`
	TenantID               string           `json:"tenant_id"`
	ConsumerGroup          string           `json:"consumer_group"`
	TimelineTopic          string           `json:"timeline_topic"`
	KafkaBrokers           []string         `json:"kafka_brokers"`
	StartedAt              time.Time        `json:"started_at"`
	FinishedAt             time.Time        `json:"finished_at"`
	Success                bool             `json:"success"`
	Error                  string           `json:"error,omitempty"`
	Scenarios              []scenarioResult `json:"scenarios"`
}

type scenarioResult struct {
	Scenario                   string   `json:"scenario"`
	ConversationID             string   `json:"conversation_id"`
	TargetUserID               string   `json:"target_user_id"`
	SenderUserID               string   `json:"sender_user_id"`
	SenderJoinSeq              int64    `json:"sender_join_seq"`
	TargetJoinSeq              int64    `json:"target_join_seq"`
	PreMessageSeq              int64    `json:"pre_message_seq"`
	PreMessageEventID          string   `json:"pre_message_event_id"`
	PreMessageID               string   `json:"pre_message_id"`
	BoundarySeq                int64    `json:"boundary_seq"`
	BoundaryChangeID           string   `json:"boundary_change_id"`
	PostMessageSeq             int64    `json:"post_message_seq"`
	PostMessageEventID         string   `json:"post_message_event_id"`
	PostMessageID              string   `json:"post_message_id"`
	MembershipStatus           string   `json:"membership_status"`
	MembershipJoinSeq          int64    `json:"membership_join_seq"`
	MembershipLeaveSeq         *int64   `json:"membership_leave_seq,omitempty"`
	TargetPreInboxCount        int64    `json:"target_pre_inbox_count"`
	TargetPostInboxCount       int64    `json:"target_post_inbox_count"`
	SenderPostInboxCount       int64    `json:"sender_post_inbox_count"`
	PullAfterBoundaryItemCount int      `json:"pull_after_boundary_item_count"`
	UnexpectedItemEventIDs     []string `json:"unexpected_item_event_ids,omitempty"`
	Success                    bool     `json:"success"`
	Error                      string   `json:"error,omitempty"`
}

type membershipProjection struct {
	Status   string
	JoinSeq  int64
	LeaveSeq *int64
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
	flag.StringVar(&cfg.deliveryTarget, "delivery-target", "127.0.0.1:10497", "delivery-service gRPC target")
	registerTLSFlags("conversation-tls", "NEXUSIM_CONVERSATION_TLS", "conversation-service", &cfg.conversationTLS)
	registerTLSFlags("delivery-tls", "NEXUSIM_DELIVERY_TLS", "delivery-service", &cfg.deliveryTLS)
	flag.StringVar(&cfg.kafkaBrokers, "kafka-brokers", "localhost:9092", "comma-separated Kafka brokers")
	flag.StringVar(&cfg.timelineTopic, "timeline-topic", "conversation.timeline.events", "conversation timeline Kafka topic")
	flag.StringVar(&cfg.resultDir, "result-dir", "loadtest/results/delivery-visibility-smoke", "result directory")
	flag.StringVar(&cfg.pgDSN, "pg-dsn", "", "PostgreSQL DSN")
	flag.DurationVar(&cfg.requestTimeout, "request-timeout", 3*time.Second, "per request timeout")
	flag.DurationVar(&cfg.waitTimeout, "wait-timeout", 15*time.Second, "wait timeout for async projections")
	flag.DurationVar(&cfg.pollInterval, "poll-interval", 200*time.Millisecond, "poll interval while waiting")
	flag.StringVar(&cfg.tenantID, "tenant-id", "tenant-delivery-visibility", "tenant id")
	flag.StringVar(&cfg.conversationPrefix, "conversation-prefix", "conv-delivery-visibility", "conversation id prefix")
	flag.StringVar(&cfg.consumerGroup, "consumer-group", "", "delivery timeline consumer group used by the running consumer")
	flag.StringVar(&cfg.operatorUserID, "operator-user-id", "owner-1", "conversation owner/operator user id")
	flag.StringVar(&cfg.senderUserID, "sender-user-id", "user-0", "active sender user id")
	flag.StringVar(&cfg.targetUserID, "target-user-id", "delivery-user-1", "target user id")
	flag.StringVar(&cfg.targetDeviceID, "target-device-id", "delivery-device-1", "target device id for PullInbox")
	flag.StringVar(&cfg.scenarios, "scenarios", "LEAVE,REMOVE", "comma-separated scenarios: LEAVE,REMOVE")
	flag.BoolVar(&cfg.cleanup, "cleanup", true, "delete existing rows for tenant before running")
	flag.Parse()
	return cfg
}

func registerTLSFlags(prefix string, envPrefix string, serviceName string, config *grpctls.Config) {
	flag.StringVar(&config.CAFile, prefix+"-ca-file", os.Getenv(envPrefix+"_CA_FILE"), "CA PEM for "+serviceName+" gRPC TLS")
	flag.StringVar(&config.ServerName, prefix+"-server-name", os.Getenv(envPrefix+"_SERVER_NAME"), "override server name for "+serviceName+" gRPC TLS")
	flag.StringVar(&config.ClientCertFile, prefix+"-client-cert-file", os.Getenv(envPrefix+"_CLIENT_CERT_FILE"), "client certificate PEM for "+serviceName+" gRPC mTLS")
	flag.StringVar(&config.ClientKeyFile, prefix+"-client-key-file", os.Getenv(envPrefix+"_CLIENT_KEY_FILE"), "client private key PEM for "+serviceName+" gRPC mTLS")
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

	conversationDialOption, err := grpctls.DialOption(cfg.conversationTLS, "conversation-tls")
	if err != nil {
		return fmt.Errorf("configure conversation-service TLS: %w", err)
	}
	conversationConn, err := grpc.NewClient(cfg.conversationTarget, conversationDialOption)
	if err != nil {
		return fmt.Errorf("dial conversation-service: %w", err)
	}
	defer conversationConn.Close()
	conversationClient := conversationv1.NewConversationServiceClient(conversationConn)

	deliveryDialOption, err := grpctls.DialOption(cfg.deliveryTLS, "delivery-tls")
	if err != nil {
		return fmt.Errorf("configure delivery-service TLS: %w", err)
	}
	deliveryConn, err := grpc.NewClient(cfg.deliveryTarget, deliveryDialOption)
	if err != nil {
		return fmt.Errorf("dial delivery-service: %w", err)
	}
	defer deliveryConn.Close()
	deliveryClient := deliveryv1.NewDeliveryServiceClient(deliveryConn)

	brokers := splitCSV(cfg.kafkaBrokers)
	if len(brokers) == 0 {
		return errors.New("kafka-brokers is required")
	}
	writer := &kafkago.Writer{
		Addr:                   kafkago.TCP(brokers...),
		Topic:                  cfg.timelineTopic,
		Balancer:               &kafkago.Hash{},
		RequiredAcks:           kafkago.RequireAll,
		AllowAutoTopicCreation: false,
	}
	defer writer.Close()

	result := summary{
		Commit:                 shortCommit(),
		CommitFull:             fullCommit(),
		GitDirty:               gitDirty(),
		GitStatusShort:         gitStatusShort(),
		ConversationTarget:     cfg.conversationTarget,
		DeliveryTarget:         cfg.deliveryTarget,
		ConversationTLSEnabled: cfg.conversationTLS.Enabled(),
		DeliveryTLSEnabled:     cfg.deliveryTLS.Enabled(),
		TenantID:               cfg.tenantID,
		ConsumerGroup:          cfg.consumerGroup,
		TimelineTopic:          cfg.timelineTopic,
		KafkaBrokers:           brokers,
		StartedAt:              time.Now().UTC(),
		Success:                true,
	}
	for _, scenario := range splitCSV(cfg.scenarios) {
		scenario = strings.ToUpper(strings.TrimSpace(scenario))
		if scenario == "" {
			continue
		}
		scenarioResult := runScenario(ctx, cfg, pool, conversationClient, deliveryClient, writer, scenario)
		if !scenarioResult.Success {
			result.Success = false
			if result.Error == "" {
				result.Error = scenarioResult.Error
			}
		}
		result.Scenarios = append(result.Scenarios, scenarioResult)
	}
	result.FinishedAt = time.Now().UTC()
	if len(result.Scenarios) == 0 {
		result.Success = false
		result.Error = "no scenarios were executed"
	}
	if err := writeSummary(cfg.resultDir, result); err != nil {
		return err
	}
	if !result.Success {
		return fmt.Errorf("delivery visibility smoke failed: %s", result.Error)
	}
	return nil
}

func runScenario(
	ctx context.Context,
	cfg config,
	pool *pgxpool.Pool,
	conversationClient conversationv1.ConversationServiceClient,
	deliveryClient deliveryv1.DeliveryServiceClient,
	writer *kafkago.Writer,
	scenario string,
) scenarioResult {
	lower := strings.ToLower(scenario)
	result := scenarioResult{
		Scenario:       scenario,
		ConversationID: cfg.conversationPrefix + "-" + lower,
		TargetUserID:   cfg.targetUserID + "-" + lower,
		SenderUserID:   cfg.senderUserID + "-" + lower,
	}
	if scenario != "LEAVE" && scenario != "REMOVE" {
		result.Error = "unsupported scenario"
		return result
	}
	if err := seedConversation(ctx, pool, cfg, result.ConversationID); err != nil {
		result.Error = err.Error()
		return result
	}
	senderJoin, err := createMemberChange(ctx, cfg, conversationClient, result.ConversationID, cfg.operatorUserID, result.SenderUserID, "JOIN", "join-sender-"+lower, 0)
	if err != nil {
		result.Error = "join sender: " + err.Error()
		return result
	}
	result.SenderJoinSeq = senderJoin.GetBoundarySeq()
	targetJoin, err := createMemberChange(ctx, cfg, conversationClient, result.ConversationID, cfg.operatorUserID, result.TargetUserID, "JOIN", "join-target-"+lower, 0)
	if err != nil {
		result.Error = "join target: " + err.Error()
		return result
	}
	result.TargetJoinSeq = targetJoin.GetBoundarySeq()
	if err := waitMembership(ctx, pool, cfg, result.ConversationID, result.SenderUserID, "ACTIVE", nil); err != nil {
		result.Error = "wait sender projection: " + err.Error()
		return result
	}
	if err := waitMembership(ctx, pool, cfg, result.ConversationID, result.TargetUserID, "ACTIVE", nil); err != nil {
		result.Error = "wait target projection: " + err.Error()
		return result
	}

	currentSeq, err := currentConversationSeq(ctx, pool, cfg, result.ConversationID)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	result.PreMessageSeq = currentSeq + 1
	result.PreMessageEventID = "evt_visibility_pre_" + lower
	result.PreMessageID = "msg_visibility_pre_" + lower
	if err := publishMessageEvent(ctx, cfg, writer, result.ConversationID, result.SenderUserID, result.PreMessageEventID, result.PreMessageID, result.PreMessageSeq); err != nil {
		result.Error = "publish pre message: " + err.Error()
		return result
	}
	if err := advanceConversationSeq(ctx, pool, cfg, result.ConversationID, result.PreMessageSeq); err != nil {
		result.Error = err.Error()
		return result
	}
	if err := waitInboxSeq(ctx, pool, cfg, result.ConversationID, result.TargetUserID, result.PreMessageSeq); err != nil {
		result.Error = "wait pre inbox: " + err.Error()
		return result
	}

	operator := cfg.operatorUserID
	if scenario == "LEAVE" {
		operator = result.TargetUserID
	}
	boundary, err := createMemberChange(ctx, cfg, conversationClient, result.ConversationID, operator, result.TargetUserID, scenario, "boundary-"+lower, 0)
	if err != nil {
		result.Error = "boundary change: " + err.Error()
		return result
	}
	result.BoundaryChangeID = boundary.GetChangeId()
	result.BoundarySeq = boundary.GetBoundarySeq()
	if err := waitMembership(ctx, pool, cfg, result.ConversationID, result.TargetUserID, "", &result.BoundarySeq); err != nil {
		result.Error = "wait boundary projection: " + err.Error()
		return result
	}
	projection, err := readMembership(ctx, pool, cfg, result.ConversationID, result.TargetUserID)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	result.MembershipStatus = projection.Status
	result.MembershipJoinSeq = projection.JoinSeq
	result.MembershipLeaveSeq = projection.LeaveSeq

	result.PostMessageSeq = result.BoundarySeq + 1
	result.PostMessageEventID = "evt_visibility_post_" + lower
	result.PostMessageID = "msg_visibility_post_" + lower
	if err := publishMessageEvent(ctx, cfg, writer, result.ConversationID, result.SenderUserID, result.PostMessageEventID, result.PostMessageID, result.PostMessageSeq); err != nil {
		result.Error = "publish post message: " + err.Error()
		return result
	}
	if err := advanceConversationSeq(ctx, pool, cfg, result.ConversationID, result.PostMessageSeq); err != nil {
		result.Error = err.Error()
		return result
	}
	if err := waitInboxSeq(ctx, pool, cfg, result.ConversationID, result.SenderUserID, result.PostMessageSeq); err != nil {
		result.Error = "wait sender post inbox: " + err.Error()
		return result
	}

	result.TargetPreInboxCount, _ = countInboxAtOrBefore(ctx, pool, cfg, result.ConversationID, result.TargetUserID, result.BoundarySeq)
	result.TargetPostInboxCount, _ = countInboxAfter(ctx, pool, cfg, result.ConversationID, result.TargetUserID, result.BoundarySeq)
	result.SenderPostInboxCount, _ = countInboxSeq(ctx, pool, cfg, result.ConversationID, result.SenderUserID, result.PostMessageSeq)
	pullCount, unexpected, err := pullAfterBoundary(ctx, cfg, deliveryClient, result.ConversationID, result.TargetUserID, result.BoundarySeq)
	if err != nil {
		result.Error = "pull after boundary: " + err.Error()
		return result
	}
	result.PullAfterBoundaryItemCount = pullCount
	result.UnexpectedItemEventIDs = unexpected
	if result.MembershipStatus == "ACTIVE" {
		result.Error = "target membership is still ACTIVE after boundary"
		return result
	}
	if result.MembershipLeaveSeq == nil || *result.MembershipLeaveSeq != result.BoundarySeq {
		result.Error = "membership leave_seq does not match boundary_seq"
		return result
	}
	if result.TargetPreInboxCount <= 0 {
		result.Error = "target did not receive pre-boundary message"
		return result
	}
	if result.SenderPostInboxCount <= 0 {
		result.Error = "post-boundary message was not consumed by active sender"
		return result
	}
	if result.TargetPostInboxCount != 0 || result.PullAfterBoundaryItemCount != 0 {
		result.Error = "target received post-boundary inbox items"
		return result
	}
	result.Success = true
	return result
}

func cleanupTenant(ctx context.Context, pool *pgxpool.Pool, cfg config) error {
	statements := []struct {
		sql  string
		args []any
	}{
		{`DELETE FROM delivery_outbox WHERE tenant_id = $1`, []any{cfg.tenantID}},
		{`DELETE FROM device_delivery_cursors WHERE tenant_id = $1`, []any{cfg.tenantID}},
		{`DELETE FROM user_inbox WHERE tenant_id = $1`, []any{cfg.tenantID}},
		{`DELETE FROM delivery_membership_projection WHERE tenant_id = $1`, []any{cfg.tenantID}},
		{`DELETE FROM delivery_kafka_checkpoints WHERE consumer_group = $1`, []any{cfg.consumerGroup}},
		{`DELETE FROM message_outbox WHERE tenant_id = $1`, []any{cfg.tenantID}},
		{`DELETE FROM conversation_timeline_events WHERE tenant_id = $1`, []any{cfg.tenantID}},
		{`DELETE FROM message_log WHERE tenant_id = $1`, []any{cfg.tenantID}},
		{`DELETE FROM conversation_seq WHERE tenant_id = $1`, []any{cfg.tenantID}},
		{`DELETE FROM member_change_saga WHERE tenant_id = $1`, []any{cfg.tenantID}},
		{`DELETE FROM conversation_members WHERE tenant_id = $1`, []any{cfg.tenantID}},
		{`DELETE FROM conversations WHERE tenant_id = $1`, []any{cfg.tenantID}},
	}
	for _, statement := range statements {
		if _, err := pool.Exec(ctx, statement.sql, statement.args...); err != nil {
			return fmt.Errorf("cleanup tenant: %w", err)
		}
	}
	return nil
}

func seedConversation(ctx context.Context, pool *pgxpool.Pool, cfg config, conversationID string) error {
	_, err := pool.Exec(ctx, `
INSERT INTO conversations (
    tenant_id, conversation_id, conversation_type, status, conversation_mode,
    fanout_mode, fanout_policy_version, member_version, permission_version, current_seq_shard
) VALUES ($1, $2, 'GROUP', 'ACTIVE', 'LOCAL_ROW_LOCK', 'WRITE_FANOUT', 1, 1, 1, 'local')
`, cfg.tenantID, conversationID)
	if err != nil {
		return fmt.Errorf("seed conversation %s: %w", conversationID, err)
	}
	_, err = pool.Exec(ctx, `
INSERT INTO conversation_members (
    tenant_id, conversation_id, user_id, role, status, member_version, permission_version
) VALUES ($1, $2, $3, 'OWNER', 'ACTIVE', 1, 1)
`, cfg.tenantID, conversationID, cfg.operatorUserID)
	if err != nil {
		return fmt.Errorf("seed conversation %s: %w", conversationID, err)
	}
	return nil
}

func createMemberChange(
	ctx context.Context,
	cfg config,
	client conversationv1.ConversationServiceClient,
	conversationID string,
	operatorUserID string,
	targetUserID string,
	changeType string,
	idempotencyKey string,
	expectedVersion int64,
) (*conversationv1.CreateMemberChangeResponse, error) {
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	response, err := client.CreateMemberChange(requestCtx, &conversationv1.CreateMemberChangeRequest{
		AuthContext: &conversationv1.AuthContext{
			TenantId:  cfg.tenantID,
			UserId:    operatorUserID,
			DeviceId:  "delivery-visibility",
			SessionId: "delivery-visibility",
			TraceId:   "delivery-visibility-" + strings.ToLower(changeType),
			RequestId: "delivery-visibility-" + idempotencyKey,
		},
		ConversationId:        conversationID,
		TargetUserId:          targetUserID,
		ChangeType:            memberChangeType(changeType),
		TargetRole:            conversationv1.MemberRole_MEMBER_ROLE_MEMBER,
		ExpectedMemberVersion: expectedVersion,
		IdempotencyKey:        idempotencyKey,
		ConflictPolicy:        conversationv1.MemberChangeConflictPolicy_MEMBER_CHANGE_CONFLICT_POLICY_REJECT,
		Reason:                "delivery visibility smoke " + strings.ToLower(changeType),
	})
	if err != nil {
		return nil, err
	}
	return response, nil
}

func memberChangeType(value string) conversationv1.MemberChangeType {
	switch strings.ToUpper(value) {
	case "JOIN":
		return conversationv1.MemberChangeType_MEMBER_CHANGE_TYPE_JOIN
	case "LEAVE":
		return conversationv1.MemberChangeType_MEMBER_CHANGE_TYPE_LEAVE
	case "REMOVE":
		return conversationv1.MemberChangeType_MEMBER_CHANGE_TYPE_REMOVE
	default:
		return conversationv1.MemberChangeType_MEMBER_CHANGE_TYPE_UNSPECIFIED
	}
}

func publishMessageEvent(
	ctx context.Context,
	cfg config,
	writer *kafkago.Writer,
	conversationID string,
	senderUserID string,
	eventID string,
	messageID string,
	seq int64,
) error {
	payload, err := structpb.NewStruct(map[string]any{
		"text": fmt.Sprintf("delivery visibility message seq %d", seq),
	})
	if err != nil {
		return err
	}
	now := timestamppb.Now()
	partitionKey := cfg.tenantID + ":" + conversationID
	event := &conversationtimelinev1.ConversationTimelineEvent{
		EventId:          eventID,
		EventType:        "message.persisted.v1",
		EventVersion:     "v1",
		TenantId:         cfg.tenantID,
		AggregateType:    "conversation",
		AggregateId:      conversationID,
		AggregateVersion: seq,
		PartitionKey:     partitionKey,
		MappingVersion:   "message.persisted.v1",
		TraceId:          "delivery-visibility-message",
		CorrelationId:    eventID,
		CausationId:      eventID,
		Producer:         "delivery-visibility-smoke",
		OccurredAt:       now,
		Metadata: &conversationtimelinev1.TimelineMetadata{
			FanoutMode:          "WRITE_FANOUT",
			FanoutPolicyVersion: 1,
			PermissionVersion:   seq,
			Classification:      "INTERNAL",
			MappingVersion:      "message.persisted.v1",
		},
		Payload: &conversationtimelinev1.ConversationTimelineEvent_MessagePersisted{
			MessagePersisted: &conversationtimelinev1.MessagePersistedV1{
				MessageId:       messageID,
				ConversationId:  conversationID,
				ConversationSeq: seq,
				SenderId:        senderUserID,
				DeviceId:        "delivery-visibility-device",
				ClientMsgId:     "client-" + eventID,
				CommandHash:     eventID,
				MessageType:     "TEXT",
				Payload:         payload,
				AcceptedAt:      now,
			},
		},
	}
	encoded, err := proto.Marshal(event)
	if err != nil {
		return err
	}
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	return writer.WriteMessages(requestCtx, kafkago.Message{
		Key:   []byte(partitionKey),
		Value: encoded,
	})
}

func currentConversationSeq(ctx context.Context, pool *pgxpool.Pool, cfg config, conversationID string) (int64, error) {
	var seq int64
	err := pool.QueryRow(ctx, `
SELECT COALESCE(current_seq, 0)
FROM conversation_seq
WHERE tenant_id = $1 AND conversation_id = $2
`, cfg.tenantID, conversationID).Scan(&seq)
	if err != nil {
		return 0, fmt.Errorf("read conversation seq: %w", err)
	}
	return seq, nil
}

func advanceConversationSeq(ctx context.Context, pool *pgxpool.Pool, cfg config, conversationID string, seq int64) error {
	_, err := pool.Exec(ctx, `
INSERT INTO conversation_seq (tenant_id, conversation_id, current_seq, updated_at)
VALUES ($1, $2, $3, now())
ON CONFLICT (tenant_id, conversation_id) DO UPDATE
SET current_seq = GREATEST(conversation_seq.current_seq, EXCLUDED.current_seq),
    updated_at = now()
`, cfg.tenantID, conversationID, seq)
	if err != nil {
		return fmt.Errorf("advance conversation seq: %w", err)
	}
	return nil
}

func waitMembership(ctx context.Context, pool *pgxpool.Pool, cfg config, conversationID string, userID string, wantStatus string, wantLeaveSeq *int64) error {
	deadline := time.Now().Add(cfg.waitTimeout)
	for {
		projection, err := readMembership(ctx, pool, cfg, conversationID, userID)
		if err == nil {
			statusOK := wantStatus == "" || projection.Status == wantStatus
			leaveOK := wantLeaveSeq == nil || (projection.LeaveSeq != nil && *projection.LeaveSeq == *wantLeaveSeq)
			if statusOK && leaveOK {
				return nil
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("membership projection timeout user=%s status=%s leave_seq=%v", userID, wantStatus, wantLeaveSeq)
		}
		time.Sleep(cfg.pollInterval)
	}
}

func readMembership(ctx context.Context, pool *pgxpool.Pool, cfg config, conversationID string, userID string) (membershipProjection, error) {
	var projection membershipProjection
	err := pool.QueryRow(ctx, `
SELECT status, join_seq, leave_seq
FROM delivery_membership_projection
WHERE tenant_id = $1 AND conversation_id = $2 AND user_id = $3
`, cfg.tenantID, conversationID, userID).Scan(&projection.Status, &projection.JoinSeq, &projection.LeaveSeq)
	if err != nil {
		return membershipProjection{}, err
	}
	return projection, nil
}

func waitInboxSeq(ctx context.Context, pool *pgxpool.Pool, cfg config, conversationID string, userID string, seq int64) error {
	deadline := time.Now().Add(cfg.waitTimeout)
	for {
		count, err := countInboxSeq(ctx, pool, cfg, conversationID, userID, seq)
		if err == nil && count > 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("inbox seq timeout user=%s seq=%d", userID, seq)
		}
		time.Sleep(cfg.pollInterval)
	}
}

func countInboxSeq(ctx context.Context, pool *pgxpool.Pool, cfg config, conversationID string, userID string, seq int64) (int64, error) {
	var count int64
	err := pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM user_inbox
WHERE tenant_id = $1 AND conversation_id = $2 AND user_id = $3 AND conversation_seq = $4
`, cfg.tenantID, conversationID, userID, seq).Scan(&count)
	return count, err
}

func countInboxAtOrBefore(ctx context.Context, pool *pgxpool.Pool, cfg config, conversationID string, userID string, seq int64) (int64, error) {
	var count int64
	err := pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM user_inbox
WHERE tenant_id = $1 AND conversation_id = $2 AND user_id = $3 AND conversation_seq <= $4
`, cfg.tenantID, conversationID, userID, seq).Scan(&count)
	return count, err
}

func countInboxAfter(ctx context.Context, pool *pgxpool.Pool, cfg config, conversationID string, userID string, seq int64) (int64, error) {
	var count int64
	err := pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM user_inbox
WHERE tenant_id = $1 AND conversation_id = $2 AND user_id = $3 AND conversation_seq > $4
`, cfg.tenantID, conversationID, userID, seq).Scan(&count)
	return count, err
}

func pullAfterBoundary(
	ctx context.Context,
	cfg config,
	client deliveryv1.DeliveryServiceClient,
	conversationID string,
	userID string,
	boundarySeq int64,
) (int, []string, error) {
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	response, err := client.PullInbox(requestCtx, &deliveryv1.PullInboxRequest{
		AuthContext: &deliveryv1.AuthContext{
			TenantId:  cfg.tenantID,
			UserId:    userID,
			DeviceId:  cfg.targetDeviceID,
			SessionId: "delivery-visibility",
			TraceId:   "delivery-visibility-pull",
			RequestId: "delivery-visibility-pull",
		},
		ConversationId: conversationID,
		AfterSeq:       boundarySeq,
		Limit:          100,
	})
	if err != nil {
		return 0, nil, err
	}
	ids := make([]string, 0, len(response.GetItems()))
	for _, item := range response.GetItems() {
		ids = append(ids, item.GetEventId())
	}
	sort.Strings(ids)
	return len(response.GetItems()), ids, nil
}

func writeSummary(resultDir string, result summary) error {
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(resultDir, "delivery-visibility-summary.json")
	if err := os.WriteFile(path, encoded, 0o644); err != nil {
		return err
	}
	fmt.Println(string(encoded))
	fmt.Printf("summary: %s\n", path)
	return nil
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

func percentile(sorted []float64, quantile float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	index := int(math.Ceil(float64(len(sorted))*quantile)) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}
