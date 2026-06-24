package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/qsyy0921/IM/internal/ai/memorycandidate"
)

type caseResult struct {
	CaseID                string   `json:"case_id"`
	Status                string   `json:"status"`
	Stage                 string   `json:"stage"`
	ExpectedErrorClass    string   `json:"expected_error_class,omitempty"`
	ResultStatus          string   `json:"result_status,omitempty"`
	CandidateCount        int      `json:"candidate_count"`
	OrdinaryMessageCount  int      `json:"ordinary_message_count,omitempty"`
	MemoryEventTypes      []string `json:"memory_event_types,omitempty"`
	ProfileReviewRequired bool     `json:"profile_review_required,omitempty"`
	RawTextReturned       bool     `json:"raw_text_returned"`
	FinalMemoryPersisted  bool     `json:"final_memory_persisted"`
	RequiresGoValidation  bool     `json:"requires_go_validation"`
	RejectedBeforeWorker  bool     `json:"rejected_before_worker,omitempty"`
}

func main() {
	var python string
	var scriptPath string
	var workDir string
	var outputPath string
	flag.StringVar(&python, "python", "python", "Python executable")
	flag.StringVar(&scriptPath, "script", "ai/python/scripts/run_memory_extraction_candidate.py", "Memory extraction Python script path")
	flag.StringVar(&workDir, "workdir", ".", "Repository working directory")
	flag.StringVar(&outputPath, "output", "", "Optional summary output path")
	flag.Parse()

	runner, err := memorycandidate.NewRunner(memorycandidate.RunnerOptions{
		Python:     python,
		ScriptPath: scriptPath,
		WorkDir:    workDir,
		Timeout:    5 * time.Second,
	})
	if err != nil {
		fail(err)
	}

	ctx := context.Background()
	cases := []caseResult{
		runExplicitCueCase(ctx, runner),
		runOrdinaryCase(ctx, runner),
		runUnsafeCase(ctx, runner),
	}
	summary := map[string]any{
		"schema_version": 1,
		"smoke":          "memory-extraction-go-adapter",
		"status":         "passed",
		"case_count":     len(cases),
		"scope":          "local Go-side memory extraction candidate adapter smoke; no external provider, no database, no business write",
		"cases":          cases,
	}
	payload, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		fail(err)
	}
	if outputPath != "" {
		if dir := filepath.Dir(outputPath); dir != "." && dir != "" {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				fail(err)
			}
		}
		if err := os.WriteFile(outputPath, append(payload, '\n'), 0o600); err != nil {
			fail(err)
		}
		return
	}
	_, _ = os.Stdout.Write(append(payload, '\n'))
}

func runExplicitCueCase(ctx context.Context, runner memorycandidate.Runner) caseResult {
	result, err := runner.Run(ctx, memorycandidate.Request{
		SchemaVersion:  1,
		TaskID:         "memory-extraction-go-smoke-explicit",
		TenantID:       "tenant-a",
		ConversationID: "conv-alpha",
		Messages: []memorycandidate.Message{
			{
				MessageID:       "msg-1",
				ConversationSeq: 7,
				SpeakerID:       "user-a",
				Text:            "decision: keep memory candidates source-backed",
			},
			{
				MessageID:       "msg-2",
				ConversationSeq: 8,
				SpeakerID:       "user-b",
				Text:            "profile_signal: user-a coordinates launch follow-up",
			},
			{
				MessageID:       "msg-3",
				ConversationSeq: 9,
				SpeakerID:       "user-c",
				Text:            "ordinary chat should not become memory",
			},
		},
	})
	if err != nil {
		fail(fmt.Errorf("explicit cue case failed: %w", err))
	}
	eventTypes := make([]string, 0, len(result.Candidates))
	profileReviewRequired := false
	for _, candidate := range result.Candidates {
		eventTypes = append(eventTypes, candidate.MemoryEventType)
		if candidate.MemoryEventType == "PROFILE_SIGNAL" && candidate.ReviewState == "NEEDS_REVIEW" {
			profileReviewRequired = true
		}
	}
	if result.CandidateCount != 2 || result.OrdinaryMessageCount != 1 || !profileReviewRequired {
		fail(fmt.Errorf("explicit cue case returned unexpected result: %+v", result))
	}
	return resultCase("python-memory-extraction-explicit-cues-hash-only", result, eventTypes, profileReviewRequired)
}

func runOrdinaryCase(ctx context.Context, runner memorycandidate.Runner) caseResult {
	result, err := runner.Run(ctx, memorycandidate.Request{
		SchemaVersion:  1,
		TaskID:         "memory-extraction-go-smoke-ordinary",
		TenantID:       "tenant-a",
		ConversationID: "conv-alpha",
		Messages: []memorycandidate.Message{
			{
				MessageID:       "msg-4",
				ConversationSeq: 10,
				SpeakerID:       "user-a",
				Text:            "ordinary chat should not become memory",
			},
		},
	})
	if err != nil {
		fail(fmt.Errorf("ordinary case failed: %w", err))
	}
	if result.CandidateCount != 0 || result.OrdinaryMessageCount != 1 {
		fail(fmt.Errorf("ordinary case returned unexpected result: %+v", result))
	}
	return resultCase("python-memory-extraction-ordinary-chat-zero-candidates", result, nil, false)
}

func runUnsafeCase(ctx context.Context, runner memorycandidate.Runner) caseResult {
	_, err := runner.Run(ctx, memorycandidate.Request{
		SchemaVersion:  1,
		TaskID:         "memory-extraction-go-smoke-unsafe",
		TenantID:       "tenant-a",
		ConversationID: "conv-alpha",
		Messages: []memorycandidate.Message{
			{
				MessageID:       "msg-5",
				ConversationSeq: 11,
				SpeakerID:       "user-a",
				Text:            "Bearer secret-token-value",
			},
		},
	})
	if !errors.Is(err, memorycandidate.ErrUnsafeRequest) {
		fail(fmt.Errorf("unsafe case error = %v, want ErrUnsafeRequest", err))
	}
	return caseResult{
		CaseID:               "python-memory-extraction-unsafe-input-fails-closed",
		Status:               "passed",
		Stage:                "python-memory-extraction-candidate",
		ExpectedErrorClass:   "UNSAFE_INPUT",
		ResultStatus:         "REJECTED",
		CandidateCount:       0,
		RawTextReturned:      false,
		FinalMemoryPersisted: false,
		RequiresGoValidation: true,
		RejectedBeforeWorker: true,
	}
}

func resultCase(caseID string, result memorycandidate.Result, eventTypes []string, profileReviewRequired bool) caseResult {
	return caseResult{
		CaseID:                caseID,
		Status:                "passed",
		Stage:                 "python-memory-extraction-candidate",
		ResultStatus:          result.Status,
		CandidateCount:        result.CandidateCount,
		OrdinaryMessageCount:  result.OrdinaryMessageCount,
		MemoryEventTypes:      eventTypes,
		ProfileReviewRequired: profileReviewRequired,
		RawTextReturned:       result.Report.RawTextReturned,
		FinalMemoryPersisted:  result.Report.FinalMemoryPersisted,
		RequiresGoValidation:  result.Report.RequiresGoValidation,
	}
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "memory-extraction-go-adapter smoke failed: %v\n", err)
	os.Exit(1)
}
