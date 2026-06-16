package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	postgresinfra "github.com/qsyy0921/IM/services/conversation-service/internal/infrastructure/postgres"
)

type memberWindowAuditOutput struct {
	GeneratedAt string                       `json:"generated_at"`
	Rows        []memberWindowAuditOutputRow `json:"rows"`
}

type memberWindowAuditOutputRow struct {
	TenantID                      string `json:"tenant_id"`
	ConversationID                string `json:"conversation_id"`
	UserID                        string `json:"user_id"`
	Role                          string `json:"role"`
	Status                        string `json:"status"`
	JoinSeq                       int64  `json:"join_seq,omitempty"`
	HasJoinSeq                    bool   `json:"has_join_seq"`
	LeaveSeq                      int64  `json:"leave_seq,omitempty"`
	HasLeaveSeq                   bool   `json:"has_leave_seq"`
	MemberVersion                 int64  `json:"member_version"`
	PermissionVersion             int64  `json:"permission_version"`
	ConversationMemberVersion     int64  `json:"conversation_member_version"`
	ConversationPermissionVersion int64  `json:"conversation_permission_version"`
	ConversationStatus            string `json:"conversation_status"`
	IssueClass                    string `json:"issue_class"`
	UpdatedAt                     string `json:"updated_at"`
}

func writeMemberWindowAuditOutput(path string, rows []postgresinfra.MemberWindowAuditRow) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	output := memberWindowAuditOutput{
		GeneratedAt: formatAuditOutputTime(time.Now()),
		Rows:        make([]memberWindowAuditOutputRow, 0, len(rows)),
	}
	for _, row := range rows {
		output.Rows = append(output.Rows, memberWindowAuditOutputRow{
			TenantID:                      row.TenantID,
			ConversationID:                row.ConversationID,
			UserID:                        row.UserID,
			Role:                          row.Role,
			Status:                        row.Status,
			JoinSeq:                       row.JoinSeq,
			HasJoinSeq:                    row.HasJoinSeq,
			LeaveSeq:                      row.LeaveSeq,
			HasLeaveSeq:                   row.HasLeaveSeq,
			MemberVersion:                 row.MemberVersion,
			PermissionVersion:             row.PermissionVersion,
			ConversationMemberVersion:     row.ConversationMemberVersion,
			ConversationPermissionVersion: row.ConversationPermissionVersion,
			ConversationStatus:            row.ConversationStatus,
			IssueClass:                    row.IssueClass,
			UpdatedAt:                     formatAuditOutputTime(row.UpdatedAt),
		})
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}
