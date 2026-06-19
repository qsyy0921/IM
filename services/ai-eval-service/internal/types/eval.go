package types

import (
	"encoding/json"
	"strings"
	"time"
)

const (
	EvalRunStatusPending = "PENDING"
	EvalRunStatusRunning = "RUNNING"
	EvalRunStatusPassed  = "PASSED"
	EvalRunStatusFailed  = "FAILED"

	DefaultListEvalRunsLimit = 50
	MaxListEvalRunsLimit     = 200
	MaxEvalRunTextLen        = 4096
	MaxEvalRunRefLen         = 1024
)

type EvalRun struct {
	TenantID     TenantID
	RunID        string
	SuiteID      string
	Stage        string
	Adapter      string
	Status       string
	CaseCount    int
	PassedCount  int
	FailedCount  int
	SkippedCount int
	SummaryRef   string
	ReportRef    string
	MetadataJSON string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	CompletedAt  time.Time
}

func (run EvalRun) Validate() error {
	if run.TenantID == "" {
		return NewInvalidArgument("tenant_id is required")
	}
	if strings.TrimSpace(run.RunID) == "" {
		return NewInvalidArgument("run_id is required")
	}
	if strings.TrimSpace(run.SuiteID) == "" {
		return NewInvalidArgument("suite_id is required")
	}
	if !isValidEvalRunStatus(run.EffectiveStatus()) {
		return NewInvalidArgument("invalid eval run status")
	}
	if run.CaseCount < 0 || run.PassedCount < 0 || run.FailedCount < 0 || run.SkippedCount < 0 {
		return NewInvalidArgument("eval run counts must be non-negative")
	}
	if run.PassedCount+run.FailedCount+run.SkippedCount > run.CaseCount {
		return NewInvalidArgument("eval run result counts exceed case_count")
	}
	if len([]rune(run.Stage)) > MaxEvalRunTextLen ||
		len([]rune(run.Adapter)) > MaxEvalRunTextLen ||
		len([]rune(run.MetadataJSON)) > MaxEvalRunTextLen {
		return NewInvalidArgument("eval run field exceeds maximum length")
	}
	if len([]rune(run.SummaryRef)) > MaxEvalRunRefLen || len([]rune(run.ReportRef)) > MaxEvalRunRefLen {
		return NewInvalidArgument("eval run reference exceeds maximum length")
	}
	if err := validateJSONObject(run.EffectiveMetadataJSON(), "metadata_json"); err != nil {
		return err
	}
	return nil
}

func (run EvalRun) Normalized() EvalRun {
	run.RunID = strings.TrimSpace(run.RunID)
	run.SuiteID = strings.TrimSpace(run.SuiteID)
	run.Stage = strings.TrimSpace(run.Stage)
	run.Adapter = strings.TrimSpace(run.Adapter)
	run.Status = run.EffectiveStatus()
	run.SummaryRef = strings.TrimSpace(run.SummaryRef)
	run.ReportRef = strings.TrimSpace(run.ReportRef)
	run.MetadataJSON = run.EffectiveMetadataJSON()
	return run
}

func (run EvalRun) EffectiveStatus() string {
	status := strings.TrimSpace(run.Status)
	if status == "" {
		return EvalRunStatusPending
	}
	return status
}

func (run EvalRun) EffectiveMetadataJSON() string {
	value := strings.TrimSpace(run.MetadataJSON)
	if value == "" {
		return "{}"
	}
	return value
}

type RecordEvalRunCommand struct {
	AuthContext AuthContext
	Run         EvalRun
}

func (command RecordEvalRunCommand) Validate() error {
	if err := command.AuthContext.Validate(); err != nil {
		return err
	}
	command.Run.TenantID = command.AuthContext.TenantID
	return command.Run.Validate()
}

type GetEvalRunCommand struct {
	AuthContext AuthContext
	RunID       string
}

func (command GetEvalRunCommand) Validate() error {
	if err := command.AuthContext.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(command.RunID) == "" {
		return NewInvalidArgument("run_id is required")
	}
	return nil
}

type ListEvalRunsCommand struct {
	AuthContext AuthContext
	SuiteID     string
	Status      string
	AfterRunID  string
	Limit       int
}

func (command ListEvalRunsCommand) Validate() error {
	if err := command.AuthContext.Validate(); err != nil {
		return err
	}
	if command.Status != "" && !isValidEvalRunStatus(command.Status) {
		return NewInvalidArgument("invalid eval run status")
	}
	if command.Limit < 0 {
		return NewInvalidArgument("limit must be non-negative")
	}
	if command.Limit > MaxListEvalRunsLimit {
		return NewInvalidArgument("limit exceeds maximum")
	}
	return nil
}

func (command ListEvalRunsCommand) EffectiveLimit() int {
	if command.Limit == 0 {
		return DefaultListEvalRunsLimit
	}
	return command.Limit
}

type ListEvalRunsResult struct {
	Runs       []EvalRun
	NextCursor string
}

func isValidEvalRunStatus(status string) bool {
	switch status {
	case EvalRunStatusPending, EvalRunStatusRunning, EvalRunStatusPassed, EvalRunStatusFailed:
		return true
	default:
		return false
	}
}

func validateJSONObject(value string, field string) error {
	var decoded any
	if err := json.Unmarshal([]byte(value), &decoded); err != nil {
		return NewInvalidArgument(field + " must be valid json")
	}
	if _, ok := decoded.(map[string]any); !ok {
		return NewInvalidArgument(field + " must be a json object")
	}
	return nil
}
