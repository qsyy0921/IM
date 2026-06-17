package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type operatorCleanupOutput struct {
	GeneratedAt      string            `json:"generated_at"`
	Deleted          int64             `json:"deleted"`
	DryRun           bool              `json:"dry_run"`
	Cutoff           string            `json:"cutoff"`
	RetentionSeconds int64             `json:"retention_seconds"`
	BatchSize        int               `json:"batch_size"`
	Filters          map[string]string `json:"filters,omitempty"`
}

func writeOperatorCleanupOutput(path string, deleted int64, cutoff time.Time, retention time.Duration, batchSize int, dryRun bool, filters map[string]string) error {
	output := operatorCleanupOutput{
		GeneratedAt:      time.Now().UTC().Format(time.RFC3339Nano),
		Deleted:          deleted,
		DryRun:           dryRun,
		Cutoff:           cutoff.UTC().Format(time.RFC3339Nano),
		RetentionSeconds: int64(retention.Seconds()),
		BatchSize:        batchSize,
		Filters:          compactCleanupFilters(filters),
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
