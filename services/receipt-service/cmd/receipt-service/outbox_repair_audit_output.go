package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	postgresinfra "github.com/qsyy0921/IM/services/receipt-service/internal/infrastructure/postgres"
)

type outboxRepairAuditOutput struct {
	GeneratedAt string                       `json:"generated_at"`
	Rows        []outboxRepairAuditOutputRow `json:"rows"`
}

type outboxRepairAuditOutputRow struct {
	EventID                string `json:"event_id"`
	TenantID               string `json:"tenant_id"`
	Reason                 string `json:"reason"`
	PreviousStatus         string `json:"previous_status"`
	PreviousRetryCount     int    `json:"previous_retry_count"`
	PreviousLastError      string `json:"previous_last_error,omitempty"`
	PreviousDeadLetteredAt string `json:"previous_dead_lettered_at,omitempty"`
	RepairedAt             string `json:"repaired_at"`
}

func writeOutboxRepairAuditOutput(path string, rows []postgresinfra.OutboxRepairAuditRow) error {
	output := outboxRepairAuditOutput{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Rows:        make([]outboxRepairAuditOutputRow, 0, len(rows)),
	}
	for _, row := range rows {
		outputRow := outboxRepairAuditOutputRow{
			EventID:            row.EventID,
			TenantID:           row.TenantID,
			Reason:             row.Reason,
			PreviousStatus:     row.PreviousStatus,
			PreviousRetryCount: row.PreviousRetryCount,
			PreviousLastError:  row.PreviousLastError,
			RepairedAt:         row.RepairedAt.UTC().Format(time.RFC3339Nano),
		}
		if row.PreviousDeadLetteredAt != nil {
			outputRow.PreviousDeadLetteredAt = row.PreviousDeadLetteredAt.UTC().Format(time.RFC3339Nano)
		}
		output.Rows = append(output.Rows, outputRow)
	}
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
