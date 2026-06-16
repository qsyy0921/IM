package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	postgresinfra "github.com/qsyy0921/IM/services/message-service/internal/infrastructure/postgres"
)

type outboxAuditOutput struct {
	GeneratedAt string                 `json:"generated_at"`
	Rows        []outboxAuditOutputRow `json:"rows"`
}

type outboxAuditOutputRow struct {
	ID               int64  `json:"id"`
	EventID          string `json:"event_id"`
	TenantID         string `json:"tenant_id"`
	ConversationID   string `json:"conversation_id,omitempty"`
	AggregateVersion int64  `json:"aggregate_version"`
	EventType        string `json:"event_type"`
	Status           string `json:"status"`
	RetryCount       int    `json:"retry_count"`
	LastError        string `json:"last_error,omitempty"`
	AvailableAt      string `json:"available_at"`
	NextRetryAt      string `json:"next_retry_at,omitempty"`
	PublishedAt      string `json:"published_at,omitempty"`
	DeadLetteredAt   string `json:"dead_lettered_at,omitempty"`
	CreatedAt        string `json:"created_at"`
}

func writeOutboxAuditOutput(path string, rows []postgresinfra.OutboxAuditRow) error {
	output := outboxAuditOutput{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Rows:        make([]outboxAuditOutputRow, 0, len(rows)),
	}
	for _, row := range rows {
		output.Rows = append(output.Rows, outboxAuditOutputRow{
			ID:               row.ID,
			EventID:          row.EventID,
			TenantID:         row.TenantID,
			ConversationID:   row.ConversationID,
			AggregateVersion: row.AggregateVersion,
			EventType:        row.EventType,
			Status:           row.Status,
			RetryCount:       row.RetryCount,
			LastError:        row.LastError,
			AvailableAt:      formatOutboxAuditTime(row.AvailableAt),
			NextRetryAt:      formatOptionalOutboxAuditTime(row.NextRetryAt),
			PublishedAt:      formatOptionalOutboxAuditTime(row.PublishedAt),
			DeadLetteredAt:   formatOptionalOutboxAuditTime(row.DeadLetteredAt),
			CreatedAt:        formatOutboxAuditTime(row.CreatedAt),
		})
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

func formatOutboxAuditTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func formatOptionalOutboxAuditTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return formatOutboxAuditTime(*value)
}
