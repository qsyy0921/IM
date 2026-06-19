package app

import (
	"context"
	"errors"
	"testing"

	"github.com/qsyy0921/IM/internal/ai/pythonworker"
	"github.com/qsyy0921/IM/services/rag-service/internal/types"
)

func TestPythonWorkerAnswerProviderReturnsGroundedAnswerAfterCandidateValidation(t *testing.T) {
	base := staticAnswerProvider{generation: groundedGeneration()}
	runner := &fakePythonCandidateRunner{}

	result, err := NewPythonWorkerAnswerProvider(base, runner).GenerateAnswer(
		context.Background(),
		types.AnswerGenerationRequest{Question: "launch decision", EvidencePack: types.EvidencePack{PackID: "pack_01"}},
	)
	if err != nil {
		t.Fatalf("GenerateAnswer() error = %v", err)
	}
	if result.AnswerText == "" || result.GeneratedByLLM {
		t.Fatalf("unexpected result: %+v", result)
	}
	if runner.request.WorkerKind != "LLM" || runner.request.OutputType != "TEXT_CANDIDATE" {
		t.Fatalf("unexpected worker request: %+v", runner.request)
	}
	if runner.request.CandidateText != result.AnswerText {
		t.Fatal("worker request must receive candidate answer text for hashing")
	}
}

func TestPythonWorkerAnswerProviderSkipsInsufficientEvidence(t *testing.T) {
	base := staticAnswerProvider{generation: types.AnswerGenerationResult{
		Status:     types.AnswerStatusInsufficientEvidence,
		AnswerText: "I do not have enough visible evidence to answer this question.",
	}}
	runner := &fakePythonCandidateRunner{}

	result, err := NewPythonWorkerAnswerProvider(base, runner).GenerateAnswer(
		context.Background(),
		types.AnswerGenerationRequest{},
	)
	if err != nil {
		t.Fatalf("GenerateAnswer() error = %v", err)
	}
	if result.Status != types.AnswerStatusInsufficientEvidence {
		t.Fatalf("unexpected status %q", result.Status)
	}
	if runner.called {
		t.Fatal("python worker should not be called for insufficient evidence")
	}
}

func TestPythonWorkerAnswerProviderFailsClosedOnWorkerFailure(t *testing.T) {
	base := staticAnswerProvider{generation: groundedGeneration()}
	runner := &fakePythonCandidateRunner{err: pythonworker.ErrWorkerFailed}

	_, err := NewPythonWorkerAnswerProvider(base, runner).GenerateAnswer(
		context.Background(),
		types.AnswerGenerationRequest{},
	)
	if !errors.Is(err, types.ErrRAGUnavailable) {
		t.Fatalf("GenerateAnswer() error = %v, want ErrRAGUnavailable", err)
	}
}

func TestPythonWorkerAnswerProviderFailsClosedOnHashMismatch(t *testing.T) {
	base := staticAnswerProvider{generation: groundedGeneration()}
	runner := &fakePythonCandidateRunner{candidate: pythonworker.Candidate{
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

	_, err := NewPythonWorkerAnswerProvider(base, runner).GenerateAnswer(
		context.Background(),
		types.AnswerGenerationRequest{},
	)
	if !errors.Is(err, types.ErrRAGUnavailable) {
		t.Fatalf("GenerateAnswer() error = %v, want ErrRAGUnavailable", err)
	}
}

func groundedGeneration() types.AnswerGenerationResult {
	return types.AnswerGenerationResult{
		Status:     types.AnswerStatusGrounded,
		AnswerText: "Grounded extractive answer: [1] ship source-backed memory.",
		Confidence: 0.8,
		Citations:  []types.Citation{{EvidenceID: "evidence_01"}},
	}
}

type staticAnswerProvider struct {
	generation types.AnswerGenerationResult
	err        error
}

func (provider staticAnswerProvider) GenerateAnswer(
	context.Context,
	types.AnswerGenerationRequest,
) (types.AnswerGenerationResult, error) {
	if provider.err != nil {
		return types.AnswerGenerationResult{}, provider.err
	}
	return provider.generation, nil
}

type fakePythonCandidateRunner struct {
	called    bool
	request   pythonworker.Request
	candidate pythonworker.Candidate
	err       error
}

func (runner *fakePythonCandidateRunner) Run(
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
		OutputSHA256:  sha256Hex(request.CandidateText),
		SourceRefs:    request.SourceRefs,
		Citations:     request.Citations,
		SafetyFlags:   []string{"LOW_SENSITIVE"},
		Confidence:    request.Confidence,
	}, nil
}
