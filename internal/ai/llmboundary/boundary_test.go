package llmboundary

import (
	"errors"
	"testing"
)

func TestBuildPromptRejectsSensitiveEvidence(t *testing.T) {
	_, err := BuildPrompt("answer", "query", []Evidence{{
		EvidenceID: "e1",
		Text:       "contact alice@example.com",
	}}, Options{})
	if !errors.Is(err, ErrUnsafeInput) {
		t.Fatalf("expected unsafe input, got %v", err)
	}
}

func TestBuildPromptAppliesBudgetAndTruncation(t *testing.T) {
	prompt, err := BuildPrompt("answer", " query ", []Evidence{
		{EvidenceID: "e1", Text: "one two three four"},
		{EvidenceID: "e2", Text: "second"},
	}, Options{TokenBudget: 300, MaxEvidenceItems: 1, MaxTextRunes: 7})
	if err != nil {
		t.Fatalf("BuildPrompt returned error: %v", err)
	}
	if prompt.TokenBudget != 300 || len(prompt.Evidence) != 1 {
		t.Fatalf("unexpected prompt: %+v", prompt)
	}
	if prompt.Evidence[0].Text != "one two" {
		t.Fatalf("expected truncated evidence, got %q", prompt.Evidence[0].Text)
	}
}

func TestValidateCandidateRejectsUnsafeOutput(t *testing.T) {
	err := ValidateCandidate(Candidate{
		Text:                "Bearer provider-token-value",
		CitationEvidenceIDs: []string{"e1"},
		Confidence:          0.7,
	}, map[string]struct{}{"e1": {}})
	if !errors.Is(err, ErrUnsafeOutput) {
		t.Fatalf("expected unsafe output, got %v", err)
	}
}

func TestValidateCandidateRejectsUnknownCitation(t *testing.T) {
	err := ValidateCandidate(Candidate{
		Text:                "grounded answer",
		CitationEvidenceIDs: []string{"missing"},
		Confidence:          0.7,
	}, map[string]struct{}{"e1": {}})
	if !errors.Is(err, ErrMalformedOutput) {
		t.Fatalf("expected malformed output, got %v", err)
	}
}
