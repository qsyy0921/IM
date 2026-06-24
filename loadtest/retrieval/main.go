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
	TenantID                 string `json:"tenant_id"`
	ConversationID           string `json:"conversation_id"`
	CrossGroupConversationID string `json:"cross_group_conversation_id"`
	ViewerUserID             string `json:"viewer_user_id"`
	SenderUserID             string `json:"sender_user_id"`
	CrossGroupActorUserID    string `json:"cross_group_actor_user_id"`
	MessageID                string `json:"message_id"`
	CrossGroupMessageID      string `json:"cross_group_message_id"`
	SourceEventID            string `json:"source_event_id"`
	CrossGroupSourceEventID  string `json:"cross_group_source_event_id"`
	MemoryEventID            string `json:"memory_event_id"`
	ProfileID                string `json:"profile_id"`
	ExpiredMemoryEventID     string `json:"expired_memory_event_id"`
	SupersededMemoryEventID  string `json:"superseded_memory_event_id"`
	FutureMemoryEventID      string `json:"future_memory_event_id"`
	MemorySourceRefID        string `json:"memory_source_ref_id"`
	CrossGroupSourceRefID    string `json:"cross_group_source_ref_id"`
	MemoryGraphEdgeID        string `json:"memory_graph_edge_id"`
	ConversationSeq          int64  `json:"conversation_seq"`
	VisibilityVersion        int64  `json:"visibility_version"`
	MemoryValidFromSeq       int64  `json:"memory_valid_from_seq"`
	MemoryValidToSeq         int64  `json:"memory_valid_to_seq"`
	MemoryProjectionVer      int64  `json:"memory_projection_version"`
}

type evidenceSummary struct {
	RunName                               string       `json:"run_name"`
	ResultDir                             string       `json:"result_dir"`
	RetrievalTarget                       string       `json:"retrieval_target"`
	Query                                 string       `json:"query"`
	Seed                                  seededData   `json:"seed"`
	PackID                                string       `json:"pack_id"`
	ItemCount                             int          `json:"item_count"`
	SearchItemCount                       int          `json:"search_item_count"`
	MemoryItemCount                       int          `json:"memory_item_count"`
	ProfileItemCount                      int          `json:"profile_item_count"`
	SourceCounts                          sourceCounts `json:"source_counts"`
	SearchProjectionVersion               int64        `json:"search_projection_version"`
	MemoryProjectionVersion               int64        `json:"memory_projection_version"`
	RetrievalVersion                      string       `json:"retrieval_version"`
	CurrentMemoryAtSeq                    int64        `json:"current_memory_at_seq"`
	CrossGroupSourceRefsPreserved         bool         `json:"cross_group_source_refs_preserved"`
	CrossGroupSpeakerAttributionPreserved bool         `json:"cross_group_speaker_attribution_preserved"`
	MemoryGraphEdgesPreserved             bool         `json:"memory_graph_edges_preserved"`
	ProfileAggregatePreserved             bool         `json:"profile_aggregate_preserved"`
	TemporalVersionSelectedByQuerySeq     bool         `json:"temporal_version_selected_by_query_seq"`
	ExpiredMemoryExcluded                 bool         `json:"expired_memory_excluded"`
	SupersededMemoryExcluded              bool         `json:"superseded_memory_excluded"`
	FutureMemoryExcluded                  bool         `json:"future_memory_excluded"`
	Verified                              []string     `json:"verified"`
	StartedAt                             time.Time    `json:"started_at"`
	FinishedAt                            time.Time    `json:"finished_at"`
}

type sourceCounts struct {
	SearchMessage    int32 `json:"search_message"`
	MemoryEvent      int32 `json:"memory_event"`
	ProfileAggregate int32 `json:"profile_aggregate"`
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

	response, err := retrieveEvidence(ctx, cfg, seed)
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
	crossGroupConversationID := cfg.conversationID + "-strategy"
	crossGroupActorUserID := cfg.senderUserID + "-strategy"
	messageID := "msg-retrieval-" + randomSuffix()
	crossGroupMessageID := "msg-retrieval-cross-" + randomSuffix()
	sourceEventID := "evt-retrieval-" + randomSuffix()
	crossGroupSourceEventID := "evt-retrieval-cross-" + randomSuffix()
	memoryEventID := "mem-retrieval-" + randomSuffix()
	profileID := "profile-retrieval-" + randomSuffix()
	memoryGraphEdgeID := "edge-retrieval-supports-" + randomSuffix()
	expiredMemoryEventID := "mem-retrieval-expired-" + randomSuffix()
	supersededMemoryEventID := "mem-retrieval-superseded-" + randomSuffix()
	futureMemoryEventID := "mem-retrieval-future-" + randomSuffix()
	sourceRefID := "ref-retrieval-" + randomSuffix()
	crossGroupSourceRefID := "ref-retrieval-cross-" + randomSuffix()
	seq := int64(2)
	visibilityVersion := int64(17)
	memoryProjectionVersion := int64(23)
	searchText := "The phoenix launch decision requires the retrieval gateway evidence pack smoke to preserve citations."
	crossGroupText := "The phoenix launch decision was confirmed by the strategy group and must keep cross group attribution."
	factText := "Phoenix launch decision requires EvidencePack source refs, cross group attribution, and temporal version preservation."
	actorJSON := jsonStringArray(cfg.senderUserID, crossGroupActorUserID)
	audienceJSON := jsonStringArray(cfg.viewerUserID)

	tx, err := pool.Begin(ctx)
	if err != nil {
		return seededData{}, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	for _, table := range []string{"search_membership_projection", "memory_membership_projection"} {
		for _, conversationID := range []string{cfg.conversationID, crossGroupConversationID} {
			if _, err := tx.Exec(ctx, fmt.Sprintf(`
INSERT INTO %s (
	tenant_id, conversation_id, user_id, role, status, join_seq, leave_seq,
	member_version, permission_version, updated_by_event_id, updated_at
) VALUES ($1, $2, $3, 'MEMBER', 'ACTIVE', 1, NULL, 1, $4, $5, $6)
`, table),
				cfg.tenantID, conversationID, cfg.viewerUserID, visibilityVersion, "member-seed-"+sourceEventID, now,
			); err != nil {
				return seededData{}, err
			}
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
INSERT INTO search_message_documents (
	tenant_id, conversation_id, message_id, conversation_seq, source_event_id,
	searchable_text, message_type, sender_id, tombstone_status, change_version,
	visibility_version, occurred_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, 'TEXT', $7, 'NONE', 1, $8, $9, $9)
`, cfg.tenantID, crossGroupConversationID, crossGroupMessageID, seq+1, crossGroupSourceEventID, crossGroupText, crossGroupActorUserID, visibilityVersion, now); err != nil {
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
	'DECISION', 'ACTIVE', 'APPROVED', $4, $5::jsonb, $6::jsonb,
	$7, $8, $9, NULL, '[]'::jsonb,
	'[]'::jsonb, 0.8700, $10, 'retrieval-smoke-v1',
	$11, $9, $9
)
`, cfg.tenantID, memoryEventID, cfg.conversationID, factText, actorJSON, audienceJSON, seq, seq+10, now, visibilityVersion, memoryProjectionVersion); err != nil {
		return seededData{}, err
	}

	if _, err := tx.Exec(ctx, `
INSERT INTO memory_structured_events (
	tenant_id, memory_event_id, scope_type, scope_id, conversation_id, topic,
	event_type, status, review_state, fact_text, actor_user_ids, audience_user_ids,
	valid_from_seq, valid_to_seq, valid_from_at, valid_to_at, supersedes_event_ids,
	contradicts_event_ids, confidence, visibility_version, extraction_version,
	source_projection_version, created_at, updated_at
) VALUES
	($1, $2, 'CONVERSATION', $4, $4, 'phoenix-launch',
	 'DECISION', 'ACTIVE', 'APPROVED', $5, $6::jsonb, $7::jsonb,
	 1, $8, $9, NULL, '[]'::jsonb,
	 '[]'::jsonb, 0.9900, $10, 'retrieval-smoke-v1',
	 $11, $9, $9),
	($1, $3, 'CONVERSATION', $4, $4, 'phoenix-launch',
	 'DECISION', 'SUPERSEDED', 'APPROVED', $5, $6::jsonb, $7::jsonb,
	 $8, $12, $9, NULL, '[]'::jsonb,
	 '[]'::jsonb, 0.9800, $10, 'retrieval-smoke-v1',
	 $11, $9, $9),
	($1, $13, 'CONVERSATION', $4, $4, 'phoenix-launch',
	 'DECISION', 'ACTIVE', 'APPROVED', $5, $6::jsonb, $7::jsonb,
	 $14, NULL, $9, NULL, '[]'::jsonb,
	 '[]'::jsonb, 0.9700, $10, 'retrieval-smoke-v1',
	 $11, $9, $9)
`, cfg.tenantID, expiredMemoryEventID, supersededMemoryEventID, cfg.conversationID, factText, actorJSON, audienceJSON, seq-1, now, visibilityVersion, memoryProjectionVersion, seq+10, futureMemoryEventID, seq+20); err != nil {
		return seededData{}, err
	}

	if _, err := tx.Exec(ctx, `
INSERT INTO memory_event_source_refs (
	tenant_id, memory_event_id, source_ref_id, source_type, source_id,
	source_event_id, conversation_id, conversation_seq, occurred_at, created_at
) VALUES
	($1, $2, $3, 'MESSAGE', $4, $5, $6, $7, $10, $10),
	($1, $2, $8, 'MESSAGE', $11, $12, $9, $13, $10, $10)
`, cfg.tenantID, memoryEventID, sourceRefID, messageID, sourceEventID, cfg.conversationID, seq, crossGroupSourceRefID, crossGroupConversationID, now, crossGroupMessageID, crossGroupSourceEventID, seq+1); err != nil {
		return seededData{}, err
	}

	if _, err := tx.Exec(ctx, `
INSERT INTO memory_graph_edges (
	tenant_id, edge_id, from_memory_event_id, to_memory_event_id,
	relation_type, confidence, source_refs, created_at
) VALUES (
	$1, $2, $3, $4, 'SUPPORTS', 0.9100,
	jsonb_build_array(
		jsonb_build_object(
			'source_type', 'MESSAGE',
			'source_id', $5::text,
			'source_event_id', $6::text,
			'conversation_id', $7::text,
			'conversation_seq', $8
		),
		jsonb_build_object(
			'source_type', 'MESSAGE',
			'source_id', $9::text,
			'source_event_id', $10::text,
			'conversation_id', $11::text,
			'conversation_seq', $12
		)
	),
	$13
)
`, cfg.tenantID, memoryGraphEdgeID, memoryEventID, supersededMemoryEventID, messageID, sourceEventID, cfg.conversationID, seq, crossGroupMessageID, crossGroupSourceEventID, crossGroupConversationID, seq+1, now); err != nil {
		return seededData{}, err
	}

	if _, err := tx.Exec(ctx, `
INSERT INTO memory_profile_aggregates (
	tenant_id, profile_id, subject_user_id, aggregate_type, aggregate_key,
	status, review_state, summary_text, supporting_memory_event_ids,
	confidence, valid_from_at, updated_by_memory_event_id, created_at, updated_at
) VALUES (
	$1, $2, $3, 'SKILL', 'phoenix-launch',
	'ACTIVE', 'APPROVED', 'viewer coordinates phoenix launch decisions across groups',
	jsonb_build_array($4::text), 0.9200, $5, $4, $5, $5
)
`, cfg.tenantID, profileID, cfg.viewerUserID, memoryEventID, now); err != nil {
		return seededData{}, err
	}

	if _, err := tx.Exec(ctx, `
INSERT INTO memory_event_source_refs (
	tenant_id, memory_event_id, source_ref_id, source_type, source_id,
	source_event_id, conversation_id, conversation_seq, occurred_at, created_at
) VALUES
	($1, $2, 'ref-expired', 'MESSAGE', $4, $5, $6, 1, $7, $7),
	($1, $3, 'ref-superseded', 'MESSAGE', $4, $5, $6, $8, $7, $7),
	($1, $9, 'ref-future', 'MESSAGE', $4, $5, $6, $10, $7, $7)
`, cfg.tenantID, expiredMemoryEventID, supersededMemoryEventID, messageID, sourceEventID, cfg.conversationID, now, seq, futureMemoryEventID, seq+20); err != nil {
		return seededData{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return seededData{}, err
	}
	return seededData{
		TenantID:                 cfg.tenantID,
		ConversationID:           cfg.conversationID,
		CrossGroupConversationID: crossGroupConversationID,
		ViewerUserID:             cfg.viewerUserID,
		SenderUserID:             cfg.senderUserID,
		CrossGroupActorUserID:    crossGroupActorUserID,
		MessageID:                messageID,
		CrossGroupMessageID:      crossGroupMessageID,
		SourceEventID:            sourceEventID,
		CrossGroupSourceEventID:  crossGroupSourceEventID,
		MemoryEventID:            memoryEventID,
		ProfileID:                profileID,
		MemoryGraphEdgeID:        memoryGraphEdgeID,
		ExpiredMemoryEventID:     expiredMemoryEventID,
		SupersededMemoryEventID:  supersededMemoryEventID,
		FutureMemoryEventID:      futureMemoryEventID,
		MemorySourceRefID:        sourceRefID,
		CrossGroupSourceRefID:    crossGroupSourceRefID,
		ConversationSeq:          seq,
		VisibilityVersion:        visibilityVersion,
		MemoryValidFromSeq:       seq,
		MemoryValidToSeq:         seq + 10,
		MemoryProjectionVer:      memoryProjectionVersion,
	}, nil
}

func retrieveEvidence(ctx context.Context, cfg config, seed seededData) (*retrievalv1.RetrieveEvidenceResponse, error) {
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
		Query:             cfg.query,
		ConversationId:    cfg.conversationID,
		AtConversationSeq: seed.ConversationSeq + 5,
		Limit:             10,
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
	var profileItem *retrievalv1.EvidenceItem
	staleMemoryExcluded := map[string]bool{
		seed.ExpiredMemoryEventID:    true,
		seed.SupersededMemoryEventID: true,
		seed.FutureMemoryEventID:     true,
	}
	for _, item := range pack.GetItems() {
		switch item.GetSourceType() {
		case retrievalv1.EvidenceSourceType_EVIDENCE_SOURCE_TYPE_SEARCH_MESSAGE:
			candidate := item
			searchItem = candidate
		case retrievalv1.EvidenceSourceType_EVIDENCE_SOURCE_TYPE_MEMORY_EVENT:
			if staleMemoryExcluded[item.GetMemoryEventId()] {
				return evidenceSummary{}, fmt.Errorf("stale memory evidence leaked into EvidencePack: %+v", item)
			}
			candidate := item
			memoryItem = candidate
		case retrievalv1.EvidenceSourceType_EVIDENCE_SOURCE_TYPE_PROFILE_AGGREGATE:
			candidate := item
			profileItem = candidate
		}
	}
	if searchItem == nil {
		return evidenceSummary{}, errors.New("missing SEARCH_MESSAGE evidence item")
	}
	if memoryItem == nil {
		return evidenceSummary{}, errors.New("missing MEMORY_EVENT evidence item")
	}
	if profileItem == nil {
		return evidenceSummary{}, errors.New("missing PROFILE_AGGREGATE evidence item")
	}

	verified := []string{}
	if err := verifySearchItem(searchItem, seed); err != nil {
		return evidenceSummary{}, err
	}
	verified = append(verified, "search item carries message id, source event id, conversation seq and visibility version")
	if err := verifyMemoryItem(memoryItem, seed); err != nil {
		return evidenceSummary{}, err
	}
	verified = append(verified, "memory item is active current-only evidence with source refs, review state and extraction version")
	if err := verifyProfileItem(profileItem, seed); err != nil {
		return evidenceSummary{}, err
	}
	verified = append(verified, "profile aggregate evidence preserves subject, aggregate key and supporting memory ids")
	verified = append(verified, "cross-group memory source refs and speaker attribution are preserved")
	verified = append(verified, "memory graph edge is preserved in EvidencePack")
	verified = append(verified, "expired, superseded and future memory decoys are excluded by query seq")

	counts := sourceCounts{}
	for _, count := range pack.GetSourceCounts() {
		switch count.GetSourceType() {
		case retrievalv1.EvidenceSourceType_EVIDENCE_SOURCE_TYPE_SEARCH_MESSAGE:
			counts.SearchMessage = count.GetCount()
		case retrievalv1.EvidenceSourceType_EVIDENCE_SOURCE_TYPE_MEMORY_EVENT:
			counts.MemoryEvent = count.GetCount()
		case retrievalv1.EvidenceSourceType_EVIDENCE_SOURCE_TYPE_PROFILE_AGGREGATE:
			counts.ProfileAggregate = count.GetCount()
		}
	}
	if counts.SearchMessage != 1 || counts.MemoryEvent != 1 || counts.ProfileAggregate != 1 {
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
	verified = append(verified, "source counts, projection versions and stale-memory filtering preserved")

	return evidenceSummary{
		RunName:                               cfg.runName,
		ResultDir:                             resultDir,
		RetrievalTarget:                       cfg.retrievalTarget,
		Query:                                 cfg.query,
		Seed:                                  seed,
		PackID:                                pack.GetPackId(),
		ItemCount:                             len(pack.GetItems()),
		SearchItemCount:                       int(counts.SearchMessage),
		MemoryItemCount:                       int(counts.MemoryEvent),
		ProfileItemCount:                      int(counts.ProfileAggregate),
		SourceCounts:                          counts,
		SearchProjectionVersion:               pack.GetSearchProjectionVersion(),
		MemoryProjectionVersion:               pack.GetMemoryProjectionVersion(),
		RetrievalVersion:                      pack.GetRetrievalVersion(),
		CurrentMemoryAtSeq:                    seed.ConversationSeq + 5,
		CrossGroupSourceRefsPreserved:         true,
		CrossGroupSpeakerAttributionPreserved: true,
		MemoryGraphEdgesPreserved:             true,
		ProfileAggregatePreserved:             true,
		TemporalVersionSelectedByQuerySeq:     true,
		ExpiredMemoryExcluded:                 true,
		SupersededMemoryExcluded:              true,
		FutureMemoryExcluded:                  true,
		Verified:                              verified,
		StartedAt:                             startedAt,
		FinishedAt:                            time.Now().UTC(),
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
	if item.GetTemporalStatus() != "ACTIVE" {
		return fmt.Errorf("unexpected temporal status %q", item.GetTemporalStatus())
	}
	if item.GetReviewState() != "APPROVED" {
		return fmt.Errorf("unexpected review state %q", item.GetReviewState())
	}
	if item.GetExtractionVersion() != "retrieval-smoke-v1" {
		return fmt.Errorf("unexpected extraction version %q", item.GetExtractionVersion())
	}
	if len(item.GetSourceRefs()) == 0 {
		return errors.New("memory item missing source refs")
	}
	primaryRef, ok := findSourceRef(item.GetSourceRefs(), seed.MessageID, seed.SourceEventID, seed.ConversationID)
	if !ok {
		return fmt.Errorf("memory item missing primary source ref for message %q", seed.MessageID)
	}
	if primaryRef.GetSourceEventId() != seed.SourceEventID ||
		primaryRef.GetSourceId() != seed.MessageID ||
		primaryRef.GetConversationId() != seed.ConversationID ||
		primaryRef.GetConversationSeq() != seed.ConversationSeq {
		return fmt.Errorf("unexpected primary memory source ref: %+v", primaryRef)
	}
	crossRef, ok := findSourceRef(item.GetSourceRefs(), seed.CrossGroupMessageID, seed.CrossGroupSourceEventID, seed.CrossGroupConversationID)
	if !ok {
		return fmt.Errorf("memory item missing cross-group source ref for message %q", seed.CrossGroupMessageID)
	}
	if crossRef.GetSourceEventId() != seed.CrossGroupSourceEventID ||
		crossRef.GetSourceId() != seed.CrossGroupMessageID ||
		crossRef.GetConversationId() != seed.CrossGroupConversationID ||
		crossRef.GetConversationSeq() != seed.ConversationSeq+1 {
		return fmt.Errorf("unexpected cross-group memory source ref: %+v", crossRef)
	}
	if !stringSliceContains(item.GetActorUserIds(), seed.SenderUserID) ||
		!stringSliceContains(item.GetActorUserIds(), seed.CrossGroupActorUserID) {
		return fmt.Errorf("unexpected actor user ids: %v", item.GetActorUserIds())
	}
	if !stringSliceContains(item.GetAudienceUserIds(), seed.ViewerUserID) {
		return fmt.Errorf("unexpected audience user ids: %v", item.GetAudienceUserIds())
	}
	if len(item.GetMemoryGraphEdges()) != 1 {
		return fmt.Errorf("memory item should preserve one graph edge, got %+v", item.GetMemoryGraphEdges())
	}
	edge := item.GetMemoryGraphEdges()[0]
	if edge.GetEdgeId() != seed.MemoryGraphEdgeID ||
		edge.GetFromMemoryEventId() != seed.MemoryEventID ||
		edge.GetToMemoryEventId() != seed.SupersededMemoryEventID ||
		edge.GetRelationType() != "SUPPORTS" ||
		len(edge.GetSourceRefs()) != 2 {
		return fmt.Errorf("unexpected memory graph edge: %+v", edge)
	}
	return nil
}

func verifyProfileItem(item *retrievalv1.EvidenceItem, seed seededData) error {
	if item.GetProfileId() != seed.ProfileID {
		return fmt.Errorf("unexpected profile_id %q", item.GetProfileId())
	}
	if item.GetProfileSubjectUserId() != seed.ViewerUserID {
		return fmt.Errorf("unexpected profile subject_user_id %q", item.GetProfileSubjectUserId())
	}
	if item.GetProfileAggregateType() != "SKILL" || item.GetProfileAggregateKey() != "phoenix-launch" {
		return fmt.Errorf("unexpected profile aggregate fields: %+v", item)
	}
	if strings.TrimSpace(item.GetText()) == "" {
		return errors.New("profile aggregate evidence text is empty")
	}
	if ids := item.GetSupportingMemoryEventIds(); len(ids) != 1 || ids[0] != seed.MemoryEventID {
		return fmt.Errorf("unexpected profile supporting memory ids: %+v", ids)
	}
	if item.GetProfileUpdatedAtUnixMs() == 0 {
		return errors.New("profile aggregate evidence missing updated_at")
	}
	return nil
}

func findSourceRef(
	refs []*retrievalv1.EvidenceSourceRef,
	sourceID string,
	sourceEventID string,
	conversationID string,
) (*retrievalv1.EvidenceSourceRef, bool) {
	for _, ref := range refs {
		if ref.GetSourceId() == sourceID &&
			ref.GetSourceEventId() == sourceEventID &&
			ref.GetConversationId() == conversationID {
			return ref, true
		}
	}
	return nil, false
}

func stringSliceContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
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
	return jsonStringArray(value)
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
