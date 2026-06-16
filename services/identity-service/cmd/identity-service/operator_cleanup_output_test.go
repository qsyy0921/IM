package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWriteOperatorCleanupOutput(t *testing.T) {
	outputPath := filepath.Join(t.TempDir(), "identity-challenge-cleanup.json")
	cutoff := time.Date(2026, 6, 16, 11, 0, 0, 0, time.UTC)

	err := writeOperatorCleanupOutput(
		outputPath,
		7,
		cutoff,
		3*time.Hour,
		500,
		map[string]string{
			"tenant_id":     "tenant-a",
			"user_id":       "user-a",
			"challenge_id":  "",
			"delivery_id":   "42",
			"failure_class": "provider_unavailable",
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
		output.Cutoff == "" ||
		output.RetentionSeconds != int64((3*time.Hour).Seconds()) ||
		output.BatchSize != 500 ||
		output.Filters["tenant_id"] != "tenant-a" ||
		output.Filters["user_id"] != "user-a" ||
		output.Filters["delivery_id"] != "42" ||
		output.Filters["failure_class"] != "provider_unavailable" {
		t.Fatalf("unexpected operator cleanup output: %+v", output)
	}
	if _, ok := output.Filters["challenge_id"]; ok {
		t.Fatalf("expected empty challenge filter to be omitted: %+v", output.Filters)
	}
}
