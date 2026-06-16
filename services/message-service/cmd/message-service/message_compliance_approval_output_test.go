package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	postgresinfra "github.com/qsyy0921/IM/services/message-service/internal/infrastructure/postgres"
)

func TestWriteMessageComplianceApprovalAuditOutputOmitsReasonText(t *testing.T) {
	outputPath := filepath.Join(t.TempDir(), "nested", "compliance-approval-audit.json")
	rows := []postgresinfra.MessageComplianceDeleteApprovalAuditRow{{
		TenantID:         "tenant-a",
		ConversationID:   "conversation-a",
		MessageID:        "message-a",
		ApprovalID:       "approval-a",
		Status:           postgresinfra.MessageComplianceApprovalStatusApproved,
		ExternalProofRef: "proof://case/a",
		ReasonPresent:    true,
		ApprovedBy:       "legal-approver",
		ApprovedAt:       time.Date(2026, 6, 16, 3, 0, 0, 0, time.UTC),
		UpdatedAt:        time.Date(2026, 6, 16, 3, 0, 0, 0, time.UTC),
	}}
	if err := writeMessageComplianceApprovalAuditOutput(outputPath, rows); err != nil {
		t.Fatalf("write compliance approval audit output: %v", err)
	}
	payload, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read compliance approval audit output: %v", err)
	}
	if strings.Contains(string(payload), "private legal reason") || strings.Contains(string(payload), "secret-token") {
		t.Fatalf("compliance approval audit output leaked reason text: %s", payload)
	}
	var output messageComplianceApprovalOutput
	if err := json.Unmarshal(payload, &output); err != nil {
		t.Fatalf("unmarshal compliance approval audit output: %v", err)
	}
	if len(output.Rows) != 1 ||
		output.Rows[0].ApprovalID != "approval-a" ||
		output.Rows[0].ExternalProofRef != "proof://case/a" ||
		!output.Rows[0].ReasonPresent {
		t.Fatalf("unexpected compliance approval audit output: %+v", output)
	}
}

func TestWriteMessageComplianceApprovalMutationOutput(t *testing.T) {
	outputPath := filepath.Join(t.TempDir(), "compliance-approval.json")
	consumedAt := time.Date(2026, 6, 16, 4, 0, 0, 0, time.UTC)
	row := postgresinfra.MessageComplianceDeleteApprovalResult{
		TenantID:         "tenant-a",
		ConversationID:   "conversation-a",
		MessageID:        "message-a",
		ApprovalID:       "approval-a",
		Status:           postgresinfra.MessageComplianceApprovalStatusConsumed,
		ExternalProofRef: "proof://case/a",
		ReasonPresent:    true,
		ApprovedBy:       "legal-approver",
		ApprovedAt:       time.Date(2026, 6, 16, 3, 0, 0, 0, time.UTC),
		ConsumedBy:       "compliance-admin",
		ConsumedEventID:  "event-a",
		ConsumedAt:       &consumedAt,
		UpdatedAt:        consumedAt,
	}
	if err := writeMessageComplianceApprovalMutationOutput(outputPath, row); err != nil {
		t.Fatalf("write compliance approval mutation output: %v", err)
	}
	payload, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read compliance approval mutation output: %v", err)
	}
	var output messageComplianceApprovalMutationOutput
	if err := json.Unmarshal(payload, &output); err != nil {
		t.Fatalf("unmarshal compliance approval mutation output: %v", err)
	}
	if output.Row.Status != postgresinfra.MessageComplianceApprovalStatusConsumed ||
		output.Row.ConsumedEventID != "event-a" ||
		output.Row.ConsumedAt == "" {
		t.Fatalf("unexpected compliance approval mutation output: %+v", output)
	}
}
