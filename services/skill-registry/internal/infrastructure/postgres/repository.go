package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qsyy0921/IM/services/skill-registry/internal/types"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (repository *Repository) UpsertSkill(
	ctx context.Context,
	skill types.SkillDefinition,
) (types.SkillDefinition, error) {
	if repository.pool == nil {
		return types.SkillDefinition{}, types.NewDBWriteFailed("skill registry repository is not configured")
	}
	allowedActions, err := json.Marshal(skill.AllowedActions)
	if err != nil {
		return types.SkillDefinition{}, types.NewInvalidArgument("allowed_actions must be json serializable")
	}
	tags, err := json.Marshal(skill.Tags)
	if err != nil {
		return types.SkillDefinition{}, types.NewInvalidArgument("tags must be json serializable")
	}
	row := repository.pool.QueryRow(ctx, `
INSERT INTO skill_registry_definitions (
	tenant_id,
	skill_id,
	display_name,
	description,
	version,
	status,
	tool_name,
	allowed_actions_json,
	input_schema_json,
	output_schema_json,
	permission_scope,
	risk_level,
	requires_approval,
	audit_event_type,
	owner_service,
	tags_json,
	metadata_json,
	created_at,
	updated_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7,
	$8::jsonb, $9::jsonb, $10::jsonb,
	$11, $12, $13, $14, $15, $16::jsonb, $17::jsonb, now(), now()
)
ON CONFLICT (tenant_id, skill_id) DO UPDATE SET
	display_name = EXCLUDED.display_name,
	description = EXCLUDED.description,
	version = EXCLUDED.version,
	status = EXCLUDED.status,
	tool_name = EXCLUDED.tool_name,
	allowed_actions_json = EXCLUDED.allowed_actions_json,
	input_schema_json = EXCLUDED.input_schema_json,
	output_schema_json = EXCLUDED.output_schema_json,
	permission_scope = EXCLUDED.permission_scope,
	risk_level = EXCLUDED.risk_level,
	requires_approval = EXCLUDED.requires_approval,
	audit_event_type = EXCLUDED.audit_event_type,
	owner_service = EXCLUDED.owner_service,
	tags_json = EXCLUDED.tags_json,
	metadata_json = EXCLUDED.metadata_json,
	updated_at = now()
RETURNING
	tenant_id,
	skill_id,
	display_name,
	description,
	version,
	status,
	tool_name,
	allowed_actions_json,
	input_schema_json,
	output_schema_json,
	permission_scope,
	risk_level,
	requires_approval,
	audit_event_type,
	owner_service,
	tags_json,
	metadata_json,
	created_at,
	updated_at
`, skill.TenantID,
		skill.SkillID,
		skill.DisplayName,
		skill.Description,
		skill.Version,
		skill.Status,
		skill.ToolName,
		string(allowedActions),
		skill.InputSchemaJSON,
		skill.OutputSchemaJSON,
		skill.PermissionScope,
		skill.RiskLevel,
		skill.RequiresApproval,
		skill.AuditEventType,
		skill.OwnerService,
		string(tags),
		skill.MetadataJSON,
	)
	result, err := scanSkill(row)
	if err != nil {
		return types.SkillDefinition{}, types.NewDBWriteFailed(err.Error())
	}
	return result, nil
}

func (repository *Repository) GetSkill(
	ctx context.Context,
	tenantID types.TenantID,
	skillID string,
) (types.SkillDefinition, error) {
	if repository.pool == nil {
		return types.SkillDefinition{}, types.NewDBReadFailed("skill registry repository is not configured")
	}
	row := repository.pool.QueryRow(ctx, `
SELECT
	tenant_id,
	skill_id,
	display_name,
	description,
	version,
	status,
	tool_name,
	allowed_actions_json,
	input_schema_json,
	output_schema_json,
	permission_scope,
	risk_level,
	requires_approval,
	audit_event_type,
	owner_service,
	tags_json,
	metadata_json,
	created_at,
	updated_at
FROM skill_registry_definitions
WHERE tenant_id = $1
  AND skill_id = $2
`, tenantID, strings.TrimSpace(skillID))
	result, err := scanSkill(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return types.SkillDefinition{}, types.NewSkillNotFound("skill not found")
		}
		return types.SkillDefinition{}, types.NewDBReadFailed(err.Error())
	}
	return result, nil
}

func (repository *Repository) ListSkills(
	ctx context.Context,
	command types.ListSkillsCommand,
	fetchLimit int,
) ([]types.SkillDefinition, error) {
	if repository.pool == nil {
		return nil, types.NewDBReadFailed("skill registry repository is not configured")
	}
	if fetchLimit <= 0 {
		return nil, nil
	}

	args := []any{command.AuthContext.TenantID, fetchLimit}
	filters := []string{"tenant_id = $1"}
	if command.Status != "" {
		args = append(args, command.Status)
		filters = append(filters, fmt.Sprintf("status = $%d", len(args)))
	}
	if strings.TrimSpace(command.OwnerService) != "" {
		args = append(args, strings.TrimSpace(command.OwnerService))
		filters = append(filters, fmt.Sprintf("owner_service = $%d", len(args)))
	}
	if strings.TrimSpace(command.ToolName) != "" {
		args = append(args, strings.TrimSpace(command.ToolName))
		filters = append(filters, fmt.Sprintf("tool_name = $%d", len(args)))
	}
	if strings.TrimSpace(command.Tag) != "" {
		args = append(args, strings.TrimSpace(command.Tag))
		filters = append(filters, fmt.Sprintf("tags_json ? $%d", len(args)))
	}
	if strings.TrimSpace(command.AfterSkillID) != "" {
		args = append(args, strings.TrimSpace(command.AfterSkillID))
		filters = append(filters, fmt.Sprintf("skill_id > $%d", len(args)))
	}

	query := `
SELECT
	tenant_id,
	skill_id,
	display_name,
	description,
	version,
	status,
	tool_name,
	allowed_actions_json,
	input_schema_json,
	output_schema_json,
	permission_scope,
	risk_level,
	requires_approval,
	audit_event_type,
	owner_service,
	tags_json,
	metadata_json,
	created_at,
	updated_at
FROM skill_registry_definitions
WHERE ` + strings.Join(filters, "\n  AND ") + `
ORDER BY skill_id ASC
LIMIT $2
`
	rows, err := repository.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, types.NewDBReadFailed(err.Error())
	}
	defer rows.Close()

	items := make([]types.SkillDefinition, 0, fetchLimit)
	for rows.Next() {
		item, err := scanSkill(rows)
		if err != nil {
			return nil, types.NewDBReadFailed(err.Error())
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, types.NewDBReadFailed(err.Error())
	}
	return items, nil
}

type skillScanner interface {
	Scan(dest ...any) error
}

func scanSkill(scanner skillScanner) (types.SkillDefinition, error) {
	var skill types.SkillDefinition
	var allowedActionsRaw []byte
	var tagsRaw []byte
	if err := scanner.Scan(
		&skill.TenantID,
		&skill.SkillID,
		&skill.DisplayName,
		&skill.Description,
		&skill.Version,
		&skill.Status,
		&skill.ToolName,
		&allowedActionsRaw,
		&skill.InputSchemaJSON,
		&skill.OutputSchemaJSON,
		&skill.PermissionScope,
		&skill.RiskLevel,
		&skill.RequiresApproval,
		&skill.AuditEventType,
		&skill.OwnerService,
		&tagsRaw,
		&skill.MetadataJSON,
		&skill.CreatedAt,
		&skill.UpdatedAt,
	); err != nil {
		return types.SkillDefinition{}, err
	}
	if err := json.Unmarshal(allowedActionsRaw, &skill.AllowedActions); err != nil {
		return types.SkillDefinition{}, err
	}
	if err := json.Unmarshal(tagsRaw, &skill.Tags); err != nil {
		return types.SkillDefinition{}, err
	}
	return skill, nil
}
