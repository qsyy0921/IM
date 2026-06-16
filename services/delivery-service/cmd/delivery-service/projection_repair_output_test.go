package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/qsyy0921/IM/services/delivery-service/internal/types"
)

func TestWriteProjectionRepairOutput(t *testing.T) {
	outputPath := filepath.Join(t.TempDir(), "delivery-projection-repair.json")
	stats := types.ProjectionCheckpointRepairStats{
		Requested: 1,
		Audited:   0,
		Mutated:   1,
		Skipped:   0,
	}
	if err := writeProjectionRepairOutput(outputPath, stats, types.ProjectionCheckpointRepairModeRewindFailure, 0, 42, true); err != nil {
		t.Fatalf("writeProjectionRepairOutput() error = %v", err)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read projection repair output: %v", err)
	}
	var output projectionRepairOutput
	if err := json.Unmarshal(data, &output); err != nil {
		t.Fatalf("unmarshal projection repair output: %v", err)
	}
	if output.Mode != types.ProjectionCheckpointRepairModeRewindFailure || !output.DryRun {
		t.Fatalf("unexpected output mode/dry_run: %+v", output)
	}
	if output.FailureOffset != 42 || output.Requested != 1 || output.Mutated != 1 {
		t.Fatalf("unexpected projection repair summary: %+v", output)
	}
}
