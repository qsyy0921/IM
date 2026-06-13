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
	"github.com/qsyy0921/IM/loadtest/internal/grpctls"
	contacteventsv1 "github.com/qsyy0921/IM/schemas/kafka/contacts/v1"
	policyeventsv1 "github.com/qsyy0921/IM/schemas/kafka/policy/v1"
	kafkago "github.com/segmentio/kafka-go"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	topicContactEvents = "im.contact.events"

	contactEventRequestAccepted = "contact.request.accepted.v1"
	contactEventEdgeBlocked     = "contact.edge.blocked.v1"
	contactEventEdgeUnblocked   = "contact.edge.unblocked.v1"

	contactEdgeStatusActive  = "ACTIVE"
	contactEdgeStatusBlocked = "BLOCKED"
)

type config struct {
	brokers        []string
	topic          string
	auditTopic     string
	consumerGroup  string
	policyGRPCAddr string
	policyTLS      grpctls.Config
	pgDSN          string
	resultDir      string
	tenantID       string
	ownerUserID    string
	contactUserID  string
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
	AuditTopic             string                 `json:"policy_audit_topic,omitempty"`
	ConsumerGroup          string                 `json:"consumer_group"`
	PolicyGRPCAddr         string                 `json:"policy_grpc_addr,omitempty"`
	PolicyTLSEnabled       bool                   `json:"policy_tls_enabled"`
	TenantID               string                 `json:"tenant_id"`
	OwnerUserID            string                 `json:"owner_user_id"`
	ContactUserID          string                 `json:"contact_user_id"`
	StartedAt              time.Time              `json:"started_at"`
	FinishedAt             time.Time              `json:"finished_at"`
	Success                bool                   `json:"success"`
	Error                  string                 `json:"error,omitempty"`
	AfterAccepted          edgeSnapshot           `json:"after_accepted"`
	AfterBlocked           edgeSnapshot           `json:"after_blocked"`
	AfterUnblocked         edgeSnapshot           `json:"after_unblocked"`
	ReverseEdge            edgeSnapshot           `json:"reverse_edge"`
	AfterAcceptedDecision  policyDecisionSnapshot `json:"after_accepted_policy_decision"`
	AfterBlockedDecision   policyDecisionSnapshot `json:"after_blocked_policy_decision"`
	AfterUnblockedDecision policyDecisionSnapshot `json:"after_unblocked_policy_decision"`
	CheckpointOffset       int64                  `json:"checkpoint_offset_value"`
	AuditOutboxCount       int64                  `json:"policy_decision_audit_outbox_count"`
	AuditOutboxStatus      auditOutboxStatus      `json:"policy_decision_audit_outbox_status"`
	AuditKafkaEventCount   int64                  `json:"policy_audit_kafka_event_count,omitempty"`
	Events                 []string               `json:"events"`
}

type auditOutboxStatus struct {
	Total     int64 `json:"total"`
	Pending   int64 `json:"pending"`
	Published int64 `json:"published"`
	DLQ       int64 `json:"dlq"`
}

type edgeSnapshot struct {
	OwnerUserID      string `json:"owner_user_id"`
	ContactUserID    string `json:"contact_user_id"`
	Status           string `json:"status"`
	EdgeVersion      int64  `json:"edge_version"`
	UpdatedByEventID string `json:"updated_by_event_id"`
}

type policyDecisionSnapshot struct {
	Checked           bool   `json:"checked"`
	Allowed           bool   `json:"allowed"`
	PermissionVersion int64  `json:"permission_version"`
	Classification    string `json:"classification"`
	Reason            string `json:"reason"`
	DirectPeerUserID  string `json:"direct_peer_user_id"`
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
	flag.StringVar(&cfg.topic, "topic", topicContactEvents, "contact events topic")
	flag.StringVar(&cfg.auditTopic, "audit-topic", "", "optional policy audit events topic for relay verification")
	flag.StringVar(&cfg.consumerGroup, "consumer-group", "nexusim-policy-contact-smoke", "policy contact consumer group")
	flag.StringVar(&cfg.policyGRPCAddr, "policy-grpc-addr", "", "optional policy-service gRPC address for direct contact-block decision checks")
	flag.StringVar(&cfg.policyTLS.CAFile, "policy-tls-ca-file", "", "CA PEM for policy-service gRPC TLS")
	flag.StringVar(&cfg.policyTLS.ServerName, "policy-tls-server-name", "", "server name for policy-service gRPC TLS")
	flag.StringVar(&cfg.policyTLS.ClientCertFile, "policy-tls-client-cert-file", "", "client certificate PEM for policy-service mTLS")
	flag.StringVar(&cfg.policyTLS.ClientKeyFile, "policy-tls-client-key-file", "", "client private key PEM for policy-service mTLS")
	flag.StringVar(&cfg.pgDSN, "pg-dsn", "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable", "PostgreSQL DSN")
	flag.StringVar(&cfg.resultDir, "result-dir", "H:\\NexusIM\\loadtest-results\\policy-contact-smoke", "result directory")
	flag.StringVar(&cfg.tenantID, "tenant-id", "tenant-policy-contact-smoke", "tenant id")
	flag.StringVar(&cfg.ownerUserID, "owner-user-id", "alice", "contact edge owner user id")
	flag.StringVar(&cfg.contactUserID, "contact-user-id", "bob", "contact user id")
	flag.DurationVar(&cfg.timeout, "timeout", 20*time.Second, "projection wait timeout")
	flag.BoolVar(&cfg.cleanup, "cleanup", true, "cleanup policy projection rows for the smoke tenant")
	flag.Parse()
	cfg.brokers = splitCSV(brokers)
	if cfg.timeout <= 0 {
		cfg.timeout = 20 * time.Second
	}
	return cfg
}

func run(cfg config) error {
	if err := os.MkdirAll(cfg.resultDir, 0o755); err != nil {
		return fmt.Errorf("create result dir: %w", err)
	}
	started := time.Now().UTC()
	s := summary{
		Commit:           gitOutput("rev-parse", "--short", "HEAD"),
		CommitFull:       gitOutput("rev-parse", "HEAD"),
		GitStatusShort:   gitOutput("status", "--short"),
		ResultDir:        cfg.resultDir,
		Topic:            cfg.topic,
		AuditTopic:       cfg.auditTopic,
		ConsumerGroup:    cfg.consumerGroup,
		PolicyGRPCAddr:   cfg.policyGRPCAddr,
		PolicyTLSEnabled: cfg.policyTLS.Enabled(),
		TenantID:         cfg.tenantID,
		OwnerUserID:      cfg.ownerUserID,
		ContactUserID:    cfg.contactUserID,
		StartedAt:        started,
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
	if err := ensureTopic(ctx, cfg.brokers, cfg.topic); err != nil {
		s.Error = fmt.Sprintf("ensure topic: %v", err)
		return fmt.Errorf("ensure topic: %w", err)
	}
	if cfg.auditTopic != "" {
		if err := ensureTopic(ctx, cfg.brokers, cfg.auditTopic); err != nil {
			s.Error = fmt.Sprintf("ensure audit topic: %v", err)
			return fmt.Errorf("ensure audit topic: %w", err)
		}
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

	var policyClient policyv1.PolicyServiceClient
	var policyConn *grpc.ClientConn
	if cfg.policyGRPCAddr != "" {
		dialOption, err := grpctls.DialOption(cfg.policyTLS, "policy-tls")
		if err != nil {
			s.Error = fmt.Sprintf("configure policy-service TLS: %v", err)
			return fmt.Errorf("configure policy-service TLS: %w", err)
		}
		policyConn, err = grpc.NewClient("passthrough:///"+cfg.policyGRPCAddr, dialOption)
		if err != nil {
			s.Error = fmt.Sprintf("dial policy service: %v", err)
			return fmt.Errorf("dial policy service: %w", err)
		}
		defer policyConn.Close()
		policyClient = policyv1.NewPolicyServiceClient(policyConn)
	}

	runID := randomSuffix()
	acceptedID := "policy-contact-accepted-" + runID
	blockedID := "policy-contact-blocked-" + runID
	unblockedID := "policy-contact-unblocked-" + runID

	if err := writer.WriteMessages(ctx, contactMessage(acceptedID, cfg, acceptedEvent(acceptedID, cfg, 1))); err != nil {
		s.Error = fmt.Sprintf("write accepted contact event: %v", err)
		return fmt.Errorf("write accepted contact event: %w", err)
	}
	s.Events = append(s.Events, acceptedID)
	s.AfterAccepted, err = waitEdge(ctx, pool, cfg, cfg.ownerUserID, cfg.contactUserID, contactEdgeStatusActive, 1, acceptedID)
	if err != nil {
		s.Error = err.Error()
		return err
	}
	s.ReverseEdge, err = waitEdge(ctx, pool, cfg, cfg.contactUserID, cfg.ownerUserID, contactEdgeStatusActive, 1, acceptedID)
	if err != nil {
		s.Error = err.Error()
		return err
	}
	s.AfterAcceptedDecision, err = waitPolicyDecision(ctx, policyClient, cfg, true, "")
	if err != nil {
		s.Error = err.Error()
		return err
	}

	if err := writer.WriteMessages(ctx, contactMessage(blockedID, cfg, blockedEvent(blockedID, cfg, 2))); err != nil {
		s.Error = fmt.Sprintf("write blocked contact event: %v", err)
		return fmt.Errorf("write blocked contact event: %w", err)
	}
	s.Events = append(s.Events, blockedID)
	s.AfterBlocked, err = waitEdge(ctx, pool, cfg, cfg.ownerUserID, cfg.contactUserID, contactEdgeStatusBlocked, 2, blockedID)
	if err != nil {
		s.Error = err.Error()
		return err
	}
	s.AfterBlockedDecision, err = waitPolicyDecision(ctx, policyClient, cfg, false, "CONTACT_BLOCKED")
	if err != nil {
		s.Error = err.Error()
		return err
	}

	if err := writer.WriteMessages(ctx, contactMessage(unblockedID, cfg, unblockedEvent(unblockedID, cfg, 3))); err != nil {
		s.Error = fmt.Sprintf("write unblocked contact event: %v", err)
		return fmt.Errorf("write unblocked contact event: %w", err)
	}
	s.Events = append(s.Events, unblockedID)
	s.AfterUnblocked, err = waitEdge(ctx, pool, cfg, cfg.ownerUserID, cfg.contactUserID, contactEdgeStatusActive, 3, unblockedID)
	if err != nil {
		s.Error = err.Error()
		return err
	}
	s.AfterUnblockedDecision, err = waitPolicyDecision(ctx, policyClient, cfg, true, "")
	if err != nil {
		s.Error = err.Error()
		return err
	}
	s.CheckpointOffset, err = waitCheckpoint(ctx, pool, cfg)
	if err != nil {
		s.Error = err.Error()
		return err
	}
	if policyClient != nil {
		s.AuditOutboxCount, err = waitAuditOutboxCount(ctx, pool, cfg, 3)
		if err != nil {
			s.Error = err.Error()
			return err
		}
		if cfg.auditTopic != "" {
			s.AuditOutboxStatus, err = waitAuditOutboxStatus(ctx, pool, cfg, auditOutboxStatus{
				Total:     3,
				Published: 3,
			})
			if err != nil {
				s.Error = err.Error()
				return err
			}
			s.AuditKafkaEventCount, err = waitPolicyAuditKafkaEvents(ctx, cfg, 3)
			if err != nil {
				s.Error = err.Error()
				return err
			}
		}
	}
	s.Success = true
	return nil
}

func contactMessage(eventID string, cfg config, event *contacteventsv1.ContactEvent) kafkago.Message {
	value, _ := proto.Marshal(event)
	return kafkago.Message{
		Key:   []byte(cfg.tenantID + ":" + cfg.ownerUserID + ":" + cfg.contactUserID),
		Value: value,
		Headers: []kafkago.Header{
			{Key: "event_id", Value: []byte(eventID)},
		},
	}
}

func acceptedEvent(eventID string, cfg config, version int64) *contacteventsv1.ContactEvent {
	return baseEvent(eventID, contactEventRequestAccepted, cfg, version, &contacteventsv1.ContactEvent_RequestAccepted{
		RequestAccepted: &contacteventsv1.ContactRequestAcceptedV1{
			TenantId:       cfg.tenantID,
			RequestId:      "request-" + eventID,
			SenderUserId:   cfg.ownerUserID,
			ReceiverUserId: cfg.contactUserID,
			Status:         "ACCEPTED",
			EdgeVersion:    version,
			OccurredAt:     timestamppb.Now(),
		},
	})
}

func blockedEvent(eventID string, cfg config, version int64) *contacteventsv1.ContactEvent {
	return baseEvent(eventID, contactEventEdgeBlocked, cfg, version, &contacteventsv1.ContactEvent_EdgeBlocked{
		EdgeBlocked: &contacteventsv1.ContactEdgeBlockedV1{
			TenantId:       cfg.tenantID,
			OwnerUserId:    cfg.ownerUserID,
			ContactUserId:  cfg.contactUserID,
			PreviousStatus: contactEdgeStatusActive,
			Status:         contactEdgeStatusBlocked,
			EdgeVersion:    version,
			Reason:         "policy contact smoke",
			OccurredAt:     timestamppb.Now(),
		},
	})
}

func unblockedEvent(eventID string, cfg config, version int64) *contacteventsv1.ContactEvent {
	return baseEvent(eventID, contactEventEdgeUnblocked, cfg, version, &contacteventsv1.ContactEvent_EdgeUnblocked{
		EdgeUnblocked: &contacteventsv1.ContactEdgeUnblockedV1{
			TenantId:       cfg.tenantID,
			OwnerUserId:    cfg.ownerUserID,
			ContactUserId:  cfg.contactUserID,
			PreviousStatus: contactEdgeStatusBlocked,
			Status:         contactEdgeStatusActive,
			EdgeVersion:    version,
			OccurredAt:     timestamppb.Now(),
		},
	})
}

func baseEvent(eventID string, eventType string, cfg config, version int64, payload any) *contacteventsv1.ContactEvent {
	event := &contacteventsv1.ContactEvent{
		EventId:          eventID,
		EventType:        eventType,
		EventVersion:     "v1",
		TenantId:         cfg.tenantID,
		AggregateType:    "contact.edge",
		AggregateId:      cfg.ownerUserID + ":" + cfg.contactUserID,
		AggregateVersion: version,
		PartitionKey:     cfg.tenantID + ":" + cfg.ownerUserID + ":" + cfg.contactUserID,
		MappingVersion:   1,
		TraceId:          "trace-policy-contact-smoke",
		CorrelationId:    "policy-contact-smoke",
		CausationId:      eventID,
		Producer:         "policy-contact-smoke",
		OccurredAt:       timestamppb.Now(),
	}
	switch typed := payload.(type) {
	case *contacteventsv1.ContactEvent_RequestAccepted:
		event.Payload = typed
	case *contacteventsv1.ContactEvent_EdgeBlocked:
		event.Payload = typed
	case *contacteventsv1.ContactEvent_EdgeUnblocked:
		event.Payload = typed
	}
	return event
}

func waitEdge(ctx context.Context, pool *pgxpool.Pool, cfg config, owner string, contactUser string, status string, version int64, eventID string) (edgeSnapshot, error) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		edge, ok, err := readEdge(ctx, pool, cfg.tenantID, owner, contactUser)
		if err != nil {
			return edgeSnapshot{}, err
		}
		if ok && edge.Status == status && edge.EdgeVersion == version && edge.UpdatedByEventID == eventID {
			return edge, nil
		}
		select {
		case <-ctx.Done():
			return edgeSnapshot{}, fmt.Errorf("timed out waiting for edge %s->%s status=%s version=%d event=%s", owner, contactUser, status, version, eventID)
		case <-ticker.C:
		}
	}
}

func readEdge(ctx context.Context, pool *pgxpool.Pool, tenantID string, owner string, contactUser string) (edgeSnapshot, bool, error) {
	var edge edgeSnapshot
	edge.OwnerUserID = owner
	edge.ContactUserID = contactUser
	err := pool.QueryRow(ctx, `
SELECT status, edge_version, updated_by_event_id
FROM policy_contact_edges_projection
WHERE tenant_id = $1
  AND owner_user_id = $2
  AND contact_user_id = $3
`, tenantID, owner, contactUser).Scan(&edge.Status, &edge.EdgeVersion, &edge.UpdatedByEventID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return edgeSnapshot{}, false, nil
		}
		return edgeSnapshot{}, false, fmt.Errorf("read projected edge: %w", err)
	}
	return edge, true, nil
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
		if offset > 0 {
			return offset, nil
		}
		select {
		case <-ctx.Done():
			return 0, fmt.Errorf("timed out waiting for policy contact checkpoint")
		case <-ticker.C:
		}
	}
}

func waitAuditOutboxCount(ctx context.Context, pool *pgxpool.Pool, cfg config, minCount int64) (int64, error) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		var count int64
		err := pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM policy_decision_audit_outbox
WHERE tenant_id = $1
`, cfg.tenantID).Scan(&count)
		if err != nil {
			return 0, fmt.Errorf("read policy decision audit outbox count: %w", err)
		}
		if count >= minCount {
			return count, nil
		}
		select {
		case <-ctx.Done():
			return 0, fmt.Errorf("timed out waiting for policy decision audit outbox count >= %d, got %d", minCount, count)
		case <-ticker.C:
		}
	}
}

func waitAuditOutboxStatus(ctx context.Context, pool *pgxpool.Pool, cfg config, want auditOutboxStatus) (auditOutboxStatus, error) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	var last auditOutboxStatus
	for {
		status, err := readAuditOutboxStatus(ctx, pool, cfg)
		if err != nil {
			return auditOutboxStatus{}, err
		}
		last = status
		if status.Total >= want.Total &&
			status.Published >= want.Published &&
			status.Pending == want.Pending &&
			status.DLQ == want.DLQ {
			return status, nil
		}
		select {
		case <-ctx.Done():
			return auditOutboxStatus{}, fmt.Errorf("timed out waiting for policy audit outbox status: last=%+v want=%+v", last, want)
		case <-ticker.C:
		}
	}
}

func readAuditOutboxStatus(ctx context.Context, pool *pgxpool.Pool, cfg config) (auditOutboxStatus, error) {
	var status auditOutboxStatus
	rows, err := pool.Query(ctx, `
SELECT status, COUNT(*)
FROM policy_decision_audit_outbox
WHERE tenant_id = $1
GROUP BY status
`, cfg.tenantID)
	if err != nil {
		return auditOutboxStatus{}, fmt.Errorf("read policy audit outbox status: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var rowStatus string
		var count int64
		if err := rows.Scan(&rowStatus, &count); err != nil {
			return auditOutboxStatus{}, fmt.Errorf("scan policy audit outbox status: %w", err)
		}
		status.Total += count
		switch rowStatus {
		case "PENDING":
			status.Pending = count
		case "PUBLISHED":
			status.Published = count
		case "DLQ":
			status.DLQ = count
		}
	}
	if err := rows.Err(); err != nil {
		return auditOutboxStatus{}, fmt.Errorf("read policy audit outbox status rows: %w", err)
	}
	return status, nil
}

func waitPolicyAuditKafkaEvents(ctx context.Context, cfg config, minCount int64) (int64, error) {
	reader := kafkago.NewReader(kafkago.ReaderConfig{
		Brokers:     cfg.brokers,
		Topic:       cfg.auditTopic,
		Partition:   0,
		MinBytes:    1,
		MaxBytes:    1 << 20,
		MaxWait:     100 * time.Millisecond,
		StartOffset: kafkago.FirstOffset,
	})
	defer reader.Close()
	var count int64
	for count < minCount {
		message, err := reader.ReadMessage(ctx)
		if err != nil {
			return count, fmt.Errorf("read policy audit kafka event: %w", err)
		}
		var event policyeventsv1.PolicyEvent
		if err := proto.Unmarshal(message.Value, &event); err != nil {
			return count, fmt.Errorf("decode policy audit kafka event: %w", err)
		}
		if event.GetTenantId() != cfg.tenantID {
			continue
		}
		if event.GetMessageActionDecision() == nil {
			return count, fmt.Errorf("policy audit kafka event missing message_action_decision payload")
		}
		count++
	}
	return count, nil
}

func waitPolicyDecision(
	ctx context.Context,
	client policyv1.PolicyServiceClient,
	cfg config,
	wantAllowed bool,
	wantClassification string,
) (policyDecisionSnapshot, error) {
	if client == nil {
		return policyDecisionSnapshot{}, nil
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	var lastErr error
	for {
		decision, err := readPolicyDecision(ctx, client, cfg)
		if err == nil &&
			decision.Allowed == wantAllowed &&
			(wantClassification == "" || decision.Classification == wantClassification) {
			return decision, nil
		}
		if err != nil {
			lastErr = err
		} else {
			lastErr = fmt.Errorf("unexpected policy decision: got allowed=%v classification=%s want allowed=%v classification=%s", decision.Allowed, decision.Classification, wantAllowed, wantClassification)
		}
		select {
		case <-ctx.Done():
			return policyDecisionSnapshot{}, fmt.Errorf("timed out waiting for policy decision: %w", lastErr)
		case <-ticker.C:
		}
	}
}

func readPolicyDecision(ctx context.Context, client policyv1.PolicyServiceClient, cfg config) (policyDecisionSnapshot, error) {
	response, err := client.CheckMessageAction(ctx, &policyv1.CheckMessageActionRequest{
		AuthContext: &policyv1.AuthContext{
			TenantId:  cfg.tenantID,
			UserId:    cfg.ownerUserID,
			DeviceId:  "policy-contact-smoke-device",
			SessionId: "policy-contact-smoke-session",
			TraceId:   "trace-policy-contact-smoke",
			RequestId: "request-policy-contact-smoke",
		},
		ConversationId:   "conv-policy-contact-smoke",
		Action:           policyv1.MessageAction_MESSAGE_ACTION_SEND,
		DirectPeerUserId: cfg.contactUserID,
	})
	if err != nil {
		return policyDecisionSnapshot{}, err
	}
	return policyDecisionSnapshot{
		Checked:           true,
		Allowed:           response.GetAllowed(),
		PermissionVersion: response.GetPermissionVersion(),
		Classification:    response.GetClassification(),
		Reason:            response.GetReason(),
		DirectPeerUserID:  cfg.contactUserID,
	}, nil
}

func cleanupTenant(ctx context.Context, pool *pgxpool.Pool, cfg config) error {
	if _, err := pool.Exec(ctx, `DELETE FROM policy_decision_audit_outbox_repair_audit WHERE tenant_id = $1`, cfg.tenantID); err != nil {
		return fmt.Errorf("cleanup policy decision audit outbox repair audit: %w", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM policy_decision_audit_outbox WHERE tenant_id = $1`, cfg.tenantID); err != nil {
		return fmt.Errorf("cleanup policy decision audit outbox: %w", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM policy_contact_edges_projection WHERE tenant_id = $1`, cfg.tenantID); err != nil {
		return fmt.Errorf("cleanup policy contact projection: %w", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM policy_kafka_checkpoints WHERE consumer_group = $1 AND topic = $2`, cfg.consumerGroup, cfg.topic); err != nil {
		return fmt.Errorf("cleanup policy contact projection: %w", err)
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
	path := filepath.Join(resultDir, "policy-contact-summary.json")
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
