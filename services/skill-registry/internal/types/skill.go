package types

import (
	"encoding/json"
	"strings"
	"time"
)

const (
	SkillStatusActive   = "ACTIVE"
	SkillStatusDisabled = "DISABLED"

	ToolActionCall    int32 = 1
	ToolActionApprove int32 = 2
	ToolActionExecute int32 = 3

	DefaultListSkillsLimit = 50
	MaxListSkillsLimit     = 200
	MaxSkillTextLen        = 4096
)

type SkillDefinition struct {
	TenantID         TenantID
	SkillID          string
	DisplayName      string
	Description      string
	Version          string
	Status           string
	ToolName         string
	AllowedActions   []int32
	InputSchemaJSON  string
	OutputSchemaJSON string
	PermissionScope  string
	RiskLevel        string
	RequiresApproval bool
	AuditEventType   string
	OwnerService     string
	Tags             []string
	MetadataJSON     string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (skill SkillDefinition) Validate() error {
	if skill.TenantID == "" {
		return NewInvalidArgument("tenant_id is required")
	}
	if strings.TrimSpace(skill.SkillID) == "" {
		return NewInvalidArgument("skill_id is required")
	}
	if strings.TrimSpace(skill.DisplayName) == "" {
		return NewInvalidArgument("display_name is required")
	}
	if strings.TrimSpace(skill.Version) == "" {
		return NewInvalidArgument("version is required")
	}
	if strings.TrimSpace(skill.ToolName) == "" {
		return NewInvalidArgument("tool_name is required")
	}
	if strings.TrimSpace(skill.RiskLevel) == "" {
		return NewInvalidArgument("risk_level is required")
	}
	if !isValidSkillStatus(skill.EffectiveStatus()) {
		return NewInvalidArgument("invalid skill status")
	}
	if len(skill.AllowedActions) == 0 {
		return NewInvalidArgument("allowed_actions is required")
	}
	for _, action := range skill.AllowedActions {
		if !isValidToolAction(action) {
			return NewInvalidArgument("invalid tool action")
		}
	}
	if len([]rune(skill.Description)) > MaxSkillTextLen ||
		len([]rune(skill.PermissionScope)) > MaxSkillTextLen ||
		len([]rune(skill.MetadataJSON)) > MaxSkillTextLen {
		return NewInvalidArgument("skill field exceeds maximum length")
	}
	if err := validateJSONObject(skill.EffectiveInputSchemaJSON(), "input_schema_json"); err != nil {
		return err
	}
	if err := validateJSONObject(skill.EffectiveOutputSchemaJSON(), "output_schema_json"); err != nil {
		return err
	}
	if err := validateJSONObject(skill.EffectiveMetadataJSON(), "metadata_json"); err != nil {
		return err
	}
	return nil
}

func (skill SkillDefinition) Normalized() SkillDefinition {
	skill.SkillID = strings.TrimSpace(skill.SkillID)
	skill.DisplayName = strings.TrimSpace(skill.DisplayName)
	skill.Description = strings.TrimSpace(skill.Description)
	skill.Version = strings.TrimSpace(skill.Version)
	skill.Status = skill.EffectiveStatus()
	skill.ToolName = strings.TrimSpace(skill.ToolName)
	skill.InputSchemaJSON = skill.EffectiveInputSchemaJSON()
	skill.OutputSchemaJSON = skill.EffectiveOutputSchemaJSON()
	skill.PermissionScope = strings.TrimSpace(skill.PermissionScope)
	skill.RiskLevel = strings.TrimSpace(skill.RiskLevel)
	skill.AuditEventType = strings.TrimSpace(skill.AuditEventType)
	skill.OwnerService = strings.TrimSpace(skill.OwnerService)
	skill.MetadataJSON = skill.EffectiveMetadataJSON()
	skill.Tags = normalizeTags(skill.Tags)
	return skill
}

func (skill SkillDefinition) EffectiveStatus() string {
	status := strings.TrimSpace(skill.Status)
	if status == "" {
		return SkillStatusActive
	}
	return status
}

func (skill SkillDefinition) EffectiveInputSchemaJSON() string {
	return defaultJSONObject(skill.InputSchemaJSON)
}

func (skill SkillDefinition) EffectiveOutputSchemaJSON() string {
	return defaultJSONObject(skill.OutputSchemaJSON)
}

func (skill SkillDefinition) EffectiveMetadataJSON() string {
	return defaultJSONObject(skill.MetadataJSON)
}

type UpsertSkillCommand struct {
	AuthContext AuthContext
	Skill       SkillDefinition
}

func (command UpsertSkillCommand) Validate() error {
	if err := command.AuthContext.Validate(); err != nil {
		return err
	}
	command.Skill.TenantID = command.AuthContext.TenantID
	return command.Skill.Validate()
}

type GetSkillCommand struct {
	AuthContext AuthContext
	SkillID     string
}

func (command GetSkillCommand) Validate() error {
	if err := command.AuthContext.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(command.SkillID) == "" {
		return NewInvalidArgument("skill_id is required")
	}
	return nil
}

type ListSkillsCommand struct {
	AuthContext  AuthContext
	Status       string
	OwnerService string
	ToolName     string
	Tag          string
	AfterSkillID string
	Limit        int
}

func (command ListSkillsCommand) Validate() error {
	if err := command.AuthContext.Validate(); err != nil {
		return err
	}
	if command.Status != "" && !isValidSkillStatus(command.Status) {
		return NewInvalidArgument("invalid skill status")
	}
	if command.Limit < 0 {
		return NewInvalidArgument("limit must be non-negative")
	}
	if command.Limit > MaxListSkillsLimit {
		return NewInvalidArgument("limit exceeds maximum")
	}
	return nil
}

func (command ListSkillsCommand) EffectiveLimit() int {
	if command.Limit == 0 {
		return DefaultListSkillsLimit
	}
	return command.Limit
}

type ListSkillsResult struct {
	Skills     []SkillDefinition
	NextCursor string
}

func isValidSkillStatus(status string) bool {
	switch status {
	case SkillStatusActive, SkillStatusDisabled:
		return true
	default:
		return false
	}
}

func isValidToolAction(action int32) bool {
	switch action {
	case ToolActionCall, ToolActionApprove, ToolActionExecute:
		return true
	default:
		return false
	}
}

func defaultJSONObject(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "{}"
	}
	return value
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

func normalizeTags(tags []string) []string {
	seen := map[string]struct{}{}
	normalized := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		normalized = append(normalized, tag)
	}
	return normalized
}
