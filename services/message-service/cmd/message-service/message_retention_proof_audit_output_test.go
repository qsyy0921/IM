package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	postgresinfra "github.com/qsyy0921/IM/services/message-service/internal/infrastructure/postgres"
)

func TestWriteMessageRetentionProofAuditOutputOmitsPayloadAndReason(t *testing.T) {
	outputPath := filepath.Join(t.TempDir(), "message-retention-proof-audit.json")
	deleteVersion := 3
	deletedAt := time.Date(2026, 6, 16, 2, 3, 4, 0, time.UTC)
	changedAt := time.Date(2026, 6, 16, 2, 3, 5, 0, time.UTC)
	rows := []postgresinfra.MessageRetentionProofAuditRow{{
		TenantID:                   "tenant-1",
		ConversationID:             "conv-1",
		MessageID:                  "msg-1",
		ConversationSeq:            9,
		SenderID:                   "user-1",
		MessageType:                "TEXT",
		Status:                     "DELETED",
		CurrentPayloadPresent:      true,
		CreatedAt:                  time.Date(2026, 6, 16, 2, 0, 0, 0, time.UTC),
		DeletedAt:                  &deletedAt,
		DeleteChangeVersion:        &deleteVersion,
		DeleteChangedBy:            "user-1",
		DeleteReasonPresent:        true,
		DeleteBeforePayloadPresent: true,
		DeleteAfterPayloadPresent:  true,
		DeleteChangedAt:            &changedAt,
		DeleteTimelineEventPresent: true,
		DeleteOutboxEventPresent:   true,
	}}
	if err := writeMessageRetentionProofAuditOutput(outputPath, rows); err != nil {
		t.Fatalf("write retention proof audit output: %v", err)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read retention proof audit output: %v", err)
	}
	for _, forbidden := range []string{"payload_json", "secret current", "private retention reason"} {
		if stringContainsAny(string(data), []string{forbidden}) {
			t.Fatalf("retention proof output leaked %q: %s", forbidden, string(data))
		}
	}
	var output messageRetentionProofAuditOutput
	if err := json.Unmarshal(data, &output); err != nil {
		t.Fatalf("decode retention proof audit output: %v", err)
	}
	if len(output.Rows) != 1 {
		t.Fatalf("unexpected output row count: %+v", output)
	}
	row := output.Rows[0]
	if row.Status != "DELETED" ||
		!row.CurrentPayloadPresent ||
		row.DeleteChangeVersion == nil ||
		*row.DeleteChangeVersion != deleteVersion ||
		!row.DeleteReasonPresent ||
		!row.DeleteTimelineEventPresent ||
		!row.DeleteOutboxEventPresent {
		t.Fatalf("unexpected retention proof output row: %+v", row)
	}
}
