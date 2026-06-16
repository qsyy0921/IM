package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	postgresinfra "github.com/qsyy0921/IM/services/message-service/internal/infrastructure/postgres"
)

type messageLegalHoldOutput struct {
	GeneratedAt string                      `json:"generated_at"`
	Rows        []messageLegalHoldOutputRow `json:"rows"`
}

type messageLegalHoldMutationOutput struct {
	GeneratedAt string                    `json:"generated_at"`
	Row         messageLegalHoldOutputRow `json:"row"`
}

type messageLegalHoldOutputRow struct {
	TenantID       string `json:"tenant_id"`
	ConversationID string `json:"conversation_id"`
	MessageID      string `json:"message_id"`
	HoldID         string `json:"hold_id"`
	Status         string `json:"status"`
	ReasonPresent  bool   `json:"reason_present"`
	CreatedBy      string `json:"created_by"`
	CreatedAt      string `json:"created_at"`
	ReleasedBy     string `json:"released_by,omitempty"`
	ReleasedAt     string `json:"released_at,omitempty"`
	UpdatedAt      string `json:"updated_at"`
}

func writeMessageLegalHoldAuditOutput(path string, rows []postgresinfra.MessageLegalHoldAuditRow) error {
	output := messageLegalHoldOutput{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Rows:        make([]messageLegalHoldOutputRow, 0, len(rows)),
	}
	for _, row := range rows {
		output.Rows = append(output.Rows, messageLegalHoldOutputRowFromResult(row))
	}
	return writeMessageLegalHoldJSON(path, output)
}

func writeMessageLegalHoldMutationOutput(path string, row postgresinfra.MessageLegalHoldMutationResult) error {
	output := messageLegalHoldMutationOutput{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Row:         messageLegalHoldOutputRowFromResult(row),
	}
	return writeMessageLegalHoldJSON(path, output)
}

func messageLegalHoldOutputRowFromResult(row postgresinfra.MessageLegalHoldMutationResult) messageLegalHoldOutputRow {
	return messageLegalHoldOutputRow{
		TenantID:       row.TenantID,
		ConversationID: row.ConversationID,
		MessageID:      row.MessageID,
		HoldID:         row.HoldID,
		Status:         row.Status,
		ReasonPresent:  row.ReasonPresent,
		CreatedBy:      row.CreatedBy,
		CreatedAt:      row.CreatedAt.UTC().Format(time.RFC3339Nano),
		ReleasedBy:     row.ReleasedBy,
		ReleasedAt:     formatOptionalTimeRFC3339Nano(row.ReleasedAt),
		UpdatedAt:      row.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func writeMessageLegalHoldJSON(path string, output any) error {
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
