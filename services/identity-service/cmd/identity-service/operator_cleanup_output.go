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
	Cutoff           string            `json:"cutoff"`
	RetentionSeconds int64             `json:"retention_seconds"`
	BatchSize        int               `json:"batch_size"`
	Filters          map[string]string `json:"filters,omitempty"`
}

func writeOperatorCleanupOutput(path string, deleted int64, cutoff time.Time, retention time.Duration, batchSize int, filters map[string]string) error {
	output := operatorCleanupOutput{
		GeneratedAt:      time.Now().UTC().Format(time.RFC3339Nano),
		Deleted:          deleted,
		Cutoff:           cutoff.UTC().Format(time.RFC3339Nano),
		RetentionSeconds: int64(retention.Seconds()),
		BatchSize:        batchSize,
		Filters:          compactOperatorCleanupFilters(filters),
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

func compactOperatorCleanupFilters(filters map[string]string) map[string]string {
	compacted := make(map[string]string, len(filters))
	for key, value := range filters {
		if value != "" {
			compacted[key] = value
		}
	}
	if len(compacted) == 0 {
		return nil
	}
	return compacted
}
