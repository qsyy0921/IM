package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	postgresinfra "github.com/qsyy0921/IM/services/contacts-service/internal/infrastructure/postgres"
)

func TestWriteOutboxAuditOutput(t *testing.T) {
	nextRetryAt := time.Date(2026, 6, 16, 9, 10, 0, 0, time.UTC)
	publishedAt := time.Date(2026, 6, 16, 9, 11, 0, 0, time.UTC)
	deadLetteredAt := time.Date(2026, 6, 16, 9, 12, 0, 0, time.UTC)
	availableAt := time.Date(2026, 6, 16, 9, 9, 0, 0, time.UTC)
	createdAt := time.Date(2026, 6, 16, 9, 8, 0, 0, time.UTC)
	outputPath := filepath.Join(t.TempDir(), "contacts-outbox-audit.json")

	err := writeOutboxAuditOutput(outputPath, []postgresinfra.OutboxAuditRow{
		{
			ID:               42,
			EventID:          "event-1",
			TenantID:         "tenant-a",
			AggregateType:    "contact",
			AggregateID:      "contact-1",
			AggregateVersion: 7,
			EventType:        "contacts.contact.created.v1",
			Status:           "DLQ",
			RetryCount:       5,
			LastError:        "contacts outbox publish failed",
			AvailableAt:      availableAt,
			NextRetryAt:      &nextRetryAt,
			PublishedAt:      &publishedAt,
			DeadLetteredAt:   &deadLetteredAt,
			CreatedAt:        createdAt,
		},
	}, map[string]string{
		"event_id":     "event-1",
		"tenant_id":    "tenant-a",
		"aggregate_id": "",
	})
	if err != nil {
		t.Fatalf("write outbox audit output: %v", err)
	}

	raw, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read outbox audit output: %v", err)
	}
	var output outboxAuditOutput
	if err := json.Unmarshal(raw, &output); err != nil {
		t.Fatalf("decode outbox audit output: %v", err)
	}
	if output.GeneratedAt == "" || len(output.Rows) != 1 {
		t.Fatalf("unexpected output header: %+v", output)
	}
	if output.Filters["event_id"] != "event-1" || output.Filters["tenant_id"] != "tenant-a" {
		t.Fatalf("unexpected filters: %+v", output.Filters)
	}
	if _, ok := output.Filters["aggregate_id"]; ok {
		t.Fatalf("empty filter should be compacted: %+v", output.Filters)
	}
	row := output.Rows[0]
	if row.ID != 42 ||
		row.EventID != "event-1" ||
		row.TenantID != "tenant-a" ||
		row.AggregateType != "contact" ||
		row.AggregateID != "contact-1" ||
		row.AggregateVersion != 7 ||
		row.EventType != "contacts.contact.created.v1" ||
		row.Status != "DLQ" ||
		row.RetryCount != 5 ||
		row.LastError != "contacts outbox publish failed" ||
		row.AvailableAt == "" ||
		row.NextRetryAt == "" ||
		row.PublishedAt == "" ||
		row.DeadLetteredAt == "" ||
		row.CreatedAt == "" {
		t.Fatalf("unexpected outbox audit output row: %+v", row)
	}
}
