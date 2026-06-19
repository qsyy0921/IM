package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/qsyy0921/IM/internal/ai/pythonworker"
	"github.com/qsyy0921/IM/services/summary-service/internal/types"
)

type PythonCandidateRunner interface {
	Run(context.Context, pythonworker.Request) (pythonworker.Candidate, error)
}

type PythonWorkerSummaryProvider struct {
	base   SummaryProvider
	runner PythonCandidateRunner
}

func NewPythonWorkerSummaryProvider(base SummaryProvider, runner PythonCandidateRunner) PythonWorkerSummaryProvider {
	if base == nil {
		base = ExtractiveSummaryProvider{}
	}
	return PythonWorkerSummaryProvider{base: base, runner: runner}
}

func (provider PythonWorkerSummaryProvider) GenerateSummary(
	ctx context.Context,
	request types.SummaryGenerationRequest,
) (types.SummaryGenerationResult, error) {
	if provider.runner == nil {
		return types.SummaryGenerationResult{}, types.ErrSummaryUnavailable
	}
	generation, err := provider.base.GenerateSummary(ctx, request)
	if err != nil {
		return types.SummaryGenerationResult{}, err
	}
	if generation.Status != types.SummaryStatusGrounded || strings.TrimSpace(generation.SummaryText) == "" {
		return generation, nil
	}
	candidateRequest := pythonworker.Request{
		TaskID:        summaryPythonWorkerTaskID(request),
		CandidateID:   summaryPythonWorkerCandidateID(generation.SummaryText),
		WorkerKind:    "LLM",
		OutputType:    "TEXT_CANDIDATE",
		CandidateText: generation.SummaryText,
		SourceRefs:    summaryCitationEvidenceIDs(generation.Citations),
		Citations:     summaryCitationEvidenceIDs(generation.Citations),
		Confidence:    &generation.Confidence,
	}
	candidate, err := provider.runner.Run(ctx, candidateRequest)
	if err != nil {
		return types.SummaryGenerationResult{}, fmt.Errorf("%w: python worker candidate rejected", types.ErrSummaryUnavailable)
	}
	if err := verifyPythonSummaryCandidate(candidate, candidateRequest, generation); err != nil {
		return types.SummaryGenerationResult{}, err
	}
	return generation, nil
}

func verifyPythonSummaryCandidate(
	candidate pythonworker.Candidate,
	request pythonworker.Request,
	generation types.SummaryGenerationResult,
) error {
	if candidate.TaskID != request.TaskID || candidate.CandidateID != request.CandidateID {
		return fmt.Errorf("%w: python worker candidate identity mismatch", types.ErrSummaryUnavailable)
	}
	if candidate.Status != pythonworker.StatusCandidate ||
		candidate.WorkerKind != "LLM" ||
		candidate.OutputType != "TEXT_CANDIDATE" {
		return fmt.Errorf("%w: python worker candidate type mismatch", types.ErrSummaryUnavailable)
	}
	if candidate.OutputSHA256 != summarySHA256Hex(generation.SummaryText) {
		return fmt.Errorf("%w: python worker candidate hash mismatch", types.ErrSummaryUnavailable)
	}
	allowed := make(map[string]struct{}, len(generation.Citations))
	for _, id := range summaryCitationEvidenceIDs(generation.Citations) {
		allowed[id] = struct{}{}
	}
	if len(candidate.Citations) == 0 {
		return fmt.Errorf("%w: python worker candidate citation missing", types.ErrSummaryUnavailable)
	}
	for _, citation := range candidate.Citations {
		if _, ok := allowed[citation]; !ok {
			return fmt.Errorf("%w: python worker candidate citation mismatch", types.ErrSummaryUnavailable)
		}
	}
	return nil
}

func summaryCitationEvidenceIDs(citations []types.Citation) []string {
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

func summaryPythonWorkerTaskID(request types.SummaryGenerationRequest) string {
	key := request.Focus + "|" + request.EvidencePack.PackID
	return "summary_task_" + summarySHA256Hex(key)[:16]
}

func summaryPythonWorkerCandidateID(summary string) string {
	return "summary_cand_" + summarySHA256Hex(summary)[:16]
}

func summarySHA256Hex(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
