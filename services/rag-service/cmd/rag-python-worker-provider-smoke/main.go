package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/qsyy0921/IM/internal/ai/pythonworker"
	"github.com/qsyy0921/IM/services/rag-service/internal/app"
	"github.com/qsyy0921/IM/services/rag-service/internal/types"
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
	provider := app.NewPythonWorkerAnswerProvider(app.ExtractiveAnswerProvider{}, runner)
	result, err := provider.GenerateAnswer(context.Background(), types.AnswerGenerationRequest{
		Question: "What is the approved memory direction?",
		EvidencePack: types.EvidencePack{
			PackID: "rag-python-worker-provider-smoke-pack",
			Items: []types.EvidenceItem{{
				EvidenceID:      "evidence_01",
				SourceType:      types.EvidenceSourceMemoryEvent,
				SourceID:        "memory_event_01",
				ConversationID:  "conv_01",
				ConversationSeq: 7,
				Text:            "Use source-backed group memory with evidence citations.",
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
	if result.Status != types.AnswerStatusGrounded || result.AnswerText == "" || len(result.Citations) == 0 {
		fail(fmt.Errorf("unexpected RAG generation result: %+v", result))
	}
	summary := map[string]any{
		"schema_version":    1,
		"smoke":             "rag-python-worker-provider",
		"status":            "passed",
		"answer_status":     result.Status,
		"generated_by_llm":  result.GeneratedByLLM,
		"citation_count":    len(result.Citations),
		"raw_output_stored": false,
		"business_write":    false,
		"external_provider": false,
		"scope":             "local rag-service provider smoke; Go owns final answer and Python returns candidate metadata only",
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(summary); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "rag-python-worker-provider smoke failed: %v\n", err)
	os.Exit(1)
}
