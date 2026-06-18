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

	"github.com/jackc/pgx/v5/pgxpool"
	memoryv1 "github.com/qsyy0921/IM/api/proto/nexusim/memory/v1"
	"github.com/qsyy0921/IM/loadtest/internal/grpctls"
	conversationtimelinev1 "github.com/qsyy0921/IM/schemas/kafka"
	kafkago "github.com/segmentio/kafka-go"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	defaultResultRoot = `H:\NexusIM\loadtest-results`
	defaultTopic      = "conversation.timeline.events"

	eventTypeMessagePersisted = "message.persisted.v1"
	eventTypeMessageRevoked   = "message.revoked.v1"
	eventTypeMemberJoined     = "conversation.member.joined.v1"
)

type config struct {
	memoryTarget    string
	memoryTLS       grpctls.Config
	kafkaBrokers    []string
	topic           string
	consumerGroup   string
	resultDir       string
	pgDSN           string
	requestTimeout  time.Duration
	waitTimeout     time.Duration
	pollInterval    time.Duration
	tenantID        string
	conversationID  string
	viewerUserID    string
	senderUserID    string
	viewerDeviceID  string
	ensureTopic     bool
	cleanup         bool
	autoTopic       bool
	replication     int
	topicPartitions int
}

type summary struct {
	Commit             string        `json:"commit"`
	CommitFull         string        `json:"commit_full"`
	GitDirty           bool          `json:"git_dirty"`
	GitStatusShort     string        `json:"git_status_short,omitempty"`
	ResultDir          string        `json:"result_dir"`
	MemoryTarget       string        `json:"memory_target"`
	MemoryTLSEnabled   bool          `json:"memory_tls_enabled"`
	TenantID           string        `json:"tenant_id"`
	ConversationID     string        `json:"conversation_id"`
	ViewerUserID       string        `json:"viewer_user_id"`
	SenderUserID       string        `json:"sender_user_id"`
	Topic              string        `json:"topic"`
	ConsumerGroup      string        `json:"consumer_group"`
	KafkaBrokers       []string      `json:"kafka_brokers"`
	StartedAt          time.Time     `json:"started_at"`
	FinishedAt         time.Time     `json:"finished_at"`
	Success            bool          `json:"success"`
	Error              string        `json:"error,omitempty"`
	Events             []eventRecord `json:"events"`
	Checks             checkRecord   `json:"checks"`
	CheckpointOffset   int64         `json:"checkpoint_offset_value"`
	MemoryEventCount   int64         `json:"memory_event_count"`
	MembershipStatus   string        `json:"membership_status"`
	MembershipJoinSeq  int64         `json:"membership_join_seq"`
	MembershipLeaveSeq *int64        `json:"membership_leave_seq,omitempty"`
	ProjectionVersion  int64         `json:"projection_version"`
	ProjectionNote     string        `json:"projection_smoke_note"`
}

type eventRecord struct {
	EventID         string `json:"event_id"`
	EventType       string `json:"event_type"`
	ConversationSeq int64  `json:"conversation_seq"`
	MessageID       string `json:"message_id,omitempty"`
}

type checkRecord struct {
	MemoryProjected       bool `json:"memory_projected"`
	SourceRefProjected    bool `json:"source_ref_projected"`
	GetMemoryEventWorks   bool `json:"get_memory_event_works"`
	StrangerHidden        bool `json:"stranger_hidden"`
	RevokedMemoryHidden   bool `json:"revoked_memory_hidden"`
	ProfileAggregatesNone bool `json:"profile_aggregates_none"`
}

type membershipRecord struct {
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
	var brokers string
	var resultRoot string
	var runName string
	flag.StringVar(&cfg.memoryTarget, "memory-target", envOr("NEXUSIM_MEMORY_GRPC_ADDR", "127.0.0.1:10580"), "memory-service gRPC target")
	registerTLSFlags("memory-tls", "NEXUSIM_MEMORY_TLS", "memory-service", &cfg.memoryTLS)
	flag.StringVar(&brokers, "kafka-brokers", envOr("NEXUSIM_KAFKA_BROKERS", "localhost:9092"), "comma-separated Kafka brokers")
	flag.StringVar(&cfg.topic, "topic", envOr("NEXUSIM_TIMELINE_TOPIC", defaultTopic), "conversation timeline Kafka topic")
	flag.StringVar(&cfg.consumerGroup, "consumer-group", envOr("NEXUSIM_MEMORY_CONSUMER_GROUP", "nexusim-memory-service"), "memory timeline consumer group")
	flag.StringVar(&resultRoot, "result-root", defaultResultRoot, "external result root for raw smoke output")
	flag.StringVar(&runName, "run-name", "", "run name under result-root")
	flag.StringVar(&cfg.pgDSN, "pg-dsn", envOr("NEXUSIM_PG_DSN", "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable"), "PostgreSQL DSN")
	flag.DurationVar(&cfg.requestTimeout, "request-timeout", 3*time.Second, "per request timeout")
	flag.DurationVar(&cfg.waitTimeout, "wait-timeout", 20*time.Second, "max wait for async projection")
	flag.DurationVar(&cfg.pollInterval, "poll-interval", 200*time.Millisecond, "poll interval while waiting")
	flag.StringVar(&cfg.tenantID, "tenant-id", "", "tenant id; defaults to tenant derived from run name")
	flag.StringVar(&cfg.conversationID, "conversation-id", "", "conversation id; defaults to conversation derived from run name")
	flag.StringVar(&cfg.viewerUserID, "viewer-user-id", "memory-viewer-1", "memory viewer user id")
	flag.StringVar(&cfg.senderUserID, "sender-user-id", "memory-sender-1", "message sender user id")
	flag.StringVar(&cfg.viewerDeviceID, "viewer-device-id", "memory-device-1", "viewer device id")
	flag.BoolVar(&cfg.ensureTopic, "ensure-topic", true, "create Kafka topic if needed before publishing")
	flag.BoolVar(&cfg.cleanup, "cleanup", true, "delete existing memory-service rows for tenant/group before running")
	flag.BoolVar(&cfg.autoTopic, "allow-auto-topic-creation", false, "allow kafka-go writer auto topic creation")
	flag.IntVar(&cfg.replication, "topic-replication-factor", 1, "Kafka topic replication factor when ensuring topic")
	flag.IntVar(&cfg.topicPartitions, "topic-partitions", 1, "Kafka topic partitions when ensuring topic")
	flag.Parse()

	cfg.kafkaBrokers = splitCSV(brokers)
	if runName == "" {
		runName = "memory-service-projection-smoke-" + time.Now().Format("20060102-150405")
	}
	if cfg.tenantID == "" {
		cfg.tenantID = "tenant-" + sanitizeRunName(runName)
	}
	if cfg.conversationID == "" {
		cfg.conversationID = "conv-" + sanitizeRunName(runName)
	}
	cfg.resultDir = filepath.Join(resultRoot, runName)
	if cfg.requestTimeout <= 0 {
		cfg.requestTimeout = 3 * time.Second
	}
	if cfg.waitTimeout <= 0 {
		cfg.waitTimeout = 20 * time.Second
	}
	if cfg.pollInterval <= 0 {
		cfg.pollInterval = 200 * time.Millisecond
	}
	if cfg.replication <= 0 {
		cfg.replication = 1
	}
	if cfg.topicPartitions <= 0 {
		cfg.topicPartitions = 1
	}
	return cfg
}

func registerTLSFlags(prefix string, envPrefix string, serviceName string, config *grpctls.Config) {
	flag.StringVar(&config.CAFile, prefix+"-ca-file", os.Getenv(envPrefix+"_CA_FILE"), "CA PEM for "+serviceName+" gRPC TLS")
	flag.StringVar(&config.ServerName, prefix+"-server-name", os.Getenv(envPrefix+"_SERVER_NAME"), "override server name for "+serviceName+" gRPC TLS")
	flag.StringVar(&config.ClientCertFile, prefix+"-client-cert-file", os.Getenv(envPrefix+"_CLIENT_CERT_FILE"), "client certificate PEM for "+serviceName+" mTLS")
	flag.StringVar(&config.ClientKeyFile, prefix+"-client-key-file", os.Getenv(envPrefix+"_CLIENT_KEY_FILE"), "client private key PEM for "+serviceName+" mTLS")
}

func run(cfg config) error {
	if err := validateConfig(cfg); err != nil {
		return err
	}
	if err := validateExternalResultDir(cfg.resultDir); err != nil {
		return err
	}
	if err := os.MkdirAll(cfg.resultDir, 0o755); err != nil {
		return fmt.Errorf("create result dir: %w", err)
	}

	result := summary{
		Commit:           gitOutput("rev-parse", "--short", "HEAD"),
		CommitFull:       gitOutput("rev-parse", "HEAD"),
		GitStatusShort:   gitOutput("status", "--short"),
		ResultDir:        cfg.resultDir,
		MemoryTarget:     cfg.memoryTarget,
		MemoryTLSEnabled: cfg.memoryTLS.Enabled(),
		TenantID:         cfg.tenantID,
		ConversationID:   cfg.conversationID,
		ViewerUserID:     cfg.viewerUserID,
		SenderUserID:     cfg.senderUserID,
		Topic:            cfg.topic,
		ConsumerGroup:    cfg.consumerGroup,
		KafkaBrokers:     cfg.kafkaBrokers,
		StartedAt:        time.Now().UTC(),
		ProjectionNote:   "Validates memory projection, source refs and visibility only; no RAG/Agent behavior is claimed.",
	}
	result.GitDirty = strings.TrimSpace(result.GitStatusShort) != ""
	defer func() {
		result.FinishedAt = time.Now().UTC()
		_ = writeSummary(cfg.resultDir, result)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), cfg.waitTimeout+30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, cfg.pgDSN)
	if err != nil {
		result.Error = "open postgres: " + err.Error()
		return fmt.Errorf("open postgres: %w", err)
	}
	defer pool.Close()
	if err := applyMemoryMigration(ctx, pool); err != nil {
		result.Error = err.Error()
		return err
	}
	if cfg.cleanup {
		if err := cleanupMemoryRows(ctx, pool, cfg); err != nil {
			result.Error = err.Error()
			return err
		}
	}
	if cfg.ensureTopic {
		if err := ensureTopic(ctx, cfg); err != nil {
			result.Error = "ensure topic: " + err.Error()
			return fmt.Errorf("ensure topic: %w", err)
		}
	}

	dialOption, err := grpctls.DialOption(cfg.memoryTLS, "memory-tls")
	if err != nil {
		result.Error = "configure memory TLS: " + err.Error()
		return err
	}
	conn, err := grpc.NewClient("passthrough:///"+cfg.memoryTarget, dialOption)
	if err != nil {
		result.Error = "dial memory-service: " + err.Error()
		return fmt.Errorf("dial memory-service: %w", err)
	}
	defer conn.Close()
	client := memoryv1.NewMemoryServiceClient(conn)

	writer := &kafkago.Writer{
		Addr:                   kafkago.TCP(cfg.kafkaBrokers...),
		Topic:                  cfg.topic,
		Balancer:               &kafkago.Hash{},
		RequiredAcks:           kafkago.RequireAll,
		AllowAutoTopicCreation: cfg.autoTopic,
		WriteTimeout:           cfg.requestTimeout,
		ReadTimeout:            cfg.requestTimeout,
	}
	defer writer.Close()

	runID := randomSuffix()
	if err := publishAndVerify(ctx, cfg, pool, client, writer, runID, &result); err != nil {
		result.Error = err.Error()
		return err
	}

	membership, err := readMembership(ctx, pool, cfg, cfg.viewerUserID)
	if err != nil {
		result.Error = err.Error()
		return err
	}
	result.MembershipStatus = membership.Status
	result.MembershipJoinSeq = membership.JoinSeq
	result.MembershipLeaveSeq = membership.LeaveSeq
	result.MemoryEventCount, _ = countMemoryEvents(ctx, pool, cfg)
	result.CheckpointOffset, _ = checkpointOffset(ctx, pool, cfg)
	result.Success = true
	return nil
}

func validateConfig(cfg config) error {
	if len(cfg.kafkaBrokers) == 0 {
		return errors.New("kafka-brokers is required")
	}
	if strings.TrimSpace(cfg.topic) == "" {
		return errors.New("topic is required")
	}
	if strings.TrimSpace(cfg.consumerGroup) == "" {
		return errors.New("consumer-group is required")
	}
	if strings.TrimSpace(cfg.pgDSN) == "" {
		return errors.New("pg-dsn is required")
	}
	if strings.TrimSpace(cfg.memoryTarget) == "" {
		return errors.New("memory-target is required")
	}
	return nil
}

func publishAndVerify(
	ctx context.Context,
	cfg config,
	pool *pgxpool.Pool,
	client memoryv1.MemoryServiceClient,
	writer *kafkago.Writer,
	runID string,
	result *summary,
) error {
	messageID := "msg-memory-" + runID
	memoryEventID := "evt-memory-persisted-" + runID
	query := "retrieval evidence"

	joined := memberJoinedEvent("evt-memory-joined-"+runID, cfg, 1, 1)
	if err := publishEvent(ctx, cfg, writer, joined); err != nil {
		return fmt.Errorf("publish member joined: %w", err)
	}
	result.Events = append(result.Events, snapshot(joined, ""))
	if err := waitMembership(ctx, pool, cfg, cfg.viewerUserID, "ACTIVE"); err != nil {
		return err
	}

	persisted := messagePersistedEvent(memoryEventID, cfg, 2, messageID, "decision: memory projection anchors retrieval evidence pack")
	if err := publishEvent(ctx, cfg, writer, persisted); err != nil {
		return fmt.Errorf("publish memory candidate: %w", err)
	}
	result.Events = append(result.Events, snapshot(persisted, messageID))
	response, err := waitMemoryQuery(ctx, cfg, client, cfg.viewerUserID, query, memoryEventID, 1)
	if err != nil {
		return err
	}
	result.ProjectionVersion = response.GetProjectionVersion()
	result.Checks.MemoryProjected = true
	if len(response.GetItems()) != 1 || len(response.GetItems()[0].GetSourceRefs()) != 1 || response.GetItems()[0].GetSourceRefs()[0].GetSourceId() != messageID {
		return fmt.Errorf("expected one source ref to %s: %+v", messageID, response.GetItems())
	}
	result.Checks.SourceRefProjected = true
	if _, err := getMemoryEvent(ctx, cfg, client, cfg.viewerUserID, memoryEventID); err != nil {
		return fmt.Errorf("get memory event: %w", err)
	}
	result.Checks.GetMemoryEventWorks = true
	if err := waitMemoryQueryCount(ctx, cfg, client, "memory-stranger-"+runID, query, 0); err != nil {
		return err
	}
	result.Checks.StrangerHidden = true
	profiles, err := listProfileAggregates(ctx, cfg, client, cfg.viewerUserID)
	if err != nil {
		return err
	}
	if len(profiles.GetItems()) != 0 {
		return fmt.Errorf("profile aggregates should stay empty in first-stage rules projection: %+v", profiles.GetItems())
	}
	result.Checks.ProfileAggregatesNone = true

	revoked := messageRevokedEvent("evt-memory-revoked-"+runID, cfg, 3, messageID)
	if err := publishEvent(ctx, cfg, writer, revoked); err != nil {
		return fmt.Errorf("publish memory revoke: %w", err)
	}
	result.Events = append(result.Events, snapshot(revoked, messageID))
	if err := waitMemoryQueryCount(ctx, cfg, client, cfg.viewerUserID, query, 0); err != nil {
		return err
	}
	result.Checks.RevokedMemoryHidden = true
	return nil
}

func publishEvent(ctx context.Context, cfg config, writer *kafkago.Writer, event *conversationtimelinev1.ConversationTimelineEvent) error {
	encoded, err := proto.Marshal(event)
	if err != nil {
		return err
	}
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	return writer.WriteMessages(requestCtx, kafkago.Message{
		Key:   []byte(event.GetPartitionKey()),
		Value: encoded,
		Headers: []kafkago.Header{
			{Key: "event_id", Value: []byte(event.GetEventId())},
			{Key: "event_type", Value: []byte(event.GetEventType())},
		},
	})
}

func memberJoinedEvent(eventID string, cfg config, seq int64, version int64) *conversationtimelinev1.ConversationTimelineEvent {
	return baseTimelineEvent(eventID, eventTypeMemberJoined, cfg, seq, version, &conversationtimelinev1.ConversationTimelineEvent_ConversationMemberJoined{
		ConversationMemberJoined: &conversationtimelinev1.ConversationMemberJoinedV1{
			ChangeId:          "change-" + eventID,
			ConversationId:    cfg.conversationID,
			BoundarySeq:       seq,
			TargetUserId:      cfg.viewerUserID,
			OperatorUserId:    cfg.senderUserID,
			ChangeType:        conversationtimelinev1.ConversationMemberChangeType_CONVERSATION_MEMBER_CHANGE_TYPE_JOIN,
			NewRole:           conversationtimelinev1.ConversationMemberRole_CONVERSATION_MEMBER_ROLE_MEMBER,
			NewStatus:         conversationtimelinev1.ConversationMemberStatus_CONVERSATION_MEMBER_STATUS_ACTIVE,
			MemberVersion:     version,
			PermissionVersion: version,
			Reason:            "memory projection smoke join",
			OccurredAt:        timestamppb.Now(),
		},
	})
}

func messagePersistedEvent(eventID string, cfg config, seq int64, messageID string, text string) *conversationtimelinev1.ConversationTimelineEvent {
	payload, _ := structpb.NewStruct(map[string]any{"memory_fact": text})
	return baseTimelineEvent(eventID, eventTypeMessagePersisted, cfg, seq, seq, &conversationtimelinev1.ConversationTimelineEvent_MessagePersisted{
		MessagePersisted: &conversationtimelinev1.MessagePersistedV1{
			MessageId:       messageID,
			ConversationId:  cfg.conversationID,
			ConversationSeq: seq,
			SenderId:        cfg.senderUserID,
			DeviceId:        "memory-smoke-device",
			ClientMsgId:     "client-" + eventID,
			CommandHash:     eventID,
			MessageType:     "TEXT",
			Payload:         payload,
			AcceptedAt:      timestamppb.Now(),
		},
	})
}

func messageRevokedEvent(eventID string, cfg config, seq int64, messageID string) *conversationtimelinev1.ConversationTimelineEvent {
	return baseTimelineEvent(eventID, eventTypeMessageRevoked, cfg, seq, seq+10, &conversationtimelinev1.ConversationTimelineEvent_MessageRevoked{
		MessageRevoked: &conversationtimelinev1.MessageRevokedV1{
			MessageId:       messageID,
			ConversationId:  cfg.conversationID,
			ConversationSeq: seq,
			RevokedBy:       cfg.senderUserID,
			RevokedAt:       timestamppb.Now(),
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
		MappingVersion:   eventType,
		TraceId:          "trace-memory-projection-smoke",
		CorrelationId:    "memory-projection-smoke",
		CausationId:      eventID,
		Producer:         "memory-projection-smoke",
		OccurredAt:       timestamppb.Now(),
		Metadata: &conversationtimelinev1.TimelineMetadata{
			FanoutMode:        "WRITE_FANOUT",
			PermissionVersion: permissionVersion,
			Classification:    "MEMORY_PROJECTION_SMOKE",
			MappingVersion:    eventType,
		},
	}
	switch typed := payload.(type) {
	case *conversationtimelinev1.ConversationTimelineEvent_ConversationMemberJoined:
		event.Payload = typed
	case *conversationtimelinev1.ConversationTimelineEvent_MessagePersisted:
		event.Payload = typed
	case *conversationtimelinev1.ConversationTimelineEvent_MessageRevoked:
		event.Payload = typed
	}
	return event
}

func waitMemoryQuery(
	ctx context.Context,
	cfg config,
	client memoryv1.MemoryServiceClient,
	userID string,
	query string,
	memoryEventID string,
	wantCount int,
) (*memoryv1.QueryMemoryEventsResponse, error) {
	var lastCount int
	var lastIDs []string
	err := waitUntil(ctx, cfg, func() (bool, error) {
		response, err := queryMemory(ctx, cfg, client, userID, query)
		if err != nil {
			return false, nil
		}
		lastCount = len(response.GetItems())
		lastIDs = lastIDs[:0]
		matchedID := memoryEventID == ""
		for _, item := range response.GetItems() {
			lastIDs = append(lastIDs, item.GetMemoryEventId())
			if item.GetMemoryEventId() == memoryEventID {
				matchedID = true
			}
		}
		return lastCount == wantCount && matchedID, nil
	})
	if err != nil {
		return nil, fmt.Errorf("wait memory query=%q count=%d memory_event_id=%s last_count=%d last_ids=%v: %w", query, wantCount, memoryEventID, lastCount, lastIDs, err)
	}
	return queryMemory(ctx, cfg, client, userID, query)
}

func waitMemoryQueryCount(ctx context.Context, cfg config, client memoryv1.MemoryServiceClient, userID string, query string, wantCount int) error {
	_, err := waitMemoryQuery(ctx, cfg, client, userID, query, "", wantCount)
	return err
}

func queryMemory(ctx context.Context, cfg config, client memoryv1.MemoryServiceClient, userID string, query string) (*memoryv1.QueryMemoryEventsResponse, error) {
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	return client.QueryMemoryEvents(requestCtx, &memoryv1.QueryMemoryEventsRequest{
		AuthContext:    auth(cfg, userID),
		ConversationId: cfg.conversationID,
		Query:          query,
		Statuses:       []memoryv1.MemoryEventStatus{memoryv1.MemoryEventStatus_MEMORY_EVENT_STATUS_PENDING},
		Limit:          20,
	})
}

func getMemoryEvent(ctx context.Context, cfg config, client memoryv1.MemoryServiceClient, userID string, memoryEventID string) (*memoryv1.GetMemoryEventResponse, error) {
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	return client.GetMemoryEvent(requestCtx, &memoryv1.GetMemoryEventRequest{
		AuthContext:   auth(cfg, userID),
		MemoryEventId: memoryEventID,
	})
}

func listProfileAggregates(ctx context.Context, cfg config, client memoryv1.MemoryServiceClient, userID string) (*memoryv1.ListProfileAggregatesResponse, error) {
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	return client.ListProfileAggregates(requestCtx, &memoryv1.ListProfileAggregatesRequest{
		AuthContext:   auth(cfg, userID),
		SubjectUserId: userID,
		Statuses:      []memoryv1.MemoryEventStatus{memoryv1.MemoryEventStatus_MEMORY_EVENT_STATUS_PENDING},
		Limit:         20,
	})
}

func auth(cfg config, userID string) *memoryv1.AuthContext {
	return &memoryv1.AuthContext{
		TenantId:  cfg.tenantID,
		UserId:    userID,
		DeviceId:  cfg.viewerDeviceID,
		SessionId: "memory-projection-smoke",
		TraceId:   "trace-memory-projection-smoke",
		RequestId: "request-memory-projection-smoke",
	}
}

func waitMembership(ctx context.Context, pool *pgxpool.Pool, cfg config, userID string, wantStatus string) error {
	return waitUntil(ctx, cfg, func() (bool, error) {
		membership, err := readMembership(ctx, pool, cfg, userID)
		if err != nil {
			return false, nil
		}
		return membership.Status == wantStatus, nil
	})
}

func waitUntil(ctx context.Context, cfg config, probe func() (bool, error)) error {
	deadline := time.Now().Add(cfg.waitTimeout)
	for {
		ok, err := probe()
		if err != nil {
			return err
		}
		if ok {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out after %s", cfg.waitTimeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(cfg.pollInterval):
		}
	}
}

func applyMemoryMigration(ctx context.Context, pool *pgxpool.Pool) error {
	migrationPath := filepath.Join(gitOutput("rev-parse", "--show-toplevel"), "migrations", "postgres", "memory", "000001_memory_core.sql")
	migration, err := os.ReadFile(migrationPath)
	if err != nil {
		return fmt.Errorf("read memory migration: %w", err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err != nil {
		return fmt.Errorf("apply memory migration: %w", err)
	}
	return nil
}

func cleanupMemoryRows(ctx context.Context, pool *pgxpool.Pool, cfg config) error {
	statements := []string{
		`DELETE FROM memory_graph_edges WHERE tenant_id = $1`,
		`DELETE FROM memory_event_source_refs WHERE tenant_id = $1`,
		`DELETE FROM memory_profile_aggregates WHERE tenant_id = $1`,
		`DELETE FROM memory_membership_projection WHERE tenant_id = $1`,
		`DELETE FROM memory_structured_events WHERE tenant_id = $1`,
	}
	for _, statement := range statements {
		if _, err := pool.Exec(ctx, statement, cfg.tenantID); err != nil {
			return fmt.Errorf("cleanup memory rows: %w", err)
		}
	}
	if _, err := pool.Exec(ctx, `DELETE FROM memory_projection_checkpoints WHERE consumer_group = $1 AND topic = $2`, cfg.consumerGroup, cfg.topic); err != nil {
		return fmt.Errorf("cleanup memory checkpoints: %w", err)
	}
	return nil
}

func readMembership(ctx context.Context, pool *pgxpool.Pool, cfg config, userID string) (membershipRecord, error) {
	var membership membershipRecord
	err := pool.QueryRow(ctx, `
SELECT status, join_seq, leave_seq
FROM memory_membership_projection
WHERE tenant_id = $1
  AND conversation_id = $2
  AND user_id = $3
`, cfg.tenantID, cfg.conversationID, userID).Scan(&membership.Status, &membership.JoinSeq, &membership.LeaveSeq)
	return membership, err
}

func countMemoryEvents(ctx context.Context, pool *pgxpool.Pool, cfg config) (int64, error) {
	var count int64
	err := pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM memory_structured_events
WHERE tenant_id = $1
  AND conversation_id = $2
`, cfg.tenantID, cfg.conversationID).Scan(&count)
	return count, err
}

func checkpointOffset(ctx context.Context, pool *pgxpool.Pool, cfg config) (int64, error) {
	var offset int64
	err := pool.QueryRow(ctx, `
SELECT COALESCE(MAX(offset_value), 0)
FROM memory_projection_checkpoints
WHERE consumer_group = $1
  AND topic = $2
`, cfg.consumerGroup, cfg.topic).Scan(&offset)
	return offset, err
}

func ensureTopic(ctx context.Context, cfg config) error {
	conn, err := kafkago.DialContext(ctx, "tcp", cfg.kafkaBrokers[0])
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
		Topic:             cfg.topic,
		NumPartitions:     cfg.topicPartitions,
		ReplicationFactor: cfg.replication,
	})
	if err != nil && !strings.Contains(strings.ToLower(err.Error()), "already") {
		return err
	}
	return nil
}

func snapshot(event *conversationtimelinev1.ConversationTimelineEvent, messageID string) eventRecord {
	return eventRecord{
		EventID:         event.GetEventId(),
		EventType:       event.GetEventType(),
		ConversationSeq: event.GetAggregateVersion(),
		MessageID:       messageID,
	}
}

func writeSummary(resultDir string, result summary) error {
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(resultDir, "memory-projection-summary.json")
	if err := os.WriteFile(path, append(encoded, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Println(string(encoded))
	fmt.Printf("summary: %s\n", path)
	return nil
}

func validateExternalResultDir(resultDir string) error {
	repo := gitOutput("rev-parse", "--show-toplevel")
	if repo == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		repo = cwd
	}
	resultFull, err := filepath.Abs(resultDir)
	if err != nil {
		return err
	}
	repoFull, err := filepath.Abs(repo)
	if err != nil {
		return err
	}
	if pathInside(resultFull, repoFull) {
		return fmt.Errorf("result-dir must not be inside repository; use %s or another external scratch directory", defaultResultRoot)
	}
	return nil
}

func pathInside(path string, root string) bool {
	path = strings.TrimRight(filepath.Clean(path), `\/`)
	root = strings.TrimRight(filepath.Clean(root), `\/`)
	if strings.EqualFold(path, root) {
		return true
	}
	prefix := root + string(os.PathSeparator)
	return strings.HasPrefix(strings.ToLower(path), strings.ToLower(prefix))
}

func sanitizeRunName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "memory-smoke"
	}
	var builder strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '-' || r == '_':
			builder.WriteRune(r)
		default:
			builder.WriteByte('-')
		}
	}
	return strings.Trim(builder.String(), "-")
}

func randomSuffix() string {
	var data [4]byte
	if _, err := rand.Read(data[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(data[:])
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

func envOr(name string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
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
