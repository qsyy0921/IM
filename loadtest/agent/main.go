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
	agentv1 "github.com/qsyy0921/IM/api/proto/nexusim/agent/v1"
	policyv1 "github.com/qsyy0921/IM/api/proto/nexusim/policy/v1"
	retrievalv1 "github.com/qsyy0921/IM/api/proto/nexusim/retrieval/v1"
	"github.com/qsyy0921/IM/loadtest/internal/grpctls"
	"google.golang.org/grpc"
)

const (
	defaultPGDSN       = "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable"
	defaultAgentTarget = "127.0.0.1:10630"
	defaultResultRoot  = `H:\NexusIM\loadtest-results`
	defaultObjective   = "phoenix launch decision"
	defaultToolName    = "conversation.note.create"
	defaultResource    = "conversation_note"
)

type config struct {
	pgDSN          string
	agentTarget    string
	resultRoot     string
	runName        string
	tenantID       string
	conversationID string
	viewerUserID   string
	senderUserID   string
	deviceID       string
	objective      string
	toolName       string
	resourceType   string
	resourceID     string
	riskLevel      string
	intent         string
	requestTimeout time.Duration
	tls            grpctls.Config
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

type agentSmokeSummary struct {
	RunName                 string       `json:"run_name"`
	ResultDir               string       `json:"result_dir"`
	AgentTarget             string       `json:"agent_target"`
	Objective               string       `json:"objective"`
	ToolName                string       `json:"tool_name"`
	ResourceType            string       `json:"resource_type"`
	RiskLevel               string       `json:"risk_level"`
	Seed                    seededData   `json:"seed"`
	ProposalID              string       `json:"proposal_id"`
	ProposalStatus          string       `json:"proposal_status"`
	ProposalText            string       `json:"proposal_text"`
	RequiresApproval        bool         `json:"requires_approval"`
	GeneratedByLLM          bool         `json:"generated_by_llm"`
	CitationCount           int          `json:"citation_count"`
	PolicyAllowed           bool         `json:"policy_allowed"`
	PolicyRequiresApproval  bool         `json:"policy_requires_approval"`
	PolicyDecisionSource    string       `json:"policy_decision_source"`
	PolicyClassification    string       `json:"policy_classification"`
	PolicyPermissionVersion int64        `json:"policy_permission_version"`
	PackID                  string       `json:"pack_id"`
	EvidenceItemCount       int          `json:"evidence_item_count"`
	SearchItemCount         int          `json:"search_item_count"`
	MemoryItemCount         int          `json:"memory_item_count"`
	SourceCounts            sourceCounts `json:"source_counts"`
	SearchProjectionVersion int64        `json:"search_projection_version"`
	MemoryProjectionVersion int64        `json:"memory_projection_version"`
	AgentVersion            string       `json:"agent_version"`
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
		fmt.Fprintf(os.Stderr, "agent smoke failed: %v\n", err)
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

	response, err := createProposal(ctx, cfg)
	if err != nil {
		return err
	}
	summary, err := verifyProposal(cfg, resultDir, seed, response, startedAt)
	if err != nil {
		return err
	}
	return writeSummary(resultDir, summary)
}

func parseConfig(args []string) (config, error) {
	cfg := config{}
	flagSet := flag.NewFlagSet("agent-smoke", flag.ContinueOnError)
	flagSet.StringVar(&cfg.pgDSN, "pg-dsn", defaultPGDSN, "PostgreSQL DSN")
	flagSet.StringVar(&cfg.agentTarget, "agent-target", defaultAgentTarget, "agent-service gRPC address")
	flagSet.StringVar(&cfg.resultRoot, "result-root", defaultResultRoot, "external result root for raw smoke output")
	flagSet.StringVar(&cfg.runName, "run-name", "", "run name under result root")
	flagSet.StringVar(&cfg.tenantID, "tenant-id", "", "tenant id for seeded projection rows")
	flagSet.StringVar(&cfg.conversationID, "conversation-id", "", "conversation id for seeded projection rows")
	flagSet.StringVar(&cfg.viewerUserID, "viewer-user-id", "agent-viewer", "viewer user id")
	flagSet.StringVar(&cfg.senderUserID, "sender-user-id", "agent-sender", "sender user id")
	flagSet.StringVar(&cfg.deviceID, "device-id", "agent-device", "viewer device id")
	flagSet.StringVar(&cfg.objective, "objective", defaultObjective, "objective sent to agent-service")
	flagSet.StringVar(&cfg.toolName, "tool-name", defaultToolName, "tool name for policy precheck")
	flagSet.StringVar(&cfg.resourceType, "resource-type", defaultResource, "resource type for policy precheck")
	flagSet.StringVar(&cfg.resourceID, "resource-id", "", "resource id for policy precheck")
	flagSet.StringVar(&cfg.riskLevel, "risk-level", "LOW", "risk level for policy precheck")
	flagSet.StringVar(&cfg.intent, "intent", "", "intent for policy precheck")
	flagSet.DurationVar(&cfg.requestTimeout, "request-timeout", 10*time.Second, "gRPC request timeout")
	flagSet.StringVar(&cfg.tls.CAFile, "agent-tls-ca-file", "", "agent gRPC TLS CA file")
	flagSet.StringVar(&cfg.tls.ServerName, "agent-tls-server-name", "", "agent gRPC TLS server name")
	flagSet.StringVar(&cfg.tls.ClientCertFile, "agent-tls-client-cert-file", "", "agent gRPC client certificate")
	flagSet.StringVar(&cfg.tls.ClientKeyFile, "agent-tls-client-key-file", "", "agent gRPC client key")
	if err := flagSet.Parse(args); err != nil {
		return config{}, err
	}
	cfg.resultRoot = strings.TrimSpace(cfg.resultRoot)
	cfg.agentTarget = strings.TrimSpace(cfg.agentTarget)
	cfg.objective = strings.TrimSpace(cfg.objective)
	cfg.toolName = strings.TrimSpace(cfg.toolName)
	cfg.resourceType = strings.TrimSpace(cfg.resourceType)
	cfg.riskLevel = strings.ToUpper(strings.TrimSpace(cfg.riskLevel))
	cfg.intent = strings.TrimSpace(cfg.intent)
	if cfg.agentTarget == "" {
		return config{}, errors.New("--agent-target is required")
	}
	if cfg.resultRoot == "" {
		return config{}, errors.New("--result-root is required")
	}
	if cfg.objective == "" {
		return config{}, errors.New("--objective is required")
	}
	if cfg.toolName == "" {
		return config{}, errors.New("--tool-name is required")
	}
	if cfg.resourceType == "" {
		return config{}, errors.New("--resource-type is required")
	}
	if cfg.riskLevel == "" {
		cfg.riskLevel = "LOW"
	}
	if cfg.runName == "" {
		cfg.runName = "agent-service-proposal-smoke-" + time.Now().UTC().Format("20060102-150405")
	}
	suffix := randomSuffix()
	if strings.TrimSpace(cfg.tenantID) == "" {
		cfg.tenantID = "tenant-agent-smoke-" + suffix
	}
	if strings.TrimSpace(cfg.conversationID) == "" {
		cfg.conversationID = "conv-agent-smoke-" + suffix
	}
	if strings.TrimSpace(cfg.resourceID) == "" {
		cfg.resourceID = cfg.conversationID
	}
	if cfg.intent == "" {
		cfg.intent = cfg.objective
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
	messageID := "msg-agent-" + randomSuffix()
	sourceEventID := "evt-agent-" + randomSuffix()
	memoryEventID := "mem-agent-" + randomSuffix()
	sourceRefID := "ref-agent-" + randomSuffix()
	seq := int64(2)
	visibilityVersion := int64(41)
	memoryProjectionVersion := int64(43)
	searchText := "The phoenix launch decision is approved with a follow-up note for rollout owners."
	factText := "Phoenix launch decision follow-up should preserve EvidencePack source references before any action execution."

	tx, err := pool.Begin(ctx)
	if err != nil {
		return seededData{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for _, table := range []string{"search_membership_projection", "memory_membership_projection"} {
		if _, err := tx.Exec(ctx, fmt.Sprintf(`
INSERT INTO %s (
	tenant_id, conversation_id, user_id, role, status, join_seq, leave_seq,
	member_version, permission_version, updated_by_event_id, updated_at
) VALUES ($1, $2, $3, 'MEMBER', 'ACTIVE', 1, NULL, 1, $4, $5, $6)
`, table), cfg.tenantID, cfg.conversationID, cfg.viewerUserID, visibilityVersion, "member-seed-"+sourceEventID, now); err != nil {
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
	'DECISION', 'ACTIVE', 'APPROVED', $4, $5::jsonb, $6::jsonb,
	$7, $8, $9, NULL, '[]'::jsonb,
	'[]'::jsonb, 0.9300, $10, 'agent-smoke-v1',
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

func createProposal(ctx context.Context, cfg config) (*agentv1.CreateAgentProposalResponse, error) {
	dialOption, err := grpctls.DialOption(cfg.tls, "agent-tls")
	if err != nil {
		return nil, err
	}
	conn, err := grpc.NewClient("passthrough:///"+cfg.agentTarget, dialOption)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	return agentv1.NewAgentServiceClient(conn).CreateAgentProposal(requestCtx, &agentv1.CreateAgentProposalRequest{
		AuthContext: &retrievalv1.AuthContext{
			TenantId:  cfg.tenantID,
			UserId:    cfg.viewerUserID,
			DeviceId:  cfg.deviceID,
			SessionId: "agent-smoke-session",
			TraceId:   "agent-smoke-trace",
			RequestId: "agent-smoke-request",
		},
		ConversationId: cfg.conversationID,
		Objective:      cfg.objective,
		ToolName:       cfg.toolName,
		ToolAction:     policyv1.ToolAction_TOOL_ACTION_CALL,
		ResourceType:   cfg.resourceType,
		ResourceId:     cfg.resourceID,
		RiskLevel:      cfg.riskLevel,
		Intent:         cfg.intent,
		Limit:          10,
		IncludeSearch:  true,
		IncludeMemory:  true,
		MemoryStatuses: []retrievalv1.EvidenceMemoryStatus{
			retrievalv1.EvidenceMemoryStatus_EVIDENCE_MEMORY_STATUS_ACTIVE,
		},
	})
}

func verifyProposal(
	cfg config,
	resultDir string,
	seed seededData,
	response *agentv1.CreateAgentProposalResponse,
	startedAt time.Time,
) (agentSmokeSummary, error) {
	if response.GetStatus() != agentv1.AgentProposalStatus_AGENT_PROPOSAL_STATUS_PROPOSED {
		return agentSmokeSummary{}, fmt.Errorf("unexpected proposal status %v", response.GetStatus())
	}
	if response.GetGeneratedByLlm() {
		return agentSmokeSummary{}, errors.New("first-stage agent smoke must not claim LLM generation")
	}
	if !response.GetRequiresApproval() {
		return agentSmokeSummary{}, errors.New("agent proposal must require approval before execution")
	}
	if strings.TrimSpace(response.GetProposalText()) == "" {
		return agentSmokeSummary{}, errors.New("proposal text is empty")
	}
	proposalText := strings.ToLower(response.GetProposalText())
	if !strings.Contains(proposalText, "no business mutation") || !strings.Contains(proposalText, "phoenix launch") {
		return agentSmokeSummary{}, fmt.Errorf("proposal text missing expected evidence/boundary text: %q", response.GetProposalText())
	}
	if len(response.GetCitations()) == 0 {
		return agentSmokeSummary{}, errors.New("missing citations")
	}
	if strings.TrimSpace(response.GetAgentVersion()) == "" {
		return agentSmokeSummary{}, errors.New("missing agent_version")
	}

	decision := response.GetToolPolicyDecision()
	if decision == nil || !decision.GetAllowed() {
		return agentSmokeSummary{}, errors.New("tool policy decision is not allowed")
	}
	if decision.GetAction() != policyv1.ToolAction_TOOL_ACTION_CALL {
		return agentSmokeSummary{}, fmt.Errorf("unexpected tool action %v", decision.GetAction())
	}
	if decision.GetToolName() != cfg.toolName || decision.GetResourceType() != cfg.resourceType {
		return agentSmokeSummary{}, fmt.Errorf("unexpected tool policy scope: %+v", decision)
	}

	pack := response.GetEvidencePack()
	if pack == nil {
		return agentSmokeSummary{}, errors.New("missing evidence pack")
	}
	if pack.GetTenantId() != cfg.tenantID || pack.GetConversationId() != cfg.conversationID {
		return agentSmokeSummary{}, fmt.Errorf("unexpected evidence pack scope tenant=%q conversation=%q", pack.GetTenantId(), pack.GetConversationId())
	}
	if pack.GetQuery() != strings.TrimSpace(cfg.objective) {
		return agentSmokeSummary{}, fmt.Errorf("unexpected evidence pack query %q", pack.GetQuery())
	}

	var searchItem *retrievalv1.EvidenceItem
	var memoryItem *retrievalv1.EvidenceItem
	for _, item := range pack.GetItems() {
		switch item.GetSourceType() {
		case retrievalv1.EvidenceSourceType_EVIDENCE_SOURCE_TYPE_SEARCH_MESSAGE:
			searchItem = item
		case retrievalv1.EvidenceSourceType_EVIDENCE_SOURCE_TYPE_MEMORY_EVENT:
			memoryItem = item
		}
	}
	if searchItem == nil {
		return agentSmokeSummary{}, errors.New("missing SEARCH_MESSAGE evidence item")
	}
	if memoryItem == nil {
		return agentSmokeSummary{}, errors.New("missing MEMORY_EVENT evidence item")
	}

	verified := []string{}
	if err := verifyCitation(response.GetCitations()[0], seed); err != nil {
		return agentSmokeSummary{}, err
	}
	verified = append(verified, "proposal carries citation back to message source ref")
	if err := verifySearchItem(searchItem, seed); err != nil {
		return agentSmokeSummary{}, err
	}
	verified = append(verified, "search evidence preserved in returned EvidencePack")
	if err := verifyMemoryItem(memoryItem, seed); err != nil {
		return agentSmokeSummary{}, err
	}
	verified = append(verified, "memory evidence preserved with active temporal status and source ref")
	counts, err := verifySourceCoverage(pack, seed)
	if err != nil {
		return agentSmokeSummary{}, err
	}
	verified = append(verified, "source coverage and projection versions preserved")
	verified = append(verified, "tool policy allow decision observed before proposal generation")
	verified = append(verified, "first-stage response is proposal-only and generated_by_llm=false")

	return agentSmokeSummary{
		RunName:                 cfg.runName,
		ResultDir:               resultDir,
		AgentTarget:             cfg.agentTarget,
		Objective:               cfg.objective,
		ToolName:                cfg.toolName,
		ResourceType:            cfg.resourceType,
		RiskLevel:               cfg.riskLevel,
		Seed:                    seed,
		ProposalID:              response.GetProposalId(),
		ProposalStatus:          proposalStatusName(response.GetStatus()),
		ProposalText:            response.GetProposalText(),
		RequiresApproval:        response.GetRequiresApproval(),
		GeneratedByLLM:          response.GetGeneratedByLlm(),
		CitationCount:           len(response.GetCitations()),
		PolicyAllowed:           decision.GetAllowed(),
		PolicyRequiresApproval:  decision.GetRequiresApproval(),
		PolicyDecisionSource:    decision.GetDecisionSource(),
		PolicyClassification:    decision.GetClassification(),
		PolicyPermissionVersion: decision.GetPermissionVersion(),
		PackID:                  pack.GetPackId(),
		EvidenceItemCount:       len(pack.GetItems()),
		SearchItemCount:         int(counts.SearchMessage),
		MemoryItemCount:         int(counts.MemoryEvent),
		SourceCounts:            counts,
		SearchProjectionVersion: pack.GetSearchProjectionVersion(),
		MemoryProjectionVersion: pack.GetMemoryProjectionVersion(),
		AgentVersion:            response.GetAgentVersion(),
		RetrievalVersion:        pack.GetRetrievalVersion(),
		Verified:                verified,
		StartedAt:               startedAt,
		FinishedAt:              time.Now().UTC(),
	}, nil
}

func verifyCitation(citation *agentv1.AgentCitation, seed seededData) error {
	if citation.GetSourceId() != seed.MessageID {
		return fmt.Errorf("unexpected citation source_id %q", citation.GetSourceId())
	}
	if citation.GetSourceEventId() != seed.SourceEventID {
		return fmt.Errorf("unexpected citation source_event_id %q", citation.GetSourceEventId())
	}
	if citation.GetConversationId() != seed.ConversationID || citation.GetConversationSeq() != seed.ConversationSeq {
		return fmt.Errorf("unexpected citation conversation ref: %+v", citation)
	}
	return nil
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
		return errors.New("search item text is empty")
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
	if item.GetTemporalStatus() != "ACTIVE" {
		return fmt.Errorf("unexpected temporal status %q", item.GetTemporalStatus())
	}
	if item.GetReviewState() != "APPROVED" {
		return fmt.Errorf("unexpected review state %q", item.GetReviewState())
	}
	if item.GetExtractionVersion() != "agent-smoke-v1" {
		return fmt.Errorf("unexpected extraction version %q", item.GetExtractionVersion())
	}
	if len(item.GetSourceRefs()) == 0 {
		return errors.New("memory item missing source refs")
	}
	ref := item.GetSourceRefs()[0]
	if ref.GetSourceEventId() != seed.SourceEventID || ref.GetSourceId() != seed.MessageID {
		return fmt.Errorf("unexpected memory source ref: %+v", ref)
	}
	return nil
}

func verifySourceCoverage(pack *retrievalv1.EvidencePack, seed seededData) (sourceCounts, error) {
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
		return counts, fmt.Errorf("unexpected source counts: %+v", counts)
	}
	if pack.GetSearchProjectionVersion() != seed.VisibilityVersion {
		return counts, fmt.Errorf("unexpected search projection version %d", pack.GetSearchProjectionVersion())
	}
	if pack.GetMemoryProjectionVersion() != seed.MemoryProjectionVer {
		return counts, fmt.Errorf("unexpected memory projection version %d", pack.GetMemoryProjectionVersion())
	}
	coverageByType := map[retrievalv1.EvidenceSourceType]*retrievalv1.EvidenceSourceCoverage{}
	for _, coverage := range pack.GetSourceCoverage() {
		coverageByType[coverage.GetSourceType()] = coverage
	}
	for _, sourceType := range []retrievalv1.EvidenceSourceType{
		retrievalv1.EvidenceSourceType_EVIDENCE_SOURCE_TYPE_SEARCH_MESSAGE,
		retrievalv1.EvidenceSourceType_EVIDENCE_SOURCE_TYPE_MEMORY_EVENT,
	} {
		coverage := coverageByType[sourceType]
		if coverage == nil {
			return counts, fmt.Errorf("missing source coverage for %s", sourceType.String())
		}
		if coverage.GetStatus() != retrievalv1.EvidenceSourceCoverageStatus_EVIDENCE_SOURCE_COVERAGE_STATUS_RETURNED {
			return counts, fmt.Errorf("unexpected coverage status for %s: %s", sourceType.String(), coverage.GetStatus().String())
		}
		if !coverage.GetRequested() || coverage.GetCandidateCount() < 1 || coverage.GetReturnedCount() < 1 {
			return counts, fmt.Errorf("unexpected coverage counts for %s: %+v", sourceType.String(), coverage)
		}
	}
	if strings.TrimSpace(pack.GetRetrievalVersion()) == "" {
		return counts, errors.New("missing retrieval version")
	}
	return counts, nil
}

func proposalStatusName(status agentv1.AgentProposalStatus) string {
	switch status {
	case agentv1.AgentProposalStatus_AGENT_PROPOSAL_STATUS_PROPOSED:
		return "PROPOSED"
	case agentv1.AgentProposalStatus_AGENT_PROPOSAL_STATUS_BLOCKED:
		return "BLOCKED"
	case agentv1.AgentProposalStatus_AGENT_PROPOSAL_STATUS_INSUFFICIENT_EVIDENCE:
		return "INSUFFICIENT_EVIDENCE"
	default:
		return "UNSPECIFIED"
	}
}

func writeSummary(resultDir string, result agentSmokeSummary) error {
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(resultDir, "agent-proposal-summary.json")
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
		return "agent-smoke"
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
