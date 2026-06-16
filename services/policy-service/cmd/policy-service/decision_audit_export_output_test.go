package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	postgresinfra "github.com/qsyy0921/IM/services/policy-service/internal/infrastructure/postgres"
)

func TestWriteDecisionAuditExportOutput(t *testing.T) {
	outputPath := filepath.Join(t.TempDir(), "policy-decision-audit-export.json")
	publishedAt := time.Date(2026, 6, 17, 9, 40, 0, 0, time.UTC)

	err := writeDecisionAuditExportOutput(outputPath, []postgresinfra.DecisionAuditExportRow{
		{
			EventID:                  "event-1",
			TenantID:                 "tenant-a",
			ActorUserKey:             "actor-key",
			DeviceKey:                "device-key",
			ConversationKey:          "conversation-key",
			MessageKey:               "message-key",
			Action:                   "SEND",
			MessageIDPresent:         true,
			DirectPeerContextPresent: true,
			DirectPeerKey:            "peer-key",
			Allowed:                  false,
			PermissionVersion:        43,
			Classification:           "CONTENT_PROVIDER_DENIED",
			ReasonCode:               "CONTENT_PROVIDER_DENIED",
			Status:                   "PUBLISHED",
			EventType:                "policy.message_action_decision.v1",
			EventVersion:             "v1",
			Producer:                 "policy-service",
			PartitionKey:             "tenant-a:conversation-key",
			CorrelationID:            "request-1",
			TraceID:                  "trace-1",
			CreatedAt:                time.Date(2026, 6, 17, 9, 39, 0, 0, time.UTC),
			PublishedAt:              &publishedAt,
		},
	}, map[string]string{
		"tenant_id":      "tenant-a",
		"allowed":        "false",
		"created_after":  "2026-06-17T09:00:00Z",
		"created_before": "2026-06-17T10:00:00Z",
		"event_id":       "",
	})
	if err != nil {
		t.Fatalf("write decision audit export output: %v", err)
	}

	raw, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read decision audit export output: %v", err)
	}
	var output decisionAuditExportOutput
	if err := json.Unmarshal(raw, &output); err != nil {
		t.Fatalf("decode decision audit export output: %v", err)
	}
	if output.GeneratedAt == "" || len(output.Rows) != 1 ||
		output.Filters["tenant_id"] != "tenant-a" ||
		output.Filters["created_after"] != "2026-06-17T09:00:00Z" ||
		output.Filters["event_id"] != "" {
		t.Fatalf("unexpected output header: %+v", output)
	}
	row := output.Rows[0]
	if row.EventID != "event-1" ||
		row.ActorUserKey != "actor-key" ||
		row.Allowed ||
		row.PermissionVersion != 43 ||
		row.Classification != "CONTENT_PROVIDER_DENIED" ||
		row.PublishedAt == "" ||
		row.CreatedAt == "" {
		t.Fatalf("unexpected decision audit export row: %+v", row)
	}
	for _, forbidden := range []string{"user@example.com", "secret-token", "provider body"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("decision audit export leaked forbidden text %q: %s", forbidden, raw)
		}
	}
}
