package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/qsyy0921/IM/internal/ai/pythonworker"
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
	confidence := 0.9
	candidate, err := runner.Run(context.Background(), pythonworker.Request{
		TaskID:        "go-python-worker-smoke-task",
		CandidateID:   "go-python-worker-smoke-candidate",
		WorkerKind:    "MEMORY_EXTRACTION",
		OutputType:    "MEMORY_EVENT_CANDIDATE",
		CandidateText: "decision: Go consumes Python candidate metadata only",
		SourceRefs:    []string{"message:tenant:conversation:seq1"},
		Citations:     []string{"message:tenant:conversation:seq1"},
		Confidence:    &confidence,
	})
	if err != nil {
		fail(err)
	}
	summary := map[string]any{
		"schema_version":        1,
		"smoke":                 "go-python-worker-adapter",
		"status":                "passed",
		"candidate_status":      candidate.Status,
		"worker_kind":           candidate.WorkerKind,
		"output_type":           candidate.OutputType,
		"output_sha256_present": candidate.OutputSHA256 != "",
		"raw_output_returned":   false,
		"source_ref_count":      len(candidate.SourceRefs),
		"citation_count":        len(candidate.Citations),
		"scope":                 "local Go-side Python candidate adapter smoke; no external provider, no database, no business write",
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(summary); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "go-python-worker-adapter smoke failed: %v\n", err)
	os.Exit(1)
}
