package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	postgresinfra "github.com/qsyy0921/IM/services/message-service/internal/infrastructure/postgres"
)

type outboxRepairAuditOutput struct {
	GeneratedAt string                       `json:"generated_at"`
	Filters     map[string]string            `json:"filters,omitempty"`
	Rows        []outboxRepairAuditOutputRow `json:"rows"`
}

type outboxRepairAuditOutputRow struct {
	EventID                string `json:"event_id"`
	TenantID               string `json:"tenant_id"`
	ConversationID         string `json:"conversation_id,omitempty"`
	Reason                 string `json:"reason"`
	PreviousStatus         string `json:"previous_status"`
	PreviousRetryCount     int    `json:"previous_retry_count"`
	PreviousLastError      string `json:"previous_last_error,omitempty"`
	PreviousDeadLetteredAt string `json:"previous_dead_lettered_at,omitempty"`
	RepairedAt             string `json:"repaired_at"`
}

func writeOutboxRepairAuditOutput(path string, rows []postgresinfra.OutboxRepairAuditRow, filters map[string]string) error {
	output := outboxRepairAuditOutput{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Filters:     compactCleanupFilters(filters),
		Rows:        make([]outboxRepairAuditOutputRow, 0, len(rows)),
	}
	for _, row := range rows {
		outputRow := outboxRepairAuditOutputRow{
			EventID:            row.EventID,
			TenantID:           row.TenantID,
			ConversationID:     row.ConversationID,
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
