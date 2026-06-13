package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	policyv1 "github.com/qsyy0921/IM/api/proto/nexusim/policy/v1"
	conversationtimelinev1 "github.com/qsyy0921/IM/schemas/kafka"
	kafkago "github.com/segmentio/kafka-go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	topicConversationTimelineEvents = "conversation.timeline.events"

	eventTypeMemberJoined      = "conversation.member.joined.v1"
	eventTypeMemberLeft        = "conversation.member.left.v1"
	eventTypeMemberRoleChanged = "conversation.member.role_changed.v1"

	roleOwner  = "OWNER"
	roleAdmin  = "ADMIN"
	roleMember = "MEMBER"

	statusActive = "ACTIVE"
	statusLeft   = "LEFT"

	tenantAllowPermissionVersion = int64(101)
	roleDenyClassification       = "CONVERSATION_ROLE_DENIED"
	roleDenyReason               = "conversation role policy denied"
	tenantAllowClassification    = "POLICY_TENANT_ALLOW"
)

type config struct {
	brokers        []string
	topic          string
	consumerGroup  string
	policyGRPCAddr string
	pgDSN          string
	resultDir      string
	tenantID       string
	conversationID string
	userID         string
	timeout        time.Duration
	cleanup        bool
}

type summary struct {
	Commit                 string                 `json:"commit"`
	CommitFull             string                 `json:"commit_full"`
	GitDirty               bool                   `json:"git_dirty"`
	GitStatusShort         string                 `json:"git_status_short,omitempty"`
	ResultDir              string                 `json:"result_dir"`
	Topic                  string                 `json:"topic"`
	ConsumerGroup          string                 `json:"consumer_group"`
	TenantID               string                 `json:"tenant_id"`
	ConversationID         string                 `json:"conversation_id"`
	UserID                 string                 `json:"user_id"`
	StartedAt              time.Time              `json:"started_at"`
	FinishedAt             time.Time              `json:"finished_at"`
	Success                bool                   `json:"success"`
	Error                  string                 `json:"error,omitempty"`
	JoinedProjection       memberProjection       `json:"joined_projection"`
	RoleChangedProjection  memberProjection       `json:"role_changed_projection"`
	LeftProjection         memberProjection       `json:"left_projection"`
	AllowedDecision        policyDecisionSnapshot `json:"allowed_decision"`
	RoleDeniedDecision     policyDecisionSnapshot `json:"role_denied_decision"`
	InactiveDeniedDecision policyDecisionSnapshot `json:"inactive_denied_decision"`
	StaleDecision          policyDecisionSnapshot `json:"stale_decision"`
	CheckpointOffset       int64                  `json:"checkpoint_offset_value"`
	Events                 []string               `json:"events"`
}

type memberProjection struct {
	ConversationID    string `json:"conversation_id"`
	UserID            string `json:"user_id"`
	Role              string `json:"role"`
	Status            string `json:"status"`
	MemberVersion     int64  `json:"member_version"`
	PermissionVersion int64  `json:"permission_version"`
	UpdatedByEventID  string `json:"updated_by_event_id"`
}

type policyDecisionSnapshot struct {
	Checked                       bool   `json:"checked"`
	Allowed                       bool   `json:"allowed"`
	PermissionVersion             int64  `json:"permission_version"`
	ConversationPermissionVersion int64  `json:"conversation_permission_version"`
	Classification                string `json:"classification"`
	Reason                        string `json:"reason,omitempty"`
	ErrorCode                     string `json:"error_code,omitempty"`
	ErrorMessage                  string `json:"error_message,omitempty"`
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
	var brokers string
	flag.StringVar(&brokers, "brokers", "localhost:9092", "comma-separated Kafka brokers")
	flag.StringVar(&cfg.topic, "topic", topicConversationTimelineEvents, "conversation timeline events topic")
	flag.StringVar(&cfg.consumerGroup, "consumer-group", "nexusim-policy-role-smoke", "policy timeline consumer group")
	flag.StringVar(&cfg.policyGRPCAddr, "policy-grpc-addr", "127.0.0.1:10800", "policy-service gRPC address")
	flag.StringVar(&cfg.pgDSN, "pg-dsn", "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable", "PostgreSQL DSN")
	flag.StringVar(&cfg.resultDir, "result-dir", "H:\\NexusIM\\loadtest-results\\policy-role-smoke", "result directory")
	flag.StringVar(&cfg.tenantID, "tenant-id", "tenant-policy-role-smoke", "tenant id")
	flag.StringVar(&cfg.conversationID, "conversation-id", "conv-policy-role-smoke", "conversation id")
	flag.StringVar(&cfg.userID, "user-id", "policy-role-user", "user id")
	flag.DurationVar(&cfg.timeout, "timeout", 25*time.Second, "smoke timeout")
	flag.BoolVar(&cfg.cleanup, "cleanup", true, "cleanup policy rows for the smoke tenant")
	flag.Parse()
	cfg.brokers = splitCSV(brokers)
	if cfg.timeout <= 0 {
		cfg.timeout = 25 * time.Second
	}
	return cfg
}

func run(cfg config) error {
	if err := os.MkdirAll(cfg.resultDir, 0o755); err != nil {
		return fmt.Errorf("create result dir: %w", err)
	}
	started := time.Now().UTC()
	s := summary{
		Commit:         gitOutput("rev-parse", "--short", "HEAD"),
		CommitFull:     gitOutput("rev-parse", "HEAD"),
		GitStatusShort: gitOutput("status", "--short"),
		ResultDir:      cfg.resultDir,
		Topic:          cfg.topic,
		ConsumerGroup:  cfg.consumerGroup,
		TenantID:       cfg.tenantID,
		ConversationID: cfg.conversationID,
		UserID:         cfg.userID,
		StartedAt:      started,
	}
	s.GitDirty = strings.TrimSpace(s.GitStatusShort) != ""
	defer func() {
		s.FinishedAt = time.Now().UTC()
		_ = writeSummary(cfg.resultDir, s)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), cfg.timeout)
	defer cancel()

	pool, err := pgxpool.New(ctx, cfg.pgDSN)
	if err != nil {
		s.Error = fmt.Sprintf("connect postgres: %v", err)
		return fmt.Errorf("connect postgres: %w", err)
	}
	defer pool.Close()
	if cfg.cleanup {
		if err := cleanupTenant(ctx, pool, cfg); err != nil {
			s.Error = err.Error()
			return err
		}
	}
	if err := seedPolicyRules(ctx, pool, cfg); err != nil {
		s.Error = err.Error()
		return err
	}
	if err := ensureTopic(ctx, cfg.brokers, cfg.topic); err != nil {
		s.Error = fmt.Sprintf("ensure topic: %v", err)
		return fmt.Errorf("ensure topic: %w", err)
	}

	writer := &kafkago.Writer{
		Addr:                   kafkago.TCP(cfg.brokers...),
		Topic:                  cfg.topic,
		Balancer:               &kafkago.Hash{},
		RequiredAcks:           kafkago.RequireAll,
		AllowAutoTopicCreation: true,
		WriteTimeout:           5 * time.Second,
		ReadTimeout:            5 * time.Second,
	}
	defer writer.Close()

	conn, err := grpc.NewClient("passthrough:///"+cfg.policyGRPCAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		s.Error = fmt.Sprintf("dial policy service: %v", err)
		return fmt.Errorf("dial policy service: %w", err)
	}
	defer conn.Close()
	client := policyv1.NewPolicyServiceClient(conn)

	runID := randomSuffix()
	joinedID := "policy-role-joined-" + runID
	roleChangedID := "policy-role-changed-" + runID
	leftID := "policy-role-left-" + runID

	if err := writer.WriteMessages(ctx, timelineMessage(joinedID, memberJoinedEvent(joinedID, cfg, 1, 7, roleAdmin, statusActive))); err != nil {
		s.Error = fmt.Sprintf("write joined event: %v", err)
		return fmt.Errorf("write joined event: %w", err)
	}
	s.Events = append(s.Events, joinedID)
	s.JoinedProjection, err = waitProjection(ctx, pool, cfg, roleAdmin, statusActive, 7, 7, joinedID)
	if err != nil {
		s.Error = err.Error()
		return err
	}
	s.AllowedDecision, err = checkPolicy(ctx, client, cfg, 7)
	if err != nil {
		s.Error = fmt.Sprintf("check allowed decision: %v", err)
		return fmt.Errorf("check allowed decision: %w", err)
	}
	if err := assertAllowedDecision(s.AllowedDecision); err != nil {
		s.Error = err.Error()
		return err
	}

	if err := writer.WriteMessages(ctx, timelineMessage(roleChangedID, memberRoleChangedEvent(roleChangedID, cfg, 2, 8, roleAdmin, roleMember, statusActive))); err != nil {
		s.Error = fmt.Sprintf("write role changed event: %v", err)
		return fmt.Errorf("write role changed event: %w", err)
	}
	s.Events = append(s.Events, roleChangedID)
	s.RoleChangedProjection, err = waitProjection(ctx, pool, cfg, roleMember, statusActive, 8, 8, roleChangedID)
	if err != nil {
		s.Error = err.Error()
		return err
	}
	s.RoleDeniedDecision, err = checkPolicy(ctx, client, cfg, 8)
	if err != nil {
		s.Error = fmt.Sprintf("check role denied decision: %v", err)
		return fmt.Errorf("check role denied decision: %w", err)
	}
	if err := assertDeniedDecision(s.RoleDeniedDecision, 8); err != nil {
		s.Error = err.Error()
		return err
	}
	s.StaleDecision, err = checkPolicy(ctx, client, cfg, 7)
	if err != nil {
		s.Error = fmt.Sprintf("check stale decision: %v", err)
		return fmt.Errorf("check stale decision: %w", err)
	}
	if s.StaleDecision.ErrorCode != codes.Unavailable.String() {
		s.Error = fmt.Sprintf("expected stale decision unavailable, got %+v", s.StaleDecision)
		return errors.New(s.Error)
	}

	if err := writer.WriteMessages(ctx, timelineMessage(leftID, memberLeftEvent(leftID, cfg, 3, 9, roleMember, statusLeft))); err != nil {
		s.Error = fmt.Sprintf("write left event: %v", err)
		return fmt.Errorf("write left event: %w", err)
	}
	s.Events = append(s.Events, leftID)
	s.LeftProjection, err = waitProjection(ctx, pool, cfg, roleMember, statusLeft, 9, 9, leftID)
	if err != nil {
		s.Error = err.Error()
		return err
	}
	s.InactiveDeniedDecision, err = checkPolicy(ctx, client, cfg, 9)
	if err != nil {
		s.Error = fmt.Sprintf("check inactive denied decision: %v", err)
		return fmt.Errorf("check inactive denied decision: %w", err)
	}
	if err := assertDeniedDecision(s.InactiveDeniedDecision, 9); err != nil {
		s.Error = err.Error()
		return err
	}
	s.CheckpointOffset, err = waitCheckpoint(ctx, pool, cfg)
	if err != nil {
		s.Error = err.Error()
		return err
	}
	s.Success = true
	return nil
}

func timelineMessage(eventID string, event *conversationtimelinev1.ConversationTimelineEvent) kafkago.Message {
	value, _ := proto.Marshal(event)
	return kafkago.Message{
		Key:   []byte(event.GetTenantId() + ":" + event.GetAggregateId()),
		Value: value,
		Headers: []kafkago.Header{
			{Key: "event_id", Value: []byte(eventID)},
		},
	}
}

func memberJoinedEvent(eventID string, cfg config, seq int64, version int64, role string, status string) *conversationtimelinev1.ConversationTimelineEvent {
	return baseTimelineEvent(eventID, eventTypeMemberJoined, cfg, seq, version, &conversationtimelinev1.ConversationTimelineEvent_ConversationMemberJoined{
		ConversationMemberJoined: &conversationtimelinev1.ConversationMemberJoinedV1{
			ChangeId:          "change-" + eventID,
			ConversationId:    cfg.conversationID,
			BoundarySeq:       seq,
			TargetUserId:      cfg.userID,
			OperatorUserId:    "policy-role-operator",
			ChangeType:        conversationtimelinev1.ConversationMemberChangeType_CONVERSATION_MEMBER_CHANGE_TYPE_JOIN,
			OldRole:           conversationtimelinev1.ConversationMemberRole_CONVERSATION_MEMBER_ROLE_UNSPECIFIED,
			NewRole:           timelineRole(role),
			OldStatus:         conversationtimelinev1.ConversationMemberStatus_CONVERSATION_MEMBER_STATUS_UNSPECIFIED,
			NewStatus:         timelineStatus(status),
			MemberVersion:     version,
			PermissionVersion: version,
			Reason:            "policy role smoke join",
			OccurredAt:        timestamppb.Now(),
		},
	})
}

func memberRoleChangedEvent(eventID string, cfg config, seq int64, version int64, oldRole string, newRole string, status string) *conversationtimelinev1.ConversationTimelineEvent {
	return baseTimelineEvent(eventID, eventTypeMemberRoleChanged, cfg, seq, version, &conversationtimelinev1.ConversationTimelineEvent_ConversationMemberRoleChanged{
		ConversationMemberRoleChanged: &conversationtimelinev1.ConversationMemberRoleChangedV1{
			ChangeId:          "change-" + eventID,
			ConversationId:    cfg.conversationID,
			BoundarySeq:       seq,
			TargetUserId:      cfg.userID,
			OperatorUserId:    "policy-role-operator",
			ChangeType:        conversationtimelinev1.ConversationMemberChangeType_CONVERSATION_MEMBER_CHANGE_TYPE_ROLE_CHANGED,
			OldRole:           timelineRole(oldRole),
			NewRole:           timelineRole(newRole),
			OldStatus:         timelineStatus(statusActive),
			NewStatus:         timelineStatus(status),
			MemberVersion:     version,
			PermissionVersion: version,
			Reason:            "policy role smoke demotion",
			OccurredAt:        timestamppb.Now(),
		},
	})
}

func memberLeftEvent(eventID string, cfg config, seq int64, version int64, role string, status string) *conversationtimelinev1.ConversationTimelineEvent {
	return baseTimelineEvent(eventID, eventTypeMemberLeft, cfg, seq, version, &conversationtimelinev1.ConversationTimelineEvent_ConversationMemberLeft{
		ConversationMemberLeft: &conversationtimelinev1.ConversationMemberLeftV1{
			ChangeId:          "change-" + eventID,
			ConversationId:    cfg.conversationID,
			BoundarySeq:       seq,
			TargetUserId:      cfg.userID,
			OperatorUserId:    cfg.userID,
			ChangeType:        conversationtimelinev1.ConversationMemberChangeType_CONVERSATION_MEMBER_CHANGE_TYPE_LEAVE,
			OldRole:           timelineRole(role),
			NewRole:           timelineRole(role),
			OldStatus:         timelineStatus(statusActive),
			NewStatus:         timelineStatus(status),
			MemberVersion:     version,
			PermissionVersion: version,
			Reason:            "policy role smoke leave",
			OccurredAt:        timestamppb.Now(),
		},
	})
}

func baseTimelineEvent(eventID string, eventType string, cfg config, seq int64, permissionVersion int64, payload any) *conversationtimelinev1.ConversationTimelineEvent {
	event := &conversationtimelinev1.ConversationTimelineEvent{
		EventId:          eventID,
		EventType:        eventType,
		EventVersion:     "v1",
		TenantId:         cfg.tenantID,
		AggregateType:    "conversation",
		AggregateId:      cfg.conversationID,
		AggregateVersion: seq,
		PartitionKey:     cfg.tenantID + ":" + cfg.conversationID,
		MappingVersion:   "1",
		TraceId:          "trace-policy-role-smoke",
		CorrelationId:    "policy-role-smoke",
		CausationId:      eventID,
		Producer:         "policy-role-smoke",
		OccurredAt:       timestamppb.Now(),
		Metadata: &conversationtimelinev1.TimelineMetadata{
			PermissionVersion: permissionVersion,
			Classification:    "POLICY_ROLE_SMOKE",
			MappingVersion:    "1",
		},
	}
	switch typed := payload.(type) {
	case *conversationtimelinev1.ConversationTimelineEvent_ConversationMemberJoined:
		event.Payload = typed
	case *conversationtimelinev1.ConversationTimelineEvent_ConversationMemberRoleChanged:
		event.Payload = typed
	case *conversationtimelinev1.ConversationTimelineEvent_ConversationMemberLeft:
		event.Payload = typed
	}
	return event
}

func timelineRole(role string) conversationtimelinev1.ConversationMemberRole {
	switch role {
	case roleOwner:
		return conversationtimelinev1.ConversationMemberRole_CONVERSATION_MEMBER_ROLE_OWNER
	case roleAdmin:
		return conversationtimelinev1.ConversationMemberRole_CONVERSATION_MEMBER_ROLE_ADMIN
	case roleMember:
		return conversationtimelinev1.ConversationMemberRole_CONVERSATION_MEMBER_ROLE_MEMBER
	default:
		return conversationtimelinev1.ConversationMemberRole_CONVERSATION_MEMBER_ROLE_UNSPECIFIED
	}
}

func timelineStatus(status string) conversationtimelinev1.ConversationMemberStatus {
	switch status {
	case statusActive:
		return conversationtimelinev1.ConversationMemberStatus_CONVERSATION_MEMBER_STATUS_ACTIVE
	case statusLeft:
		return conversationtimelinev1.ConversationMemberStatus_CONVERSATION_MEMBER_STATUS_LEFT
	default:
		return conversationtimelinev1.ConversationMemberStatus_CONVERSATION_MEMBER_STATUS_UNSPECIFIED
	}
}

func waitProjection(ctx context.Context, pool *pgxpool.Pool, cfg config, role string, memberStatus string, memberVersion int64, permissionVersion int64, eventID string) (memberProjection, error) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		projection, ok, err := readProjection(ctx, pool, cfg)
		if err != nil {
			return memberProjection{}, err
		}
		if ok &&
			projection.Role == role &&
			projection.Status == memberStatus &&
			projection.MemberVersion == memberVersion &&
			projection.PermissionVersion == permissionVersion &&
			projection.UpdatedByEventID == eventID {
			return projection, nil
		}
		select {
		case <-ctx.Done():
			return memberProjection{}, fmt.Errorf("timed out waiting for projection role=%s status=%s member_version=%d permission_version=%d event=%s", role, memberStatus, memberVersion, permissionVersion, eventID)
		case <-ticker.C:
		}
	}
}

func readProjection(ctx context.Context, pool *pgxpool.Pool, cfg config) (memberProjection, bool, error) {
	projection := memberProjection{ConversationID: cfg.conversationID, UserID: cfg.userID}
	err := pool.QueryRow(ctx, `
SELECT role, status, member_version, permission_version, updated_by_event_id
FROM policy_conversation_members_projection
WHERE tenant_id = $1
  AND conversation_id = $2
  AND user_id = $3
`, cfg.tenantID, cfg.conversationID, cfg.userID).Scan(
		&projection.Role,
		&projection.Status,
		&projection.MemberVersion,
		&projection.PermissionVersion,
		&projection.UpdatedByEventID,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return memberProjection{}, false, nil
	}
	if err != nil {
		return memberProjection{}, false, fmt.Errorf("read conversation member projection: %w", err)
	}
	return projection, true, nil
}

func checkPolicy(ctx context.Context, client policyv1.PolicyServiceClient, cfg config, conversationPermissionVersion int64) (policyDecisionSnapshot, error) {
	response, err := client.CheckMessageAction(ctx, &policyv1.CheckMessageActionRequest{
		AuthContext: &policyv1.AuthContext{
			TenantId:  cfg.tenantID,
			UserId:    cfg.userID,
			DeviceId:  "policy-role-smoke-device",
			SessionId: "policy-role-smoke-session",
			TraceId:   "trace-policy-role-smoke",
			RequestId: fmt.Sprintf("policy-role-smoke-%d", conversationPermissionVersion),
		},
		ConversationId:                cfg.conversationID,
		Action:                        policyv1.MessageAction_MESSAGE_ACTION_SEND,
		ConversationPermissionVersion: conversationPermissionVersion,
	})
	decision := policyDecisionSnapshot{
		Checked:                       true,
		ConversationPermissionVersion: conversationPermissionVersion,
	}
	if err != nil {
		decision.ErrorCode = status.Code(err).String()
		decision.ErrorMessage = status.Convert(err).Message()
		return decision, nil
	}
	decision.Allowed = response.GetAllowed()
	decision.PermissionVersion = response.GetPermissionVersion()
	decision.Classification = response.GetClassification()
	decision.Reason = response.GetReason()
	return decision, nil
}

func assertAllowedDecision(decision policyDecisionSnapshot) error {
	if decision.ErrorCode != "" {
		return fmt.Errorf("expected allowed decision, got error %+v", decision)
	}
	if !decision.Allowed ||
		decision.PermissionVersion != tenantAllowPermissionVersion ||
		decision.Classification != tenantAllowClassification {
		return fmt.Errorf("unexpected allowed decision: %+v", decision)
	}
	return nil
}

func assertDeniedDecision(decision policyDecisionSnapshot, expectedPermissionVersion int64) error {
	if decision.ErrorCode != "" {
		return fmt.Errorf("expected role denied decision, got error %+v", decision)
	}
	if decision.Allowed ||
		decision.PermissionVersion != expectedPermissionVersion ||
		decision.Classification != roleDenyClassification ||
		decision.Reason != roleDenyReason {
		return fmt.Errorf("unexpected role denied decision: %+v", decision)
	}
	return nil
}

func waitCheckpoint(ctx context.Context, pool *pgxpool.Pool, cfg config) (int64, error) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		var offset int64
		err := pool.QueryRow(ctx, `
SELECT COALESCE(MAX(offset_value), 0)
FROM policy_kafka_checkpoints
WHERE consumer_group = $1
  AND topic = $2
`, cfg.consumerGroup, cfg.topic).Scan(&offset)
		if err != nil {
			return 0, fmt.Errorf("read checkpoint: %w", err)
		}
		if offset >= 3 {
			return offset, nil
		}
		select {
		case <-ctx.Done():
			return 0, fmt.Errorf("timed out waiting for policy timeline checkpoint")
		case <-ticker.C:
		}
	}
}

func cleanupTenant(ctx context.Context, pool *pgxpool.Pool, cfg config) error {
	if _, err := pool.Exec(ctx, `DELETE FROM policy_decision_audit_outbox_repair_audit WHERE tenant_id = $1`, cfg.tenantID); err != nil {
		return fmt.Errorf("cleanup policy decision audit repair audit: %w", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM policy_decision_audit_outbox WHERE tenant_id = $1`, cfg.tenantID); err != nil {
		return fmt.Errorf("cleanup policy decision audit outbox: %w", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM policy_conversation_role_action_rules WHERE tenant_id = $1`, cfg.tenantID); err != nil {
		return fmt.Errorf("cleanup role rules: %w", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM policy_tenant_message_action_rules WHERE tenant_id = $1`, cfg.tenantID); err != nil {
		return fmt.Errorf("cleanup tenant rules: %w", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM policy_conversation_members_projection WHERE tenant_id = $1`, cfg.tenantID); err != nil {
		return fmt.Errorf("cleanup conversation members projection: %w", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM policy_kafka_checkpoints WHERE consumer_group = $1 AND topic = $2`, cfg.consumerGroup, cfg.topic); err != nil {
		return fmt.Errorf("cleanup policy kafka checkpoint: %w", err)
	}
	return nil
}

func seedPolicyRules(ctx context.Context, pool *pgxpool.Pool, cfg config) error {
	if _, err := pool.Exec(ctx, `
INSERT INTO policy_conversation_role_action_rules (
    tenant_id,
    action,
    min_role,
    classification,
    reason,
    source,
    updated_at
) VALUES ($1, 'SEND', $2, $3, $4, 'policy-role-smoke', now())
ON CONFLICT (tenant_id, action) DO UPDATE
SET min_role = EXCLUDED.min_role,
    classification = EXCLUDED.classification,
    reason = EXCLUDED.reason,
    source = EXCLUDED.source,
    updated_at = now()
`, cfg.tenantID, roleAdmin, roleDenyClassification, roleDenyReason); err != nil {
		return fmt.Errorf("seed role rule: %w", err)
	}
	if _, err := pool.Exec(ctx, `
INSERT INTO policy_tenant_message_action_rules (
    tenant_id,
    action,
    allowed,
    permission_version,
    classification,
    reason,
    source,
    updated_at
) VALUES ($1, 'SEND', true, $2, $3, '', 'policy-role-smoke', now())
ON CONFLICT (tenant_id, action) DO UPDATE
SET allowed = EXCLUDED.allowed,
    permission_version = EXCLUDED.permission_version,
    classification = EXCLUDED.classification,
    reason = EXCLUDED.reason,
    source = EXCLUDED.source,
    updated_at = now()
`, cfg.tenantID, tenantAllowPermissionVersion, tenantAllowClassification); err != nil {
		return fmt.Errorf("seed tenant allow rule: %w", err)
	}
	return nil
}

func ensureTopic(ctx context.Context, brokers []string, topic string) error {
	if len(brokers) == 0 {
		return fmt.Errorf("kafka brokers are required")
	}
	conn, err := kafkago.DialContext(ctx, "tcp", brokers[0])
	if err != nil {
		return err
	}
	defer conn.Close()
	controller, err := conn.Controller()
	if err != nil {
		return err
	}
	controllerAddr := controller.Host + ":" + strconv.Itoa(controller.Port)
	controllerConn, err := kafkago.DialContext(ctx, "tcp", controllerAddr)
	if err != nil {
		return err
	}
	defer controllerConn.Close()
	err = controllerConn.CreateTopics(kafkago.TopicConfig{
		Topic:             topic,
		NumPartitions:     1,
		ReplicationFactor: 1,
	})
	if err != nil && !strings.Contains(strings.ToLower(err.Error()), "already") {
		return err
	}
	return nil
}

func writeSummary(resultDir string, s summary) error {
	path := filepath.Join(resultDir, "policy-role-summary.json")
	bytes, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(bytes, '\n'), 0o644)
}

func gitOutput(args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", args...).CombinedOutput()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func randomSuffix() string {
	var bytes [4]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(bytes[:])
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			values = append(values, trimmed)
		}
	}
	return values
}
