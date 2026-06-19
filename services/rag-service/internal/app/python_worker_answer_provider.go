package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/qsyy0921/IM/internal/ai/pythonworker"
	"github.com/qsyy0921/IM/services/rag-service/internal/types"
)

type PythonCandidateRunner interface {
	Run(context.Context, pythonworker.Request) (pythonworker.Candidate, error)
}

type PythonWorkerAnswerProvider struct {
	base   AnswerProvider
	runner PythonCandidateRunner
}

func NewPythonWorkerAnswerProvider(base AnswerProvider, runner PythonCandidateRunner) PythonWorkerAnswerProvider {
	if base == nil {
		base = ExtractiveAnswerProvider{}
	}
	return PythonWorkerAnswerProvider{base: base, runner: runner}
}

func (provider PythonWorkerAnswerProvider) GenerateAnswer(
	ctx context.Context,
	request types.AnswerGenerationRequest,
) (types.AnswerGenerationResult, error) {
	if provider.runner == nil {
		return types.AnswerGenerationResult{}, types.ErrRAGUnavailable
	}
	generation, err := provider.base.GenerateAnswer(ctx, request)
	if err != nil {
		return types.AnswerGenerationResult{}, err
	}
	if generation.Status != types.AnswerStatusGrounded || strings.TrimSpace(generation.AnswerText) == "" {
		return generation, nil
	}
	candidateRequest := pythonworker.Request{
		TaskID:        pythonWorkerTaskID(request),
		CandidateID:   pythonWorkerCandidateID(generation.AnswerText),
		WorkerKind:    "LLM",
		OutputType:    "TEXT_CANDIDATE",
		CandidateText: generation.AnswerText,
		SourceRefs:    citationEvidenceIDs(generation.Citations),
		Citations:     citationEvidenceIDs(generation.Citations),
		Confidence:    &generation.Confidence,
	}
	candidate, err := provider.runner.Run(ctx, candidateRequest)
	if err != nil {
		return types.AnswerGenerationResult{}, fmt.Errorf("%w: python worker candidate rejected", types.ErrRAGUnavailable)
	}
	if err := verifyPythonAnswerCandidate(candidate, candidateRequest, generation); err != nil {
		return types.AnswerGenerationResult{}, err
	}
	return generation, nil
}

func verifyPythonAnswerCandidate(
	candidate pythonworker.Candidate,
	request pythonworker.Request,
	generation types.AnswerGenerationResult,
) error {
	if candidate.TaskID != request.TaskID || candidate.CandidateID != request.CandidateID {
		return fmt.Errorf("%w: python worker candidate identity mismatch", types.ErrRAGUnavailable)
	}
	if candidate.Status != pythonworker.StatusCandidate ||
		candidate.WorkerKind != "LLM" ||
		candidate.OutputType != "TEXT_CANDIDATE" {
		return fmt.Errorf("%w: python worker candidate type mismatch", types.ErrRAGUnavailable)
	}
	if candidate.OutputSHA256 != sha256Hex(generation.AnswerText) {
		return fmt.Errorf("%w: python worker candidate hash mismatch", types.ErrRAGUnavailable)
	}
	allowed := make(map[string]struct{}, len(generation.Citations))
	for _, id := range citationEvidenceIDs(generation.Citations) {
		allowed[id] = struct{}{}
	}
	if len(candidate.Citations) == 0 {
		return fmt.Errorf("%w: python worker candidate citation missing", types.ErrRAGUnavailable)
	}
	for _, citation := range candidate.Citations {
		if _, ok := allowed[citation]; !ok {
			return fmt.Errorf("%w: python worker candidate citation mismatch", types.ErrRAGUnavailable)
		}
	}
	return nil
}

func citationEvidenceIDs(citations []types.Citation) []string {
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

func pythonWorkerTaskID(request types.AnswerGenerationRequest) string {
	key := request.Question + "|" + request.EvidencePack.PackID
	return "rag_task_" + sha256Hex(key)[:16]
}

func pythonWorkerCandidateID(answer string) string {
	return "rag_cand_" + sha256Hex(answer)[:16]
}

func sha256Hex(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
