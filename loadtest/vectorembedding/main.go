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
	knowledgev1 "github.com/qsyy0921/IM/api/proto/nexusim/knowledge/v1"
	vectorv1 "github.com/qsyy0921/IM/api/proto/nexusim/vector/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const defaultResultRoot = `H:\NexusIM\loadtest-results`

type config struct {
	phase                     string
	knowledgeTarget           string
	vectorTarget              string
	pgDSN                     string
	pgVectorDSN               string
	pgVectorTable             string
	openSearchVectorEndpoint  string
	openSearchVectorIndex     string
	openSearchVectorField     string
	openSearchVectorDimension int
	resultDir                 string
	requestTimeout            time.Duration
	waitTimeout               time.Duration
	pollInterval              time.Duration
	tenantID                  string
	userID                    string
	idempotencyKey            string
	cleanup                   bool
	applyMigration            bool
	traceID                   string
	sourceID                  string
	documentID                string
	visibilityScope           string
	policyVersion             string
	expectedCount             int
	embeddingModel            string
	embeddingDim              int
}

type summary struct {
	Phase                             string    `json:"phase"`
	Commit                            string    `json:"commit"`
	CommitFull                        string    `json:"commit_full"`
	GitDirty                          bool      `json:"git_dirty"`
	GitStatusShort                    string    `json:"git_status_short,omitempty"`
	ResultDir                         string    `json:"result_dir"`
	KnowledgeTarget                   string    `json:"knowledge_target,omitempty"`
	VectorTarget                      string    `json:"vector_target,omitempty"`
	TenantID                          string    `json:"tenant_id"`
	UserID                            string    `json:"user_id"`
	StartedAt                         time.Time `json:"started_at"`
	FinishedAt                        time.Time `json:"finished_at"`
	Success                           bool      `json:"success"`
	Error                             string    `json:"error,omitempty"`
	KnowledgeSourceID                 string    `json:"knowledge_source_id,omitempty"`
	KnowledgeJobID                    string    `json:"knowledge_job_id,omitempty"`
	DocumentID                        string    `json:"document_id,omitempty"`
	ChunkCount                        int       `json:"chunk_count"`
	ExpectedCount                     int       `json:"expected_count"`
	VectorSearchCount                 int       `json:"vector_search_count"`
	PGVectorConfigured                bool      `json:"pgvector_configured"`
	PGVectorTable                     string    `json:"pgvector_table,omitempty"`
	PGVectorAvailable                 bool      `json:"pgvector_available"`
	PGVectorInstalled                 bool      `json:"pgvector_installed"`
	PGVectorDefaultVersion            string    `json:"pgvector_default_version,omitempty"`
	PGVectorInstalledVersion          string    `json:"pgvector_installed_version,omitempty"`
	PGVectorItemCount                 int       `json:"pgvector_item_count"`
	PGVectorActiveCount               int       `json:"pgvector_active_count"`
	PGVectorMinDimension              int       `json:"pgvector_min_dimension"`
	PGVectorMaxDimension              int       `json:"pgvector_max_dimension"`
	OpenSearchVectorConfigured        bool      `json:"opensearch_vector_configured"`
	OpenSearchVectorEndpoint          string    `json:"opensearch_vector_endpoint,omitempty"`
	OpenSearchVectorIndex             string    `json:"opensearch_vector_index,omitempty"`
	OpenSearchVectorField             string    `json:"opensearch_vector_field,omitempty"`
	OpenSearchVectorAvailable         bool      `json:"opensearch_vector_available"`
	OpenSearchVectorIndexExists       bool      `json:"opensearch_vector_index_exists"`
	OpenSearchVectorMappingVerified   bool      `json:"opensearch_vector_mapping_verified"`
	OpenSearchVectorFieldType         string    `json:"opensearch_vector_field_type,omitempty"`
	OpenSearchVectorDimension         int       `json:"opensearch_vector_dimension"`
	OpenSearchVectorExpectedDimension int       `json:"opensearch_vector_expected_dimension"`
	EmbeddingTaskCount                int       `json:"embedding_task_count"`
	EmbeddingTaskPending              int       `json:"embedding_task_pending"`
	EmbeddingTaskRunning              int       `json:"embedding_task_running"`
	EmbeddingTaskCompleted            int       `json:"embedding_task_completed"`
	RebuildJobID                      string    `json:"rebuild_job_id,omitempty"`
	RebuildJobStatus                  string    `json:"rebuild_job_status,omitempty"`
	RebuildCheckpoint                 string    `json:"rebuild_checkpoint_status,omitempty"`
	RebuildCursorValue                string    `json:"rebuild_cursor_value,omitempty"`
	VisibilityScope                   string    `json:"visibility_scope,omitempty"`
	PolicyVersion                     string    `json:"policy_version,omitempty"`
	EmbeddingModelRef                 string    `json:"embedding_model_ref"`
	EmbeddingDimension                int       `json:"embedding_dimension"`
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
	flag.StringVar(&cfg.phase, "phase", "prepare", "phase: prepare or verify")
	flag.StringVar(&cfg.knowledgeTarget, "knowledge-target", envOr("NEXUSIM_KNOWLEDGE_INGESTION_GRPC_ADDR", "127.0.0.1:10740"), "knowledge-ingestion-service gRPC target")
	flag.StringVar(&cfg.vectorTarget, "vector-target", envOr("NEXUSIM_VECTOR_INDEX_GRPC_ADDR", "127.0.0.1:10760"), "vector-index-service gRPC target")
	flag.StringVar(&cfg.pgDSN, "pg-dsn", envOr("NEXUSIM_PG_DSN", "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable"), "PostgreSQL DSN")
	flag.StringVar(&cfg.pgVectorDSN, "pgvector-dsn", envOr("NEXUSIM_VECTOR_PGVECTOR_DSN", ""), "optional pgvector backend DSN for verify phase")
	flag.StringVar(&cfg.pgVectorTable, "pgvector-table", envOr("NEXUSIM_VECTOR_PGVECTOR_TABLE", "vector_embedding_items"), "optional pgvector backend table for verify phase")
	flag.StringVar(&cfg.openSearchVectorEndpoint, "opensearch-vector-endpoint", envOr("NEXUSIM_VECTOR_OPENSEARCH_ENDPOINT", ""), "optional OpenSearch vector endpoint for preflight-opensearch-vector")
	flag.StringVar(&cfg.openSearchVectorIndex, "opensearch-vector-index", envOr("NEXUSIM_VECTOR_OPENSEARCH_INDEX", "nexusim-vector-items"), "optional OpenSearch vector index for preflight-opensearch-vector")
	flag.StringVar(&cfg.openSearchVectorField, "opensearch-vector-field", envOr("NEXUSIM_VECTOR_OPENSEARCH_VECTOR_FIELD", "embedding_vector"), "optional OpenSearch knn_vector field for preflight-opensearch-vector")
	flag.IntVar(&cfg.openSearchVectorDimension, "opensearch-vector-dimension", 0, "expected OpenSearch knn_vector dimension; defaults to embedding-dimension")
	flag.StringVar(&resultRoot, "result-root", defaultResultRoot, "external result root for raw smoke output")
	flag.StringVar(&runName, "run-name", "", "run name under result-root")
	flag.DurationVar(&cfg.requestTimeout, "request-timeout", 5*time.Second, "per request timeout")
	flag.DurationVar(&cfg.waitTimeout, "wait-timeout", 20*time.Second, "verification wait timeout")
	flag.DurationVar(&cfg.pollInterval, "poll-interval", 500*time.Millisecond, "verification poll interval")
	flag.StringVar(&cfg.tenantID, "tenant-id", "", "tenant id; defaults to tenant derived from run name")
	flag.StringVar(&cfg.userID, "user-id", "vector-embedding-smoke-user", "requesting user id")
	flag.StringVar(&cfg.idempotencyKey, "idempotency-key", "", "idempotency key; defaults to key derived from run name")
	flag.BoolVar(&cfg.cleanup, "cleanup", true, "delete smoke tenant rows before prepare")
	flag.BoolVar(&cfg.applyMigration, "apply-migration", true, "apply knowledge/model/vector migrations before prepare")
	flag.StringVar(&cfg.sourceID, "source-id", "", "knowledge source id for verify phase")
	flag.StringVar(&cfg.documentID, "document-id", "", "knowledge document id for verify phase")
	flag.StringVar(&cfg.visibilityScope, "visibility-scope", "", "visibility scope for vector search verify phase")
	flag.StringVar(&cfg.policyVersion, "policy-version", "", "policy version for vector search verify phase")
	flag.IntVar(&cfg.expectedCount, "expected-count", 2, "expected vector search result count")
	flag.StringVar(&cfg.embeddingModel, "embedding-model", "deterministic-embedding-v1", "embedding model ref expected from worker")
	flag.IntVar(&cfg.embeddingDim, "embedding-dimension", 8, "embedding dimension expected from worker")
	flag.Parse()

	cfg.phase = strings.ToLower(strings.TrimSpace(cfg.phase))
	if runName == "" {
		runName = "vector-embedding-producer-smoke-" + time.Now().Format("20060102-150405")
	}
	safeRunName := sanitizeRunName(runName)
	if cfg.tenantID == "" {
		cfg.tenantID = "tenant-" + safeRunName
	}
	if cfg.idempotencyKey == "" {
		cfg.idempotencyKey = "idem-" + safeRunName
	}
	cfg.traceID = "trace-" + safeRunName
	cfg.resultDir = filepath.Join(resultRoot, runName)
	if cfg.requestTimeout <= 0 {
		cfg.requestTimeout = 5 * time.Second
	}
	if cfg.waitTimeout <= 0 {
		cfg.waitTimeout = 20 * time.Second
	}
	if cfg.pollInterval <= 0 {
		cfg.pollInterval = 500 * time.Millisecond
	}
	if cfg.embeddingDim <= 0 {
		cfg.embeddingDim = 8
	}
	if cfg.openSearchVectorDimension <= 0 {
		cfg.openSearchVectorDimension = cfg.embeddingDim
	}
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
		Phase:                             cfg.phase,
		Commit:                            gitOutput("rev-parse", "--short", "HEAD"),
		CommitFull:                        gitOutput("rev-parse", "HEAD"),
		GitStatusShort:                    gitOutput("status", "--short", "--untracked-files=all"),
		ResultDir:                         cfg.resultDir,
		KnowledgeTarget:                   cfg.knowledgeTarget,
		VectorTarget:                      cfg.vectorTarget,
		TenantID:                          cfg.tenantID,
		UserID:                            cfg.userID,
		StartedAt:                         time.Now().UTC(),
		ExpectedCount:                     cfg.expectedCount,
		EmbeddingModelRef:                 cfg.embeddingModel,
		EmbeddingDimension:                cfg.embeddingDim,
		PGVectorConfigured:                strings.TrimSpace(cfg.pgVectorDSN) != "",
		PGVectorTable:                     strings.TrimSpace(cfg.pgVectorTable),
		OpenSearchVectorConfigured:        strings.TrimSpace(cfg.openSearchVectorEndpoint) != "",
		OpenSearchVectorEndpoint:          safeOpenSearchEndpointForSummary(cfg.openSearchVectorEndpoint),
		OpenSearchVectorIndex:             strings.TrimSpace(cfg.openSearchVectorIndex),
		OpenSearchVectorField:             strings.TrimSpace(cfg.openSearchVectorField),
		OpenSearchVectorExpectedDimension: cfg.openSearchVectorDimension,
	}
	result.GitDirty = strings.TrimSpace(result.GitStatusShort) != ""
	defer func() {
		result.FinishedAt = time.Now().UTC()
		_ = writeSummary(cfg.resultDir, result)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), cfg.waitTimeout+30*time.Second)
	defer cancel()

	switch cfg.phase {
	case "prepare":
		if err := prepare(ctx, cfg, &result); err != nil {
			result.Error = err.Error()
			return err
		}
	case "verify":
		if err := verify(ctx, cfg, &result); err != nil {
			result.Error = err.Error()
			return err
		}
	case "preflight-pgvector":
		if err := preflightPGVector(ctx, cfg, &result); err != nil {
			result.Error = err.Error()
			return err
		}
	case "preflight-opensearch-vector":
		if err := preflightOpenSearchVector(ctx, cfg, &result); err != nil {
			result.Error = err.Error()
			return err
		}
	case "verify-queue":
		if err := verifyQueue(ctx, cfg, &result); err != nil {
			result.Error = err.Error()
			return err
		}
	case "request-rebuild":
		if err := requestRebuild(ctx, cfg, &result); err != nil {
			result.Error = err.Error()
			return err
		}
	default:
		result.Error = "unsupported phase"
		return fmt.Errorf("unsupported phase %q", cfg.phase)
	}
	result.Success = true
	return nil
}

func verifyQueue(ctx context.Context, cfg config, result *summary) error {
	pool, err := pgxpool.New(ctx, cfg.pgDSN)
	if err != nil {
		return fmt.Errorf("open postgres: %w", err)
	}
	defer pool.Close()
	deadline := time.Now().Add(cfg.waitTimeout)
	for {
		counts, err := readEmbeddingTaskCounts(ctx, pool, cfg)
		if err == nil {
			result.EmbeddingTaskCount = counts.total
			result.EmbeddingTaskPending = counts.pending
			result.EmbeddingTaskRunning = counts.running
			result.EmbeddingTaskCompleted = counts.completed
			result.KnowledgeSourceID = cfg.sourceID
			result.DocumentID = cfg.documentID
			result.ChunkCount = cfg.expectedCount
			result.ExpectedCount = cfg.expectedCount
			if counts.total >= cfg.expectedCount && counts.pending >= cfg.expectedCount {
				return nil
			}
		}
		if time.Now().After(deadline) {
			if err != nil {
				return fmt.Errorf("wait for vector embedding queue tasks: %w", err)
			}
			return fmt.Errorf("expected at least %d pending vector embedding tasks, got total=%d pending=%d running=%d completed=%d",
				cfg.expectedCount, result.EmbeddingTaskCount, result.EmbeddingTaskPending, result.EmbeddingTaskRunning, result.EmbeddingTaskCompleted)
		}
		timer := time.NewTimer(cfg.pollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

type embeddingTaskCounts struct {
	total     int
	pending   int
	running   int
	completed int
}

func readEmbeddingTaskCounts(ctx context.Context, pool *pgxpool.Pool, cfg config) (embeddingTaskCounts, error) {
	prefix := strings.Trim(strings.TrimSpace(cfg.sourceID)+":"+strings.TrimSpace(cfg.documentID), ":")
	rows, err := pool.Query(ctx, `
SELECT status, count(*)
FROM vector_embedding_tasks
WHERE tenant_id = $1
  AND source_service = 'knowledge-ingestion-service'
  AND collection_type = 'KNOWLEDGE_CHUNK'
  AND embedding_model_ref = $2
  AND ($3 = '' OR source_id LIKE $3 || ':%')
GROUP BY status
`, cfg.tenantID, cfg.embeddingModel, prefix)
	if err != nil {
		return embeddingTaskCounts{}, fmt.Errorf("query vector embedding tasks: %w", err)
	}
	defer rows.Close()
	counts := embeddingTaskCounts{}
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return embeddingTaskCounts{}, fmt.Errorf("scan vector embedding tasks: %w", err)
		}
		counts.total += count
		switch status {
		case "PENDING":
			counts.pending = count
		case "RUNNING":
			counts.running = count
		case "COMPLETED":
			counts.completed = count
		}
	}
	if err := rows.Err(); err != nil {
		return embeddingTaskCounts{}, fmt.Errorf("read vector embedding task rows: %w", err)
	}
	return counts, nil
}

func preflightPGVector(ctx context.Context, cfg config, result *summary) error {
	pool, err := pgxpool.New(ctx, cfg.pgVectorDSN)
	if err != nil {
		return fmt.Errorf("open pgvector postgres: %w", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("connect pgvector postgres: %w", err)
	}
	var available bool
	var defaultVersion string
	var installedVersion string
	err = pool.QueryRow(ctx, `
SELECT EXISTS (
           SELECT 1
           FROM pg_available_extensions
           WHERE name = 'vector'
       ),
       COALESCE((
           SELECT default_version
           FROM pg_available_extensions
           WHERE name = 'vector'
       ), ''),
       COALESCE((
           SELECT installed_version
           FROM pg_available_extensions
           WHERE name = 'vector'
       ), '')
`).Scan(&available, &defaultVersion, &installedVersion)
	if err != nil {
		return fmt.Errorf("inspect pgvector extension: %w", err)
	}
	result.PGVectorAvailable = available
	result.PGVectorInstalled = strings.TrimSpace(installedVersion) != ""
	result.PGVectorDefaultVersion = strings.TrimSpace(defaultVersion)
	result.PGVectorInstalledVersion = strings.TrimSpace(installedVersion)
	if !available {
		return errors.New("pgvector extension is not available on the configured PostgreSQL instance")
	}
	if _, err := quoteSQLIdentifier(cfg.pgVectorTable); err != nil {
		return err
	}
	return nil
}

func prepare(ctx context.Context, cfg config, result *summary) error {
	pool, err := pgxpool.New(ctx, cfg.pgDSN)
	if err != nil {
		return fmt.Errorf("open postgres: %w", err)
	}
	defer pool.Close()
	if cfg.applyMigration {
		if err := applyMigrations(ctx, pool); err != nil {
			return err
		}
	}
	if cfg.cleanup {
		if err := cleanupTenant(ctx, pool, cfg.tenantID); err != nil {
			return err
		}
	}

	conn, err := grpc.DialContext(ctx, cfg.knowledgeTarget, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		return fmt.Errorf("dial knowledge-ingestion-service: %w", err)
	}
	defer conn.Close()
	client := knowledgev1.NewKnowledgeIngestionServiceClient(conn)

	source, err := createKnowledgeSource(ctx, client, cfg)
	if err != nil {
		return fmt.Errorf("create knowledge source: %w", err)
	}
	result.KnowledgeSourceID = source.GetSource().GetSourceId()
	result.VisibilityScope = source.GetSource().GetVisibilityScope()
	if result.KnowledgeSourceID == "" || result.VisibilityScope == "" {
		return errors.New("knowledge source response missing source id or visibility scope")
	}

	job, err := submitKnowledgeJob(ctx, client, cfg, source.GetSource())
	if err != nil {
		return fmt.Errorf("submit knowledge job: %w", err)
	}
	result.KnowledgeJobID = job.GetJob().GetJobId()
	result.DocumentID = job.GetDocumentId()
	result.ChunkCount = int(job.GetChunkCount())
	result.ExpectedCount = result.ChunkCount
	result.PolicyVersion = "policy-vector-embedding-smoke"
	if result.KnowledgeJobID == "" || result.DocumentID == "" || result.ChunkCount <= 0 {
		return errors.New("knowledge job response missing job id, document id, or chunks")
	}

	chunks, err := listKnowledgeChunks(ctx, client, cfg, result.KnowledgeSourceID, result.DocumentID)
	if err != nil {
		return fmt.Errorf("list knowledge chunks: %w", err)
	}
	if len(chunks) != result.ChunkCount {
		return fmt.Errorf("expected %d knowledge chunks, got %d", result.ChunkCount, len(chunks))
	}
	if len(chunks) > 0 {
		result.VisibilityScope = chunks[0].GetVisibilityScope()
		result.PolicyVersion = chunks[0].GetPolicyVersion()
	}
	return nil
}

func verify(ctx context.Context, cfg config, result *summary) error {
	conn, err := grpc.DialContext(ctx, cfg.vectorTarget, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		return fmt.Errorf("dial vector-index-service: %w", err)
	}
	defer conn.Close()
	client := vectorv1.NewVectorIndexServiceClient(conn)

	deadline := time.Now().Add(cfg.waitTimeout)
	for {
		search, err := searchVectors(ctx, client, cfg)
		lastErr := err
		if err == nil {
			result.VectorSearchCount = len(search.GetResults())
			result.VisibilityScope = cfg.visibilityScope
			result.PolicyVersion = cfg.policyVersion
			result.KnowledgeSourceID = cfg.sourceID
			result.DocumentID = cfg.documentID
			result.ChunkCount = cfg.expectedCount
			result.ExpectedCount = cfg.expectedCount
			if result.VectorSearchCount >= cfg.expectedCount {
				if strings.TrimSpace(cfg.pgVectorDSN) != "" {
					counts, err := readPGVectorCounts(ctx, cfg)
					if err != nil {
						lastErr = err
					} else {
						result.PGVectorItemCount = counts.total
						result.PGVectorActiveCount = counts.active
						result.PGVectorMinDimension = counts.minDimension
						result.PGVectorMaxDimension = counts.maxDimension
						if counts.active >= cfg.expectedCount && counts.minDimension == cfg.embeddingDim && counts.maxDimension == cfg.embeddingDim {
							return nil
						}
						lastErr = fmt.Errorf("expected at least %d active pgvector items with dimension %d, got active=%d min_dimension=%d max_dimension=%d",
							cfg.expectedCount, cfg.embeddingDim, counts.active, counts.minDimension, counts.maxDimension)
					}
					err = lastErr
				} else {
					return nil
				}
			}
		}
		if time.Now().After(deadline) {
			if lastErr != nil {
				return fmt.Errorf("wait for vector search and pgvector results: %w", lastErr)
			}
			return fmt.Errorf("expected at least %d vector search results, got %d", cfg.expectedCount, result.VectorSearchCount)
		}
		timer := time.NewTimer(cfg.pollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func requestRebuild(ctx context.Context, cfg config, result *summary) error {
	conn, err := grpc.DialContext(ctx, cfg.vectorTarget, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		return fmt.Errorf("dial vector-index-service: %w", err)
	}
	defer conn.Close()
	client := vectorv1.NewVectorIndexServiceClient(conn)

	response, err := requestVectorRebuild(ctx, client, cfg)
	if err != nil {
		return fmt.Errorf("request vector rebuild: %w", err)
	}
	jobID := response.GetJob().GetJobId()
	if jobID == "" {
		return errors.New("vector rebuild response missing job id")
	}
	result.RebuildJobID = jobID
	result.RebuildCheckpoint = response.GetCheckpoint().GetStatus()
	result.RebuildCursorValue = response.GetCheckpoint().GetCursorValue()
	result.KnowledgeSourceID = cfg.sourceID
	result.DocumentID = cfg.documentID
	result.ChunkCount = cfg.expectedCount
	result.ExpectedCount = cfg.expectedCount
	result.VisibilityScope = cfg.visibilityScope
	result.PolicyVersion = cfg.policyVersion

	completed, err := waitVectorJobStatus(ctx, client, cfg, jobID, "INDEXED")
	if err != nil {
		return err
	}
	result.RebuildJobStatus = completed.GetJob().GetStatus()

	checkpoint, err := readRebuildCheckpoint(ctx, cfg, jobID, rebuildPartitionKey(cfg))
	if err != nil {
		return err
	}
	result.RebuildCheckpoint = checkpoint.status
	result.RebuildCursorValue = checkpoint.cursorValue
	if checkpoint.status != "COMPLETED" {
		return fmt.Errorf("expected rebuild checkpoint COMPLETED, got %s", checkpoint.status)
	}
	if cfg.expectedCount > 1 && !strings.HasPrefix(checkpoint.cursorValue, "embedding-task:") {
		return fmt.Errorf("expected paginated rebuild cursor to start with embedding-task:, got %q", checkpoint.cursorValue)
	}

	pool, err := pgxpool.New(ctx, cfg.pgDSN)
	if err != nil {
		return fmt.Errorf("open postgres: %w", err)
	}
	defer pool.Close()
	counts, err := readEmbeddingTaskCounts(ctx, pool, cfg)
	if err != nil {
		return err
	}
	result.EmbeddingTaskCount = counts.total
	result.EmbeddingTaskPending = counts.pending
	result.EmbeddingTaskRunning = counts.running
	result.EmbeddingTaskCompleted = counts.completed
	if counts.completed < cfg.expectedCount {
		return fmt.Errorf("expected at least %d completed embedding tasks before rebuild backfill, got %d", cfg.expectedCount, counts.completed)
	}
	return nil
}

type pgVectorCounts struct {
	total        int
	active       int
	minDimension int
	maxDimension int
}

func readPGVectorCounts(ctx context.Context, cfg config) (pgVectorCounts, error) {
	pool, err := pgxpool.New(ctx, cfg.pgVectorDSN)
	if err != nil {
		return pgVectorCounts{}, fmt.Errorf("open pgvector postgres: %w", err)
	}
	defer pool.Close()
	table, err := quoteSQLIdentifier(cfg.pgVectorTable)
	if err != nil {
		return pgVectorCounts{}, err
	}
	var counts pgVectorCounts
	query := fmt.Sprintf(`
SELECT count(*),
       count(*) FILTER (WHERE status = 'ACTIVE'),
       COALESCE(min(dimension), 0),
       COALESCE(max(dimension), 0)
FROM %s
WHERE tenant_id = $1
`, table)
	if err := pool.QueryRow(ctx, query, cfg.tenantID).Scan(&counts.total, &counts.active, &counts.minDimension, &counts.maxDimension); err != nil {
		return pgVectorCounts{}, fmt.Errorf("read pgvector backend rows: %w", err)
	}
	return counts, nil
}

type rebuildCheckpointSummary struct {
	status      string
	cursorValue string
}

func readRebuildCheckpoint(ctx context.Context, cfg config, jobID string, partitionKey string) (rebuildCheckpointSummary, error) {
	pool, err := pgxpool.New(ctx, cfg.pgDSN)
	if err != nil {
		return rebuildCheckpointSummary{}, fmt.Errorf("open postgres: %w", err)
	}
	defer pool.Close()
	var checkpoint rebuildCheckpointSummary
	err = pool.QueryRow(ctx, `
SELECT status, cursor_value
FROM vector_rebuild_checkpoints
WHERE tenant_id = $1
  AND rebuild_job_id = $2
  AND partition_key = $3
`, cfg.tenantID, jobID, partitionKey).Scan(&checkpoint.status, &checkpoint.cursorValue)
	if err != nil {
		return rebuildCheckpointSummary{}, fmt.Errorf("read vector rebuild checkpoint: %w", err)
	}
	return checkpoint, nil
}

func requestVectorRebuild(ctx context.Context, client vectorv1.VectorIndexServiceClient, cfg config) (*vectorv1.RequestVectorRebuildResponse, error) {
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	return client.RequestVectorRebuild(requestCtx, &vectorv1.RequestVectorRebuildRequest{
		AuthContext:       vectorAuth(cfg, "vector-index-service", "vector-embedding-rebuild"),
		CollectionType:    "KNOWLEDGE_CHUNK",
		EmbeddingModelRef: cfg.embeddingModel,
		Dimension:         int32(cfg.embeddingDim),
		SourceService:     "knowledge-ingestion-service",
		PartitionKey:      rebuildPartitionKey(cfg),
		CursorValue:       "cursor:start",
		IdempotencyKey:    cfg.idempotencyKey + "-rebuild-backfill",
		CorrelationId:     cfg.idempotencyKey,
		CausationId:       cfg.idempotencyKey,
		TraceId:           cfg.traceID,
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
		timer := time.NewTimer(cfg.pollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func getVectorJob(ctx context.Context, client vectorv1.VectorIndexServiceClient, cfg config, jobID string) (*vectorv1.GetVectorIndexJobResponse, error) {
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	return client.GetVectorIndexJob(requestCtx, &vectorv1.GetVectorIndexJobRequest{
		AuthContext: vectorAuth(cfg, "vector-index-service", "vector-embedding-get-rebuild"),
		JobId:       jobID,
	})
}

func rebuildPartitionKey(cfg config) string {
	return "knowledge-ingestion-service:" + cfg.tenantID + ":" + cfg.sourceID + ":" + cfg.documentID
}

func createKnowledgeSource(ctx context.Context, client knowledgev1.KnowledgeIngestionServiceClient, cfg config) (*knowledgev1.CreateKnowledgeSourceResponse, error) {
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	return client.CreateKnowledgeSource(requestCtx, &knowledgev1.CreateKnowledgeSourceRequest{
		AuthContext:        knowledgeAuth(cfg, "knowledge-ingestion-service", "vector-embedding-source"),
		SourceType:         "MANUAL_MARKDOWN",
		SourceRef:          "vector-embedding-producer-smoke-source",
		SourceUriHash:      hashRef(cfg.tenantID + "|source-uri"),
		MediaObjectRef:     "media-object-ref-vector-embedding-smoke",
		OwnerRef:           "owner-ref-vector-embedding-smoke",
		VisibilityScope:    "conversation:vector-embedding-producer-smoke",
		DataClass:          "BUSINESS_INTERNAL",
		ContentHash:        hashRef(cfg.tenantID + "|content"),
		MimeType:           "text/markdown",
		SizeBytes:          256,
		SourceVersion:      "1",
		RetentionPolicyRef: "retention-vector-embedding-smoke",
		IdempotencyKey:     cfg.idempotencyKey + "-source",
		CorrelationId:      cfg.idempotencyKey,
		CausationId:        cfg.idempotencyKey,
		TraceId:            cfg.traceID,
	})
}

func submitKnowledgeJob(ctx context.Context, client knowledgev1.KnowledgeIngestionServiceClient, cfg config, source *knowledgev1.KnowledgeSource) (*knowledgev1.SubmitIngestionJobResponse, error) {
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	chunks := []*knowledgev1.ChunkManifestItem{
		{
			ChunkHash:            hashRef(cfg.tenantID + "|chunk|0"),
			ChunkPreviewRedacted: "redacted vector embedding producer chunk 0",
			VisibilityScope:      source.GetVisibilityScope(),
			DataClass:            source.GetDataClass(),
			PolicyVersion:        "policy-vector-embedding-smoke",
			ChunkVersion:         "1",
		},
		{
			ChunkHash:            hashRef(cfg.tenantID + "|chunk|1"),
			ChunkPreviewRedacted: "redacted vector embedding producer chunk 1",
			VisibilityScope:      source.GetVisibilityScope(),
			DataClass:            source.GetDataClass(),
			PolicyVersion:        "policy-vector-embedding-smoke",
			ChunkVersion:         "1",
		},
	}
	return client.SubmitIngestionJob(requestCtx, &knowledgev1.SubmitIngestionJobRequest{
		AuthContext:        knowledgeAuth(cfg, "knowledge-ingestion-service", "vector-embedding-job"),
		SourceId:           source.GetSourceId(),
		SourceVersion:      source.GetSourceVersion(),
		JobType:            "INGEST",
		ParserProfile:      "local-manifest-v1",
		ChunkProfile:       "fixed-manifest-v1",
		EmbeddingPolicyRef: "embedding-policy-vector-producer-smoke",
		VectorPolicyRef:    "vector-policy-vector-producer-smoke",
		RequestedBy:        cfg.userID,
		IdempotencyKey:     cfg.idempotencyKey + "-job",
		DocumentHash:       hashRef(cfg.tenantID + "|document"),
		MimeType:           source.GetMimeType(),
		SizeBytes:          source.GetSizeBytes(),
		PageCount:          1,
		Language:           "zh",
		Chunks:             chunks,
		CorrelationId:      cfg.idempotencyKey,
		CausationId:        cfg.idempotencyKey,
		TraceId:            cfg.traceID,
	})
}

func listKnowledgeChunks(ctx context.Context, client knowledgev1.KnowledgeIngestionServiceClient, cfg config, sourceID string, documentID string) ([]*knowledgev1.KnowledgeChunk, error) {
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	response, err := client.ListKnowledgeChunks(requestCtx, &knowledgev1.ListKnowledgeChunksRequest{
		AuthContext: knowledgeAuth(cfg, "knowledge-ingestion-service", "vector-embedding-list"),
		SourceId:    sourceID,
		DocumentId:  documentID,
		PageSize:    10,
	})
	if err != nil {
		return nil, err
	}
	return response.GetChunks(), nil
}

func searchVectors(ctx context.Context, client vectorv1.VectorIndexServiceClient, cfg config) (*vectorv1.SearchVectorsResponse, error) {
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	return client.SearchVectors(requestCtx, &vectorv1.SearchVectorsRequest{
		AuthContext:        vectorAuth(cfg, "retrieval-gateway", "vector-embedding-search"),
		RequesterRef:       "requester-ref-vector-embedding-smoke",
		RetrievalRequestId: "retrieval-" + sanitizeRunName(cfg.idempotencyKey),
		CollectionTypes:    []string{"KNOWLEDGE_CHUNK"},
		QueryEmbeddingRef:  hashRef(cfg.tenantID + "|query|" + cfg.idempotencyKey),
		TopK:               10,
		MinScore:           0,
		VisibilityScope:    cfg.visibilityScope,
		PolicyVersion:      cfg.policyVersion,
		AtUnixMs:           time.Now().UnixMilli(),
	})
}

func knowledgeAuth(cfg config, serviceName string, requestID string) *knowledgev1.AuthContext {
	return &knowledgev1.AuthContext{
		TenantId:    cfg.tenantID,
		UserId:      cfg.userID,
		ServiceName: serviceName,
		InstanceRef: "loadtest-vector-embedding",
		TraceId:     cfg.traceID,
		RequestId:   requestID,
	}
}

func vectorAuth(cfg config, serviceName string, requestID string) *vectorv1.AuthContext {
	return &vectorv1.AuthContext{
		TenantId:    cfg.tenantID,
		UserId:      cfg.userID,
		ServiceName: serviceName,
		InstanceRef: "loadtest-vector-embedding",
		TraceId:     cfg.traceID,
		RequestId:   requestID,
	}
}

func applyMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	for _, dir := range []string{
		filepath.Join("migrations", "postgres", "knowledge-ingestion"),
		filepath.Join("migrations", "postgres", "model-gateway"),
		filepath.Join("migrations", "postgres", "vector-index"),
	} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return fmt.Errorf("read migration dir %s: %w", dir, err)
		}
		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".sql" {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			content, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("read migration %s: %w", path, err)
			}
			if _, err := pool.Exec(ctx, string(content)); err != nil {
				return fmt.Errorf("apply migration %s: %w", path, err)
			}
		}
	}
	return nil
}

func cleanupTenant(ctx context.Context, pool *pgxpool.Pool, tenantID string) error {
	queries := []string{
		`DELETE FROM vector_embedding_tasks WHERE tenant_id = $1`,
		`DELETE FROM vector_outbox WHERE tenant_id = $1`,
		`DELETE FROM vector_rebuild_checkpoints WHERE tenant_id = $1`,
		`DELETE FROM vector_tombstones WHERE tenant_id = $1`,
		`DELETE FROM vector_index_jobs WHERE tenant_id = $1`,
		`DELETE FROM vector_items WHERE tenant_id = $1`,
		`DELETE FROM vector_collections WHERE tenant_id = $1`,
		`DELETE FROM model_outbox WHERE tenant_id = $1`,
		`DELETE FROM model_invocations WHERE tenant_id = $1`,
		`DELETE FROM model_budget_windows WHERE tenant_id = $1`,
		`DELETE FROM model_provider_route_snapshots WHERE tenant_id = $1`,
		`DELETE FROM model_provider_failures WHERE tenant_id = $1`,
		`DELETE FROM knowledge_outbox WHERE tenant_id = $1`,
		`DELETE FROM knowledge_delete_proofs WHERE tenant_id = $1`,
		`DELETE FROM knowledge_chunks WHERE tenant_id = $1`,
		`DELETE FROM knowledge_documents WHERE tenant_id = $1`,
		`DELETE FROM knowledge_ingestion_jobs WHERE tenant_id = $1`,
		`DELETE FROM knowledge_sources WHERE tenant_id = $1`,
	}
	for _, query := range queries {
		if _, err := pool.Exec(ctx, query, tenantID); err != nil {
			return fmt.Errorf("cleanup vector embedding tenant: %w", err)
		}
	}
	return nil
}

func validateConfig(cfg config) error {
	switch cfg.phase {
	case "prepare":
		if strings.TrimSpace(cfg.knowledgeTarget) == "" {
			return errors.New("knowledge-target is required")
		}
		if strings.TrimSpace(cfg.pgDSN) == "" {
			return errors.New("pg-dsn is required")
		}
	case "verify":
		if strings.TrimSpace(cfg.vectorTarget) == "" {
			return errors.New("vector-target is required")
		}
		if strings.TrimSpace(cfg.visibilityScope) == "" || strings.TrimSpace(cfg.policyVersion) == "" {
			return errors.New("visibility-scope and policy-version are required for verify")
		}
		if cfg.expectedCount <= 0 {
			return errors.New("expected-count must be positive")
		}
	case "preflight-pgvector":
		if strings.TrimSpace(cfg.pgVectorDSN) == "" {
			return errors.New("pgvector-dsn is required for preflight-pgvector")
		}
	case "preflight-opensearch-vector":
		if strings.TrimSpace(cfg.openSearchVectorEndpoint) == "" {
			return errors.New("opensearch-vector-endpoint is required for preflight-opensearch-vector")
		}
		if err := validateOpenSearchEndpoint(cfg.openSearchVectorEndpoint); err != nil {
			return err
		}
		if !isSafeOpenSearchIndexName(cfg.openSearchVectorIndex) {
			return fmt.Errorf("unsafe opensearch vector index %q", cfg.openSearchVectorIndex)
		}
		if !isSafeOpenSearchFieldPath(cfg.openSearchVectorField) {
			return fmt.Errorf("unsafe opensearch vector field %q", cfg.openSearchVectorField)
		}
		if cfg.openSearchVectorDimension <= 0 {
			return errors.New("opensearch-vector-dimension must be positive")
		}
	case "verify-queue":
		if strings.TrimSpace(cfg.pgDSN) == "" {
			return errors.New("pg-dsn is required")
		}
		if cfg.expectedCount <= 0 {
			return errors.New("expected-count must be positive")
		}
	case "request-rebuild":
		if strings.TrimSpace(cfg.vectorTarget) == "" {
			return errors.New("vector-target is required")
		}
		if strings.TrimSpace(cfg.pgDSN) == "" {
			return errors.New("pg-dsn is required")
		}
		if strings.TrimSpace(cfg.sourceID) == "" || strings.TrimSpace(cfg.documentID) == "" {
			return errors.New("source-id and document-id are required for request-rebuild")
		}
		if cfg.expectedCount <= 0 {
			return errors.New("expected-count must be positive")
		}
	default:
		return fmt.Errorf("unsupported phase %q", cfg.phase)
	}
	if strings.TrimSpace(cfg.tenantID) == "" || strings.TrimSpace(cfg.userID) == "" {
		return errors.New("tenant-id and user-id are required")
	}
	if strings.TrimSpace(cfg.pgVectorDSN) != "" && strings.TrimSpace(cfg.pgVectorTable) == "" {
		return errors.New("pgvector-table is required when pgvector-dsn is set")
	}
	return nil
}

func quoteSQLIdentifier(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("sql identifier is required")
	}
	parts := strings.Split(value, ".")
	for _, part := range parts {
		if !isSafeSQLIdentifier(part) {
			return "", fmt.Errorf("unsafe sql identifier %q", value)
		}
	}
	for index, part := range parts {
		parts[index] = `"` + part + `"`
	}
	return strings.Join(parts, "."), nil
}

func isSafeSQLIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for index, r := range value {
		if r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || index > 0 && r >= '0' && r <= '9' {
			continue
		}
		return false
	}
	return true
}

func writeSummary(resultDir string, result summary) error {
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(resultDir, "vector-embedding-producer-summary.json")
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
		return "vector-embedding-producer-smoke"
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
