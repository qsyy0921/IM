package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	postgresinfra "github.com/qsyy0921/IM/services/message-service/internal/infrastructure/postgres"
)

type messageComplianceApprovalOutput struct {
	GeneratedAt string                               `json:"generated_at"`
	Filters     map[string]string                    `json:"filters,omitempty"`
	Rows        []messageComplianceApprovalOutputRow `json:"rows"`
}

type messageComplianceApprovalMutationOutput struct {
	GeneratedAt string                             `json:"generated_at"`
	Row         messageComplianceApprovalOutputRow `json:"row"`
}

type messageComplianceApprovalOutputRow struct {
	TenantID         string `json:"tenant_id"`
	ConversationID   string `json:"conversation_id"`
	MessageID        string `json:"message_id"`
	ApprovalID       string `json:"approval_id"`
	Status           string `json:"status"`
	ExternalProofRef string `json:"external_proof_ref"`
	ReasonPresent    bool   `json:"reason_present"`
	ApprovedBy       string `json:"approved_by"`
	ApprovedAt       string `json:"approved_at"`
	ConsumedBy       string `json:"consumed_by,omitempty"`
	ConsumedEventID  string `json:"consumed_event_id,omitempty"`
	ConsumedAt       string `json:"consumed_at,omitempty"`
	CanceledBy       string `json:"canceled_by,omitempty"`
	CanceledAt       string `json:"canceled_at,omitempty"`
	UpdatedAt        string `json:"updated_at"`
}

func writeMessageComplianceApprovalAuditOutput(path string, rows []postgresinfra.MessageComplianceDeleteApprovalAuditRow, filters map[string]string) error {
	output := messageComplianceApprovalOutput{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Filters:     compactCleanupFilters(filters),
		Rows:        make([]messageComplianceApprovalOutputRow, 0, len(rows)),
	}
	for _, row := range rows {
		output.Rows = append(output.Rows, messageComplianceApprovalOutputRowFromResult(row))
	}
	return writeMessageComplianceApprovalJSON(path, output)
}

func writeMessageComplianceApprovalMutationOutput(path string, row postgresinfra.MessageComplianceDeleteApprovalResult) error {
	output := messageComplianceApprovalMutationOutput{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Row:         messageComplianceApprovalOutputRowFromResult(row),
	}
	return writeMessageComplianceApprovalJSON(path, output)
}

func messageComplianceApprovalOutputRowFromResult(row postgresinfra.MessageComplianceDeleteApprovalResult) messageComplianceApprovalOutputRow {
	return messageComplianceApprovalOutputRow{
		TenantID:         row.TenantID,
		ConversationID:   row.ConversationID,
		MessageID:        row.MessageID,
		ApprovalID:       row.ApprovalID,
		Status:           row.Status,
		ExternalProofRef: row.ExternalProofRef,
		ReasonPresent:    row.ReasonPresent,
		ApprovedBy:       row.ApprovedBy,
		ApprovedAt:       row.ApprovedAt.UTC().Format(time.RFC3339Nano),
		ConsumedBy:       row.ConsumedBy,
		ConsumedEventID:  row.ConsumedEventID,
		ConsumedAt:       formatOptionalTimeRFC3339Nano(row.ConsumedAt),
		CanceledBy:       row.CanceledBy,
		CanceledAt:       formatOptionalTimeRFC3339Nano(row.CanceledAt),
		UpdatedAt:        row.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func writeMessageComplianceApprovalJSON(path string, output any) error {
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
