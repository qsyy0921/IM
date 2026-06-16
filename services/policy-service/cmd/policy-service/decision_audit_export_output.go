package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	postgresinfra "github.com/qsyy0921/IM/services/policy-service/internal/infrastructure/postgres"
)

type decisionAuditExportOutput struct {
	GeneratedAt string                         `json:"generated_at"`
	Filters     map[string]string              `json:"filters,omitempty"`
	Rows        []decisionAuditExportOutputRow `json:"rows"`
}

type decisionAuditExportOutputRow struct {
	EventID                  string `json:"event_id"`
	TenantID                 string `json:"tenant_id"`
	ActorUserKey             string `json:"actor_user_key"`
	DeviceKey                string `json:"device_key,omitempty"`
	ConversationKey          string `json:"conversation_key"`
	MessageKey               string `json:"message_key,omitempty"`
	Action                   string `json:"action"`
	MessageIDPresent         bool   `json:"message_id_present"`
	DirectPeerContextPresent bool   `json:"direct_peer_context_present"`
	DirectPeerKey            string `json:"direct_peer_key,omitempty"`
	Allowed                  bool   `json:"allowed"`
	PermissionVersion        int64  `json:"permission_version"`
	Classification           string `json:"classification"`
	ReasonCode               string `json:"reason_code,omitempty"`
	Status                   string `json:"status"`
	EventType                string `json:"event_type"`
	EventVersion             string `json:"event_version"`
	Producer                 string `json:"producer"`
	PartitionKey             string `json:"partition_key"`
	CorrelationID            string `json:"correlation_id,omitempty"`
	TraceID                  string `json:"trace_id,omitempty"`
	CreatedAt                string `json:"created_at"`
	PublishedAt              string `json:"published_at,omitempty"`
}

func writeDecisionAuditExportOutput(path string, rows []postgresinfra.DecisionAuditExportRow, filters map[string]string) error {
	output := decisionAuditExportOutput{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Filters:     compactCleanupFilters(filters),
		Rows:        make([]decisionAuditExportOutputRow, 0, len(rows)),
	}
	for _, row := range rows {
		outputRow := decisionAuditExportOutputRow{
			EventID:                  row.EventID,
			TenantID:                 row.TenantID,
			ActorUserKey:             row.ActorUserKey,
			DeviceKey:                row.DeviceKey,
			ConversationKey:          row.ConversationKey,
			MessageKey:               row.MessageKey,
			Action:                   row.Action,
			MessageIDPresent:         row.MessageIDPresent,
			DirectPeerContextPresent: row.DirectPeerContextPresent,
			DirectPeerKey:            row.DirectPeerKey,
			Allowed:                  row.Allowed,
			PermissionVersion:        row.PermissionVersion,
			Classification:           row.Classification,
			ReasonCode:               row.ReasonCode,
			Status:                   row.Status,
			EventType:                row.EventType,
			EventVersion:             row.EventVersion,
			Producer:                 row.Producer,
			PartitionKey:             row.PartitionKey,
			CorrelationID:            row.CorrelationID,
			TraceID:                  row.TraceID,
			CreatedAt:                formatOutboxAuditTime(row.CreatedAt),
		}
		if row.PublishedAt != nil {
			outputRow.PublishedAt = formatOutboxAuditTime(*row.PublishedAt)
		}
		output.Rows = append(output.Rows, outputRow)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}
