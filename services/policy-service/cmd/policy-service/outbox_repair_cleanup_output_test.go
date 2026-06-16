package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	postgresinfra "github.com/qsyy0921/IM/services/policy-service/internal/infrastructure/postgres"
)

func TestWriteOutboxRepairCleanupOutput(t *testing.T) {
	outputPath := filepath.Join(t.TempDir(), "policy-outbox-repair-cleanup.json")
	cutoff := time.Date(2026, 6, 16, 10, 0, 0, 0, time.UTC)

	err := writeOutboxRepairCleanupOutput(
		outputPath,
		postgresinfra.OutboxRepairCleanupStats{Deleted: 3},
		cutoff,
		2*time.Hour,
		200,
		true,
		map[string]string{"tenant_id": "tenant-a", "event_id": "", "operator": "operator-a", "outcome": "MUTATED"},
	)
	if err != nil {
		t.Fatalf("write cleanup output: %v", err)
	}

	raw, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read cleanup output: %v", err)
	}
	var output outboxRepairCleanupOutput
	if err := json.Unmarshal(raw, &output); err != nil {
		t.Fatalf("decode cleanup output: %v", err)
	}
	if output.GeneratedAt == "" ||
		output.Deleted != 3 ||
		output.Cutoff == "" ||
		output.RetentionSeconds != int64((2*time.Hour).Seconds()) ||
		output.BatchSize != 200 ||
		!output.DryRun ||
		output.Filters["tenant_id"] != "tenant-a" ||
		output.Filters["operator"] != "operator-a" ||
		output.Filters["outcome"] != "MUTATED" {
		t.Fatalf("unexpected cleanup output: %+v", output)
	}
	if _, ok := output.Filters["event_id"]; ok {
		t.Fatalf("expected empty event filter to be omitted: %+v", output.Filters)
	}
}
