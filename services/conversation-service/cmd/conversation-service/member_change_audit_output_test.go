package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	postgresinfra "github.com/qsyy0921/IM/services/conversation-service/internal/infrastructure/postgres"
)

func TestWriteMemberChangeAuditOutput(t *testing.T) {
	now := time.Date(2026, 6, 16, 10, 11, 12, 13, time.FixedZone("test", 8*60*60))
	nextRetryAt := now.Add(time.Minute)
	deadLetteredAt := now.Add(2 * time.Minute)
	completedAt := now.Add(3 * time.Minute)
	outputPath := filepath.Join(t.TempDir(), "nested", "member-change-audit.json")

	err := writeMemberChangeAuditOutput(outputPath, []postgresinfra.MemberChangeAuditRow{
		{
			ChangeID:        "change-1",
			TenantID:        "tenant-a",
			ConversationID:  "conv-1",
			TargetUserID:    "target-user",
			OperatorUserID:  "operator-user",
			ChangeType:      "JOIN",
			Status:          "DONE",
			BoundarySeq:     7,
			TimelineEventID: "timeline-event",
			OutboxEventID:   "outbox-event",
			RetryCount:      2,
			LastError:       "member change processing failed",
			NextRetryAt:     &nextRetryAt,
			DeadLetteredAt:  &deadLetteredAt,
			CompletedAt:     &completedAt,
			CreatedAt:       now,
			UpdatedAt:       now.Add(4 * time.Minute),
		},
	})
	if err != nil {
		t.Fatalf("writeMemberChangeAuditOutput() error = %v", err)
	}

	payload, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var output memberChangeAuditOutput
	if err := json.Unmarshal(payload, &output); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if output.GeneratedAt == "" {
		t.Fatal("generated_at is empty")
	}
	if len(output.Rows) != 1 {
		t.Fatalf("rows length = %d, want 1", len(output.Rows))
	}
	row := output.Rows[0]
	if row.ChangeID != "change-1" ||
		row.TenantID != "tenant-a" ||
		row.ConversationID != "conv-1" ||
		row.TargetUserID != "target-user" ||
		row.OperatorUserID != "operator-user" ||
		row.ChangeType != "JOIN" ||
		row.Status != "DONE" ||
		row.BoundarySeq != 7 ||
		row.TimelineEventID != "timeline-event" ||
		row.OutboxEventID != "outbox-event" ||
		row.RetryCount != 2 ||
		row.LastError != "member change processing failed" {
		t.Fatalf("unexpected row: %+v", row)
	}
	if row.NextRetryAt == "" || row.DeadLetteredAt == "" || row.CompletedAt == "" || row.CreatedAt == "" || row.UpdatedAt == "" {
		t.Fatalf("expected all timestamp fields to be populated: %+v", row)
	}
}
