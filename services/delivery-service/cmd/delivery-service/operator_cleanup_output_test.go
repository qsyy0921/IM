package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWriteOperatorCleanupOutput(t *testing.T) {
	outputPath := filepath.Join(t.TempDir(), "delivery-projection-cleanup.json")
	cutoff := time.Date(2026, 6, 16, 11, 0, 0, 0, time.UTC)

	err := writeOperatorCleanupOutput(
		outputPath,
		7,
		cutoff,
		3*time.Hour,
		500,
		true,
		map[string]string{
			"consumer_group": "delivery-consumer",
			"topic":          "conversation.timeline.events",
			"partition_id":   "2",
			"failure_class":  "",
		},
	)
	if err != nil {
		t.Fatalf("write operator cleanup output: %v", err)
	}

	raw, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read operator cleanup output: %v", err)
	}
	var output operatorCleanupOutput
	if err := json.Unmarshal(raw, &output); err != nil {
		t.Fatalf("decode operator cleanup output: %v", err)
	}
	if output.GeneratedAt == "" ||
		output.Deleted != 7 ||
		!output.DryRun ||
		output.Cutoff == "" ||
		output.RetentionSeconds != int64((3*time.Hour).Seconds()) ||
		output.BatchSize != 500 ||
		output.Filters["consumer_group"] != "delivery-consumer" ||
		output.Filters["topic"] != "conversation.timeline.events" ||
		output.Filters["partition_id"] != "2" {
		t.Fatalf("unexpected operator cleanup output: %+v", output)
	}
	if _, ok := output.Filters["failure_class"]; ok {
		t.Fatalf("expected empty failure class filter to be omitted: %+v", output.Filters)
	}
}
