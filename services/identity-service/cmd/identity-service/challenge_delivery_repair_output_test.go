package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/qsyy0921/IM/services/identity-service/internal/types"
)

func TestWriteChallengeDeliveryRepairOutput(t *testing.T) {
	outputPath := filepath.Join(t.TempDir(), "identity-challenge-delivery-repair.json")
	stats := types.ChallengeDeliveryRepairStats{
		Requested: 2,
		Audited:   0,
		Mutated:   1,
		Skipped:   1,
	}
	if err := writeChallengeDeliveryRepairOutput(outputPath, stats, 2, types.ChallengeDeliveryRepairModeRedriveActivePending, true); err != nil {
		t.Fatalf("writeChallengeDeliveryRepairOutput() error = %v", err)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read challenge delivery repair output: %v", err)
	}
	var output challengeDeliveryRepairOutput
	if err := json.Unmarshal(data, &output); err != nil {
		t.Fatalf("unmarshal challenge delivery repair output: %v", err)
	}
	if output.Mode != types.ChallengeDeliveryRepairModeRedriveActivePending || !output.DryRun {
		t.Fatalf("unexpected output mode/dry_run: %+v", output)
	}
	if output.DeliveryCount != 2 || output.Requested != 2 || output.Mutated != 1 || output.Skipped != 1 {
		t.Fatalf("unexpected challenge delivery repair summary: %+v", output)
	}
}
