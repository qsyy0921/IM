package pythonworker

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/qsyy0921/IM/internal/ai/llmboundary"
)

const (
	StatusCandidate = "CANDIDATE"
	StatusAbstain   = "ABSTAIN"
	StatusFailed    = "FAILED"
)

var (
	ErrUnsafeRequest      = errors.New("python worker request rejected")
	ErrUnsafeCandidate    = errors.New("python worker candidate rejected")
	ErrMalformedRequest   = errors.New("python worker request malformed")
	ErrMalformedCandidate = errors.New("python worker candidate malformed")
)

var allowedWorkerKinds = map[string]struct{}{
	"LLM":               {},
	"EMBEDDING":         {},
	"RERANK":            {},
	"MEMORY_EXTRACTION": {},
	"PLANNER":           {},
	"EVAL":              {},
}

var allowedStatuses = map[string]struct{}{
	StatusCandidate: {},
	StatusAbstain:   {},
	StatusFailed:    {},
}

var allowedOutputTypes = map[string]struct{}{
	"TEXT_CANDIDATE":         {},
	"EMBEDDING_CANDIDATE":    {},
	"RERANK_CANDIDATE":       {},
	"MEMORY_EVENT_CANDIDATE": {},
	"PLAN_CANDIDATE":         {},
	"EVAL_RESULT":            {},
}

var forbiddenFields = map[string]struct{}{
	"approval_id":       {},
	"approved":          {},
	"approved_at":       {},
	"business_status":   {},
	"db_dsn":            {},
	"execution_id":      {},
	"final_answer_id":   {},
	"password":          {},
	"proposal_status":   {},
	"raw_output":        {},
	"raw_prompt":        {},
	"raw_provider_body": {},
	"refresh_token":     {},
	"secret":            {},
	"session_id":        {},
	"sql":               {},
	"token":             {},
}

type Request struct {
	TaskID        string   `json:"task_id"`
	CandidateID   string   `json:"candidate_id"`
	WorkerKind    string   `json:"worker_kind"`
	OutputType    string   `json:"output_type"`
	CandidateText string   `json:"candidate_text"`
	SourceRefs    []string `json:"source_refs,omitempty"`
	Citations     []string `json:"citations,omitempty"`
	Confidence    *float64 `json:"confidence,omitempty"`
}

type Candidate struct {
	SchemaVersion int      `json:"schema_version"`
	TaskID        string   `json:"task_id"`
	CandidateID   string   `json:"candidate_id"`
	WorkerKind    string   `json:"worker_kind"`
	Status        string   `json:"status"`
	OutputType    string   `json:"output_type"`
	OutputSHA256  string   `json:"output_sha256"`
	SourceRefs    []string `json:"source_refs"`
	Citations     []string `json:"citations"`
	SafetyFlags   []string `json:"safety_flags"`
	Confidence    *float64 `json:"confidence,omitempty"`
	ErrorClass    string   `json:"error_class,omitempty"`
}

func ValidateRequest(request Request) error {
	if required(request.TaskID) == "" {
		return fmt.Errorf("%w: task_id is required", ErrMalformedRequest)
	}
	if required(request.CandidateID) == "" {
		return fmt.Errorf("%w: candidate_id is required", ErrMalformedRequest)
	}
	if _, ok := allowedWorkerKinds[required(request.WorkerKind)]; !ok {
		return fmt.Errorf("%w: unsupported worker_kind", ErrMalformedRequest)
	}
	if _, ok := allowedOutputTypes[required(request.OutputType)]; !ok {
		return fmt.Errorf("%w: unsupported output_type", ErrMalformedRequest)
	}
	if required(request.CandidateText) == "" {
		return fmt.Errorf("%w: candidate_text is required", ErrMalformedRequest)
	}
	if containsSensitive(request.TaskID) ||
		containsSensitive(request.CandidateID) ||
		containsSensitive(request.CandidateText) ||
		containsSensitive(request.WorkerKind) ||
		containsSensitive(request.OutputType) {
		return fmt.Errorf("%w: sensitive text", ErrUnsafeRequest)
	}
	if err := validateStringList(request.SourceRefs, "source_refs"); err != nil {
		return err
	}
	if err := validateStringList(request.Citations, "citations"); err != nil {
		return err
	}
	if request.Confidence != nil && (*request.Confidence < 0 || *request.Confidence > 1) {
		return fmt.Errorf("%w: confidence must be in [0,1]", ErrMalformedRequest)
	}
	return nil
}

func ValidateCandidate(candidate Candidate) error {
	if candidate.SchemaVersion != 1 {
		return fmt.Errorf("%w: schema_version must be 1", ErrMalformedCandidate)
	}
	if required(candidate.TaskID) == "" {
		return fmt.Errorf("%w: task_id is required", ErrMalformedCandidate)
	}
	if required(candidate.CandidateID) == "" {
		return fmt.Errorf("%w: candidate_id is required", ErrMalformedCandidate)
	}
	if _, ok := allowedWorkerKinds[required(candidate.WorkerKind)]; !ok {
		return fmt.Errorf("%w: unsupported worker_kind", ErrMalformedCandidate)
	}
	status := required(candidate.Status)
	if _, ok := allowedStatuses[status]; !ok {
		return fmt.Errorf("%w: unsupported status", ErrMalformedCandidate)
	}
	if _, ok := allowedOutputTypes[required(candidate.OutputType)]; !ok {
		return fmt.Errorf("%w: unsupported output_type", ErrMalformedCandidate)
	}
	if candidate.Confidence != nil && (*candidate.Confidence < 0 || *candidate.Confidence > 1) {
		return fmt.Errorf("%w: confidence must be in [0,1]", ErrMalformedCandidate)
	}
	if status == StatusCandidate {
		if !isLowerSHA256(candidate.OutputSHA256) {
			return fmt.Errorf("%w: candidate output_sha256 must be lowercase sha256", ErrMalformedCandidate)
		}
	} else if strings.TrimSpace(candidate.OutputSHA256) != "" {
		return fmt.Errorf("%w: non-candidate output_sha256 must be empty", ErrMalformedCandidate)
	}
	if status == StatusFailed && required(candidate.ErrorClass) == "" {
		return fmt.Errorf("%w: failed candidate error_class is required", ErrMalformedCandidate)
	}
	if err := validateStringList(candidate.SourceRefs, "source_refs"); err != nil {
		return err
	}
	if err := validateStringList(candidate.Citations, "citations"); err != nil {
		return err
	}
	if err := validateStringList(candidate.SafetyFlags, "safety_flags"); err != nil {
		return err
	}
	if containsSensitive(candidate.TaskID) ||
		containsSensitive(candidate.CandidateID) ||
		containsSensitive(candidate.OutputSHA256) ||
		containsSensitive(candidate.ErrorClass) {
		return fmt.Errorf("%w: sensitive text", ErrUnsafeCandidate)
	}
	return nil
}

func DecodeCandidate(payload []byte) (Candidate, error) {
	var raw map[string]any
	if err := json.Unmarshal(payload, &raw); err != nil {
		return Candidate{}, fmt.Errorf("%w: decode candidate", ErrMalformedCandidate)
	}
	if err := assertNoForbiddenFields(raw, "candidate"); err != nil {
		return Candidate{}, err
	}
	var candidate Candidate
	if err := json.Unmarshal(payload, &candidate); err != nil {
		return Candidate{}, fmt.Errorf("%w: decode candidate", ErrMalformedCandidate)
	}
	if err := ValidateCandidate(candidate); err != nil {
		return Candidate{}, err
	}
	return candidate, nil
}

func MarshalRequest(request Request) ([]byte, error) {
	if err := ValidateRequest(request); err != nil {
		return nil, err
	}
	return json.Marshal(request)
}

func assertNoForbiddenFields(value any, path string) error {
	switch typed := value.(type) {
	case map[string]any:
		for key, nested := range typed {
			normalized := strings.ToLower(strings.TrimSpace(key))
			currentPath := path + "." + normalized
			if _, forbidden := forbiddenFields[normalized]; forbidden {
				return fmt.Errorf("%w: forbidden field %s", ErrUnsafeCandidate, currentPath)
			}
			if containsSensitive(normalized) {
				return fmt.Errorf("%w: sensitive field %s", ErrUnsafeCandidate, currentPath)
			}
			if err := assertNoForbiddenFields(nested, currentPath); err != nil {
				return err
			}
		}
	case []any:
		for index, nested := range typed {
			if err := assertNoForbiddenFields(nested, fmt.Sprintf("%s[%d]", path, index)); err != nil {
				return err
			}
		}
	case string:
		if containsSensitive(typed) {
			return fmt.Errorf("%w: sensitive value at %s", ErrUnsafeCandidate, path)
		}
	}
	return nil
}

func validateStringList(values []string, fieldName string) error {
	seen := make(map[string]struct{}, len(values))
	for _, raw := range values {
		value := required(raw)
		if value == "" {
			return fmt.Errorf("%w: %s contains empty item", ErrMalformedCandidate, fieldName)
		}
		if containsSensitive(value) {
			return fmt.Errorf("%w: %s contains sensitive text", ErrUnsafeCandidate, fieldName)
		}
		if _, exists := seen[value]; exists {
			return fmt.Errorf("%w: %s contains duplicate item", ErrMalformedCandidate, fieldName)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func isLowerSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, ch := range value {
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') {
			return false
		}
	}
	return true
}

func required(value string) string {
	return strings.TrimSpace(value)
}

func containsSensitive(value string) bool {
	return llmboundary.ContainsSensitiveText(value)
}
