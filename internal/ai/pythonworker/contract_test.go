package pythonworker

import (
	"errors"
	"testing"
)

func TestValidateCandidateAcceptsLowSensitiveCandidate(t *testing.T) {
	candidate := validCandidate()

	if err := ValidateCandidate(candidate); err != nil {
		t.Fatalf("ValidateCandidate() error = %v", err)
	}
}

func TestValidateCandidateRejectsForbiddenField(t *testing.T) {
	payload := []byte(`{
		"schema_version":1,
		"task_id":"task_01",
		"candidate_id":"cand_01",
		"worker_kind":"MEMORY_EXTRACTION",
		"status":"CANDIDATE",
		"output_type":"MEMORY_EVENT_CANDIDATE",
		"output_sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"raw_output":"secret text"
	}`)

	_, err := DecodeCandidate(payload)
	if !errors.Is(err, ErrUnsafeCandidate) {
		t.Fatalf("DecodeCandidate() error = %v, want ErrUnsafeCandidate", err)
	}
}

func TestValidateCandidateRejectsMalformedHash(t *testing.T) {
	candidate := validCandidate()
	candidate.OutputSHA256 = "ABC"

	err := ValidateCandidate(candidate)
	if !errors.Is(err, ErrMalformedCandidate) {
		t.Fatalf("ValidateCandidate() error = %v, want ErrMalformedCandidate", err)
	}
}

func TestValidateCandidateRejectsSensitiveCitation(t *testing.T) {
	candidate := validCandidate()
	candidate.Citations = []string{"Bearer secret-token-value"}

	err := ValidateCandidate(candidate)
	if !errors.Is(err, ErrUnsafeCandidate) {
		t.Fatalf("ValidateCandidate() error = %v, want ErrUnsafeCandidate", err)
	}
}

func TestValidateRequestRejectsSensitiveCandidateText(t *testing.T) {
	request := validRequest()
	request.CandidateText = "Bearer secret-token-value"

	_, err := MarshalRequest(request)
	if !errors.Is(err, ErrUnsafeRequest) {
		t.Fatalf("MarshalRequest() error = %v, want ErrUnsafeRequest", err)
	}
}

func validRequest() Request {
	confidence := 0.9
	return Request{
		TaskID:        "task_01",
		CandidateID:   "cand_01",
		WorkerKind:    "MEMORY_EXTRACTION",
		OutputType:    "MEMORY_EVENT_CANDIDATE",
		CandidateText: "decision: keep Python worker candidate-only",
		SourceRefs:    []string{"message:tenant:conversation:seq1"},
		Citations:     []string{"message:tenant:conversation:seq1"},
		Confidence:    &confidence,
	}
}

func validCandidate() Candidate {
	confidence := 0.9
	return Candidate{
		SchemaVersion: 1,
		TaskID:        "task_01",
		CandidateID:   "cand_01",
		WorkerKind:    "MEMORY_EXTRACTION",
		Status:        StatusCandidate,
		OutputType:    "MEMORY_EVENT_CANDIDATE",
		OutputSHA256:  "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		SourceRefs:    []string{"message:tenant:conversation:seq1"},
		Citations:     []string{"message:tenant:conversation:seq1"},
		SafetyFlags:   []string{"LOW_SENSITIVE"},
		Confidence:    &confidence,
	}
}
