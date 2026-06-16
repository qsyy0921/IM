package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	postgresinfra "github.com/qsyy0921/IM/services/conversation-service/internal/infrastructure/postgres"
)

type memberChangeAuditOutput struct {
	GeneratedAt string                       `json:"generated_at"`
	Filters     map[string]string            `json:"filters,omitempty"`
	Rows        []memberChangeAuditOutputRow `json:"rows"`
}

type memberChangeAuditOutputRow struct {
	ChangeID        string `json:"change_id"`
	TenantID        string `json:"tenant_id"`
	ConversationID  string `json:"conversation_id"`
	TargetUserID    string `json:"target_user_id"`
	OperatorUserID  string `json:"operator_user_id"`
	ChangeType      string `json:"change_type"`
	Status          string `json:"status"`
	BoundarySeq     int64  `json:"boundary_seq"`
	TimelineEventID string `json:"timeline_event_id,omitempty"`
	OutboxEventID   string `json:"outbox_event_id,omitempty"`
	RetryCount      int    `json:"retry_count"`
	LastError       string `json:"last_error,omitempty"`
	NextRetryAt     string `json:"next_retry_at,omitempty"`
	DeadLetteredAt  string `json:"dead_lettered_at,omitempty"`
	CompletedAt     string `json:"completed_at,omitempty"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
}

func writeMemberChangeAuditOutput(path string, rows []postgresinfra.MemberChangeAuditRow, filters map[string]string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	output := memberChangeAuditOutput{
		GeneratedAt: formatAuditOutputTime(time.Now()),
		Filters:     compactAuditOutputFilters(filters),
		Rows:        make([]memberChangeAuditOutputRow, 0, len(rows)),
	}
	for _, row := range rows {
		output.Rows = append(output.Rows, memberChangeAuditOutputRow{
			ChangeID:        row.ChangeID,
			TenantID:        row.TenantID,
			ConversationID:  row.ConversationID,
			TargetUserID:    row.TargetUserID,
			OperatorUserID:  row.OperatorUserID,
			ChangeType:      row.ChangeType,
			Status:          row.Status,
			BoundarySeq:     row.BoundarySeq,
			TimelineEventID: row.TimelineEventID,
			OutboxEventID:   row.OutboxEventID,
			RetryCount:      row.RetryCount,
			LastError:       row.LastError,
			NextRetryAt:     formatOptionalAuditOutputTime(row.NextRetryAt),
			DeadLetteredAt:  formatOptionalAuditOutputTime(row.DeadLetteredAt),
			CompletedAt:     formatOptionalAuditOutputTime(row.CompletedAt),
			CreatedAt:       formatAuditOutputTime(row.CreatedAt),
			UpdatedAt:       formatAuditOutputTime(row.UpdatedAt),
		})
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}

func compactAuditOutputFilters(filters map[string]string) map[string]string {
	if len(filters) == 0 {
		return nil
	}
	compacted := make(map[string]string, len(filters))
	for key, value := range filters {
		if value == "" {
			continue
		}
		compacted[key] = value
	}
	if len(compacted) == 0 {
		return nil
	}
	return compacted
}

func formatOptionalAuditOutputTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return formatAuditOutputTime(*value)
}

func formatAuditOutputTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}
