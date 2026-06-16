package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/qsyy0921/IM/services/delivery-service/internal/types"
)

func TestWriteOutboxRepairOutput(t *testing.T) {
	outputPath := filepath.Join(t.TempDir(), "nested", "repair.json")
	stats := types.OutboxRepairStats{Requested: 4, Audited: 3, Mutated: 2, Skipped: 1}

	if err := writeOutboxRepairOutput(outputPath, stats, 5, types.OutboxRepairModeRedriveDLQPending, true); err != nil {
		t.Fatalf("writeOutboxRepairOutput() error = %v", err)
	}

	raw, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var output outboxRepairOutput
	if err := json.Unmarshal(raw, &output); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if output.GeneratedAt == "" ||
		output.OutboxCount != 5 ||
		output.Mode != types.OutboxRepairModeRedriveDLQPending ||
		!output.DryRun ||
		output.Requested != 4 ||
		output.Audited != 3 ||
		output.Mutated != 2 ||
		output.Skipped != 1 {
		t.Fatalf("unexpected repair output: %+v", output)
	}
}
