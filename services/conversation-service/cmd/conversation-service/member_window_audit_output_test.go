package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	postgresinfra "github.com/qsyy0921/IM/services/conversation-service/internal/infrastructure/postgres"
)

func TestWriteMemberWindowAuditOutput(t *testing.T) {
	now := time.Date(2026, 6, 16, 13, 14, 15, 16, time.FixedZone("test", 8*60*60))
	outputPath := filepath.Join(t.TempDir(), "nested", "member-window-audit.json")

	err := writeMemberWindowAuditOutput(outputPath, []postgresinfra.MemberWindowAuditRow{
		{
			TenantID:                      "tenant-window",
			ConversationID:                "conv-window",
			UserID:                        "user-window",
			Role:                          "MEMBER",
			Status:                        "ACTIVE",
			JoinSeq:                       0,
			HasJoinSeq:                    false,
			LeaveSeq:                      0,
			HasLeaveSeq:                   false,
			MemberVersion:                 12,
			PermissionVersion:             13,
			ConversationMemberVersion:     10,
			ConversationPermissionVersion: 13,
			ConversationStatus:            "ACTIVE",
			IssueClass:                    "MEMBER_VERSION_AHEAD_CONVERSATION",
			UpdatedAt:                     now,
		},
	}, map[string]string{
		"tenant_id":     "tenant-window",
		"issue_class":   "MEMBER_VERSION_AHEAD_CONVERSATION",
		"updated_after": "",
	})
	if err != nil {
		t.Fatalf("writeMemberWindowAuditOutput() error = %v", err)
	}

	payload, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var output memberWindowAuditOutput
	if err := json.Unmarshal(payload, &output); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if output.GeneratedAt == "" {
		t.Fatal("generated_at is empty")
	}
	if output.Filters["tenant_id"] != "tenant-window" ||
		output.Filters["issue_class"] != "MEMBER_VERSION_AHEAD_CONVERSATION" ||
		len(output.Filters) != 2 {
		t.Fatalf("unexpected filters: %+v", output.Filters)
	}
	if _, ok := output.Filters["updated_after"]; ok {
		t.Fatalf("empty filter should be omitted: %+v", output.Filters)
	}
	if len(output.Rows) != 1 {
		t.Fatalf("rows length = %d, want 1", len(output.Rows))
	}
	row := output.Rows[0]
	if row.TenantID != "tenant-window" ||
		row.ConversationID != "conv-window" ||
		row.UserID != "user-window" ||
		row.IssueClass != "MEMBER_VERSION_AHEAD_CONVERSATION" ||
		row.MemberVersion != 12 ||
		row.ConversationMemberVersion != 10 ||
		row.HasJoinSeq ||
		row.HasLeaveSeq ||
		row.UpdatedAt == "" {
		t.Fatalf("unexpected row: %+v", row)
	}
}
