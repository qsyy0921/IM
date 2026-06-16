package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	postgresinfra "github.com/qsyy0921/IM/services/policy-service/internal/infrastructure/postgres"
)

type tenantQuotaOutput struct {
	GeneratedAt string                 `json:"generated_at"`
	Rows        []tenantQuotaOutputRow `json:"rows"`
}

type tenantQuotaSetOutput struct {
	GeneratedAt string               `json:"generated_at"`
	Row         tenantQuotaOutputRow `json:"row"`
}

type tenantQuotaOutputRow struct {
	TenantID          string `json:"tenant_id"`
	Action            string `json:"action"`
	MaxDecisions      int    `json:"max_decisions"`
	WindowSeconds     int    `json:"window_seconds"`
	PermissionVersion int64  `json:"permission_version"`
	Classification    string `json:"classification"`
	ReasonPresent     bool   `json:"reason_present"`
	Enabled           bool   `json:"enabled"`
	Source            string `json:"source"`
	UpdatedAt         string `json:"updated_at"`
}

func writeTenantQuotaAuditOutput(path string, rows []postgresinfra.TenantQuotaRow) error {
	output := tenantQuotaOutput{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Rows:        make([]tenantQuotaOutputRow, 0, len(rows)),
	}
	for _, row := range rows {
		output.Rows = append(output.Rows, tenantQuotaOutputRowFromRow(row))
	}
	return writeTenantQuotaJSON(path, output)
}

func writeTenantQuotaSetOutput(path string, row postgresinfra.TenantQuotaRow) error {
	return writeTenantQuotaJSON(path, tenantQuotaSetOutput{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Row:         tenantQuotaOutputRowFromRow(row),
	})
}

func tenantQuotaOutputRowFromRow(row postgresinfra.TenantQuotaRow) tenantQuotaOutputRow {
	return tenantQuotaOutputRow{
		TenantID:          row.TenantID,
		Action:            row.Action,
		MaxDecisions:      row.MaxDecisions,
		WindowSeconds:     row.WindowSeconds,
		PermissionVersion: row.PermissionVersion,
		Classification:    row.Classification,
		ReasonPresent:     row.Reason != "",
		Enabled:           row.Enabled,
		Source:            row.Source,
		UpdatedAt:         formatOutboxAuditTime(row.UpdatedAt),
	}
}

func writeTenantQuotaJSON(path string, output any) error {
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
