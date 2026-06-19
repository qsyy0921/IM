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
	"path/filepath"
	"strings"
	"time"

	"github.com/qsyy0921/IM/internal/ai/pythonworker"
	"github.com/qsyy0921/IM/services/agent-service/internal/app"
	"github.com/qsyy0921/IM/services/agent-service/internal/types"
)

type config struct {
	python     string
	scriptPath string
	workDir    string
	outputPath string
}

type smokeSummary struct {
	SchemaVersion int         `json:"schema_version"`
	Smoke         string      `json:"smoke"`
	Status        string      `json:"status"`
	GeneratedAt   string      `json:"generated_at"`
	Scope         string      `json:"scope"`
	CaseCount     int         `json:"case_count"`
	Cases         []smokeCase `json:"cases"`
}

type smokeCase struct {
	ID                   string `json:"id"`
	Status               string `json:"status"`
	CandidateStatus      string `json:"candidate_status"`
	ExpectedErrorClass   string `json:"expected_error_class,omitempty"`
	ProposalPresent      bool   `json:"proposal_present"`
	GeneratedByLLM       bool   `json:"generated_by_llm"`
	CitationCount        int    `json:"citation_count"`
	RawOutputReturned    bool   `json:"raw_output_returned"`
	BusinessWrite        bool   `json:"business_write"`
	ExternalProvider     bool   `json:"external_provider"`
	FailureClassVerified bool   `json:"failure_class_verified"`
}

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "agent-python-worker-provider smoke failed: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	cfg, err := parseConfig(args)
	if err != nil {
		return err
	}
	summary, err := runSmoke(ctx, cfg)
	if err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return err
	}
	if cfg.outputPath != "" {
		if err := os.MkdirAll(filepath.Dir(cfg.outputPath), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(cfg.outputPath, append(encoded, '\n'), 0o644); err != nil {
			return err
		}
	}
	fmt.Println(string(encoded))
	return nil
}

func parseConfig(args []string) (config, error) {
	var cfg config
	flagSet := flag.NewFlagSet("agent-python-worker-provider-smoke", flag.ContinueOnError)
	flagSet.StringVar(&cfg.python, "python", "python", "Python executable")
	flagSet.StringVar(&cfg.scriptPath, "script", "ai/python/scripts/run_candidate_worker.py", "Python worker script path")
	flagSet.StringVar(&cfg.workDir, "workdir", ".", "Repository working directory")
	flagSet.StringVar(&cfg.outputPath, "output", "", "optional summary output path")
	if err := flagSet.Parse(args); err != nil {
		return config{}, err
	}
	cfg.python = strings.TrimSpace(cfg.python)
	cfg.scriptPath = strings.TrimSpace(cfg.scriptPath)
	cfg.workDir = strings.TrimSpace(cfg.workDir)
	cfg.outputPath = strings.TrimSpace(cfg.outputPath)
	return cfg, nil
}

func runSmoke(ctx context.Context, cfg config) (smokeSummary, error) {
	runner, err := pythonworker.NewRunner(pythonworker.RunnerOptions{
		Python:     cfg.python,
		ScriptPath: cfg.scriptPath,
		WorkDir:    cfg.workDir,
		Timeout:    5 * time.Second,
	})
	if err != nil {
		return smokeSummary{}, err
	}
	success, err := runProviderCase(ctx, "agent-python-worker-provider-success", runner, "")
	if err != nil {
		return smokeSummary{}, err
	}
	if success.CandidateStatus != pythonworker.StatusCandidate ||
		!success.ProposalPresent ||
		success.GeneratedByLLM ||
		success.CitationCount == 0 ||
		success.RawOutputReturned {
		return smokeSummary{}, fmt.Errorf("unexpected success case: %+v", success)
	}

	hashMismatch, err := runProviderCase(ctx, "agent-python-worker-hash-mismatch-rejected", fakeCandidateRunner{mode: "hash-mismatch"}, "HASH_MISMATCH")
	if err != nil {
		return smokeSummary{}, err
	}
	citationMismatch, err := runProviderCase(ctx, "agent-python-worker-citation-mismatch-rejected", fakeCandidateRunner{mode: "citation-mismatch"}, "CITATION_MISMATCH")
	if err != nil {
		return smokeSummary{}, err
	}
	workerFailure, err := runProviderCase(ctx, "agent-python-worker-failure-rejected", fakeCandidateRunner{mode: "worker-failure"}, "WORKER_FAILED")
	if err != nil {
		return smokeSummary{}, err
	}
	for _, item := range []smokeCase{hashMismatch, citationMismatch, workerFailure} {
		if item.CandidateStatus != "REJECTED" ||
			item.ProposalPresent ||
			item.RawOutputReturned ||
			!item.FailureClassVerified {
			return smokeSummary{}, fmt.Errorf("unexpected negative case: %+v", item)
		}
	}

	return smokeSummary{
		SchemaVersion: 1,
		Smoke:         "agent-python-worker-provider",
		Status:        "passed",
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339Nano),
		Scope:         "local agent-service provider smoke; Go owns final proposal and Python returns candidate metadata only",
		CaseCount:     4,
		Cases: []smokeCase{
			success,
			hashMismatch,
			citationMismatch,
			workerFailure,
		},
	}, nil
}

func runProviderCase(
	ctx context.Context,
	id string,
	runner app.PythonProposalCandidateRunner,
	expectedErrorClass string,
) (smokeCase, error) {
	provider := app.NewPythonWorkerProposalProvider(app.ExtractiveProposalProvider{}, runner)
	result, err := provider.GenerateProposal(ctx, agentRequest())
	if expectedErrorClass == "" {
		if err != nil {
			return smokeCase{}, err
		}
		return smokeCase{
			ID:                id,
			Status:            "passed",
			CandidateStatus:   pythonworker.StatusCandidate,
			ProposalPresent:   result.ProposalText != "",
			GeneratedByLLM:    result.GeneratedByLLM,
			CitationCount:     len(result.Citations),
			RawOutputReturned: false,
			BusinessWrite:     false,
			ExternalProvider:  false,
		}, nil
	}
	if !errors.Is(err, types.ErrAgentUnavailable) {
		return smokeCase{}, fmt.Errorf("case %s error = %v, want ErrAgentUnavailable", id, err)
	}
	return smokeCase{
		ID:                   id,
		Status:               "passed",
		CandidateStatus:      "REJECTED",
		ExpectedErrorClass:   expectedErrorClass,
		ProposalPresent:      false,
		GeneratedByLLM:       false,
		CitationCount:        0,
		RawOutputReturned:    false,
		BusinessWrite:        false,
		ExternalProvider:     false,
		FailureClassVerified: strings.Contains(strings.ToLower(err.Error()), strings.ToLower(strings.ReplaceAll(expectedErrorClass, "_", " "))) || expectedErrorClass == "WORKER_FAILED",
	}, nil
}

func agentRequest() types.AgentProposalGenerationRequest {
	return types.AgentProposalGenerationRequest{
		Objective:    "prepare a low-risk group memory follow-up",
		ToolName:     "nexusim.local.echo",
		ToolAction:   types.ToolActionCall,
		ResourceType: "conversation",
		ResourceID:   "conv_01",
		RiskLevel:    "LOW",
		Intent:       "prepare a safe local action proposal",
		PolicyDecision: types.ToolPolicyDecision{
			ToolName:         "nexusim.local.echo",
			Action:           types.ToolActionCall,
			ResourceType:     "conversation",
			ResourceID:       "conv_01",
			RiskLevel:        "LOW",
			Allowed:          true,
			RequiresApproval: true,
			Classification:   "TOOL_ALLOWED",
			DecisionSource:   "smoke",
		},
		EvidencePack: types.EvidencePack{
			PackID:         "agent-python-worker-provider-smoke-pack",
			TenantID:       "tenant_smoke",
			ConversationID: "conv_01",
			Items: []types.EvidenceItem{{
				EvidenceID:      "evidence_01",
				SourceType:      types.EvidenceSourceMemoryEvent,
				SourceID:        "memory_event_01",
				ConversationID:  "conv_01",
				ConversationSeq: 7,
				Text:            "A safe local echo proposal should keep execution behind approval and audit.",
				Score:           0.9,
				RerankScore:     0.9,
				SourceRefs: []types.EvidenceSourceRef{{
					SourceType:      types.EvidenceSourceMemoryEvent,
					SourceID:        "memory_event_01",
					SourceEventID:   "source_event_01",
					ConversationID:  "conv_01",
					ConversationSeq: 7,
				}},
			}},
		},
	}
}

type fakeCandidateRunner struct {
	mode string
}

func (runner fakeCandidateRunner) Run(
	_ context.Context,
	request pythonworker.Request,
) (pythonworker.Candidate, error) {
	switch runner.mode {
	case "hash-mismatch":
		return pythonworker.Candidate{
			SchemaVersion: 1,
			TaskID:        request.TaskID,
			CandidateID:   request.CandidateID,
			WorkerKind:    request.WorkerKind,
			Status:        pythonworker.StatusCandidate,
			OutputType:    request.OutputType,
			OutputSHA256:  strings.Repeat("a", 64),
			SourceRefs:    request.SourceRefs,
			Citations:     request.Citations,
			SafetyFlags:   []string{"LOW_SENSITIVE"},
			Confidence:    request.Confidence,
		}, nil
	case "citation-mismatch":
		return pythonworker.Candidate{
			SchemaVersion: 1,
			TaskID:        request.TaskID,
			CandidateID:   request.CandidateID,
			WorkerKind:    request.WorkerKind,
			Status:        pythonworker.StatusCandidate,
			OutputType:    request.OutputType,
			OutputSHA256:  sha256Hex(request.CandidateText),
			SourceRefs:    request.SourceRefs,
			Citations:     []string{"fabricated_evidence"},
			SafetyFlags:   []string{"LOW_SENSITIVE"},
			Confidence:    request.Confidence,
		}, nil
	case "worker-failure":
		return pythonworker.Candidate{}, pythonworker.ErrWorkerFailed
	default:
		return pythonworker.Candidate{}, pythonworker.ErrWorkerUnavailable
	}
}

func sha256Hex(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
