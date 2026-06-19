package app

import (
	"context"
	"errors"
	"testing"

	"github.com/qsyy0921/IM/internal/ai/pythonworker"
	"github.com/qsyy0921/IM/services/agent-service/internal/types"
)

func TestPythonWorkerProposalProviderReturnsProposalAfterCandidateValidation(t *testing.T) {
	base := staticProposalProvider{generation: groundedProposalGeneration()}
	runner := &fakePythonProposalCandidateRunner{}

	result, err := NewPythonWorkerProposalProvider(base, runner).GenerateProposal(
		context.Background(),
		types.AgentProposalGenerationRequest{
			Objective:    "draft action plan",
			ToolName:     "conversation.note.create",
			ResourceType: "conversation",
			EvidencePack: types.EvidencePack{PackID: "pack_01"},
		},
	)
	if err != nil {
		t.Fatalf("GenerateProposal() error = %v", err)
	}
	if result.ProposalText == "" || result.GeneratedByLLM {
		t.Fatalf("unexpected result: %+v", result)
	}
	if runner.request.WorkerKind != "PLANNER" || runner.request.OutputType != "PLAN_CANDIDATE" {
		t.Fatalf("unexpected worker request: %+v", runner.request)
	}
	if runner.request.CandidateText != result.ProposalText {
		t.Fatal("worker request must receive candidate proposal text for hashing")
	}
	if len(runner.request.Citations) != 1 || runner.request.Citations[0] != "evidence_01" {
		t.Fatalf("unexpected worker citations: %+v", runner.request.Citations)
	}
}

func TestPythonWorkerProposalProviderSkipsEmptyProposal(t *testing.T) {
	base := staticProposalProvider{generation: types.AgentProposalGenerationResult{}}
	runner := &fakePythonProposalCandidateRunner{}

	result, err := NewPythonWorkerProposalProvider(base, runner).GenerateProposal(
		context.Background(),
		types.AgentProposalGenerationRequest{},
	)
	if err != nil {
		t.Fatalf("GenerateProposal() error = %v", err)
	}
	if result.ProposalText != "" {
		t.Fatalf("unexpected proposal text %q", result.ProposalText)
	}
	if runner.called {
		t.Fatal("python worker should not be called for empty proposals")
	}
}

func TestPythonWorkerProposalProviderFailsClosedOnWorkerFailure(t *testing.T) {
	base := staticProposalProvider{generation: groundedProposalGeneration()}
	runner := &fakePythonProposalCandidateRunner{err: pythonworker.ErrWorkerFailed}

	_, err := NewPythonWorkerProposalProvider(base, runner).GenerateProposal(
		context.Background(),
		types.AgentProposalGenerationRequest{},
	)
	if !errors.Is(err, types.ErrAgentUnavailable) {
		t.Fatalf("GenerateProposal() error = %v, want ErrAgentUnavailable", err)
	}
}

func TestPythonWorkerProposalProviderFailsClosedOnHashMismatch(t *testing.T) {
	base := staticProposalProvider{generation: groundedProposalGeneration()}
	runner := &fakePythonProposalCandidateRunner{candidate: pythonworker.Candidate{
		SchemaVersion: 1,
		TaskID:        "task_01",
		CandidateID:   "cand_01",
		WorkerKind:    "PLANNER",
		Status:        pythonworker.StatusCandidate,
		OutputType:    "PLAN_CANDIDATE",
		OutputSHA256:  "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Citations:     []string{"evidence_01"},
		SafetyFlags:   []string{"LOW_SENSITIVE"},
	}}

	_, err := NewPythonWorkerProposalProvider(base, runner).GenerateProposal(
		context.Background(),
		types.AgentProposalGenerationRequest{},
	)
	if !errors.Is(err, types.ErrAgentUnavailable) {
		t.Fatalf("GenerateProposal() error = %v, want ErrAgentUnavailable", err)
	}
}

func TestPythonWorkerProposalProviderFailsClosedOnCitationMismatch(t *testing.T) {
	base := staticProposalProvider{generation: groundedProposalGeneration()}
	runner := &fakePythonProposalCandidateRunner{candidate: pythonworker.Candidate{
		SchemaVersion: 1,
		TaskID:        "task_01",
		CandidateID:   "cand_01",
		WorkerKind:    "PLANNER",
		Status:        pythonworker.StatusCandidate,
		OutputType:    "PLAN_CANDIDATE",
		OutputSHA256:  agentSHA256Hex(groundedProposalGeneration().ProposalText),
		Citations:     []string{"fabricated_evidence"},
		SafetyFlags:   []string{"LOW_SENSITIVE"},
	}}

	_, err := NewPythonWorkerProposalProvider(base, runner).GenerateProposal(
		context.Background(),
		types.AgentProposalGenerationRequest{},
	)
	if !errors.Is(err, types.ErrAgentUnavailable) {
		t.Fatalf("GenerateProposal() error = %v, want ErrAgentUnavailable", err)
	}
}

func groundedProposalGeneration() types.AgentProposalGenerationResult {
	return types.AgentProposalGenerationResult{
		ProposalText:   "Agent proposal based on visible evidence: [1] create a low-risk note.",
		Citations:      []types.Citation{{EvidenceID: "evidence_01"}},
		GeneratedByLLM: false,
	}
}

type staticProposalProvider struct {
	generation types.AgentProposalGenerationResult
	err        error
}

func (provider staticProposalProvider) GenerateProposal(
	context.Context,
	types.AgentProposalGenerationRequest,
) (types.AgentProposalGenerationResult, error) {
	if provider.err != nil {
		return types.AgentProposalGenerationResult{}, provider.err
	}
	return provider.generation, nil
}

type fakePythonProposalCandidateRunner struct {
	called    bool
	request   pythonworker.Request
	candidate pythonworker.Candidate
	err       error
}

func (runner *fakePythonProposalCandidateRunner) Run(
	_ context.Context,
	request pythonworker.Request,
) (pythonworker.Candidate, error) {
	runner.called = true
	runner.request = request
	if runner.err != nil {
		return pythonworker.Candidate{}, runner.err
	}
	if runner.candidate.SchemaVersion != 0 {
		runner.candidate.TaskID = request.TaskID
		runner.candidate.CandidateID = request.CandidateID
		return runner.candidate, nil
	}
	return pythonworker.Candidate{
		SchemaVersion: 1,
		TaskID:        request.TaskID,
		CandidateID:   request.CandidateID,
		WorkerKind:    request.WorkerKind,
		Status:        pythonworker.StatusCandidate,
		OutputType:    request.OutputType,
		OutputSHA256:  agentSHA256Hex(request.CandidateText),
		SourceRefs:    request.SourceRefs,
		Citations:     request.Citations,
		SafetyFlags:   []string{"LOW_SENSITIVE"},
		Confidence:    request.Confidence,
	}, nil
}
