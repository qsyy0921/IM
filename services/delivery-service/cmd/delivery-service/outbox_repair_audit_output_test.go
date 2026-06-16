package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	postgresinfra "github.com/qsyy0921/IM/services/delivery-service/internal/infrastructure/postgres"
)

func TestWriteOutboxRepairAuditOutput(t *testing.T) {
	beforeDeadLetteredAt := time.Date(2026, 6, 16, 9, 10, 0, 0, time.UTC)
	afterNextRetryAt := time.Date(2026, 6, 16, 9, 20, 0, 0, time.UTC)
	createdAt := time.Date(2026, 6, 16, 9, 15, 0, 0, time.UTC)
	outputPath := filepath.Join(t.TempDir(), "delivery-outbox-repair-audit.json")

	err := writeOutboxRepairAuditOutput(outputPath, []postgresinfra.OutboxRepairAuditRow{
		{
			OutboxID:             42,
			EventID:              "event-1",
			TenantID:             "tenant-a",
			ConversationID:       "conv-1",
			AggregateVersion:     7,
			Mode:                 "REDRIVE",
			Outcome:              "MUTATED",
			Operator:             "local-operator",
			Reason:               "manual replay",
			DryRun:               false,
			BeforeStatus:         "DLQ",
			BeforeRetryCount:     5,
			BeforeLastError:      "delivery outbox publish failed",
			BeforeDeadLetteredAt: &beforeDeadLetteredAt,
			AfterStatus:          "PENDING",
			AfterRetryCount:      0,
			AfterNextRetryAt:     &afterNextRetryAt,
			CreatedAt:            createdAt,
		},
	}, map[string]string{
		"outbox_id":       "42",
		"repaired_after":  "2026-06-16T09:00:00Z",
		"repaired_before": "",
	})
	if err != nil {
		t.Fatalf("write outbox repair audit output: %v", err)
	}

	raw, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read outbox repair audit output: %v", err)
	}
	var output struct {
		Filters map[string]string `json:"filters"`
		Rows    []struct {
			OutboxID             int64  `json:"outbox_id"`
			EventID              string `json:"event_id"`
			TenantID             string `json:"tenant_id"`
			ConversationID       string `json:"conversation_id"`
			Mode                 string `json:"mode"`
			Outcome              string `json:"outcome"`
			BeforeStatus         string `json:"before_status"`
			BeforeRetryCount     int    `json:"before_retry_count"`
			BeforeLastError      string `json:"before_last_error"`
			BeforeDeadLetteredAt string `json:"before_dead_lettered_at"`
			AfterStatus          string `json:"after_status"`
			AfterRetryCount      int    `json:"after_retry_count"`
			AfterNextRetryAt     string `json:"after_next_retry_at"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(raw, &output); err != nil {
		t.Fatalf("decode outbox repair audit output: %v", err)
	}
	if len(output.Rows) != 1 {
		t.Fatalf("unexpected row count: %+v", output)
	}
	if output.Filters["outbox_id"] != "42" ||
		output.Filters["repaired_after"] != "2026-06-16T09:00:00Z" {
		t.Fatalf("unexpected filters: %+v", output.Filters)
	}
	if _, ok := output.Filters["repaired_before"]; ok {
		t.Fatalf("expected empty filter to be compacted: %+v", output.Filters)
	}
	row := output.Rows[0]
	if row.OutboxID != 42 ||
		row.EventID != "event-1" ||
		row.TenantID != "tenant-a" ||
		row.ConversationID != "conv-1" ||
		row.Mode != "REDRIVE" ||
		row.Outcome != "MUTATED" ||
		row.BeforeStatus != "DLQ" ||
		row.BeforeRetryCount != 5 ||
		row.BeforeLastError != "delivery outbox publish failed" ||
		row.BeforeDeadLetteredAt == "" ||
		row.AfterStatus != "PENDING" ||
		row.AfterRetryCount != 0 ||
		row.AfterNextRetryAt == "" {
		t.Fatalf("unexpected repair audit output row: %+v", row)
	}
}
