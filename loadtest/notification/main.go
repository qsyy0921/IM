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
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	notificationv1 "github.com/qsyy0921/IM/api/proto/nexusim/notification/v1"
	notificationeventsv1 "github.com/qsyy0921/IM/schemas/kafka/notification/v1"
	kafkago "github.com/segmentio/kafka-go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

const defaultResultRoot = `H:\NexusIM\loadtest-results`

type config struct {
	notificationTarget string
	pgDSN              string
	resultDir          string
	requestTimeout     time.Duration
	waitTimeout        time.Duration
	pollInterval       time.Duration
	kafkaBrokers       []string
	notificationTopic  string
	tenantID           string
	userID             string
	deviceID           string
	requesterService   string
	idempotencyKey     string
	expectDelivered    bool
	cleanup            bool
	applyMigration     bool
}

type summary struct {
	Commit                 string                   `json:"commit"`
	CommitFull             string                   `json:"commit_full"`
	GitDirty               bool                     `json:"git_dirty"`
	GitStatusShort         string                   `json:"git_status_short,omitempty"`
	ResultDir              string                   `json:"result_dir"`
	NotificationTarget     string                   `json:"notification_target"`
	KafkaBrokers           []string                 `json:"kafka_brokers,omitempty"`
	NotificationTopic      string                   `json:"notification_topic,omitempty"`
	TenantID               string                   `json:"tenant_id"`
	UserID                 string                   `json:"user_id"`
	RequesterService       string                   `json:"requester_service"`
	StartedAt              time.Time                `json:"started_at"`
	FinishedAt             time.Time                `json:"finished_at"`
	Success                bool                     `json:"success"`
	Error                  string                   `json:"error,omitempty"`
	RequestID              string                   `json:"request_id"`
	RequestStatus          string                   `json:"request_status"`
	ExpectedDelivered      bool                     `json:"expected_delivered"`
	DeliveryAttemptCount   int64                    `json:"delivery_attempt_count,omitempty"`
	Outbox                 outboxSummary            `json:"outbox"`
	NotificationEventCount int64                    `json:"notification_event_count,omitempty"`
	NotificationEvents     []notificationKafkaEvent `json:"notification_events,omitempty"`
}

type outboxSummary struct {
	Total     int64 `json:"total"`
	Accepted  int64 `json:"accepted"`
	Succeeded int64 `json:"succeeded"`
	Failed    int64 `json:"failed"`
	Dead      int64 `json:"dead_lettered"`
	Pending   int64 `json:"pending"`
	Published int64 `json:"published"`
	DLQ       int64 `json:"dlq"`
}

type notificationKafkaEvent struct {
	EventID          string `json:"event_id"`
	EventType        string `json:"event_type"`
	AggregateID      string `json:"aggregate_id"`
	AggregateVersion int64  `json:"aggregate_version"`
	PartitionKey     string `json:"partition_key"`
	PayloadKind      string `json:"payload_kind"`
	Channel          string `json:"channel"`
	Status           string `json:"status"`
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
	var resultRoot string
	var runName string
	var kafkaBrokers string
	flag.StringVar(&cfg.notificationTarget, "notification-target", envOr("NEXUSIM_NOTIFICATION_GRPC_ADDR", "127.0.0.1:10690"), "notification-service gRPC target")
	flag.StringVar(&cfg.pgDSN, "pg-dsn", envOr("NEXUSIM_PG_DSN", "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable"), "PostgreSQL DSN")
	flag.StringVar(&resultRoot, "result-root", defaultResultRoot, "external result root for raw smoke output")
	flag.StringVar(&runName, "run-name", "", "run name under result-root")
	flag.DurationVar(&cfg.requestTimeout, "request-timeout", 3*time.Second, "per request timeout")
	flag.DurationVar(&cfg.waitTimeout, "wait-timeout", 15*time.Second, "wait timeout for outbox relay and Kafka readback")
	flag.DurationVar(&cfg.pollInterval, "poll-interval", 200*time.Millisecond, "poll interval for outbox relay and Kafka readback")
	flag.StringVar(&kafkaBrokers, "kafka-brokers", os.Getenv("NEXUSIM_KAFKA_BROKERS"), "Kafka brokers for optional notification event readback")
	flag.StringVar(&cfg.notificationTopic, "notification-events-topic", os.Getenv("NEXUSIM_NOTIFICATION_EVENTS_TOPIC"), "Kafka topic for optional notification event readback")
	flag.StringVar(&cfg.tenantID, "tenant-id", "", "tenant id; defaults to tenant derived from run name")
	flag.StringVar(&cfg.userID, "user-id", "notification-user-1", "requesting user id")
	flag.StringVar(&cfg.deviceID, "device-id", "notification-device-1", "requesting device id")
	flag.StringVar(&cfg.requesterService, "requester-service", "identity-service", "requester service")
	flag.StringVar(&cfg.idempotencyKey, "idempotency-key", "", "idempotency key; defaults to key derived from run name")
	flag.BoolVar(&cfg.expectDelivered, "expect-delivered", false, "wait for delivery worker to mark the request delivered")
	flag.BoolVar(&cfg.cleanup, "cleanup", true, "delete notification rows for the smoke tenant before running")
	flag.BoolVar(&cfg.applyMigration, "apply-migration", true, "apply notification migrations before running")
	flag.Parse()

	if runName == "" {
		runName = "notification-service-outbox-relay-smoke-" + time.Now().Format("20060102-150405")
	}
	safeRunName := sanitizeRunName(runName)
	if cfg.tenantID == "" {
		cfg.tenantID = "tenant-" + safeRunName
	}
	if cfg.idempotencyKey == "" {
		cfg.idempotencyKey = "idem-" + safeRunName
	}
	if cfg.requestTimeout <= 0 {
		cfg.requestTimeout = 3 * time.Second
	}
	if cfg.waitTimeout <= 0 {
		cfg.waitTimeout = 15 * time.Second
	}
	if cfg.pollInterval <= 0 {
		cfg.pollInterval = 200 * time.Millisecond
	}
	cfg.kafkaBrokers = splitCSV(kafkaBrokers)
	cfg.resultDir = filepath.Join(resultRoot, runName)
	return cfg
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
		Commit:             gitOutput("rev-parse", "--short", "HEAD"),
		CommitFull:         gitOutput("rev-parse", "HEAD"),
		GitStatusShort:     gitOutput("status", "--short"),
		ResultDir:          cfg.resultDir,
		NotificationTarget: cfg.notificationTarget,
		KafkaBrokers:       cfg.kafkaBrokers,
		NotificationTopic:  cfg.notificationTopic,
		TenantID:           cfg.tenantID,
		UserID:             cfg.userID,
		RequesterService:   cfg.requesterService,
		StartedAt:          time.Now().UTC(),
		ExpectedDelivered:  cfg.expectDelivered,
	}
	result.GitDirty = strings.TrimSpace(result.GitStatusShort) != ""
	defer func() {
		result.FinishedAt = time.Now().UTC()
		_ = writeSummary(cfg.resultDir, result)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), cfg.requestTimeout+cfg.waitTimeout+10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, cfg.pgDSN)
	if err != nil {
		result.Error = "open postgres: " + err.Error()
		return fmt.Errorf("open postgres: %w", err)
	}
	defer pool.Close()
	if cfg.applyMigration {
		if err := applyNotificationMigrations(ctx, pool); err != nil {
			result.Error = "apply migrations: " + err.Error()
			return err
		}
	}
	if cfg.cleanup {
		if err := cleanupTenant(ctx, pool, cfg.tenantID); err != nil {
			result.Error = "cleanup tenant: " + err.Error()
			return err
		}
	}

	conn, err := grpc.DialContext(ctx, cfg.notificationTarget, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		result.Error = "dial notification-service: " + err.Error()
		return fmt.Errorf("dial notification-service: %w", err)
	}
	defer conn.Close()
	client := notificationv1.NewNotificationServiceClient(conn)

	requestCtx, requestCancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer requestCancel()
	response, err := client.CreateNotificationRequest(requestCtx, &notificationv1.CreateNotificationRequestRequest{
		AuthContext: &notificationv1.AuthContext{
			TenantId:  cfg.tenantID,
			UserId:    cfg.userID,
			DeviceId:  cfg.deviceID,
			TraceId:   "trace-" + cfg.idempotencyKey,
			RequestId: "request-" + cfg.idempotencyKey,
		},
		RequesterService:      cfg.requesterService,
		RequesterUserId:       cfg.userID,
		Channel:               notificationv1.NotificationChannel_NOTIFICATION_CHANNEL_EMAIL,
		RecipientRef:          "user:" + cfg.userID,
		DestinationRef:        cfg.userID + "@example.com",
		DestinationMasked:     "n***@example.com",
		TemplateKey:           "identity.challenge",
		TemplateVersion:       "v1",
		Locale:                "zh-CN",
		Priority:              notificationv1.NotificationPriority_NOTIFICATION_PRIORITY_NORMAL,
		IdempotencyKey:        cfg.idempotencyKey,
		TemplateVariablesJson: `{"purpose":"smoke"}`,
		CorrelationId:         "corr-" + cfg.idempotencyKey,
		CausationId:           "cause-" + cfg.idempotencyKey,
		TraceId:               "trace-" + cfg.idempotencyKey,
	})
	if err != nil {
		result.Error = "create notification request: " + err.Error()
		return fmt.Errorf("create notification request: %w", err)
	}
	result.RequestID = response.GetRequestId()
	result.RequestStatus = response.GetStatus().String()
	if strings.TrimSpace(result.RequestID) == "" || response.GetStatus() != notificationv1.NotificationRequestStatus_NOTIFICATION_REQUEST_STATUS_ACCEPTED {
		result.Error = fmt.Sprintf("unexpected create response request_id=%q status=%s", result.RequestID, response.GetStatus())
		return errors.New(result.Error)
	}

	wantPublished := int64(1)
	wantEvents := 1
	if cfg.expectDelivered {
		wantPublished = 2
		wantEvents = 2
		status, attemptCount, err := waitNotificationDelivered(ctx, pool, cfg, result.RequestID)
		if err != nil {
			result.Error = err.Error()
			return err
		}
		result.RequestStatus = status
		result.DeliveryAttemptCount = attemptCount
	}

	var outbox outboxSummary
	if cfg.kafkaEnabled() {
		outbox, err = waitOutboxPublished(ctx, pool, cfg, result.RequestID, wantPublished)
	} else {
		outbox, err = readOutboxSummary(ctx, pool, cfg.tenantID, result.RequestID)
	}
	if err != nil {
		result.Error = err.Error()
		return err
	}
	result.Outbox = outbox
	if outbox.Accepted != 1 || outbox.DLQ != 0 {
		result.Error = fmt.Sprintf("unexpected outbox summary: %+v", outbox)
		return errors.New(result.Error)
	}
	if cfg.expectDelivered && outbox.Succeeded != 1 {
		result.Error = fmt.Sprintf("expected one delivery succeeded event, got outbox summary: %+v", outbox)
		return errors.New(result.Error)
	}
	if cfg.kafkaEnabled() {
		events, err := readNotificationEvents(ctx, cfg, result.RequestID, wantEvents)
		if err != nil {
			result.Error = err.Error()
			return err
		}
		result.NotificationEvents = events
		result.NotificationEventCount = int64(len(events))
	}
	result.Success = true
	return nil
}

func readOutboxSummary(ctx context.Context, pool *pgxpool.Pool, tenantID string, requestID string) (outboxSummary, error) {
	rows, err := pool.Query(ctx, `
SELECT event_type, status, payload_json::text
FROM notification_outbox
WHERE tenant_id = $1
  AND request_id = $2
`, tenantID, requestID)
	if err != nil {
		return outboxSummary{}, fmt.Errorf("read notification outbox: %w", err)
	}
	defer rows.Close()
	var summary outboxSummary
	for rows.Next() {
		var eventType string
		var status string
		var payload string
		if err := rows.Scan(&eventType, &status, &payload); err != nil {
			return outboxSummary{}, fmt.Errorf("scan notification outbox: %w", err)
		}
		summary.Total++
		if !payloadSafe(payload) {
			return outboxSummary{}, fmt.Errorf("notification outbox leaked internal field: %s", payload)
		}
		switch eventType {
		case "notification.request.accepted.v1":
			summary.Accepted++
		case "notification.delivery.succeeded.v1":
			summary.Succeeded++
		case "notification.delivery.failed.v1":
			summary.Failed++
		case "notification.delivery.dead_lettered.v1":
			summary.Dead++
		}
		switch status {
		case "PENDING":
			summary.Pending++
		case "PUBLISHED":
			summary.Published++
		case "DLQ":
			summary.DLQ++
		}
	}
	if err := rows.Err(); err != nil {
		return outboxSummary{}, fmt.Errorf("iterate notification outbox: %w", err)
	}
	return summary, nil
}

func waitNotificationDelivered(ctx context.Context, pool *pgxpool.Pool, cfg config, requestID string) (string, int64, error) {
	deadline := time.Now().Add(cfg.waitTimeout)
	for {
		status, attemptCount, err := readNotificationRequestState(ctx, pool, cfg.tenantID, requestID)
		if err != nil {
			return "", 0, err
		}
		if status == "DELIVERED" {
			return status, attemptCount, nil
		}
		if status == "DLQ" || status == "CANCELED" {
			return status, attemptCount, fmt.Errorf("notification request reached terminal non-delivered status %s", status)
		}
		if time.Now().After(deadline) {
			return status, attemptCount, fmt.Errorf("notification request was not delivered before timeout: status=%s attempts=%d", status, attemptCount)
		}
		time.Sleep(cfg.pollInterval)
	}
}

func readNotificationRequestState(ctx context.Context, pool *pgxpool.Pool, tenantID string, requestID string) (string, int64, error) {
	var status string
	var attemptCount int64
	err := pool.QueryRow(ctx, `
SELECT status, attempt_count
FROM notification_requests
WHERE tenant_id = $1
  AND request_id = $2
`, tenantID, requestID).Scan(&status, &attemptCount)
	if err != nil {
		return "", 0, fmt.Errorf("read notification request state: %w", err)
	}
	return status, attemptCount, nil
}

func waitOutboxPublished(ctx context.Context, pool *pgxpool.Pool, cfg config, requestID string, wantPublished int64) (outboxSummary, error) {
	deadline := time.Now().Add(cfg.waitTimeout)
	for {
		summary, err := readOutboxSummary(ctx, pool, cfg.tenantID, requestID)
		if err != nil {
			return outboxSummary{}, err
		}
		if summary.Published >= wantPublished && summary.Pending == 0 && summary.DLQ == 0 {
			return summary, nil
		}
		if time.Now().After(deadline) {
			return summary, fmt.Errorf("notification outbox did not drain: %+v", summary)
		}
		time.Sleep(cfg.pollInterval)
	}
}

func readNotificationEvents(ctx context.Context, cfg config, requestID string, want int) ([]notificationKafkaEvent, error) {
	if !cfg.kafkaEnabled() {
		return nil, nil
	}
	reader := kafkago.NewReader(kafkago.ReaderConfig{
		Brokers:   cfg.kafkaBrokers,
		Topic:     cfg.notificationTopic,
		Partition: 0,
		MinBytes:  1,
		MaxBytes:  10e6,
	})
	defer reader.Close()
	if err := reader.SetOffset(kafkago.FirstOffset); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(cfg.waitTimeout)
	events := make([]notificationKafkaEvent, 0, want)
	seen := map[string]bool{}
	for len(events) < want && time.Now().Before(deadline) {
		readCtx, cancel := context.WithTimeout(ctx, cfg.pollInterval)
		message, err := reader.ReadMessage(readCtx)
		cancel()
		if err != nil {
			continue
		}
		var event notificationeventsv1.NotificationEvent
		if err := proto.Unmarshal(message.Value, &event); err != nil {
			continue
		}
		if event.GetTenantId() != cfg.tenantID || event.GetAggregateId() != requestID || seen[event.GetEventId()] {
			continue
		}
		if !protoMessageSafe(&event) {
			return events, fmt.Errorf("notification Kafka event leaked internal field: %s", event.GetEventId())
		}
		seen[event.GetEventId()] = true
		events = append(events, summarizeNotificationEvent(&event))
	}
	if len(events) < want {
		return events, fmt.Errorf("expected %d notification Kafka events, got %d", want, len(events))
	}
	return events, nil
}

func summarizeNotificationEvent(event *notificationeventsv1.NotificationEvent) notificationKafkaEvent {
	result := notificationKafkaEvent{
		EventID:          event.GetEventId(),
		EventType:        event.GetEventType(),
		AggregateID:      event.GetAggregateId(),
		AggregateVersion: event.GetAggregateVersion(),
		PartitionKey:     event.GetPartitionKey(),
	}
	switch payload := event.GetPayload().(type) {
	case *notificationeventsv1.NotificationEvent_RequestAccepted:
		result.PayloadKind = "request_accepted"
		result.Channel = payload.RequestAccepted.GetChannel()
		result.Status = payload.RequestAccepted.GetStatus()
	case *notificationeventsv1.NotificationEvent_DeliverySucceeded:
		result.PayloadKind = "delivery_succeeded"
		result.Channel = payload.DeliverySucceeded.GetChannel()
		result.Status = "DELIVERED"
	case *notificationeventsv1.NotificationEvent_DeliveryFailed:
		result.PayloadKind = "delivery_failed"
		result.Channel = payload.DeliveryFailed.GetChannel()
		result.Status = "RETRY_WAIT"
	case *notificationeventsv1.NotificationEvent_DeliveryDeadLettered:
		result.PayloadKind = "delivery_dead_lettered"
		result.Channel = payload.DeliveryDeadLettered.GetChannel()
		result.Status = "DLQ"
	}
	return result
}

func applyNotificationMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	dir := filepath.Join("migrations", "postgres", "notification")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read notification migration dir: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".sql" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read notification migration %s: %w", entry.Name(), err)
		}
		if _, err := pool.Exec(ctx, string(content)); err != nil {
			return fmt.Errorf("apply notification migration %s: %w", entry.Name(), err)
		}
	}
	return nil
}

func cleanupTenant(ctx context.Context, pool *pgxpool.Pool, tenantID string) error {
	queries := []string{
		`DELETE FROM notification_delivery_attempts WHERE tenant_id = $1`,
		`DELETE FROM notification_outbox WHERE tenant_id = $1`,
		`DELETE FROM notification_suppressions WHERE tenant_id = $1`,
		`DELETE FROM notification_requests WHERE tenant_id = $1`,
	}
	for _, query := range queries {
		if _, err := pool.Exec(ctx, query, tenantID); err != nil {
			return fmt.Errorf("cleanup notification tenant: %w", err)
		}
	}
	return nil
}

func validateConfig(cfg config) error {
	if strings.TrimSpace(cfg.notificationTarget) == "" {
		return errors.New("notification-target is required")
	}
	if strings.TrimSpace(cfg.pgDSN) == "" {
		return errors.New("pg-dsn is required")
	}
	if strings.TrimSpace(cfg.tenantID) == "" || strings.TrimSpace(cfg.userID) == "" || strings.TrimSpace(cfg.requesterService) == "" {
		return errors.New("tenant-id, user-id and requester-service are required")
	}
	if (len(cfg.kafkaBrokers) == 0) != (strings.TrimSpace(cfg.notificationTopic) == "") {
		return errors.New("kafka-brokers and notification-events-topic must be provided together")
	}
	return nil
}

func (cfg config) kafkaEnabled() bool {
	return len(cfg.kafkaBrokers) > 0 && strings.TrimSpace(cfg.notificationTopic) != ""
}

func writeSummary(resultDir string, result summary) error {
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(resultDir, "notification-outbox-relay-summary.json")
	if err := os.WriteFile(path, append(encoded, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Println(string(encoded))
	fmt.Printf("summary: %s\n", path)
	return nil
}

func payloadSafe(value string) bool {
	lowered := strings.ToLower(value)
	for _, marker := range forbiddenMarkers() {
		if strings.Contains(lowered, marker) {
			return false
		}
	}
	return true
}

func protoMessageSafe(message proto.Message) bool {
	encoded, err := protojson.Marshal(message)
	return err == nil && payloadSafe(string(encoded))
}

func forbiddenMarkers() []string {
	return []string{
		"destination_ref",
		"destination_hash",
		"secret_payload",
		"provider_body",
		"provider_response",
		"authorization",
		"smtp_transcript",
		"reset_token",
		"challenge_code",
		"totp",
		"recovery_code",
	}
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
		return "notification-smoke"
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

func envOr(name string, defaultValue string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return defaultValue
	}
	return value
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			items = append(items, part)
		}
	}
	return items
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
