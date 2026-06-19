package main

import (
	"encoding/json"
	"strings"
	"testing"

	aievalv1 "github.com/qsyy0921/IM/api/proto/nexusim/aieval/v1"
)

func TestBuildEvalRunFromSummaryMapsPassedCases(t *testing.T) {
	summary := map[string]any{
		"schema_version": float64(1),
		"adapter":        "profile-agent-output-safety",
		"run_name":       "profile-agent-safety-eval-20260619-010203",
		"scope":          "low-sensitive fixture",
		"case_path":      "docs/runbook/ai-eval/retrieval-eval-cases.json",
		"case_count":     float64(2),
		"cases": []any{
			map[string]any{"id": "profile-1", "stage": "memory-profile-safety", "passed": true},
			map[string]any{"id": "agent-1", "stage": "agent-output-safety", "status": "passed"},
		},
	}
	content, err := json.Marshal(summary)
	if err != nil {
		t.Fatal(err)
	}
	run, err := buildEvalRunFromSummary(config{
		summaryPath: "profile-agent-safety-eval-summary.json",
		tenantID:    "tenant-1",
	}, content, summary)
	if err != nil {
		t.Fatalf("build eval run: %v", err)
	}
	if run.GetRunId() != "profile-agent-safety-eval-20260619-010203" {
		t.Fatalf("unexpected run_id %q", run.GetRunId())
	}
	if run.GetSuiteId() != "ai-eval-profile-agent-output-safety" {
		t.Fatalf("unexpected suite_id %q", run.GetSuiteId())
	}
	if run.GetStage() != "memory-profile-safety" {
		t.Fatalf("unexpected stage %q", run.GetStage())
	}
	if run.GetStatus() != aievalv1.EvalRunStatus_EVAL_RUN_STATUS_PASSED {
		t.Fatalf("unexpected status %s", run.GetStatus())
	}
	if run.GetCaseCount() != 2 || run.GetPassedCount() != 2 || run.GetFailedCount() != 0 {
		t.Fatalf("unexpected counts: %+v", run)
	}
	if strings.Contains(run.GetMetadataJson(), "profile-1") || strings.Contains(run.GetMetadataJson(), "agent-1") {
		t.Fatalf("metadata should not include raw case ids: %s", run.GetMetadataJson())
	}
}

func TestBuildEvalRunFromSummaryMapsFailedCase(t *testing.T) {
	summary := map[string]any{
		"adapter":    "python-ai-worker",
		"status":     "failed",
		"case_count": float64(1),
		"cases": []any{
			map[string]any{"case_id": "unsafe-output", "status": "failed"},
		},
	}
	content, err := json.Marshal(summary)
	if err != nil {
		t.Fatal(err)
	}
	run, err := buildEvalRunFromSummary(config{
		summaryPath: "python-worker-eval-summary.json",
		tenantID:    "tenant-1",
		runID:       "override-run",
		suiteID:     "override-suite",
		stage:       "python-worker-output-safety",
	}, content, summary)
	if err != nil {
		t.Fatalf("build eval run: %v", err)
	}
	if run.GetRunId() != "override-run" || run.GetSuiteId() != "override-suite" {
		t.Fatalf("overrides not applied: %+v", run)
	}
	if run.GetStatus() != aievalv1.EvalRunStatus_EVAL_RUN_STATUS_FAILED {
		t.Fatalf("unexpected status %s", run.GetStatus())
	}
	if run.GetCaseCount() != 1 || run.GetFailedCount() != 1 {
		t.Fatalf("unexpected counts: %+v", run)
	}
}
