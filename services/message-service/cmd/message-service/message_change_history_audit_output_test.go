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

func TestWriteMessageChangeHistoryAuditOutputOmitsPayloadAndReason(t *testing.T) {
	outputPath := filepath.Join(t.TempDir(), "message-change-history-audit.json")
	rows := []postgresinfra.MessageChangeHistoryAuditRow{{
		TenantID:             "tenant-1",
		ConversationID:       "conv-1",
		MessageID:            "msg-1",
		ChangeVersion:        2,
		ChangeType:           "DELETE",
		BeforePayloadPresent: true,
		AfterPayloadPresent:  true,
		BeforeStatus:         "NORMAL",
		AfterStatus:          "DELETED",
		ChangedBy:            "user-1",
		ReasonPresent:        true,
		TraceID:              "trace-1",
		ChangedAt:            time.Date(2026, 6, 16, 1, 2, 3, 0, time.UTC),
	}}
	if err := writeMessageChangeHistoryAuditOutput(outputPath, rows); err != nil {
		t.Fatalf("writeMessageChangeHistoryAuditOutput() error = %v", err)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read message change history audit output: %v", err)
	}
	if string(data) == "" {
		t.Fatalf("expected output data")
	}
	if stringContainsAny(string(data), []string{"before_payload_json", "after_payload_json", "cleanup because user asked"}) {
		t.Fatalf("output leaked payload or reason fields: %s", string(data))
	}
	var output messageChangeHistoryAuditOutput
	if err := json.Unmarshal(data, &output); err != nil {
		t.Fatalf("unmarshal message change history audit output: %v", err)
	}
	if len(output.Rows) != 1 || output.Rows[0].ChangeType != "DELETE" || !output.Rows[0].ReasonPresent {
		t.Fatalf("unexpected output: %+v", output)
	}
}

func stringContainsAny(value string, needles []string) bool {
	for _, needle := range needles {
		if needle != "" && strings.Contains(value, needle) {
			return true
		}
	}
	return false
}
