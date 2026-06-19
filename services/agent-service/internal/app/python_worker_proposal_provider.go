package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/qsyy0921/IM/internal/ai/pythonworker"
	"github.com/qsyy0921/IM/services/agent-service/internal/types"
)

type PythonProposalCandidateRunner interface {
	Run(context.Context, pythonworker.Request) (pythonworker.Candidate, error)
}

type PythonWorkerProposalProvider struct {
	base   ProposalProvider
	runner PythonProposalCandidateRunner
}

func NewPythonWorkerProposalProvider(
	base ProposalProvider,
	runner PythonProposalCandidateRunner,
) PythonWorkerProposalProvider {
	if base == nil {
		base = ExtractiveProposalProvider{}
	}
	return PythonWorkerProposalProvider{base: base, runner: runner}
}

func (provider PythonWorkerProposalProvider) GenerateProposal(
	ctx context.Context,
	request types.AgentProposalGenerationRequest,
) (types.AgentProposalGenerationResult, error) {
	if provider.runner == nil {
		return types.AgentProposalGenerationResult{}, types.ErrAgentUnavailable
	}
	generation, err := provider.base.GenerateProposal(ctx, request)
	if err != nil {
		return types.AgentProposalGenerationResult{}, err
	}
	if strings.TrimSpace(generation.ProposalText) == "" {
		return generation, nil
	}
	candidateRequest := pythonworker.Request{
		TaskID:        agentPythonWorkerTaskID(request),
		CandidateID:   agentPythonWorkerCandidateID(generation.ProposalText),
		WorkerKind:    "PLANNER",
		OutputType:    "PLAN_CANDIDATE",
		CandidateText: generation.ProposalText,
		SourceRefs:    agentProposalCitationEvidenceIDs(generation.Citations),
		Citations:     agentProposalCitationEvidenceIDs(generation.Citations),
	}
	candidate, err := provider.runner.Run(ctx, candidateRequest)
	if err != nil {
		return types.AgentProposalGenerationResult{}, fmt.Errorf("%w: python worker candidate rejected", types.ErrAgentUnavailable)
	}
	if err := verifyPythonProposalCandidate(candidate, candidateRequest, generation); err != nil {
		return types.AgentProposalGenerationResult{}, err
	}
	return generation, nil
}

func verifyPythonProposalCandidate(
	candidate pythonworker.Candidate,
	request pythonworker.Request,
	generation types.AgentProposalGenerationResult,
) error {
	if candidate.TaskID != request.TaskID || candidate.CandidateID != request.CandidateID {
		return fmt.Errorf("%w: python worker candidate identity mismatch", types.ErrAgentUnavailable)
	}
	if candidate.Status != pythonworker.StatusCandidate ||
		candidate.WorkerKind != "PLANNER" ||
		candidate.OutputType != "PLAN_CANDIDATE" {
		return fmt.Errorf("%w: python worker candidate type mismatch", types.ErrAgentUnavailable)
	}
	if candidate.OutputSHA256 != agentSHA256Hex(generation.ProposalText) {
		return fmt.Errorf("%w: python worker candidate hash mismatch", types.ErrAgentUnavailable)
	}
	allowed := make(map[string]struct{}, len(generation.Citations))
	for _, id := range agentProposalCitationEvidenceIDs(generation.Citations) {
		allowed[id] = struct{}{}
	}
	if len(candidate.Citations) == 0 {
		return fmt.Errorf("%w: python worker candidate citation missing", types.ErrAgentUnavailable)
	}
	for _, citation := range candidate.Citations {
		if _, ok := allowed[citation]; !ok {
			return fmt.Errorf("%w: python worker candidate citation mismatch", types.ErrAgentUnavailable)
		}
	}
	return nil
}

func agentProposalCitationEvidenceIDs(citations []types.Citation) []string {
	ids := make([]string, 0, len(citations))
	seen := make(map[string]struct{}, len(citations))
	for _, citation := range citations {
		id := strings.TrimSpace(citation.EvidenceID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids
}

func agentPythonWorkerTaskID(request types.AgentProposalGenerationRequest) string {
	key := request.Objective + "|" + request.ToolName + "|" + request.ResourceType + "|" + request.EvidencePack.PackID
	return "agent_task_" + agentSHA256Hex(key)[:16]
}

func agentPythonWorkerCandidateID(proposal string) string {
	return "agent_cand_" + agentSHA256Hex(proposal)[:16]
}

func agentSHA256Hex(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
