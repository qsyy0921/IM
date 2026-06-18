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
)

const (
	defaultPGDSN           = "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable"
	defaultRetrievalTarget = "127.0.0.1:10590"
	defaultResultRoot      = `H:\NexusIM\loadtest-results`
	defaultQuery           = "phoenix launch decision"
)

type config struct {
	pgDSN           string
	retrievalTarget string
	resultRoot      string
	runName         string
	tenantID        string
	conversationID  string
	viewerUserID    string
	senderUserID    string
	deviceID        string
	query           string
	requestTimeout  time.Duration
	tls             grpctls.Config
}

type seededData struct {
	TenantID            string `json:"tenant_id"`
	ConversationID      string `json:"conversation_id"`
	ViewerUserID        string `json:"viewer_user_id"`
	SenderUserID        string `json:"sender_user_id"`
	MessageID           string `json:"message_id"`
	SourceEventID       string `json:"source_event_id"`
	MemoryEventID       string `json:"memory_event_id"`
	MemorySourceRefID   string `json:"memory_source_ref_id"`
	ConversationSeq     int64  `json:"conversation_seq"`
	VisibilityVersion   int64  `json:"visibility_version"`
	MemoryValidFromSeq  int64  `json:"memory_valid_from_seq"`
	MemoryValidToSeq    int64  `json:"memory_valid_to_seq"`
	MemoryProjectionVer int64  `json:"memory_projection_version"`
}

type evidenceSummary struct {
	RunName                 string       `json:"run_name"`
	ResultDir               string       `json:"result_dir"`
	RetrievalTarget         string       `json:"retrieval_target"`
	Query                   string       `json:"query"`
	Seed                    seededData   `json:"seed"`
	PackID                  string       `json:"pack_id"`
	ItemCount               int          `json:"item_count"`
	SearchItemCount         int          `json:"search_item_count"`
	MemoryItemCount         int          `json:"memory_item_count"`
	SourceCounts            sourceCounts `json:"source_counts"`
	SearchProjectionVersion int64        `json:"search_projection_version"`
	MemoryProjectionVersion int64        `json:"memory_projection_version"`
	RetrievalVersion        string       `json:"retrieval_version"`
	Verified                []string     `json:"verified"`
	StartedAt               time.Time    `json:"started_at"`
	FinishedAt              time.Time    `json:"finished_at"`
}

type sourceCounts struct {
	SearchMessage int32 `json:"search_message"`
	MemoryEvent   int32 `json:"memory_event"`
}

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "retrieval smoke failed: %v\n", err)
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
	if err := cleanupTenant(ctx, pool, cfg.tenantID); err != nil {
		return err
	}
	seed, err := seedProjectionRows(ctx, pool, cfg)
	if err != nil {
		return err
	}

	response, err := retrieveEvidence(ctx, cfg)
	if err != nil {
		return err
	}
	summary, err := verifyEvidence(cfg, resultDir, seed, response, startedAt)
	if err != nil {
		return err
	}
	return writeSummary(resultDir, summary)
}

func parseConfig(args []string) (config, error) {
	cfg := config{}
	flagSet := flag.NewFlagSet("retrieval-smoke", flag.ContinueOnError)
	flagSet.StringVar(&cfg.pgDSN, "pg-dsn", defaultPGDSN, "PostgreSQL DSN")
	flagSet.StringVar(&cfg.retrievalTarget, "retrieval-target", defaultRetrievalTarget, "retrieval-gateway gRPC address")
	flagSet.StringVar(&cfg.resultRoot, "result-root", defaultResultRoot, "external result root for raw smoke output")
	flagSet.StringVar(&cfg.runName, "run-name", "", "run name under result root")
	flagSet.StringVar(&cfg.tenantID, "tenant-id", "", "tenant id for seeded projection rows")
	flagSet.StringVar(&cfg.conversationID, "conversation-id", "", "conversation id for seeded projection rows")
	flagSet.StringVar(&cfg.viewerUserID, "viewer-user-id", "retrieval-viewer", "viewer user id")
	flagSet.StringVar(&cfg.senderUserID, "sender-user-id", "retrieval-sender", "sender user id")
	flagSet.StringVar(&cfg.deviceID, "device-id", "retrieval-device", "viewer device id")
	flagSet.StringVar(&cfg.query, "query", defaultQuery, "query used for search and memory")
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
	cfg.query = strings.TrimSpace(cfg.query)
	if cfg.retrievalTarget == "" {
		return config{}, errors.New("--retrieval-target is required")
	}
	if cfg.resultRoot == "" {
		return config{}, errors.New("--result-root is required")
	}
	if cfg.query == "" {
		return config{}, errors.New("--query is required")
	}
	if cfg.runName == "" {
		cfg.runName = "retrieval-gateway-evidence-smoke-" + time.Now().UTC().Format("20060102-150405")
	}
	suffix := randomSuffix()
	if strings.TrimSpace(cfg.tenantID) == "" {
		cfg.tenantID = "tenant-retrieval-smoke-" + suffix
	}
	if strings.TrimSpace(cfg.conversationID) == "" {
		cfg.conversationID = "conv-retrieval-smoke-" + suffix
	}
	return cfg, nil
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

func seedProjectionRows(ctx context.Context, pool *pgxpool.Pool, cfg config) (seededData, error) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	messageID := "msg-retrieval-" + randomSuffix()
	sourceEventID := "evt-retrieval-" + randomSuffix()
	memoryEventID := "mem-retrieval-" + randomSuffix()
	sourceRefID := "ref-retrieval-" + randomSuffix()
	seq := int64(2)
	visibilityVersion := int64(17)
	memoryProjectionVersion := int64(23)
	searchText := "The phoenix launch decision requires the retrieval gateway evidence pack smoke to preserve citations."
	factText := "Phoenix launch decision requires EvidencePack source refs and temporal version preservation."

	tx, err := pool.Begin(ctx)
	if err != nil {
		return seededData{}, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	for _, table := range []string{"search_membership_projection", "memory_membership_projection"} {
		if _, err := tx.Exec(ctx, fmt.Sprintf(`
INSERT INTO %s (
	tenant_id, conversation_id, user_id, role, status, join_seq, leave_seq,
	member_version, permission_version, updated_by_event_id, updated_at
) VALUES ($1, $2, $3, 'MEMBER', 'ACTIVE', 1, NULL, 1, $4, $5, $6)
`, table),
			cfg.tenantID, cfg.conversationID, cfg.viewerUserID, visibilityVersion, "member-seed-"+sourceEventID, now,
		); err != nil {
			return seededData{}, err
		}
	}

	if _, err := tx.Exec(ctx, `
INSERT INTO search_message_documents (
	tenant_id, conversation_id, message_id, conversation_seq, source_event_id,
	searchable_text, message_type, sender_id, tombstone_status, change_version,
	visibility_version, occurred_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, 'TEXT', $7, 'NONE', 1, $8, $9, $9)
`, cfg.tenantID, cfg.conversationID, messageID, seq, sourceEventID, searchText, cfg.senderUserID, visibilityVersion, now); err != nil {
		return seededData{}, err
	}

	if _, err := tx.Exec(ctx, `
INSERT INTO memory_structured_events (
	tenant_id, memory_event_id, scope_type, scope_id, conversation_id, topic,
	event_type, status, review_state, fact_text, actor_user_ids, audience_user_ids,
	valid_from_seq, valid_to_seq, valid_from_at, valid_to_at, supersedes_event_ids,
	contradicts_event_ids, confidence, visibility_version, extraction_version,
	source_projection_version, created_at, updated_at
) VALUES (
	$1, $2, 'CONVERSATION', $3, $3, 'phoenix-launch',
	'DECISION', 'PENDING', 'UNREVIEWED', $4, $5::jsonb, $6::jsonb,
	$7, $8, $9, NULL, '[]'::jsonb,
	'[]'::jsonb, 0.8700, $10, 'retrieval-smoke-v1',
	$11, $9, $9
)
`, cfg.tenantID, memoryEventID, cfg.conversationID, factText, jsonArray(cfg.senderUserID), jsonArray(cfg.viewerUserID), seq, seq+10, now, visibilityVersion, memoryProjectionVersion); err != nil {
		return seededData{}, err
	}

	if _, err := tx.Exec(ctx, `
INSERT INTO memory_event_source_refs (
	tenant_id, memory_event_id, source_ref_id, source_type, source_id,
	source_event_id, conversation_id, conversation_seq, occurred_at, created_at
) VALUES ($1, $2, $3, 'MESSAGE', $4, $5, $6, $7, $8, $8)
`, cfg.tenantID, memoryEventID, sourceRefID, messageID, sourceEventID, cfg.conversationID, seq, now); err != nil {
		return seededData{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return seededData{}, err
	}
	return seededData{
		TenantID:            cfg.tenantID,
		ConversationID:      cfg.conversationID,
		ViewerUserID:        cfg.viewerUserID,
		SenderUserID:        cfg.senderUserID,
		MessageID:           messageID,
		SourceEventID:       sourceEventID,
		MemoryEventID:       memoryEventID,
		MemorySourceRefID:   sourceRefID,
		ConversationSeq:     seq,
		VisibilityVersion:   visibilityVersion,
		MemoryValidFromSeq:  seq,
		MemoryValidToSeq:    seq + 10,
		MemoryProjectionVer: memoryProjectionVersion,
	}, nil
}

func retrieveEvidence(ctx context.Context, cfg config) (*retrievalv1.RetrieveEvidenceResponse, error) {
	dialOption, err := grpctls.DialOption(cfg.tls, "retrieval-tls")
	if err != nil {
		return nil, err
	}
	conn, err := grpc.NewClient("passthrough:///"+cfg.retrievalTarget, dialOption)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	return retrievalv1.NewRetrievalGatewayClient(conn).RetrieveEvidence(requestCtx, &retrievalv1.RetrieveEvidenceRequest{
		AuthContext: &retrievalv1.AuthContext{
			TenantId:  cfg.tenantID,
			UserId:    cfg.viewerUserID,
			DeviceId:  cfg.deviceID,
			SessionId: "retrieval-smoke-session",
			TraceId:   "retrieval-smoke-trace",
			RequestId: "retrieval-smoke-request",
		},
		Query:          cfg.query,
		ConversationId: cfg.conversationID,
		Limit:          10,
	})
}

func verifyEvidence(
	cfg config,
	resultDir string,
	seed seededData,
	response *retrievalv1.RetrieveEvidenceResponse,
	startedAt time.Time,
) (evidenceSummary, error) {
	pack := response.GetPack()
	if pack == nil {
		return evidenceSummary{}, errors.New("missing evidence pack")
	}
	if pack.GetTenantId() != cfg.tenantID {
		return evidenceSummary{}, fmt.Errorf("unexpected tenant_id %q", pack.GetTenantId())
	}
	if pack.GetConversationId() != cfg.conversationID {
		return evidenceSummary{}, fmt.Errorf("unexpected conversation_id %q", pack.GetConversationId())
	}
	if pack.GetQuery() != strings.ToLower(cfg.query) {
		return evidenceSummary{}, fmt.Errorf("unexpected normalized query %q", pack.GetQuery())
	}

	var searchItem *retrievalv1.EvidenceItem
	var memoryItem *retrievalv1.EvidenceItem
	for _, item := range pack.GetItems() {
		switch item.GetSourceType() {
		case retrievalv1.EvidenceSourceType_EVIDENCE_SOURCE_TYPE_SEARCH_MESSAGE:
			candidate := item
			searchItem = candidate
		case retrievalv1.EvidenceSourceType_EVIDENCE_SOURCE_TYPE_MEMORY_EVENT:
			candidate := item
			memoryItem = candidate
		}
	}
	if searchItem == nil {
		return evidenceSummary{}, errors.New("missing SEARCH_MESSAGE evidence item")
	}
	if memoryItem == nil {
		return evidenceSummary{}, errors.New("missing MEMORY_EVENT evidence item")
	}

	verified := []string{}
	if err := verifySearchItem(searchItem, seed); err != nil {
		return evidenceSummary{}, err
	}
	verified = append(verified, "search item carries message id, source event id, conversation seq and visibility version")
	if err := verifyMemoryItem(memoryItem, seed); err != nil {
		return evidenceSummary{}, err
	}
	verified = append(verified, "memory item carries source refs, temporal status, review state and extraction version")

	counts := sourceCounts{}
	for _, count := range pack.GetSourceCounts() {
		switch count.GetSourceType() {
		case retrievalv1.EvidenceSourceType_EVIDENCE_SOURCE_TYPE_SEARCH_MESSAGE:
			counts.SearchMessage = count.GetCount()
		case retrievalv1.EvidenceSourceType_EVIDENCE_SOURCE_TYPE_MEMORY_EVENT:
			counts.MemoryEvent = count.GetCount()
		}
	}
	if counts.SearchMessage < 1 || counts.MemoryEvent < 1 {
		return evidenceSummary{}, fmt.Errorf("unexpected source counts: %+v", counts)
	}
	if pack.GetSearchProjectionVersion() != seed.VisibilityVersion {
		return evidenceSummary{}, fmt.Errorf("unexpected search projection version %d", pack.GetSearchProjectionVersion())
	}
	if pack.GetMemoryProjectionVersion() != seed.MemoryProjectionVer {
		return evidenceSummary{}, fmt.Errorf("unexpected memory projection version %d", pack.GetMemoryProjectionVersion())
	}
	if strings.TrimSpace(pack.GetRetrievalVersion()) == "" {
		return evidenceSummary{}, errors.New("missing retrieval version")
	}
	verified = append(verified, "source counts and projection versions preserved")

	return evidenceSummary{
		RunName:                 cfg.runName,
		ResultDir:               resultDir,
		RetrievalTarget:         cfg.retrievalTarget,
		Query:                   cfg.query,
		Seed:                    seed,
		PackID:                  pack.GetPackId(),
		ItemCount:               len(pack.GetItems()),
		SearchItemCount:         int(counts.SearchMessage),
		MemoryItemCount:         int(counts.MemoryEvent),
		SourceCounts:            counts,
		SearchProjectionVersion: pack.GetSearchProjectionVersion(),
		MemoryProjectionVersion: pack.GetMemoryProjectionVersion(),
		RetrievalVersion:        pack.GetRetrievalVersion(),
		Verified:                verified,
		StartedAt:               startedAt,
		FinishedAt:              time.Now().UTC(),
	}, nil
}

func verifySearchItem(item *retrievalv1.EvidenceItem, seed seededData) error {
	if item.GetMessageId() != seed.MessageID {
		return fmt.Errorf("unexpected search message_id %q", item.GetMessageId())
	}
	if item.GetConversationSeq() != seed.ConversationSeq {
		return fmt.Errorf("unexpected search conversation_seq %d", item.GetConversationSeq())
	}
	if item.GetVisibilityVersion() != seed.VisibilityVersion {
		return fmt.Errorf("unexpected search visibility_version %d", item.GetVisibilityVersion())
	}
	if len(item.GetSourceRefs()) == 0 {
		return errors.New("search item missing source refs")
	}
	ref := item.GetSourceRefs()[0]
	if ref.GetSourceEventId() != seed.SourceEventID || ref.GetSourceId() != seed.MessageID {
		return fmt.Errorf("unexpected search source ref: %+v", ref)
	}
	if strings.TrimSpace(item.GetText()) == "" {
		return errors.New("search item snippet is empty")
	}
	return nil
}

func verifyMemoryItem(item *retrievalv1.EvidenceItem, seed seededData) error {
	if item.GetMemoryEventId() != seed.MemoryEventID {
		return fmt.Errorf("unexpected memory_event_id %q", item.GetMemoryEventId())
	}
	if item.GetValidFromSeq() != seed.MemoryValidFromSeq || item.GetValidToSeq() != seed.MemoryValidToSeq {
		return fmt.Errorf("unexpected memory validity window %d..%d", item.GetValidFromSeq(), item.GetValidToSeq())
	}
	if item.GetVisibilityVersion() != seed.VisibilityVersion {
		return fmt.Errorf("unexpected memory visibility_version %d", item.GetVisibilityVersion())
	}
	if item.GetTemporalStatus() != "PENDING" {
		return fmt.Errorf("unexpected temporal status %q", item.GetTemporalStatus())
	}
	if item.GetReviewState() != "UNREVIEWED" {
		return fmt.Errorf("unexpected review state %q", item.GetReviewState())
	}
	if item.GetExtractionVersion() != "retrieval-smoke-v1" {
		return fmt.Errorf("unexpected extraction version %q", item.GetExtractionVersion())
	}
	if len(item.GetSourceRefs()) == 0 {
		return errors.New("memory item missing source refs")
	}
	ref := item.GetSourceRefs()[0]
	if ref.GetSourceEventId() != seed.SourceEventID || ref.GetSourceId() != seed.MessageID {
		return fmt.Errorf("unexpected memory source ref: %+v", ref)
	}
	if len(item.GetActorUserIds()) == 0 || item.GetActorUserIds()[0] != seed.SenderUserID {
		return fmt.Errorf("unexpected actor user ids: %v", item.GetActorUserIds())
	}
	if len(item.GetAudienceUserIds()) == 0 || item.GetAudienceUserIds()[0] != seed.ViewerUserID {
		return fmt.Errorf("unexpected audience user ids: %v", item.GetAudienceUserIds())
	}
	return nil
}

func writeSummary(resultDir string, result evidenceSummary) error {
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(resultDir, "retrieval-evidence-summary.json")
	if err := os.WriteFile(path, append(encoded, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Println(string(encoded))
	fmt.Printf("summary: %s\n", path)
	return nil
}

func jsonArray(value string) string {
	encoded, _ := json.Marshal([]string{value})
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
		return "retrieval-smoke"
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
