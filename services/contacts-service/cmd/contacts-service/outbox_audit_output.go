package main

import (
	"time"

	postgresinfra "github.com/qsyy0921/IM/services/contacts-service/internal/infrastructure/postgres"
)

type outboxAuditOutput struct {
	GeneratedAt string                 `json:"generated_at"`
	Rows        []outboxAuditOutputRow `json:"rows"`
}

type outboxAuditOutputRow struct {
	ID               int64  `json:"id"`
	EventID          string `json:"event_id"`
	TenantID         string `json:"tenant_id"`
	AggregateType    string `json:"aggregate_type"`
	AggregateID      string `json:"aggregate_id"`
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
			AggregateType:    row.AggregateType,
			AggregateID:      row.AggregateID,
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
	return writeJSONFile(path, output)
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
