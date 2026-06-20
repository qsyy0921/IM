package main

import (
	"context"
	"crypto/sha256"
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
	vectorv1 "github.com/qsyy0921/IM/api/proto/nexusim/vector/v1"
	vectoreventsv1 "github.com/qsyy0921/IM/schemas/kafka/vector/v1"
	kafkago "github.com/segmentio/kafka-go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

const defaultResultRoot = `H:\NexusIM\loadtest-results`

type config struct {
	vectorTarget       string
	pgDSN              string
	resultDir          string
	requestTimeout     time.Duration
	waitTimeout        time.Duration
	pollInterval       time.Duration
	kafkaBrokers       []string
	vectorEventsTopic  string
	tenantID           string
	userID             string
	idempotencyKey     string
	cleanup            bool
	applyMigration     bool
	expectRebuildDone  bool
	sourceService      string
	collectionType     string
	embeddingModelRef  string
	visibilityScope    string
	policyVersion      string
	dataClass          string
	sourceID           string
	sourceVersion      int64
	sourceRefHash      string
	sourceHash         string
	chunkHash          string
	embeddingVectorRef string
	traceID            string
}

type summary struct {
	Commit                 string             `json:"commit"`
	CommitFull             string             `json:"commit_full"`
	GitDirty               bool               `json:"git_dirty"`
	GitStatusShort         string             `json:"git_status_short,omitempty"`
	ResultDir              string             `json:"result_dir"`
	VectorTarget           string             `json:"vector_target"`
	KafkaBrokers           []string           `json:"kafka_brokers,omitempty"`
	VectorEventsTopic      string             `json:"vector_events_topic,omitempty"`
	TenantID               string             `json:"tenant_id"`
	UserID                 string             `json:"user_id"`
	StartedAt              time.Time          `json:"started_at"`
	FinishedAt             time.Time          `json:"finished_at"`
	Success                bool               `json:"success"`
	Error                  string             `json:"error,omitempty"`
	VectorItemID           string             `json:"vector_item_id"`
	VectorItemRefHash      string             `json:"vector_item_ref_hash"`
	UpsertJobID            string             `json:"upsert_job_id"`
	RebuildJobID           string             `json:"rebuild_job_id"`
	RebuildJobRefHash      string             `json:"rebuild_job_ref_hash"`
	RebuildCheckpoint      string             `json:"rebuild_checkpoint_status"`
	TombstoneJobID         string             `json:"tombstone_job_id"`
	TombstoneID            string             `json:"tombstone_id"`
	SearchBeforeCount      int                `json:"search_before_count"`
	SearchAfterCount       int                `json:"search_after_count"`
	Outbox                 outboxSummary      `json:"outbox"`
	RebuildOutbox          *outboxSummary     `json:"rebuild_outbox,omitempty"`
	VectorKafkaEventCount  int                `json:"vector_kafka_event_count,omitempty"`
	VectorKafkaEvents      []vectorKafkaEvent `json:"vector_kafka_events,omitempty"`
	RebuildKafkaEventCount int                `json:"rebuild_kafka_event_count,omitempty"`
	RebuildKafkaEvents     []vectorKafkaEvent `json:"rebuild_kafka_events,omitempty"`
}

type outboxSummary struct {
	Total            int64 `json:"total"`
	Indexed          int64 `json:"indexed"`
	Tombstoned       int64 `json:"tombstoned"`
	RebuildStarted   int64 `json:"rebuild_started"`
	RebuildCompleted int64 `json:"rebuild_completed"`
	Pending          int64 `json:"pending"`
	Published        int64 `json:"published"`
	DLQ              int64 `json:"dlq"`
}

type vectorKafkaEvent struct {
	EventID          string `json:"event_id"`
	EventType        string `json:"event_type"`
	AggregateID      string `json:"aggregate_id"`
	AggregateVersion int64  `json:"aggregate_version"`
	PartitionKey     string `json:"partition_key"`
	PayloadKind      string `json:"payload_kind"`
	CollectionType   string `json:"collection_type"`
	TombstoneStatus  string `json:"tombstone_status"`
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
	flag.StringVar(&cfg.vectorTarget, "vector-target", envOr("NEXUSIM_VECTOR_INDEX_GRPC_ADDR", "127.0.0.1:10760"), "vector-index-service gRPC target")
	flag.StringVar(&cfg.pgDSN, "pg-dsn", envOr("NEXUSIM_PG_DSN", "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable"), "PostgreSQL DSN")
	flag.StringVar(&resultRoot, "result-root", defaultResultRoot, "external result root for raw smoke output")
	flag.StringVar(&runName, "run-name", "", "run name under result-root")
	flag.DurationVar(&cfg.requestTimeout, "request-timeout", 3*time.Second, "per request timeout")
	flag.DurationVar(&cfg.waitTimeout, "wait-timeout", 15*time.Second, "wait timeout for outbox relay and Kafka readback")
	flag.DurationVar(&cfg.pollInterval, "poll-interval", 200*time.Millisecond, "poll interval for outbox relay and Kafka readback")
	flag.StringVar(&kafkaBrokers, "kafka-brokers", os.Getenv("NEXUSIM_KAFKA_BROKERS"), "Kafka brokers for optional vector event readback")
	flag.StringVar(&cfg.vectorEventsTopic, "vector-events-topic", envOr("NEXUSIM_VECTOR_EVENTS_TOPIC", "im.vector.events"), "Kafka topic for optional vector event readback")
	flag.StringVar(&cfg.tenantID, "tenant-id", "", "tenant id; defaults to tenant derived from run name")
	flag.StringVar(&cfg.userID, "user-id", "vector-smoke-user", "requesting user id")
	flag.StringVar(&cfg.sourceService, "source-service", "knowledge-ingestion-service", "source service")
	flag.StringVar(&cfg.collectionType, "collection-type", "KNOWLEDGE_CHUNK", "vector collection type")
	flag.StringVar(&cfg.embeddingModelRef, "embedding-model-ref", "embedding-model-smoke-v1", "low-sensitive embedding model ref")
	flag.StringVar(&cfg.visibilityScope, "visibility-scope", "conversation:vector-smoke", "low-sensitive visibility scope")
	flag.StringVar(&cfg.policyVersion, "policy-version", "policy-vector-smoke-v1", "low-sensitive policy version")
	flag.StringVar(&cfg.dataClass, "data-class", "INTERNAL", "low-sensitive data class")
	flag.StringVar(&cfg.idempotencyKey, "idempotency-key", "", "idempotency key; defaults to key derived from run name")
	flag.BoolVar(&cfg.cleanup, "cleanup", true, "delete vector rows for the smoke tenant before running")
	flag.BoolVar(&cfg.applyMigration, "apply-migration", true, "apply vector-index migrations before running")
	flag.BoolVar(&cfg.expectRebuildDone, "expect-rebuild-completed", envBool("NEXUSIM_VECTOR_EXPECT_REBUILD_COMPLETED", false), "wait for rebuild-worker to complete the rebuild checkpoint")
	flag.Parse()

	if runName == "" {
		runName = "vector-index-outbox-relay-smoke-" + time.Now().Format("20060102-150405")
	}
	safeRunName := sanitizeRunName(runName)
	if cfg.tenantID == "" {
		cfg.tenantID = "tenant-" + safeRunName
	}
	if cfg.idempotencyKey == "" {
		cfg.idempotencyKey = "idem-" + safeRunName
	}
	cfg.sourceID = "source-" + safeRunName
	cfg.sourceVersion = 1
	cfg.sourceRefHash = hashRef(cfg.tenantID + "|source-ref|" + safeRunName)
	cfg.sourceHash = hashRef(cfg.tenantID + "|source|" + safeRunName)
	cfg.chunkHash = hashRef(cfg.tenantID + "|chunk|" + safeRunName)
	cfg.embeddingVectorRef = hashRef(cfg.tenantID + "|embedding-vector|" + safeRunName)
	cfg.traceID = "trace-" + safeRunName
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
		Commit:            gitOutput("rev-parse", "--short", "HEAD"),
		CommitFull:        gitOutput("rev-parse", "HEAD"),
		GitStatusShort:    gitOutput("status", "--short"),
		ResultDir:         cfg.resultDir,
		VectorTarget:      cfg.vectorTarget,
		KafkaBrokers:      cfg.kafkaBrokers,
		VectorEventsTopic: cfg.vectorEventsTopic,
		TenantID:          cfg.tenantID,
		UserID:            cfg.userID,
		StartedAt:         time.Now().UTC(),
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
		if err := applyVectorMigrations(ctx, pool); err != nil {
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

	conn, err := grpc.DialContext(ctx, cfg.vectorTarget, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		result.Error = "dial vector-index-service: " + err.Error()
		return fmt.Errorf("dial vector-index-service: %w", err)
	}
	defer conn.Close()
	client := vectorv1.NewVectorIndexServiceClient(conn)

	upsert, err := upsertVectorItem(ctx, client, cfg)
	if err != nil {
		result.Error = "upsert vector item: " + err.Error()
		return err
	}
	result.VectorItemID = upsert.GetItem().GetVectorItemId()
	result.UpsertJobID = upsert.GetJob().GetJobId()
	result.VectorItemRefHash = hashRef(result.VectorItemID)
	if result.VectorItemID == "" || result.UpsertJobID == "" {
		result.Error = "upsert response missing item or job id"
		return errors.New(result.Error)
	}

	searchBefore, err := searchVectors(ctx, client, cfg)
	if err != nil {
		result.Error = "search vectors before tombstone: " + err.Error()
		return err
	}
	result.SearchBeforeCount = len(searchBefore.GetResults())
	if result.SearchBeforeCount != 1 {
		result.Error = fmt.Sprintf("expected one search result before tombstone, got %d", result.SearchBeforeCount)
		return errors.New(result.Error)
	}

	rebuild, err := requestVectorRebuild(ctx, client, cfg)
	if err != nil {
		result.Error = "request vector rebuild: " + err.Error()
		return err
	}
	result.RebuildJobID = rebuild.GetJob().GetJobId()
	result.RebuildJobRefHash = hashRef(result.RebuildJobID)
	result.RebuildCheckpoint = rebuild.GetCheckpoint().GetStatus()
	if result.RebuildJobID == "" || rebuild.GetJob().GetStatus() != "PENDING" || result.RebuildCheckpoint != "PENDING" {
		result.Error = fmt.Sprintf("unexpected rebuild response: job=%+v checkpoint=%+v", rebuild.GetJob(), rebuild.GetCheckpoint())
		return errors.New(result.Error)
	}
	loadedRebuild, err := getVectorJob(ctx, client, cfg, result.RebuildJobID)
	if err != nil {
		result.Error = "get rebuild job: " + err.Error()
		return err
	}
	if loadedRebuild.GetJob().GetJobType() != "REBUILD" {
		result.Error = fmt.Sprintf("unexpected loaded rebuild job: %+v", loadedRebuild.GetJob())
		return errors.New(result.Error)
	}
	if cfg.expectRebuildDone {
		completedRebuild, err := waitVectorJobStatus(ctx, client, cfg, result.RebuildJobID, "INDEXED")
		if err != nil {
			result.Error = err.Error()
			return err
		}
		result.RebuildCheckpoint, err = readRebuildCheckpointStatus(ctx, pool, cfg, result.RebuildJobID)
		if err != nil {
			result.Error = err.Error()
			return err
		}
		if completedRebuild.GetJob().GetStatus() != "INDEXED" || result.RebuildCheckpoint != "COMPLETED" {
			result.Error = fmt.Sprintf("unexpected completed rebuild: job=%+v checkpoint=%s", completedRebuild.GetJob(), result.RebuildCheckpoint)
			return errors.New(result.Error)
		}
		var rebuildOutbox outboxSummary
		if cfg.kafkaEnabled() {
			rebuildOutbox, err = waitOutboxPublished(ctx, pool, cfg, result.RebuildJobRefHash, 2)
		} else {
			rebuildOutbox, err = readOutboxSummary(ctx, pool, cfg.tenantID, result.RebuildJobRefHash)
		}
		if err != nil {
			result.Error = err.Error()
			return err
		}
		result.RebuildOutbox = &rebuildOutbox
		if rebuildOutbox.RebuildStarted != 1 || rebuildOutbox.RebuildCompleted != 1 || rebuildOutbox.DLQ != 0 {
			result.Error = fmt.Sprintf("unexpected rebuild outbox summary: %+v", rebuildOutbox)
			return errors.New(result.Error)
		}
		if cfg.kafkaEnabled() {
			events, err := readVectorEvents(ctx, cfg, result.RebuildJobRefHash, 2)
			if err != nil {
				result.Error = err.Error()
				return err
			}
			result.RebuildKafkaEvents = events
			result.RebuildKafkaEventCount = len(events)
		}
	} else if loadedRebuild.GetJob().GetStatus() != "PENDING" {
		result.Error = fmt.Sprintf("unexpected loaded rebuild job: %+v", loadedRebuild.GetJob())
		return errors.New(result.Error)
	}

	tombstone, err := tombstoneVectorItem(ctx, client, cfg, result.VectorItemID)
	if err != nil {
		result.Error = "tombstone vector item: " + err.Error()
		return err
	}
	result.TombstoneJobID = tombstone.GetJob().GetJobId()
	result.TombstoneID = tombstone.GetTombstoneId()
	if result.TombstoneJobID == "" || result.TombstoneID == "" {
		result.Error = "tombstone response missing job or tombstone id"
		return errors.New(result.Error)
	}

	searchAfter, err := searchVectors(ctx, client, cfg)
	if err != nil {
		result.Error = "search vectors after tombstone: " + err.Error()
		return err
	}
	result.SearchAfterCount = len(searchAfter.GetResults())
	if result.SearchAfterCount != 0 {
		result.Error = fmt.Sprintf("expected zero search results after tombstone, got %d", result.SearchAfterCount)
		return errors.New(result.Error)
	}

	var outbox outboxSummary
	if cfg.kafkaEnabled() {
		outbox, err = waitOutboxPublished(ctx, pool, cfg, result.VectorItemRefHash, 2)
	} else {
		outbox, err = readOutboxSummary(ctx, pool, cfg.tenantID, result.VectorItemRefHash)
	}
	if err != nil {
		result.Error = err.Error()
		return err
	}
	result.Outbox = outbox
	if outbox.Indexed != 1 || outbox.Tombstoned != 1 || outbox.DLQ != 0 {
		result.Error = fmt.Sprintf("unexpected vector outbox summary: %+v", outbox)
		return errors.New(result.Error)
	}
	if cfg.kafkaEnabled() {
		events, err := readVectorEvents(ctx, cfg, result.VectorItemRefHash, 2)
		if err != nil {
			result.Error = err.Error()
			return err
		}
		result.VectorKafkaEvents = events
		result.VectorKafkaEventCount = len(events)
	}
	result.Success = true
	return nil
}

func upsertVectorItem(ctx context.Context, client vectorv1.VectorIndexServiceClient, cfg config) (*vectorv1.UpsertVectorItemResponse, error) {
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	return client.UpsertVectorItem(requestCtx, &vectorv1.UpsertVectorItemRequest{
		AuthContext:         authContext(cfg, cfg.sourceService, "vector-smoke-upsert"),
		SourceService:       cfg.sourceService,
		CollectionType:      cfg.collectionType,
		SourceRefHash:       cfg.sourceRefHash,
		SourceId:            cfg.sourceID,
		SourceVersion:       cfg.sourceVersion,
		SourceHash:          cfg.sourceHash,
		ChunkHash:           cfg.chunkHash,
		EmbeddingModelRef:   cfg.embeddingModelRef,
		EmbeddingVectorHash: cfg.embeddingVectorRef,
		Dimension:           3,
		VisibilityScope:     cfg.visibilityScope,
		VisibilityVersion:   1,
		PolicyVersion:       cfg.policyVersion,
		DataClass:           cfg.dataClass,
		DeleteProofId:       "",
		RetentionPolicyRef:  "retention-vector-smoke-v1",
		IdempotencyKey:      cfg.idempotencyKey + "-upsert",
		CorrelationId:       cfg.idempotencyKey,
		CausationId:         cfg.idempotencyKey,
		TraceId:             cfg.traceID,
	})
}

func requestVectorRebuild(ctx context.Context, client vectorv1.VectorIndexServiceClient, cfg config) (*vectorv1.RequestVectorRebuildResponse, error) {
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	return client.RequestVectorRebuild(requestCtx, &vectorv1.RequestVectorRebuildRequest{
		AuthContext:       authContext(cfg, "vector-index-service", "vector-smoke-rebuild"),
		CollectionType:    cfg.collectionType,
		EmbeddingModelRef: cfg.embeddingModelRef,
		Dimension:         3,
		SourceService:     cfg.sourceService,
		PartitionKey:      rebuildPartitionKey(cfg),
		CursorValue:       "cursor:start",
		IdempotencyKey:    cfg.idempotencyKey + "-rebuild",
		CorrelationId:     cfg.idempotencyKey,
		CausationId:       cfg.idempotencyKey,
		TraceId:           cfg.traceID,
	})
}

func rebuildPartitionKey(cfg config) string {
	return cfg.sourceService + ":" + cfg.tenantID
}

func getVectorJob(ctx context.Context, client vectorv1.VectorIndexServiceClient, cfg config, jobID string) (*vectorv1.GetVectorIndexJobResponse, error) {
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	return client.GetVectorIndexJob(requestCtx, &vectorv1.GetVectorIndexJobRequest{
		AuthContext: authContext(cfg, "vector-index-service", "vector-smoke-get-job"),
		JobId:       jobID,
	})
}

func waitVectorJobStatus(ctx context.Context, client vectorv1.VectorIndexServiceClient, cfg config, jobID string, wantStatus string) (*vectorv1.GetVectorIndexJobResponse, error) {
	deadline := time.Now().Add(cfg.waitTimeout)
	var last *vectorv1.GetVectorIndexJobResponse
	for {
		response, err := getVectorJob(ctx, client, cfg, jobID)
		if err != nil {
			return nil, fmt.Errorf("get vector job %s: %w", jobID, err)
		}
		last = response
		if response.GetJob().GetStatus() == wantStatus {
			return response, nil
		}
		if time.Now().After(deadline) {
			return last, fmt.Errorf("vector job %s did not reach %s; last=%+v", jobID, wantStatus, last.GetJob())
		}
		time.Sleep(cfg.pollInterval)
	}
}

func tombstoneVectorItem(ctx context.Context, client vectorv1.VectorIndexServiceClient, cfg config, vectorItemID string) (*vectorv1.TombstoneVectorItemResponse, error) {
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	return client.TombstoneVectorItem(requestCtx, &vectorv1.TombstoneVectorItemRequest{
		AuthContext:    authContext(cfg, cfg.sourceService, "vector-smoke-tombstone"),
		VectorItemId:   vectorItemID,
		DeleteProofId:  "delete-proof-" + sanitizeRunName(cfg.idempotencyKey),
		ReasonClass:    "SMOKE_DELETE",
		IdempotencyKey: cfg.idempotencyKey + "-tombstone",
		CorrelationId:  cfg.idempotencyKey,
		CausationId:    cfg.idempotencyKey,
		TraceId:        cfg.traceID,
	})
}

func searchVectors(ctx context.Context, client vectorv1.VectorIndexServiceClient, cfg config) (*vectorv1.SearchVectorsResponse, error) {
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	return client.SearchVectors(requestCtx, &vectorv1.SearchVectorsRequest{
		AuthContext:        authContext(cfg, "retrieval-gateway", "vector-smoke-search"),
		RequesterRef:       "requester-ref-vector-smoke",
		RetrievalRequestId: "retrieval-" + sanitizeRunName(cfg.idempotencyKey),
		CollectionTypes:    []string{cfg.collectionType},
		QueryEmbeddingRef:  hashRef(cfg.tenantID + "|query|" + cfg.idempotencyKey),
		TopK:               10,
		MinScore:           0,
		VisibilityScope:    cfg.visibilityScope,
		PolicyVersion:      cfg.policyVersion,
		AtUnixMs:           time.Now().UnixMilli(),
	})
}

func authContext(cfg config, serviceName string, requestID string) *vectorv1.AuthContext {
	return &vectorv1.AuthContext{
		TenantId:    cfg.tenantID,
		UserId:      cfg.userID,
		ServiceName: serviceName,
		InstanceRef: "loadtest-vectorindex",
		TraceId:     cfg.traceID,
		RequestId:   requestID,
	}
}

func readOutboxSummary(ctx context.Context, pool *pgxpool.Pool, tenantID string, vectorItemRefHash string) (outboxSummary, error) {
	rows, err := pool.Query(ctx, `
SELECT event_type, status, payload_json::text
FROM vector_outbox
WHERE tenant_id = $1
  AND aggregate_id = $2
`, tenantID, vectorItemRefHash)
	if err != nil {
		return outboxSummary{}, fmt.Errorf("read vector outbox: %w", err)
	}
	defer rows.Close()
	var summary outboxSummary
	for rows.Next() {
		var eventType string
		var status string
		var payload string
		if err := rows.Scan(&eventType, &status, &payload); err != nil {
			return outboxSummary{}, fmt.Errorf("scan vector outbox: %w", err)
		}
		summary.Total++
		if !payloadSafe(payload) {
			return outboxSummary{}, fmt.Errorf("vector outbox leaked internal field: %s", eventType)
		}
		switch eventType {
		case "vector.item.indexed.v1":
			summary.Indexed++
		case "vector.item.tombstoned.v1":
			summary.Tombstoned++
		case "vector.rebuild.started.v1":
			summary.RebuildStarted++
		case "vector.rebuild.completed.v1":
			summary.RebuildCompleted++
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
		return outboxSummary{}, fmt.Errorf("iterate vector outbox: %w", err)
	}
	return summary, nil
}

func readRebuildCheckpointStatus(ctx context.Context, pool *pgxpool.Pool, cfg config, rebuildJobID string) (string, error) {
	var status string
	err := pool.QueryRow(ctx, `
SELECT status
FROM vector_rebuild_checkpoints
WHERE tenant_id = $1
  AND rebuild_job_id = $2
  AND partition_key = $3
`, cfg.tenantID, rebuildJobID, rebuildPartitionKey(cfg)).Scan(&status)
	if err != nil {
		return "", fmt.Errorf("read rebuild checkpoint status: %w", err)
	}
	return status, nil
}

func waitOutboxPublished(ctx context.Context, pool *pgxpool.Pool, cfg config, vectorItemRefHash string, wantPublished int64) (outboxSummary, error) {
	deadline := time.Now().Add(cfg.waitTimeout)
	for {
		summary, err := readOutboxSummary(ctx, pool, cfg.tenantID, vectorItemRefHash)
		if err != nil {
			return outboxSummary{}, err
		}
		if summary.Published >= wantPublished && summary.Pending == 0 && summary.DLQ == 0 {
			return summary, nil
		}
		if time.Now().After(deadline) {
			return summary, fmt.Errorf("vector outbox did not drain: %+v", summary)
		}
		time.Sleep(cfg.pollInterval)
	}
}

func readVectorEvents(ctx context.Context, cfg config, vectorItemRefHash string, want int) ([]vectorKafkaEvent, error) {
	if !cfg.kafkaEnabled() {
		return nil, nil
	}
	reader := kafkago.NewReader(kafkago.ReaderConfig{
		Brokers:   cfg.kafkaBrokers,
		Topic:     cfg.vectorEventsTopic,
		Partition: 0,
		MinBytes:  1,
		MaxBytes:  10e6,
	})
	defer reader.Close()
	if err := reader.SetOffset(kafkago.FirstOffset); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(cfg.waitTimeout)
	events := make([]vectorKafkaEvent, 0, want)
	seen := map[string]bool{}
	for len(events) < want && time.Now().Before(deadline) {
		readCtx, cancel := context.WithTimeout(ctx, cfg.pollInterval)
		message, err := reader.ReadMessage(readCtx)
		cancel()
		if err != nil {
			continue
		}
		var event vectoreventsv1.VectorEvent
		if err := proto.Unmarshal(message.Value, &event); err != nil {
			continue
		}
		if event.GetTenantId() != cfg.tenantID || event.GetAggregateId() != vectorItemRefHash || seen[event.GetEventId()] {
			continue
		}
		if !protoMessageSafe(&event) {
			return events, fmt.Errorf("vector Kafka event leaked internal field: %s", event.GetEventId())
		}
		seen[event.GetEventId()] = true
		events = append(events, summarizeVectorEvent(&event))
	}
	if len(events) < want {
		return events, fmt.Errorf("expected %d vector Kafka events, got %d", want, len(events))
	}
	return events, nil
}

func summarizeVectorEvent(event *vectoreventsv1.VectorEvent) vectorKafkaEvent {
	result := vectorKafkaEvent{
		EventID:          event.GetEventId(),
		EventType:        event.GetEventType(),
		AggregateID:      event.GetAggregateId(),
		AggregateVersion: event.GetAggregateVersion(),
		PartitionKey:     event.GetPartitionKey(),
	}
	switch payload := event.GetPayload().(type) {
	case *vectoreventsv1.VectorEvent_ItemIndexed:
		result.PayloadKind = "item_indexed"
		result.CollectionType = payload.ItemIndexed.GetCollectionType()
		result.TombstoneStatus = payload.ItemIndexed.GetTombstoneStatus()
	case *vectoreventsv1.VectorEvent_ItemTombstoned:
		result.PayloadKind = "item_tombstoned"
		result.CollectionType = payload.ItemTombstoned.GetCollectionType()
		result.TombstoneStatus = payload.ItemTombstoned.GetTombstoneStatus()
	case *vectoreventsv1.VectorEvent_RebuildStarted:
		result.PayloadKind = "rebuild_started"
		result.CollectionType = payload.RebuildStarted.GetCollectionType()
	case *vectoreventsv1.VectorEvent_RebuildCompleted:
		result.PayloadKind = "rebuild_completed"
		result.CollectionType = payload.RebuildCompleted.GetCollectionType()
	}
	return result
}

func applyVectorMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	dir := filepath.Join("migrations", "postgres", "vector-index")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read vector migration dir: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".sql" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read vector migration %s: %w", entry.Name(), err)
		}
		if _, err := pool.Exec(ctx, string(content)); err != nil {
			return fmt.Errorf("apply vector migration %s: %w", entry.Name(), err)
		}
	}
	return nil
}

func cleanupTenant(ctx context.Context, pool *pgxpool.Pool, tenantID string) error {
	queries := []string{
		`DELETE FROM vector_outbox WHERE tenant_id = $1`,
		`DELETE FROM vector_rebuild_checkpoints WHERE tenant_id = $1`,
		`DELETE FROM vector_tombstones WHERE tenant_id = $1`,
		`DELETE FROM vector_index_jobs WHERE tenant_id = $1`,
		`DELETE FROM vector_items WHERE tenant_id = $1`,
		`DELETE FROM vector_collections WHERE tenant_id = $1`,
	}
	for _, query := range queries {
		if _, err := pool.Exec(ctx, query, tenantID); err != nil {
			return fmt.Errorf("cleanup vector tenant: %w", err)
		}
	}
	return nil
}

func validateConfig(cfg config) error {
	if strings.TrimSpace(cfg.vectorTarget) == "" {
		return errors.New("vector-target is required")
	}
	if strings.TrimSpace(cfg.pgDSN) == "" {
		return errors.New("pg-dsn is required")
	}
	if strings.TrimSpace(cfg.tenantID) == "" || strings.TrimSpace(cfg.userID) == "" {
		return errors.New("tenant-id and user-id are required")
	}
	if len(cfg.kafkaBrokers) > 0 && strings.TrimSpace(cfg.vectorEventsTopic) == "" {
		return errors.New("vector-events-topic is required when kafka-brokers is provided")
	}
	return nil
}

func (cfg config) kafkaEnabled() bool {
	return len(cfg.kafkaBrokers) > 0 && strings.TrimSpace(cfg.vectorEventsTopic) != ""
}

func writeSummary(resultDir string, result summary) error {
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(resultDir, "vector-index-outbox-relay-summary.json")
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
		"raw_text",
		"message_body",
		"embedding_vector\":",
		"embeddingVector\":",
		"vector_array",
		"source_uri",
		"object_key",
		"connector_secret",
		"authorization",
		"api_key",
		"private_key",
		"password",
		"postgres://",
		"http://",
		"https://",
		"s3://",
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
		return "vector-index-smoke"
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

func hashRef(value string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func envOr(name string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}

func envBool(name string, fallback bool) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	switch value {
	case "":
		return fallback
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return fallback
	}
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
