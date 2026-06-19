package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/qsyy0921/IM/internal/ai/pythonworker"
	"github.com/qsyy0921/IM/services/agent-service/internal/app"
	"github.com/qsyy0921/IM/services/agent-service/internal/types"
)

func main() {
	var python string
	var scriptPath string
	var workDir string
	flag.StringVar(&python, "python", "python", "Python executable")
	flag.StringVar(&scriptPath, "script", "ai/python/scripts/run_candidate_worker.py", "Python worker script path")
	flag.StringVar(&workDir, "workdir", ".", "Repository working directory")
	flag.Parse()

	runner, err := pythonworker.NewRunner(pythonworker.RunnerOptions{
		Python:     python,
		ScriptPath: scriptPath,
		WorkDir:    workDir,
		Timeout:    5 * time.Second,
	})
	if err != nil {
		fail(err)
	}
	provider := app.NewPythonWorkerProposalProvider(app.ExtractiveProposalProvider{}, runner)
	result, err := provider.GenerateProposal(context.Background(), types.AgentProposalGenerationRequest{
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
	})
	if err != nil {
		fail(err)
	}
	if result.ProposalText == "" || len(result.Citations) == 0 || result.GeneratedByLLM {
		fail(fmt.Errorf("unexpected proposal generation result: %+v", result))
	}
	summary := map[string]any{
		"schema_version":    1,
		"smoke":             "agent-python-worker-provider",
		"status":            "passed",
		"proposal_present":  true,
		"generated_by_llm":  result.GeneratedByLLM,
		"citation_count":    len(result.Citations),
		"raw_output_stored": false,
		"business_write":    false,
		"external_provider": false,
		"scope":             "local agent-service provider smoke; Go owns final proposal and Python returns candidate metadata only",
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(summary); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "agent-python-worker-provider smoke failed: %v\n", err)
	os.Exit(1)
}
