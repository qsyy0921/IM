package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/qsyy0921/IM/services/delivery-service/internal/types"
)

type projectionRepairOutput struct {
	GeneratedAt   string `json:"generated_at"`
	Mode          string `json:"mode"`
	TargetOffset  int64  `json:"target_offset"`
	FailureOffset int64  `json:"failure_offset"`
	DryRun        bool   `json:"dry_run"`
	Requested     int    `json:"requested"`
	Audited       int    `json:"audited"`
	Mutated       int    `json:"mutated"`
	Skipped       int    `json:"skipped"`
}

func writeProjectionRepairOutput(path string, stats types.ProjectionCheckpointRepairStats, mode string, targetOffset int64, failureOffset int64, dryRun bool) error {
	output := projectionRepairOutput{
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339Nano),
		Mode:          mode,
		TargetOffset:  targetOffset,
		FailureOffset: failureOffset,
		DryRun:        dryRun,
		Requested:     stats.Requested,
		Audited:       stats.Audited,
		Mutated:       stats.Mutated,
		Skipped:       stats.Skipped,
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
