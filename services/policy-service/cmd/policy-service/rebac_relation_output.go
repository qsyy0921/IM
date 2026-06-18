package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	postgresinfra "github.com/qsyy0921/IM/services/policy-service/internal/infrastructure/postgres"
)

type rebacRelationRuleOutput struct {
	GeneratedAt string                       `json:"generated_at"`
	Rows        []rebacRelationRuleOutputRow `json:"rows"`
}

type rebacRelationRuleSetOutput struct {
	GeneratedAt string                     `json:"generated_at"`
	Row         rebacRelationRuleOutputRow `json:"row"`
}

type rebacRelationRuleOutputRow struct {
	TenantID          string `json:"tenant_id"`
	Action            string `json:"action"`
	RelationType      string `json:"relation_type"`
	ConversationScope string `json:"conversation_scope"`
	PermissionVersion int64  `json:"permission_version"`
	Classification    string `json:"classification"`
	ReasonPresent     bool   `json:"reason_present"`
	Priority          int    `json:"priority"`
	Enabled           bool   `json:"enabled"`
	Source            string `json:"source"`
	UpdatedAt         string `json:"updated_at"`
}

func writeReBACRelationRuleAuditOutput(path string, rows []postgresinfra.ReBACRelationRuleRow) error {
	output := rebacRelationRuleOutput{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Rows:        make([]rebacRelationRuleOutputRow, 0, len(rows)),
	}
	for _, row := range rows {
		output.Rows = append(output.Rows, rebacRelationRuleOutputRowFromRow(row))
	}
	return writeReBACRelationRuleJSON(path, output)
}

func writeReBACRelationRuleSetOutput(path string, row postgresinfra.ReBACRelationRuleRow) error {
	return writeReBACRelationRuleJSON(path, rebacRelationRuleSetOutput{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Row:         rebacRelationRuleOutputRowFromRow(row),
	})
}

func rebacRelationRuleOutputRowFromRow(row postgresinfra.ReBACRelationRuleRow) rebacRelationRuleOutputRow {
	return rebacRelationRuleOutputRow{
		TenantID:          row.TenantID,
		Action:            row.Action,
		RelationType:      row.RelationType,
		ConversationScope: row.ConversationScope,
		PermissionVersion: row.PermissionVersion,
		Classification:    row.Classification,
		ReasonPresent:     row.Reason != "",
		Priority:          row.Priority,
		Enabled:           row.Enabled,
		Source:            row.Source,
		UpdatedAt:         formatOutboxAuditTime(row.UpdatedAt),
	}
}

func writeReBACRelationRuleJSON(path string, output any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}
