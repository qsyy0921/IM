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
	knowledgeTarget string
	vectorTarget    string
	pgDSN           string
	resultDir       string
	requestTimeout  time.Duration
	tenantID        string
	userID          string
	idempotencyKey  string
	cleanup         bool
	applyMigration  bool
	traceID         string
}

type summary struct {
	Commit              string              `json:"commit"`
	CommitFull          string              `json:"commit_full"`
	GitDirty            bool                `json:"git_dirty"`
	GitStatusShort      string              `json:"git_status_short,omitempty"`
	ResultDir           string              `json:"result_dir"`
	KnowledgeTarget     string              `json:"knowledge_target"`
	VectorTarget        string              `json:"vector_target"`
	TenantID            string              `json:"tenant_id"`
	UserID              string              `json:"user_id"`
	StartedAt           time.Time           `json:"started_at"`
	FinishedAt          time.Time           `json:"finished_at"`
	Success             bool                `json:"success"`
	Error               string              `json:"error,omitempty"`
	KnowledgeSourceID   string              `json:"knowledge_source_id"`
	KnowledgeJobID      string              `json:"knowledge_job_id"`
	DocumentID          string              `json:"document_id"`
	KnowledgeChunkCount int                 `json:"knowledge_chunk_count"`
	VectorUpserts       []vectorUpsertState `json:"vector_upserts"`
	VectorSearchCount   int                 `json:"vector_search_count"`
}

type vectorUpsertState struct {
	ChunkID           string `json:"chunk_id"`
	ChunkHash         string `json:"chunk_hash"`
	VectorItemID      string `json:"vector_item_id"`
	VectorItemRefHash string `json:"vector_item_ref_hash"`
	VectorJobID       string `json:"vector_job_id"`
	CollectionType    string `json:"collection_type"`
	VisibilityScope   string `json:"visibility_scope"`
	PolicyVersion     string `json:"policy_version"`
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
	flag.StringVar(&cfg.knowledgeTarget, "knowledge-target", envOr("NEXUSIM_KNOWLEDGE_INGESTION_GRPC_ADDR", "127.0.0.1:10740"), "knowledge-ingestion-service gRPC target")
	flag.StringVar(&cfg.vectorTarget, "vector-target", envOr("NEXUSIM_VECTOR_INDEX_GRPC_ADDR", "127.0.0.1:10760"), "vector-index-service gRPC target")
	flag.StringVar(&cfg.pgDSN, "pg-dsn", envOr("NEXUSIM_PG_DSN", "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable"), "PostgreSQL DSN")
	flag.StringVar(&resultRoot, "result-root", defaultResultRoot, "external result root for raw smoke output")
	flag.StringVar(&runName, "run-name", "", "run name under result-root")
	flag.DurationVar(&cfg.requestTimeout, "request-timeout", 3*time.Second, "per request timeout")
	flag.StringVar(&cfg.tenantID, "tenant-id", "", "tenant id; defaults to tenant derived from run name")
	flag.StringVar(&cfg.userID, "user-id", "knowledge-vector-smoke-user", "requesting user id")
	flag.StringVar(&cfg.idempotencyKey, "idempotency-key", "", "idempotency key; defaults to key derived from run name")
	flag.BoolVar(&cfg.cleanup, "cleanup", true, "delete smoke tenant rows before running")
	flag.BoolVar(&cfg.applyMigration, "apply-migration", true, "apply knowledge/vector migrations before running")
	flag.Parse()

	if runName == "" {
		runName = "knowledge-vector-handoff-smoke-" + time.Now().Format("20060102-150405")
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
		cfg.requestTimeout = 3 * time.Second
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
		Commit:          gitOutput("rev-parse", "--short", "HEAD"),
		CommitFull:      gitOutput("rev-parse", "HEAD"),
		GitStatusShort:  gitOutput("status", "--short"),
		ResultDir:       cfg.resultDir,
		KnowledgeTarget: cfg.knowledgeTarget,
		VectorTarget:    cfg.vectorTarget,
		TenantID:        cfg.tenantID,
		UserID:          cfg.userID,
		StartedAt:       time.Now().UTC(),
	}
	result.GitDirty = strings.TrimSpace(result.GitStatusShort) != ""
	defer func() {
		result.FinishedAt = time.Now().UTC()
		_ = writeSummary(cfg.resultDir, result)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, cfg.pgDSN)
	if err != nil {
		result.Error = "open postgres: " + err.Error()
		return fmt.Errorf("open postgres: %w", err)
	}
	defer pool.Close()
	if cfg.applyMigration {
		if err := applyMigrations(ctx, pool); err != nil {
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

	knowledgeConn, err := grpc.DialContext(ctx, cfg.knowledgeTarget, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		result.Error = "dial knowledge-ingestion-service: " + err.Error()
		return fmt.Errorf("dial knowledge-ingestion-service: %w", err)
	}
	defer knowledgeConn.Close()
	vectorConn, err := grpc.DialContext(ctx, cfg.vectorTarget, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		result.Error = "dial vector-index-service: " + err.Error()
		return fmt.Errorf("dial vector-index-service: %w", err)
	}
	defer vectorConn.Close()

	knowledgeClient := knowledgev1.NewKnowledgeIngestionServiceClient(knowledgeConn)
	vectorClient := vectorv1.NewVectorIndexServiceClient(vectorConn)

	source, err := createKnowledgeSource(ctx, knowledgeClient, cfg)
	if err != nil {
		result.Error = "create knowledge source: " + err.Error()
		return err
	}
	result.KnowledgeSourceID = source.GetSource().GetSourceId()
	if result.KnowledgeSourceID == "" {
		result.Error = "knowledge source id missing"
		return errors.New(result.Error)
	}

	job, err := submitKnowledgeJob(ctx, knowledgeClient, cfg, source.GetSource())
	if err != nil {
		result.Error = "submit knowledge job: " + err.Error()
		return err
	}
	result.KnowledgeJobID = job.GetJob().GetJobId()
	result.DocumentID = job.GetDocumentId()
	if result.KnowledgeJobID == "" || result.DocumentID == "" {
		result.Error = "knowledge job or document id missing"
		return errors.New(result.Error)
	}

	chunks, err := listKnowledgeChunks(ctx, knowledgeClient, cfg, result.KnowledgeSourceID, result.DocumentID)
	if err != nil {
		result.Error = "list knowledge chunks: " + err.Error()
		return err
	}
	result.KnowledgeChunkCount = len(chunks)
	if result.KnowledgeChunkCount != 2 {
		result.Error = fmt.Sprintf("expected 2 knowledge chunks, got %d", result.KnowledgeChunkCount)
		return errors.New(result.Error)
	}

	for index, chunk := range chunks {
		upsert, err := upsertVectorFromChunk(ctx, vectorClient, cfg, source.GetSource(), chunk, index)
		if err != nil {
			result.Error = "upsert vector from knowledge chunk: " + err.Error()
			return err
		}
		state := vectorUpsertState{
			ChunkID:           chunk.GetChunkId(),
			ChunkHash:         chunk.GetChunkHash(),
			VectorItemID:      upsert.GetItem().GetVectorItemId(),
			VectorItemRefHash: hashRef(upsert.GetItem().GetVectorItemId()),
			VectorJobID:       upsert.GetJob().GetJobId(),
			CollectionType:    upsert.GetItem().GetCollectionType(),
			VisibilityScope:   chunk.GetVisibilityScope(),
			PolicyVersion:     chunk.GetPolicyVersion(),
		}
		if state.VectorItemID == "" || state.VectorJobID == "" {
			result.Error = "vector upsert response missing item or job id"
			return errors.New(result.Error)
		}
		result.VectorUpserts = append(result.VectorUpserts, state)
	}

	search, err := searchVectors(ctx, vectorClient, cfg, chunks[0].GetVisibilityScope(), chunks[0].GetPolicyVersion())
	if err != nil {
		result.Error = "search vector handoff results: " + err.Error()
		return err
	}
	result.VectorSearchCount = len(search.GetResults())
	if result.VectorSearchCount != result.KnowledgeChunkCount {
		result.Error = fmt.Sprintf("expected %d vector search results, got %d", result.KnowledgeChunkCount, result.VectorSearchCount)
		return errors.New(result.Error)
	}
	result.Success = true
	return nil
}

func createKnowledgeSource(ctx context.Context, client knowledgev1.KnowledgeIngestionServiceClient, cfg config) (*knowledgev1.CreateKnowledgeSourceResponse, error) {
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	return client.CreateKnowledgeSource(requestCtx, &knowledgev1.CreateKnowledgeSourceRequest{
		AuthContext:        knowledgeAuth(cfg, "knowledge-ingestion-service", "knowledge-vector-source"),
		SourceType:         "MANUAL_MARKDOWN",
		SourceRef:          "knowledge-vector-smoke-source",
		SourceUriHash:      hashRef(cfg.tenantID + "|source-uri"),
		MediaObjectRef:     "media-object-ref-smoke",
		OwnerRef:           "owner-ref-smoke",
		VisibilityScope:    "conversation:knowledge-vector-smoke",
		DataClass:          "BUSINESS_INTERNAL",
		ContentHash:        hashRef(cfg.tenantID + "|content"),
		MimeType:           "text/markdown",
		SizeBytes:          128,
		SourceVersion:      "1",
		RetentionPolicyRef: "retention-knowledge-vector-smoke",
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
			ChunkPreviewRedacted: "redacted chunk 0",
			VisibilityScope:      source.GetVisibilityScope(),
			DataClass:            source.GetDataClass(),
			PolicyVersion:        "policy-knowledge-vector-smoke",
			ChunkVersion:         "1",
		},
		{
			ChunkHash:            hashRef(cfg.tenantID + "|chunk|1"),
			ChunkPreviewRedacted: "redacted chunk 1",
			VisibilityScope:      source.GetVisibilityScope(),
			DataClass:            source.GetDataClass(),
			PolicyVersion:        "policy-knowledge-vector-smoke",
			ChunkVersion:         "1",
		},
	}
	return client.SubmitIngestionJob(requestCtx, &knowledgev1.SubmitIngestionJobRequest{
		AuthContext:        knowledgeAuth(cfg, "knowledge-ingestion-service", "knowledge-vector-job"),
		SourceId:           source.GetSourceId(),
		SourceVersion:      source.GetSourceVersion(),
		JobType:            "INGEST",
		ParserProfile:      "local-manifest-v1",
		ChunkProfile:       "fixed-manifest-v1",
		EmbeddingPolicyRef: "embedding-policy-smoke",
		VectorPolicyRef:    "vector-policy-smoke",
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
		AuthContext: knowledgeAuth(cfg, "knowledge-ingestion-service", "knowledge-vector-list"),
		SourceId:    sourceID,
		DocumentId:  documentID,
		PageSize:    10,
	})
	if err != nil {
		return nil, err
	}
	return response.GetChunks(), nil
}

func upsertVectorFromChunk(ctx context.Context, client vectorv1.VectorIndexServiceClient, cfg config, source *knowledgev1.KnowledgeSource, chunk *knowledgev1.KnowledgeChunk, index int) (*vectorv1.UpsertVectorItemResponse, error) {
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	return client.UpsertVectorItem(requestCtx, &vectorv1.UpsertVectorItemRequest{
		AuthContext:         vectorAuth(cfg, "knowledge-ingestion-service", fmt.Sprintf("knowledge-vector-upsert-%d", index)),
		SourceService:       "knowledge-ingestion-service",
		CollectionType:      "KNOWLEDGE_CHUNK",
		SourceRefHash:       source.GetSourceRefHash(),
		SourceId:            chunk.GetChunkId(),
		SourceVersion:       int64(index + 1),
		SourceHash:          source.GetContentHash(),
		ChunkHash:           chunk.GetChunkHash(),
		EmbeddingModelRef:   "embedding-model-handoff-v1",
		EmbeddingVectorHash: hashRef(cfg.tenantID + "|embedding|" + chunk.GetChunkId()),
		Dimension:           3,
		VisibilityScope:     chunk.GetVisibilityScope(),
		VisibilityVersion:   1,
		PolicyVersion:       chunk.GetPolicyVersion(),
		DataClass:           chunk.GetDataClass(),
		DeleteProofId:       chunk.GetDeleteProofId(),
		RetentionPolicyRef:  source.GetRetentionPolicyRef(),
		IdempotencyKey:      cfg.idempotencyKey + "-vector-" + fmt.Sprint(index),
		CorrelationId:       cfg.idempotencyKey,
		CausationId:         cfg.idempotencyKey,
		TraceId:             cfg.traceID,
	})
}

func searchVectors(ctx context.Context, client vectorv1.VectorIndexServiceClient, cfg config, visibilityScope string, policyVersion string) (*vectorv1.SearchVectorsResponse, error) {
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	return client.SearchVectors(requestCtx, &vectorv1.SearchVectorsRequest{
		AuthContext:        vectorAuth(cfg, "retrieval-gateway", "knowledge-vector-search"),
		RequesterRef:       "requester-ref-knowledge-vector-smoke",
		RetrievalRequestId: "retrieval-" + sanitizeRunName(cfg.idempotencyKey),
		CollectionTypes:    []string{"KNOWLEDGE_CHUNK"},
		QueryEmbeddingRef:  hashRef(cfg.tenantID + "|query|" + cfg.idempotencyKey),
		TopK:               10,
		MinScore:           0,
		VisibilityScope:    visibilityScope,
		PolicyVersion:      policyVersion,
		AtUnixMs:           time.Now().UnixMilli(),
	})
}

func knowledgeAuth(cfg config, serviceName string, requestID string) *knowledgev1.AuthContext {
	return &knowledgev1.AuthContext{
		TenantId:    cfg.tenantID,
		UserId:      cfg.userID,
		ServiceName: serviceName,
		InstanceRef: "loadtest-knowledge-vector",
		TraceId:     cfg.traceID,
		RequestId:   requestID,
	}
}

func vectorAuth(cfg config, serviceName string, requestID string) *vectorv1.AuthContext {
	return &vectorv1.AuthContext{
		TenantId:    cfg.tenantID,
		UserId:      cfg.userID,
		ServiceName: serviceName,
		InstanceRef: "loadtest-knowledge-vector",
		TraceId:     cfg.traceID,
		RequestId:   requestID,
	}
}

func applyMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	for _, dir := range []string{
		filepath.Join("migrations", "postgres", "knowledge-ingestion"),
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
		`DELETE FROM vector_outbox WHERE tenant_id = $1`,
		`DELETE FROM vector_rebuild_checkpoints WHERE tenant_id = $1`,
		`DELETE FROM vector_tombstones WHERE tenant_id = $1`,
		`DELETE FROM vector_index_jobs WHERE tenant_id = $1`,
		`DELETE FROM vector_items WHERE tenant_id = $1`,
		`DELETE FROM vector_collections WHERE tenant_id = $1`,
		`DELETE FROM knowledge_outbox WHERE tenant_id = $1`,
		`DELETE FROM knowledge_delete_proofs WHERE tenant_id = $1`,
		`DELETE FROM knowledge_chunks WHERE tenant_id = $1`,
		`DELETE FROM knowledge_documents WHERE tenant_id = $1`,
		`DELETE FROM knowledge_ingestion_jobs WHERE tenant_id = $1`,
		`DELETE FROM knowledge_sources WHERE tenant_id = $1`,
	}
	for _, query := range queries {
		if _, err := pool.Exec(ctx, query, tenantID); err != nil {
			return fmt.Errorf("cleanup handoff tenant: %w", err)
		}
	}
	return nil
}

func validateConfig(cfg config) error {
	if strings.TrimSpace(cfg.knowledgeTarget) == "" || strings.TrimSpace(cfg.vectorTarget) == "" {
		return errors.New("knowledge-target and vector-target are required")
	}
	if strings.TrimSpace(cfg.pgDSN) == "" {
		return errors.New("pg-dsn is required")
	}
	if strings.TrimSpace(cfg.tenantID) == "" || strings.TrimSpace(cfg.userID) == "" {
		return errors.New("tenant-id and user-id are required")
	}
	return nil
}

func writeSummary(resultDir string, result summary) error {
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(resultDir, "knowledge-vector-handoff-summary.json")
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
		return "knowledge-vector-smoke"
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

func gitOutput(args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", args...).CombinedOutput()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
