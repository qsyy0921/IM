package main

import (
	"context"
	"crypto/rand"
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
)

const (
	defaultPGDSN        = "postgres://nexusim:nexusim@localhost:5432/nexusim?sslmode=disable"
	defaultRAGTarget    = "127.0.0.1:10610"
	defaultAgentTarget  = "127.0.0.1:10630"
	defaultActionTarget = "127.0.0.1:10660"
	defaultResultRoot   = `H:\NexusIM\loadtest-results`
	defaultQuestion     = "phoenix launch decision"
	defaultObjective    = "phoenix launch decision"
)

type config struct {
	pgDSN          string
	ragTarget      string
	agentTarget    string
	actionTarget   string
	resultRoot     string
	runName        string
	tenantID       string
	conversationID string
	viewerUserID   string
	senderUserID   string
	deviceID       string
	question       string
	objective      string
	expectExecuted bool
	requestTimeout time.Duration
	ragTLS         tlsFlags
	agentTLS       tlsFlags
}

type tlsFlags struct {
	caFile         string
	serverName     string
	clientCertFile string
	clientKeyFile  string
}

type ragPartialSummary struct {
	RunName                               string      `json:"run_name"`
	ResultDir                             string      `json:"result_dir"`
	RAGTarget                             string      `json:"rag_target"`
	Question                              string      `json:"question"`
	Seed                                  seedSummary `json:"seed"`
	AnswerID                              string      `json:"answer_id"`
	AnswerStatus                          string      `json:"answer_status"`
	AnswerText                            string      `json:"answer_text"`
	CitationCount                         int         `json:"citation_count"`
	EvidenceItemCount                     int         `json:"evidence_item_count"`
	SearchItemCount                       int         `json:"search_item_count"`
	MemoryItemCount                       int         `json:"memory_item_count"`
	ProfileItemCount                      int         `json:"profile_item_count"`
	CrossGroupSourceRefsPreserved         bool        `json:"cross_group_source_refs_preserved"`
	CrossGroupSpeakerAttributionPreserved bool        `json:"cross_group_speaker_attribution_preserved"`
	MemoryGraphEdgesPreserved             bool        `json:"memory_graph_edges_preserved"`
	ProfileAggregatePreserved             bool        `json:"profile_aggregate_preserved"`
	RAGVersion                            string      `json:"rag_version"`
	RetrievalVersion                      string      `json:"retrieval_version"`
	Verified                              []string    `json:"verified"`
	StartedAt                             time.Time   `json:"started_at"`
	FinishedAt                            time.Time   `json:"finished_at"`
}

type agentPartialSummary struct {
	RunName                               string      `json:"run_name"`
	ResultDir                             string      `json:"result_dir"`
	AgentTarget                           string      `json:"agent_target"`
	ActionExecutorTarget                  string      `json:"action_executor_target"`
	Objective                             string      `json:"objective"`
	Seed                                  seedSummary `json:"seed"`
	ProposalID                            string      `json:"proposal_id"`
	PreparedAuditID                       string      `json:"prepared_audit_id"`
	ProposalStatus                        string      `json:"proposal_status"`
	ApprovalID                            string      `json:"approval_id"`
	ExecutionID                           string      `json:"execution_id"`
	ExecutionStatus                       string      `json:"execution_status"`
	ExecutionExecuted                     bool        `json:"execution_executed"`
	ExecutionResultID                     string      `json:"execution_result_id"`
	ProposalText                          string      `json:"proposal_text"`
	RequiresApproval                      bool        `json:"requires_approval"`
	CitationCount                         int         `json:"citation_count"`
	PolicyAllowed                         bool        `json:"policy_allowed"`
	PolicyRequiresApproval                bool        `json:"policy_requires_approval"`
	PackID                                string      `json:"pack_id"`
	EvidenceItemCount                     int         `json:"evidence_item_count"`
	SearchItemCount                       int         `json:"search_item_count"`
	MemoryItemCount                       int         `json:"memory_item_count"`
	ProfileItemCount                      int         `json:"profile_item_count"`
	CrossGroupSourceRefsPreserved         bool        `json:"cross_group_source_refs_preserved"`
	CrossGroupSpeakerAttributionPreserved bool        `json:"cross_group_speaker_attribution_preserved"`
	MemoryGraphEdgesPreserved             bool        `json:"memory_graph_edges_preserved"`
	ProfileAggregatePreserved             bool        `json:"profile_aggregate_preserved"`
	AgentVersion                          string      `json:"agent_version"`
	RetrievalVersion                      string      `json:"retrieval_version"`
	Verified                              []string    `json:"verified"`
	StartedAt                             time.Time   `json:"started_at"`
	FinishedAt                            time.Time   `json:"finished_at"`
}

type seedSummary struct {
	TenantID                 string `json:"tenant_id"`
	ConversationID           string `json:"conversation_id"`
	CrossGroupConversationID string `json:"cross_group_conversation_id,omitempty"`
	ViewerUserID             string `json:"viewer_user_id"`
	SenderUserID             string `json:"sender_user_id"`
	MemoryEventID            string `json:"memory_event_id"`
	ProfileID                string `json:"profile_id,omitempty"`
	MemoryGraphEdgeID        string `json:"memory_graph_edge_id,omitempty"`
	ConversationSeq          int64  `json:"conversation_seq"`
	CurrentMemoryAtSeq       int64  `json:"current_memory_at_seq"`
}

type combinedSummary struct {
	RunName                               string    `json:"run_name"`
	ResultDir                             string    `json:"result_dir"`
	RAGSummaryPath                        string    `json:"rag_summary_path"`
	AgentSummaryPath                      string    `json:"agent_summary_path"`
	TenantID                              string    `json:"tenant_id"`
	ConversationID                        string    `json:"conversation_id"`
	ViewerUserID                          string    `json:"viewer_user_id"`
	RAGAnswered                           bool      `json:"rag_answered"`
	RAGAnswerID                           string    `json:"rag_answer_id"`
	RAGAnswerStatus                       string    `json:"rag_answer_status"`
	RAGAnswerTextSHA256                   string    `json:"rag_answer_text_sha256"`
	RAGCitationCount                      int       `json:"rag_citation_count"`
	RAGEvidenceItemCount                  int       `json:"rag_evidence_item_count"`
	AgentProposalCreated                  bool      `json:"agent_proposal_created"`
	AgentProposalID                       string    `json:"agent_proposal_id"`
	AgentProposalStatus                   string    `json:"agent_proposal_status"`
	AgentProposalTextSHA256               string    `json:"agent_proposal_text_sha256"`
	AgentRequiresApproval                 bool      `json:"agent_requires_approval"`
	AgentApprovalRecorded                 bool      `json:"agent_approval_recorded"`
	AgentApprovalID                       string    `json:"agent_approval_id"`
	ActionExecutionRecorded               bool      `json:"action_execution_recorded"`
	ActionExecutionID                     string    `json:"action_execution_id"`
	ActionExecutionStatus                 string    `json:"action_execution_status"`
	ActionExecuted                        bool      `json:"action_executed"`
	ActionResultRecorded                  bool      `json:"action_result_recorded"`
	SharedTenantAndConversation           bool      `json:"shared_tenant_and_conversation"`
	CrossGroupSourceRefsPreserved         bool      `json:"cross_group_source_refs_preserved"`
	CrossGroupSpeakerAttributionPreserved bool      `json:"cross_group_speaker_attribution_preserved"`
	MemoryGraphEdgesPreserved             bool      `json:"memory_graph_edges_preserved"`
	ProfileAggregatePreserved             bool      `json:"profile_aggregate_preserved"`
	RAGVersion                            string    `json:"rag_version"`
	AgentVersion                          string    `json:"agent_version"`
	RetrievalVersions                     []string  `json:"retrieval_versions"`
	Verified                              []string  `json:"verified"`
	StartedAt                             time.Time `json:"started_at"`
	FinishedAt                            time.Time `json:"finished_at"`
}

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "rag-agent demo failed: %v\n", err)
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
	startedAt := time.Now().UTC()

	ragRunName := cfg.runName + "-rag"
	agentRunName := cfg.runName + "-agent"
	ragSummaryPath := filepath.Join(cfg.resultRoot, sanitizeRunName(ragRunName), "rag-answer-summary.json")
	agentSummaryPath := filepath.Join(cfg.resultRoot, sanitizeRunName(agentRunName), "agent-proposal-summary.json")

	if err := runChild(ctx, "rag", ragArgs(cfg, ragRunName)); err != nil {
		return err
	}
	if err := runChild(ctx, "agent", agentArgs(cfg, agentRunName)); err != nil {
		return err
	}

	ragSummary, err := readJSON[ragPartialSummary](ragSummaryPath)
	if err != nil {
		return err
	}
	agentSummary, err := readJSON[agentPartialSummary](agentSummaryPath)
	if err != nil {
		return err
	}
	combined, err := verifyCombined(cfg, resultDir, ragSummaryPath, agentSummaryPath, ragSummary, agentSummary, startedAt)
	if err != nil {
		return err
	}
	return writeSummary(resultDir, combined)
}

func parseConfig(args []string) (config, error) {
	cfg := config{}
	flagSet := flag.NewFlagSet("ragagent-demo", flag.ContinueOnError)
	flagSet.StringVar(&cfg.pgDSN, "pg-dsn", defaultPGDSN, "PostgreSQL DSN")
	flagSet.StringVar(&cfg.ragTarget, "rag-target", defaultRAGTarget, "rag-service gRPC address")
	flagSet.StringVar(&cfg.agentTarget, "agent-target", defaultAgentTarget, "agent-service gRPC address")
	flagSet.StringVar(&cfg.actionTarget, "action-executor-target", defaultActionTarget, "action-executor gRPC address")
	flagSet.StringVar(&cfg.resultRoot, "result-root", defaultResultRoot, "external result root for raw demo output")
	flagSet.StringVar(&cfg.runName, "run-name", "", "run name under result root")
	flagSet.StringVar(&cfg.tenantID, "tenant-id", "", "tenant id shared by RAG and Agent child runs")
	flagSet.StringVar(&cfg.conversationID, "conversation-id", "", "conversation id shared by RAG and Agent child runs")
	flagSet.StringVar(&cfg.viewerUserID, "viewer-user-id", "ragagent-viewer", "viewer user id")
	flagSet.StringVar(&cfg.senderUserID, "sender-user-id", "ragagent-sender", "sender user id")
	flagSet.StringVar(&cfg.deviceID, "device-id", "ragagent-device", "viewer device id")
	flagSet.StringVar(&cfg.question, "question", defaultQuestion, "question sent to rag-service")
	flagSet.StringVar(&cfg.objective, "objective", defaultObjective, "objective sent to agent-service")
	flagSet.BoolVar(&cfg.expectExecuted, "expect-executed", false, "expect action-executor to run the safe local tool in the Agent child run")
	flagSet.DurationVar(&cfg.requestTimeout, "request-timeout", 10*time.Second, "child gRPC request timeout")
	flagSet.StringVar(&cfg.ragTLS.caFile, "rag-tls-ca-file", "", "rag gRPC TLS CA file")
	flagSet.StringVar(&cfg.ragTLS.serverName, "rag-tls-server-name", "", "rag gRPC TLS server name")
	flagSet.StringVar(&cfg.ragTLS.clientCertFile, "rag-tls-client-cert-file", "", "rag gRPC client certificate")
	flagSet.StringVar(&cfg.ragTLS.clientKeyFile, "rag-tls-client-key-file", "", "rag gRPC client key")
	flagSet.StringVar(&cfg.agentTLS.caFile, "agent-tls-ca-file", "", "agent gRPC TLS CA file")
	flagSet.StringVar(&cfg.agentTLS.serverName, "agent-tls-server-name", "", "agent gRPC TLS server name")
	flagSet.StringVar(&cfg.agentTLS.clientCertFile, "agent-tls-client-cert-file", "", "agent gRPC client certificate")
	flagSet.StringVar(&cfg.agentTLS.clientKeyFile, "agent-tls-client-key-file", "", "agent gRPC client key")
	if err := flagSet.Parse(args); err != nil {
		return config{}, err
	}
	cfg.pgDSN = strings.TrimSpace(cfg.pgDSN)
	cfg.ragTarget = strings.TrimSpace(cfg.ragTarget)
	cfg.agentTarget = strings.TrimSpace(cfg.agentTarget)
	cfg.actionTarget = strings.TrimSpace(cfg.actionTarget)
	cfg.resultRoot = strings.TrimSpace(cfg.resultRoot)
	cfg.question = strings.TrimSpace(cfg.question)
	cfg.objective = strings.TrimSpace(cfg.objective)
	if cfg.pgDSN == "" {
		return config{}, errors.New("--pg-dsn is required")
	}
	if cfg.ragTarget == "" {
		return config{}, errors.New("--rag-target is required")
	}
	if cfg.agentTarget == "" {
		return config{}, errors.New("--agent-target is required")
	}
	if cfg.actionTarget == "" {
		return config{}, errors.New("--action-executor-target is required")
	}
	if cfg.resultRoot == "" {
		return config{}, errors.New("--result-root is required")
	}
	if cfg.question == "" {
		return config{}, errors.New("--question is required")
	}
	if cfg.objective == "" {
		return config{}, errors.New("--objective is required")
	}
	if cfg.runName == "" {
		cfg.runName = "rag-agent-demo-" + time.Now().UTC().Format("20060102-150405")
	}
	suffix, err := randomSuffix()
	if err != nil {
		return config{}, err
	}
	if strings.TrimSpace(cfg.tenantID) == "" {
		cfg.tenantID = "tenant-ragagent-demo-" + suffix
	}
	if strings.TrimSpace(cfg.conversationID) == "" {
		cfg.conversationID = "conv-ragagent-demo-" + suffix
	}
	return cfg, nil
}

func ragArgs(cfg config, runName string) []string {
	args := []string{
		"run", "./loadtest/rag",
		"--pg-dsn", cfg.pgDSN,
		"--rag-target", cfg.ragTarget,
		"--result-root", cfg.resultRoot,
		"--run-name", runName,
		"--tenant-id", cfg.tenantID,
		"--conversation-id", cfg.conversationID,
		"--viewer-user-id", cfg.viewerUserID,
		"--sender-user-id", cfg.senderUserID,
		"--device-id", cfg.deviceID,
		"--question", cfg.question,
		"--request-timeout", cfg.requestTimeout.String(),
	}
	return appendTLSArgs(args, "rag", cfg.ragTLS)
}

func agentArgs(cfg config, runName string) []string {
	args := []string{
		"run", "./loadtest/agent",
		"--pg-dsn", cfg.pgDSN,
		"--agent-target", cfg.agentTarget,
		"--action-executor-target", cfg.actionTarget,
		"--result-root", cfg.resultRoot,
		"--run-name", runName,
		"--tenant-id", cfg.tenantID,
		"--conversation-id", cfg.conversationID,
		"--viewer-user-id", cfg.viewerUserID,
		"--sender-user-id", cfg.senderUserID,
		"--device-id", cfg.deviceID,
		"--objective", cfg.objective,
		"--request-timeout", cfg.requestTimeout.String(),
	}
	if cfg.expectExecuted {
		args = append(args,
			"--expect-executed",
			"--tool-name", "nexusim.local.echo",
			"--skill-id", "nexusim.local.echo",
			"--resource-type", "diagnostic",
		)
	}
	return appendTLSArgs(args, "agent", cfg.agentTLS)
}

func appendTLSArgs(args []string, prefix string, tls tlsFlags) []string {
	if strings.TrimSpace(tls.caFile) != "" {
		args = append(args, "--"+prefix+"-tls-ca-file", tls.caFile)
	}
	if strings.TrimSpace(tls.serverName) != "" {
		args = append(args, "--"+prefix+"-tls-server-name", tls.serverName)
	}
	if strings.TrimSpace(tls.clientCertFile) != "" {
		args = append(args, "--"+prefix+"-tls-client-cert-file", tls.clientCertFile)
	}
	if strings.TrimSpace(tls.clientKeyFile) != "" {
		args = append(args, "--"+prefix+"-tls-client-key-file", tls.clientKeyFile)
	}
	return args
}

func runChild(ctx context.Context, name string, args []string) error {
	repo := gitOutput("rev-parse", "--show-toplevel")
	if repo == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		repo = cwd
	}
	childCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(childCtx, "go", args...)
	cmd.Dir = repo
	if output, err := cmd.CombinedOutput(); err != nil {
		if len(output) != 0 {
			return fmt.Errorf("%s child run failed: %w; rerun child directly for details", name, err)
		}
		return fmt.Errorf("%s child run failed: %w", name, err)
	}
	return nil
}

func readJSON[T any](path string) (T, error) {
	var value T
	encoded, err := os.ReadFile(path)
	if err != nil {
		return value, fmt.Errorf("read summary %s: %w", path, err)
	}
	if err := json.Unmarshal(encoded, &value); err != nil {
		return value, fmt.Errorf("decode summary %s: %w", path, err)
	}
	return value, nil
}

func verifyCombined(
	cfg config,
	resultDir string,
	ragSummaryPath string,
	agentSummaryPath string,
	rag ragPartialSummary,
	agent agentPartialSummary,
	startedAt time.Time,
) (combinedSummary, error) {
	verified := make([]string, 0, 10)
	if rag.AnswerStatus != "GROUNDED" {
		return combinedSummary{}, fmt.Errorf("rag answer status %q, want GROUNDED", rag.AnswerStatus)
	}
	if strings.TrimSpace(rag.AnswerID) == "" || rag.CitationCount <= 0 || rag.EvidenceItemCount <= 0 {
		return combinedSummary{}, errors.New("rag answer missing answer id, citations or evidence")
	}
	verified = append(verified, "RAG returned grounded answer with citations and EvidencePack")

	if agent.ProposalStatus != "APPROVED" {
		return combinedSummary{}, fmt.Errorf("agent proposal status %q, want APPROVED", agent.ProposalStatus)
	}
	if strings.TrimSpace(agent.ProposalID) == "" || strings.TrimSpace(agent.PreparedAuditID) == "" {
		return combinedSummary{}, errors.New("agent proposal missing proposal id or prepared audit id")
	}
	if !agent.RequiresApproval || strings.TrimSpace(agent.ApprovalID) == "" {
		return combinedSummary{}, errors.New("agent approval was not recorded")
	}
	if strings.TrimSpace(agent.ExecutionID) == "" || agent.ExecutionStatus != "RECORDED" {
		return combinedSummary{}, fmt.Errorf("action execution status %q, want RECORDED with execution id", agent.ExecutionStatus)
	}
	if cfg.expectExecuted && !agent.ExecutionExecuted {
		return combinedSummary{}, errors.New("expected safe local action execution but child report says execution did not run")
	}
	verified = append(verified, "Agent proposal, approval and action-executor audit were recorded")

	shared := rag.Seed.TenantID == agent.Seed.TenantID &&
		rag.Seed.ConversationID == agent.Seed.ConversationID &&
		rag.Seed.ViewerUserID == agent.Seed.ViewerUserID
	if !shared {
		return combinedSummary{}, errors.New("rag and agent child runs do not share tenant, conversation and viewer")
	}
	verified = append(verified, "RAG and Agent child runs used the same tenant, conversation and viewer")

	crossGroupRefs := rag.CrossGroupSourceRefsPreserved && agent.CrossGroupSourceRefsPreserved
	speaker := rag.CrossGroupSpeakerAttributionPreserved && agent.CrossGroupSpeakerAttributionPreserved
	graph := rag.MemoryGraphEdgesPreserved && agent.MemoryGraphEdgesPreserved
	profile := rag.ProfileAggregatePreserved && agent.ProfileAggregatePreserved
	if !crossGroupRefs || !speaker || !graph || !profile {
		return combinedSummary{}, errors.New("RAG and Agent did not both preserve cross-group refs, speaker attribution, graph edges and profile evidence")
	}
	verified = append(verified, "EvidencePack cross-group refs, speaker attribution, graph edges and profile evidence were preserved in both paths")

	return combinedSummary{
		RunName:                               cfg.runName,
		ResultDir:                             resultDir,
		RAGSummaryPath:                        ragSummaryPath,
		AgentSummaryPath:                      agentSummaryPath,
		TenantID:                              rag.Seed.TenantID,
		ConversationID:                        rag.Seed.ConversationID,
		ViewerUserID:                          rag.Seed.ViewerUserID,
		RAGAnswered:                           true,
		RAGAnswerID:                           rag.AnswerID,
		RAGAnswerStatus:                       rag.AnswerStatus,
		RAGAnswerTextSHA256:                   sha256Hex(rag.AnswerText),
		RAGCitationCount:                      rag.CitationCount,
		RAGEvidenceItemCount:                  rag.EvidenceItemCount,
		AgentProposalCreated:                  true,
		AgentProposalID:                       agent.ProposalID,
		AgentProposalStatus:                   agent.ProposalStatus,
		AgentProposalTextSHA256:               sha256Hex(agent.ProposalText),
		AgentRequiresApproval:                 agent.RequiresApproval,
		AgentApprovalRecorded:                 true,
		AgentApprovalID:                       agent.ApprovalID,
		ActionExecutionRecorded:               true,
		ActionExecutionID:                     agent.ExecutionID,
		ActionExecutionStatus:                 agent.ExecutionStatus,
		ActionExecuted:                        agent.ExecutionExecuted,
		ActionResultRecorded:                  strings.TrimSpace(agent.ExecutionResultID) != "",
		SharedTenantAndConversation:           shared,
		CrossGroupSourceRefsPreserved:         crossGroupRefs,
		CrossGroupSpeakerAttributionPreserved: speaker,
		MemoryGraphEdgesPreserved:             graph,
		ProfileAggregatePreserved:             profile,
		RAGVersion:                            rag.RAGVersion,
		AgentVersion:                          agent.AgentVersion,
		RetrievalVersions:                     uniqueNonEmpty(rag.RetrievalVersion, agent.RetrievalVersion),
		Verified:                              verified,
		StartedAt:                             startedAt,
		FinishedAt:                            time.Now().UTC(),
	}, nil
}

func writeSummary(resultDir string, result combinedSummary) error {
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(resultDir, "rag-agent-demo-summary.json")
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
		return "rag-agent-demo"
	}
	var builder strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '-' || r == '_' || r == '.':
			builder.WriteRune(r)
		default:
			builder.WriteByte('-')
		}
	}
	cleaned := strings.Trim(builder.String(), "-_.")
	if cleaned == "" {
		return "rag-agent-demo"
	}
	return cleaned
}

func randomSuffix() (string, error) {
	var bytes [4]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", fmt.Errorf("generate random run suffix: %w", err)
	}
	return hex.EncodeToString(bytes[:]), nil
}

func gitOutput(args ...string) string {
	cmd := exec.Command("git", args...)
	encoded, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(encoded))
}

func sha256Hex(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func uniqueNonEmpty(values ...string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
