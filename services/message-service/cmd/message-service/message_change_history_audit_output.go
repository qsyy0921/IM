package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	postgresinfra "github.com/qsyy0921/IM/services/message-service/internal/infrastructure/postgres"
)

type messageChangeHistoryAuditOutput struct {
	GeneratedAt string                               `json:"generated_at"`
	Rows        []messageChangeHistoryAuditOutputRow `json:"rows"`
}

type messageChangeHistoryAuditOutputRow struct {
	TenantID             string `json:"tenant_id"`
	ConversationID       string `json:"conversation_id"`
	MessageID            string `json:"message_id"`
	ChangeVersion        int    `json:"change_version"`
	ChangeType           string `json:"change_type"`
	BeforePayloadPresent bool   `json:"before_payload_present"`
	AfterPayloadPresent  bool   `json:"after_payload_present"`
	BeforeStatus         string `json:"before_status"`
	AfterStatus          string `json:"after_status"`
	ChangedBy            string `json:"changed_by"`
	ReasonPresent        bool   `json:"reason_present"`
	TraceID              string `json:"trace_id,omitempty"`
	ChangedAt            string `json:"changed_at"`
}

func writeMessageChangeHistoryAuditOutput(path string, rows []postgresinfra.MessageChangeHistoryAuditRow) error {
	output := messageChangeHistoryAuditOutput{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Rows:        make([]messageChangeHistoryAuditOutputRow, 0, len(rows)),
	}
	for _, row := range rows {
		output.Rows = append(output.Rows, messageChangeHistoryAuditOutputRow{
			TenantID:             row.TenantID,
			ConversationID:       row.ConversationID,
			MessageID:            row.MessageID,
			ChangeVersion:        row.ChangeVersion,
			ChangeType:           row.ChangeType,
			BeforePayloadPresent: row.BeforePayloadPresent,
			AfterPayloadPresent:  row.AfterPayloadPresent,
			BeforeStatus:         row.BeforeStatus,
			AfterStatus:          row.AfterStatus,
			ChangedBy:            row.ChangedBy,
			ReasonPresent:        row.ReasonPresent,
			TraceID:              row.TraceID,
			ChangedAt:            row.ChangedAt.UTC().Format(time.RFC3339Nano),
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
