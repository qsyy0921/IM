package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	postgresinfra "github.com/qsyy0921/IM/services/delivery-service/internal/infrastructure/postgres"
)

type projectionRepairAuditOutput struct {
	GeneratedAt string                           `json:"generated_at"`
	Rows        []projectionRepairAuditOutputRow `json:"rows"`
}

type projectionRepairAuditOutputRow struct {
	ConsumerGroup string `json:"consumer_group"`
	Topic         string `json:"topic"`
	PartitionID   int32  `json:"partition_id"`
	Mode          string `json:"mode"`
	Outcome       string `json:"outcome"`
	SkipReason    string `json:"skip_reason,omitempty"`
	Operator      string `json:"operator"`
	Reason        string `json:"reason"`
	DryRun        bool   `json:"dry_run"`
	BeforeOffset  int64  `json:"before_offset"`
	AfterOffset   int64  `json:"after_offset"`
	FailureOffset *int64 `json:"failure_offset,omitempty"`
	FailureEvent  string `json:"failure_event_id,omitempty"`
	FailureClass  string `json:"failure_class,omitempty"`
	CreatedAt     string `json:"created_at"`
}

func writeProjectionRepairAuditOutput(path string, rows []postgresinfra.ProjectionRepairAuditRow) error {
	output := projectionRepairAuditOutput{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Rows:        make([]projectionRepairAuditOutputRow, 0, len(rows)),
	}
	for _, row := range rows {
		output.Rows = append(output.Rows, projectionRepairAuditOutputRow{
			ConsumerGroup: row.ConsumerGroup,
			Topic:         row.Topic,
			PartitionID:   row.PartitionID,
			Mode:          row.Mode,
			Outcome:       row.Outcome,
			SkipReason:    row.SkipReason,
			Operator:      row.Operator,
			Reason:        row.Reason,
			DryRun:        row.DryRun,
			BeforeOffset:  row.BeforeOffset,
			AfterOffset:   row.AfterOffset,
			FailureOffset: row.FailureOffset,
			FailureEvent:  row.FailureEvent,
			FailureClass:  row.FailureClass,
			CreatedAt:     row.CreatedAt.UTC().Format(time.RFC3339Nano),
		})
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
