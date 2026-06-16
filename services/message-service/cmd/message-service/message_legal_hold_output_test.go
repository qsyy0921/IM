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

func TestWriteMessageLegalHoldAuditOutputOmitsReasonText(t *testing.T) {
	outputPath := filepath.Join(t.TempDir(), "nested", "legal-hold-audit.json")
	rows := []postgresinfra.MessageLegalHoldAuditRow{{
		TenantID:       "tenant-a",
		ConversationID: "conversation-a",
		MessageID:      "message-a",
		HoldID:         "hold-a",
		Status:         postgresinfra.MessageLegalHoldStatusActive,
		ReasonPresent:  true,
		CreatedBy:      "legal-ops",
		CreatedAt:      time.Date(2026, 6, 16, 1, 0, 0, 0, time.UTC),
		UpdatedAt:      time.Date(2026, 6, 16, 1, 0, 0, 0, time.UTC),
	}}
	if err := writeMessageLegalHoldAuditOutput(outputPath, rows); err != nil {
		t.Fatalf("write legal hold audit output: %v", err)
	}
	payload, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read legal hold audit output: %v", err)
	}
	if strings.Contains(string(payload), "secret") || strings.Contains(string(payload), "reason text") {
		t.Fatalf("legal hold audit output leaked reason text: %s", payload)
	}
	var output messageLegalHoldOutput
	if err := json.Unmarshal(payload, &output); err != nil {
		t.Fatalf("unmarshal legal hold audit output: %v", err)
	}
	if len(output.Rows) != 1 || !output.Rows[0].ReasonPresent || output.Rows[0].HoldID != "hold-a" {
		t.Fatalf("unexpected legal hold audit output: %+v", output)
	}
}

func TestWriteMessageLegalHoldMutationOutput(t *testing.T) {
	outputPath := filepath.Join(t.TempDir(), "legal-hold.json")
	releasedAt := time.Date(2026, 6, 16, 2, 0, 0, 0, time.UTC)
	row := postgresinfra.MessageLegalHoldMutationResult{
		TenantID:       "tenant-a",
		ConversationID: "conversation-a",
		MessageID:      "message-a",
		HoldID:         "hold-a",
		Status:         postgresinfra.MessageLegalHoldStatusReleased,
		ReasonPresent:  true,
		CreatedBy:      "legal-ops",
		CreatedAt:      time.Date(2026, 6, 16, 1, 0, 0, 0, time.UTC),
		ReleasedBy:     "legal-ops",
		ReleasedAt:     &releasedAt,
		UpdatedAt:      releasedAt,
	}
	if err := writeMessageLegalHoldMutationOutput(outputPath, row); err != nil {
		t.Fatalf("write legal hold mutation output: %v", err)
	}
	payload, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read legal hold mutation output: %v", err)
	}
	var output messageLegalHoldMutationOutput
	if err := json.Unmarshal(payload, &output); err != nil {
		t.Fatalf("unmarshal legal hold mutation output: %v", err)
	}
	if output.Row.Status != postgresinfra.MessageLegalHoldStatusReleased ||
		output.Row.ReleasedAt == "" ||
		output.Row.ReleasedBy != "legal-ops" {
		t.Fatalf("unexpected legal hold mutation output: %+v", output)
	}
}
