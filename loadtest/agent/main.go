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
	actionexecutorv1 "github.com/qsyy0921/IM/api/proto/nexusim/actionexecutor/v1"
	agentv1 "github.com/qsyy0921/IM/api/proto/nexusim/agent/v1"
	policyv1 "github.com/qsyy0921/IM/api/proto/nexusim/policy/v1"
	retrievalv1 "github.com/qsyy0921/IM/api/proto/nexusim/retrieval/v1"
	"github.com/qsyy0921/IM/loadtest/internal/grpctls"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	defaultPGDSN        = "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable"
	defaultAgentTarget  = "127.0.0.1:10630"
	defaultActionTarget = "127.0.0.1:10660"
	defaultResultRoot   = `H:\NexusIM\loadtest-results`
	defaultObjective    = "phoenix launch decision"
	defaultToolName     = "conversation.note.create"
	defaultSkillID      = "conversation.note.create"
	defaultResource     = "conversation_note"

	scenarioApproved     = "approved"
	scenarioPolicyDenied = "policy-denied"
)

type config struct {
	pgDSN          string
	agentTarget    string
	actionTarget   string
	resultRoot     string
	runName        string
	tenantID       string
	conversationID string
	viewerUserID   string
	senderUserID   string
	deviceID       string
	objective      string
	toolName       string
	skillID        string
	resourceType   string
	resourceID     string
	riskLevel      string
	intent         string
	scenario       string
	expectExecuted bool
	requestTimeout time.Duration
	tls            grpctls.Config
}

type seededData struct {
	TenantID                 string `json:"tenant_id"`
	ConversationID           string `json:"conversation_id"`
	CrossGroupConversationID string `json:"cross_group_conversation_id,omitempty"`
	ViewerUserID             string `json:"viewer_user_id"`
	SenderUserID             string `json:"sender_user_id"`
	CrossGroupActorUserID    string `json:"cross_group_actor_user_id,omitempty"`
	MessageID                string `json:"message_id"`
	CrossGroupMessageID      string `json:"cross_group_message_id,omitempty"`
	SourceEventID            string `json:"source_event_id"`
	CrossGroupSourceEventID  string `json:"cross_group_source_event_id,omitempty"`
	MemoryEventID            string `json:"memory_event_id"`
	ProfileID                string `json:"profile_id,omitempty"`
	MemorySourceRefID        string `json:"memory_source_ref_id"`
	CrossGroupSourceRefID    string `json:"cross_group_source_ref_id,omitempty"`
	MemoryGraphEdgeID        string `json:"memory_graph_edge_id,omitempty"`
	ExpiredMemoryEventID     string `json:"expired_memory_event_id,omitempty"`
	SupersededMemoryEventID  string `json:"superseded_memory_event_id,omitempty"`
	FutureMemoryEventID      string `json:"future_memory_event_id,omitempty"`
	ConversationSeq          int64  `json:"conversation_seq"`
	VisibilityVersion        int64  `json:"visibility_version"`
	MemoryValidFromSeq       int64  `json:"memory_valid_from_seq"`
	MemoryValidToSeq         int64  `json:"memory_valid_to_seq"`
	MemoryProjectionVer      int64  `json:"memory_projection_version"`
	CurrentMemoryAtSeq       int64  `json:"current_memory_at_seq"`
}

type agentSmokeSummary struct {
	RunName                               string       `json:"run_name"`
	ResultDir                             string       `json:"result_dir"`
	AgentTarget                           string       `json:"agent_target"`
	ActionExecutorTarget                  string       `json:"action_executor_target"`
	Objective                             string       `json:"objective"`
	Scenario                              string       `json:"scenario"`
	ToolName                              string       `json:"tool_name"`
	SkillID                               string       `json:"skill_id"`
	ResourceType                          string       `json:"resource_type"`
	RiskLevel                             string       `json:"risk_level"`
	Seed                                  seededData   `json:"seed"`
	ProposalID                            string       `json:"proposal_id"`
	PreparedAuditID                       string       `json:"prepared_audit_id"`
	ProposalStatus                        string       `json:"proposal_status"`
	ApprovalID                            string       `json:"approval_id"`
	ApprovedByUserID                      string       `json:"approved_by_user_id"`
	ApprovedAtUnixMs                      int64        `json:"approved_at_unix_ms"`
	ExecutionID                           string       `json:"execution_id"`
	ExecutionStatus                       string       `json:"execution_status"`
	ExecutionAllowed                      bool         `json:"execution_allowed"`
	ExecutionRequiresApproval             bool         `json:"execution_requires_approval"`
	ExecutionDecisionSource               string       `json:"execution_decision_source"`
	ExecutionClassification               string       `json:"execution_classification"`
	ExecutionExecuted                     bool         `json:"execution_executed"`
	ExecutionResultID                     string       `json:"execution_result_id"`
	ExecutionResultStatus                 string       `json:"execution_result_status"`
	ExecutionResultRef                    string       `json:"execution_result_ref"`
	ProposalText                          string       `json:"proposal_text"`
	RequiresApproval                      bool         `json:"requires_approval"`
	GeneratedByLLM                        bool         `json:"generated_by_llm"`
	CitationCount                         int          `json:"citation_count"`
	PolicyAllowed                         bool         `json:"policy_allowed"`
	PolicyRequiresApproval                bool         `json:"policy_requires_approval"`
	PolicyDecisionSource                  string       `json:"policy_decision_source"`
	PolicyClassification                  string       `json:"policy_classification"`
	PolicyPermissionVersion               int64        `json:"policy_permission_version"`
	MCPAudit                              mcpAudit     `json:"mcp_audit"`
	ActionAudit                           actionAudit  `json:"action_audit"`
	PackID                                string       `json:"pack_id"`
	EvidenceItemCount                     int          `json:"evidence_item_count"`
	SearchItemCount                       int          `json:"search_item_count"`
	MemoryItemCount                       int          `json:"memory_item_count"`
	ProfileItemCount                      int          `json:"profile_item_count"`
	CurrentMemoryAtSeq                    int64        `json:"current_memory_at_seq"`
	ExpiredMemoryExcluded                 bool         `json:"expired_memory_excluded"`
	SupersededMemoryExcluded              bool         `json:"superseded_memory_excluded"`
	FutureMemoryExcluded                  bool         `json:"future_memory_excluded"`
	CrossGroupSourceRefsPreserved         bool         `json:"cross_group_source_refs_preserved"`
	CrossGroupSpeakerAttributionPreserved bool         `json:"cross_group_speaker_attribution_preserved"`
	MemoryGraphEdgesPreserved             bool         `json:"memory_graph_edges_preserved"`
	ProfileAggregatePreserved             bool         `json:"profile_aggregate_preserved"`
	SourceCounts                          sourceCounts `json:"source_counts"`
	SearchProjectionVersion               int64        `json:"search_projection_version"`
	MemoryProjectionVersion               int64        `json:"memory_projection_version"`
	AgentVersion                          string       `json:"agent_version"`
	RetrievalVersion                      string       `json:"retrieval_version"`
	Verified                              []string     `json:"verified"`
	StartedAt                             time.Time    `json:"started_at"`
	FinishedAt                            time.Time    `json:"finished_at"`
}

type mcpAudit struct {
	AuditID               string `json:"audit_id"`
	Status                string `json:"status"`
	Allowed               bool   `json:"allowed"`
	RequiresApproval      bool   `json:"requires_approval"`
	PermissionVersion     int64  `json:"permission_version"`
	Classification        string `json:"classification"`
	DecisionSource        string `json:"decision_source"`
	ToolAction            string `json:"tool_action"`
	InputSHA256Present    bool   `json:"input_sha256_present"`
	IdempotencyKeyMatches bool   `json:"idempotency_key_matches"`
	LowSensitiveAuditOnly bool   `json:"low_sensitive_audit_only"`
	RawInputColumnAbsent  bool   `json:"raw_input_column_absent"`
}

type actionAudit struct {
	ExecutionID            string `json:"execution_id"`
	Status                 string `json:"status"`
	Allowed                bool   `json:"allowed"`
	RequiresApproval       bool   `json:"requires_approval"`
	PermissionVersion      int64  `json:"permission_version"`
	Classification         string `json:"classification"`
	DecisionSource         string `json:"decision_source"`
	ToolAction             string `json:"tool_action"`
	Executed               bool   `json:"executed"`
	InputSHA256Present     bool   `json:"input_sha256_present"`
	OutputSHA256Present    bool   `json:"output_sha256_present"`
	RawInputColumnAbsent   bool   `json:"raw_input_column_absent"`
	RawOutputColumnAbsent  bool   `json:"raw_output_column_absent"`
	ResultRawOutputAbsent  bool   `json:"result_raw_output_column_absent"`
	ResultID               string `json:"result_id"`
	ResultStatus           string `json:"result_status"`
	ResultRefPresent       bool   `json:"result_ref_present"`
	ResultExecutionMatches bool   `json:"result_execution_matches"`
	ProposalIDMatches      bool   `json:"proposal_id_matches"`
	ApprovalIDMatches      bool   `json:"approval_id_matches"`
	PreparedAuditIDMatches bool   `json:"prepared_audit_id_matches"`
	LowSensitiveAuditOnly  bool   `json:"low_sensitive_audit_only"`
}

type sourceCounts struct {
	SearchMessage    int32 `json:"search_message"`
	MemoryEvent      int32 `json:"memory_event"`
	ProfileAggregate int32 `json:"profile_aggregate"`
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
	if err := seedToolingRows(ctx, pool, cfg); err != nil {
		return err
	}
	seed, err := seedProjectionRows(ctx, pool, cfg)
	if err != nil {
		return err
	}

	response, err := createProposal(ctx, cfg, seed)
	if err != nil {
		return err
	}
	if cfg.scenario == scenarioPolicyDenied {
		summary, err := verifyPolicyDeniedWorkflow(ctx, pool, cfg, resultDir, seed, response, startedAt)
		if err != nil {
			return err
		}
		return writeSummary(resultDir, summary)
	}
	approval, err := approveProposal(ctx, cfg, response.GetProposalId())
	if err != nil {
		return err
	}
	execution, err := executeApprovedAction(ctx, cfg, response, approval)
	if err != nil {
		return err
	}
	summary, err := verifyWorkflow(ctx, pool, cfg, resultDir, seed, response, approval, execution, startedAt)
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
	flagSet.StringVar(&cfg.actionTarget, "action-executor-target", defaultActionTarget, "action-executor gRPC address")
	flagSet.StringVar(&cfg.resultRoot, "result-root", defaultResultRoot, "external result root for raw smoke output")
	flagSet.StringVar(&cfg.runName, "run-name", "", "run name under result root")
	flagSet.StringVar(&cfg.tenantID, "tenant-id", "", "tenant id for seeded projection rows")
	flagSet.StringVar(&cfg.conversationID, "conversation-id", "", "conversation id for seeded projection rows")
	flagSet.StringVar(&cfg.viewerUserID, "viewer-user-id", "agent-viewer", "viewer user id")
	flagSet.StringVar(&cfg.senderUserID, "sender-user-id", "agent-sender", "sender user id")
	flagSet.StringVar(&cfg.deviceID, "device-id", "agent-device", "viewer device id")
	flagSet.StringVar(&cfg.objective, "objective", defaultObjective, "objective sent to agent-service")
	flagSet.StringVar(&cfg.toolName, "tool-name", defaultToolName, "tool name for policy precheck")
	flagSet.StringVar(&cfg.skillID, "skill-id", defaultSkillID, "skill id registered in skill-registry")
	flagSet.StringVar(&cfg.resourceType, "resource-type", defaultResource, "resource type for policy precheck")
	flagSet.StringVar(&cfg.resourceID, "resource-id", "", "resource id for policy precheck")
	flagSet.StringVar(&cfg.riskLevel, "risk-level", "LOW", "risk level for policy precheck")
	flagSet.StringVar(&cfg.intent, "intent", "", "intent for policy precheck")
	flagSet.StringVar(&cfg.scenario, "scenario", scenarioApproved, "smoke scenario: approved or policy-denied")
	flagSet.BoolVar(&cfg.expectExecuted, "expect-executed", false, "expect action-executor to run a safe local tool and record a successful result projection")
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
	cfg.actionTarget = strings.TrimSpace(cfg.actionTarget)
	cfg.objective = strings.TrimSpace(cfg.objective)
	cfg.toolName = strings.TrimSpace(cfg.toolName)
	cfg.skillID = strings.TrimSpace(cfg.skillID)
	cfg.resourceType = strings.TrimSpace(cfg.resourceType)
	cfg.riskLevel = strings.ToUpper(strings.TrimSpace(cfg.riskLevel))
	cfg.intent = strings.TrimSpace(cfg.intent)
	cfg.scenario = strings.ToLower(strings.TrimSpace(cfg.scenario))
	if cfg.agentTarget == "" {
		return config{}, errors.New("--agent-target is required")
	}
	if cfg.actionTarget == "" {
		return config{}, errors.New("--action-executor-target is required")
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
	if cfg.skillID == "" {
		cfg.skillID = cfg.toolName
	}
	if cfg.resourceType == "" {
		return config{}, errors.New("--resource-type is required")
	}
	if cfg.riskLevel == "" {
		cfg.riskLevel = "LOW"
	}
	if cfg.scenario == "" {
		cfg.scenario = scenarioApproved
	}
	if cfg.scenario != scenarioApproved && cfg.scenario != scenarioPolicyDenied {
		return config{}, fmt.Errorf("unsupported --scenario %q", cfg.scenario)
	}
	if cfg.scenario == scenarioPolicyDenied && cfg.expectExecuted {
		return config{}, errors.New("--expect-executed is incompatible with --scenario policy-denied")
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
		filepath.Join("migrations", "postgres", "skill-registry", "000001_skill_registry_core.sql"),
		filepath.Join("migrations", "postgres", "policy", "000014_policy_tool_action_rules.sql"),
		filepath.Join("migrations", "postgres", "mcp-gateway", "000001_mcp_gateway_core.sql"),
		filepath.Join("migrations", "postgres", "agent-service", "000001_agent_proposals.sql"),
		filepath.Join("migrations", "postgres", "action-executor", "000001_action_executor_core.sql"),
		filepath.Join("migrations", "postgres", "action-executor", "000002_action_executor_tool_results.sql"),
		filepath.Join("migrations", "postgres", "action-executor", "000003_action_executor_provider_failures.sql"),
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
		`DELETE FROM action_executor_provider_failures WHERE tenant_id = $1`,
		`DELETE FROM action_executor_tool_results WHERE tenant_id = $1`,
		`DELETE FROM action_executor_execution_audits WHERE tenant_id = $1`,
		`DELETE FROM agent_proposals WHERE tenant_id = $1`,
		`DELETE FROM mcp_gateway_tool_call_audits WHERE tenant_id = $1`,
		`DELETE FROM policy_tool_decision_audit WHERE tenant_id = $1`,
		`DELETE FROM policy_tool_action_rules WHERE tenant_id = $1`,
		`DELETE FROM skill_registry_definitions WHERE tenant_id = $1`,
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

func seedToolingRows(ctx context.Context, pool *pgxpool.Pool, cfg config) error {
	now := time.Now().UTC().Truncate(time.Millisecond)
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	skillRequiresApproval := cfg.scenario != scenarioPolicyDenied
	if _, err := tx.Exec(ctx, `
INSERT INTO skill_registry_definitions (
	tenant_id, skill_id, display_name, description, version, status, tool_name,
	allowed_actions_json, input_schema_json, output_schema_json, permission_scope,
	risk_level, requires_approval, audit_event_type, owner_service, tags_json,
	metadata_json, created_at, updated_at
) VALUES (
	$1, $2, $2, 'Agent smoke skill for controlled conversation note proposal.',
	'v1', 'ACTIVE', $3, '[1,3]'::jsonb,
	'{"type":"object","additionalProperties":true}'::jsonb,
	'{"type":"object","additionalProperties":true}'::jsonb,
	$4, $5, $6, 'conversation.note.proposed.v1', 'agent-service',
	'["agent-smoke"]'::jsonb, '{"source":"loadtest/agent"}'::jsonb, $7, $7
)
ON CONFLICT (tenant_id, skill_id) DO UPDATE SET
	display_name = EXCLUDED.display_name,
	description = EXCLUDED.description,
	version = EXCLUDED.version,
	status = EXCLUDED.status,
	tool_name = EXCLUDED.tool_name,
	allowed_actions_json = EXCLUDED.allowed_actions_json,
	input_schema_json = EXCLUDED.input_schema_json,
	output_schema_json = EXCLUDED.output_schema_json,
	permission_scope = EXCLUDED.permission_scope,
	risk_level = EXCLUDED.risk_level,
	requires_approval = EXCLUDED.requires_approval,
	audit_event_type = EXCLUDED.audit_event_type,
	owner_service = EXCLUDED.owner_service,
	tags_json = EXCLUDED.tags_json,
	metadata_json = EXCLUDED.metadata_json,
	updated_at = EXCLUDED.updated_at
`, cfg.tenantID, cfg.skillID, cfg.toolName, cfg.resourceType, cfg.riskLevel, skillRequiresApproval, now); err != nil {
		return fmt.Errorf("seed skill registry definition: %w", err)
	}

	allowed := true
	requiresApproval := true
	classification := "TOOL_APPROVAL_REQUIRED"
	reason := "agent smoke requires approval before execution"
	if cfg.scenario == scenarioPolicyDenied {
		allowed = false
		requiresApproval = false
		classification = "TOOL_POLICY_DENIED"
		reason = "agent smoke denies policy before proposal"
	}

	for _, action := range []string{"CALL", "EXECUTE"} {
		if _, err := tx.Exec(ctx, `
INSERT INTO policy_tool_action_rules (
	tenant_id, tool_name, action, resource_type, risk_level, allowed,
	requires_approval, permission_version, classification, reason, priority,
	enabled, source, updated_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, 42,
	$8, $9, 10, true, 'agent-smoke', $10
)
ON CONFLICT (tenant_id, tool_name, action, resource_type, risk_level) DO UPDATE SET
	allowed = EXCLUDED.allowed,
	requires_approval = EXCLUDED.requires_approval,
	permission_version = EXCLUDED.permission_version,
	classification = EXCLUDED.classification,
	reason = EXCLUDED.reason,
	priority = EXCLUDED.priority,
	enabled = EXCLUDED.enabled,
	source = EXCLUDED.source,
	updated_at = EXCLUDED.updated_at
`, cfg.tenantID, cfg.toolName, action, cfg.resourceType, cfg.riskLevel, allowed, requiresApproval, classification, reason, now); err != nil {
			return fmt.Errorf("seed policy tool action rule %s: %w", action, err)
		}
	}

	return tx.Commit(ctx)
}

func seedProjectionRows(ctx context.Context, pool *pgxpool.Pool, cfg config) (seededData, error) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	crossGroupConversationID := cfg.conversationID + "-strategy"
	crossGroupActorUserID := cfg.senderUserID + "-strategy"
	messageID := "msg-agent-" + randomSuffix()
	crossGroupMessageID := "msg-agent-cross-" + randomSuffix()
	sourceEventID := "evt-agent-" + randomSuffix()
	crossGroupSourceEventID := "evt-agent-cross-" + randomSuffix()
	memoryEventID := "mem-agent-" + randomSuffix()
	profileID := "profile-agent-" + randomSuffix()
	sourceRefID := "ref-agent-" + randomSuffix()
	crossGroupSourceRefID := "ref-agent-cross-" + randomSuffix()
	memoryGraphEdgeID := "edge-agent-supports-" + randomSuffix()
	expiredMemoryEventID := "mem-agent-expired-" + randomSuffix()
	expiredSourceRefID := "ref-agent-expired-" + randomSuffix()
	supersededMemoryEventID := "mem-agent-superseded-" + randomSuffix()
	supersededSourceRefID := "ref-agent-superseded-" + randomSuffix()
	futureMemoryEventID := "mem-agent-future-" + randomSuffix()
	futureSourceRefID := "ref-agent-future-" + randomSuffix()
	seq := int64(2)
	currentMemoryAtSeq := seq + 5
	visibilityVersion := int64(41)
	memoryProjectionVersion := int64(43)
	searchText := "The phoenix launch decision is approved with a follow-up note for rollout owners."
	crossGroupText := "The strategy group confirmed the phoenix launch decision before any agent proposal."
	factText := "Phoenix launch decision follow-up should preserve EvidencePack source references and cross group attribution before any action execution."
	actorJSON := jsonStringArray(cfg.senderUserID, crossGroupActorUserID)
	audienceJSON := jsonStringArray(cfg.viewerUserID)

	tx, err := pool.Begin(ctx)
	if err != nil {
		return seededData{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for _, table := range []string{"search_membership_projection", "memory_membership_projection"} {
		for _, conversationID := range []string{cfg.conversationID, crossGroupConversationID} {
			if _, err := tx.Exec(ctx, fmt.Sprintf(`
INSERT INTO %s (
	tenant_id, conversation_id, user_id, role, status, join_seq, leave_seq,
	member_version, permission_version, updated_by_event_id, updated_at
) VALUES ($1, $2, $3, 'MEMBER', 'ACTIVE', 1, NULL, 1, $4, $5, $6)
`, table), cfg.tenantID, conversationID, cfg.viewerUserID, visibilityVersion, "member-seed-"+sourceEventID, now); err != nil {
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
	$7::bigint, $8::bigint, $9, NULL, '[]'::jsonb,
	'[]'::jsonb, 0.9300, $10::bigint, 'agent-smoke-v1',
	$11::bigint, $9, $9
)
`, cfg.tenantID, memoryEventID, cfg.conversationID, factText, actorJSON, audienceJSON, seq, seq+10, now, visibilityVersion, memoryProjectionVersion); err != nil {
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

	staleMemories := []struct {
		eventID    string
		sourceRef  string
		status     string
		factText   string
		validFrom  int64
		validTo    int64
		confidence float64
	}{
		{
			eventID:    expiredMemoryEventID,
			sourceRef:  expiredSourceRefID,
			status:     "ACTIVE",
			factText:   "Expired phoenix launch action reminder is outside the requested conversation sequence window.",
			validFrom:  seq,
			validTo:    seq + 1,
			confidence: 0.8800,
		},
		{
			eventID:    supersededMemoryEventID,
			sourceRef:  supersededSourceRefID,
			status:     "SUPERSEDED",
			factText:   "Superseded phoenix launch action reminder should not appear as current evidence.",
			validFrom:  seq,
			validTo:    seq + 10,
			confidence: 0.8700,
		},
		{
			eventID:    futureMemoryEventID,
			sourceRef:  futureSourceRefID,
			status:     "ACTIVE",
			factText:   "Future phoenix launch action reminder should not appear before its valid sequence.",
			validFrom:  seq + 20,
			validTo:    seq + 30,
			confidence: 0.8600,
		},
	}
	for _, stale := range staleMemories {
		if _, err := tx.Exec(ctx, `
INSERT INTO memory_structured_events (
	tenant_id, memory_event_id, scope_type, scope_id, conversation_id, topic,
	event_type, status, review_state, fact_text, actor_user_ids, audience_user_ids,
	valid_from_seq, valid_to_seq, valid_from_at, valid_to_at, supersedes_event_ids,
	contradicts_event_ids, confidence, visibility_version, extraction_version,
	source_projection_version, created_at, updated_at
) VALUES (
	$1, $2, 'CONVERSATION', $3, $3, 'phoenix-launch',
	'DECISION', $4, 'APPROVED', $5, $6::jsonb, $7::jsonb,
	$8::bigint, $9::bigint, $10, NULL, '[]'::jsonb,
	'[]'::jsonb, $11, $12::bigint, 'agent-smoke-v1',
	$13::bigint, $10, $10
)
`, cfg.tenantID, stale.eventID, cfg.conversationID, stale.status, stale.factText, actorJSON, audienceJSON, stale.validFrom, stale.validTo, now, stale.confidence, visibilityVersion, memoryProjectionVersion); err != nil {
			return seededData{}, err
		}
		if _, err := tx.Exec(ctx, `
INSERT INTO memory_event_source_refs (
	tenant_id, memory_event_id, source_ref_id, source_type, source_id,
	source_event_id, conversation_id, conversation_seq, occurred_at, created_at
) VALUES ($1, $2, $3, 'MESSAGE', $4, $5, $6, $7, $8, $8)
`, cfg.tenantID, stale.eventID, stale.sourceRef, messageID, sourceEventID, cfg.conversationID, seq, now); err != nil {
			return seededData{}, err
		}
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
			'conversation_seq', $8::bigint
		),
		jsonb_build_object(
			'source_type', 'MESSAGE',
			'source_id', $9::text,
			'source_event_id', $10::text,
			'conversation_id', $11::text,
			'conversation_seq', $12::bigint
		)
	),
	$13
)
`, cfg.tenantID, memoryGraphEdgeID, memoryEventID, supersededMemoryEventID, messageID, sourceEventID, cfg.conversationID, seq, crossGroupMessageID, crossGroupSourceEventID, crossGroupConversationID, seq+1, now); err != nil {
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
		MemorySourceRefID:        sourceRefID,
		CrossGroupSourceRefID:    crossGroupSourceRefID,
		MemoryGraphEdgeID:        memoryGraphEdgeID,
		ExpiredMemoryEventID:     expiredMemoryEventID,
		SupersededMemoryEventID:  supersededMemoryEventID,
		FutureMemoryEventID:      futureMemoryEventID,
		ConversationSeq:          seq,
		VisibilityVersion:        visibilityVersion,
		MemoryValidFromSeq:       seq,
		MemoryValidToSeq:         seq + 10,
		MemoryProjectionVer:      memoryProjectionVersion,
		CurrentMemoryAtSeq:       currentMemoryAtSeq,
	}, nil
}

func createProposal(ctx context.Context, cfg config, seed seededData) (*agentv1.CreateAgentProposalResponse, error) {
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
		ConversationId:    cfg.conversationID,
		Objective:         cfg.objective,
		SkillId:           cfg.skillID,
		ToolName:          cfg.toolName,
		ToolAction:        policyv1.ToolAction_TOOL_ACTION_CALL,
		ResourceType:      cfg.resourceType,
		ResourceId:        cfg.resourceID,
		RiskLevel:         cfg.riskLevel,
		Intent:            cfg.intent,
		AtConversationSeq: seed.CurrentMemoryAtSeq,
		Limit:             10,
		IncludeSearch:     true,
		IncludeMemory:     true,
		MemoryStatuses: []retrievalv1.EvidenceMemoryStatus{
			retrievalv1.EvidenceMemoryStatus_EVIDENCE_MEMORY_STATUS_ACTIVE,
		},
	})
}

func approveProposal(
	ctx context.Context,
	cfg config,
	proposalID string,
) (*agentv1.ApproveAgentProposalResponse, error) {
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
	return agentv1.NewAgentServiceClient(conn).ApproveAgentProposal(requestCtx, &agentv1.ApproveAgentProposalRequest{
		AuthContext: &retrievalv1.AuthContext{
			TenantId:  cfg.tenantID,
			UserId:    cfg.viewerUserID,
			DeviceId:  cfg.deviceID,
			SessionId: "agent-smoke-session",
			TraceId:   "agent-smoke-trace",
			RequestId: "agent-smoke-approve-request",
		},
		ProposalId: proposalID,
		Reason:     "agent smoke approval for first-stage execution boundary",
	})
}

func executeApprovedAction(
	ctx context.Context,
	cfg config,
	proposal *agentv1.CreateAgentProposalResponse,
	approval *agentv1.ApproveAgentProposalResponse,
) (*actionexecutorv1.ExecuteApprovedActionResponse, error) {
	conn, err := grpc.NewClient(
		"passthrough:///"+cfg.actionTarget,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	inputJSON, err := json.Marshal(map[string]string{
		"conversation_id": cfg.conversationID,
		"objective":       cfg.objective,
		"source":          "loadtest/agent",
	})
	if err != nil {
		return nil, err
	}
	requestCtx, cancel := context.WithTimeout(ctx, cfg.requestTimeout)
	defer cancel()
	return actionexecutorv1.NewActionExecutorServiceClient(conn).ExecuteApprovedAction(
		requestCtx,
		&actionexecutorv1.ExecuteApprovedActionRequest{
			AuthContext: &actionexecutorv1.AuthContext{
				TenantId:  cfg.tenantID,
				UserId:    cfg.viewerUserID,
				DeviceId:  cfg.deviceID,
				SessionId: "agent-smoke-session",
				TraceId:   "agent-smoke-trace",
				RequestId: "agent-smoke-execute-request",
			},
			ProposalId:      proposal.GetProposalId(),
			ApprovalId:      approval.GetApprovalId(),
			PreparedAuditId: proposal.GetPreparedAuditId(),
			SkillId:         cfg.skillID,
			ToolName:        cfg.toolName,
			Action:          policyv1.ToolAction_TOOL_ACTION_EXECUTE,
			ResourceType:    cfg.resourceType,
			ResourceId:      cfg.resourceID,
			RiskLevel:       cfg.riskLevel,
			Intent:          cfg.intent,
			InputJson:       string(inputJSON),
			IdempotencyKey:  proposal.GetProposalId() + "-execute",
		},
	)
}

func verifyWorkflow(
	ctx context.Context,
	pool *pgxpool.Pool,
	cfg config,
	resultDir string,
	seed seededData,
	response *agentv1.CreateAgentProposalResponse,
	approval *agentv1.ApproveAgentProposalResponse,
	execution *actionexecutorv1.ExecuteApprovedActionResponse,
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
	if strings.TrimSpace(response.GetSkillId()) != cfg.skillID {
		return agentSmokeSummary{}, fmt.Errorf("unexpected skill_id %q", response.GetSkillId())
	}
	if strings.TrimSpace(response.GetPreparedAuditId()) == "" {
		return agentSmokeSummary{}, errors.New("missing prepared_audit_id")
	}
	if approval == nil {
		return agentSmokeSummary{}, errors.New("missing approval response")
	}
	if approval.GetProposalId() != response.GetProposalId() {
		return agentSmokeSummary{}, fmt.Errorf("approval proposal mismatch %q != %q", approval.GetProposalId(), response.GetProposalId())
	}
	if approval.GetStatus() != agentv1.AgentProposalStatus_AGENT_PROPOSAL_STATUS_APPROVED {
		return agentSmokeSummary{}, fmt.Errorf("unexpected approval status %v", approval.GetStatus())
	}
	if strings.TrimSpace(approval.GetApprovalId()) == "" {
		return agentSmokeSummary{}, errors.New("missing approval_id")
	}
	if approval.GetApprovedByUserId() != cfg.viewerUserID {
		return agentSmokeSummary{}, fmt.Errorf("unexpected approved_by_user_id %q", approval.GetApprovedByUserId())
	}
	if approval.GetApprovedAtUnixMs() <= 0 {
		return agentSmokeSummary{}, errors.New("missing approved_at")
	}
	if execution == nil {
		return agentSmokeSummary{}, errors.New("missing action execution response")
	}
	if execution.GetProposalId() != response.GetProposalId() ||
		execution.GetApprovalId() != approval.GetApprovalId() ||
		execution.GetPreparedAuditId() != response.GetPreparedAuditId() {
		return agentSmokeSummary{}, fmt.Errorf("action execution linkage mismatch: %+v", execution)
	}
	if execution.GetStatus() != actionexecutorv1.ActionExecutionStatus_ACTION_EXECUTION_STATUS_RECORDED {
		return agentSmokeSummary{}, fmt.Errorf("unexpected execution status %v", execution.GetStatus())
	}
	if execution.GetAction() != policyv1.ToolAction_TOOL_ACTION_EXECUTE {
		return agentSmokeSummary{}, fmt.Errorf("unexpected execution action %v", execution.GetAction())
	}
	if !execution.GetAllowed() || !execution.GetRequiresApproval() {
		return agentSmokeSummary{}, fmt.Errorf("unexpected execution decision allowed=%v requires_approval=%v", execution.GetAllowed(), execution.GetRequiresApproval())
	}
	if cfg.expectExecuted {
		if !execution.GetExecuted() {
			return agentSmokeSummary{}, errors.New("expected action-executor to execute safe local tool")
		}
		if execution.GetResultStatus() != "SUCCEEDED" || !strings.Contains(execution.GetOutputJson(), "local-safe-echo") {
			return agentSmokeSummary{}, fmt.Errorf("unexpected executed result: status=%q output=%q", execution.GetResultStatus(), execution.GetOutputJson())
		}
	} else if execution.GetExecuted() {
		return agentSmokeSummary{}, errors.New("first-stage action-executor must not execute external tool")
	}
	if strings.TrimSpace(execution.GetExecutionId()) == "" {
		return agentSmokeSummary{}, errors.New("missing execution_id")
	}
	expectedResultStatus := "NOT_EXECUTED"
	if cfg.expectExecuted {
		expectedResultStatus = "SUCCEEDED"
	}
	if strings.TrimSpace(execution.GetResultId()) == "" || execution.GetResultStatus() != expectedResultStatus || strings.TrimSpace(execution.GetResultRef()) == "" {
		return agentSmokeSummary{}, fmt.Errorf("unexpected execution result projection fields: result_id=%q status=%q ref=%q", execution.GetResultId(), execution.GetResultStatus(), execution.GetResultRef())
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
	var profileItem *retrievalv1.EvidenceItem
	for _, item := range pack.GetItems() {
		switch item.GetSourceType() {
		case retrievalv1.EvidenceSourceType_EVIDENCE_SOURCE_TYPE_SEARCH_MESSAGE:
			searchItem = item
		case retrievalv1.EvidenceSourceType_EVIDENCE_SOURCE_TYPE_MEMORY_EVENT:
			memoryItem = item
		case retrievalv1.EvidenceSourceType_EVIDENCE_SOURCE_TYPE_PROFILE_AGGREGATE:
			profileItem = item
		}
	}
	if searchItem == nil {
		return agentSmokeSummary{}, errors.New("missing SEARCH_MESSAGE evidence item")
	}
	if memoryItem == nil {
		return agentSmokeSummary{}, errors.New("missing MEMORY_EVENT evidence item")
	}
	if profileItem == nil {
		return agentSmokeSummary{}, errors.New("missing PROFILE_AGGREGATE evidence item")
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
	if err := verifyProfileItem(profileItem, seed); err != nil {
		return agentSmokeSummary{}, err
	}
	verified = append(verified, "profile aggregate evidence preserved with supporting memory ids")
	verified = append(verified, "cross-group memory source refs and speaker attribution preserved in EvidencePack")
	verified = append(verified, "memory graph edge preserved in EvidencePack")
	currentCheck, err := verifyCurrentMemoryOnly(pack, seed)
	if err != nil {
		return agentSmokeSummary{}, err
	}
	verified = append(verified, "expired, superseded and future memory decoys excluded from current EvidencePack")
	counts, err := verifySourceCoverage(pack, seed)
	if err != nil {
		return agentSmokeSummary{}, err
	}
	verified = append(verified, "source coverage and projection versions preserved")
	verified = append(verified, "tool policy allow decision observed before proposal generation")
	audit, err := verifyMCPAudit(ctx, pool, cfg, response)
	if err != nil {
		return agentSmokeSummary{}, err
	}
	verified = append(verified, "mcp-gateway prepare audit recorded before proposal generation")
	actionAudit, err := verifyActionAudit(ctx, pool, cfg, response, approval, execution)
	if err != nil {
		return agentSmokeSummary{}, err
	}
	verified = append(verified, "agent proposal approval recorded and verified by action-executor")
	if cfg.expectExecuted {
		verified = append(verified, "action-executor executed safe local tool and recorded output hash only")
	} else {
		verified = append(verified, "action-executor recorded approved execution audit with executed=false")
	}
	verified = append(verified, "action-executor recorded low-sensitive tool result projection")
	verified = append(verified, "first-stage response is proposal-only and generated_by_llm=false")

	return agentSmokeSummary{
		RunName:                               cfg.runName,
		ResultDir:                             resultDir,
		AgentTarget:                           cfg.agentTarget,
		ActionExecutorTarget:                  cfg.actionTarget,
		Objective:                             cfg.objective,
		Scenario:                              cfg.scenario,
		ToolName:                              cfg.toolName,
		SkillID:                               cfg.skillID,
		ResourceType:                          cfg.resourceType,
		RiskLevel:                             cfg.riskLevel,
		Seed:                                  seed,
		ProposalID:                            response.GetProposalId(),
		PreparedAuditID:                       response.GetPreparedAuditId(),
		ProposalStatus:                        proposalStatusName(approval.GetStatus()),
		ApprovalID:                            approval.GetApprovalId(),
		ApprovedByUserID:                      approval.GetApprovedByUserId(),
		ApprovedAtUnixMs:                      approval.GetApprovedAtUnixMs(),
		ExecutionID:                           execution.GetExecutionId(),
		ExecutionStatus:                       executionStatusName(execution.GetStatus()),
		ExecutionAllowed:                      execution.GetAllowed(),
		ExecutionRequiresApproval:             execution.GetRequiresApproval(),
		ExecutionDecisionSource:               execution.GetDecisionSource(),
		ExecutionClassification:               execution.GetClassification(),
		ExecutionExecuted:                     execution.GetExecuted(),
		ExecutionResultID:                     execution.GetResultId(),
		ExecutionResultStatus:                 execution.GetResultStatus(),
		ExecutionResultRef:                    execution.GetResultRef(),
		ProposalText:                          response.GetProposalText(),
		RequiresApproval:                      response.GetRequiresApproval(),
		GeneratedByLLM:                        response.GetGeneratedByLlm(),
		CitationCount:                         len(response.GetCitations()),
		PolicyAllowed:                         decision.GetAllowed(),
		PolicyRequiresApproval:                decision.GetRequiresApproval(),
		PolicyDecisionSource:                  decision.GetDecisionSource(),
		PolicyClassification:                  decision.GetClassification(),
		PolicyPermissionVersion:               decision.GetPermissionVersion(),
		MCPAudit:                              audit,
		ActionAudit:                           actionAudit,
		PackID:                                pack.GetPackId(),
		EvidenceItemCount:                     len(pack.GetItems()),
		SearchItemCount:                       int(counts.SearchMessage),
		MemoryItemCount:                       int(counts.MemoryEvent),
		ProfileItemCount:                      int(counts.ProfileAggregate),
		CurrentMemoryAtSeq:                    currentCheck.AtConversationSeq,
		ExpiredMemoryExcluded:                 currentCheck.ExpiredMemoryExcluded,
		SupersededMemoryExcluded:              currentCheck.SupersededMemoryExcluded,
		FutureMemoryExcluded:                  currentCheck.FutureMemoryExcluded,
		CrossGroupSourceRefsPreserved:         true,
		CrossGroupSpeakerAttributionPreserved: true,
		MemoryGraphEdgesPreserved:             true,
		ProfileAggregatePreserved:             true,
		SourceCounts:                          counts,
		SearchProjectionVersion:               pack.GetSearchProjectionVersion(),
		MemoryProjectionVersion:               pack.GetMemoryProjectionVersion(),
		AgentVersion:                          response.GetAgentVersion(),
		RetrievalVersion:                      pack.GetRetrievalVersion(),
		Verified:                              verified,
		StartedAt:                             startedAt,
		FinishedAt:                            time.Now().UTC(),
	}, nil
}

func verifyPolicyDeniedWorkflow(
	ctx context.Context,
	pool *pgxpool.Pool,
	cfg config,
	resultDir string,
	seed seededData,
	response *agentv1.CreateAgentProposalResponse,
	startedAt time.Time,
) (agentSmokeSummary, error) {
	if response.GetStatus() != agentv1.AgentProposalStatus_AGENT_PROPOSAL_STATUS_BLOCKED {
		return agentSmokeSummary{}, fmt.Errorf("unexpected denied proposal status %v", response.GetStatus())
	}
	if response.GetGeneratedByLlm() {
		return agentSmokeSummary{}, errors.New("policy-denied proposal must not claim LLM generation")
	}
	if response.GetRequiresApproval() {
		return agentSmokeSummary{}, errors.New("policy-denied proposal must not require approval")
	}
	if strings.TrimSpace(response.GetProposalText()) == "" {
		return agentSmokeSummary{}, errors.New("policy-denied proposal text is empty")
	}
	if len(response.GetCitations()) != 0 {
		return agentSmokeSummary{}, errors.New("policy-denied proposal must not include citations")
	}
	if response.GetEvidencePack() != nil && len(response.GetEvidencePack().GetItems()) != 0 {
		return agentSmokeSummary{}, errors.New("policy-denied proposal must not retrieve evidence")
	}
	if strings.TrimSpace(response.GetPreparedAuditId()) == "" {
		return agentSmokeSummary{}, errors.New("missing denied prepared_audit_id")
	}
	if strings.TrimSpace(response.GetAgentVersion()) == "" {
		return agentSmokeSummary{}, errors.New("missing agent_version")
	}
	decision := response.GetToolPolicyDecision()
	if decision == nil {
		return agentSmokeSummary{}, errors.New("missing denied tool policy decision")
	}
	if decision.GetAllowed() || decision.GetRequiresApproval() {
		return agentSmokeSummary{}, fmt.Errorf("unexpected denied policy decision allowed=%v requires_approval=%v", decision.GetAllowed(), decision.GetRequiresApproval())
	}
	if decision.GetAction() != policyv1.ToolAction_TOOL_ACTION_CALL {
		return agentSmokeSummary{}, fmt.Errorf("unexpected denied tool action %v", decision.GetAction())
	}
	if decision.GetToolName() != cfg.toolName || decision.GetResourceType() != cfg.resourceType {
		return agentSmokeSummary{}, fmt.Errorf("unexpected denied policy scope: %+v", decision)
	}
	if decision.GetClassification() != "TOOL_POLICY_DENIED" || decision.GetDecisionSource() != "TOOL_RULE" || decision.GetPermissionVersion() != 42 {
		return agentSmokeSummary{}, fmt.Errorf("unexpected denied policy metadata: %+v", decision)
	}

	audit, err := verifyMCPAuditDenied(ctx, pool, cfg, response)
	if err != nil {
		return agentSmokeSummary{}, err
	}
	verified := []string{
		"agent proposal was blocked by policy before retrieval",
		"policy-denied branch produced no approval or execution",
		"mcp-gateway recorded blocked prepare audit with input hash only",
	}
	return agentSmokeSummary{
		RunName:                 cfg.runName,
		ResultDir:               resultDir,
		AgentTarget:             cfg.agentTarget,
		ActionExecutorTarget:    cfg.actionTarget,
		Objective:               cfg.objective,
		Scenario:                cfg.scenario,
		ToolName:                cfg.toolName,
		SkillID:                 cfg.skillID,
		ResourceType:            cfg.resourceType,
		RiskLevel:               cfg.riskLevel,
		Seed:                    seed,
		ProposalID:              response.GetProposalId(),
		PreparedAuditID:         response.GetPreparedAuditId(),
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
		MCPAudit:                audit,
		AgentVersion:            response.GetAgentVersion(),
		Verified:                verified,
		StartedAt:               startedAt,
		FinishedAt:              time.Now().UTC(),
	}, nil
}

func verifyMCPAudit(
	ctx context.Context,
	pool *pgxpool.Pool,
	cfg config,
	response *agentv1.CreateAgentProposalResponse,
) (mcpAudit, error) {
	var audit mcpAudit
	var inputSHA256 string
	var idempotencyKey string
	var resourceID string
	err := pool.QueryRow(ctx, `
SELECT
	audit_id,
	status,
	allowed,
	requires_approval,
	permission_version,
	classification,
	decision_source,
	tool_action,
	input_sha256,
	idempotency_key,
	resource_id
FROM mcp_gateway_tool_call_audits
WHERE tenant_id = $1
  AND audit_id = $2
  AND skill_id = $3
  AND tool_name = $4
  AND resource_type = $5
  AND risk_level = $6
`, cfg.tenantID, response.GetPreparedAuditId(), cfg.skillID, cfg.toolName, cfg.resourceType, cfg.riskLevel).Scan(
		&audit.AuditID,
		&audit.Status,
		&audit.Allowed,
		&audit.RequiresApproval,
		&audit.PermissionVersion,
		&audit.Classification,
		&audit.DecisionSource,
		&audit.ToolAction,
		&inputSHA256,
		&idempotencyKey,
		&resourceID,
	)
	if err != nil {
		return mcpAudit{}, fmt.Errorf("query mcp-gateway prepare audit: %w", err)
	}
	rawInputColumnAbsent, err := columnAbsent(ctx, pool, "mcp_gateway_tool_call_audits", "input_json")
	if err != nil {
		return mcpAudit{}, err
	}
	audit.InputSHA256Present = strings.TrimSpace(inputSHA256) != ""
	audit.IdempotencyKeyMatches = idempotencyKey == response.GetProposalId()
	audit.RawInputColumnAbsent = rawInputColumnAbsent
	audit.LowSensitiveAuditOnly = audit.InputSHA256Present && audit.RawInputColumnAbsent
	if audit.Status != "ALLOWED" || !audit.Allowed || !audit.RequiresApproval {
		return mcpAudit{}, fmt.Errorf("unexpected mcp audit decision: %+v", audit)
	}
	if audit.ToolAction != "CALL" {
		return mcpAudit{}, fmt.Errorf("unexpected mcp audit action %q", audit.ToolAction)
	}
	if audit.PermissionVersion != 42 || audit.Classification != "TOOL_APPROVAL_REQUIRED" || audit.DecisionSource != "TOOL_RULE" {
		return mcpAudit{}, fmt.Errorf("unexpected mcp audit policy metadata: %+v", audit)
	}
	if !audit.InputSHA256Present || !audit.RawInputColumnAbsent {
		return mcpAudit{}, errors.New("mcp audit must store low-sensitive input_sha256 and no raw input column")
	}
	if !audit.IdempotencyKeyMatches {
		return mcpAudit{}, fmt.Errorf("mcp audit idempotency key mismatch: %q != %q", idempotencyKey, response.GetProposalId())
	}
	if resourceID != cfg.resourceID {
		return mcpAudit{}, fmt.Errorf("unexpected mcp audit resource_id %q", resourceID)
	}
	return audit, nil
}

func verifyMCPAuditDenied(
	ctx context.Context,
	pool *pgxpool.Pool,
	cfg config,
	response *agentv1.CreateAgentProposalResponse,
) (mcpAudit, error) {
	var audit mcpAudit
	var inputSHA256 string
	var idempotencyKey string
	var resourceID string
	err := pool.QueryRow(ctx, `
SELECT
	audit_id,
	status,
	allowed,
	requires_approval,
	permission_version,
	classification,
	decision_source,
	tool_action,
	input_sha256,
	idempotency_key,
	resource_id
FROM mcp_gateway_tool_call_audits
WHERE tenant_id = $1
  AND audit_id = $2
  AND skill_id = $3
  AND tool_name = $4
  AND resource_type = $5
  AND risk_level = $6
`, cfg.tenantID, response.GetPreparedAuditId(), cfg.skillID, cfg.toolName, cfg.resourceType, cfg.riskLevel).Scan(
		&audit.AuditID,
		&audit.Status,
		&audit.Allowed,
		&audit.RequiresApproval,
		&audit.PermissionVersion,
		&audit.Classification,
		&audit.DecisionSource,
		&audit.ToolAction,
		&inputSHA256,
		&idempotencyKey,
		&resourceID,
	)
	if err != nil {
		return mcpAudit{}, fmt.Errorf("query denied mcp-gateway prepare audit: %w", err)
	}
	rawInputColumnAbsent, err := columnAbsent(ctx, pool, "mcp_gateway_tool_call_audits", "input_json")
	if err != nil {
		return mcpAudit{}, err
	}
	audit.InputSHA256Present = strings.TrimSpace(inputSHA256) != ""
	audit.IdempotencyKeyMatches = idempotencyKey == response.GetProposalId()
	audit.RawInputColumnAbsent = rawInputColumnAbsent
	audit.LowSensitiveAuditOnly = audit.InputSHA256Present && audit.RawInputColumnAbsent
	if audit.Status != "BLOCKED" || audit.Allowed || audit.RequiresApproval {
		return mcpAudit{}, fmt.Errorf("unexpected denied mcp audit decision: %+v", audit)
	}
	if audit.ToolAction != "CALL" {
		return mcpAudit{}, fmt.Errorf("unexpected denied mcp audit action %q", audit.ToolAction)
	}
	if audit.PermissionVersion != 42 || audit.Classification != "TOOL_POLICY_DENIED" || audit.DecisionSource != "TOOL_RULE" {
		return mcpAudit{}, fmt.Errorf("unexpected denied mcp audit policy metadata: %+v", audit)
	}
	if !audit.InputSHA256Present || !audit.RawInputColumnAbsent {
		return mcpAudit{}, errors.New("denied mcp audit must store input_sha256 and no raw input column")
	}
	if !audit.IdempotencyKeyMatches {
		return mcpAudit{}, fmt.Errorf("denied mcp audit idempotency key mismatch: %q != %q", idempotencyKey, response.GetProposalId())
	}
	if resourceID != cfg.resourceID {
		return mcpAudit{}, fmt.Errorf("unexpected denied mcp audit resource_id %q", resourceID)
	}
	return audit, nil
}

func verifyActionAudit(
	ctx context.Context,
	pool *pgxpool.Pool,
	cfg config,
	proposal *agentv1.CreateAgentProposalResponse,
	approval *agentv1.ApproveAgentProposalResponse,
	execution *actionexecutorv1.ExecuteApprovedActionResponse,
) (actionAudit, error) {
	var audit actionAudit
	var inputSHA256 string
	var outputSHA256 string
	var proposalID string
	var approvalID string
	var preparedAuditID string
	err := pool.QueryRow(ctx, `
SELECT
	execution_id,
	proposal_id,
	approval_id,
	prepared_audit_id,
	status,
	allowed,
	requires_approval,
	permission_version,
	classification,
	decision_source,
	tool_action,
	executed,
	input_sha256,
	output_sha256
FROM action_executor_execution_audits
WHERE tenant_id = $1
  AND execution_id = $2
  AND skill_id = $3
  AND tool_name = $4
  AND resource_type = $5
  AND resource_id = $6
  AND risk_level = $7
`, cfg.tenantID, execution.GetExecutionId(), cfg.skillID, cfg.toolName, cfg.resourceType, cfg.resourceID, cfg.riskLevel).Scan(
		&audit.ExecutionID,
		&proposalID,
		&approvalID,
		&preparedAuditID,
		&audit.Status,
		&audit.Allowed,
		&audit.RequiresApproval,
		&audit.PermissionVersion,
		&audit.Classification,
		&audit.DecisionSource,
		&audit.ToolAction,
		&audit.Executed,
		&inputSHA256,
		&outputSHA256,
	)
	if err != nil {
		return actionAudit{}, fmt.Errorf("query action-executor execution audit: %w", err)
	}
	rawInputColumnAbsent, err := columnAbsent(ctx, pool, "action_executor_execution_audits", "input_json")
	if err != nil {
		return actionAudit{}, err
	}
	rawOutputColumnAbsent, err := columnAbsent(ctx, pool, "action_executor_execution_audits", "output_json")
	if err != nil {
		return actionAudit{}, err
	}
	resultRawOutputAbsent, err := columnAbsent(ctx, pool, "action_executor_tool_results", "output_json")
	if err != nil {
		return actionAudit{}, err
	}
	audit.InputSHA256Present = strings.TrimSpace(inputSHA256) != ""
	audit.OutputSHA256Present = strings.TrimSpace(outputSHA256) != ""
	audit.RawInputColumnAbsent = rawInputColumnAbsent
	audit.RawOutputColumnAbsent = rawOutputColumnAbsent
	audit.ResultRawOutputAbsent = resultRawOutputAbsent
	audit.ProposalIDMatches = proposalID == proposal.GetProposalId()
	audit.ApprovalIDMatches = approvalID == approval.GetApprovalId()
	audit.PreparedAuditIDMatches = preparedAuditID == proposal.GetPreparedAuditId()
	audit.LowSensitiveAuditOnly = audit.InputSHA256Present &&
		audit.RawInputColumnAbsent &&
		audit.RawOutputColumnAbsent &&
		audit.ResultRawOutputAbsent &&
		(!audit.Executed || audit.OutputSHA256Present)
	var resultExecutionID string
	var resultRef string
	err = pool.QueryRow(ctx, `
SELECT result_id, execution_id, status, result_ref
FROM action_executor_tool_results
WHERE tenant_id = $1
  AND execution_id = $2
  AND proposal_id = $3
  AND approval_id = $4
  AND prepared_audit_id = $5
`, cfg.tenantID, execution.GetExecutionId(), proposal.GetProposalId(), approval.GetApprovalId(), proposal.GetPreparedAuditId()).Scan(
		&audit.ResultID,
		&resultExecutionID,
		&audit.ResultStatus,
		&resultRef,
	)
	if err != nil {
		return actionAudit{}, fmt.Errorf("query action-executor tool result projection: %w", err)
	}
	audit.ResultRefPresent = strings.TrimSpace(resultRef) != ""
	audit.ResultExecutionMatches = resultExecutionID == execution.GetExecutionId()
	if audit.Status != "RECORDED" || !audit.Allowed || !audit.RequiresApproval {
		return actionAudit{}, fmt.Errorf("unexpected action audit decision: %+v", audit)
	}
	if audit.ToolAction != "EXECUTE" {
		return actionAudit{}, fmt.Errorf("unexpected action audit action %q", audit.ToolAction)
	}
	if audit.PermissionVersion != 42 || audit.Classification != "TOOL_APPROVAL_REQUIRED" || audit.DecisionSource != "TOOL_RULE" {
		return actionAudit{}, fmt.Errorf("unexpected action audit policy metadata: %+v", audit)
	}
	if audit.Executed != cfg.expectExecuted {
		return actionAudit{}, fmt.Errorf("unexpected action audit executed=%v expect=%v", audit.Executed, cfg.expectExecuted)
	}
	if !audit.InputSHA256Present {
		return actionAudit{}, errors.New("action audit must store low-sensitive input_sha256")
	}
	expectedResultStatus := "NOT_EXECUTED"
	if cfg.expectExecuted {
		expectedResultStatus = "SUCCEEDED"
		if !audit.OutputSHA256Present {
			return actionAudit{}, errors.New("executed action audit must store output_sha256 only")
		}
	} else if audit.OutputSHA256Present {
		return actionAudit{}, errors.New("not-executed action audit must not store output_sha256")
	}
	if audit.ResultID != execution.GetResultId() || audit.ResultStatus != expectedResultStatus || !audit.ResultRefPresent || !audit.ResultExecutionMatches {
		return actionAudit{}, fmt.Errorf("unexpected action result projection: %+v", audit)
	}
	if !audit.ProposalIDMatches || !audit.ApprovalIDMatches || !audit.PreparedAuditIDMatches {
		return actionAudit{}, fmt.Errorf("action audit linkage mismatch: %+v", audit)
	}
	return audit, nil
}

func columnAbsent(ctx context.Context, pool *pgxpool.Pool, tableName string, columnName string) (bool, error) {
	var count int
	if err := pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM information_schema.columns
WHERE table_schema = 'public'
  AND table_name = $1
  AND column_name = $2
`, tableName, columnName).Scan(&count); err != nil {
		return false, fmt.Errorf("query column presence %s.%s: %w", tableName, columnName, err)
	}
	return count == 0, nil
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
	primaryRef, ok := findSourceRef(item.GetSourceRefs(), seed.MessageID, seed.SourceEventID, seed.ConversationID)
	if !ok {
		return fmt.Errorf("memory item missing primary source ref for message %q", seed.MessageID)
	}
	if primaryRef.GetConversationSeq() != seed.ConversationSeq {
		return fmt.Errorf("unexpected primary source ref seq: %+v", primaryRef)
	}
	crossRef, ok := findSourceRef(item.GetSourceRefs(), seed.CrossGroupMessageID, seed.CrossGroupSourceEventID, seed.CrossGroupConversationID)
	if !ok {
		return fmt.Errorf("memory item missing cross-group source ref for message %q", seed.CrossGroupMessageID)
	}
	if crossRef.GetConversationSeq() != seed.ConversationSeq+1 {
		return fmt.Errorf("unexpected cross-group source ref seq: %+v", crossRef)
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

type currentMemoryCheck struct {
	AtConversationSeq        int64
	ExpiredMemoryExcluded    bool
	SupersededMemoryExcluded bool
	FutureMemoryExcluded     bool
}

func verifyCurrentMemoryOnly(pack *retrievalv1.EvidencePack, seed seededData) (currentMemoryCheck, error) {
	check := currentMemoryCheck{
		AtConversationSeq:        seed.CurrentMemoryAtSeq,
		ExpiredMemoryExcluded:    true,
		SupersededMemoryExcluded: true,
		FutureMemoryExcluded:     true,
	}
	currentFound := false
	for _, item := range pack.GetItems() {
		if item.GetSourceType() != retrievalv1.EvidenceSourceType_EVIDENCE_SOURCE_TYPE_MEMORY_EVENT {
			continue
		}
		switch item.GetMemoryEventId() {
		case seed.MemoryEventID:
			currentFound = true
		case seed.ExpiredMemoryEventID:
			check.ExpiredMemoryExcluded = false
		case seed.SupersededMemoryEventID:
			check.SupersededMemoryExcluded = false
		case seed.FutureMemoryEventID:
			check.FutureMemoryExcluded = false
		}
	}
	if !currentFound {
		return check, errors.New("current memory event missing from EvidencePack")
	}
	if !check.ExpiredMemoryExcluded {
		return check, fmt.Errorf("expired memory event %q returned as current evidence", seed.ExpiredMemoryEventID)
	}
	if !check.SupersededMemoryExcluded {
		return check, fmt.Errorf("superseded memory event %q returned as current evidence", seed.SupersededMemoryEventID)
	}
	if !check.FutureMemoryExcluded {
		return check, fmt.Errorf("future memory event %q returned as current evidence", seed.FutureMemoryEventID)
	}
	return check, nil
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

func verifySourceCoverage(pack *retrievalv1.EvidencePack, seed seededData) (sourceCounts, error) {
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
	if counts.SearchMessage < 1 || counts.MemoryEvent < 1 || counts.ProfileAggregate < 1 {
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
		retrievalv1.EvidenceSourceType_EVIDENCE_SOURCE_TYPE_PROFILE_AGGREGATE,
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
	case agentv1.AgentProposalStatus_AGENT_PROPOSAL_STATUS_APPROVED:
		return "APPROVED"
	default:
		return "UNSPECIFIED"
	}
}

func executionStatusName(status actionexecutorv1.ActionExecutionStatus) string {
	switch status {
	case actionexecutorv1.ActionExecutionStatus_ACTION_EXECUTION_STATUS_RECORDED:
		return "RECORDED"
	case actionexecutorv1.ActionExecutionStatus_ACTION_EXECUTION_STATUS_BLOCKED:
		return "BLOCKED"
	case actionexecutorv1.ActionExecutionStatus_ACTION_EXECUTION_STATUS_FAILED:
		return "FAILED"
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
