package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	postgresinfra "github.com/qsyy0921/IM/services/message-service/internal/infrastructure/postgres"
)

type messageRetentionProofAuditOutput struct {
	GeneratedAt string                                `json:"generated_at"`
	Rows        []messageRetentionProofAuditOutputRow `json:"rows"`
}

type messageRetentionProofAuditOutputRow struct {
	TenantID                   string `json:"tenant_id"`
	ConversationID             string `json:"conversation_id"`
	MessageID                  string `json:"message_id"`
	ConversationSeq            int64  `json:"conversation_seq"`
	SenderID                   string `json:"sender_id"`
	MessageType                string `json:"message_type"`
	Status                     string `json:"status"`
	CurrentPayloadPresent      bool   `json:"current_payload_present"`
	CreatedAt                  string `json:"created_at"`
	DeletedAt                  string `json:"deleted_at,omitempty"`
	DeleteChangeVersion        *int   `json:"delete_change_version,omitempty"`
	DeleteChangedBy            string `json:"delete_changed_by,omitempty"`
	DeleteReasonPresent        bool   `json:"delete_reason_present"`
	DeleteBeforePayloadPresent bool   `json:"delete_before_payload_present"`
	DeleteAfterPayloadPresent  bool   `json:"delete_after_payload_present"`
	DeleteChangedAt            string `json:"delete_changed_at,omitempty"`
	DeleteTimelineEventPresent bool   `json:"delete_timeline_event_present"`
	DeleteOutboxEventPresent   bool   `json:"delete_outbox_event_present"`
}

func writeMessageRetentionProofAuditOutput(path string, rows []postgresinfra.MessageRetentionProofAuditRow) error {
	output := messageRetentionProofAuditOutput{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Rows:        make([]messageRetentionProofAuditOutputRow, 0, len(rows)),
	}
	for _, row := range rows {
		output.Rows = append(output.Rows, messageRetentionProofAuditOutputRow{
			TenantID:                   row.TenantID,
			ConversationID:             row.ConversationID,
			MessageID:                  row.MessageID,
			ConversationSeq:            row.ConversationSeq,
			SenderID:                   row.SenderID,
			MessageType:                row.MessageType,
			Status:                     row.Status,
			CurrentPayloadPresent:      row.CurrentPayloadPresent,
			CreatedAt:                  row.CreatedAt.UTC().Format(time.RFC3339Nano),
			DeletedAt:                  formatOptionalTimeRFC3339Nano(row.DeletedAt),
			DeleteChangeVersion:        row.DeleteChangeVersion,
			DeleteChangedBy:            row.DeleteChangedBy,
			DeleteReasonPresent:        row.DeleteReasonPresent,
			DeleteBeforePayloadPresent: row.DeleteBeforePayloadPresent,
			DeleteAfterPayloadPresent:  row.DeleteAfterPayloadPresent,
			DeleteChangedAt:            formatOptionalTimeRFC3339Nano(row.DeleteChangedAt),
			DeleteTimelineEventPresent: row.DeleteTimelineEventPresent,
			DeleteOutboxEventPresent:   row.DeleteOutboxEventPresent,
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

func formatOptionalTimeRFC3339Nano(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}
