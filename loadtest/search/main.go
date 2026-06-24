package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	searchv1 "github.com/qsyy0921/IM/api/proto/nexusim/search/v1"
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
	eventTypeMessageEdited    = "message.edited.v1"
	eventTypeMessageRevoked   = "message.revoked.v1"
	eventTypeMessageDeleted   = "message.deleted.v1"
	eventTypeMemberJoined     = "conversation.member.joined.v1"
	eventTypeMemberLeft       = "conversation.member.left.v1"
)

type config struct {
	phase           string
	searchTarget    string
	searchTLS       grpctls.Config
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

	searchBackend        string
	openSearchEndpoint   string
	openSearchIndex      string
	openSearchUsername   string
	openSearchPassword   string
	openSearchAPIKey     string
	openSearchHTTPClient *http.Client
}

type summary struct {
	Phase               string          `json:"phase"`
	Commit              string          `json:"commit"`
	CommitFull          string          `json:"commit_full"`
	GitDirty            bool            `json:"git_dirty"`
	GitStatusShort      string          `json:"git_status_short,omitempty"`
	ResultDir           string          `json:"result_dir"`
	SearchTarget        string          `json:"search_target"`
	SearchTLSEnabled    bool            `json:"search_tls_enabled"`
	SearchBackend       string          `json:"search_backend"`
	OpenSearchEndpoint  string          `json:"opensearch_endpoint,omitempty"`
	OpenSearchIndex     string          `json:"opensearch_index,omitempty"`
	OpenSearchReady     bool            `json:"opensearch_ready,omitempty"`
	RequestTimeoutMs    int64           `json:"request_timeout_ms"`
	TenantID            string          `json:"tenant_id"`
	ConversationID      string          `json:"conversation_id"`
	ViewerUserID        string          `json:"viewer_user_id"`
	SenderUserID        string          `json:"sender_user_id"`
	Topic               string          `json:"topic"`
	ConsumerGroup       string          `json:"consumer_group"`
	KafkaBrokers        []string        `json:"kafka_brokers"`
	StartedAt           time.Time       `json:"started_at"`
	FinishedAt          time.Time       `json:"finished_at"`
	Success             bool            `json:"success"`
	Error               string          `json:"error,omitempty"`
	Events              []eventSnapshot `json:"events"`
	Checks              checkSnapshot   `json:"checks"`
	CheckpointOffset    int64           `json:"checkpoint_offset_value"`
	DocumentCount       int64           `json:"document_count"`
	MembershipStatus    string          `json:"membership_status"`
	MembershipJoinSeq   int64           `json:"membership_join_seq"`
	MembershipLeaveSeq  *int64          `json:"membership_leave_seq,omitempty"`
	ProjectionVersion   int64           `json:"projection_version"`
	OpenSearchDocuments int             `json:"opensearch_fixture_document_count,omitempty"`
	ProjectionSmokeNote string          `json:"projection_smoke_note"`
}

type eventSnapshot struct {
	EventID         string `json:"event_id"`
	EventType       string `json:"event_type"`
	ConversationSeq int64  `json:"conversation_seq"`
	MessageID       string `json:"message_id,omitempty"`
}

type checkSnapshot struct {
	PersistedVisible        bool `json:"persisted_visible"`
	EditedVisible           bool `json:"edited_visible"`
	OriginalHiddenAfterEdit bool `json:"original_hidden_after_edit"`
	RevokedHidden           bool `json:"revoked_hidden"`
	DeletedHidden           bool `json:"deleted_hidden"`
	AfterLeaveHidden        bool `json:"after_leave_hidden"`
	StrangerHidden          bool `json:"stranger_hidden"`
}

type membershipSnapshot struct {
	Status   string
	JoinSeq  int64
	LeaveSeq *int64
}

type searchDocumentSnapshot struct {
	TenantID        string `json:"tenant_id"`
	ConversationID  string `json:"conversation_id"`
	MessageID       string `json:"message_id"`
	ConversationSeq int64  `json:"conversation_seq"`
	SearchableText  string `json:"searchable_text"`
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
	flag.StringVar(&cfg.phase, "phase", envOr("NEXUSIM_SEARCH_SMOKE_PHASE", "smoke"), "phase: smoke or preflight-opensearch")
	flag.StringVar(&cfg.searchTarget, "search-target", envOr("NEXUSIM_SEARCH_GRPC_ADDR", "127.0.0.1:10570"), "search-service gRPC target")
	registerTLSFlags("search-tls", "NEXUSIM_SEARCH_TLS", "search-service", &cfg.searchTLS)
	flag.StringVar(&brokers, "kafka-brokers", envOr("NEXUSIM_KAFKA_BROKERS", "localhost:9092"), "comma-separated Kafka brokers")
	flag.StringVar(&cfg.topic, "topic", envOr("NEXUSIM_TIMELINE_TOPIC", defaultTopic), "conversation timeline Kafka topic")
	flag.StringVar(&cfg.consumerGroup, "consumer-group", envOr("NEXUSIM_SEARCH_CONSUMER_GROUP", "nexusim-search-service"), "search timeline consumer group")
	flag.StringVar(&resultRoot, "result-root", defaultResultRoot, "external result root for raw smoke output")
	flag.StringVar(&runName, "run-name", "", "run name under result-root")
	flag.StringVar(&cfg.pgDSN, "pg-dsn", envOr("NEXUSIM_PG_DSN", "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable"), "PostgreSQL DSN")
	flag.DurationVar(&cfg.requestTimeout, "request-timeout", 3*time.Second, "per request timeout")
	flag.DurationVar(&cfg.waitTimeout, "wait-timeout", 20*time.Second, "max wait for async projection")
	flag.DurationVar(&cfg.pollInterval, "poll-interval", 200*time.Millisecond, "poll interval while waiting")
	flag.StringVar(&cfg.tenantID, "tenant-id", "", "tenant id; defaults to tenant derived from run name")
	flag.StringVar(&cfg.conversationID, "conversation-id", "", "conversation id; defaults to conversation derived from run name")
	flag.StringVar(&cfg.viewerUserID, "viewer-user-id", "search-viewer-1", "search viewer user id")
	flag.StringVar(&cfg.senderUserID, "sender-user-id", "search-sender-1", "message sender user id")
	flag.StringVar(&cfg.viewerDeviceID, "viewer-device-id", "search-device-1", "viewer device id")
	flag.BoolVar(&cfg.ensureTopic, "ensure-topic", true, "create Kafka topic if needed before publishing")
	flag.BoolVar(&cfg.cleanup, "cleanup", true, "delete existing search-service rows for tenant/group before running")
	flag.BoolVar(&cfg.autoTopic, "allow-auto-topic-creation", false, "allow kafka-go writer auto topic creation")
	flag.IntVar(&cfg.replication, "topic-replication-factor", 1, "Kafka topic replication factor when ensuring topic")
	flag.IntVar(&cfg.topicPartitions, "topic-partitions", 1, "Kafka topic partitions when ensuring topic")
	flag.StringVar(&cfg.searchBackend, "search-backend", envOr("NEXUSIM_SEARCH_BACKEND", "postgres"), "search backend expected from search-service: postgres or opensearch")
	flag.StringVar(&cfg.openSearchEndpoint, "opensearch-endpoint", envOr("NEXUSIM_SEARCH_OPENSEARCH_ENDPOINT", ""), "OpenSearch endpoint used only when search-backend=opensearch")
	flag.StringVar(&cfg.openSearchIndex, "opensearch-index", envOr("NEXUSIM_SEARCH_OPENSEARCH_INDEX", ""), "OpenSearch index used only when search-backend=opensearch")
	flag.StringVar(&cfg.openSearchUsername, "opensearch-username", os.Getenv("NEXUSIM_SEARCH_OPENSEARCH_USERNAME"), "OpenSearch basic auth username; not written to summary")
	flag.StringVar(&cfg.openSearchPassword, "opensearch-password", os.Getenv("NEXUSIM_SEARCH_OPENSEARCH_PASSWORD"), "OpenSearch basic auth password; not written to summary")
	flag.StringVar(&cfg.openSearchAPIKey, "opensearch-api-key", os.Getenv("NEXUSIM_SEARCH_OPENSEARCH_API_KEY"), "OpenSearch API key; not written to summary")
	flag.Parse()

	cfg.phase = strings.ToLower(strings.TrimSpace(cfg.phase))
	cfg.kafkaBrokers = splitCSV(brokers)
	if runName == "" {
		runName = "search-service-projection-smoke-" + time.Now().Format("20060102-150405")
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
	cfg.searchBackend = strings.ToLower(strings.TrimSpace(cfg.searchBackend))
	cfg.openSearchEndpoint = strings.TrimRight(strings.TrimSpace(cfg.openSearchEndpoint), "/")
	cfg.openSearchIndex = strings.TrimSpace(cfg.openSearchIndex)
	cfg.openSearchUsername = strings.TrimSpace(cfg.openSearchUsername)
	cfg.openSearchAPIKey = strings.TrimSpace(cfg.openSearchAPIKey)
	cfg.openSearchHTTPClient = &http.Client{Timeout: cfg.requestTimeout}
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
		Phase:               cfg.phase,
		Commit:              gitOutput("rev-parse", "--short", "HEAD"),
		CommitFull:          gitOutput("rev-parse", "HEAD"),
		GitStatusShort:      gitOutput("status", "--short"),
		ResultDir:           cfg.resultDir,
		SearchTarget:        cfg.searchTarget,
		SearchTLSEnabled:    cfg.searchTLS.Enabled(),
		SearchBackend:       cfg.searchBackend,
		OpenSearchEndpoint:  cfg.openSearchEndpoint,
		OpenSearchIndex:     cfg.openSearchIndex,
		RequestTimeoutMs:    cfg.requestTimeout.Milliseconds(),
		TenantID:            cfg.tenantID,
		ConversationID:      cfg.conversationID,
		ViewerUserID:        cfg.viewerUserID,
		SenderUserID:        cfg.senderUserID,
		Topic:               cfg.topic,
		ConsumerGroup:       cfg.consumerGroup,
		KafkaBrokers:        cfg.kafkaBrokers,
		StartedAt:           time.Now().UTC(),
		ProjectionSmokeNote: "Validates search projection and visibility only; no LLM/RAG behavior is claimed.",
	}
	result.GitDirty = strings.TrimSpace(result.GitStatusShort) != ""
	defer func() {
		result.FinishedAt = time.Now().UTC()
		_ = writeSummary(cfg.resultDir, result)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), cfg.waitTimeout+30*time.Second)
	defer cancel()

	if cfg.phase == "preflight-opensearch" {
		if err := preflightOpenSearch(ctx, cfg, &result); err != nil {
			result.Error = err.Error()
			return err
		}
		result.Success = true
		return nil
	}

	pool, err := pgxpool.New(ctx, cfg.pgDSN)
	if err != nil {
		result.Error = "open postgres: " + err.Error()
		return fmt.Errorf("open postgres: %w", err)
	}
	defer pool.Close()
	if cfg.cleanup {
		if err := cleanupSearchRows(ctx, pool, cfg); err != nil {
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

	dialOption, err := grpctls.DialOption(cfg.searchTLS, "search-tls")
	if err != nil {
		result.Error = "configure search TLS: " + err.Error()
		return err
	}
	conn, err := grpc.NewClient("passthrough:///"+cfg.searchTarget, dialOption)
	if err != nil {
		result.Error = "dial search-service: " + err.Error()
		return fmt.Errorf("dial search-service: %w", err)
	}
	defer conn.Close()
	client := searchv1.NewSearchServiceClient(conn)

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
	result.DocumentCount, _ = countDocuments(ctx, pool, cfg)
	result.CheckpointOffset, _ = checkpointOffset(ctx, pool, cfg)
	result.Success = true
	return nil
}

func validateConfig(cfg config) error {
	switch cfg.phase {
	case "smoke":
	case "preflight-opensearch":
		if cfg.searchBackend != "opensearch" {
			return errors.New("search-backend must be opensearch for preflight-opensearch")
		}
		return validateOpenSearchConfig(cfg)
	default:
		return fmt.Errorf("unsupported phase %q", cfg.phase)
	}
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
	if strings.TrimSpace(cfg.searchTarget) == "" {
		return errors.New("search-target is required")
	}
	switch cfg.searchBackend {
	case "postgres":
	case "opensearch":
		return validateOpenSearchConfig(cfg)
	default:
		return fmt.Errorf("unsupported search-backend %q", cfg.searchBackend)
	}
	return nil
}

func validateOpenSearchConfig(cfg config) error {
	if cfg.openSearchEndpoint == "" {
		return errors.New("opensearch-endpoint is required when search-backend=opensearch")
	}
	parsedEndpoint, err := url.Parse(cfg.openSearchEndpoint)
	if err != nil || parsedEndpoint.Scheme == "" || parsedEndpoint.Host == "" {
		return errors.New("opensearch-endpoint must be an absolute http or https URL")
	}
	if parsedEndpoint.Scheme != "http" && parsedEndpoint.Scheme != "https" {
		return errors.New("opensearch-endpoint must use http or https")
	}
	if parsedEndpoint.User != nil || parsedEndpoint.RawQuery != "" || parsedEndpoint.Fragment != "" {
		return errors.New("opensearch-endpoint must not include credentials, query, or fragment")
	}
	if cfg.openSearchIndex == "" {
		return errors.New("opensearch-index is required when search-backend=opensearch")
	}
	if strings.TrimSpace(cfg.openSearchUsername) != "" && cfg.openSearchPassword == "" {
		return errors.New("opensearch-password is required when opensearch-username is set")
	}
	return nil
}

func publishAndVerify(
	ctx context.Context,
	cfg config,
	pool *pgxpool.Pool,
	client searchv1.SearchServiceClient,
	writer *kafkago.Writer,
	runID string,
	result *summary,
) error {
	messageID := "msg-search-edit-" + runID
	revokedMessageID := "msg-search-revoke-" + runID
	deletedMessageID := "msg-search-delete-" + runID
	afterLeaveMessageID := "msg-search-after-leave-" + runID

	joined := memberJoinedEvent("evt-search-joined-"+runID, cfg, 1, 1)
	if err := publishEvent(ctx, cfg, writer, joined); err != nil {
		return fmt.Errorf("publish member joined: %w", err)
	}
	result.Events = append(result.Events, snapshot(joined, ""))
	if err := waitMembership(ctx, pool, cfg, cfg.viewerUserID, "ACTIVE", nil); err != nil {
		return err
	}
	if err := ensureOpenSearchIndex(ctx, cfg); err != nil {
		return err
	}

	persisted := messagePersistedEvent("evt-search-persisted-"+runID, cfg, 2, messageID, "search smoke original phrase")
	if err := publishEvent(ctx, cfg, writer, persisted); err != nil {
		return fmt.Errorf("publish persisted: %w", err)
	}
	result.Events = append(result.Events, snapshot(persisted, messageID))
	if err := waitDocumentAndSyncOpenSearch(ctx, cfg, pool, messageID, "original phrase", result); err != nil {
		return err
	}
	if _, err := waitSearchHits(ctx, cfg, client, cfg.viewerUserID, "original phrase", messageID, 1); err != nil {
		return err
	}
	result.Checks.PersistedVisible = true

	edited := messageEditedEvent("evt-search-edited-"+runID, cfg, 2, messageID, "search smoke edited phrase")
	if err := publishEvent(ctx, cfg, writer, edited); err != nil {
		return fmt.Errorf("publish edited: %w", err)
	}
	result.Events = append(result.Events, snapshot(edited, messageID))
	if err := waitDocumentAndSyncOpenSearch(ctx, cfg, pool, messageID, "edited phrase", result); err != nil {
		return err
	}
	editedResponse, err := waitSearchHits(ctx, cfg, client, cfg.viewerUserID, "edited phrase", messageID, 1)
	if err != nil {
		return err
	}
	result.ProjectionVersion = editedResponse.GetProjectionVersion()
	result.Checks.EditedVisible = true
	if err := waitSearchHitsCount(ctx, cfg, client, cfg.viewerUserID, "original phrase", 0); err != nil {
		return err
	}
	result.Checks.OriginalHiddenAfterEdit = true

	if err := waitSearchHitsCount(ctx, cfg, client, "search-stranger-"+runID, "edited phrase", 0); err != nil {
		return err
	}
	result.Checks.StrangerHidden = true

	revokedPersisted := messagePersistedEvent("evt-search-revoke-persisted-"+runID, cfg, 3, revokedMessageID, "search smoke revoked phrase")
	if err := publishEvent(ctx, cfg, writer, revokedPersisted); err != nil {
		return fmt.Errorf("publish revoked persisted: %w", err)
	}
	result.Events = append(result.Events, snapshot(revokedPersisted, revokedMessageID))
	if err := waitDocumentAndSyncOpenSearch(ctx, cfg, pool, revokedMessageID, "revoked phrase", result); err != nil {
		return err
	}
	if _, err := waitSearchHits(ctx, cfg, client, cfg.viewerUserID, "revoked phrase", revokedMessageID, 1); err != nil {
		return err
	}
	revoked := messageRevokedEvent("evt-search-revoked-"+runID, cfg, 4, revokedMessageID)
	if err := publishEvent(ctx, cfg, writer, revoked); err != nil {
		return fmt.Errorf("publish revoked tombstone: %w", err)
	}
	result.Events = append(result.Events, snapshot(revoked, revokedMessageID))
	if err := waitSearchHitsCount(ctx, cfg, client, cfg.viewerUserID, "revoked phrase", 0); err != nil {
		return err
	}
	result.Checks.RevokedHidden = true

	deletedPersisted := messagePersistedEvent("evt-search-delete-persisted-"+runID, cfg, 5, deletedMessageID, "search smoke deleted phrase")
	if err := publishEvent(ctx, cfg, writer, deletedPersisted); err != nil {
		return fmt.Errorf("publish deleted persisted: %w", err)
	}
	result.Events = append(result.Events, snapshot(deletedPersisted, deletedMessageID))
	if err := waitDocumentAndSyncOpenSearch(ctx, cfg, pool, deletedMessageID, "deleted phrase", result); err != nil {
		return err
	}
	if _, err := waitSearchHits(ctx, cfg, client, cfg.viewerUserID, "deleted phrase", deletedMessageID, 1); err != nil {
		return err
	}
	deleted := messageDeletedEvent("evt-search-deleted-"+runID, cfg, 6, deletedMessageID)
	if err := publishEvent(ctx, cfg, writer, deleted); err != nil {
		return fmt.Errorf("publish deleted tombstone: %w", err)
	}
	result.Events = append(result.Events, snapshot(deleted, deletedMessageID))
	if err := waitSearchHitsCount(ctx, cfg, client, cfg.viewerUserID, "deleted phrase", 0); err != nil {
		return err
	}
	result.Checks.DeletedHidden = true

	left := memberLeftEvent("evt-search-left-"+runID, cfg, 7, 2)
	if err := publishEvent(ctx, cfg, writer, left); err != nil {
		return fmt.Errorf("publish member left: %w", err)
	}
	result.Events = append(result.Events, snapshot(left, ""))
	leaveSeq := int64(7)
	if err := waitMembership(ctx, pool, cfg, cfg.viewerUserID, "LEFT", &leaveSeq); err != nil {
		return err
	}

	afterLeave := messagePersistedEvent("evt-search-after-leave-"+runID, cfg, 8, afterLeaveMessageID, "search smoke after boundary phrase")
	if err := publishEvent(ctx, cfg, writer, afterLeave); err != nil {
		return fmt.Errorf("publish after-leave message: %w", err)
	}
	result.Events = append(result.Events, snapshot(afterLeave, afterLeaveMessageID))
	if err := waitDocumentAndSyncOpenSearch(ctx, cfg, pool, afterLeaveMessageID, "after boundary phrase", result); err != nil {
		return err
	}
	if err := waitSearchHitsCount(ctx, cfg, client, cfg.viewerUserID, "after boundary phrase", 0); err != nil {
		return err
	}
	result.Checks.AfterLeaveHidden = true
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
			Reason:            "search projection smoke join",
			OccurredAt:        timestamppb.Now(),
		},
	})
}

func memberLeftEvent(eventID string, cfg config, seq int64, version int64) *conversationtimelinev1.ConversationTimelineEvent {
	return baseTimelineEvent(eventID, eventTypeMemberLeft, cfg, seq, version, &conversationtimelinev1.ConversationTimelineEvent_ConversationMemberLeft{
		ConversationMemberLeft: &conversationtimelinev1.ConversationMemberLeftV1{
			ChangeId:          "change-" + eventID,
			ConversationId:    cfg.conversationID,
			BoundarySeq:       seq,
			TargetUserId:      cfg.viewerUserID,
			OperatorUserId:    cfg.viewerUserID,
			ChangeType:        conversationtimelinev1.ConversationMemberChangeType_CONVERSATION_MEMBER_CHANGE_TYPE_LEAVE,
			OldRole:           conversationtimelinev1.ConversationMemberRole_CONVERSATION_MEMBER_ROLE_MEMBER,
			NewRole:           conversationtimelinev1.ConversationMemberRole_CONVERSATION_MEMBER_ROLE_MEMBER,
			OldStatus:         conversationtimelinev1.ConversationMemberStatus_CONVERSATION_MEMBER_STATUS_ACTIVE,
			NewStatus:         conversationtimelinev1.ConversationMemberStatus_CONVERSATION_MEMBER_STATUS_LEFT,
			MemberVersion:     version,
			PermissionVersion: version,
			Reason:            "search projection smoke leave",
			OccurredAt:        timestamppb.Now(),
		},
	})
}

func messagePersistedEvent(eventID string, cfg config, seq int64, messageID string, text string) *conversationtimelinev1.ConversationTimelineEvent {
	payload, _ := structpb.NewStruct(map[string]any{"text": text})
	return baseTimelineEvent(eventID, eventTypeMessagePersisted, cfg, seq, seq, &conversationtimelinev1.ConversationTimelineEvent_MessagePersisted{
		MessagePersisted: &conversationtimelinev1.MessagePersistedV1{
			MessageId:       messageID,
			ConversationId:  cfg.conversationID,
			ConversationSeq: seq,
			SenderId:        cfg.senderUserID,
			DeviceId:        "search-smoke-device",
			ClientMsgId:     "client-" + eventID,
			CommandHash:     eventID,
			MessageType:     "TEXT",
			Payload:         payload,
			AcceptedAt:      timestamppb.Now(),
		},
	})
}

func messageEditedEvent(eventID string, cfg config, seq int64, messageID string, text string) *conversationtimelinev1.ConversationTimelineEvent {
	payload, _ := structpb.NewStruct(map[string]any{"text": text})
	return baseTimelineEvent(eventID, eventTypeMessageEdited, cfg, seq, seq+10, &conversationtimelinev1.ConversationTimelineEvent_MessageEdited{
		MessageEdited: &conversationtimelinev1.MessageEditedV1{
			MessageId:       messageID,
			ConversationId:  cfg.conversationID,
			ConversationSeq: seq,
			EditedBy:        cfg.senderUserID,
			AfterPayload:    payload,
			EditedAt:        timestamppb.Now(),
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

func messageDeletedEvent(eventID string, cfg config, seq int64, messageID string) *conversationtimelinev1.ConversationTimelineEvent {
	return baseTimelineEvent(eventID, eventTypeMessageDeleted, cfg, seq, seq+10, &conversationtimelinev1.ConversationTimelineEvent_MessageDeleted{
		MessageDeleted: &conversationtimelinev1.MessageDeletedV1{
			MessageId:       messageID,
			ConversationId:  cfg.conversationID,
			ConversationSeq: seq,
			DeletedBy:       cfg.senderUserID,
			DeleteScope:     conversationtimelinev1.MessageDeleteScope_MESSAGE_DELETE_SCOPE_CONVERSATION_VIEW,
			DeletedAt:       timestamppb.Now(),
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
		TraceId:          "trace-search-projection-smoke",
		CorrelationId:    "search-projection-smoke",
		CausationId:      eventID,
		Producer:         "search-projection-smoke",
		OccurredAt:       timestamppb.Now(),
		Metadata: &conversationtimelinev1.TimelineMetadata{
			FanoutMode:        "WRITE_FANOUT",
			PermissionVersion: permissionVersion,
			Classification:    "SEARCH_PROJECTION_SMOKE",
			MappingVersion:    eventType,
		},
	}
	switch typed := payload.(type) {
	case *conversationtimelinev1.ConversationTimelineEvent_ConversationMemberJoined:
		event.Payload = typed
	case *conversationtimelinev1.ConversationTimelineEvent_ConversationMemberLeft:
		event.Payload = typed
	case *conversationtimelinev1.ConversationTimelineEvent_MessagePersisted:
		event.Payload = typed
	case *conversationtimelinev1.ConversationTimelineEvent_MessageEdited:
		event.Payload = typed
	case *conversationtimelinev1.ConversationTimelineEvent_MessageRevoked:
		event.Payload = typed
	case *conversationtimelinev1.ConversationTimelineEvent_MessageDeleted:
		event.Payload = typed
	}
	return event
}

func waitSearchHits(
	ctx context.Context,
	cfg config,
	client searchv1.SearchServiceClient,
	userID string,
	query string,
	messageID string,
	wantCount int,
) (*searchv1.SearchMessagesResponse, error) {
	var lastCount int
	var lastIDs []string
	err := waitUntil(ctx, cfg, func() (bool, error) {
		response, err := searchMessages(ctx, cfg, client, userID, query)
		if err != nil {
			return false, nil
		}
		lastCount = len(response.GetItems())
		lastIDs = lastIDs[:0]
		matchedID := messageID == ""
		for _, item := range response.GetItems() {
			lastIDs = append(lastIDs, item.GetMessageId())
			if item.GetMessageId() == messageID {
				matchedID = true
			}
		}
		return lastCount == wantCount && matchedID, nil
	})
	if err != nil {
		return nil, fmt.Errorf("wait search query=%q count=%d message_id=%s last_count=%d last_ids=%v: %w", query, wantCount, messageID, lastCount, lastIDs, err)
	}
	return searchMessages(ctx, cfg, client, userID, query)
}

func waitSearchHitsCount(ctx context.Context, cfg config, client searchv1.SearchServiceClient, userID string, query string, wantCount int) error {
	_, err := waitSearchHits(ctx, cfg, client, userID, query, "", wantCount)
	return err
}

func searchMessages(ctx context.Context, cfg config, client searchv1.SearchServiceClient, userID string, query string) (*searchv1.SearchMessagesResponse, error) {
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	return client.SearchMessages(requestCtx, &searchv1.SearchMessagesRequest{
		AuthContext: &searchv1.AuthContext{
			TenantId:  cfg.tenantID,
			UserId:    userID,
			DeviceId:  cfg.viewerDeviceID,
			SessionId: "search-projection-smoke",
			TraceId:   "trace-search-projection-smoke",
			RequestId: "request-search-projection-smoke",
		},
		Query:          query,
		ConversationId: cfg.conversationID,
		Limit:          20,
	})
}

func waitDocumentAndSyncOpenSearch(ctx context.Context, cfg config, pool *pgxpool.Pool, messageID string, textContains string, result *summary) error {
	var document searchDocumentSnapshot
	err := waitUntil(ctx, cfg, func() (bool, error) {
		read, err := readSearchDocument(ctx, pool, cfg, messageID)
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if textContains != "" && !strings.Contains(read.SearchableText, textContains) {
			return false, nil
		}
		document = read
		return true, nil
	})
	if err != nil {
		return fmt.Errorf("wait document %s text=%q: %w", messageID, textContains, err)
	}
	if cfg.searchBackend != "opensearch" {
		return nil
	}
	if err := indexOpenSearchDocument(ctx, cfg, document); err != nil {
		return err
	}
	result.OpenSearchDocuments++
	return nil
}

func preflightOpenSearch(ctx context.Context, cfg config, result *summary) error {
	status, _, err := openSearchRequest(ctx, cfg, http.MethodGet, "/", nil)
	if err != nil {
		return fmt.Errorf("connect opensearch endpoint: %w", err)
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return fmt.Errorf("opensearch endpoint returned status %d", status)
	}
	if err := ensureOpenSearchIndex(ctx, cfg); err != nil {
		return err
	}
	result.OpenSearchReady = true
	return nil
}

func ensureOpenSearchIndex(ctx context.Context, cfg config) error {
	if cfg.searchBackend != "opensearch" {
		return nil
	}
	body := map[string]any{
		"settings": map[string]any{
			"index": map[string]any{
				"number_of_shards":   1,
				"number_of_replicas": 0,
			},
		},
		"mappings": map[string]any{
			"properties": map[string]any{
				"tenant_id":        map[string]any{"type": "keyword"},
				"conversation_id":  map[string]any{"type": "keyword"},
				"message_id":       map[string]any{"type": "keyword"},
				"conversation_seq": map[string]any{"type": "long"},
				"searchable_text":  map[string]any{"type": "text"},
			},
		},
	}
	status, responseBody, err := openSearchRequest(ctx, cfg, http.MethodPut, "/"+url.PathEscape(cfg.openSearchIndex), body)
	if err != nil {
		return err
	}
	if status == http.StatusOK || status == http.StatusCreated {
		return nil
	}
	if status == http.StatusBadRequest && strings.Contains(responseBody, "resource_already_exists_exception") {
		return nil
	}
	return fmt.Errorf("create opensearch index returned status %d", status)
}

func indexOpenSearchDocument(ctx context.Context, cfg config, document searchDocumentSnapshot) error {
	body := map[string]any{
		"tenant_id":        document.TenantID,
		"conversation_id":  document.ConversationID,
		"message_id":       document.MessageID,
		"conversation_seq": document.ConversationSeq,
		"searchable_text":  document.SearchableText,
	}
	documentID := url.PathEscape(document.TenantID + ":" + document.ConversationID + ":" + document.MessageID)
	path := "/" + url.PathEscape(cfg.openSearchIndex) + "/_doc/" + documentID
	status, _, err := openSearchRequest(ctx, cfg, http.MethodPut, path, body)
	if err != nil {
		return err
	}
	if status != http.StatusOK && status != http.StatusCreated {
		return fmt.Errorf("index opensearch document returned status %d", status)
	}
	status, _, err = openSearchRequest(ctx, cfg, http.MethodPost, "/"+url.PathEscape(cfg.openSearchIndex)+"/_refresh", nil)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("refresh opensearch index returned status %d", status)
	}
	return nil
}

func openSearchRequest(ctx context.Context, cfg config, method string, path string, body any) (int, string, error) {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return 0, "", fmt.Errorf("encode opensearch request: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, method, openSearchURL(cfg, path), reader)
	if err != nil {
		return 0, "", fmt.Errorf("build opensearch request: %w", err)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if cfg.openSearchAPIKey != "" {
		request.Header.Set("Authorization", "ApiKey "+cfg.openSearchAPIKey)
	} else if cfg.openSearchUsername != "" {
		request.SetBasicAuth(cfg.openSearchUsername, cfg.openSearchPassword)
	}
	response, err := cfg.openSearchHTTPClient.Do(request)
	if err != nil {
		return 0, "", fmt.Errorf("opensearch request failed: %w", err)
	}
	defer response.Body.Close()
	bodyBytes, err := io.ReadAll(io.LimitReader(response.Body, 64*1024))
	if err != nil {
		return response.StatusCode, "", fmt.Errorf("read opensearch response: %w", err)
	}
	return response.StatusCode, string(bodyBytes), nil
}

func openSearchURL(cfg config, path string) string {
	base := strings.TrimRight(cfg.openSearchEndpoint, "/")
	if strings.HasPrefix(path, "/") {
		return base + path
	}
	return base + "/" + path
}

func waitMembership(ctx context.Context, pool *pgxpool.Pool, cfg config, userID string, wantStatus string, wantLeaveSeq *int64) error {
	return waitUntil(ctx, cfg, func() (bool, error) {
		membership, err := readMembership(ctx, pool, cfg, userID)
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if membership.Status != wantStatus {
			return false, nil
		}
		if wantLeaveSeq != nil {
			return membership.LeaveSeq != nil && *membership.LeaveSeq == *wantLeaveSeq, nil
		}
		return true, nil
	})
}

func readSearchDocument(ctx context.Context, pool *pgxpool.Pool, cfg config, messageID string) (searchDocumentSnapshot, error) {
	var document searchDocumentSnapshot
	err := pool.QueryRow(ctx, `
SELECT tenant_id, conversation_id, message_id, conversation_seq, searchable_text
FROM search_message_documents
WHERE tenant_id = $1
  AND conversation_id = $2
  AND message_id = $3
`, cfg.tenantID, cfg.conversationID, messageID).Scan(
		&document.TenantID,
		&document.ConversationID,
		&document.MessageID,
		&document.ConversationSeq,
		&document.SearchableText,
	)
	return document, err
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

func cleanupSearchRows(ctx context.Context, pool *pgxpool.Pool, cfg config) error {
	statements := []struct {
		query string
		args  []any
	}{
		{`DELETE FROM search_message_documents WHERE tenant_id = $1`, []any{cfg.tenantID}},
		{`DELETE FROM search_membership_projection WHERE tenant_id = $1`, []any{cfg.tenantID}},
		{`DELETE FROM search_projection_checkpoints WHERE consumer_group = $1 AND topic = $2`, []any{cfg.consumerGroup, cfg.topic}},
	}
	for _, statement := range statements {
		if _, err := pool.Exec(ctx, statement.query, statement.args...); err != nil {
			return fmt.Errorf("cleanup search rows: %w", err)
		}
	}
	return nil
}

func readMembership(ctx context.Context, pool *pgxpool.Pool, cfg config, userID string) (membershipSnapshot, error) {
	var membership membershipSnapshot
	err := pool.QueryRow(ctx, `
SELECT status, join_seq, leave_seq
FROM search_membership_projection
WHERE tenant_id = $1
  AND conversation_id = $2
  AND user_id = $3
`, cfg.tenantID, cfg.conversationID, userID).Scan(&membership.Status, &membership.JoinSeq, &membership.LeaveSeq)
	return membership, err
}

func countDocuments(ctx context.Context, pool *pgxpool.Pool, cfg config) (int64, error) {
	var count int64
	err := pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM search_message_documents
WHERE tenant_id = $1
  AND conversation_id = $2
`, cfg.tenantID, cfg.conversationID).Scan(&count)
	return count, err
}

func checkpointOffset(ctx context.Context, pool *pgxpool.Pool, cfg config) (int64, error) {
	var offset int64
	err := pool.QueryRow(ctx, `
SELECT COALESCE(MAX(offset_value), 0)
FROM search_projection_checkpoints
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

func snapshot(event *conversationtimelinev1.ConversationTimelineEvent, messageID string) eventSnapshot {
	return eventSnapshot{
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
	path := filepath.Join(resultDir, "search-projection-summary.json")
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
		return "search-smoke"
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

func envOr(name string, defaultValue string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return defaultValue
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
