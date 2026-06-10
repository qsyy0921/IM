package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	contactsv1 "github.com/qsyy0921/IM/api/proto/nexusim/contacts/v1"
	contacteventsv1 "github.com/qsyy0921/IM/schemas/kafka/contacts/v1"
	kafkago "github.com/segmentio/kafka-go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

type config struct {
	target           string
	resultDir        string
	pgDSN            string
	kafkaBrokers     []string
	contactTopic     string
	requestTimeout   time.Duration
	waitTimeout      time.Duration
	pollInterval     time.Duration
	tenantID         string
	senderUserID     string
	receiverUserID   string
	senderDeviceID   string
	receiverDeviceID string
	scenario         string
	cleanup          bool
}

type summary struct {
	Commit                string              `json:"commit"`
	CommitFull            string              `json:"commit_full"`
	GitDirty              bool                `json:"git_dirty"`
	GitStatusShort        string              `json:"git_status_short,omitempty"`
	Target                string              `json:"target"`
	ResultDir             string              `json:"result_dir"`
	TenantID              string              `json:"tenant_id"`
	SenderUserID          string              `json:"sender_user_id"`
	ReceiverUserID        string              `json:"receiver_user_id"`
	Scenario              string              `json:"scenario"`
	ContactTopic          string              `json:"contact_topic"`
	StartedAt             time.Time           `json:"started_at"`
	FinishedAt            time.Time           `json:"finished_at"`
	Success               bool                `json:"success"`
	Error                 string              `json:"error,omitempty"`
	SendContactRequest    sendSummary         `json:"send_contact_request"`
	RespondContactRequest respondSummary      `json:"respond_contact_request"`
	SenderList            listSummary         `json:"sender_list"`
	ReceiverList          listSummary         `json:"receiver_list"`
	SenderState           stateSummary        `json:"sender_state"`
	ReceiverState         stateSummary        `json:"receiver_state"`
	ContactsOutbox        outboxStats         `json:"contacts_outbox"`
	ContactKafkaEvents    []contactKafkaEvent `json:"contact_kafka_events"`
	LatenciesMS           map[string]float64  `json:"latencies_ms"`
}

type sendSummary struct {
	RequestID        string `json:"request_id"`
	Status           string `json:"status"`
	IdempotentReplay bool   `json:"idempotent_replay"`
}

type respondSummary struct {
	RequestID        string `json:"request_id"`
	Status           string `json:"status"`
	IdempotentReplay bool   `json:"idempotent_replay"`
}

type listSummary struct {
	OwnerUserID    string   `json:"owner_user_id"`
	ContactCount   int      `json:"contact_count"`
	ContactUserIDs []string `json:"contact_user_ids"`
}

type stateSummary struct {
	OwnerUserID     string `json:"owner_user_id"`
	ContactUserID   string `json:"contact_user_id"`
	Status          string `json:"status"`
	SourceRequestID string `json:"source_request_id"`
	Version         int64  `json:"version"`
	Error           string `json:"error,omitempty"`
}

type outboxStats struct {
	Total     int64 `json:"total"`
	Pending   int64 `json:"pending"`
	Published int64 `json:"published"`
	DLQ       int64 `json:"dlq"`
}

type contactKafkaEvent struct {
	EventID          string `json:"event_id"`
	EventType        string `json:"event_type"`
	RequestID        string `json:"request_id"`
	SenderUserID     string `json:"sender_user_id"`
	ReceiverUserID   string `json:"receiver_user_id"`
	Status           string `json:"status"`
	AggregateVersion int64  `json:"aggregate_version"`
	PartitionKey     string `json:"partition_key"`
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
	flag.StringVar(&cfg.target, "target", "127.0.0.1:10500", "contacts-service gRPC target")
	flag.StringVar(&cfg.resultDir, "result-dir", "H:\\NexusIM\\loadtest-results\\contacts-smoke", "result directory")
	flag.StringVar(&cfg.pgDSN, "pg-dsn", "", "PostgreSQL DSN")
	flag.StringVar(&brokers, "kafka-brokers", "localhost:9092", "comma-separated Kafka brokers")
	flag.StringVar(&cfg.contactTopic, "contact-topic", "im.contact.events", "contacts Kafka topic")
	flag.DurationVar(&cfg.requestTimeout, "request-timeout", 5*time.Second, "per-request timeout")
	flag.DurationVar(&cfg.waitTimeout, "wait-timeout", 10*time.Second, "wait timeout for outbox/Kafka")
	flag.DurationVar(&cfg.pollInterval, "poll-interval", 200*time.Millisecond, "poll interval")
	flag.StringVar(&cfg.tenantID, "tenant-id", "tenant-contacts-smoke", "tenant id")
	flag.StringVar(&cfg.senderUserID, "sender-user-id", "contact-sender", "sender user id")
	flag.StringVar(&cfg.receiverUserID, "receiver-user-id", "contact-receiver", "receiver user id")
	flag.StringVar(&cfg.senderDeviceID, "sender-device-id", "sender-device-1", "sender device id")
	flag.StringVar(&cfg.receiverDeviceID, "receiver-device-id", "receiver-device-1", "receiver device id")
	flag.StringVar(&cfg.scenario, "scenario", "accept", "scenario: accept or decline")
	flag.BoolVar(&cfg.cleanup, "cleanup", false, "delete tenant contacts rows before running")
	flag.Parse()
	cfg.kafkaBrokers = splitCSV(brokers)
	cfg.scenario = strings.ToLower(strings.TrimSpace(cfg.scenario))
	if cfg.scenario == "" {
		cfg.scenario = "accept"
	}
	if cfg.pollInterval <= 0 {
		cfg.pollInterval = 200 * time.Millisecond
	}
	if cfg.requestTimeout <= 0 {
		cfg.requestTimeout = 5 * time.Second
	}
	if cfg.waitTimeout <= 0 {
		cfg.waitTimeout = 10 * time.Second
	}
	return cfg
}

func run(cfg config) error {
	if err := os.MkdirAll(cfg.resultDir, 0o755); err != nil {
		return fmt.Errorf("create result dir: %w", err)
	}
	startedAt := time.Now().UTC()
	s := summary{
		Commit:         gitOutput("rev-parse", "--short", "HEAD"),
		CommitFull:     gitOutput("rev-parse", "HEAD"),
		GitStatusShort: gitOutput("status", "--short"),
		Target:         cfg.target,
		ResultDir:      cfg.resultDir,
		TenantID:       cfg.tenantID,
		SenderUserID:   cfg.senderUserID,
		ReceiverUserID: cfg.receiverUserID,
		Scenario:       cfg.scenario,
		ContactTopic:   cfg.contactTopic,
		StartedAt:      startedAt,
		LatenciesMS:    map[string]float64{},
	}
	s.GitDirty = strings.TrimSpace(s.GitStatusShort) != ""
	defer func() {
		s.FinishedAt = time.Now().UTC()
		_ = writeSummary(cfg.resultDir, s)
	}()

	if cfg.pgDSN != "" {
		pool, err := pgxpool.New(context.Background(), cfg.pgDSN)
		if err != nil {
			s.Error = err.Error()
			return fmt.Errorf("open postgres: %w", err)
		}
		defer pool.Close()
		if cfg.cleanup {
			if err := cleanupTenant(context.Background(), pool, cfg.tenantID); err != nil {
				s.Error = err.Error()
				return err
			}
		}
	}

	conn, err := grpc.NewClient(cfg.target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		s.Error = err.Error()
		return fmt.Errorf("dial contacts service: %w", err)
	}
	defer conn.Close()
	client := contactsv1.NewContactsServiceClient(conn)

	requestIDSuffix := time.Now().UTC().Format("20060102150405")
	sendResult, elapsed, err := sendContactRequest(cfg, client, requestIDSuffix)
	s.LatenciesMS["send_contact_request"] = elapsed
	if err != nil {
		s.Error = err.Error()
		return err
	}
	s.SendContactRequest = sendResult

	respondResult, elapsed, err := respondContactRequest(cfg, client, sendResult.RequestID, requestIDSuffix)
	s.LatenciesMS["respond_contact_request"] = elapsed
	if err != nil {
		s.Error = err.Error()
		return err
	}
	s.RespondContactRequest = respondResult

	senderList, elapsed, err := listContacts(cfg, client, cfg.senderUserID)
	s.LatenciesMS["list_sender_contacts"] = elapsed
	if err != nil {
		s.Error = err.Error()
		return err
	}
	s.SenderList = senderList
	receiverList, elapsed, err := listContacts(cfg, client, cfg.receiverUserID)
	s.LatenciesMS["list_receiver_contacts"] = elapsed
	if err != nil {
		s.Error = err.Error()
		return err
	}
	s.ReceiverList = receiverList

	senderState, elapsed, err := getContactState(cfg, client, cfg.senderUserID, cfg.receiverUserID)
	s.LatenciesMS["get_sender_state"] = elapsed
	if err != nil && cfg.scenario == "accept" {
		s.Error = err.Error()
		return err
	}
	s.SenderState = senderState
	receiverState, elapsed, err := getContactState(cfg, client, cfg.receiverUserID, cfg.senderUserID)
	s.LatenciesMS["get_receiver_state"] = elapsed
	if err != nil && cfg.scenario == "accept" {
		s.Error = err.Error()
		return err
	}
	s.ReceiverState = receiverState

	if cfg.pgDSN != "" {
		pool, err := pgxpool.New(context.Background(), cfg.pgDSN)
		if err != nil {
			s.Error = err.Error()
			return fmt.Errorf("open postgres for stats: %w", err)
		}
		defer pool.Close()
		outbox, err := waitOutboxPublished(context.Background(), pool, cfg, 2)
		if err != nil {
			s.Error = err.Error()
			return err
		}
		s.ContactsOutbox = outbox
	}

	events, err := readContactEvents(context.Background(), cfg, 2)
	if err != nil {
		s.Error = err.Error()
		return err
	}
	s.ContactKafkaEvents = events

	if err := validateSummary(s); err != nil {
		s.Error = err.Error()
		return err
	}
	s.Success = true
	return nil
}

func sendContactRequest(cfg config, client contactsv1.ContactsServiceClient, suffix string) (sendSummary, float64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), cfg.requestTimeout)
	defer cancel()
	begin := time.Now()
	resp, err := client.SendContactRequest(ctx, &contactsv1.SendContactRequestRequest{
		AuthContext: &contactsv1.AuthContext{
			TenantId:  cfg.tenantID,
			UserId:    cfg.senderUserID,
			DeviceId:  cfg.senderDeviceID,
			RequestId: "contact-send-" + suffix,
			TraceId:   "trace-contact-" + suffix,
		},
		TargetUserId:   cfg.receiverUserID,
		IdempotencyKey: "send-" + suffix,
		Message:        "hello from contacts smoke",
	})
	elapsed := elapsedMS(begin)
	if err != nil {
		return sendSummary{}, elapsed, fmt.Errorf("send contact request: %w", err)
	}
	return sendSummary{
		RequestID:        resp.GetRequestId(),
		Status:           resp.GetStatus().String(),
		IdempotentReplay: resp.GetIdempotentReplay(),
	}, elapsed, nil
}

func respondContactRequest(cfg config, client contactsv1.ContactsServiceClient, requestID string, suffix string) (respondSummary, float64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), cfg.requestTimeout)
	defer cancel()
	begin := time.Now()
	decision := contactsv1.ContactDecision_CONTACT_DECISION_ACCEPT
	if cfg.scenario == "decline" {
		decision = contactsv1.ContactDecision_CONTACT_DECISION_DECLINE
	}
	resp, err := client.RespondContactRequest(ctx, &contactsv1.RespondContactRequestRequest{
		AuthContext: &contactsv1.AuthContext{
			TenantId:  cfg.tenantID,
			UserId:    cfg.receiverUserID,
			DeviceId:  cfg.receiverDeviceID,
			RequestId: "contact-respond-" + suffix,
			TraceId:   "trace-contact-" + suffix,
		},
		RequestId:      requestID,
		Decision:       decision,
		IdempotencyKey: "respond-" + suffix,
	})
	elapsed := elapsedMS(begin)
	if err != nil {
		return respondSummary{}, elapsed, fmt.Errorf("respond contact request: %w", err)
	}
	return respondSummary{
		RequestID:        resp.GetRequestId(),
		Status:           resp.GetStatus().String(),
		IdempotentReplay: resp.GetIdempotentReplay(),
	}, elapsed, nil
}

func listContacts(cfg config, client contactsv1.ContactsServiceClient, userID string) (listSummary, float64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), cfg.requestTimeout)
	defer cancel()
	begin := time.Now()
	resp, err := client.ListContacts(ctx, &contactsv1.ListContactsRequest{
		AuthContext: &contactsv1.AuthContext{
			TenantId: cfg.tenantID,
			UserId:   userID,
		},
		PageSize: 10,
	})
	elapsed := elapsedMS(begin)
	if err != nil {
		return listSummary{}, elapsed, fmt.Errorf("list contacts for %s: %w", userID, err)
	}
	result := listSummary{
		OwnerUserID:    resp.GetOwnerUserId(),
		ContactCount:   len(resp.GetContacts()),
		ContactUserIDs: make([]string, 0, len(resp.GetContacts())),
	}
	for _, item := range resp.GetContacts() {
		result.ContactUserIDs = append(result.ContactUserIDs, item.GetContactUserId())
	}
	return result, elapsed, nil
}

func getContactState(cfg config, client contactsv1.ContactsServiceClient, userID string, otherUserID string) (stateSummary, float64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), cfg.requestTimeout)
	defer cancel()
	begin := time.Now()
	resp, err := client.GetContactState(ctx, &contactsv1.GetContactStateRequest{
		AuthContext: &contactsv1.AuthContext{
			TenantId: cfg.tenantID,
			UserId:   userID,
		},
		OtherUserId: otherUserID,
	})
	elapsed := elapsedMS(begin)
	if err != nil {
		return stateSummary{
			OwnerUserID:   userID,
			ContactUserID: otherUserID,
			Error:         status.Code(err).String(),
		}, elapsed, fmt.Errorf("get contact state %s -> %s: %w", userID, otherUserID, err)
	}
	return stateSummary{
		OwnerUserID:     resp.GetOwnerUserId(),
		ContactUserID:   resp.GetContactUserId(),
		Status:          resp.GetStatus().String(),
		SourceRequestID: resp.GetSourceRequestId(),
		Version:         resp.GetVersion(),
	}, elapsed, nil
}

func waitOutboxPublished(ctx context.Context, pool *pgxpool.Pool, cfg config, wantPublished int64) (outboxStats, error) {
	deadline := time.Now().Add(cfg.waitTimeout)
	for {
		stats, err := queryOutboxStats(ctx, pool, cfg.tenantID)
		if err != nil {
			return outboxStats{}, err
		}
		if stats.Published >= wantPublished && stats.Pending == 0 && stats.DLQ == 0 {
			return stats, nil
		}
		if time.Now().After(deadline) {
			return stats, fmt.Errorf("contacts outbox did not drain: %+v", stats)
		}
		time.Sleep(cfg.pollInterval)
	}
}

func queryOutboxStats(ctx context.Context, pool *pgxpool.Pool, tenantID string) (outboxStats, error) {
	var stats outboxStats
	err := pool.QueryRow(ctx, `
SELECT
    COUNT(*),
    COUNT(*) FILTER (WHERE status = 'PENDING'),
    COUNT(*) FILTER (WHERE status = 'PUBLISHED'),
    COUNT(*) FILTER (WHERE status = 'DLQ')
FROM contacts_outbox
WHERE tenant_id = $1
`, tenantID).Scan(&stats.Total, &stats.Pending, &stats.Published, &stats.DLQ)
	return stats, err
}

func readContactEvents(ctx context.Context, cfg config, want int) ([]contactKafkaEvent, error) {
	if len(cfg.kafkaBrokers) == 0 || cfg.contactTopic == "" {
		return nil, nil
	}
	reader := kafkago.NewReader(kafkago.ReaderConfig{
		Brokers:   cfg.kafkaBrokers,
		Topic:     cfg.contactTopic,
		Partition: 0,
		MinBytes:  1,
		MaxBytes:  10e6,
	})
	defer reader.Close()
	if err := reader.SetOffset(kafkago.FirstOffset); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(cfg.waitTimeout)
	events := make([]contactKafkaEvent, 0, want)
	seen := map[string]bool{}
	for len(events) < want && time.Now().Before(deadline) {
		readCtx, cancel := context.WithTimeout(ctx, cfg.pollInterval)
		message, err := reader.ReadMessage(readCtx)
		cancel()
		if err != nil {
			continue
		}
		var event contacteventsv1.ContactEvent
		if err := proto.Unmarshal(message.Value, &event); err != nil {
			continue
		}
		if event.GetTenantId() != cfg.tenantID || seen[event.GetEventId()] {
			continue
		}
		seen[event.GetEventId()] = true
		events = append(events, summarizeContactEvent(&event))
	}
	if len(events) < want {
		return events, fmt.Errorf("expected %d contact Kafka events, got %d", want, len(events))
	}
	return events, nil
}

func summarizeContactEvent(event *contacteventsv1.ContactEvent) contactKafkaEvent {
	result := contactKafkaEvent{
		EventID:          event.GetEventId(),
		EventType:        event.GetEventType(),
		AggregateVersion: event.GetAggregateVersion(),
		PartitionKey:     event.GetPartitionKey(),
	}
	switch payload := event.GetPayload().(type) {
	case *contacteventsv1.ContactEvent_RequestCreated:
		result.RequestID = payload.RequestCreated.GetRequestId()
		result.SenderUserID = payload.RequestCreated.GetSenderUserId()
		result.ReceiverUserID = payload.RequestCreated.GetReceiverUserId()
		result.Status = payload.RequestCreated.GetStatus()
	case *contacteventsv1.ContactEvent_RequestAccepted:
		result.RequestID = payload.RequestAccepted.GetRequestId()
		result.SenderUserID = payload.RequestAccepted.GetSenderUserId()
		result.ReceiverUserID = payload.RequestAccepted.GetReceiverUserId()
		result.Status = payload.RequestAccepted.GetStatus()
	case *contacteventsv1.ContactEvent_RequestDeclined:
		result.RequestID = payload.RequestDeclined.GetRequestId()
		result.SenderUserID = payload.RequestDeclined.GetSenderUserId()
		result.ReceiverUserID = payload.RequestDeclined.GetReceiverUserId()
		result.Status = payload.RequestDeclined.GetStatus()
	}
	return result
}

func validateSummary(s summary) error {
	switch s.Scenario {
	case "accept":
		return validateAcceptSummary(s)
	case "decline":
		return validateDeclineSummary(s)
	default:
		return fmt.Errorf("unsupported scenario %q", s.Scenario)
	}
}

func validateAcceptSummary(s summary) error {
	if s.SendContactRequest.Status != "CONTACT_REQUEST_STATUS_PENDING" {
		return fmt.Errorf("send status=%s, want PENDING", s.SendContactRequest.Status)
	}
	if s.RespondContactRequest.Status != "CONTACT_REQUEST_STATUS_ACCEPTED" {
		return fmt.Errorf("respond status=%s, want ACCEPTED", s.RespondContactRequest.Status)
	}
	if !contains(s.SenderList.ContactUserIDs, s.ReceiverUserID) {
		return fmt.Errorf("sender list missing receiver: %+v", s.SenderList)
	}
	if !contains(s.ReceiverList.ContactUserIDs, s.SenderUserID) {
		return fmt.Errorf("receiver list missing sender: %+v", s.ReceiverList)
	}
	if s.SenderState.Status != "CONTACT_EDGE_STATUS_ACTIVE" || s.ReceiverState.Status != "CONTACT_EDGE_STATUS_ACTIVE" {
		return fmt.Errorf("contact state not active: sender=%s receiver=%s", s.SenderState.Status, s.ReceiverState.Status)
	}
	if s.ContactsOutbox.Total > 0 && (s.ContactsOutbox.Pending != 0 || s.ContactsOutbox.DLQ != 0 || s.ContactsOutbox.Published < 2) {
		return fmt.Errorf("unexpected outbox stats: %+v", s.ContactsOutbox)
	}
	eventTypes := map[string]bool{}
	for _, event := range s.ContactKafkaEvents {
		eventTypes[event.EventType] = true
	}
	if len(s.ContactKafkaEvents) > 0 && (!eventTypes["contact.request.created.v1"] || !eventTypes["contact.request.accepted.v1"]) {
		return fmt.Errorf("missing expected contact Kafka events: %+v", s.ContactKafkaEvents)
	}
	return nil
}

func validateDeclineSummary(s summary) error {
	if s.SendContactRequest.Status != "CONTACT_REQUEST_STATUS_PENDING" {
		return fmt.Errorf("send status=%s, want PENDING", s.SendContactRequest.Status)
	}
	if s.RespondContactRequest.Status != "CONTACT_REQUEST_STATUS_DECLINED" {
		return fmt.Errorf("respond status=%s, want DECLINED", s.RespondContactRequest.Status)
	}
	if s.SenderList.ContactCount != 0 || s.ReceiverList.ContactCount != 0 {
		return fmt.Errorf("decline should not create contacts: sender=%+v receiver=%+v", s.SenderList, s.ReceiverList)
	}
	if s.SenderState.Error != codes.NotFound.String() || s.ReceiverState.Error != codes.NotFound.String() {
		return fmt.Errorf("decline should leave no contact state: sender=%+v receiver=%+v", s.SenderState, s.ReceiverState)
	}
	if s.ContactsOutbox.Total > 0 && (s.ContactsOutbox.Pending != 0 || s.ContactsOutbox.DLQ != 0 || s.ContactsOutbox.Published < 2) {
		return fmt.Errorf("unexpected outbox stats: %+v", s.ContactsOutbox)
	}
	eventTypes := map[string]bool{}
	for _, event := range s.ContactKafkaEvents {
		eventTypes[event.EventType] = true
	}
	if len(s.ContactKafkaEvents) > 0 && (!eventTypes["contact.request.created.v1"] || !eventTypes["contact.request.declined.v1"]) {
		return fmt.Errorf("missing expected contact Kafka events: %+v", s.ContactKafkaEvents)
	}
	return nil
}

func cleanupTenant(ctx context.Context, pool *pgxpool.Pool, tenantID string) error {
	statements := []string{
		`DELETE FROM contacts_outbox WHERE tenant_id = $1`,
		`DELETE FROM contact_command_idempotency WHERE tenant_id = $1`,
		`DELETE FROM contact_edges WHERE tenant_id = $1`,
		`DELETE FROM contact_requests WHERE tenant_id = $1`,
	}
	for _, statement := range statements {
		if _, err := pool.Exec(ctx, statement, tenantID); err != nil {
			return err
		}
	}
	return nil
}

func writeSummary(resultDir string, s summary) error {
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(resultDir, "contacts-summary.json"), raw, 0o644)
}

func gitOutput(args ...string) string {
	output, err := exec.Command("git", args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
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

func elapsedMS(begin time.Time) float64 {
	return float64(time.Since(begin).Microseconds()) / 1000.0
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
