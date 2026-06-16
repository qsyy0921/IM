package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	postgresinfra "github.com/qsyy0921/IM/services/policy-service/internal/infrastructure/postgres"
)

func TestWriteOutboxRepairAuditOutput(t *testing.T) {
	deadLetteredAt := time.Date(2026, 6, 16, 9, 10, 0, 0, time.UTC)
	repairedAt := time.Date(2026, 6, 16, 9, 15, 0, 0, time.UTC)
	outputPath := filepath.Join(t.TempDir(), "policy-outbox-repair-audit.json")

	err := writeOutboxRepairAuditOutput(outputPath, []postgresinfra.OutboxRepairAuditRow{
		{
			EventID:                "event-1",
			TenantID:               "tenant-a",
			Operator:               "local-operator",
			Outcome:                "REPAIRED",
			Reason:                 "manual replay",
			PreviousStatus:         "DLQ",
			PreviousRetryCount:     5,
			PreviousLastError:      "policy outbox publish failed",
			PreviousDeadLetteredAt: &deadLetteredAt,
			RepairedAt:             repairedAt,
		},
	})
	if err != nil {
		t.Fatalf("write outbox repair audit output: %v", err)
	}

	raw, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read outbox repair audit output: %v", err)
	}
	var output struct {
		Rows []struct {
			EventID                string `json:"event_id"`
			TenantID               string `json:"tenant_id"`
			Operator               string `json:"operator"`
			Outcome                string `json:"outcome"`
			PreviousStatus         string `json:"previous_status"`
			PreviousRetryCount     int    `json:"previous_retry_count"`
			PreviousLastError      string `json:"previous_last_error"`
			PreviousDeadLetteredAt string `json:"previous_dead_lettered_at"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(raw, &output); err != nil {
		t.Fatalf("decode outbox repair audit output: %v", err)
	}
	if len(output.Rows) != 1 {
		t.Fatalf("unexpected row count: %+v", output)
	}
	row := output.Rows[0]
	if row.EventID != "event-1" ||
		row.TenantID != "tenant-a" ||
		row.Operator != "local-operator" ||
		row.Outcome != "REPAIRED" ||
		row.PreviousStatus != "DLQ" ||
		row.PreviousRetryCount != 5 ||
		row.PreviousLastError != "policy outbox publish failed" ||
		row.PreviousDeadLetteredAt == "" {
		t.Fatalf("unexpected repair audit output row: %+v", row)
	}
}
