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
	retrievalv1 "github.com/qsyy0921/IM/api/proto/nexusim/retrieval/v1"
	"github.com/qsyy0921/IM/loadtest/internal/grpctls"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	defaultPGDSN           = "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable"
	defaultRetrievalTarget = "127.0.0.1:10590"
	defaultResultRoot      = `H:\NexusIM\loadtest-results`
)

type config struct {
	pgDSN           string
	retrievalTarget string
	resultRoot      string
	runName         string
	tenantID        string
	viewerUserID    string
	senderUserID    string
	deviceID        string
	requestTimeout  time.Duration
	tls             grpctls.Config
}

type seedData struct {
	TenantID                  string `json:"tenant_id"`
	OtherTenantID             string `json:"other_tenant_id"`
	ViewerUserID              string `json:"viewer_user_id"`
	SenderUserID              string `json:"sender_user_id"`
	DeviceID                  string `json:"device_id"`
	MissConversationID        string `json:"miss_conversation_id"`
	TemporalConversationID    string `json:"temporal_conversation_id"`
	AttributionConversationID string `json:"attribution_conversation_id"`
	PrivateConversationID     string `json:"private_conversation_id"`
	SearchMessageID           string `json:"search_message_id"`
	SearchSourceEventID       string `json:"search_source_event_id"`
	ActiveMemoryEventID       string `json:"active_memory_event_id"`
	SupersededMemoryEventID   string `json:"superseded_memory_event_id"`
	AttributionMemoryEventID  string `json:"attribution_memory_event_id"`
	PrivateSearchMessageID    string `json:"private_search_message_id"`
	PrivateSourceEventID      string `json:"private_source_event_id"`
	ProjectionVersion         int64  `json:"projection_version"`
	ConversationSeq           int64  `json:"conversation_seq"`
}

type assertionResult struct {
	Type     string `json:"type"`
	Passed   bool   `json:"passed"`
	Evidence string `json:"evidence,omitempty"`
}

type caseResult struct {
	ID         string            `json:"id"`
	Family     string            `json:"family"`
	Stage      string            `json:"stage"`
	Status     string            `json:"status"`
	Passed     bool              `json:"passed"`
	Query      string            `json:"query"`
	Assertions []assertionResult `json:"assertions"`
}

type summary struct {
	SchemaVersion   int          `json:"schema_version"`
	Adapter         string       `json:"adapter"`
	GeneratedAt     time.Time    `json:"generated_at"`
	Scope           string       `json:"scope"`
	RunName         string       `json:"run_name"`
	ResultDir       string       `json:"result_dir"`
	RetrievalTarget string       `json:"retrieval_target"`
	Seed            seedData     `json:"seed"`
	CaseCount       int          `json:"case_count"`
	PassedCount     int          `json:"passed_count"`
	FailedCount     int          `json:"failed_count"`
	SkippedCount    int          `json:"skipped_count"`
	Cases           []caseResult `json:"cases"`
	StartedAt       time.Time    `json:"started_at"`
	FinishedAt      time.Time    `json:"finished_at"`
}

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "retrieval negative smoke failed: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	cfg, err := parseConfig(args)
	if err != nil {
		return err
	}
	resultDir := filepath.Join(cfg.resultRoot, sanitizeRunName(cfg.runName))
	if err := validateExternalResultDir(resultDir); err != nil {
		return err
	}
	if err := os.MkdirAll(resultDir, 0o755); err != nil {
		return err
	}

	pool, err := openPGPool(ctx, cfg.pgDSN)
	if err != nil {
		return err
	}
	defer pool.Close()

	startedAt := time.Now().UTC()
	if err := applyProjectionMigrations(ctx, pool); err != nil {
		return err
	}
	seed := newSeed(cfg)
	if err := cleanupTenant(ctx, pool, seed.TenantID); err != nil {
		return err
	}
	if err := cleanupTenant(ctx, pool, seed.OtherTenantID); err != nil {
		return err
	}
	if err := seedProjectionRows(ctx, pool, seed); err != nil {
		return err
	}

	client, closeClient, err := newRetrievalClient(cfg)
	if err != nil {
		return err
	}
	defer closeClient()

	cases := []caseResult{}
	for _, runner := range []func(context.Context, config, seedData, retrievalv1.RetrievalGatewayClient) (caseResult, error){
		runSourceCoverageEmptyMemoryCase,
		runTemporalSupersededFilterCase,
		runAttributionSourceRefCase,
		runCrossTenantIsolationCase,
	} {
		caseResult, err := runner(ctx, cfg, seed, client)
		if err != nil {
			return err
		}
		cases = append(cases, caseResult)
	}

	result := buildSummary(cfg, resultDir, seed, cases, startedAt)
	return writeSummary(resultDir, result)
}

func parseConfig(args []string) (config, error) {
	cfg := config{}
	flagSet := flag.NewFlagSet("retrieval-negative-smoke", flag.ContinueOnError)
	flagSet.StringVar(&cfg.pgDSN, "pg-dsn", defaultPGDSN, "PostgreSQL DSN")
	flagSet.StringVar(&cfg.retrievalTarget, "retrieval-target", defaultRetrievalTarget, "retrieval-gateway gRPC address")
	flagSet.StringVar(&cfg.resultRoot, "result-root", defaultResultRoot, "external result root for raw smoke output")
	flagSet.StringVar(&cfg.runName, "run-name", "", "run name under result root")
	flagSet.StringVar(&cfg.tenantID, "tenant-id", "", "tenant id for seeded projection rows")
	flagSet.StringVar(&cfg.viewerUserID, "viewer-user-id", "retrieval-negative-viewer", "viewer user id")
	flagSet.StringVar(&cfg.senderUserID, "sender-user-id", "retrieval-negative-sender", "sender user id")
	flagSet.StringVar(&cfg.deviceID, "device-id", "retrieval-negative-device", "viewer device id")
	flagSet.DurationVar(&cfg.requestTimeout, "request-timeout", 10*time.Second, "gRPC request timeout")
	flagSet.StringVar(&cfg.tls.CAFile, "retrieval-tls-ca-file", "", "retrieval gRPC TLS CA file")
	flagSet.StringVar(&cfg.tls.ServerName, "retrieval-tls-server-name", "", "retrieval gRPC TLS server name")
	flagSet.StringVar(&cfg.tls.ClientCertFile, "retrieval-tls-client-cert-file", "", "retrieval gRPC client certificate")
	flagSet.StringVar(&cfg.tls.ClientKeyFile, "retrieval-tls-client-key-file", "", "retrieval gRPC client key")
	if err := flagSet.Parse(args); err != nil {
		return config{}, err
	}
	cfg.resultRoot = strings.TrimSpace(cfg.resultRoot)
	cfg.retrievalTarget = strings.TrimSpace(cfg.retrievalTarget)
	cfg.viewerUserID = strings.TrimSpace(cfg.viewerUserID)
	cfg.senderUserID = strings.TrimSpace(cfg.senderUserID)
	cfg.deviceID = strings.TrimSpace(cfg.deviceID)
	if cfg.retrievalTarget == "" {
		return config{}, errors.New("--retrieval-target is required")
	}
	if cfg.resultRoot == "" {
		return config{}, errors.New("--result-root is required")
	}
	if cfg.viewerUserID == "" || cfg.senderUserID == "" || cfg.deviceID == "" {
		return config{}, errors.New("viewer, sender and device ids are required")
	}
	if cfg.runName == "" {
		cfg.runName = "retrieval-negative-eval-adapter-" + time.Now().UTC().Format("20060102-150405")
	}
	if strings.TrimSpace(cfg.tenantID) == "" {
		cfg.tenantID = "tenant-retrieval-negative-" + randomSuffix()
	}
	return cfg, nil
}

func newSeed(cfg config) seedData {
	suffix := randomSuffix()
	return seedData{
		TenantID:                  cfg.tenantID,
		OtherTenantID:             cfg.tenantID + "-other",
		ViewerUserID:              cfg.viewerUserID,
		SenderUserID:              cfg.senderUserID,
		DeviceID:                  cfg.deviceID,
		MissConversationID:        "conv-retrieval-miss-" + suffix,
		TemporalConversationID:    "conv-retrieval-temporal-" + suffix,
		AttributionConversationID: "conv-retrieval-attribution-" + suffix,
		PrivateConversationID:     "conv-retrieval-private-" + suffix,
		SearchMessageID:           "msg-retrieval-miss-" + suffix,
		SearchSourceEventID:       "evt-retrieval-miss-" + suffix,
		ActiveMemoryEventID:       "mem-retrieval-active-" + suffix,
		SupersededMemoryEventID:   "mem-retrieval-superseded-" + suffix,
		AttributionMemoryEventID:  "mem-retrieval-attribution-" + suffix,
		PrivateSearchMessageID:    "msg-retrieval-private-" + suffix,
		PrivateSourceEventID:      "evt-retrieval-private-" + suffix,
		ProjectionVersion:         41,
		ConversationSeq:           7,
	}
}

func openPGPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(strings.TrimSpace(dsn))
	if err != nil {
		return nil, err
	}
	connectCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return pgxpool.NewWithConfig(connectCtx, config)
}

func applyProjectionMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	for _, path := range []string{
		filepath.Join("migrations", "postgres", "search", "000001_search_core.sql"),
		filepath.Join("migrations", "postgres", "memory", "000001_memory_core.sql"),
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if _, err := pool.Exec(ctx, string(data)); err != nil {
			return fmt.Errorf("apply %s: %w", path, err)
		}
	}
	return nil
}

func cleanupTenant(ctx context.Context, pool *pgxpool.Pool, tenantID string) error {
	for _, statement := range []string{
		`DELETE FROM memory_graph_edges WHERE tenant_id = $1`,
		`DELETE FROM memory_profile_aggregates WHERE tenant_id = $1`,
		`DELETE FROM memory_event_source_refs WHERE tenant_id = $1`,
		`DELETE FROM memory_structured_events WHERE tenant_id = $1`,
		`DELETE FROM memory_membership_projection WHERE tenant_id = $1`,
		`DELETE FROM search_message_documents WHERE tenant_id = $1`,
		`DELETE FROM search_membership_projection WHERE tenant_id = $1`,
	} {
		if _, err := pool.Exec(ctx, statement, tenantID); err != nil {
			return err
		}
	}
	return nil
}

func seedProjectionRows(ctx context.Context, pool *pgxpool.Pool, seed seedData) error {
	now := time.Now().UTC().Truncate(time.Millisecond)
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for _, table := range []string{"search_membership_projection", "memory_membership_projection"} {
		for _, conversationID := range []string{
			seed.MissConversationID,
			seed.TemporalConversationID,
			seed.AttributionConversationID,
			seed.PrivateConversationID,
		} {
			if _, err := tx.Exec(ctx, fmt.Sprintf(`
INSERT INTO %s (
	tenant_id, conversation_id, user_id, role, status, join_seq, leave_seq,
	member_version, permission_version, updated_by_event_id, updated_at
) VALUES ($1, $2, $3, 'MEMBER', 'ACTIVE', 1, NULL, 1, $4, $5, $6)
`, table),
				seed.TenantID, conversationID, seed.ViewerUserID, seed.ProjectionVersion,
				"member-seed-"+conversationID, now,
			); err != nil {
				return err
			}
		}
	}

	if _, err := tx.Exec(ctx, `
INSERT INTO search_message_documents (
	tenant_id, conversation_id, message_id, conversation_seq, source_event_id,
	searchable_text, message_type, sender_id, tombstone_status, change_version,
	visibility_version, occurred_at, updated_at
) VALUES
	($1, $2, $3, $4, $5, $6, 'TEXT', $7, 'NONE', 1, $8, $9, $9),
	($10, $11, $12, $4, $13, $14, 'TEXT', $7, 'NONE', 1, $8, $9, $9)
`, seed.TenantID, seed.MissConversationID, seed.SearchMessageID, seed.ConversationSeq,
		seed.SearchSourceEventID, "project launch blocker visible through search only",
		seed.SenderUserID, seed.ProjectionVersion, now,
		seed.TenantID, seed.PrivateConversationID, seed.PrivateSearchMessageID,
		seed.PrivateSourceEventID, "private roadmap status must not cross tenant boundary"); err != nil {
		return err
	}

	actorJSON := jsonStringArray(seed.SenderUserID)
	audienceJSON := jsonStringArray(seed.ViewerUserID)
	if _, err := tx.Exec(ctx, `
INSERT INTO memory_structured_events (
	tenant_id, memory_event_id, scope_type, scope_id, conversation_id, topic,
	event_type, status, review_state, fact_text, actor_user_ids, audience_user_ids,
	valid_from_seq, valid_to_seq, valid_from_at, valid_to_at, supersedes_event_ids,
	contradicts_event_ids, confidence, visibility_version, extraction_version,
	source_projection_version, created_at, updated_at
) VALUES
	($1, $2, 'CONVERSATION', $4, $4, 'rollout-decision',
	 'DECISION', 'ACTIVE', 'APPROVED', 'current rollout decision is owned by the integration lead',
	 $5::jsonb, $6::jsonb, $7, NULL, $8, NULL, $9::jsonb,
	 '[]'::jsonb, 0.93, $10, 'retrieval-negative-v1', $10, $8, $8),
	($1, $3, 'CONVERSATION', $4, $4, 'rollout-decision',
	 'DECISION', 'SUPERSEDED', 'APPROVED', 'current rollout decision used to be owned by the old lead',
	 $5::jsonb, $6::jsonb, 1, $7, $8, NULL, '[]'::jsonb,
	 '[]'::jsonb, 0.91, $10, 'retrieval-negative-v1', $10, $8, $8),
	($1, $11, 'CONVERSATION', $12, $12, 'integration-follow-up',
	 'TASK', 'ACTIVE', 'APPROVED', 'who owns the integration follow up is the integration lead',
	 $5::jsonb, $6::jsonb, $7, NULL, $8, NULL, '[]'::jsonb,
	 '[]'::jsonb, 0.89, $10, 'retrieval-negative-v1', $10, $8, $8)
`, seed.TenantID, seed.ActiveMemoryEventID, seed.SupersededMemoryEventID,
		seed.TemporalConversationID, actorJSON, audienceJSON, seed.ConversationSeq, now,
		jsonStringArray(seed.SupersededMemoryEventID), seed.ProjectionVersion,
		seed.AttributionMemoryEventID, seed.AttributionConversationID); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `
INSERT INTO memory_event_source_refs (
	tenant_id, memory_event_id, source_ref_id, source_type, source_id,
	source_event_id, conversation_id, conversation_seq, occurred_at, created_at
) VALUES
	($1, $2, 'ref-active', 'MESSAGE', 'msg-rollout-active', 'evt-rollout-active', $3, $4, $5, $5),
	($1, $6, 'ref-superseded', 'MESSAGE', 'msg-rollout-old', 'evt-rollout-old', $3, 1, $5, $5),
	($1, $7, 'ref-attribution', 'MESSAGE', 'msg-integration-follow-up', 'evt-integration-follow-up', $8, $4, $5, $5)
`, seed.TenantID, seed.ActiveMemoryEventID, seed.TemporalConversationID, seed.ConversationSeq, now,
		seed.SupersededMemoryEventID, seed.AttributionMemoryEventID, seed.AttributionConversationID); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func newRetrievalClient(cfg config) (retrievalv1.RetrievalGatewayClient, func(), error) {
	dialOption, err := grpctls.DialOption(cfg.tls, "retrieval-tls")
	if err != nil {
		return nil, nil, err
	}
	conn, err := grpc.NewClient("passthrough:///"+cfg.retrievalTarget, dialOption)
	if err != nil {
		return nil, nil, err
	}
	return retrievalv1.NewRetrievalGatewayClient(conn), func() { _ = conn.Close() }, nil
}

func retrieve(
	ctx context.Context,
	cfg config,
	client retrievalv1.RetrievalGatewayClient,
	tenantID string,
	conversationID string,
	query string,
	includeSearch bool,
	includeMemory bool,
) (*retrievalv1.RetrieveEvidenceResponse, error) {
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	return client.RetrieveEvidence(requestCtx, &retrievalv1.RetrieveEvidenceRequest{
		AuthContext: &retrievalv1.AuthContext{
			TenantId:  tenantID,
			UserId:    cfg.viewerUserID,
			DeviceId:  cfg.deviceID,
			SessionId: "retrieval-negative-session",
			TraceId:   "retrieval-negative-trace",
			RequestId: "retrieval-negative-request",
		},
		Query:             query,
		ConversationId:    conversationID,
		IncludeSearch:     includeSearch,
		IncludeMemory:     includeMemory,
		AtConversationSeq: 9,
		Limit:             10,
	})
}

func runSourceCoverageEmptyMemoryCase(
	ctx context.Context,
	cfg config,
	seed seedData,
	client retrievalv1.RetrievalGatewayClient,
) (caseResult, error) {
	response, err := retrieve(ctx, cfg, client, seed.TenantID, seed.MissConversationID, "project launch blocker", false, false)
	if err != nil {
		return caseResult{}, err
	}
	pack := response.GetPack()
	searchReturned := countItems(pack, retrievalv1.EvidenceSourceType_EVIDENCE_SOURCE_TYPE_SEARCH_MESSAGE) > 0
	memoryEmpty := coverageStatus(pack, retrievalv1.EvidenceSourceType_EVIDENCE_SOURCE_TYPE_MEMORY_EVENT) ==
		retrievalv1.EvidenceSourceCoverageStatus_EVIDENCE_SOURCE_COVERAGE_STATUS_EMPTY
	return requireCasePass(caseResult{
		ID:     "retrieval-source-coverage-empty-memory",
		Family: "retrieval_miss",
		Stage:  "retrieval-gateway",
		Status: "active",
		Query:  "project launch blocker",
		Assertions: []assertionResult{
			{Type: "source_coverage_status", Passed: memoryEmpty, Evidence: coverageEvidence(pack, retrievalv1.EvidenceSourceType_EVIDENCE_SOURCE_TYPE_MEMORY_EVENT)},
			{Type: "must_return_source_type", Passed: searchReturned, Evidence: "SEARCH_MESSAGE count > 0"},
		},
	})
}

func runTemporalSupersededFilterCase(
	ctx context.Context,
	cfg config,
	seed seedData,
	client retrievalv1.RetrievalGatewayClient,
) (caseResult, error) {
	response, err := retrieve(ctx, cfg, client, seed.TenantID, seed.TemporalConversationID, "current rollout decision", false, true)
	if err != nil {
		return caseResult{}, err
	}
	pack := response.GetPack()
	activeReturned := false
	supersededLeaked := false
	for _, item := range pack.GetItems() {
		if item.GetSourceType() != retrievalv1.EvidenceSourceType_EVIDENCE_SOURCE_TYPE_MEMORY_EVENT {
			continue
		}
		if item.GetMemoryEventId() == seed.ActiveMemoryEventID && item.GetTemporalStatus() == "ACTIVE" {
			activeReturned = true
		}
		if item.GetMemoryEventId() == seed.SupersededMemoryEventID || item.GetTemporalStatus() == "SUPERSEDED" {
			supersededLeaked = true
		}
	}
	return requireCasePass(caseResult{
		ID:     "retrieval-temporal-superseded-filter",
		Family: "temporal_version",
		Stage:  "retrieval-gateway",
		Status: "active",
		Query:  "current rollout decision",
		Assertions: []assertionResult{
			{Type: "must_return_source_type", Passed: activeReturned, Evidence: "ACTIVE MEMORY_EVENT returned"},
			{Type: "must_exclude_source_ref", Passed: !supersededLeaked, Evidence: "SUPERSEDED memory event absent from EvidencePack"},
		},
	})
}

func runAttributionSourceRefCase(
	ctx context.Context,
	cfg config,
	seed seedData,
	client retrievalv1.RetrievalGatewayClient,
) (caseResult, error) {
	response, err := retrieve(ctx, cfg, client, seed.TenantID, seed.AttributionConversationID, "who owns the integration follow up", false, true)
	if err != nil {
		return caseResult{}, err
	}
	pack := response.GetPack()
	sourceRefPresent := false
	dedupeReasonOK := false
	for _, item := range pack.GetItems() {
		if item.GetMemoryEventId() != seed.AttributionMemoryEventID {
			continue
		}
		for _, ref := range item.GetSourceRefs() {
			if ref.GetSourceType() == "MESSAGE" && strings.TrimSpace(ref.GetSourceId()) != "" {
				sourceRefPresent = true
			}
		}
		switch item.GetDedupeReason() {
		case "UNIQUE_SOURCE", "KEPT_FIRST_DUPLICATE_SOURCE":
			dedupeReasonOK = true
		}
	}
	return requireCasePass(caseResult{
		ID:     "retrieval-attribution-source-ref-required",
		Family: "attribution",
		Stage:  "retrieval-gateway",
		Status: "active",
		Query:  "who owns the integration follow up",
		Assertions: []assertionResult{
			{Type: "must_include_source_ref", Passed: sourceRefPresent, Evidence: "MEMORY_EVENT carries MESSAGE source ref"},
			{Type: "dedupe_reason", Passed: dedupeReasonOK, Evidence: "dedupe_reason is UNIQUE_SOURCE or KEPT_FIRST_DUPLICATE_SOURCE"},
		},
	})
}

func runCrossTenantIsolationCase(
	ctx context.Context,
	cfg config,
	seed seedData,
	client retrievalv1.RetrievalGatewayClient,
) (caseResult, error) {
	response, err := retrieve(ctx, cfg, client, seed.OtherTenantID, seed.PrivateConversationID, "private roadmap status", false, false)
	denied := false
	noSearchLeak := true
	evidence := "cross-tenant request returned empty EvidencePack"
	if err != nil {
		if status.Code(err) == codes.PermissionDenied {
			denied = true
			evidence = "retrieval-gateway returned PermissionDenied"
		} else {
			return caseResult{}, err
		}
	} else {
		pack := response.GetPack()
		for _, item := range pack.GetItems() {
			if item.GetSourceType() == retrievalv1.EvidenceSourceType_EVIDENCE_SOURCE_TYPE_SEARCH_MESSAGE {
				noSearchLeak = false
			}
		}
		denied = len(pack.GetItems()) == 0
	}
	return requireCasePass(caseResult{
		ID:     "retrieval-cross-tenant-permission-deny",
		Family: "permission_leak",
		Stage:  "retrieval-gateway",
		Status: "active",
		Query:  "private roadmap status",
		Assertions: []assertionResult{
			{Type: "must_deny", Passed: denied, Evidence: evidence},
			{Type: "must_not_return_source_type", Passed: noSearchLeak, Evidence: "SEARCH_MESSAGE absent for cross-tenant auth"},
		},
	})
}

func requireCasePass(result caseResult) (caseResult, error) {
	result.Passed = true
	for _, assertion := range result.Assertions {
		if !assertion.Passed {
			result.Passed = false
		}
	}
	if !result.Passed {
		return result, fmt.Errorf("retrieval negative case failed: %s", result.ID)
	}
	return result, nil
}

func countItems(pack *retrievalv1.EvidencePack, sourceType retrievalv1.EvidenceSourceType) int {
	count := 0
	for _, item := range pack.GetItems() {
		if item.GetSourceType() == sourceType {
			count++
		}
	}
	return count
}

func coverageStatus(
	pack *retrievalv1.EvidencePack,
	sourceType retrievalv1.EvidenceSourceType,
) retrievalv1.EvidenceSourceCoverageStatus {
	for _, coverage := range pack.GetSourceCoverage() {
		if coverage.GetSourceType() == sourceType {
			return coverage.GetStatus()
		}
	}
	return retrievalv1.EvidenceSourceCoverageStatus_EVIDENCE_SOURCE_COVERAGE_STATUS_UNSPECIFIED
}

func coverageEvidence(pack *retrievalv1.EvidencePack, sourceType retrievalv1.EvidenceSourceType) string {
	for _, coverage := range pack.GetSourceCoverage() {
		if coverage.GetSourceType() == sourceType {
			return fmt.Sprintf("requested=%t candidate=%d returned=%d status=%s",
				coverage.GetRequested(),
				coverage.GetCandidateCount(),
				coverage.GetReturnedCount(),
				coverage.GetStatus().String(),
			)
		}
	}
	return "coverage row missing"
}

func buildSummary(cfg config, resultDir string, seed seedData, cases []caseResult, startedAt time.Time) summary {
	passed := 0
	failed := 0
	for _, item := range cases {
		if item.Passed {
			passed++
		} else {
			failed++
		}
	}
	return summary{
		SchemaVersion:   1,
		Adapter:         "retrieval-gateway-negative",
		GeneratedAt:     time.Now().UTC(),
		Scope:           "first-stage Retrieval negative/miss eval adapter; uses seeded search/memory projections plus live retrieval-gateway gRPC; not a production benchmark",
		RunName:         cfg.runName,
		ResultDir:       resultDir,
		RetrievalTarget: cfg.retrievalTarget,
		Seed:            seed,
		CaseCount:       len(cases),
		PassedCount:     passed,
		FailedCount:     failed,
		SkippedCount:    0,
		Cases:           cases,
		StartedAt:       startedAt,
		FinishedAt:      time.Now().UTC(),
	}
}

func writeSummary(resultDir string, result summary) error {
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(resultDir, "retrieval-negative-eval-adapter-summary.json")
	if err := os.WriteFile(path, append(encoded, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Println(string(encoded))
	fmt.Printf("summary: %s\n", path)
	return nil
}

func jsonStringArray(values ...string) string {
	encoded, _ := json.Marshal(values)
	return string(encoded)
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
		return "retrieval-negative-smoke"
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

func gitOutput(args ...string) string {
	command := exec.Command("git", args...)
	out, err := command.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
