package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	postgresinfra "github.com/qsyy0921/IM/services/conversation-service/internal/infrastructure/postgres"
)

type memberWindowRepairOutput struct {
	GeneratedAt string                                `json:"generated_at"`
	Stats       postgresinfra.MemberWindowRepairStats `json:"stats"`
	Options     memberWindowRepairOutputOptions       `json:"options"`
}

type memberWindowRepairOutputOptions struct {
	TenantID       string `json:"tenant_id,omitempty"`
	ConversationID string `json:"conversation_id,omitempty"`
	UserID         string `json:"user_id,omitempty"`
	IssueClass     string `json:"issue_class"`
	DryRun         bool   `json:"dry_run"`
}

type memberWindowRepairAuditOutput struct {
	GeneratedAt string                             `json:"generated_at"`
	Rows        []memberWindowRepairAuditOutputRow `json:"rows"`
}

type memberWindowRepairAuditOutputRow struct {
	ID               int64  `json:"id"`
	TenantID         string `json:"tenant_id"`
	ConversationID   string `json:"conversation_id"`
	UserID           string `json:"user_id"`
	IssueClass       string `json:"issue_class"`
	RepairAction     string `json:"repair_action"`
	RepairOutcome    string `json:"repair_outcome"`
	PreviousJoinSeq  int64  `json:"previous_join_seq,omitempty"`
	HasJoinSeq       bool   `json:"has_join_seq"`
	PreviousLeaveSeq int64  `json:"previous_leave_seq,omitempty"`
	HasLeaveSeq      bool   `json:"has_leave_seq"`
	NewLeaveSeq      int64  `json:"new_leave_seq,omitempty"`
	HasNewLeaveSeq   bool   `json:"has_new_leave_seq"`
	OperatorID       string `json:"operator_id"`
	Reason           string `json:"reason,omitempty"`
	DryRun           bool   `json:"dry_run"`
	RepairedAt       string `json:"repaired_at"`
}

func writeMemberWindowRepairOutput(path string, stats postgresinfra.MemberWindowRepairStats, options postgresinfra.MemberWindowRepairOptions) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	output := memberWindowRepairOutput{
		GeneratedAt: formatAuditOutputTime(time.Now()),
		Stats:       stats,
		Options: memberWindowRepairOutputOptions{
			TenantID:       options.TenantID,
			ConversationID: options.ConversationID,
			UserID:         options.UserID,
			IssueClass:     options.IssueClass,
			DryRun:         options.DryRun,
		},
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}

func writeMemberWindowRepairAuditOutput(path string, rows []postgresinfra.MemberWindowRepairAuditRow) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	output := memberWindowRepairAuditOutput{
		GeneratedAt: formatAuditOutputTime(time.Now()),
		Rows:        make([]memberWindowRepairAuditOutputRow, 0, len(rows)),
	}
	for _, row := range rows {
		output.Rows = append(output.Rows, memberWindowRepairAuditOutputRow{
			ID:               row.ID,
			TenantID:         row.TenantID,
			ConversationID:   row.ConversationID,
			UserID:           row.UserID,
			IssueClass:       row.IssueClass,
			RepairAction:     row.RepairAction,
			RepairOutcome:    row.RepairOutcome,
			PreviousJoinSeq:  row.PreviousJoinSeq,
			HasJoinSeq:       row.HasJoinSeq,
			PreviousLeaveSeq: row.PreviousLeaveSeq,
			HasLeaveSeq:      row.HasLeaveSeq,
			NewLeaveSeq:      row.NewLeaveSeq,
			HasNewLeaveSeq:   row.HasNewLeaveSeq,
			OperatorID:       row.OperatorID,
			Reason:           row.Reason,
			DryRun:           row.DryRun,
			RepairedAt:       formatAuditOutputTime(row.RepairedAt),
		})
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}
