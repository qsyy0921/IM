package memorycandidate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/qsyy0921/IM/internal/ai/llmboundary"
	"github.com/qsyy0921/IM/internal/ai/pythonworker"
)

const (
	DefaultMaxOutputBytes int64 = 256 * 1024
	DefaultMaxTextRunes         = 4000

	StatusCompleted = "COMPLETED"
	StatusFailed    = "FAILED"
)

var (
	ErrMalformedRequest  = errors.New("memory extraction request malformed")
	ErrUnsafeRequest     = errors.New("memory extraction request rejected")
	ErrMalformedResult   = errors.New("memory extraction result malformed")
	ErrUnsafeResult      = errors.New("memory extraction result rejected")
	ErrWorkerUnavailable = errors.New(
		"memory extraction worker unavailable",
	)
	ErrWorkerFailed = errors.New("memory extraction worker returned failed result")
)

var allowedMemoryEventTypes = map[string]struct{}{
	"DECISION":       {},
	"TASK":           {},
	"STATUS":         {},
	"BLOCKER":        {},
	"FILE":           {},
	"PROFILE_SIGNAL": {},
}

var forbiddenResultFields = map[string]struct{}{
	"conversation_id":   {},
	"db_dsn":            {},
	"fact":              {},
	"fact_text":         {},
	"message_id":        {},
	"message_text":      {},
	"password":          {},
	"raw_message":       {},
	"raw_output":        {},
	"raw_prompt":        {},
	"raw_provider_body": {},
	"raw_text":          {},
	"refresh_token":     {},
	"secret":            {},
	"session_id":        {},
	"speaker_id":        {},
	"sql":               {},
	"tenant_id":         {},
	"text":              {},
	"token":             {},
}

type Message struct {
	MessageID       string `json:"message_id"`
	ConversationSeq int    `json:"conversation_seq"`
	SpeakerID       string `json:"speaker_id"`
	Text            string `json:"text"`
}

type Request struct {
	SchemaVersion  int       `json:"schema_version"`
	TaskID         string    `json:"task_id"`
	TenantID       string    `json:"tenant_id"`
	ConversationID string    `json:"conversation_id"`
	Messages       []Message `json:"messages"`
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

	MemoryEventType string `json:"memory_event_type"`
	ReviewState     string `json:"review_state"`
	FactSHA256      string `json:"fact_sha256"`
	SpeakerIDHash   string `json:"speaker_id_hash"`
	MessageIDHash   string `json:"message_id_hash"`
	ConversationSeq int    `json:"conversation_seq"`
	SourceRefCount  int    `json:"source_ref_count"`
}

type Report struct {
	SchemaVersion                  int            `json:"schema_version"`
	Scope                          string         `json:"scope"`
	MessageCount                   int            `json:"message_count,omitempty"`
	CandidateCount                 int            `json:"candidate_count"`
	OrdinaryMessageCount           int            `json:"ordinary_message_count,omitempty"`
	EventTypeCounts                map[string]int `json:"event_type_counts,omitempty"`
	CandidateHashes                []string       `json:"candidate_hashes,omitempty"`
	RawTextReturned                bool           `json:"raw_text_returned"`
	FinalMemoryPersisted           bool           `json:"final_memory_persisted"`
	RequiresGoValidation           bool           `json:"requires_go_validation"`
	RequiresReviewForProfileSignal bool           `json:"requires_review_for_profile_signal,omitempty"`
}

type Result struct {
	SchemaVersion        int         `json:"schema_version"`
	TaskID               string      `json:"task_id"`
	ExtractorVersion     string      `json:"extractor_version"`
	Status               string      `json:"status"`
	TenantIDHash         string      `json:"tenant_id_hash,omitempty"`
	ConversationIDHash   string      `json:"conversation_id_hash,omitempty"`
	MessageCount         int         `json:"message_count,omitempty"`
	CandidateCount       int         `json:"candidate_count"`
	OrdinaryMessageCount int         `json:"ordinary_message_count,omitempty"`
	Candidates           []Candidate `json:"candidates"`
	Report               Report      `json:"report"`
	ErrorClass           string      `json:"error_class,omitempty"`
}

type RunnerOptions struct {
	Python         string
	ScriptPath     string
	WorkDir        string
	Timeout        time.Duration
	MaxOutputBytes int64
	MaxTextRunes   int
}

type Runner struct {
	python         string
	scriptPath     string
	workDir        string
	timeout        time.Duration
	maxOutputBytes int64
	maxTextRunes   int
}

func NewRunner(options RunnerOptions) (Runner, error) {
	python := normalize(options.Python)
	if python == "" {
		python = "python"
	}
	scriptPath := normalize(options.ScriptPath)
	if scriptPath == "" {
		return Runner{}, errors.New("memory extraction script path is required")
	}
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	maxOutputBytes := options.MaxOutputBytes
	if maxOutputBytes <= 0 {
		maxOutputBytes = DefaultMaxOutputBytes
	}
	maxTextRunes := options.MaxTextRunes
	if maxTextRunes <= 0 {
		maxTextRunes = DefaultMaxTextRunes
	}
	return Runner{
		python:         python,
		scriptPath:     scriptPath,
		workDir:        options.WorkDir,
		timeout:        timeout,
		maxOutputBytes: maxOutputBytes,
		maxTextRunes:   maxTextRunes,
	}, nil
}

func (runner Runner) Run(ctx context.Context, request Request) (Result, error) {
	requestPayload, err := runner.marshalRequest(request)
	if err != nil {
		return Result{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, runner.timeout)
	defer cancel()

	tempDir, err := os.MkdirTemp("", "nexusim-memory-candidate-*")
	if err != nil {
		return Result{}, fmt.Errorf("%w: create temp dir", ErrWorkerUnavailable)
	}
	defer os.RemoveAll(tempDir)

	requestPath := filepath.Join(tempDir, "request.json")
	if err := os.WriteFile(requestPath, requestPayload, 0o600); err != nil {
		return Result{}, fmt.Errorf("%w: write request", ErrWorkerUnavailable)
	}

	command := exec.CommandContext(ctx, runner.python, runner.scriptPath, requestPath)
	if runner.workDir != "" {
		command.Dir = runner.workDir
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, runErr := command.Output()
	if ctx.Err() != nil {
		return Result{}, fmt.Errorf("%w: timeout", ErrWorkerUnavailable)
	}
	if len(output) == 0 && runErr != nil {
		return Result{}, fmt.Errorf("%w: process failed", ErrWorkerUnavailable)
	}
	if int64(len(output)) > runner.maxOutputBytes {
		return Result{}, fmt.Errorf("%w: output too large", ErrMalformedResult)
	}
	result, decodeErr := DecodeResult(limitBytes(output, runner.maxOutputBytes))
	if decodeErr != nil {
		return Result{}, decodeErr
	}
	if runErr != nil || result.Status == StatusFailed {
		return result, ErrWorkerFailed
	}
	return result, nil
}

func DecodeResult(payload []byte) (Result, error) {
	var raw map[string]any
	if err := json.Unmarshal(payload, &raw); err != nil {
		return Result{}, fmt.Errorf("%w: decode result", ErrMalformedResult)
	}
	if err := assertNoForbiddenResult(raw, "result"); err != nil {
		return Result{}, err
	}
	var result Result
	if err := json.Unmarshal(payload, &result); err != nil {
		return Result{}, fmt.Errorf("%w: decode result", ErrMalformedResult)
	}
	if err := ValidateResult(result); err != nil {
		return Result{}, err
	}
	return result, nil
}

func ValidateResult(result Result) error {
	if result.SchemaVersion != 1 {
		return fmt.Errorf("%w: schema_version must be 1", ErrMalformedResult)
	}
	if normalize(result.TaskID) == "" {
		return fmt.Errorf("%w: task_id is required", ErrMalformedResult)
	}
	if containsSensitive(result.TaskID) || containsSensitive(result.ErrorClass) {
		return fmt.Errorf("%w: sensitive result metadata", ErrUnsafeResult)
	}
	if result.Report.RawTextReturned {
		return fmt.Errorf("%w: raw text returned", ErrUnsafeResult)
	}
	if result.Report.FinalMemoryPersisted {
		return fmt.Errorf("%w: final memory persisted", ErrUnsafeResult)
	}
	if !result.Report.RequiresGoValidation {
		return fmt.Errorf("%w: go validation flag is required", ErrMalformedResult)
	}
	switch normalize(result.Status) {
	case StatusCompleted:
		return validateCompletedResult(result)
	case StatusFailed:
		return validateFailedResult(result)
	default:
		return fmt.Errorf("%w: unsupported status", ErrMalformedResult)
	}
}

func ValidateRequest(request Request, maxTextRunes int) error {
	if maxTextRunes <= 0 {
		maxTextRunes = DefaultMaxTextRunes
	}
	if request.SchemaVersion != 1 {
		return fmt.Errorf("%w: schema_version must be 1", ErrMalformedRequest)
	}
	if normalize(request.TaskID) == "" ||
		normalize(request.TenantID) == "" ||
		normalize(request.ConversationID) == "" {
		return fmt.Errorf("%w: required request field is empty", ErrMalformedRequest)
	}
	if containsSensitive(request.TaskID) ||
		containsSensitive(request.TenantID) ||
		containsSensitive(request.ConversationID) {
		return fmt.Errorf("%w: sensitive request metadata", ErrUnsafeRequest)
	}
	if len(request.Messages) == 0 {
		return fmt.Errorf("%w: messages are required", ErrMalformedRequest)
	}
	for index, message := range request.Messages {
		if normalize(message.MessageID) == "" ||
			normalize(message.SpeakerID) == "" ||
			normalize(message.Text) == "" {
			return fmt.Errorf("%w: messages[%d] has empty field", ErrMalformedRequest, index)
		}
		if message.ConversationSeq <= 0 {
			return fmt.Errorf("%w: messages[%d] conversation_seq must be positive", ErrMalformedRequest, index)
		}
		if len([]rune(message.Text)) > maxTextRunes {
			return fmt.Errorf("%w: messages[%d] text exceeds max length", ErrMalformedRequest, index)
		}
		if containsSensitive(message.MessageID) ||
			containsSensitive(message.SpeakerID) ||
			containsSensitive(message.Text) {
			return fmt.Errorf("%w: messages[%d] contains sensitive text", ErrUnsafeRequest, index)
		}
	}
	return nil
}

func (runner Runner) marshalRequest(request Request) ([]byte, error) {
	if err := ValidateRequest(request, runner.maxTextRunes); err != nil {
		return nil, err
	}
	return json.Marshal(request)
}

func validateCompletedResult(result Result) error {
	if result.CandidateCount != len(result.Candidates) {
		return fmt.Errorf("%w: candidate_count mismatch", ErrMalformedResult)
	}
	if result.Report.CandidateCount != result.CandidateCount {
		return fmt.Errorf("%w: report candidate_count mismatch", ErrMalformedResult)
	}
	if result.Report.MessageCount != result.MessageCount {
		return fmt.Errorf("%w: report message_count mismatch", ErrMalformedResult)
	}
	profileSeen := false
	for index, candidate := range result.Candidates {
		if err := ValidateCandidate(candidate); err != nil {
			return fmt.Errorf("%w: candidate[%d]: %v", ErrMalformedResult, index, err)
		}
		if candidate.MemoryEventType == "PROFILE_SIGNAL" {
			profileSeen = true
		}
	}
	if profileSeen && !result.Report.RequiresReviewForProfileSignal {
		return fmt.Errorf("%w: profile signal review flag is required", ErrMalformedResult)
	}
	return nil
}

func validateFailedResult(result Result) error {
	if normalize(result.ErrorClass) == "" {
		return fmt.Errorf("%w: failed result error_class is required", ErrMalformedResult)
	}
	if result.CandidateCount != 0 || len(result.Candidates) != 0 {
		return fmt.Errorf("%w: failed result must not include candidates", ErrMalformedResult)
	}
	return nil
}

func ValidateCandidate(candidate Candidate) error {
	base := pythonworker.Candidate{
		SchemaVersion: candidate.SchemaVersion,
		TaskID:        candidate.TaskID,
		CandidateID:   candidate.CandidateID,
		WorkerKind:    candidate.WorkerKind,
		Status:        candidate.Status,
		OutputType:    candidate.OutputType,
		OutputSHA256:  candidate.OutputSHA256,
		SourceRefs:    candidate.SourceRefs,
		Citations:     candidate.Citations,
		SafetyFlags:   candidate.SafetyFlags,
		Confidence:    candidate.Confidence,
		ErrorClass:    candidate.ErrorClass,
	}
	if err := pythonworker.ValidateCandidate(base); err != nil {
		if errors.Is(err, pythonworker.ErrUnsafeCandidate) {
			return fmt.Errorf("%w: %v", ErrUnsafeResult, err)
		}
		return fmt.Errorf("%w: %v", ErrMalformedResult, err)
	}
	if candidate.WorkerKind != "MEMORY_EXTRACTION" ||
		candidate.Status != pythonworker.StatusCandidate ||
		candidate.OutputType != "MEMORY_EVENT_CANDIDATE" {
		return fmt.Errorf("%w: unsupported memory candidate envelope", ErrMalformedResult)
	}
	if _, ok := allowedMemoryEventTypes[normalize(candidate.MemoryEventType)]; !ok {
		return fmt.Errorf("%w: unsupported memory_event_type", ErrMalformedResult)
	}
	if normalize(candidate.FactSHA256) == "" ||
		normalize(candidate.SpeakerIDHash) == "" ||
		normalize(candidate.MessageIDHash) == "" {
		return fmt.Errorf("%w: memory candidate hash fields are required", ErrMalformedResult)
	}
	if candidate.ConversationSeq <= 0 {
		return fmt.Errorf("%w: conversation_seq must be positive", ErrMalformedResult)
	}
	if candidate.SourceRefCount != len(candidate.SourceRefs) ||
		candidate.SourceRefCount == 0 ||
		len(candidate.Citations) == 0 {
		return fmt.Errorf("%w: source refs and citations are required", ErrMalformedResult)
	}
	if !hasFlag(candidate.SafetyFlags, "LOW_SENSITIVE") ||
		!hasFlag(candidate.SafetyFlags, "GO_VALIDATION_REQUIRED") {
		return fmt.Errorf("%w: required safety flag missing", ErrMalformedResult)
	}
	if candidate.MemoryEventType == "PROFILE_SIGNAL" {
		if candidate.ReviewState != "NEEDS_REVIEW" ||
			!hasFlag(candidate.SafetyFlags, "GROUP_SCOPE_PROFILE_SIGNAL") ||
			!hasFlag(candidate.SafetyFlags, "NEEDS_REVIEW") {
			return fmt.Errorf("%w: profile signal must require review", ErrMalformedResult)
		}
	} else if normalize(candidate.ReviewState) == "" {
		return fmt.Errorf("%w: review_state is required", ErrMalformedResult)
	}
	return nil
}

func assertNoForbiddenResult(value any, path string) error {
	switch typed := value.(type) {
	case map[string]any:
		for key, nested := range typed {
			normalized := strings.ToLower(strings.TrimSpace(key))
			currentPath := path + "." + normalized
			if _, forbidden := forbiddenResultFields[normalized]; forbidden {
				return fmt.Errorf("%w: forbidden field %s", ErrUnsafeResult, currentPath)
			}
			if containsSensitive(normalized) {
				return fmt.Errorf("%w: sensitive field %s", ErrUnsafeResult, currentPath)
			}
			if err := assertNoForbiddenResult(nested, currentPath); err != nil {
				return err
			}
		}
	case []any:
		for index, nested := range typed {
			if err := assertNoForbiddenResult(nested, fmt.Sprintf("%s[%d]", path, index)); err != nil {
				return err
			}
		}
	case string:
		if containsSensitive(typed) {
			return fmt.Errorf("%w: sensitive value at %s", ErrUnsafeResult, path)
		}
	}
	return nil
}

func hasFlag(flags []string, expected string) bool {
	for _, flag := range flags {
		if strings.TrimSpace(flag) == expected {
			return true
		}
	}
	return false
}

func limitBytes(payload []byte, maxBytes int64) []byte {
	if maxBytes <= 0 || int64(len(payload)) <= maxBytes {
		return payload
	}
	reader := io.LimitReader(bytes.NewReader(payload), maxBytes)
	limited, _ := io.ReadAll(reader)
	return limited
}

func normalize(value string) string {
	return strings.TrimSpace(value)
}

func containsSensitive(value string) bool {
	return llmboundary.ContainsSensitiveText(value)
}
