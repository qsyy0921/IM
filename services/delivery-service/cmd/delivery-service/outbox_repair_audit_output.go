package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	postgresinfra "github.com/qsyy0921/IM/services/delivery-service/internal/infrastructure/postgres"
)

type outboxRepairAuditOutput struct {
	GeneratedAt string                       `json:"generated_at"`
	Filters     map[string]string            `json:"filters,omitempty"`
	Rows        []outboxRepairAuditOutputRow `json:"rows"`
}

type outboxRepairAuditOutputRow struct {
	OutboxID             int64  `json:"outbox_id"`
	EventID              string `json:"event_id"`
	TenantID             string `json:"tenant_id"`
	ConversationID       string `json:"conversation_id,omitempty"`
	AggregateVersion     int64  `json:"aggregate_version"`
	Mode                 string `json:"mode"`
	Outcome              string `json:"outcome"`
	SkipReason           string `json:"skip_reason,omitempty"`
	Operator             string `json:"operator,omitempty"`
	Reason               string `json:"reason"`
	DryRun               bool   `json:"dry_run"`
	BeforeStatus         string `json:"before_status"`
	BeforeRetryCount     int    `json:"before_retry_count"`
	BeforeLastError      string `json:"before_last_error,omitempty"`
	BeforeNextRetryAt    string `json:"before_next_retry_at,omitempty"`
	BeforeDeadLetteredAt string `json:"before_dead_lettered_at,omitempty"`
	AfterStatus          string `json:"after_status"`
	AfterRetryCount      int    `json:"after_retry_count"`
	AfterLastError       string `json:"after_last_error,omitempty"`
	AfterNextRetryAt     string `json:"after_next_retry_at,omitempty"`
	AfterDeadLetteredAt  string `json:"after_dead_lettered_at,omitempty"`
	CreatedAt            string `json:"created_at"`
}

func writeOutboxRepairAuditOutput(path string, rows []postgresinfra.OutboxRepairAuditRow, filters map[string]string) error {
	output := outboxRepairAuditOutput{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Filters:     compactCleanupFilters(filters),
		Rows:        make([]outboxRepairAuditOutputRow, 0, len(rows)),
	}
	for _, row := range rows {
		outputRow := outboxRepairAuditOutputRow{
			OutboxID:         row.OutboxID,
			EventID:          row.EventID,
			TenantID:         row.TenantID,
			ConversationID:   row.ConversationID,
			AggregateVersion: row.AggregateVersion,
			Mode:             row.Mode,
			Outcome:          row.Outcome,
			SkipReason:       row.SkipReason,
			Operator:         row.Operator,
			Reason:           row.Reason,
			DryRun:           row.DryRun,
			BeforeStatus:     row.BeforeStatus,
			BeforeRetryCount: row.BeforeRetryCount,
			BeforeLastError:  row.BeforeLastError,
			AfterStatus:      row.AfterStatus,
			AfterRetryCount:  row.AfterRetryCount,
			AfterLastError:   row.AfterLastError,
			CreatedAt:        row.CreatedAt.UTC().Format(time.RFC3339Nano),
		}
		if row.BeforeNextRetryAt != nil {
			outputRow.BeforeNextRetryAt = row.BeforeNextRetryAt.UTC().Format(time.RFC3339Nano)
		}
		if row.BeforeDeadLetteredAt != nil {
			outputRow.BeforeDeadLetteredAt = row.BeforeDeadLetteredAt.UTC().Format(time.RFC3339Nano)
		}
		if row.AfterNextRetryAt != nil {
			outputRow.AfterNextRetryAt = row.AfterNextRetryAt.UTC().Format(time.RFC3339Nano)
		}
		if row.AfterDeadLetteredAt != nil {
			outputRow.AfterDeadLetteredAt = row.AfterDeadLetteredAt.UTC().Format(time.RFC3339Nano)
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
