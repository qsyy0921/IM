package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/qsyy0921/IM/services/delivery-service/internal/types"
)

type projectionFailureResolveOutput struct {
	GeneratedAt   string `json:"generated_at"`
	Requested     int    `json:"requested"`
	Audited       int    `json:"audited"`
	Resolved      int    `json:"resolved"`
	ConsumerGroup string `json:"consumer_group"`
	Topic         string `json:"topic"`
	PartitionID   int32  `json:"partition_id"`
	OffsetValue   int64  `json:"offset_value"`
	DryRun        bool   `json:"dry_run"`
	Operator      string `json:"operator"`
	ReasonPresent bool   `json:"reason_present"`
}

func writeProjectionFailureResolveOutput(path string, stats types.ProjectionFailureResolveStats, options types.ProjectionFailureResolveOptions) error {
	output := projectionFailureResolveOutput{
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339Nano),
		Requested:     stats.Requested,
		Audited:       stats.Audited,
		Resolved:      stats.Resolved,
		ConsumerGroup: options.ConsumerGroup,
		Topic:         options.Topic,
		PartitionID:   options.PartitionID,
		OffsetValue:   options.OffsetValue,
		DryRun:        options.DryRun,
		Operator:      options.Operator,
		ReasonPresent: options.Reason != "",
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
