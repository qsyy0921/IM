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

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	mediav1 "github.com/qsyy0921/IM/api/proto/nexusim/media/v1"
	"github.com/qsyy0921/IM/loadtest/internal/grpctls"
	mediaeventsv1 "github.com/qsyy0921/IM/schemas/kafka/media/v1"
	kafkago "github.com/segmentio/kafka-go"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

const (
	defaultResultRoot = `H:\NexusIM\loadtest-results`
	defaultSHA256     = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
)

type config struct {
	mediaTarget    string
	mediaTLS       grpctls.Config
	pgDSN          string
	resultDir      string
	requestTimeout time.Duration
	waitTimeout    time.Duration
	pollInterval   time.Duration
	kafkaBrokers   []string
	mediaTopic     string
	tenantID       string
	userID         string
	deviceID       string
	conversationID string
	messageID      string
	cleanup        bool
	applyMigration bool
}

type summary struct {
	Commit               string            `json:"commit"`
	CommitFull           string            `json:"commit_full"`
	GitDirty             bool              `json:"git_dirty"`
	GitStatusShort       string            `json:"git_status_short,omitempty"`
	ResultDir            string            `json:"result_dir"`
	MediaTarget          string            `json:"media_target"`
	MediaTLSEnabled      bool              `json:"media_tls_enabled"`
	KafkaBrokers         []string          `json:"kafka_brokers,omitempty"`
	MediaTopic           string            `json:"media_topic,omitempty"`
	TenantID             string            `json:"tenant_id"`
	UserID               string            `json:"user_id"`
	ConversationID       string            `json:"conversation_id"`
	MessageID            string            `json:"message_id"`
	StartedAt            time.Time         `json:"started_at"`
	FinishedAt           time.Time         `json:"finished_at"`
	Success              bool              `json:"success"`
	Error                string            `json:"error,omitempty"`
	AssetID              string            `json:"asset_id"`
	UploadSessionID      string            `json:"upload_session_id"`
	AssetStatus          string            `json:"asset_status"`
	ScanStatus           string            `json:"scan_status"`
	ThumbnailStatus      string            `json:"thumbnail_status"`
	TranscodeStatus      string            `json:"transcode_status"`
	UploadURLSafe        bool              `json:"upload_url_safe"`
	DownloadURLSafe      bool              `json:"download_url_safe"`
	PublicAssetSafe      bool              `json:"public_asset_safe"`
	Outbox               outboxSummary     `json:"outbox"`
	MediaKafkaEventCount int64             `json:"media_kafka_event_count,omitempty"`
	MediaKafkaEvents     []mediaKafkaEvent `json:"media_kafka_events,omitempty"`
	AccessAuditAllowed   int64             `json:"access_audit_allowed"`
}

type outboxSummary struct {
	Total     int64 `json:"total"`
	Uploaded  int64 `json:"uploaded"`
	Ready     int64 `json:"ready"`
	Pending   int64 `json:"pending"`
	Published int64 `json:"published"`
	DLQ       int64 `json:"dlq"`
}

type mediaKafkaEvent struct {
	EventID          string `json:"event_id"`
	EventType        string `json:"event_type"`
	AggregateID      string `json:"aggregate_id"`
	AggregateVersion int64  `json:"aggregate_version"`
	PartitionKey     string `json:"partition_key"`
	PayloadKind      string `json:"payload_kind"`
	Status           string `json:"status"`
}

type assetSnapshot struct {
	Status          string
	ScanStatus      string
	ThumbnailStatus string
	TranscodeStatus string
	ObjectKey       string
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
	flag.StringVar(&cfg.mediaTarget, "media-target", envOr("NEXUSIM_MEDIA_GRPC_ADDR", "127.0.0.1:10680"), "media-service gRPC target")
	registerTLSFlags("media-tls", "NEXUSIM_MEDIA_TLS", "media-service", &cfg.mediaTLS)
	flag.StringVar(&cfg.pgDSN, "pg-dsn", envOr("NEXUSIM_PG_DSN", "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable"), "PostgreSQL DSN")
	flag.StringVar(&resultRoot, "result-root", defaultResultRoot, "external result root for raw smoke output")
	flag.StringVar(&runName, "run-name", "", "run name under result-root")
	flag.DurationVar(&cfg.requestTimeout, "request-timeout", 3*time.Second, "per request timeout")
	flag.DurationVar(&cfg.waitTimeout, "wait-timeout", 15*time.Second, "wait timeout for outbox relay and Kafka readback")
	flag.DurationVar(&cfg.pollInterval, "poll-interval", 200*time.Millisecond, "poll interval for outbox relay and Kafka readback")
	flag.StringVar(&kafkaBrokers, "kafka-brokers", os.Getenv("NEXUSIM_KAFKA_BROKERS"), "Kafka brokers for optional media event readback")
	flag.StringVar(&cfg.mediaTopic, "media-events-topic", os.Getenv("NEXUSIM_MEDIA_EVENTS_TOPIC"), "Kafka topic for optional media event readback")
	flag.StringVar(&cfg.tenantID, "tenant-id", "", "tenant id; defaults to tenant derived from run name")
	flag.StringVar(&cfg.userID, "user-id", "media-user-1", "requesting user id")
	flag.StringVar(&cfg.deviceID, "device-id", "media-device-1", "requesting device id")
	flag.StringVar(&cfg.conversationID, "conversation-id", "", "conversation id; defaults to conversation derived from run name")
	flag.StringVar(&cfg.messageID, "message-id", "", "message id for download audit; defaults to message derived from run name")
	flag.BoolVar(&cfg.cleanup, "cleanup", true, "delete media rows for the smoke tenant before running")
	flag.BoolVar(&cfg.applyMigration, "apply-migration", true, "apply media migration before running")
	flag.Parse()

	if runName == "" {
		runName = "media-service-grpc-smoke-" + time.Now().Format("20060102-150405")
	}
	safeRunName := sanitizeRunName(runName)
	if cfg.tenantID == "" {
		cfg.tenantID = "tenant-" + safeRunName
	}
	if cfg.conversationID == "" {
		cfg.conversationID = "conv-" + safeRunName
	}
	if cfg.messageID == "" {
		cfg.messageID = "msg-" + safeRunName
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
		Commit:          gitOutput("rev-parse", "--short", "HEAD"),
		CommitFull:      gitOutput("rev-parse", "HEAD"),
		GitStatusShort:  gitOutput("status", "--short"),
		ResultDir:       cfg.resultDir,
		MediaTarget:     cfg.mediaTarget,
		MediaTLSEnabled: cfg.mediaTLS.Enabled(),
		KafkaBrokers:    cfg.kafkaBrokers,
		MediaTopic:      cfg.mediaTopic,
		TenantID:        cfg.tenantID,
		UserID:          cfg.userID,
		ConversationID:  cfg.conversationID,
		MessageID:       cfg.messageID,
		StartedAt:       time.Now().UTC(),
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
		if err := applyMediaMigrations(ctx, pool); err != nil {
			result.Error = "apply media migrations: " + err.Error()
			return err
		}
	}
	if cfg.cleanup {
		if err := cleanupTenant(ctx, pool, cfg.tenantID); err != nil {
			result.Error = "cleanup media rows: " + err.Error()
			return err
		}
	}

	dialOption, err := grpctls.DialOption(cfg.mediaTLS, "media-tls")
	if err != nil {
		result.Error = "configure media TLS: " + err.Error()
		return err
	}
	conn, err := grpc.NewClient("passthrough:///"+cfg.mediaTarget, dialOption)
	if err != nil {
		result.Error = "dial media-service: " + err.Error()
		return fmt.Errorf("dial media-service: %w", err)
	}
	defer conn.Close()
	client := mediav1.NewMediaServiceClient(conn)

	if err := runSmoke(ctx, cfg, pool, client, &result); err != nil {
		result.Error = err.Error()
		return err
	}
	result.Success = true
	return nil
}

func runSmoke(ctx context.Context, cfg config, pool *pgxpool.Pool, client mediav1.MediaServiceClient, result *summary) error {
	auth := &mediav1.AuthContext{
		TenantId:  cfg.tenantID,
		UserId:    cfg.userID,
		DeviceId:  cfg.deviceID,
		SessionId: "media-smoke-session",
		TraceId:   "trace-media-grpc-smoke",
		RequestId: "request-media-create",
	}
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	createResponse, err := client.CreateUploadSession(requestCtx, &mediav1.CreateUploadSessionRequest{
		AuthContext:    auth,
		ConversationId: cfg.conversationID,
		MediaKind:      mediav1.MediaKind_MEDIA_KIND_IMAGE,
		FileName:       "media-smoke.png",
		ContentType:    "image/png",
		SizeBytes:      64,
		Sha256:         defaultSHA256,
		IdempotencyKey: "media-smoke-" + randomSuffix(),
	})
	cancel()
	if err != nil {
		return fmt.Errorf("create upload session: %w", err)
	}
	result.AssetID = createResponse.GetAssetId()
	result.UploadSessionID = createResponse.GetUploadSessionId()
	result.UploadURLSafe = isURLSafe(createResponse.GetUploadUrl())
	if !result.UploadURLSafe {
		return fmt.Errorf("upload URL leaked internal object key fields: %s", createResponse.GetUploadUrl())
	}

	completeAuth := protoCloneAuth(auth)
	completeAuth.RequestId = "request-media-complete"
	requestCtx, cancel = context.WithTimeout(ctx, cfg.requestTimeout)
	completeResponse, err := client.CompleteUpload(requestCtx, &mediav1.CompleteUploadRequest{
		AuthContext:     completeAuth,
		AssetId:         createResponse.GetAssetId(),
		UploadSessionId: createResponse.GetUploadSessionId(),
		Sha256:          defaultSHA256,
		SizeBytes:       64,
	})
	cancel()
	if err != nil {
		return fmt.Errorf("complete upload: %w", err)
	}
	if completeResponse.GetAsset().GetStatus() != mediav1.MediaAssetStatus_MEDIA_ASSET_STATUS_PROCESSING &&
		completeResponse.GetAsset().GetStatus() != mediav1.MediaAssetStatus_MEDIA_ASSET_STATUS_READY {
		return fmt.Errorf("asset did not enter processing or ready state: %s", completeResponse.GetAsset().GetStatus())
	}
	result.PublicAssetSafe = protoMessageSafe(completeResponse.GetAsset())
	if !result.PublicAssetSafe {
		return fmt.Errorf("complete response leaked object key")
	}

	snapshot, err := waitAssetReady(ctx, pool, cfg, createResponse.GetAssetId())
	if err != nil {
		return err
	}
	result.AssetStatus = snapshot.Status
	result.ScanStatus = snapshot.ScanStatus
	result.ThumbnailStatus = snapshot.ThumbnailStatus
	result.TranscodeStatus = snapshot.TranscodeStatus

	assetAuth := protoCloneAuth(auth)
	assetAuth.RequestId = "request-media-get-asset"
	requestCtx, cancel = context.WithTimeout(ctx, cfg.requestTimeout)
	assetResponse, err := client.GetMediaAsset(requestCtx, &mediav1.GetMediaAssetRequest{
		AuthContext: assetAuth,
		AssetId:     createResponse.GetAssetId(),
	})
	cancel()
	if err != nil {
		return fmt.Errorf("get media asset: %w", err)
	}
	if !protoMessageSafe(assetResponse.GetAsset()) {
		return fmt.Errorf("get asset response leaked object key")
	}

	downloadAuth := protoCloneAuth(auth)
	downloadAuth.RequestId = "request-media-download"
	requestCtx, cancel = context.WithTimeout(ctx, cfg.requestTimeout)
	downloadResponse, err := client.GetMediaDownloadURL(requestCtx, &mediav1.GetMediaDownloadURLRequest{
		AuthContext:      downloadAuth,
		AssetId:          createResponse.GetAssetId(),
		ConversationId:   cfg.conversationID,
		MessageId:        cfg.messageID,
		RequestedVariant: mediav1.MediaVariant_MEDIA_VARIANT_ORIGINAL,
	})
	cancel()
	if err != nil {
		return fmt.Errorf("get media download url: %w", err)
	}
	result.DownloadURLSafe = isURLSafe(downloadResponse.GetDownloadUrl())
	if !result.DownloadURLSafe {
		return fmt.Errorf("download URL leaked internal object key fields: %s", downloadResponse.GetDownloadUrl())
	}

	if snapshot.Status != "READY" || snapshot.ScanStatus != "PASSED" {
		return fmt.Errorf("unexpected asset state: %+v", snapshot)
	}
	var outbox outboxSummary
	if cfg.kafkaEnabled() {
		outbox, err = waitOutboxPublished(ctx, pool, cfg, createResponse.GetAssetId(), snapshot.ObjectKey, 2)
	} else {
		outbox, err = readOutboxSummary(ctx, pool, cfg.tenantID, createResponse.GetAssetId(), snapshot.ObjectKey)
	}
	if err != nil {
		return err
	}
	result.Outbox = outbox
	if outbox.Uploaded != 1 || outbox.Ready != 1 || outbox.DLQ != 0 {
		return fmt.Errorf("unexpected outbox summary: %+v", outbox)
	}
	if cfg.kafkaEnabled() {
		events, err := readMediaEvents(ctx, cfg, createResponse.GetAssetId(), 2)
		if err != nil {
			return err
		}
		result.MediaKafkaEvents = events
		result.MediaKafkaEventCount = int64(len(events))
	}
	auditAllowed, err := countAccessAudit(ctx, pool, cfg.tenantID, createResponse.GetAssetId())
	if err != nil {
		return err
	}
	result.AccessAuditAllowed = auditAllowed
	if auditAllowed != 1 {
		return fmt.Errorf("expected one allowed access audit row, got %d", auditAllowed)
	}
	return nil
}

func waitAssetReady(ctx context.Context, pool *pgxpool.Pool, cfg config, assetID string) (assetSnapshot, error) {
	deadline := time.Now().Add(cfg.waitTimeout)
	for {
		snapshot, err := readAssetSnapshot(ctx, pool, cfg.tenantID, assetID)
		if err != nil {
			return assetSnapshot{}, err
		}
		if snapshot.Status == "READY" {
			return snapshot, nil
		}
		if time.Now().After(deadline) {
			return assetSnapshot{}, fmt.Errorf("asset did not become ready before timeout: %+v", snapshot)
		}
		select {
		case <-ctx.Done():
			return assetSnapshot{}, ctx.Err()
		case <-time.After(cfg.pollInterval):
		}
	}
}

func protoCloneAuth(auth *mediav1.AuthContext) *mediav1.AuthContext {
	clone := *auth
	return &clone
}

func readAssetSnapshot(ctx context.Context, pool *pgxpool.Pool, tenantID string, assetID string) (assetSnapshot, error) {
	var snapshot assetSnapshot
	err := pool.QueryRow(ctx, `
SELECT status, scan_status, thumbnail_status, transcode_status, object_key
FROM media_assets
WHERE tenant_id = $1
  AND asset_id = $2
`, tenantID, assetID).Scan(
		&snapshot.Status,
		&snapshot.ScanStatus,
		&snapshot.ThumbnailStatus,
		&snapshot.TranscodeStatus,
		&snapshot.ObjectKey,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return assetSnapshot{}, fmt.Errorf("asset row not found: %s", assetID)
	}
	if err != nil {
		return assetSnapshot{}, fmt.Errorf("read media asset: %w", err)
	}
	return snapshot, nil
}

func readOutboxSummary(ctx context.Context, pool *pgxpool.Pool, tenantID string, assetID string, objectKey string) (outboxSummary, error) {
	rows, err := pool.Query(ctx, `
SELECT event_type, status, payload_json::text
FROM media_outbox
WHERE tenant_id = $1
  AND asset_id = $2
`, tenantID, assetID)
	if err != nil {
		return outboxSummary{}, fmt.Errorf("read media outbox: %w", err)
	}
	defer rows.Close()
	var summary outboxSummary
	for rows.Next() {
		var eventType string
		var status string
		var payload string
		if err := rows.Scan(&eventType, &status, &payload); err != nil {
			return outboxSummary{}, fmt.Errorf("scan media outbox: %w", err)
		}
		summary.Total++
		if strings.Contains(payload, objectKey) ||
			strings.Contains(payload, "object_key") ||
			strings.Contains(payload, "download_url") {
			return outboxSummary{}, fmt.Errorf("media outbox leaked internal object key fields: %s", payload)
		}
		switch eventType {
		case "media.asset.uploaded.v1":
			summary.Uploaded++
		case "media.asset.ready.v1":
			summary.Ready++
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
		return outboxSummary{}, fmt.Errorf("iterate media outbox: %w", err)
	}
	return summary, nil
}

func waitOutboxPublished(ctx context.Context, pool *pgxpool.Pool, cfg config, assetID string, objectKey string, wantPublished int64) (outboxSummary, error) {
	deadline := time.Now().Add(cfg.waitTimeout)
	for {
		summary, err := readOutboxSummary(ctx, pool, cfg.tenantID, assetID, objectKey)
		if err != nil {
			return outboxSummary{}, err
		}
		if summary.Published >= wantPublished && summary.Pending == 0 && summary.DLQ == 0 {
			return summary, nil
		}
		if time.Now().After(deadline) {
			return summary, fmt.Errorf("media outbox did not drain: %+v", summary)
		}
		time.Sleep(cfg.pollInterval)
	}
}

func readMediaEvents(ctx context.Context, cfg config, assetID string, want int) ([]mediaKafkaEvent, error) {
	if !cfg.kafkaEnabled() {
		return nil, nil
	}
	reader := kafkago.NewReader(kafkago.ReaderConfig{
		Brokers:   cfg.kafkaBrokers,
		Topic:     cfg.mediaTopic,
		Partition: 0,
		MinBytes:  1,
		MaxBytes:  10e6,
	})
	defer reader.Close()
	if err := reader.SetOffset(kafkago.FirstOffset); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(cfg.waitTimeout)
	events := make([]mediaKafkaEvent, 0, want)
	seen := map[string]bool{}
	for len(events) < want && time.Now().Before(deadline) {
		readCtx, cancel := context.WithTimeout(ctx, cfg.pollInterval)
		message, err := reader.ReadMessage(readCtx)
		cancel()
		if err != nil {
			continue
		}
		var event mediaeventsv1.MediaEvent
		if err := proto.Unmarshal(message.Value, &event); err != nil {
			continue
		}
		if event.GetTenantId() != cfg.tenantID || event.GetAggregateId() != assetID || seen[event.GetEventId()] {
			continue
		}
		if !protoMessageSafe(&event) {
			return events, fmt.Errorf("media Kafka event leaked internal object key fields: %s", event.GetEventId())
		}
		seen[event.GetEventId()] = true
		events = append(events, summarizeMediaEvent(&event))
	}
	if len(events) < want {
		return events, fmt.Errorf("expected %d media Kafka events, got %d", want, len(events))
	}
	return events, nil
}

func summarizeMediaEvent(event *mediaeventsv1.MediaEvent) mediaKafkaEvent {
	result := mediaKafkaEvent{
		EventID:          event.GetEventId(),
		EventType:        event.GetEventType(),
		AggregateID:      event.GetAggregateId(),
		AggregateVersion: event.GetAggregateVersion(),
		PartitionKey:     event.GetPartitionKey(),
	}
	switch payload := event.GetPayload().(type) {
	case *mediaeventsv1.MediaEvent_AssetUploaded:
		result.PayloadKind = "asset_uploaded"
		result.Status = payload.AssetUploaded.GetStatus()
	case *mediaeventsv1.MediaEvent_AssetReady:
		result.PayloadKind = "asset_ready"
		result.Status = payload.AssetReady.GetStatus()
	case *mediaeventsv1.MediaEvent_AssetDeleted:
		result.PayloadKind = "asset_deleted"
		result.Status = payload.AssetDeleted.GetStatus()
	case *mediaeventsv1.MediaEvent_AssetQuarantined:
		result.PayloadKind = "asset_quarantined"
		result.Status = payload.AssetQuarantined.GetStatus()
	}
	return result
}

func countAccessAudit(ctx context.Context, pool *pgxpool.Pool, tenantID string, assetID string) (int64, error) {
	var count int64
	err := pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM media_access_audit
WHERE tenant_id = $1
  AND asset_id = $2
  AND decision = 'ALLOW'
`, tenantID, assetID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count media access audit: %w", err)
	}
	return count, nil
}

func applyMediaMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	dir := filepath.Join("migrations", "postgres", "media")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read media migration dir: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".sql" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read media migration %s: %w", entry.Name(), err)
		}
		if _, err := pool.Exec(ctx, string(content)); err != nil {
			return fmt.Errorf("apply media migration %s: %w", entry.Name(), err)
		}
	}
	return nil
}

func cleanupTenant(ctx context.Context, pool *pgxpool.Pool, tenantID string) error {
	queries := []string{
		`DELETE FROM media_access_audit WHERE tenant_id = $1`,
		`DELETE FROM media_outbox WHERE tenant_id = $1`,
		`DELETE FROM media_processing_jobs WHERE tenant_id = $1`,
		`DELETE FROM media_upload_sessions WHERE tenant_id = $1`,
		`DELETE FROM media_assets WHERE tenant_id = $1`,
	}
	for _, query := range queries {
		if _, err := pool.Exec(ctx, query, tenantID); err != nil {
			return fmt.Errorf("cleanup media tenant: %w", err)
		}
	}
	return nil
}

func validateConfig(cfg config) error {
	if strings.TrimSpace(cfg.mediaTarget) == "" {
		return errors.New("media-target is required")
	}
	if strings.TrimSpace(cfg.pgDSN) == "" {
		return errors.New("pg-dsn is required")
	}
	if strings.TrimSpace(cfg.tenantID) == "" || strings.TrimSpace(cfg.userID) == "" || strings.TrimSpace(cfg.conversationID) == "" {
		return errors.New("tenant-id, user-id and conversation-id are required")
	}
	if (len(cfg.kafkaBrokers) == 0) != (strings.TrimSpace(cfg.mediaTopic) == "") {
		return errors.New("kafka-brokers and media-events-topic must be provided together")
	}
	return nil
}

func (cfg config) kafkaEnabled() bool {
	return len(cfg.kafkaBrokers) > 0 && strings.TrimSpace(cfg.mediaTopic) != ""
}

func writeSummary(resultDir string, result summary) error {
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(resultDir, "media-grpc-summary.json")
	if err := os.WriteFile(path, append(encoded, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Println(string(encoded))
	fmt.Printf("summary: %s\n", path)
	return nil
}

func isURLSafe(value string) bool {
	lowered := strings.ToLower(value)
	return strings.TrimSpace(value) != "" &&
		!strings.Contains(lowered, "object_key") &&
		!strings.Contains(lowered, "object-key") &&
		!strings.Contains(lowered, "key=")
}

func protoMessageSafe(message proto.Message) bool {
	encoded, err := protojson.Marshal(message)
	return err == nil && !strings.Contains(strings.ToLower(string(encoded)), "object_key")
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
		return "media-smoke"
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

func envOr(name string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
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
