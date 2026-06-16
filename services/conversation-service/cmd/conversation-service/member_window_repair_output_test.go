package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	postgresinfra "github.com/qsyy0921/IM/services/conversation-service/internal/infrastructure/postgres"
)

func TestWriteMemberWindowRepairOutput(t *testing.T) {
	outputPath := filepath.Join(t.TempDir(), "nested", "member-window-repair.json")
	stats := postgresinfra.MemberWindowRepairStats{Requested: 2, Repaired: 1, Skipped: 1, DryRun: false}
	options := postgresinfra.MemberWindowRepairOptions{
		TenantID:       "tenant-repair",
		ConversationID: "conv-repair",
		UserID:         "user-repair",
		IssueClass:     "ACTIVE_WITH_LEAVE_SEQ",
		DryRun:         false,
	}
	if err := writeMemberWindowRepairOutput(outputPath, stats, options); err != nil {
		t.Fatalf("writeMemberWindowRepairOutput() error = %v", err)
	}

	payload, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var output memberWindowRepairOutput
	if err := json.Unmarshal(payload, &output); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if output.GeneratedAt == "" ||
		output.Stats.Requested != 2 ||
		output.Stats.Repaired != 1 ||
		output.Options.TenantID != "tenant-repair" ||
		output.Options.IssueClass != "ACTIVE_WITH_LEAVE_SEQ" ||
		output.Options.DryRun {
		t.Fatalf("unexpected output: %+v", output)
	}
}

func TestWriteMemberWindowRepairAuditOutput(t *testing.T) {
	now := time.Date(2026, 6, 16, 15, 16, 17, 18, time.FixedZone("test", 8*60*60))
	outputPath := filepath.Join(t.TempDir(), "nested", "member-window-repair-audit.json")
	rows := []postgresinfra.MemberWindowRepairAuditRow{{
		ID:                       7,
		TenantID:                 "tenant-repair",
		ConversationID:           "conv-repair",
		UserID:                   "user-repair",
		IssueClass:               "ACTIVE_WITH_LEAVE_SEQ",
		RepairAction:             "clear_active_leave_seq",
		RepairOutcome:            "MUTATED",
		PreviousJoinSeq:          4,
		HasJoinSeq:               true,
		NewJoinSeq:               6,
		HasNewJoinSeq:            true,
		PreviousLeaveSeq:         9,
		HasLeaveSeq:              true,
		PreviousMemberVersion:    5,
		HasPreviousMemberVersion: true,
		NewMemberVersion:         12,
		HasNewMemberVersion:      true,
		ConversationStatus:       "ARCHIVED",
		PreviousMemberStatus:     "ACTIVE",
		NewMemberStatus:          "LEFT",
		OperatorID:               "operator-1",
		Reason:                   "clear stale leave seq",
		DryRun:                   false,
		RepairedAt:               now,
	}}
	if err := writeMemberWindowRepairAuditOutput(outputPath, rows); err != nil {
		t.Fatalf("writeMemberWindowRepairAuditOutput() error = %v", err)
	}

	payload, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var output memberWindowRepairAuditOutput
	if err := json.Unmarshal(payload, &output); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if output.GeneratedAt == "" || len(output.Rows) != 1 {
		t.Fatalf("unexpected output header: %+v", output)
	}
	row := output.Rows[0]
	if row.ID != 7 ||
		row.UserID != "user-repair" ||
		row.RepairOutcome != "MUTATED" ||
		row.PreviousJoinSeq != 4 ||
		row.NewJoinSeq != 6 ||
		row.PreviousLeaveSeq != 9 ||
		row.PreviousMemberVersion != 5 ||
		row.NewMemberVersion != 12 ||
		row.ConversationStatus != "ARCHIVED" ||
		row.PreviousMemberStatus != "ACTIVE" ||
		row.NewMemberStatus != "LEFT" ||
		!row.HasNewJoinSeq ||
		!row.HasNewMemberVersion ||
		row.HasNewLeaveSeq ||
		row.RepairedAt == "" {
		t.Fatalf("unexpected audit output row: %+v", row)
	}
}
