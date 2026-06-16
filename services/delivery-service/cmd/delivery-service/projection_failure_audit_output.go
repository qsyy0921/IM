package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	postgresinfra "github.com/qsyy0921/IM/services/delivery-service/internal/infrastructure/postgres"
)

type projectionFailureAuditOutput struct {
	GeneratedAt     string                            `json:"generated_at"`
	IncludeResolved bool                              `json:"include_resolved"`
	UnresolvedCount int                               `json:"unresolved_count"`
	Filters         map[string]string                 `json:"filters,omitempty"`
	Rows            []projectionFailureAuditOutputRow `json:"rows"`
}

type projectionFailureAuditOutputRow struct {
	ConsumerGroup            string `json:"consumer_group"`
	Topic                    string `json:"topic"`
	PartitionID              int32  `json:"partition_id"`
	OffsetValue              int64  `json:"offset_value"`
	EventID                  string `json:"event_id"`
	EventType                string `json:"event_type"`
	FailureClass             string `json:"failure_class"`
	FailureCount             int64  `json:"failure_count"`
	LastError                string `json:"last_error"`
	LastSeenAt               string `json:"last_seen_at"`
	Resolved                 bool   `json:"resolved"`
	ResolvedAt               string `json:"resolved_at,omitempty"`
	ResolvedCheckpointOffset *int64 `json:"resolved_checkpoint_offset,omitempty"`
}

func writeProjectionFailureAuditOutput(path string, rows []postgresinfra.ProjectionFailureAuditRow, includeResolved bool, filters map[string]string) error {
	output := projectionFailureAuditOutput{
		GeneratedAt:     time.Now().UTC().Format(time.RFC3339Nano),
		IncludeResolved: includeResolved,
		Filters:         compactCleanupFilters(filters),
		Rows:            make([]projectionFailureAuditOutputRow, 0, len(rows)),
	}
	for _, row := range rows {
		outputRow := projectionFailureAuditOutputRow{
			ConsumerGroup:            row.ConsumerGroup,
			Topic:                    row.Topic,
			PartitionID:              row.PartitionID,
			OffsetValue:              row.OffsetValue,
			EventID:                  row.EventID,
			EventType:                row.EventType,
			FailureClass:             row.FailureClass,
			FailureCount:             row.FailureCount,
			LastError:                row.LastError,
			LastSeenAt:               row.LastSeenAt.UTC().Format(time.RFC3339Nano),
			ResolvedCheckpointOffset: row.ResolvedCheckpointOffset,
		}
		if row.ResolvedAt != nil {
			outputRow.Resolved = true
			outputRow.ResolvedAt = row.ResolvedAt.UTC().Format(time.RFC3339Nano)
		} else {
			output.UnresolvedCount++
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
