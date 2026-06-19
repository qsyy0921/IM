package app

import (
	"context"
	"errors"
	"testing"

	"github.com/qsyy0921/IM/internal/ai/pythonworker"
	"github.com/qsyy0921/IM/services/summary-service/internal/types"
)

func TestPythonWorkerSummaryProviderReturnsGroundedSummaryAfterCandidateValidation(t *testing.T) {
	base := staticSummaryProvider{generation: groundedSummaryGeneration()}
	runner := &fakePythonSummaryCandidateRunner{}

	result, err := NewPythonWorkerSummaryProvider(base, runner).GenerateSummary(
		context.Background(),
		types.SummaryGenerationRequest{Focus: "release recap", EvidencePack: types.EvidencePack{PackID: "pack_01"}},
	)
	if err != nil {
		t.Fatalf("GenerateSummary() error = %v", err)
	}
	if result.SummaryText == "" || result.GeneratedByLLM {
		t.Fatalf("unexpected result: %+v", result)
	}
	if runner.request.WorkerKind != "LLM" || runner.request.OutputType != "TEXT_CANDIDATE" {
		t.Fatalf("unexpected worker request: %+v", runner.request)
	}
	if runner.request.CandidateText != result.SummaryText {
		t.Fatal("worker request must receive candidate summary text for hashing")
	}
}

func TestPythonWorkerSummaryProviderSkipsInsufficientEvidence(t *testing.T) {
	base := staticSummaryProvider{generation: types.SummaryGenerationResult{
		Status:      types.SummaryStatusInsufficientEvidence,
		SummaryText: "I do not have enough visible evidence to summarize this conversation.",
	}}
	runner := &fakePythonSummaryCandidateRunner{}

	result, err := NewPythonWorkerSummaryProvider(base, runner).GenerateSummary(
		context.Background(),
		types.SummaryGenerationRequest{},
	)
	if err != nil {
		t.Fatalf("GenerateSummary() error = %v", err)
	}
	if result.Status != types.SummaryStatusInsufficientEvidence {
		t.Fatalf("unexpected status %q", result.Status)
	}
	if runner.called {
		t.Fatal("python worker should not be called for insufficient evidence")
	}
}

func TestPythonWorkerSummaryProviderFailsClosedOnWorkerFailure(t *testing.T) {
	base := staticSummaryProvider{generation: groundedSummaryGeneration()}
	runner := &fakePythonSummaryCandidateRunner{err: pythonworker.ErrWorkerFailed}

	_, err := NewPythonWorkerSummaryProvider(base, runner).GenerateSummary(
		context.Background(),
		types.SummaryGenerationRequest{},
	)
	if !errors.Is(err, types.ErrSummaryUnavailable) {
		t.Fatalf("GenerateSummary() error = %v, want ErrSummaryUnavailable", err)
	}
}

func TestPythonWorkerSummaryProviderFailsClosedOnHashMismatch(t *testing.T) {
	base := staticSummaryProvider{generation: groundedSummaryGeneration()}
	runner := &fakePythonSummaryCandidateRunner{candidate: pythonworker.Candidate{
		SchemaVersion: 1,
		TaskID:        "task_01",
		CandidateID:   "cand_01",
		WorkerKind:    "LLM",
		Status:        pythonworker.StatusCandidate,
		OutputType:    "TEXT_CANDIDATE",
		OutputSHA256:  "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Citations:     []string{"evidence_01"},
		SafetyFlags:   []string{"LOW_SENSITIVE"},
	}}

	_, err := NewPythonWorkerSummaryProvider(base, runner).GenerateSummary(
		context.Background(),
		types.SummaryGenerationRequest{},
	)
	if !errors.Is(err, types.ErrSummaryUnavailable) {
		t.Fatalf("GenerateSummary() error = %v, want ErrSummaryUnavailable", err)
	}
}

func groundedSummaryGeneration() types.SummaryGenerationResult {
	return types.SummaryGenerationResult{
		Status:      types.SummaryStatusGrounded,
		SummaryText: "Summary based on visible evidence:\n- Source-backed memory is required.",
		Confidence:  0.8,
		Citations:   []types.Citation{{EvidenceID: "evidence_01"}},
	}
}

type staticSummaryProvider struct {
	generation types.SummaryGenerationResult
	err        error
}

func (provider staticSummaryProvider) GenerateSummary(
	context.Context,
	types.SummaryGenerationRequest,
) (types.SummaryGenerationResult, error) {
	if provider.err != nil {
		return types.SummaryGenerationResult{}, provider.err
	}
	return provider.generation, nil
}

type fakePythonSummaryCandidateRunner struct {
	called    bool
	request   pythonworker.Request
	candidate pythonworker.Candidate
	err       error
}

func (runner *fakePythonSummaryCandidateRunner) Run(
	_ context.Context,
	request pythonworker.Request,
) (pythonworker.Candidate, error) {
	runner.called = true
	runner.request = request
	if runner.err != nil {
		return pythonworker.Candidate{}, runner.err
	}
	if runner.candidate.SchemaVersion != 0 {
		return runner.candidate, nil
	}
	return pythonworker.Candidate{
		SchemaVersion: 1,
		TaskID:        request.TaskID,
		CandidateID:   request.CandidateID,
		WorkerKind:    request.WorkerKind,
		Status:        pythonworker.StatusCandidate,
		OutputType:    request.OutputType,
		OutputSHA256:  summarySHA256Hex(request.CandidateText),
		SourceRefs:    request.SourceRefs,
		Citations:     request.Citations,
		SafetyFlags:   []string{"LOW_SENSITIVE"},
		Confidence:    request.Confidence,
	}, nil
}
